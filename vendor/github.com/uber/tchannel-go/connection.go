// Copyright (c) 2015 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tchannel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/uber/tchannel-go/tos"

	"go.uber.org/atomic"
	"golang.org/x/net/context"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	// CurrentProtocolVersion is the current version of the TChannel protocol
	// supported by this stack
	CurrentProtocolVersion = 0x02

	// DefaultConnectTimeout is the default timeout used by net.Dial, if no timeout
	// is specified in the context.
	DefaultConnectTimeout = 5 * time.Second

	// defaultConnectionBufferSize is the default size for the connection's
	// read and write channels.
	defaultConnectionBufferSize = 512
)

// PeerVersion contains version related information for a specific peer.
// These values are extracted from the init headers.
type PeerVersion struct {
	Language        string `json:"language"`
	LanguageVersion string `json:"languageVersion"`
	TChannelVersion string `json:"tchannelVersion"`
}

// PeerInfo contains information about a TChannel peer
type PeerInfo struct {
	// The host and port that can be used to contact the peer, as encoded by net.JoinHostPort
	HostPort string `json:"hostPort"`

	// The logical process name for the peer, used for only for logging / debugging
	ProcessName string `json:"processName"`

	// IsEphemeral returns whether the remote host:port is ephemeral (e.g. not listening).
	IsEphemeral bool `json:"isEphemeral"`

	// Version returns the version information for the remote peer.
	Version PeerVersion `json:"version"`
}

func (p PeerInfo) String() string {
	return fmt.Sprintf("%s(%s)", p.HostPort, p.ProcessName)
}

// IsEphemeralHostPort returns whether the connection is from an ephemeral host:port.
func (p PeerInfo) IsEphemeralHostPort() bool {
	return p.IsEphemeral
}

// LocalPeerInfo adds service name to the peer info, only required for the local peer.
type LocalPeerInfo struct {
	PeerInfo

	// ServiceName is the service name for the local peer.
	ServiceName string `json:"serviceName"`
}

func (p LocalPeerInfo) String() string {
	return fmt.Sprintf("%v: %v", p.ServiceName, p.PeerInfo)
}

var (
	// ErrConnectionClosed is returned when a caller performs an method
	// on a closed connection
	ErrConnectionClosed = errors.New("connection is closed")

	// ErrSendBufferFull is returned when a message cannot be sent to the
	// peer because the frame sending buffer has become full.  Typically
	// this indicates that the connection is stuck and writes have become
	// backed up
	ErrSendBufferFull = errors.New("connection send buffer is full, cannot send frame")

	// ErrConnectionNotReady is no longer used.
	ErrConnectionNotReady = errors.New("connection is not yet ready")
)

// errConnectionInvalidState is returned when the connection is in an unknown state.
type errConnectionUnknownState struct {
	site  string
	state connectionState
}

func (e errConnectionUnknownState) Error() string {
	return fmt.Sprintf("connection is in unknown state: %v at %v", e.state, e.site)
}

// ConnectionOptions are options that control the behavior of a Connection
type ConnectionOptions struct {
	// The frame pool, allowing better management of frame buffers. Defaults to using raw heap.
	FramePool FramePool

	// NOTE: This is deprecated and not used for anything.
	RecvBufferSize int

	// The size of send channel buffers. Defaults to 512.
	SendBufferSize int

	// The type of checksum to use when sending messages.
	ChecksumType ChecksumType

	// ToS class name marked on outbound packets.
	TosPriority tos.ToS

	// HealthChecks configures active connection health checking for this channel.
	// By default, health checks are not enabled.
	HealthChecks HealthCheckOptions

	// MaxCloseTime controls how long we allow a connection to complete pending
	// calls before shutting down. Only used if it is non-zero.
	MaxCloseTime time.Duration
}

// connectionEvents are the events that can be triggered by a connection.
type connectionEvents struct {
	// OnActive is called when a connection becomes active.
	OnActive func(c *Connection)

	// OnCloseStateChange is called when a connection that is closing changes state.
	OnCloseStateChange func(c *Connection)

	// OnExchangeUpdated is called when a message exchange added or removed.
	OnExchangeUpdated func(c *Connection)
}

