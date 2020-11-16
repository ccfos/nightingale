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

import "github.com/uber/tchannel-go/relay"

// RelayHost is the interface used to create RelayCalls when the relay
// receives an incoming call.
type RelayHost interface {
	// SetChannels is called on creation of the channel. It's used to set a
	// channel reference which can be used to get references to *Peer.
	SetChannel(ch *Channel)

	// Start starts a new RelayCall given the call frame and connection.
	// It may return a call and an error, in which case the caller will
	// call Failed/End on the RelayCall.
	Start(relay.CallFrame, *relay.Conn) (RelayCall, error)
}

// RelayCall abstracts away peer selection, stats, and any other business
// logic from the underlying relay implementation. A RelayCall may not
// have a destination if there was an error during peer selection
// (which should be returned from start).
type RelayCall interface {
	// Destination returns the selected peer (if there was no error from Start).
	Destination() (peer *Peer, ok bool)

	// The call succeeded (possibly after retrying).
	Succeeded()

	// The call failed.
	Failed(reason string)

	// End stats collection for this RPC. Will be called exactly once.
	End()
}
