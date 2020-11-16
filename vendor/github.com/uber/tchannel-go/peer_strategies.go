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

import "math"

// ScoreCalculator defines the interface to calculate the score.
type ScoreCalculator interface {
	GetScore(p *Peer) uint64
}

// ScoreCalculatorFunc is an adapter that allows functions to be used as ScoreCalculator
type ScoreCalculatorFunc func(p *Peer) uint64

// GetScore calls the underlying function.
func (f ScoreCalculatorFunc) GetScore(p *Peer) uint64 {
	return f(p)
}

type zeroCalculator struct{}

func (zeroCalculator) GetScore(p *Peer) uint64 {
	return 0
}

func newZeroCalculator() zeroCalculator {
	return zeroCalculator{}
}

type leastPendingCalculator struct{}

func (leastPendingCalculator) GetScore(p *Peer) uint64 {
	inbound, outbound := p.NumConnections()
	if inbound+outbound == 0 {
		return math.MaxUint64
	}

	return uint64(p.NumPendingOutbound())
}

// newLeastPendingCalculator returns a strategy prefers any connected peer.
// Within connected peers, least pending calls is used. Peers with less pending outbound calls
// get a smaller score.
func newLeastPendingCalculator() leastPendingCalculator {
	return leastPendingCalculator{}
}

type preferIncomingCalculator struct{}

func (preferIncomingCalculator) GetScore(p *Peer) uint64 {
	inbound, outbound := p.NumConnections()
	if inbound+outbound == 0 {
		return math.MaxUint64
	}

	numPendingOutbound := uint64(p.NumPendingOutbound())
	if inbound == 0 {
		return math.MaxInt32 + numPendingOutbound
	}

	return numPendingOutbound
}

// newPreferIncomingCalculator returns a strategy that prefers peers with incoming connections.
// The scoring tiers are:
// Peers with incoming connections, peers with any connections, unconnected peers.
// Within each tier, least pending calls is used. Peers with less pending outbound calls
// get a smaller score.
func newPreferIncomingCalculator() preferIncomingCalculator {
	return preferIncomingCalculator{}
}
