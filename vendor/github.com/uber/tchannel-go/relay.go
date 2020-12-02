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
	"math"
	"sync"
	"time"

	"github.com/uber/tchannel-go/relay"

	"go.uber.org/atomic"
)

const (
	// _maxRelayTombs is the maximum number of tombs we'll accumulate in a
	// single relayItems.
	_maxRelayTombs = 3e4
	// _relayTombTTL is the length of time we'll keep a tomb before GC'ing it.
	_relayTombTTL = 3 * time.Second
	// _defaultRelayMaxTimeout is the default max TTL for relayed calls.
	_defaultRelayMaxTimeout = 2 * time.Minute
)

// Error strings.
const (
	_relayErrorNotFound       = "relay-not-found"
	_relayErrorDestConnSlow   = "relay-dest-conn-slow"
	_relayErrorSourceConnSlow = "relay-source-conn-slow"
)

var (
	errRelayMethodFragmented = NewSystemError(ErrCodeBadRequest, "relay handler cannot receive fragmented calls")
	errFrameNotSent          = NewSystemError(ErrCodeNetwork, "frame was not sent to remote side")
	errBadRelayHost          = NewSystemError(ErrCodeDeclined, "bad relay host implementation")
	errUnknownID             = errors.New("non-callReq for inactive ID")
)

type relayItem struct {
	remapID     uint32
	tomb        bool
	local       bool
	call        RelayCall
	destination *Relayer
	span        Span
	timeout     *relayTimer
}

type relayItems struct {
	sync.RWMutex

	logger   Logger
	timeouts *relayTimerPool
	tombs    uint64
	items    map[uint32]relayItem
}

func newRelayItems(logger Logger) *relayItems {
	return &relayItems{
		items:  make(map[uint32]relayItem),
		logger: logger,
	}
}

// Count returns the number of non-tombstone items in the relay.
func (r *relayItems) Count() int {
	r.RLock()
	n := len(r.items) - int(r.tombs)
	r.RUnlock()
	return n
}

// Get checks for a relay item by ID, returning the item and a bool indicating
// whether the item was found.
func (r *relayItems) Get(id uint32) (relayItem, bool) {
	r.RLock()
	item, ok := r.items[id]
	r.RUnlock()

	return item, ok
}

// Add adds a relay item.
func (r *relayItems) Add(id uint32, item relayItem) {
	r.Lock()
	r.items[id] = item
	r.Unlock()
}

// Delete removes a relayItem completely (without leaving a tombstone). It
// returns the deleted item, along with a bool indicating whether we completed a
// relayed call.
func (r *relayItems) Delete(id uint32) (relayItem, bool) {
	r.Lock()
	item, ok := r.items[id]
	if !ok {
		r.Unlock()
		r.logger.WithFields(LogField{"id", id}).Warn("Attempted to delete non-existent relay item.")
		return item, false
	}
	delete(r.items, id)
	if item.tomb {
		r.tombs--
	}
	r.Unlock()

	item.timeout.Release()
	return item, !item.tomb
}

// Entomb sets the tomb bit on a relayItem and schedules a garbage collection. It
// returns the entombed item, along with a bool indicating whether we completed
// a relayed call.
func (r *relayItems) Entomb(id uint32, deleteAfter time.Duration) (relayItem, bool) {
	r.Lock()
	if r.tombs > _maxRelayTombs {
		r.Unlock()
		r.logger.WithFields(LogField{"id", id}).Warn("Too many tombstones, deleting relay item immediately.")
		return r.Delete(id)
	}
	item, ok := r.items[id]
	if !ok {
		r.Unlock()
		r.logger.WithFields(LogField{"id", id}).Warn("Can't find relay item to entomb.")
		return item, false
	}
	if item.tomb {
		r.Unlock()
		r.logger.WithFields(LogField{"id", id}).Warn("Re-entombing a tombstone.")
		return item, false
	}
	r.tombs++
	item.tomb = true
	r.items[id] = item
	r.Unlock()

	// TODO: We should be clearing these out in batches, rather than creating
	// individual timers for each item.
	time.AfterFunc(deleteAfter, func() { r.Delete(id) })
	return item, true
}

