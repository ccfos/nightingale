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
	"errors"
	"fmt"
	"sync"

	"github.com/uber/tchannel-go/typed"

	"go.uber.org/atomic"
	"golang.org/x/net/context"
)

var (
	errDuplicateMex        = errors.New("multiple attempts to use the message id")
	errMexShutdown         = errors.New("mex has been shutdown")
	errMexSetShutdown      = errors.New("mexset has been shutdown")
	errMexChannelFull      = NewSystemError(ErrCodeBusy, "cannot send frame to message exchange channel")
	errUnexpectedFrameType = errors.New("unexpected frame received")
)

const (
	messageExchangeSetInbound  = "inbound"
	messageExchangeSetOutbound = "outbound"

	// mexChannelBufferSize is the size of the message exchange channel buffer.
	mexChannelBufferSize = 2
)

type errNotifier struct {
	c        chan struct{}
	err      error
	notified atomic.Bool
}

func newErrNotifier() errNotifier {
	return errNotifier{c: make(chan struct{})}
}

// Notify will store the error and notify all waiters on c that there's an error.
func (e *errNotifier) Notify(err error) error {
	// The code should never try to Notify(nil).
	if err == nil {
		panic("cannot Notify with no error")
	}

	// There may be some sort of race where we try to notify the mex twice.
	if !e.notified.CAS(false, true) {
		return fmt.Errorf("cannot broadcast error: %v, already have: %v", err, e.err)
	}

	e.err = err
	close(e.c)
	return nil
}

// checkErr returns previously notified errors (if any).
func (e *errNotifier) checkErr() error {
	select {
	case <-e.c:
		return e.err
	default:
		return nil
	}
}

// A messageExchange tracks this Connections's side of a message exchange with a
// peer.  Each message exchange has a channel that can be used to receive
// frames from the peer, and a Context that can controls when the exchange has
// timed out or been cancelled.
type messageExchange struct {
	recvCh    chan *Frame
	errCh     errNotifier
	ctx       context.Context
	msgID     uint32
	msgType   messageType
	mexset    *messageExchangeSet
	framePool FramePool

	shutdownAtomic atomic.Bool
	errChNotified  atomic.Bool
}

// checkError is called before waiting on the mex channels.
// It returns any existing errors (timeout, cancellation, connection errors).
func (mex *messageExchange) checkError() error {
	if err := mex.ctx.Err(); err != nil {
		return GetContextError(err)
	}

	return mex.errCh.checkErr()
}

// forwardPeerFrame forwards a frame from a peer to the message exchange, where
// it can be pulled by whatever application thread is handling the exchange
func (mex *messageExchange) forwardPeerFrame(frame *Frame) error {
	// We want a very specific priority here:
	// 1. Timeouts/cancellation (mex.ctx errors)
	// 2. Whether recvCh has buffer space (non-blocking select over mex.recvCh)
	// 3. Other mex errors (mex.errCh)
	// Which is why we check the context error only (instead of mex.checkError).
	// In the mex.errCh case, we do a non-blocking write to recvCh to prioritize it.
	if err := mex.ctx.Err(); err != nil {
		return GetContextError(err)
	}

	select {
	case mex.recvCh <- frame:
		return nil
	case <-mex.ctx.Done():
		// Note: One slow reader processing a large request could stall the connection.
		// If we see this, we need to increase the recvCh buffer size.
		return GetContextError(mex.ctx.Err())
	case <-mex.errCh.c:
		// Select will randomly choose a case, but we want to prioritize
		// sending a frame over the errCh. Try a non-blocking write.
		select {
		case mex.recvCh <- frame:
			return nil
		default:
		}
		return mex.errCh.err
	}
}

func (mex *messageExchange) checkFrame(frame *Frame) error {
	if frame.Header.ID != mex.msgID {
		mex.mexset.log.WithFields(
			LogField{"msgId", mex.msgID},
			LogField{"header", frame.Header},
		).Error("recvPeerFrame received msg with unexpected ID.")
		return errUnexpectedFrameType
	}
	return nil
}

// recvPeerFrame waits for a new frame from the peer, or until the context
// expires or is cancelled
func (mex *messageExchange) recvPeerFrame() (*Frame, error) {
	// We have to check frames/errors in a very specific order here:
	// 1. Timeouts/cancellation (mex.ctx errors)
	// 2. Any pending frames (non-blocking select over mex.recvCh)
	// 3. Other mex errors (mex.errCh)
	// Which is why we check the context error only (instead of mex.checkError)e
	// In the mex.errCh case, we do a non-blocking read from recvCh to prioritize it.
	if err := mex.ctx.Err(); err != nil {
		return nil, GetContextError(err)
	}

	select {
	case frame := <-mex.recvCh:
		if err := mex.checkFrame(frame); err != nil {
			return nil, err
		}
		return frame, nil
	case <-mex.ctx.Done():
		return nil, GetContextError(mex.ctx.Err())
	case <-mex.errCh.c:
		// Select will randomly choose a case, but we want to prioritize
		// receiving a frame over errCh. Try a non-blocking read.
		select {
		case frame := <-mex.recvCh:
			if err := mex.checkFrame(frame); err != nil {
				return nil, err
			}
			return frame, nil
		default:
		}
		return nil, mex.errCh.err
	}
}

