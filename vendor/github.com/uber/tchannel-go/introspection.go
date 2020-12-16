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
	"encoding/json"
	"runtime"
	"sort"
	"strconv"
	"time"

	"golang.org/x/net/context"
)

// IntrospectionOptions are the options used when introspecting the Channel.
type IntrospectionOptions struct {
	// IncludeExchanges will include all the IDs in the message exchanges.
	IncludeExchanges bool `json:"includeExchanges"`

	// IncludeEmptyPeers will include peers, even if they have no connections.
	IncludeEmptyPeers bool `json:"includeEmptyPeers"`

	// IncludeTombstones will include tombstones when introspecting relays.
	IncludeTombstones bool `json:"includeTombstones"`

	// IncludeOtherChannels will include basic information about other channels
	// created in the same process as this channel.
	IncludeOtherChannels bool `json:"includeOtherChannels"`
}

// RuntimeVersion includes version information about the runtime and
// the tchannel library.
type RuntimeVersion struct {
	GoVersion      string `json:"goVersion"`
	LibraryVersion string `json:"tchannelVersion"`
}

// RuntimeState is a snapshot of the runtime state for a channel.
type RuntimeState struct {
	ID           uint32 `json:"id"`
	ChannelState string `json:"channelState"`

	// CreatedStack is the stack for how this channel was created.
	CreatedStack string `json:"createdStack"`

	// LocalPeer is the local peer information (service name, host-port, etc).
	LocalPeer LocalPeerInfo `json:"localPeer"`

	// SubChannels contains information about any subchannels.
	SubChannels map[string]SubChannelRuntimeState `json:"subChannels"`

	// RootPeers contains information about all the peers on this channel and their connections.
	RootPeers map[string]PeerRuntimeState `json:"rootPeers"`

	// Peers is the list of shared peers for this channel.
	Peers []SubPeerScore `json:"peers"`

	// NumConnections is the number of connections stored in the channel.
	NumConnections int `json:"numConnections"`

	// Connections is the list of connection IDs in the channel
	Connections []uint32 ` json:"connections"`

	// InactiveConnections is the connection state for connections that are not active,
	// and hence are not reported as part of root peers.
	InactiveConnections []ConnectionRuntimeState `json:"inactiveConnections"`

	// OtherChannels is information about any other channels running in this process.
	OtherChannels map[string][]ChannelInfo `json:"otherChannels,omitEmpty"`

	// RuntimeVersion is the version information about the runtime and the library.
	RuntimeVersion RuntimeVersion `json:"runtimeVersion"`
}

// GoRuntimeStateOptions are the options used when getting Go runtime state.
type GoRuntimeStateOptions struct {
	// IncludeGoStacks will include all goroutine stacks.
	IncludeGoStacks bool `json:"includeGoStacks"`
}

// ChannelInfo is the state of other channels in the same process.
type ChannelInfo struct {
	ID           uint32        `json:"id"`
	CreatedStack string        `json:"createdStack"`
	LocalPeer    LocalPeerInfo `json:"localPeer"`
}

// GoRuntimeState is a snapshot of runtime stats from the runtime.
type GoRuntimeState struct {
	MemStats      runtime.MemStats `json:"memStats"`
	NumGoroutines int              `json:"numGoRoutines"`
	NumCPU        int              `json:"numCPU"`
	NumCGo        int64            `json:"numCGo"`
	GoStacks      []byte           `json:"goStacks,omitempty"`
}

// SubChannelRuntimeState is the runtime state for a subchannel.
type SubChannelRuntimeState struct {
	Service  string `json:"service"`
	Isolated bool   `json:"isolated"`
	// IsolatedPeers is the list of all isolated peers for this channel.
	IsolatedPeers []SubPeerScore      `json:"isolatedPeers,omitempty"`
	Handler       HandlerRuntimeState `json:"handler"`
}

// HandlerRuntimeState TODO
type HandlerRuntimeState struct {
	Type    handlerType `json:"type"`
	Methods []string    `json:"methods,omitempty"`
}

type handlerType string

func (h handlerType) String() string { return string(h) }

const (
	methodHandler   handlerType = "methods"
	overrideHandler             = "overriden"
)

// SubPeerScore show the runtime state of a peer with score.
type SubPeerScore struct {
	HostPort string `json:"hostPort"`
	Score    uint64 `json:"score"`
}

