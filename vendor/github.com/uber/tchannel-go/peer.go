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
	"container/heap"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/uber/tchannel-go/trand"

	"go.uber.org/atomic"
	"golang.org/x/net/context"
)

var (
	// ErrInvalidConnectionState indicates that the connection is not in a valid state.
	// This may be due to a race between selecting the connection and it closing, so
	// it is a network failure that can be retried.
	ErrInvalidConnectionState = NewSystemError(ErrCodeNetwork, "connection is in an invalid state")

	// ErrNoPeers indicates that there are no peers.
	ErrNoPeers = errors.New("no peers available")

	// ErrPeerNotFound indicates that the specified peer was not found.
	ErrPeerNotFound = errors.New("peer not found")

	// ErrNoNewPeers indicates that no previously unselected peer is available.
	ErrNoNewPeers = errors.New("no new peer available")

	peerRng = trand.NewSeeded()
)

// Connectable is the interface used by peers to create connections.
type Connectable interface {
	// Connect tries to connect to the given hostPort.
	Connect(ctx context.Context, hostPort string) (*Connection, error)
	// Logger returns the logger to use.
	Logger() Logger
}

// PeerList maintains a list of Peers.
type PeerList struct {
	sync.RWMutex

	parent          *RootPeerList
	peersByHostPort map[string]*peerScore
	peerHeap        *peerHeap
	scoreCalculator ScoreCalculator
	lastSelected    uint64
}

func newPeerList(root *RootPeerList) *PeerList {
	return &PeerList{
		parent:          root,
		peersByHostPort: make(map[string]*peerScore),
		scoreCalculator: newPreferIncomingCalculator(),
		peerHeap:        newPeerHeap(),
	}
}

// SetStrategy sets customized peer selection strategy.
func (l *PeerList) SetStrategy(sc ScoreCalculator) {
	l.Lock()
	defer l.Unlock()

	l.scoreCalculator = sc
	for _, ps := range l.peersByHostPort {
		newScore := l.scoreCalculator.GetScore(ps.Peer)
		l.updatePeer(ps, newScore)
	}
}

// Siblings don't share peer lists (though they take care not to double-connect
// to the same hosts).
func (l *PeerList) newSibling() *PeerList {
	sib := newPeerList(l.parent)
	return sib
}

// Add adds a peer to the list if it does not exist, or returns any existing peer.
func (l *PeerList) Add(hostPort string) *Peer {
	if ps, ok := l.exists(hostPort); ok {
		return ps.Peer
	}
	l.Lock()
	defer l.Unlock()

	if p, ok := l.peersByHostPort[hostPort]; ok {
		return p.Peer
	}

	p := l.parent.Add(hostPort)
	p.addSC()
	ps := newPeerScore(p, l.scoreCalculator.GetScore(p))

	l.peersByHostPort[hostPort] = ps
	l.peerHeap.addPeer(ps)
	return p
}

// GetNew returns a new, previously unselected peer from the peer list, or nil,
// if no new unselected peer can be found.
func (l *PeerList) GetNew(prevSelected map[string]struct{}) (*Peer, error) {
	l.Lock()
	defer l.Unlock()
	if l.peerHeap.Len() == 0 {
		return nil, ErrNoPeers
	}

	// Select a peer, avoiding previously selected peers. If all peers have been previously
	// selected, then it's OK to repick them.
	peer := l.choosePeer(prevSelected, true /* avoidHost */)
	if peer == nil {
		peer = l.choosePeer(prevSelected, false /* avoidHost */)
	}
	if peer == nil {
		return nil, ErrNoNewPeers
	}
	return peer, nil
}

// Get returns a peer from the peer list, or nil if none can be found,
// will avoid previously selected peers if possible.
func (l *PeerList) Get(prevSelected map[string]struct{}) (*Peer, error) {
	peer, err := l.GetNew(prevSelected)
	if err == ErrNoNewPeers {
		l.Lock()
		peer = l.choosePeer(nil, false /* avoidHost */)
		l.Unlock()
	} else if err != nil {
		return nil, err
	}
	if peer == nil {
		return nil, ErrNoPeers
	}
	return peer, nil
}

