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
	"fmt"
	"sync"

	"github.com/m3db/m3/src/cluster/services"
	"github.com/m3db/m3/src/cluster/services/leader/campaign"

	"go.etcd.io/etcd/clientv3"
)

const (
	leaderKeyPrefix = "_ld"
	keyFormat       = "%s/%s"
)

var (
	// errClientClosed indicates the election service client has been closed and
	// no more elections can be started.
	errClientClosed = errors.New("election client is closed")
)

type multiClient struct {
	sync.RWMutex

	closed     bool
	clients    map[string]*client
	opts       Options
	etcdClient *clientv3.Client
}

// NewService creates a new leader service client based on an etcd client.
func NewService(cli *clientv3.Client, opts Options) (services.LeaderService, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	return &multiClient{
		clients:    make(map[string]*client),
		opts:       opts,
		etcdClient: cli,
	}, nil
}

// Close closes all underlying election clients and returns all errors
// encountered, if any.
func (s *multiClient) Close() error {
	s.Lock()
	if s.closed {
		s.Unlock()
		return nil
	}

	s.closed = true
	s.Unlock()

	return s.closeClients()
}

func (s *multiClient) isClosed() bool {
	s.RLock()
	defer s.RUnlock()
	return s.closed
}

func (s *multiClient) closeClients() error {
	s.RLock()
	errC := make(chan error, 1)
	var wg sync.WaitGroup

	for _, cl := range s.clients {
		wg.Add(1)

		go func(cl *client) {
			if err := cl.close(); err != nil {
				select {
				case errC <- err:
				default:
				}
			}
			wg.Done()
		}(cl)
	}

	s.RUnlock()

	wg.Wait()
	close(errC)

	select {
	case err := <-errC:
		return err
	default:
		return nil
	}
}

func (s *multiClient) getOrCreateClient(electionID string) (*client, error) {
	s.RLock()
	cl, ok := s.clients[electionID]
	s.RUnlock()
	if ok {
		return cl, nil
	}

	clientNew, err := newClient(s.etcdClient, s.opts, electionID)
	if err != nil {
		return nil, err
	}

	s.Lock()
	defer s.Unlock()

	cl, ok = s.clients[electionID]
	if ok {
		// another client was created between RLock and now, close new one
		go clientNew.close()

		return cl, nil
	}

	s.clients[electionID] = clientNew
	return clientNew, nil
}

func (s *multiClient) Campaign(electionID string, opts services.CampaignOptions) (<-chan campaign.Status, error) {
	if opts == nil {
		return nil, errors.New("cannot pass nil campaign options")
	}

	if s.isClosed() {
		return nil, errClientClosed
	}

	client, err := s.getOrCreateClient(electionID)
	if err != nil {
		return nil, err
	}

	return client.campaign(opts)
}

func (s *multiClient) Resign(electionID string) error {
	if s.isClosed() {
		return errClientClosed
	}

	s.RLock()
	cl, ok := s.clients[electionID]
	s.RUnlock()

	if !ok {
		return fmt.Errorf("no election with ID '%s' to resign", electionID)
	}

	return cl.resign()
}

func (s *multiClient) Leader(electionID string) (string, error) {
	if s.isClosed() {
		return "", errClientClosed
	}

	// always create a client so we can check election statuses without
	// campaigning
	client, err := s.getOrCreateClient(electionID)
	if err != nil {
		return "", err
	}

	return client.leader()
}

func (s *multiClient) Observe(electionID string) (<-chan string, error) {
	if s.isClosed() {
		return nil, errClientClosed
	}

	cl, err := s.getOrCreateClient(electionID)
	if err != nil {
		return nil, err
	}

	return cl.observe()
}