// Connection represents a connection to a remote peer.
type Connection struct {
	channelConnectionCommon

	connID          uint32
	connDirection   connectionDirection
	opts            ConnectionOptions
	conn            net.Conn
	localPeerInfo   LocalPeerInfo
	remotePeerInfo  PeerInfo
	sendCh          chan *Frame
	stopCh          chan struct{}
	state           connectionState
	stateMut        sync.RWMutex
	inbound         *messageExchangeSet
	outbound        *messageExchangeSet
	handler         Handler
	nextMessageID   atomic.Uint32
	events          connectionEvents
	commonStatsTags map[string]string
	relay           *Relayer

	// outboundHP is the host:port we used to create this outbound connection.
	// It may not match remotePeerInfo.HostPort, in which case the connection is
	// added to peers for both host:ports. For inbound connections, this is empty.
	outboundHP string

	// closeNetworkCalled is used to avoid errors from being logged
	// when this side closes a connection.
	closeNetworkCalled atomic.Bool
	// stoppedExchanges is atomically set when exchanges are stopped due to error.
	stoppedExchanges atomic.Bool
	// remotePeerAddress is used as a cache for remote peer address parsed into individual
	// components that can be used to set peer tags on OpenTracing Span.
	remotePeerAddress peerAddressComponents

	// healthCheckCtx/Quit are used to stop health checks.
	healthCheckCtx     context.Context
	healthCheckQuit    context.CancelFunc
	healthCheckDone    chan struct{}
	healthCheckHistory *healthHistory

	// lastActivity is used to track how long the connection has been idle.
	// (unix time, nano)
	lastActivity atomic.Int64
}

type peerAddressComponents struct {
	port     uint16
	ipv4     uint32
	ipv6     string
	hostname string
}

// _nextConnID is used to allocate unique IDs to every connection for debugging purposes.
var _nextConnID atomic.Uint32

type connectionState int

const (
	// Connection is fully active
	connectionActive connectionState = iota + 1

	// Connection is starting to close; new incoming requests are rejected, outbound
	// requests are allowed to proceed
	connectionStartClose

	// Connection has finished processing all active inbound, and is
	// waiting for outbound requests to complete or timeout
	connectionInboundClosed

	// Connection is fully closed
	connectionClosed
)

//go:generate stringer -type=connectionState

func getTimeout(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return DefaultConnectTimeout
	}

	return deadline.Sub(time.Now())
}

func (co ConnectionOptions) withDefaults() ConnectionOptions {
	if co.ChecksumType == ChecksumTypeNone {
		co.ChecksumType = ChecksumTypeCrc32
	}
	if co.FramePool == nil {
		co.FramePool = DefaultFramePool
	}
	if co.SendBufferSize <= 0 {
		co.SendBufferSize = defaultConnectionBufferSize
	}
	co.HealthChecks = co.HealthChecks.withDefaults()
	return co
}

func (ch *Channel) setConnectionTosPriority(tosPriority tos.ToS, c net.Conn) error {
	tcpAddr, isTCP := c.RemoteAddr().(*net.TCPAddr)
	if !isTCP {
		return nil
	}

	// Handle dual stack listeners and set Traffic Class.
	var err error
	switch ip := tcpAddr.IP; {
	case ip.To16() != nil && ip.To4() == nil:
		err = ipv6.NewConn(c).SetTrafficClass(int(tosPriority))
	case ip.To4() != nil:
		err = ipv4.NewConn(c).SetTOS(int(tosPriority))
	}
	return err
}