// Remove removes a peer from the peer list. It returns an error if the peer cannot be found.
// Remove does not affect connections to the peer in any way.
func (l *PeerList) Remove(hostPort string) error {
	l.Lock()
	defer l.Unlock()

	p, ok := l.peersByHostPort[hostPort]
	if !ok {
		return ErrPeerNotFound
	}

	p.delSC()
	delete(l.peersByHostPort, hostPort)
	l.peerHeap.removePeer(p)

	return nil
}
func (l *PeerList) choosePeer(prevSelected map[string]struct{}, avoidHost bool) *Peer {
	var psPopList []*peerScore
	var ps *peerScore

	canChoosePeer := func(hostPort string) bool {
		if _, ok := prevSelected[hostPort]; ok {
			return false
		}
		if avoidHost {
			if _, ok := prevSelected[getHost(hostPort)]; ok {
				return false
			}
		}
		return true
	}

	size := l.peerHeap.Len()
	for i := 0; i < size; i++ {
		popped := l.peerHeap.popPeer()

		if canChoosePeer(popped.HostPort()) {
			ps = popped
			break
		}
		psPopList = append(psPopList, popped)
	}

	for _, p := range psPopList {
		heap.Push(l.peerHeap, p)
	}

	if ps == nil {
		return nil
	}

	l.peerHeap.pushPeer(ps)
	ps.chosenCount.Inc()
	return ps.Peer
}

// GetOrAdd returns a peer for the given hostPort, creating one if it doesn't yet exist.
func (l *PeerList) GetOrAdd(hostPort string) *Peer {
	if ps, ok := l.exists(hostPort); ok {
		return ps.Peer
	}
	return l.Add(hostPort)
}

// Copy returns a copy of the PeerList as a map from hostPort to peer.
func (l *PeerList) Copy() map[string]*Peer {
	l.RLock()
	defer l.RUnlock()

	listCopy := make(map[string]*Peer)
	for k, v := range l.peersByHostPort {
		listCopy[k] = v.Peer
	}
	return listCopy
}

// Len returns the length of the PeerList.
func (l *PeerList) Len() int {
	l.RLock()
	defer l.RUnlock()
	return l.peerHeap.Len()
}

// exists checks if a hostport exists in the peer list.
func (l *PeerList) exists(hostPort string) (*peerScore, bool) {
	l.RLock()
	ps, ok := l.peersByHostPort[hostPort]
	l.RUnlock()

	return ps, ok
}

// getPeerScore is called to find the peer and its score from a host port key.
// Note that at least a Read lock must be held to call this function.
func (l *PeerList) getPeerScore(hostPort string) (*peerScore, uint64, bool) {
	ps, ok := l.peersByHostPort[hostPort]
	if !ok {
		return nil, 0, false
	}
	return ps, ps.score, ok
}

// onPeerChange is called when there is a change that may cause the peer's score to change.
// The new score is calculated, and the peer heap is updated with the new score if the score changes.
func (l *PeerList) onPeerChange(p *Peer) {
	l.RLock()
	ps, psScore, ok := l.getPeerScore(p.hostPort)
	sc := l.scoreCalculator
	l.RUnlock()
	if !ok {
		return
	}

	newScore := sc.GetScore(ps.Peer)
	if newScore == psScore {
		return
	}

	l.Lock()
	l.updatePeer(ps, newScore)
	l.Unlock()
}

// updatePeer is called to update the score of the peer given the existing score.
// Note that a Write lock must be held to call this function.
func (l *PeerList) updatePeer(ps *peerScore, newScore uint64) {
	if ps.score == newScore {
		return
	}

	ps.score = newScore
	l.peerHeap.updatePeer(ps)
}

// peerScore represents a peer and scoring for the peer heap.
// It is not safe for concurrent access, it should only be used through the PeerList.
type peerScore struct {
	*Peer

	// score according to the current peer list's ScoreCalculator.
	score uint64
	// index of the peerScore in the peerHeap. Used to interact with container/heap.
	index int
	// order is the tiebreaker for when score is equal. It is set when a peer
	// is pushed to the heap based on peerHeap.order with jitter.
	order uint64
}

func newPeerScore(p *Peer, score uint64) *peerScore {
	return &peerScore{
		Peer:  p,
		score: score,
		index: -1,
	}
}