// ConnectionRuntimeState is the runtime state for a single connection.
type ConnectionRuntimeState struct {
	ID               uint32                  `json:"id"`
	ConnectionState  string                  `json:"connectionState"`
	LocalHostPort    string                  `json:"localHostPort"`
	RemoteHostPort   string                  `json:"remoteHostPort"`
	OutboundHostPort string                  `json:"outboundHostPort"`
	RemotePeer       PeerInfo                `json:"remotePeer"`
	InboundExchange  ExchangeSetRuntimeState `json:"inboundExchange"`
	OutboundExchange ExchangeSetRuntimeState `json:"outboundExchange"`
	Relayer          RelayerRuntimeState     `json:"relayer"`
	HealthChecks     []bool                  `json:"healthChecks,omitempty"`
	LastActivity     int64                   `json:"lastActivity"`
}

// RelayerRuntimeState is the runtime state for a single relayer.
type RelayerRuntimeState struct {
	Count         int               `json:"count"`
	InboundItems  RelayItemSetState `json:"inboundItems"`
	OutboundItems RelayItemSetState `json:"outboundItems"`
	MaxTimeout    time.Duration     `json:"maxTimeout"`
}

// ExchangeSetRuntimeState is the runtime state for a message exchange set.
type ExchangeSetRuntimeState struct {
	Name      string                          `json:"name"`
	Count     int                             `json:"count"`
	Exchanges map[string]ExchangeRuntimeState `json:"exchanges,omitempty"`
}

// RelayItemSetState is the runtime state for a list of relay items.
type RelayItemSetState struct {
	Name  string                    `json:"name"`
	Count int                       `json:"count"`
	Items map[string]RelayItemState `json:"items,omitempty"`
}

// ExchangeRuntimeState is the runtime state for a single message exchange.
type ExchangeRuntimeState struct {
	ID          uint32      `json:"id"`
	MessageType messageType `json:"messageType"`
}

// RelayItemState is the runtime state for a single relay item.
type RelayItemState struct {
	ID                      uint32 `json:"id"`
	RemapID                 uint32 `json:"remapID"`
	DestinationConnectionID uint32 `json:"destinationConnectionID"`
	Tomb                    bool   `json:"tomb"`
}

// PeerRuntimeState is the runtime state for a single peer.
type PeerRuntimeState struct {
	HostPort            string                   `json:"hostPort"`
	OutboundConnections []ConnectionRuntimeState `json:"outboundConnections"`
	InboundConnections  []ConnectionRuntimeState `json:"inboundConnections"`
	ChosenCount         uint64                   `json:"chosenCount"`
	SCCount             uint32                   `json:"scCount"`
}

// IntrospectState returns the RuntimeState for this channel.
// Note: this is purely for debugging and monitoring, and may slow down your Channel.
func (ch *Channel) IntrospectState(opts *IntrospectionOptions) *RuntimeState {
	if opts == nil {
		opts = &IntrospectionOptions{}
	}

	ch.mutable.RLock()
	state := ch.mutable.state
	numConns := len(ch.mutable.conns)
	inactiveConns := make([]*Connection, 0, numConns)
	connIDs := make([]uint32, 0, numConns)
	for id, conn := range ch.mutable.conns {
		connIDs = append(connIDs, id)
		if !conn.IsActive() {
			inactiveConns = append(inactiveConns, conn)
		}
	}

	ch.mutable.RUnlock()

	ch.State()
	return &RuntimeState{
		ID:                  ch.chID,
		ChannelState:        state.String(),
		CreatedStack:        ch.createdStack,
		LocalPeer:           ch.PeerInfo(),
		SubChannels:         ch.subChannels.IntrospectState(opts),
		RootPeers:           ch.RootPeers().IntrospectState(opts),
		Peers:               ch.Peers().IntrospectList(opts),
		NumConnections:      numConns,
		Connections:         connIDs,
		InactiveConnections: getConnectionRuntimeState(inactiveConns, opts),
		OtherChannels:       ch.IntrospectOthers(opts),
		RuntimeVersion:      introspectRuntimeVersion(),
	}
}

// IntrospectOthers returns the ChannelInfo for all other channels in this process.
func (ch *Channel) IntrospectOthers(opts *IntrospectionOptions) map[string][]ChannelInfo {
	if !opts.IncludeOtherChannels {
		return nil
	}

	channelMap.Lock()
	defer channelMap.Unlock()

	states := make(map[string][]ChannelInfo)
	for svc, channels := range channelMap.existing {
		channelInfos := make([]ChannelInfo, 0, len(channels))
		for _, otherChan := range channels {
			if ch == otherChan {
				continue
			}
			channelInfos = append(channelInfos, otherChan.ReportInfo(opts))
		}
		states[svc] = channelInfos
	}

	return states
}