// recvPeerFrameOfType waits for a new frame of a given type from the peer, failing
// if the next frame received is not of that type.
// If an error frame is returned, then the errorMessage is returned as the error.
func (mex *messageExchange) recvPeerFrameOfType(msgType messageType) (*Frame, error) {
	frame, err := mex.recvPeerFrame()
	if err != nil {
		return nil, err
	}

	switch frame.Header.messageType {
	case msgType:
		return frame, nil

	case messageTypeError:
		// If we read an error frame, we can release it once we deserialize it.
		defer mex.framePool.Release(frame)

		errMsg := errorMessage{
			id: frame.Header.ID,
		}
		var rbuf typed.ReadBuffer
		rbuf.Wrap(frame.SizedPayload())
		if err := errMsg.read(&rbuf); err != nil {
			return nil, err
		}
		return nil, errMsg

	default:
		// TODO(mmihic): Should be treated as a protocol error
		mex.mexset.log.WithFields(
			LogField{"header", frame.Header},
			LogField{"expectedType", msgType},
			LogField{"expectedID", mex.msgID},
		).Warn("Received unexpected frame.")
		return nil, errUnexpectedFrameType
	}
}

// shutdown shuts down the message exchange, removing it from the message
// exchange set so  that it cannot receive more messages from the peer.  The
// receive channel remains open, however, in case there are concurrent
// goroutines sending to it.
func (mex *messageExchange) shutdown() {
	// The reader and writer side can both hit errors and try to shutdown the mex,
	// so we ensure that it's only shut down once.
	if !mex.shutdownAtomic.CAS(false, true) {
		return
	}

	if mex.errChNotified.CAS(false, true) {
		mex.errCh.Notify(errMexShutdown)
	}

	mex.mexset.removeExchange(mex.msgID)
}

// inboundExpired is called when an exchange is canceled or it times out,
// but a handler may still be running in the background. Since the handler may
// still write to the exchange, we cannot shutdown the exchange, but we should
// remove it from the connection's exchange list.
func (mex *messageExchange) inboundExpired() {
	mex.mexset.expireExchange(mex.msgID)
}

// A messageExchangeSet manages a set of active message exchanges.  It is
// mainly used to route frames from a peer to the appropriate messageExchange,
// or to cancel or mark a messageExchange as being in error.  Each Connection
// maintains two messageExchangeSets, one to manage exchanges that it has
// initiated (outbound), and another to manage exchanges that the peer has
// initiated (inbound).  The message-type specific handlers are responsible for
// ensuring that their message exchanges are properly registered and removed
// from the corresponding exchange set.
type messageExchangeSet struct {
	sync.RWMutex

	log       Logger
	name      string
	onRemoved func()
	onAdded   func()

	// maps are mutable, and are protected by the mutex.
	exchanges        map[uint32]*messageExchange
	expiredExchanges map[uint32]struct{}
	shutdown         bool
}

// newMessageExchangeSet creates a new messageExchangeSet with a given name.
func newMessageExchangeSet(log Logger, name string) *messageExchangeSet {
	return &messageExchangeSet{
		name:             name,
		log:              log.WithFields(LogField{"exchange", name}),
		exchanges:        make(map[uint32]*messageExchange),
		expiredExchanges: make(map[uint32]struct{}),
	}
}

// addExchange adds an exchange, it must be called with the mexset locked.
func (mexset *messageExchangeSet) addExchange(mex *messageExchange) error {
	if mexset.shutdown {
		return errMexSetShutdown
	}

	if _, ok := mexset.exchanges[mex.msgID]; ok {
		return errDuplicateMex
	}

	mexset.exchanges[mex.msgID] = mex
	return nil
}

// newExchange creates and adds a new message exchange to this set
func (mexset *messageExchangeSet) newExchange(ctx context.Context, framePool FramePool,
	msgType messageType, msgID uint32, bufferSize int) (*messageExchange, error) {
	if mexset.log.Enabled(LogLevelDebug) {
		mexset.log.Debugf("Creating new %s message exchange for [%v:%d]", mexset.name, msgType, msgID)
	}

	mex := &messageExchange{
		msgType:   msgType,
		msgID:     msgID,
		ctx:       ctx,
		recvCh:    make(chan *Frame, bufferSize),
		errCh:     newErrNotifier(),
		mexset:    mexset,
		framePool: framePool,
	}

	mexset.Lock()
	addErr := mexset.addExchange(mex)
	mexset.Unlock()

	if addErr != nil {
		logger := mexset.log.WithFields(
			LogField{"msgID", mex.msgID},
			LogField{"msgType", mex.msgType},
			LogField{"exchange", mexset.name},
		)
		if addErr == errMexSetShutdown {
			logger.Warn("Attempted to create new mex after mexset shutdown.")
		} else if addErr == errDuplicateMex {
			logger.Warn("Duplicate msg ID for active and new mex.")
		}

		return nil, addErr
	}

	mexset.onAdded()

	// TODO(mmihic): Put into a deadline ordered heap so we can garbage collected expired exchanges
	return mex, nil
}

