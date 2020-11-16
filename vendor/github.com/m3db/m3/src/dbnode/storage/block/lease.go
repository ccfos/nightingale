// Copyright (c) 2019 Uber Technologies, Inc.
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

package block

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	errLeaserAlreadyRegistered        = errors.New("leaser already registered")
	errLeaserNotRegistered            = errors.New("leaser not registered")
	errOpenLeaseVerifierNotSet        = errors.New("cannot open leases while verifier is not set")
	errUpdateOpenLeasesVerifierNotSet = errors.New("cannot update open leases while verifier is not set")
	errConcurrentUpdateOpenLeases     = errors.New("cannot call updateOpenLeases() concurrently")
)

type leaseManager struct {
	sync.Mutex
	updateOpenLeasesInProgress sync.Map
	leasers                    []Leaser
	verifier                   LeaseVerifier
}

// NewLeaseManager creates a new lease manager with a provided
// lease verifier (to ensure leases are valid when made).
func NewLeaseManager(verifier LeaseVerifier) LeaseManager {
	return &leaseManager{
		verifier: verifier,
	}
}

func (m *leaseManager) RegisterLeaser(leaser Leaser) error {
	m.Lock()
	defer m.Unlock()

	if m.isRegistered(leaser) {
		return errLeaserAlreadyRegistered
	}
	m.leasers = append(m.leasers, leaser)

	return nil
}

func (m *leaseManager) UnregisterLeaser(leaser Leaser) error {
	m.Lock()
	defer m.Unlock()

	var leasers []Leaser
	for _, l := range m.leasers {
		if l != leaser {
			leasers = append(leasers, l)
		}
	}

	if len(leasers) != len(m.leasers)-1 {
		return errLeaserNotRegistered
	}

	m.leasers = leasers

	return nil
}

func (m *leaseManager) OpenLease(
	leaser Leaser,
	descriptor LeaseDescriptor,
	state LeaseState,
) error {
	// NB(r): Take exclusive lock so that upgrade leases can't be called
	// while we are verifying a lease (racey)
	// NB(bodu): We don't use defer here since the lease verifier takes out a
	// a db lock when retrieving flush states resulting in a potential deadlock.
	m.Lock()

	if m.verifier == nil {
		m.Unlock()
		return errOpenLeaseVerifierNotSet
	}

	if !m.isRegistered(leaser) {
		m.Unlock()
		return errLeaserNotRegistered
	}

	m.Unlock()
	return m.verifier.VerifyLease(descriptor, state)
}

func (m *leaseManager) OpenLatestLease(
	leaser Leaser,
	descriptor LeaseDescriptor,
) (LeaseState, error) {
	// NB(r): Take exclusive lock so that upgrade leases can't be called
	// while we are verifying a lease (racey)
	// NB(bodu): We don't use defer here since the lease verifier takes out a
	// a db lock when retrieving flush states resulting in a potential deadlock.
	m.Lock()

	if m.verifier == nil {
		m.Unlock()
		return LeaseState{}, errOpenLeaseVerifierNotSet
	}

	if !m.isRegistered(leaser) {
		m.Unlock()
		return LeaseState{}, errLeaserNotRegistered
	}

	m.Unlock()
	return m.verifier.LatestState(descriptor)
}

func (m *leaseManager) UpdateOpenLeases(
	descriptor LeaseDescriptor,
	state LeaseState,
) (UpdateLeasesResult, error) {
	m.Lock()
	if m.verifier == nil {
		m.Unlock()
		return UpdateLeasesResult{}, errUpdateOpenLeasesVerifierNotSet
	}
	// NB(rartoul): Release lock while calling UpdateOpenLease() so that
	// calls to OpenLease() and OpenLatestLease() are not blocked which
	// would blocks reads and could cause deadlocks if those calls were
	// made while holding locks that would not allow UpdateOpenLease() to
	// return before being released.
	m.Unlock()

	hashableDescriptor := newHashableLeaseDescriptor(descriptor)
	if _, ok := m.updateOpenLeasesInProgress.LoadOrStore(hashableDescriptor, struct{}{}); ok {
		// Prevent UpdateOpenLeases() calls from happening concurrently (since the lock
		// is not held for the duration) to ensure that Leaser's receive all updates
		// and in the correct order.
		return UpdateLeasesResult{}, errConcurrentUpdateOpenLeases
	}

	defer m.updateOpenLeasesInProgress.Delete(hashableDescriptor)

	var result UpdateLeasesResult
	for _, l := range m.leasers {
		r, err := l.UpdateOpenLease(descriptor, state)
		if err != nil {
			return result, err
		}

		switch r {
		case UpdateOpenLease:
			result.LeasersUpdatedLease++
		case NoOpenLease:
			result.LeasersNoOpenLease++
		default:
			return result, fmt.Errorf("unknown update open lease result: %d", r)
		}
	}

	return result, nil
}

func (m *leaseManager) SetLeaseVerifier(leaseVerifier LeaseVerifier) error {
	m.Lock()
	defer m.Unlock()
	m.verifier = leaseVerifier
	return nil
}

func (m *leaseManager) isRegistered(leaser Leaser) bool {
	for _, l := range m.leasers {
		if l == leaser {
			return true
		}
	}
	return false
}

type hashableLeaseDescriptor struct {
	namespace  string
	shard      uint32
	blockStart time.Time
}

func newHashableLeaseDescriptor(descriptor LeaseDescriptor) hashableLeaseDescriptor {
	ns := ""
	if descriptor.Namespace != nil {
		ns = descriptor.Namespace.String()
	}
	return hashableLeaseDescriptor{
		namespace:  ns,
		shard:      descriptor.Shard,
		blockStart: descriptor.BlockStart,
	}
}