// ReportInfo returns ChannelInfo for a channel.
func (ch *Channel) ReportInfo(opts *IntrospectionOptions) ChannelInfo {
	return ChannelInfo{
		ID:           ch.chID,
		CreatedStack: ch.createdStack,
		LocalPeer:    ch.PeerInfo(),
	}
}

type containsPeerList interface {
	Copy() map[string]*Peer
}

func fromPeerList(peers containsPeerList, opts *IntrospectionOptions) map[string]PeerRuntimeState {
	m := make(map[string]PeerRuntimeState)
	for _, peer := range peers.Copy() {
		peerState := peer.IntrospectState(opts)
		if len(peerState.InboundConnections)+len(peerState.OutboundConnections) > 0 || opts.IncludeEmptyPeers {
			m[peer.HostPort()] = peerState
		}
	}
	return m
}

// IntrospectState returns the runtime state of the
func (l *RootPeerList) IntrospectState(opts *IntrospectionOptions) map[string]PeerRuntimeState {
	return fromPeerList(l, opts)
}

// IntrospectState returns the runtime state of the subchannels.
func (subChMap *subChannelMap) IntrospectState(opts *IntrospectionOptions) map[string]SubChannelRuntimeState {
	m := make(map[string]SubChannelRuntimeState)
	subChMap.RLock()
	for k, sc := range subChMap.subchannels {
		state := SubChannelRuntimeState{
			Service:  k,
			Isolated: sc.Isolated(),
		}
		if state.Isolated {
			state.IsolatedPeers = sc.Peers().IntrospectList(opts)
		}
		if hmap, ok := sc.handler.(*handlerMap); ok {
			state.Handler.Type = methodHandler
			methods := make([]string, 0, len(hmap.handlers))
			for k := range hmap.handlers {
				methods = append(methods, k)
			}
			sort.Strings(methods)
			state.Handler.Methods = methods
		} else {
			state.Handler.Type = overrideHandler
		}
		m[k] = state
	}
	subChMap.RUnlock()
	return m
}

func getConnectionRuntimeState(conns []*Connection, opts *IntrospectionOptions) []ConnectionRuntimeState {
	connStates := make([]ConnectionRuntimeState, len(conns))

	for i, conn := range conns {
		connStates[i] = conn.IntrospectState(opts)
	}

	return connStates
}

// IntrospectState returns the runtime state for this peer.
func (p *Peer) IntrospectState(opts *IntrospectionOptions) PeerRuntimeState {
	p.RLock()
	defer p.RUnlock()

	return PeerRuntimeState{
		HostPort:            p.hostPort,
		InboundConnections:  getConnectionRuntimeState(p.inboundConnections, opts),
		OutboundConnections: getConnectionRuntimeState(p.outboundConnections, opts),
		ChosenCount:         p.chosenCount.Load(),
		SCCount:             p.scCount,
	}
}

// IntrospectState returns the runtime state for this connection.
func (c *Connection) IntrospectState(opts *IntrospectionOptions) ConnectionRuntimeState {
	c.stateMut.RLock()
	defer c.stateMut.RUnlock()

	// TODO(prashantv): Add total number of health checks, and health check options.
	state := ConnectionRuntimeState{
		ID:               c.connID,
		ConnectionState:  c.state.String(),
		LocalHostPort:    c.conn.LocalAddr().String(),
		RemoteHostPort:   c.conn.RemoteAddr().String(),
		OutboundHostPort: c.outboundHP,
		RemotePeer:       c.remotePeerInfo,
		InboundExchange:  c.inbound.IntrospectState(opts),
		OutboundExchange: c.outbound.IntrospectState(opts),
		HealthChecks:     c.healthCheckHistory.asBools(),
		LastActivity:     c.lastActivity.Load(),
	}
	if c.relay != nil {
		state.Relayer = c.relay.IntrospectState(opts)
	}
	return state
}

// IntrospectState returns the runtime state for this relayer.
func (r *Relayer) IntrospectState(opts *IntrospectionOptions) RelayerRuntimeState {
	count := r.inbound.Count() + r.outbound.Count()
	return RelayerRuntimeState{
		Count:         count,
		InboundItems:  r.inbound.IntrospectState(opts, "inbound"),
		OutboundItems: r.outbound.IntrospectState(opts, "outbound"),
		MaxTimeout:    r.maxTimeout,
	}
}

