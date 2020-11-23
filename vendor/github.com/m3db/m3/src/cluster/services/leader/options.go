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

package leader

import (
	"errors"

	"github.com/m3db/m3/src/cluster/services"
)

var (
	errMissingSid   = errors.New("leader options must specify service ID")
	errMissingEOpts = errors.New("leader options election opts cannot be nil")
)

// Options describe options for creating a leader service.
type Options interface {
	// Service the election is campaigning for.
	ServiceID() services.ServiceID
	SetServiceID(sid services.ServiceID) Options

	ElectionOpts() services.ElectionOptions
	SetElectionOpts(e services.ElectionOptions) Options

	Validate() error
}

// NewOptions returns an instance of leader options.
func NewOptions() Options {
	return options{
		eo: services.NewElectionOptions(),
	}
}

type options struct {
	sid services.ServiceID
	eo  services.ElectionOptions
}

func (o options) ServiceID() services.ServiceID {
	return o.sid
}

func (o options) SetServiceID(sid services.ServiceID) Options {
	o.sid = sid
	return o
}

func (o options) ElectionOpts() services.ElectionOptions {
	return o.eo
}

func (o options) SetElectionOpts(eo services.ElectionOptions) Options {
	o.eo = eo
	return o
}

func (o options) Validate() error {
	if o.sid == nil {
		return errMissingSid
	}

	// This shouldn't happen since we have sane defaults but prevents user error.
	if o.eo == nil {
		return errMissingEOpts
	}

	return nil
}
