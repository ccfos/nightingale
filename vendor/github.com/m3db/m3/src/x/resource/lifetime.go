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

package resource

import "sync"

// CancellableLifetime describes a lifetime for a resource that
// allows checking out the resource and returning it and once
// cancelled will not allow any further checkouts.
type CancellableLifetime struct {
	mu        sync.RWMutex
	cancelled bool
}

// NewCancellableLifetime returns a new cancellable resource lifetime.
func NewCancellableLifetime() *CancellableLifetime {
	return &CancellableLifetime{}
}

// TryCheckout will try to checkout the resource, if the lifetime
// is already cancelled this will return false, otherwise it will return
// true and guarantee the lifetime is not cancelled until the checkout
// is returned.
// If this returns true you MUST call ReleaseCheckout later, otherwise
// the lifetime will never close and any caller calling Cancel will be
// blocked indefinitely.
func (l *CancellableLifetime) TryCheckout() bool {
	l.mu.RLock()
	if l.cancelled {
		// Already cancelled, close the RLock don't need to keep it open
		l.mu.RUnlock()
		return false
	}

	// Keep the RLock open
	return true
}

// ReleaseCheckout will decrement the number of current checkouts, it MUST
// only be called after a call to TryCheckout and must not be called more
// than once per call to TryCheckout or else it will panic as it will try
// to unlock an unlocked resource.
func (l *CancellableLifetime) ReleaseCheckout() {
	l.mu.RUnlock()
}

// Cancel will wait for all current checkouts to be returned
// and then will cancel the lifetime so that it cannot be
// checked out any longer.
func (l *CancellableLifetime) Cancel() {
	l.mu.Lock()
	l.cancelled = true
	l.mu.Unlock()
}