func (ch *Channel) newConnection(conn net.Conn, initialID uint32, outboundHP string, remotePeer PeerInfo, remotePeerAddress peerAddressComponents, events connectionEvents) *Connection {
	opts := ch.connectionOptions.withDefaults()

	connID := _nextConnID.Inc()
	connDirection := inbound
	log := ch.log.WithFields(LogFields{
		{"connID", connID},
		{"localAddr", conn.LocalAddr().String()},
		{"remoteAddr", conn.RemoteAddr().String()},
		{"remoteHostPort", remotePeer.HostPort},
		{"remoteIsEphemeral", remotePeer.IsEphemeral},
		{"remoteProcess", remotePeer.ProcessName},
	}...)
	if outboundHP != "" {
		connDirection = outbound
		log = log.WithFields(LogField{"outboundHP", outboundHP})
	}

	log = log.WithFields(LogField{"connectionDirection", connDirection})
	peerInfo := ch.PeerInfo()

	c := &Connection{
		channelConnectionCommon: ch.channelConnectionCommon,

		connID:             connID,
		conn:               conn,
		connDirection:      connDirection,
		opts:               opts,
		state:              connectionActive,
		sendCh:             make(chan *Frame, opts.SendBufferSize),
		stopCh:             make(chan struct{}),
		localPeerInfo:      peerInfo,
		remotePeerInfo:     remotePeer,
		remotePeerAddress:  remotePeerAddress,
		outboundHP:         outboundHP,
		inbound:            newMessageExchangeSet(log, messageExchangeSetInbound),
		outbound:           newMessageExchangeSet(log, messageExchangeSetOutbound),
		handler:            ch.handler,
		events:             events,
		commonStatsTags:    ch.commonStatsTags,
		healthCheckHistory: newHealthHistory(),
		lastActivity:       *atomic.NewInt64(ch.timeNow().UnixNano()),
	}

	if tosPriority := opts.TosPriority; tosPriority > 0 {
		if err := ch.setConnectionTosPriority(tosPriority, conn); err != nil {
			log.WithFields(ErrField(err)).Error("Failed to set ToS priority.")
		}
	}

	c.nextMessageID.Store(initialID)
	c.log = log
	c.inbound.onRemoved = c.checkExchanges
	c.outbound.onRemoved = c.checkExchanges
	c.inbound.onAdded = c.onExchangeAdded
	c.outbound.onAdded = c.onExchangeAdded

	if ch.RelayHost() != nil {
		c.relay = NewRelayer(ch, c)
	}

	// Connections are activated as soon as they are created.
	c.callOnActive()

	go c.readFrames(connID)
	go c.writeFrames(connID)
	return c
}

func (c *Connection) onExchangeAdded() {
	c.callOnExchangeChange()
}

// IsActive returns whether this connection is in an active state.
func (c *Connection) IsActive() bool {
	return c.readState() == connectionActive
}

func (c *Connection) callOnActive() {
	log := c.log
	if remoteVersion := c.remotePeerInfo.Version; remoteVersion != (PeerVersion{}) {
		log = log.WithFields(LogFields{
			{"remotePeerLanguage", remoteVersion.Language},
			{"remotePeerLanguageVersion", remoteVersion.LanguageVersion},
			{"remotePeerTChannelVersion", remoteVersion.TChannelVersion},
		}...)
	}
	log.Info("Created new active connection.")

	if f := c.events.OnActive; f != nil {
		f(c)
	}

	if c.opts.HealthChecks.enabled() {
		c.healthCheckCtx, c.healthCheckQuit = context.WithCancel(context.Background())
		c.healthCheckDone = make(chan struct{})
		go c.healthCheck(c.connID)
	}
}

func (c *Connection) callOnCloseStateChange() {
	if f := c.events.OnCloseStateChange; f != nil {
		f(c)
	}
}

func (c *Connection) callOnExchangeChange() {
	if f := c.events.OnExchangeUpdated; f != nil {
		f(c)
	}
}

// ping sends a ping message and waits for a ping response.
func (c *Connection) ping(ctx context.Context) error {
	req := &pingReq{id: c.NextMessageID()}
	mex, err := c.outbound.newExchange(ctx, c.opts.FramePool, req.messageType(), req.ID(), 1)
	if err != nil {
		return c.connectionError("create ping exchange", err)
	}
	defer c.outbound.removeExchange(req.ID())

	if err := c.sendMessage(req); err != nil {
		return c.connectionError("send ping", err)
	}

	return c.recvMessage(ctx, &pingRes{}, mex)
}

// handlePingRes calls registered ping handlers.
func (c *Connection) handlePingRes(frame *Frame) bool {
	if err := c.outbound.forwardPeerFrame(frame); err != nil {
		c.log.WithFields(LogField{"response", frame.Header}).Warn("Unexpected ping response.")
		return true
	}
	// ping req is waiting for this frame, and will release it.
	return false
}

// handlePingReq responds to the pingReq message with a pingRes.
func (c *Connection) handlePingReq(frame *Frame) {
	if state := c.readState(); state != connectionActive {
		c.protocolError(frame.Header.ID, errConnNotActive{"ping on incoming", state})
		return
	}

	pingRes := &pingRes{id: frame.Header.ID}
	if err := c.sendMessage(pingRes); err != nil {
		c.connectionError("send pong", err)
	}
}

// sendMessage sends a standalone message (typically a control message)
func (c *Connection) sendMessage(msg message) error {
	frame := c.opts.FramePool.Get()
	if err := frame.write(msg); err != nil {
		c.opts.FramePool.Release(frame)
		return err
	}

	select {
	case c.sendCh <- frame:
		return nil
	default:
		return ErrSendBufferFull
	}
}