type frameType int

const (
	requestFrame  frameType = 0
	responseFrame frameType = 1
)

// A Relayer forwards frames.
type Relayer struct {
	relayHost  RelayHost
	maxTimeout time.Duration

	// localHandlers is the set of service names that are handled by the local
	// channel.
	localHandler map[string]struct{}

	// outbound is the remapping for requests that originated on this
	// connection, and are outbound towards some other connection.
	// It stores remappings for all request frames read on this connection.
	outbound *relayItems

	// inbound is the remapping for requests that originated on some other
	// connection which was directed to this connection.
	// It stores remappings for all response frames read on this connection.
	inbound *relayItems

	// timeouts is the pool of timers used to track call timeouts.
	// It allows timer re-use, while allowing timers to be created and started separately.
	timeouts *relayTimerPool

	peers     *RootPeerList
	conn      *Connection
	relayConn *relay.Conn
	logger    Logger
	pending   atomic.Uint32
}

// NewRelayer constructs a Relayer.
func NewRelayer(ch *Channel, conn *Connection) *Relayer {
	r := &Relayer{
		relayHost:    ch.RelayHost(),
		maxTimeout:   ch.relayMaxTimeout,
		localHandler: ch.relayLocal,
		outbound:     newRelayItems(conn.log.WithFields(LogField{"relayItems", "outbound"})),
		inbound:      newRelayItems(conn.log.WithFields(LogField{"relayItems", "inbound"})),
		peers:        ch.RootPeers(),
		conn:         conn,
		relayConn: &relay.Conn{
			RemoteAddr:        conn.conn.RemoteAddr().String(),
			RemoteProcessName: conn.RemotePeerInfo().ProcessName,
			IsOutbound:        conn.connDirection == outbound,
		},
		logger: conn.log,
	}
	r.timeouts = newRelayTimerPool(r.timeoutRelayItem, ch.relayTimerVerify)
	return r
}

// Relay is called for each frame that is read on the connection.
func (r *Relayer) Relay(f *Frame) error {
	if f.messageType() != messageTypeCallReq {
		err := r.handleNonCallReq(f)
		if err == errUnknownID {
			// This ID may be owned by an outgoing call, so check the outbound
			// message exchange, and if it succeeds, then the frame has been
			// handled successfully.
			if err := r.conn.outbound.forwardPeerFrame(f); err == nil {
				return nil
			}
		}
		return err
	}
	return r.handleCallReq(newLazyCallReq(f))
}

// Receive receives frames intended for this connection.
// It returns whether the frame was sent and a reason for failure if it failed.
func (r *Relayer) Receive(f *Frame, fType frameType) (sent bool, failureReason string) {
	id := f.Header.ID

	// If we receive a response frame, we expect to find that ID in our outbound.
	// If we receive a request frame, we expect to find that ID in our inbound.
	items := r.receiverItems(fType)

	item, ok := items.Get(id)
	if !ok {
		r.logger.WithFields(
			LogField{"id", id},
		).Warn("Received a frame without a RelayItem.")
		return false, _relayErrorNotFound
	}

	finished := finishesCall(f)
	if item.tomb {
		// Call timed out, ignore this frame. (We've already handled stats.)
		// TODO: metrics for late-arriving frames.
		return true, ""
	}

	// If the call is finished, we stop the timeout to ensure
	// we don't have concurrent calls to end the call.
	if finished && !item.timeout.Stop() {
		// Timeout goroutine is already ending this call.
		return true, ""
	}

	// call res frames don't include the OK bit, so we can't wait until the last
	// frame of a relayed RPC to determine if the call succeeded.
	if fType == responseFrame {
		// If we've gotten a response frame, we're the originating relayer and
		// should handle stats.
		if succeeded, failMsg := determinesCallSuccess(f); succeeded {
			item.call.Succeeded()
		} else if len(failMsg) > 0 {
			item.call.Failed(failMsg)
		}
	}
	select {
	case r.conn.sendCh <- f:
	default:
		// Buffer is full, so drop this frame and cancel the call.
		r.logger.WithFields(
			LogField{"id", id},
		).Warn("Dropping call due to slow connection.")

		items := r.receiverItems(fType)

		err := _relayErrorDestConnSlow
		// If we're dealing with a response frame, then the client is slow.
		if fType == responseFrame {
			err = _relayErrorSourceConnSlow
		}

		r.failRelayItem(items, id, err)
		return false, err
	}

	if finished {
		r.finishRelayItem(items, id)
	}

	return true, ""
}

