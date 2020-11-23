// Copyright (c) 2016 Uber Technologies, Inc.
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

package client

import (
	"sync"
)

type client struct {
	sync.Mutex

	opts         Options
	asyncOpts    []Options
	newSessionFn newReplicatedSessionFn
	session      AdminSession // default cached session
}

// type newReplicatedSessionFn func(Options) (replicatedSession, error)
type newReplicatedSessionFn func(Options, []Options, ...replicatedSessionOption) (clientSession, error)

// NewClient creates a new client
func NewClient(opts Options, asyncOpts ...Options) (Client, error) {
	return newClient(opts, asyncOpts...)
}

// NewAdminClient creates a new administrative client
func NewAdminClient(opts AdminOptions, asyncOpts ...Options) (AdminClient, error) {
	return newClient(opts, asyncOpts...)
}

func newClient(opts Options, asyncOpts ...Options) (*client, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	return &client{opts: opts, asyncOpts: asyncOpts, newSessionFn: newReplicatedSession}, nil
}

func (c *client) newSession() (AdminSession, error) {
	session, err := c.newSessionFn(c.opts, c.asyncOpts)
	if err != nil {
		return nil, err
	}
	if err := session.Open(); err != nil {
		return nil, err
	}
	return session, nil
}

func (c *client) defaultSession() (AdminSession, error) {
	c.Lock()
	if c.session != nil {
		session := c.session
		c.Unlock()
		return session, nil
	}
	c.Unlock()

	session, err := c.newSession()
	if err != nil {
		return nil, err
	}

	c.Lock()
	if c.session != nil {
		session := c.session
		c.Unlock()
		return session, nil
	}
	c.session = session
	c.Unlock()

	return session, nil
}

func (c *client) Options() Options {
	return c.opts
}

func (c *client) NewSession() (Session, error) {
	return c.newSession()
}

func (c *client) DefaultSession() (Session, error) {
	return c.defaultSession()
}

func (c *client) NewAdminSession() (AdminSession, error) {
	return c.newSession()
}

func (c *client) DefaultAdminSession() (AdminSession, error) {
	return c.defaultSession()
}

func (c *client) DefaultSessionActive() bool {
	c.Lock()
	defer c.Unlock()
	return c.session != nil
}
