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

import "sync"

// RootPeerList is the root peer list which is only used to connect to
// peers and share peers between subchannels.
type RootPeerList struct {
	sync.RWMutex

	channel             Connectable
	onPeerStatusChanged func(*Peer)
	peersByHostPort     map[string]*Peer
}

func newRootPeerList(ch Connectable, onPeerStatusChanged func(*Peer)) *RootPeerList {
	return &RootPeerList{
		channel:             ch,
		onPeerStatusChanged: onPeerStatusChanged,
		peersByHostPort:     make(map[string]*Peer),
	}
}

// newChild returns a new isolated peer list that shares the underlying peers
// with the root peer list.
func (l *RootPeerList) newChild() *PeerList {
	return newPeerList(l)
}

// Add adds a peer to the root peer list if it does not exist, or return
// an existing peer if it exists.
func (l *RootPeerList) Add(hostPort string) *Peer {
	l.RLock()

	if p, ok := l.peersByHostPort[hostPort]; ok {
		l.RUnlock()
		return p
	}

	l.RUnlock()
	l.Lock()
	defer l.Unlock()

	if p, ok := l.peersByHostPort[hostPort]; ok {
		return p
	}

	var p *Peer
	// To avoid duplicate connections, only the root list should create new
	// peers. All other lists should keep refs to the root list's peers.
	p = newPeer(l.channel, hostPort, l.onPeerStatusChanged, l.onClosedConnRemoved)
	l.peersByHostPort[hostPort] = p
	return p
}

// GetOrAdd returns a peer for the given hostPort, creating one if it doesn't yet exist.
func (l *RootPeerList) GetOrAdd(hostPort string) *Peer {
	peer, ok := l.Get(hostPort)
	if ok {
		return peer
	}

	return l.Add(hostPort)
}

// Get returns a peer for the given hostPort if it exists.
func (l *RootPeerList) Get(hostPort string) (*Peer, bool) {
	l.RLock()
	p, ok := l.peersByHostPort[hostPort]
	l.RUnlock()
	return p, ok
}

func (l *RootPeerList) onClosedConnRemoved(peer *Peer) {
	hostPort := peer.HostPort()
	p, ok := l.Get(hostPort)
	if !ok {
		// It's possible that multiple connections were closed and removed at the same time,
		// so multiple goroutines might be removing the peer from the root peer list.
		return
	}

	if p.canRemove() {
		l.Lock()
		delete(l.peersByHostPort, hostPort)
		l.Unlock()
		l.channel.Logger().WithFields(
			LogField{"remoteHostPort", hostPort},
		).Debug("Removed peer from root peer list.")
	}
}

// Copy returns a map of the peer list. This method should only be used for testing.
func (l *RootPeerList) Copy() map[string]*Peer {
	l.RLock()
	defer l.RUnlock()

	listCopy := make(map[string]*Peer)
	for k, v := range l.peersByHostPort {
		listCopy[k] = v
	}
	return listCopy
}
