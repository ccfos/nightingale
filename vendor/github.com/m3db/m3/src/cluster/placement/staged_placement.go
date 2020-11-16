// Copyright (c) 2017 Uber Technologies, Inc.
//
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

package placement

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/m3db/m3/src/cluster/generated/proto/placementpb"
	"github.com/m3db/m3/src/x/clock"
)

var (
	errNoApplicablePlacement       = errors.New("no applicable placement found")
	errActiveStagedPlacementClosed = errors.New("active staged placement is closed")
)

type activeStagedPlacement struct {
	sync.RWMutex

	placements            Placements
	nowFn                 clock.NowFn
	onPlacementsAddedFn   OnPlacementsAddedFn
	onPlacementsRemovedFn OnPlacementsRemovedFn

	expiring int32
	closed   bool
	doneFn   DoneFn
}

func newActiveStagedPlacement(
	placements []Placement,
	opts ActiveStagedPlacementOptions,
) ActiveStagedPlacement {
	if opts == nil {
		opts = NewActiveStagedPlacementOptions()
	}
	p := &activeStagedPlacement{
		placements:            placements,
		nowFn:                 opts.ClockOptions().NowFn(),
		onPlacementsAddedFn:   opts.OnPlacementsAddedFn(),
		onPlacementsRemovedFn: opts.OnPlacementsRemovedFn(),
	}
	p.doneFn = p.onPlacementDone

	if p.onPlacementsAddedFn != nil {
		p.onPlacementsAddedFn(placements)
	}

	return p
}

func (p *activeStagedPlacement) ActivePlacement() (Placement, DoneFn, error) {
	p.RLock()
	placement, err := p.activePlacementWithLock(p.nowFn().UnixNano())
	if err != nil {
		p.RUnlock()
		return nil, nil, err
	}
	return placement, p.doneFn, nil
}

func (p *activeStagedPlacement) Close() error {
	p.Lock()
	defer p.Unlock()

	if p.closed {
		return errActiveStagedPlacementClosed
	}
	if p.onPlacementsRemovedFn != nil {
		p.onPlacementsRemovedFn(p.placements)
	}
	p.placements = nil
	return nil
}

func (p *activeStagedPlacement) onPlacementDone() { p.RUnlock() }

func (p *activeStagedPlacement) activePlacementWithLock(timeNanos int64) (Placement, error) {
	if p.closed {
		return nil, errActiveStagedPlacementClosed
	}
	idx := p.placements.ActiveIndex(timeNanos)
	if idx < 0 {
		return nil, errNoApplicablePlacement
	}
	placement := p.placements[idx]
	// If the placement that's in effect is not the first placment, expire the stale ones.
	if idx > 0 && atomic.CompareAndSwapInt32(&p.expiring, 0, 1) {
		go p.expire()
	}
	return placement, nil
}

func (p *activeStagedPlacement) expire() {
	// NB(xichen): this improves readability at the slight cost of lambda capture
	// because this code path is triggered very infrequently.
	cleanup := func() {
		p.Unlock()
		atomic.StoreInt32(&p.expiring, 0)
	}
	p.Lock()
	defer cleanup()

	if p.closed {
		return
	}
	idx := p.placements.ActiveIndex(p.nowFn().UnixNano())
	if idx <= 0 {
		return
	}
	if p.onPlacementsRemovedFn != nil {
		p.onPlacementsRemovedFn(p.placements[:idx])
	}
	n := copy(p.placements[0:], p.placements[idx:])
	for i := n; i < len(p.placements); i++ {
		p.placements[i] = nil
	}
	p.placements = p.placements[:n]
}

type stagedPlacement struct {
	version    int
	placements Placements
	opts       ActiveStagedPlacementOptions
}

// NewStagedPlacement creates an empty staged placement.
func NewStagedPlacement() StagedPlacement {
	return &stagedPlacement{}
}

// NewStagedPlacementFromProto creates a new staged placement from proto.
func NewStagedPlacementFromProto(
	version int,
	p *placementpb.PlacementSnapshots,
	opts ActiveStagedPlacementOptions,
) (StagedPlacement, error) {
	placements, err := NewPlacementsFromProto(p)
	if err != nil {
		return nil, err
	}

	return &stagedPlacement{
		version:    version,
		placements: placements,
		opts:       opts,
	}, nil
}

func (sp *stagedPlacement) ActiveStagedPlacement(timeNanos int64) ActiveStagedPlacement {
	idx := len(sp.placements) - 1
	for idx >= 0 && sp.placements[idx].CutoverNanos() > timeNanos {
		idx--
	}
	if idx < 0 {
		return newActiveStagedPlacement(sp.placements, sp.opts)
	}
	return newActiveStagedPlacement(sp.placements[idx:], sp.opts)
}

func (sp *stagedPlacement) Version() int { return sp.version }

func (sp *stagedPlacement) SetVersion(version int) StagedPlacement {
	sp.version = version
	return sp
}

func (sp *stagedPlacement) Placements() Placements { return sp.placements }

func (sp *stagedPlacement) SetPlacements(placements []Placement) StagedPlacement {
	sort.Sort(placementsByCutoverAsc(placements))
	sp.placements = placements
	return sp
}

func (sp *stagedPlacement) ActiveStagedPlacementOptions() ActiveStagedPlacementOptions {
	return sp.opts
}

func (sp *stagedPlacement) SetActiveStagedPlacementOptions(
	opts ActiveStagedPlacementOptions,
) StagedPlacement {
	sp.opts = opts
	return sp
}

func (sp *stagedPlacement) Proto() (*placementpb.PlacementSnapshots, error) {
	return sp.Placements().Proto()
}

type placementsByCutoverAsc []Placement

func (s placementsByCutoverAsc) Len() int { return len(s) }

func (s placementsByCutoverAsc) Less(i, j int) bool {
	return s[i].CutoverNanos() < s[j].CutoverNanos()
}

func (s placementsByCutoverAsc) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