// recvMessage blocks waiting for a standalone response message (typically a
// control message)
func (c *Connection) recvMessage(ctx context.Context, msg message, mex *messageExchange) error {
	frame, err := mex.recvPeerFrameOfType(msg.messageType())
	if err != nil {
		if err, ok := err.(errorMessage); ok {
			return err.AsSystemError()
		}
		return err
	}

	err = frame.read(msg)
	c.opts.FramePool.Release(frame)
	return err
}

// RemotePeerInfo returns the peer info for the remote peer.
func (c *Connection) RemotePeerInfo() PeerInfo {
	return c.remotePeerInfo
}

// NextMessageID reserves the next available message id for this connection
func (c *Connection) NextMessageID() uint32 {
	return c.nextMessageID.Inc()
}

// SendSystemError sends an error frame for the given system error.
func (c *Connection) SendSystemError(id uint32, span Span, err error) error {
	frame := c.opts.FramePool.Get()

	if err := frame.write(&errorMessage{
		id:      id,
		errCode: GetSystemErrorCode(err),
		tracing: span,
		message: GetSystemErrorMessage(err),
	}); err != nil {

		// This shouldn't happen - it means writing the errorMessage is broken.
		c.log.WithFields(
			LogField{"remotePeer", c.remotePeerInfo},
			LogField{"id", id},
			ErrField(err),
		).Warn("Couldn't create outbound frame.")
		return fmt.Errorf("failed to create outbound error frame: %v", err)
	}

	// When sending errors, we hold the state rlock to ensure that sendCh is not closed
	// as we are sending the frame.
	return c.withStateRLock(func() error {
		// Errors cannot be sent if the connection has been closed.
		if c.state == connectionClosed {
			c.log.WithFields(
				LogField{"remotePeer", c.remotePeerInfo},
				LogField{"id", id},
			).Info("Could not send error frame on closed connection.")
			return fmt.Errorf("failed to send error frame, connection state %v", c.state)
		}

		select {
		case c.sendCh <- frame: // Good to go
			return nil
		default: // If the send buffer is full, log and return an error.
		}
		c.log.WithFields(
			LogField{"remotePeer", c.remotePeerInfo},
			LogField{"id", id},
			ErrField(err),
		).Warn("Couldn't send outbound frame.")
		return fmt.Errorf("failed to send error frame, buffer full")
	})
}

func (c *Connection) logConnectionError(site string, err error) error {
	errCode := ErrCodeNetwork
	if err == io.EOF {
		c.log.Debugf("Connection got EOF")
	} else {
		logger := c.log.WithFields(
			LogField{"site", site},
			ErrField(err),
		)

		if se, ok := err.(SystemError); ok && se.Code() != ErrCodeNetwork {
			errCode = se.Code()
			logger.Error("Connection error.")
		} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
			logger.Warn("Connection error due to timeout.")
		} else {
			logger.Info("Connection error.")
		}
	}
	return NewWrappedSystemError(errCode, err)
}

// connectionError handles a connection level error
func (c *Connection) connectionError(site string, err error) error {
	var closeLogFields LogFields
	if err == io.EOF {
		closeLogFields = LogFields{{"reason", "network connection EOF"}}
	} else {
		closeLogFields = LogFields{
			{"reason", "connection error"},
			ErrField(err),
		}
	}

	c.stopHealthCheck()
	err = c.logConnectionError(site, err)
	c.close(closeLogFields...)

	// On any connection error, notify the exchanges of this error.
	if c.stoppedExchanges.CAS(false, true) {
		c.outbound.stopExchanges(err)
		c.inbound.stopExchanges(err)
	}

	// checkExchanges will close the connection due to stoppedExchanges.
	c.checkExchanges()
	return err
}

func (c *Connection) protocolError(id uint32, err error) error {
	c.log.WithFields(ErrField(err)).Warn("Protocol error.")
	sysErr := NewWrappedSystemError(ErrCodeProtocol, err)
	c.SendSystemError(id, Span{}, sysErr)
	// Don't close the connection until the error has been sent.
	c.close(
		LogField{"reason", "protocol error"},
		ErrField(err),
	)

	// On any connection error, notify the exchanges of this error.
	if c.stoppedExchanges.CAS(false, true) {
		c.outbound.stopExchanges(sysErr)
		c.inbound.stopExchanges(sysErr)
	}
	return sysErr
}

