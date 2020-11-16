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
	"math/rand"

	"github.com/uber/tchannel-go/trand"
)

// peerHeap maintains a min-heap of peers based on the peers' score. All method
// calls must be serialized externally.
type peerHeap struct {
	peerScores []*peerScore
	rng        *rand.Rand
	order      uint64
}

func newPeerHeap() *peerHeap {
	return &peerHeap{rng: trand.NewSeeded()}
}

func (ph peerHeap) Len() int { return len(ph.peerScores) }

func (ph *peerHeap) Less(i, j int) bool {
	if ph.peerScores[i].score == ph.peerScores[j].score {
		return ph.peerScores[i].order < ph.peerScores[j].order
	}
	return ph.peerScores[i].score < ph.peerScores[j].score
}

func (ph peerHeap) Swap(i, j int) {
	ph.peerScores[i], ph.peerScores[j] = ph.peerScores[j], ph.peerScores[i]
	ph.peerScores[i].index = i
	ph.peerScores[j].index = j
}

// Push implements heap Push interface
func (ph *peerHeap) Push(x interface{}) {
	n := len(ph.peerScores)
	item := x.(*peerScore)
	item.index = n
	ph.peerScores = append(ph.peerScores, item)
}

// Pop implements heap Pop interface
func (ph *peerHeap) Pop() interface{} {
	old := *ph
	n := len(old.peerScores)
	item := old.peerScores[n-1]
	item.index = -1 // for safety
	ph.peerScores = old.peerScores[:n-1]
	return item
}

// updatePeer updates the score for the given peer.
func (ph *peerHeap) updatePeer(peerScore *peerScore) {
	heap.Fix(ph, peerScore.index)
}

// removePeer remove peer at specific index.
func (ph *peerHeap) removePeer(peerScore *peerScore) {
	heap.Remove(ph, peerScore.index)
}

// popPeer pops the top peer of the heap.
func (ph *peerHeap) popPeer() *peerScore {
	return heap.Pop(ph).(*peerScore)
}

// pushPeer pushes the new peer into the heap.
func (ph *peerHeap) pushPeer(peerScore *peerScore) {
	ph.order++
	newOrder := ph.order
	// randRange will affect the deviation of peer's chosenCount
	randRange := ph.Len()/2 + 1
	peerScore.order = newOrder + uint64(ph.rng.Intn(randRange))
	heap.Push(ph, peerScore)
}

func (ph *peerHeap) swapOrder(i, j int) {
	if i == j {
		return
	}

	ph.peerScores[i].order, ph.peerScores[j].order = ph.peerScores[j].order, ph.peerScores[i].order
	heap.Fix(ph, i)
	heap.Fix(ph, j)
}

// AddPeer adds a peer to the peer heap.
func (ph *peerHeap) addPeer(peerScore *peerScore) {
	ph.pushPeer(peerScore)

	// Pick a random element, and swap the order with that peerScore.
	r := ph.rng.Intn(ph.Len())
	ph.swapOrder(peerScore.index, r)
}

// Exposed for testing purposes.
func (ph *peerHeap) peek() *peerScore {
	return ph.peerScores[0]
}