// Peer represents a single autobahn service or client with a unique host:port.
type Peer struct {
	sync.RWMutex

	channel             Connectable
	hostPort            string
	onStatusChanged     func(*Peer)
	onClosedConnRemoved func(*Peer)

	// scCount is the number of subchannels that this peer is added to.
	scCount uint32

	// connections are mutable, and are protected by the mutex.
	newConnLock         sync.Mutex
	inboundConnections  []*Connection
	outboundConnections []*Connection
	chosenCount         atomic.Uint64

	// onUpdate is a test-only hook.
	onUpdate func(*Peer)
}

func newPeer(channel Connectable, hostPort string, onStatusChanged func(*Peer), onClosedConnRemoved func(*Peer)) *Peer {
	if hostPort == "" {
		panic("Cannot create peer with blank hostPort")
	}
	if onStatusChanged == nil {
		onStatusChanged = noopOnStatusChanged
	}
	return &Peer{
		channel:             channel,
		hostPort:            hostPort,
		onStatusChanged:     onStatusChanged,
		onClosedConnRemoved: onClosedConnRemoved,
	}
}

// HostPort returns the host:port used to connect to this peer.
func (p *Peer) HostPort() string {
	return p.hostPort
}

// getConn treats inbound and outbound connections as a single virtual list
// that can be indexed. The peer must be read-locked.
func (p *Peer) getConn(i int) *Connection {
	inboundLen := len(p.inboundConnections)
	if i < inboundLen {
		return p.inboundConnections[i]
	}

	return p.outboundConnections[i-inboundLen]
}

func (p *Peer) getActiveConnLocked() (*Connection, bool) {
	allConns := len(p.inboundConnections) + len(p.outboundConnections)
	if allConns == 0 {
		return nil, false
	}

	// We cycle through the connection list, starting at a random point
	// to avoid always choosing the same connection.
	startOffset := peerRng.Intn(allConns)
	for i := 0; i < allConns; i++ {
		connIndex := (i + startOffset) % allConns
		if conn := p.getConn(connIndex); conn.IsActive() {
			return conn, true
		}
	}

	return nil, false
}

// getActiveConn will randomly select an active connection.
// TODO(prashant): Should we clear inactive connections?
// TODO(prashant): Do we want some sort of scoring for connections?
func (p *Peer) getActiveConn() (*Connection, bool) {
	p.RLock()
	conn, ok := p.getActiveConnLocked()
	p.RUnlock()

	return conn, ok
}

// GetConnection returns an active connection to this peer. If no active connections
// are found, it will create a new outbound connection and return it.
func (p *Peer) GetConnection(ctx context.Context) (*Connection, error) {
	if activeConn, ok := p.getActiveConn(); ok {
		return activeConn, nil
	}

	// Lock here to restrict new connection creation attempts to one goroutine
	p.newConnLock.Lock()
	defer p.newConnLock.Unlock()

	// Check active connections again in case someone else got ahead of us.
	if activeConn, ok := p.getActiveConn(); ok {
		return activeConn, nil
	}

	// No active connections, make a new outgoing connection.
	return p.Connect(ctx)
}

// getConnectionRelay gets a connection, and uses the given timeout to lazily
// create a context if a new connection is required.
func (p *Peer) getConnectionRelay(timeout time.Duration) (*Connection, error) {
	if conn, ok := p.getActiveConn(); ok {
		return conn, nil
	}

	// Lock here to restrict new connection creation attempts to one goroutine
	p.newConnLock.Lock()
	defer p.newConnLock.Unlock()

	// Check active connections again in case someone else got ahead of us.
	if activeConn, ok := p.getActiveConn(); ok {
		return activeConn, nil
	}

	// When the relay creates outbound connections, we don't want those services
	// to ever connect back to us and send us traffic. We hide the host:port
	// so that service instances on remote machines don't try to connect back
	// and don't try to send Hyperbahn traffic on this connection.
	ctx, cancel := NewContextBuilder(timeout).HideListeningOnOutbound().Build()
	defer cancel()

	return p.Connect(ctx)
}

// addSC adds a reference to a peer from a subchannel (e.g. peer list).
func (p *Peer) addSC() {
	p.Lock()
	p.scCount++
	p.Unlock()
}

// delSC removes a reference to a peer from a subchannel (e.g. peer list).
func (p *Peer) delSC() {
	p.Lock()
	p.scCount--
	p.Unlock()
}