// deleteExchange will delete msgID, and return whether it was found or whether it was
// timed out. This method must be called with the lock.
func (mexset *messageExchangeSet) deleteExchange(msgID uint32) (found, timedOut bool) {
	if _, found := mexset.exchanges[msgID]; found {
		delete(mexset.exchanges, msgID)
		return true, false
	}

	if _, expired := mexset.expiredExchanges[msgID]; expired {
		delete(mexset.expiredExchanges, msgID)
		return false, true
	}

	return false, false
}

// removeExchange removes a message exchange from the set, if it exists.
func (mexset *messageExchangeSet) removeExchange(msgID uint32) {
	if mexset.log.Enabled(LogLevelDebug) {
		mexset.log.Debugf("Removing %s message exchange %d", mexset.name, msgID)
	}

	mexset.Lock()
	found, expired := mexset.deleteExchange(msgID)
	mexset.Unlock()

	if !found && !expired {
		mexset.log.WithFields(
			LogField{"msgID", msgID},
		).Error("Tried to remove exchange multiple times")
		return
	}

	// If the message exchange was found, then we perform clean up actions.
	// These clean up actions can only be run once per exchange.
	mexset.onRemoved()
}

// expireExchange is similar to removeExchange, but it marks the exchange as
// expired.
func (mexset *messageExchangeSet) expireExchange(msgID uint32) {
	mexset.log.Debugf(
		"Removing %s message exchange %d due to timeout, cancellation or blackhole",
		mexset.name,
		msgID,
	)

	mexset.Lock()
	// TODO(aniketp): explore if cancel can be called everytime we expire an exchange
	found, expired := mexset.deleteExchange(msgID)
	if found || expired {
		// Record in expiredExchanges if we deleted the exchange.
		mexset.expiredExchanges[msgID] = struct{}{}
	}
	mexset.Unlock()

	if expired {
		mexset.log.WithFields(LogField{"msgID", msgID}).Info("Exchange expired already")
	}

	mexset.onRemoved()
}

func (mexset *messageExchangeSet) count() int {
	mexset.RLock()
	count := len(mexset.exchanges)
	mexset.RUnlock()

	return count
}

// forwardPeerFrame forwards a frame from the peer to the appropriate message
// exchange
func (mexset *messageExchangeSet) forwardPeerFrame(frame *Frame) error {
	if mexset.log.Enabled(LogLevelDebug) {
		mexset.log.Debugf("forwarding %s %s", mexset.name, frame.Header)
	}

	mexset.RLock()
	mex := mexset.exchanges[frame.Header.ID]
	mexset.RUnlock()

	if mex == nil {
		// This is ok since the exchange might have expired or been cancelled
		mexset.log.WithFields(
			LogField{"frameHeader", frame.Header.String()},
			LogField{"exchange", mexset.name},
		).Info("Received frame for unknown message exchange.")
		return nil
	}

	if err := mex.forwardPeerFrame(frame); err != nil {
		mexset.log.WithFields(
			LogField{"frameHeader", frame.Header.String()},
			LogField{"frameSize", frame.Header.FrameSize()},
			LogField{"exchange", mexset.name},
			ErrField(err),
		).Info("Failed to forward frame.")
		return err
	}

	return nil
}

// copyExchanges returns a copy of the exchanges if the exchange is active.
// The caller must lock the mexset.
func (mexset *messageExchangeSet) copyExchanges() (shutdown bool, exchanges map[uint32]*messageExchange) {
	if mexset.shutdown {
		return true, nil
	}

	exchangesCopy := make(map[uint32]*messageExchange, len(mexset.exchanges))
	for k, mex := range mexset.exchanges {
		exchangesCopy[k] = mex
	}

	return false, exchangesCopy
}

// stopExchanges stops all message exchanges to unblock all waiters on the mex.
// This should only be called on connection failures.
func (mexset *messageExchangeSet) stopExchanges(err error) {
	if mexset.log.Enabled(LogLevelDebug) {
		mexset.log.Debugf("stopping %v exchanges due to error: %v", mexset.count(), err)
	}

	mexset.Lock()
	shutdown, exchanges := mexset.copyExchanges()
	mexset.shutdown = true
	mexset.Unlock()

	if shutdown {
		mexset.log.Debugf("mexset has already been shutdown")
		return
	}

	for _, mex := range exchanges {
		// When there's a connection failure, we want to notify blocked callers that the
		// call will fail, but we don't want to shutdown the exchange as only the
		// arg reader/writer should shutdown the exchange. Otherwise, our guarantee
		// on sendChRefs that there's no references to sendCh is violated since
		// readers/writers could still have a reference to sendCh even though
		// we shutdown the exchange and called Done on sendChRefs.
		if mex.errChNotified.CAS(false, true) {
			mex.errCh.Notify(err)
		}
	}
}
