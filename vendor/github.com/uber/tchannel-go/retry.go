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
	"net"
	"sync"
	"time"

	"golang.org/x/net/context"
)

// RetryOn represents the types of errors to retry on.
type RetryOn int

//go:generate stringer -type=RetryOn

const (
	// RetryDefault is currently the same as RetryConnectionError.
	RetryDefault RetryOn = iota

	// RetryConnectionError retries on busy frames, declined frames, and connection errors.
	RetryConnectionError

	// RetryNever never retries any errors.
	RetryNever

	// RetryNonIdempotent will retry errors that occur before a request has been picked up.
	// E.g. busy frames and declined frames.
	// This should be used when making calls to non-idempotent endpoints.
	RetryNonIdempotent

	// RetryUnexpected will retry busy frames, declined frames, and unenxpected frames.
	RetryUnexpected

	// RetryIdempotent will retry all errors that can be retried. This should be used
	// for idempotent endpoints.
	RetryIdempotent
)

// RequestState is a global request state that persists across retries.
type RequestState struct {
	// Start is the time at which the request was initiated by the caller of RunWithRetry.
	Start time.Time
	// SelectedPeers is a set of host:ports that have been selected previously.
	SelectedPeers map[string]struct{}
	// Attempt is 1 for the first attempt, and so on.
	Attempt   int
	retryOpts *RetryOptions
}

// RetriableFunc is the type of function that can be passed to RunWithRetry.
type RetriableFunc func(context.Context, *RequestState) error

func isNetError(err error) bool {
	// TODO(prashantv): Should TChannel internally these to ErrCodeNetwork before returning
	// them to the user?
	_, ok := err.(net.Error)
	return ok
}

func getErrCode(err error) SystemErrCode {
	code := GetSystemErrorCode(err)
	if isNetError(err) {
		code = ErrCodeNetwork
	}
	return code
}

// CanRetry returns whether an error can be retried for the given retry option.
func (r RetryOn) CanRetry(err error) bool {
	if r == RetryNever {
		return false
	}
	if r == RetryDefault {
		r = RetryConnectionError
	}

	code := getErrCode(err)

	if code == ErrCodeBusy || code == ErrCodeDeclined {
		return true
	}
	// Never retry bad requests, since it will probably cause another bad request.
	if code == ErrCodeBadRequest {
		return false
	}

	switch r {
	case RetryConnectionError:
		return code == ErrCodeNetwork
	case RetryUnexpected:
		return code == ErrCodeUnexpected
	case RetryIdempotent:
		return true
	}

	return false
}

// RetryOptions are the retry options used to configure RunWithRetry.
type RetryOptions struct {
	// MaxAttempts is the maximum number of calls and retries that will be made.
	// If this is 0, the default number of attempts (5) is used.
	MaxAttempts int

	// RetryOn is the types of errors to retry on.
	RetryOn RetryOn

	// TimeoutPerAttempt is the per-retry timeout to use.
	// If this is zero, then the original timeout is used.
	TimeoutPerAttempt time.Duration
}

var defaultRetryOptions = &RetryOptions{
	MaxAttempts: 5,
}

var requestStatePool = sync.Pool{
	New: func() interface{} { return &RequestState{} },
}

func getRetryOptions(ctx context.Context) *RetryOptions {
	params := getTChannelParams(ctx)
	if params == nil {
		return defaultRetryOptions
	}

	opts := params.retryOptions
	if opts == nil {
		return defaultRetryOptions
	}

	if opts.MaxAttempts == 0 {
		opts.MaxAttempts = defaultRetryOptions.MaxAttempts
	}
	return opts
}

// HasRetries will return true if there are more retries left.
func (rs *RequestState) HasRetries(err error) bool {
	if rs == nil {
		return false
	}
	rOpts := rs.retryOpts
	return rs.Attempt < rOpts.MaxAttempts && rOpts.RetryOn.CanRetry(err)
}

// SinceStart returns the time since the start of the request. If there is no request state,
// then the fallback is returned.
func (rs *RequestState) SinceStart(now time.Time, fallback time.Duration) time.Duration {
	if rs == nil {
		return fallback
	}
	return now.Sub(rs.Start)
}

// PrevSelectedPeers returns the previously selected peers for this request.
func (rs *RequestState) PrevSelectedPeers() map[string]struct{} {
	if rs == nil {
		return nil
	}
	return rs.SelectedPeers
}

// AddSelectedPeer adds a given peer to the set of selected peers.
func (rs *RequestState) AddSelectedPeer(hostPort string) {
	if rs == nil {
		return
	}

	host := getHost(hostPort)
	if rs.SelectedPeers == nil {
		rs.SelectedPeers = map[string]struct{}{
			hostPort: {},
			host:     {},
		}
	} else {
		rs.SelectedPeers[hostPort] = struct{}{}
		rs.SelectedPeers[host] = struct{}{}
	}
}

// RetryCount returns the retry attempt this is. Essentially, Attempt - 1.
func (rs *RequestState) RetryCount() int {
	if rs == nil {
		return 0
	}
	return rs.Attempt - 1
}

// RunWithRetry will take a function that makes the TChannel call, and will
// rerun it as specifed in the RetryOptions in the Context.
func (ch *Channel) RunWithRetry(runCtx context.Context, f RetriableFunc) error {
	var err error

	opts := getRetryOptions(runCtx)
	rs := ch.getRequestState(opts)
	defer requestStatePool.Put(rs)

	for i := 0; i < opts.MaxAttempts; i++ {
		rs.Attempt++

		if opts.TimeoutPerAttempt == 0 {
			err = f(runCtx, rs)
		} else {
			attemptCtx, cancel := context.WithTimeout(runCtx, opts.TimeoutPerAttempt)
			err = f(attemptCtx, rs)
			cancel()
		}

		if err == nil {
			return nil
		}
		if !opts.RetryOn.CanRetry(err) {
			if ch.log.Enabled(LogLevelInfo) {
				ch.log.WithFields(ErrField(err)).Info("Failed after non-retriable error.")
			}
			return err
		}

		ch.log.WithFields(
			ErrField(err),
			LogField{"attempt", rs.Attempt},
			LogField{"maxAttempts", opts.MaxAttempts},
		).Info("Retrying request after retryable error.")
	}

	// Too many retries, return the last error
	return err
}

func (ch *Channel) getRequestState(retryOpts *RetryOptions) *RequestState {
	rs := requestStatePool.Get().(*RequestState)
	*rs = RequestState{
		Start:     ch.timeNow(),
		retryOpts: retryOpts,
	}
	return rs
}

// getHost returns the host part of a host:port. If no ':' is found, it returns the
// original string. Note: This hand-rolled loop is faster than using strings.IndexByte.
func getHost(hostPort string) string {
	for i := 0; i < len(hostPort); i++ {
		if hostPort[i] == ':' {
			return hostPort[:i]
		}
	}
	return hostPort
}
