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

package tnet

import (
	"net"
	"sync"
)

// Wrap returns a new Listener around the provided net.Listener.
// The returned Listener has a guarantee that when Close returns, it will no longer
// accept any new connections.
// See: https://github.com/uber/tchannel-go/issues/141
func Wrap(l net.Listener) net.Listener {
	return &listener{Listener: l, cond: sync.NewCond(&sync.Mutex{})}
}

// listener wraps a net.Listener and ensures that once Listener.Close returns,
// the underlying socket has been closed.
//
// The default Listener returns from Close before the underlying socket has been closed
// if another goroutine has an active reference (e.g. is in Accept).
// The following can happen:
// Goroutine 1 is running Accept, and is blocked, waiting for epoll
// Goroutine 2 calls Close. It sees an extra reference, and so cannot destroy
//  the socket, but instead decrements a reference, marks the connection as closed
//  and unblocks epoll.
// Goroutine 2 returns to the caller, makes a new connection.
// The new connection is sent to the socket (since it hasn't been destroyed)
// Goroutine 1 returns from epoll, and accepts the new connection.
//
// To avoid accepting connections after Close, we block Goroutine 2 from returning from Close
// till Accept returns an error to the user.
type listener struct {
	net.Listener

	// cond is used signal Close when there are no references to the listener.
	cond *sync.Cond
	refs int
}

func (s *listener) incRef() {
	s.cond.L.Lock()
	s.refs++
	s.cond.L.Unlock()
}

func (s *listener) decRef() {
	s.cond.L.Lock()
	s.refs--
	newRefs := s.refs
	s.cond.L.Unlock()
	if newRefs == 0 {
		s.cond.Broadcast()
	}
}

// Accept waits for and returns the next connection to the listener.
func (s *listener) Accept() (net.Conn, error) {
	s.incRef()
	defer s.decRef()
	return s.Listener.Accept()
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (s *listener) Close() error {
	if err := s.Listener.Close(); err != nil {
		return err
	}

	s.cond.L.Lock()
	for s.refs > 0 {
		s.cond.Wait()
	}
	s.cond.L.Unlock()
	return nil
}