func (r *Relayer) canHandleNewCall() (bool, connectionState) {
	var (
		canHandle bool
		curState  connectionState
	)

	r.conn.withStateRLock(func() error {
		curState = r.conn.state
		canHandle = curState == connectionActive
		if canHandle {
			r.pending.Inc()
		}
		return nil
	})
	return canHandle, curState
}

func (r *Relayer) getDestination(f lazyCallReq, call RelayCall) (*Connection, bool, error) {
	if _, ok := r.outbound.Get(f.Header.ID); ok {
		r.logger.WithFields(
			LogField{"id", f.Header.ID},
			LogField{"source", string(f.Caller())},
			LogField{"dest", string(f.Service())},
			LogField{"method", string(f.Method())},
		).Warn("Received duplicate callReq.")
		call.Failed(ErrCodeProtocol.relayMetricsKey())
		// TODO: this is a protocol error, kill the connection.
		return nil, false, errors.New("callReq with already active ID")
	}

	// Get the destination
	peer, ok := call.Destination()
	if !ok {
		call.Failed("relay-bad-relay-host")
		r.conn.SendSystemError(f.Header.ID, f.Span(), errBadRelayHost)
		return nil, false, errBadRelayHost
	}

	// TODO: Should connections use the call timeout? Or a separate timeout?
	remoteConn, err := peer.getConnectionRelay(f.TTL())
	if err != nil {
		r.logger.WithFields(
			ErrField(err),
			LogField{"source", string(f.Caller())},
			LogField{"dest", string(f.Service())},
			LogField{"method", string(f.Method())},
			LogField{"selectedPeer", peer},
		).Warn("Failed to connect to relay host.")
		call.Failed("relay-connection-failed")
		r.conn.SendSystemError(f.Header.ID, f.Span(), NewWrappedSystemError(ErrCodeNetwork, err))
		return nil, false, nil
	}

	return remoteConn, true, nil
}

