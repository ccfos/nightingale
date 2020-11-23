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
	"fmt"
	"sync"

	"github.com/opentracing/opentracing-go"
	"golang.org/x/net/context"
)

// SubChannelOption are used to set options for subchannels.
type SubChannelOption func(*SubChannel)

// Isolated is a SubChannelOption that creates an isolated subchannel.
func Isolated(s *SubChannel) {
	s.Lock()
	s.peers = s.topChannel.peers.newSibling()
	s.peers.SetStrategy(newLeastPendingCalculator())
	s.Unlock()
}

// SubChannel allows calling a specific service on a channel.
// TODO(prashant): Allow creating a subchannel with default call options.
// TODO(prashant): Allow registering handlers on a subchannel.
type SubChannel struct {
	sync.RWMutex
	serviceName        string
	topChannel         *Channel
	defaultCallOptions *CallOptions
	peers              *PeerList
	handler            Handler
	logger             Logger
	statsReporter      StatsReporter
}

// Map of subchannel and the corresponding service
type subChannelMap struct {
	sync.RWMutex
	subchannels map[string]*SubChannel
}

func newSubChannel(serviceName string, ch *Channel) *SubChannel {
	logger := ch.Logger().WithFields(LogField{"subchannel", serviceName})
	return &SubChannel{
		serviceName:   serviceName,
		peers:         ch.peers,
		topChannel:    ch,
		handler:       &handlerMap{}, // use handlerMap by default
		logger:        logger,
		statsReporter: ch.StatsReporter(),
	}
}

// ServiceName returns the service name that this subchannel is for.
func (c *SubChannel) ServiceName() string {
	return c.serviceName
}

// BeginCall starts a new call to a remote peer, returning an OutboundCall that can
// be used to write the arguments of the call.
func (c *SubChannel) BeginCall(ctx context.Context, methodName string, callOptions *CallOptions) (*OutboundCall, error) {
	if callOptions == nil {
		callOptions = defaultCallOptions
	}

	peer, err := c.peers.Get(callOptions.RequestState.PrevSelectedPeers())
	if err != nil {
		return nil, err
	}

	return peer.BeginCall(ctx, c.ServiceName(), methodName, callOptions)
}

// Peers returns the PeerList for this subchannel.
func (c *SubChannel) Peers() *PeerList {
	return c.peers
}

// Isolated returns whether this subchannel is an isolated subchannel.
func (c *SubChannel) Isolated() bool {
	c.RLock()
	defer c.RUnlock()
	return c.topChannel.Peers() != c.peers
}

// Register registers a handler on the subchannel for the given method.
//
// This function panics if the Handler for the SubChannel was overwritten with
// SetHandler.
func (c *SubChannel) Register(h Handler, methodName string) {
	handlers, ok := c.handler.(*handlerMap)
	if !ok {
		panic(fmt.Sprintf(
			"handler for SubChannel(%v) was changed to disallow method registration",
			c.ServiceName(),
		))
	}
	handlers.register(h, methodName)
}

// GetHandlers returns all handlers registered on this subchannel by method name.
//
// This function panics if the Handler for the SubChannel was overwritten with
// SetHandler.
func (c *SubChannel) GetHandlers() map[string]Handler {
	handlers, ok := c.handler.(*handlerMap)
	if !ok {
		panic(fmt.Sprintf(
			"handler for SubChannel(%v) was changed to disallow method registration",
			c.ServiceName(),
		))
	}

	handlers.RLock()
	handlersMap := make(map[string]Handler, len(handlers.handlers))
	for k, v := range handlers.handlers {
		handlersMap[k] = v
	}
	handlers.RUnlock()
	return handlersMap
}

// SetHandler changes the SubChannel's underlying handler. This may be used to
// set up a catch-all Handler for all requests received by this SubChannel.
//
// Methods registered on this SubChannel using Register() before calling
// SetHandler() will be forgotten. Further calls to Register() on this
// SubChannel after SetHandler() is called will cause panics.
func (c *SubChannel) SetHandler(h Handler) {
	c.handler = h
}

// Logger returns the logger for this subchannel.
func (c *SubChannel) Logger() Logger {
	return c.logger
}

// StatsReporter returns the stats reporter for this subchannel.
func (c *SubChannel) StatsReporter() StatsReporter {
	return c.topChannel.StatsReporter()
}

// StatsTags returns the stats tags for this subchannel.
func (c *SubChannel) StatsTags() map[string]string {
	tags := c.topChannel.StatsTags()
	tags["subchannel"] = c.serviceName
	return tags
}

// Tracer returns OpenTracing Tracer from the top channel.
func (c *SubChannel) Tracer() opentracing.Tracer {
	return c.topChannel.Tracer()
}

// Register a new subchannel for the given serviceName
func (subChMap *subChannelMap) registerNewSubChannel(serviceName string, ch *Channel) (_ *SubChannel, added bool) {
	subChMap.Lock()
	defer subChMap.Unlock()

	if subChMap.subchannels == nil {
		subChMap.subchannels = make(map[string]*SubChannel)
	}

	if sc, ok := subChMap.subchannels[serviceName]; ok {
		return sc, false
	}

	sc := newSubChannel(serviceName, ch)
	subChMap.subchannels[serviceName] = sc
	return sc, true
}

// Get subchannel if, we have one
func (subChMap *subChannelMap) get(serviceName string) (*SubChannel, bool) {
	subChMap.RLock()
	sc, ok := subChMap.subchannels[serviceName]
	subChMap.RUnlock()
	return sc, ok
}

// GetOrAdd a subchannel for the given serviceName on the map
func (subChMap *subChannelMap) getOrAdd(serviceName string, ch *Channel) (_ *SubChannel, added bool) {
	if sc, ok := subChMap.get(serviceName); ok {
		return sc, false
	}

	return subChMap.registerNewSubChannel(serviceName, ch)
}

func (subChMap *subChannelMap) updatePeer(p *Peer) {
	subChMap.RLock()
	for _, subCh := range subChMap.subchannels {
		if subCh.Isolated() {
			subCh.RLock()
			subCh.Peers().onPeerChange(p)
			subCh.RUnlock()
		}
	}
	subChMap.RUnlock()
}