// withStateLock performs an action with the connection state mutex locked
func (c *Connection) withStateLock(f func() error) error {
	c.stateMut.Lock()
	err := f()
	c.stateMut.Unlock()

	return err
}

// withStateRLock performs an action with the connection state mutex rlocked.
func (c *Connection) withStateRLock(f func() error) error {
	c.stateMut.RLock()
	err := f()
	c.stateMut.RUnlock()

	return err
}

func (c *Connection) readState() connectionState {
	c.stateMut.RLock()
	state := c.state
	c.stateMut.RUnlock()
	return state
}

// readFrames is the loop that reads frames from the network connection and
// dispatches to the appropriate handler. Run within its own goroutine to
// prevent overlapping reads on the socket.  Most handlers simply send the
// incoming frame to a channel; the init handlers are a notable exception,
// since we cannot process new frames until the initialization is complete.
func (c *Connection) readFrames(_ uint32) {
	headerBuf := make([]byte, FrameHeaderSize)

	handleErr := func(err error) {
		if !c.closeNetworkCalled.Load() {
			c.connectionError("read frames", err)
		} else {
			c.log.Debugf("Ignoring error after connection was closed: %v", err)
		}
	}

	for {
		// Read the header, avoid allocating the frame till we know the size
		// we need to allocate.
		if _, err := io.ReadFull(c.conn, headerBuf); err != nil {
			handleErr(err)
			return
		}

		frame := c.opts.FramePool.Get()
		if err := frame.ReadBody(headerBuf, c.conn); err != nil {
			handleErr(err)
			c.opts.FramePool.Release(frame)
			return
		}

		c.updateLastActivity(frame)

		var releaseFrame bool
		if c.relay == nil {
			releaseFrame = c.handleFrameNoRelay(frame)
		} else {
			releaseFrame = c.handleFrameRelay(frame)
		}
		if releaseFrame {
			c.opts.FramePool.Release(frame)
		}
	}
}

func (c *Connection) handleFrameRelay(frame *Frame) bool {
	switch frame.Header.messageType {
	case messageTypeCallReq, messageTypeCallReqContinue, messageTypeCallRes, messageTypeCallResContinue, messageTypeError:
		if err := c.relay.Relay(frame); err != nil {
			c.log.WithFields(
				ErrField(err),
				LogField{"header", frame.Header},
				LogField{"remotePeer", c.remotePeerInfo},
			).Error("Failed to relay frame.")
		}
		return false
	default:
		return c.handleFrameNoRelay(frame)
	}
}

func (c *Connection) handleFrameNoRelay(frame *Frame) bool {
	releaseFrame := true

	// call req and call res messages may not want the frame released immediately.
	switch frame.Header.messageType {
	case messageTypeCallReq:
		releaseFrame = c.handleCallReq(frame)
	case messageTypeCallReqContinue:
		releaseFrame = c.handleCallReqContinue(frame)
	case messageTypeCallRes:
		releaseFrame = c.handleCallRes(frame)
	case messageTypeCallResContinue:
		releaseFrame = c.handleCallResContinue(frame)
	case messageTypePingReq:
		c.handlePingReq(frame)
	case messageTypePingRes:
		releaseFrame = c.handlePingRes(frame)
	case messageTypeError:
		releaseFrame = c.handleError(frame)
	default:
		// TODO(mmihic): Log and close connection with protocol error
		c.log.WithFields(
			LogField{"header", frame.Header},
			LogField{"remotePeer", c.remotePeerInfo},
		).Error("Received unexpected frame.")
	}

	return releaseFrame
}

// writeFrames is the main loop that pulls frames from the send channel and
// writes them to the connection.
func (c *Connection) writeFrames(_ uint32) {
	for {
		select {
		case f := <-c.sendCh:
			if c.log.Enabled(LogLevelDebug) {
				c.log.Debugf("Writing frame %s", f.Header)
			}

			c.updateLastActivity(f)
			err := f.WriteOut(c.conn)
			c.opts.FramePool.Release(f)
			if err != nil {
				c.connectionError("write frames", err)
				return
			}
		case <-c.stopCh:
			// If there are frames in sendCh, we want to drain them.
			if len(c.sendCh) > 0 {
				continue
			}
			// Close the network once we're no longer writing frames.
			c.closeNetwork()
			return
		}
	}
}