func (r *Relayer) handleCallReq(f lazyCallReq) error {
	if handled := r.handleLocalCallReq(f); handled {
		return nil
	}

	call, err := r.relayHost.Start(f, r.relayConn)
	if err != nil {
		// If we have a RateLimitDropError we record the statistic, but
		// we *don't* send an error frame back to the client.
		if _, silentlyDrop := err.(relay.RateLimitDropError); silentlyDrop {
			if call != nil {
				call.Failed("relay-dropped")
				call.End()
			}
			return nil
		}
		if _, ok := err.(SystemError); !ok {
			err = NewSystemError(ErrCodeDeclined, err.Error())
		}
		if call != nil {
			call.Failed(GetSystemErrorCode(err).relayMetricsKey())
			call.End()
		}
		r.conn.SendSystemError(f.Header.ID, f.Span(), err)

		// If the RelayHost returns a protocol error, close the connection.
		if GetSystemErrorCode(err) == ErrCodeProtocol {
			return r.conn.close(LogField{"reason", "RelayHost returned protocol error"})
		}
		return nil
	}

	// Check that the current connection is in a valid state to handle a new call.
	if canHandle, state := r.canHandleNewCall(); !canHandle {
		call.Failed("relay-client-conn-inactive")
		call.End()
		err := errConnNotActive{"incoming", state}
		r.conn.SendSystemError(f.Header.ID, f.Span(), NewWrappedSystemError(ErrCodeDeclined, err))
		return err
	}

	// Get a remote connection and check whether it can handle this call.
	remoteConn, ok, err := r.getDestination(f, call)
	if err == nil && ok {
		if canHandle, state := remoteConn.relay.canHandleNewCall(); !canHandle {
			err = NewWrappedSystemError(ErrCodeNetwork, errConnNotActive{"selected remote", state})
			call.Failed("relay-remote-inactive")
			r.conn.SendSystemError(f.Header.ID, f.Span(), NewWrappedSystemError(ErrCodeDeclined, err))
		}
	}
	if err != nil || !ok {
		// Failed to get a remote connection, or the connection is not in the right
		// state to handle this call. Since we already incremented pending on
		// the current relay, we need to decrement it.
		r.decrementPending()
		call.End()
		return err
	}

	origID := f.Header.ID
	destinationID := remoteConn.NextMessageID()
	ttl := f.TTL()
	if ttl > r.maxTimeout {
		ttl = r.maxTimeout
		f.SetTTL(r.maxTimeout)
	}
	span := f.Span()
	// The remote side of the relay doesn't need to track stats.
	remoteConn.relay.addRelayItem(false /* isOriginator */, destinationID, f.Header.ID, r, ttl, span, nil)
	relayToDest := r.addRelayItem(true /* isOriginator */, f.Header.ID, destinationID, remoteConn.relay, ttl, span, call)

	f.Header.ID = destinationID
	sent, failure := relayToDest.destination.Receive(f.Frame, requestFrame)
	if !sent {
		r.failRelayItem(r.outbound, origID, failure)
		return nil
	}

	return nil
}

// Handle all frames except messageTypeCallReq.
func (r *Relayer) handleNonCallReq(f *Frame) error {
	frameType := frameTypeFor(f)
	finished := finishesCall(f)

	// If we read a request frame, we need to use the outbound map to decide
	// the destination. Otherwise, we use the inbound map.
	items := r.outbound
	if frameType == responseFrame {
		items = r.inbound
	}

	item, ok := items.Get(f.Header.ID)
	if !ok {
		return errUnknownID
	}
	if item.tomb {
		// Call timed out, ignore this frame. (We've already handled stats.)
		// TODO: metrics for late-arriving frames.
		return nil
	}

	// If the call is finished, we stop the timeout to ensure
	// we don't have concurrent calls to end the call.
	if finished && !item.timeout.Stop() {
		// Timeout goroutine is already ending this call.
		return nil
	}

	originalID := f.Header.ID
	f.Header.ID = item.remapID

	sent, failure := item.destination.Receive(f, frameType)
	if !sent {
		r.failRelayItem(items, originalID, failure)
		return nil
	}

	if finished {
		r.finishRelayItem(items, originalID)
	}
	return nil
}

// addRelayItem adds a relay item to either outbound or inbound.
func (r *Relayer) addRelayItem(isOriginator bool, id, remapID uint32, destination *Relayer, ttl time.Duration, span Span, call RelayCall) relayItem {
	item := relayItem{
		call:        call,
		remapID:     remapID,
		destination: destination,
		span:        span,
	}

	items := r.inbound
	if isOriginator {
		items = r.outbound
	}
	item.timeout = r.timeouts.Get()
	items.Add(id, item)
	item.timeout.Start(ttl, items, id, isOriginator)
	return item
}

func (r *Relayer) timeoutRelayItem(items *relayItems, id uint32, isOriginator bool) {
	item, ok := items.Entomb(id, _relayTombTTL)
	if !ok {
		return
	}
	if isOriginator {
		r.conn.SendSystemError(id, item.span, ErrTimeout)
		item.call.Failed("timeout")
		item.call.End()
	}

	r.decrementPending()
}