// canRemove returns whether this peer can be safely removed from the root peer list.
func (p *Peer) canRemove() bool {
	p.RLock()
	count := len(p.inboundConnections) + len(p.outboundConnections) + int(p.scCount)
	p.RUnlock()
	return count == 0
}

// addConnection adds an active connection to the peer's connection list.
// If a connection is not active, returns ErrInvalidConnectionState.
func (p *Peer) addConnection(c *Connection, direction connectionDirection) error {
	conns := p.connectionsFor(direction)

	if c.readState() != connectionActive {
		return ErrInvalidConnectionState
	}

	p.Lock()
	*conns = append(*conns, c)
	p.Unlock()

	// Inform third parties that a peer gained a connection.
	p.onStatusChanged(p)

	return nil
}

func (p *Peer) connectionsFor(direction connectionDirection) *[]*Connection {
	if direction == inbound {
		return &p.inboundConnections
	}
	return &p.outboundConnections
}

// removeConnection will check remove the connection if it exists on connsPtr
// and returns whether it removed the connection.
func (p *Peer) removeConnection(connsPtr *[]*Connection, changed *Connection) bool {
	conns := *connsPtr
	for i, c := range conns {
		if c == changed {
			// Remove the connection by moving the last item forward, and slicing the list.
			last := len(conns) - 1
			conns[i], conns[last] = conns[last], nil
			*connsPtr = conns[:last]
			return true
		}
	}

	return false
}

// connectionStateChanged is called when one of the peers' connections states changes.
// All non-active connections are removed from the peer. The connection will
// still be tracked by the channel until it's completely closed.
func (p *Peer) connectionCloseStateChange(changed *Connection) {
	if changed.IsActive() {
		return
	}

	p.Lock()
	found := p.removeConnection(&p.inboundConnections, changed)
	if !found {
		found = p.removeConnection(&p.outboundConnections, changed)
	}
	p.Unlock()

	if found {
		p.onClosedConnRemoved(p)
		// Inform third parties that a peer lost a connection.
		p.onStatusChanged(p)
	}
}

// Connect adds a new outbound connection to the peer.
func (p *Peer) Connect(ctx context.Context) (*Connection, error) {
	return p.channel.Connect(ctx, p.hostPort)
}

// BeginCall starts a new call to this specific peer, returning an OutboundCall that can
// be used to write the arguments of the call.
func (p *Peer) BeginCall(ctx context.Context, serviceName, methodName string, callOptions *CallOptions) (*OutboundCall, error) {
	if callOptions == nil {
		callOptions = defaultCallOptions
	}
	callOptions.RequestState.AddSelectedPeer(p.HostPort())

	if err := validateCall(ctx, serviceName, methodName, callOptions); err != nil {
		return nil, err
	}

	conn, err := p.GetConnection(ctx)
	if err != nil {
		return nil, err
	}

	call, err := conn.beginCall(ctx, serviceName, methodName, callOptions)
	if err != nil {
		return nil, err
	}

	return call, err
}

// NumConnections returns the number of inbound and outbound connections for this peer.
func (p *Peer) NumConnections() (inbound int, outbound int) {
	p.RLock()
	inbound = len(p.inboundConnections)
	outbound = len(p.outboundConnections)
	p.RUnlock()
	return inbound, outbound
}

// NumPendingOutbound returns the number of pending outbound calls.
func (p *Peer) NumPendingOutbound() int {
	count := 0
	p.RLock()
	for _, c := range p.outboundConnections {
		count += c.outbound.count()
	}

	for _, c := range p.inboundConnections {
		count += c.outbound.count()
	}
	p.RUnlock()
	return count
}

func (p *Peer) runWithConnections(f func(*Connection)) {
	p.RLock()
	for _, c := range p.inboundConnections {
		f(c)
	}

	for _, c := range p.outboundConnections {
		f(c)
	}
	p.RUnlock()
}

func (p *Peer) callOnUpdateComplete() {
	p.RLock()
	f := p.onUpdate
	p.RUnlock()

	if f != nil {
		f(p)
	}
}

func noopOnStatusChanged(*Peer) {}

// isEphemeralHostPort returns if hostPort is the default ephemeral hostPort.
func isEphemeralHostPort(hostPort string) bool {
	return hostPort == "" || hostPort == ephemeralHostPort || strings.HasSuffix(hostPort, ":0")
}