// updateLastActivity marks when the last message was received/sent on the channel.
// This is used for monitoring idle connections and timing them out.
func (c *Connection) updateLastActivity(frame *Frame) {
	// Pings are ignored for last activity.
	switch frame.Header.messageType {
	case messageTypeCallReq, messageTypeCallReqContinue, messageTypeCallRes, messageTypeCallResContinue, messageTypeError:
		c.lastActivity.Store(c.timeNow().UnixNano())
	}
}

// hasPendingCalls returns whether there's any pending inbound or outbound calls on this connection.
func (c *Connection) hasPendingCalls() bool {
	if c.inbound.count() > 0 || c.outbound.count() > 0 {
		return true
	}
	if !c.relay.canClose() {
		return true
	}

	return false
}

// checkExchanges is called whenever an exchange is removed, and when Close is called.
func (c *Connection) checkExchanges() {
	c.callOnExchangeChange()

	moveState := func(fromState, toState connectionState) bool {
		err := c.withStateLock(func() error {
			if c.state != fromState {
				return errors.New("")
			}
			c.state = toState
			return nil
		})
		return err == nil
	}

	curState := c.readState()
	origState := curState

	if curState != connectionClosed && c.stoppedExchanges.Load() {
		if moveState(curState, connectionClosed) {
			curState = connectionClosed
		}
	}

	if curState == connectionStartClose {
		if !c.relay.canClose() {
			return
		}
		if c.inbound.count() == 0 && moveState(connectionStartClose, connectionInboundClosed) {
			curState = connectionInboundClosed
		}
	}

	if curState == connectionInboundClosed {
		// Safety check -- this should never happen since we already did the check
		// when transitioning to connectionInboundClosed.
		if !c.relay.canClose() {
			c.relay.logger.Error("Relay can't close even though state is InboundClosed.")
			return
		}

		if c.outbound.count() == 0 && moveState(connectionInboundClosed, connectionClosed) {
			curState = connectionClosed
		}
	}

	if curState != origState {
		// If the connection is closed, we can notify writeFrames to stop which
		// closes the underlying network connection. We never close sendCh to avoid
		// races causing panics, see 93ef5c112c8b321367ae52d2bd79396e2e874f31
		if curState == connectionClosed {
			close(c.stopCh)
		}

		c.log.WithFields(
			LogField{"newState", curState},
		).Debug("Connection state updated during shutdown.")
		c.callOnCloseStateChange()
	}
}

func (c *Connection) close(fields ...LogField) error {
	c.log.WithFields(fields...).Info("Connection closing.")

	// Update the state which will start blocking incoming calls.
	if err := c.withStateLock(func() error {
		switch s := c.state; s {
		case connectionActive:
			c.state = connectionStartClose
		default:
			return fmt.Errorf("connection must be Active to Close, but it is %v", s)
		}
		return nil
	}); err != nil {
		return err
	}

	// Set a read deadline with any close timeout. This will cause a i/o timeout
	// if the connection isn't closed by then.
	if c.opts.MaxCloseTime > 0 {
		c.conn.SetReadDeadline(c.timeNow().Add(c.opts.MaxCloseTime))
	}

	c.log.WithFields(
		LogField{"newState", c.readState()},
	).Debug("Connection state updated in Close.")
	c.callOnCloseStateChange()

	// Check all in-flight requests to see whether we can transition the Close state.
	c.checkExchanges()

	return nil
}

// Close starts a graceful Close which will first reject incoming calls, reject outgoing calls
// before finally marking the connection state as closed.
func (c *Connection) Close() error {
	return c.close(LogField{"reason", "user initiated"})
}

// closeNetwork closes the network connection and all network-related channels.
// This should only be done in response to a fatal connection or protocol
// error, or after all pending frames have been sent.
func (c *Connection) closeNetwork() {
	// NB(mmihic): The sender goroutine will exit once the connection is
	// closed; no need to close the send channel (and closing the send
	// channel would be dangerous since other goroutine might be sending)
	c.log.Debugf("Closing underlying network connection")
	c.stopHealthCheck()
	c.closeNetworkCalled.Store(true)
	if err := c.conn.Close(); err != nil {
		c.log.WithFields(
			LogField{"remotePeer", c.remotePeerInfo},
			ErrField(err),
		).Warn("Couldn't close connection to peer.")
	}
}

// getLastActivityTime returns the timestamp of the last frame read or written,
// excluding pings. If no frames were transmitted yet, it will return the time
// this connection was created.
func (c *Connection) getLastActivityTime() time.Time {
	return time.Unix(0, c.lastActivity.Load())
}