// failRelayItem tombs the relay item so that future frames for this call are not
// forwarded. We keep the relay item tombed, rather than delete it to ensure that
// future frames do not cause error logs.
func (r *Relayer) failRelayItem(items *relayItems, id uint32, failure string) {
	item, ok := items.Get(id)
	if !ok {
		items.logger.WithFields(LogField{"id", id}).Warn("Attempted to fail non-existent relay item.")
		return
	}

	// The call could time-out right as we entomb it, which would cause spurious
	// error logs, so ensure we can stop the timeout.
	if !item.timeout.Stop() {
		return
	}

	// Entomb it so that we don't get unknown exchange errors on further frames
	// for this call.
	item, ok = items.Entomb(id, _relayTombTTL)
	if !ok {
		return
	}
	if item.call != nil {
		// If the client is too slow, then there's no point sending an error frame.
		if failure != _relayErrorSourceConnSlow {
			r.conn.SendSystemError(id, item.span, errFrameNotSent)
		}
		item.call.Failed(failure)
		item.call.End()
	}

	r.decrementPending()
}

func (r *Relayer) finishRelayItem(items *relayItems, id uint32) {
	item, ok := items.Delete(id)
	if !ok {
		return
	}
	if item.call != nil {
		item.call.End()
	}
	r.decrementPending()
}

func (r *Relayer) decrementPending() {
	r.pending.Dec()
	r.conn.checkExchanges()
}

func (r *Relayer) canClose() bool {
	if r == nil {
		return true
	}
	return r.countPending() == 0
}

func (r *Relayer) countPending() uint32 {
	return r.pending.Load()
}

func (r *Relayer) receiverItems(fType frameType) *relayItems {
	if fType == requestFrame {
		return r.inbound
	}
	return r.outbound
}

func (r *Relayer) handleLocalCallReq(cr lazyCallReq) bool {
	// Check whether this is a service we want to handle locally.
	if _, ok := r.localHandler[string(cr.Service())]; !ok {
		return false
	}

	f := cr.Frame

	// We can only handle non-fragmented calls in the relay channel.
	// This is a simplification to avoid back references from a mex to a
	// relayItem so that the relayItem is cleared when the call completes.
	if cr.HasMoreFragments() {
		r.logger.WithFields(
			LogField{"id", cr.Header.ID},
			LogField{"source", string(cr.Caller())},
			LogField{"dest", string(cr.Service())},
			LogField{"method", string(cr.Method())},
		).Error("Received fragmented callReq intended for local relay channel, can only handle unfragmented calls.")
		r.conn.SendSystemError(f.Header.ID, cr.Span(), errRelayMethodFragmented)
		return true
	}

	if release := r.conn.handleFrameNoRelay(f); release {
		r.conn.opts.FramePool.Release(f)
	}
	return true
}

func frameTypeFor(f *Frame) frameType {
	switch t := f.Header.messageType; t {
	case messageTypeCallRes, messageTypeCallResContinue, messageTypeError, messageTypePingRes:
		return responseFrame
	case messageTypeCallReq, messageTypeCallReqContinue, messageTypePingReq:
		return requestFrame
	default:
		panic(fmt.Sprintf("unsupported frame type: %v", t))
	}
}

func determinesCallSuccess(f *Frame) (succeeded bool, failMsg string) {
	switch f.messageType() {
	case messageTypeError:
		msg := newLazyError(f).Code().MetricsKey()
		return false, msg
	case messageTypeCallRes:
		if newLazyCallRes(f).OK() {
			return true, ""
		}
		return false, "application-error"
	default:
		return false, ""
	}
}

func validateRelayMaxTimeout(d time.Duration, logger Logger) time.Duration {
	maxMillis := d / time.Millisecond
	if maxMillis > 0 && maxMillis <= math.MaxUint32 {
		return d
	}
	if d == 0 {
		return _defaultRelayMaxTimeout
	}
	logger.WithFields(
		LogField{"configuredMaxTimeout", d},
		LogField{"defaultMaxTimeout", _defaultRelayMaxTimeout},
	).Warn("Configured RelayMaxTimeout is invalid, using default instead.")
	return _defaultRelayMaxTimeout
}