// IntrospectState returns the runtime state for this relayItems.
func (ri *relayItems) IntrospectState(opts *IntrospectionOptions, name string) RelayItemSetState {
	setState := RelayItemSetState{
		Name:  name,
		Count: ri.Count(),
	}
	if opts.IncludeExchanges {
		ri.RLock()
		defer ri.RUnlock()

		setState.Items = make(map[string]RelayItemState, len(ri.items))
		for k, v := range ri.items {
			if !opts.IncludeTombstones && v.tomb {
				continue
			}
			state := RelayItemState{
				ID:                      k,
				RemapID:                 v.remapID,
				DestinationConnectionID: v.destination.conn.connID,
				Tomb:                    v.tomb,
			}
			setState.Items[strconv.Itoa(int(k))] = state
		}
	}

	return setState
}

// IntrospectState returns the runtime state for this messsage exchange set.
func (mexset *messageExchangeSet) IntrospectState(opts *IntrospectionOptions) ExchangeSetRuntimeState {
	mexset.RLock()
	setState := ExchangeSetRuntimeState{
		Name:  mexset.name,
		Count: len(mexset.exchanges),
	}

	if opts.IncludeExchanges {
		setState.Exchanges = make(map[string]ExchangeRuntimeState, len(mexset.exchanges))
		for k, v := range mexset.exchanges {
			state := ExchangeRuntimeState{
				ID:          k,
				MessageType: v.msgType,
			}
			setState.Exchanges[strconv.Itoa(int(k))] = state
		}
	}

	mexset.RUnlock()
	return setState
}

func getStacks(all bool) []byte {
	var buf []byte
	for n := 4096; n < 10*1024*1024; n *= 2 {
		buf = make([]byte, n)
		stackLen := runtime.Stack(buf, all)
		if stackLen < n {
			return buf[:stackLen]
		}
	}

	// return the first 10MB of stacks if we have more than 10MB.
	return buf
}
func (ch *Channel) handleIntrospection(arg3 []byte) interface{} {
	var opts IntrospectionOptions
	json.Unmarshal(arg3, &opts)
	return ch.IntrospectState(&opts)
}

// IntrospectList returns the list of peers (hostport, score) in this peer list.
func (l *PeerList) IntrospectList(opts *IntrospectionOptions) []SubPeerScore {
	var peers []SubPeerScore
	l.RLock()
	for _, ps := range l.peerHeap.peerScores {
		peers = append(peers, SubPeerScore{
			HostPort: ps.Peer.hostPort,
			Score:    ps.score,
		})
	}
	l.RUnlock()

	return peers
}

// IntrospectNumConnections returns the number of connections returns the number
// of connections. Note: like other introspection APIs, this is not a stable API.
func (ch *Channel) IntrospectNumConnections() int {
	ch.mutable.RLock()
	numConns := len(ch.mutable.conns)
	ch.mutable.RUnlock()
	return numConns
}

func handleInternalRuntime(arg3 []byte) interface{} {
	var opts GoRuntimeStateOptions
	json.Unmarshal(arg3, &opts)

	state := GoRuntimeState{
		NumGoroutines: runtime.NumGoroutine(),
		NumCPU:        runtime.NumCPU(),
		NumCGo:        runtime.NumCgoCall(),
	}
	runtime.ReadMemStats(&state.MemStats)
	if opts.IncludeGoStacks {
		state.GoStacks = getStacks(true /* all */)
	}

	return state
}

func introspectRuntimeVersion() RuntimeVersion {
	return RuntimeVersion{
		GoVersion:      runtime.Version(),
		LibraryVersion: VersionInfo,
	}
}

// registerInternal registers the following internal handlers which return runtime state:
//  _gometa_introspect: TChannel internal state.
//  _gometa_runtime: Golang runtime stats.
func (ch *Channel) registerInternal() {
	endpoints := []struct {
		name    string
		handler func([]byte) interface{}
	}{
		{"_gometa_introspect", ch.handleIntrospection},
		{"_gometa_runtime", handleInternalRuntime},
	}

	tchanSC := ch.GetSubChannel("tchannel")
	for _, ep := range endpoints {
		// We need ep in our closure.
		ep := ep
		handler := func(ctx context.Context, call *InboundCall) {
			var arg2, arg3 []byte
			if err := NewArgReader(call.Arg2Reader()).Read(&arg2); err != nil {
				return
			}
			if err := NewArgReader(call.Arg3Reader()).Read(&arg3); err != nil {
				return
			}
			if err := NewArgWriter(call.Response().Arg2Writer()).Write(nil); err != nil {
				return
			}
			NewArgWriter(call.Response().Arg3Writer()).WriteJSON(ep.handler(arg3))
		}
		ch.Register(HandlerFunc(handler), ep.name)
		tchanSC.Register(HandlerFunc(handler), ep.name)
	}
}
