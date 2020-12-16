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

package storage

import (
	"github.com/m3db/m3/src/cluster/kv"
	"github.com/m3db/m3/src/cluster/placement"

	"github.com/golang/protobuf/proto"
	"go.uber.org/zap"
)

const errorVersionValue = 0

type storage struct {
	helper helper
	key    string
	store  kv.Store
	opts   placement.Options
	logger *zap.Logger
}

// NewPlacementStorage creates a placement.Storage.
func NewPlacementStorage(store kv.Store, key string, opts placement.Options) placement.Storage {
	if opts == nil {
		opts = placement.NewOptions()
	}
	return &storage{
		key:    key,
		store:  store,
		helper: newHelper(store, key, opts),
		opts:   opts,
		logger: opts.InstrumentOptions().Logger(),
	}
}

func (s *storage) CheckAndSetProto(p proto.Message, version int) (int, error) {
	if err := s.helper.ValidateProto(p); err != nil {
		return errorVersionValue, err
	}

	if s.opts.Dryrun() {
		s.logger.Info("this is a dryrun, the operation is not persisted")
		return version + 1, nil
	}

	return s.store.CheckAndSet(s.key, version, p)
}

func (s *storage) SetProto(p proto.Message) (int, error) {
	if err := s.helper.ValidateProto(p); err != nil {
		return errorVersionValue, err
	}

	if s.opts.Dryrun() {
		s.logger.Info("this is a dryrun, the operation is not persisted")
		return errorVersionValue, nil
	}
	return s.store.Set(s.key, p)
}

func (s *storage) Proto() (proto.Message, int, error) {
	return s.helper.PlacementProto()
}

func (s *storage) Set(p placement.Placement) (placement.Placement, error) {
	if err := placement.Validate(p); err != nil {
		return nil, err
	}

	placementProto, err := s.helper.GenerateProto(p)
	if err != nil {
		return nil, err
	}

	if s.opts.Dryrun() {
		s.logger.Info("this is a dryrun, the operation is not persisted")
		return p, nil
	}

	v, err := s.store.Set(s.key, placementProto)
	if err != nil {
		return nil, err
	}

	return p.Clone().SetVersion(v), nil
}

func (s *storage) CheckAndSet(p placement.Placement, version int) (placement.Placement, error) {
	if err := placement.Validate(p); err != nil {
		return nil, err
	}

	placementProto, err := s.helper.GenerateProto(p)
	if err != nil {
		return nil, err
	}

	if s.opts.Dryrun() {
		s.logger.Info("this is a dryrun, the operation is not persisted")
		return p, nil
	}

	v, err := s.store.CheckAndSet(
		s.key,
		version,
		placementProto,
	)

	if err != nil {
		return nil, err
	}

	return p.Clone().SetVersion(v), nil
}

func (s *storage) SetIfNotExist(p placement.Placement) (placement.Placement, error) {
	if err := placement.Validate(p); err != nil {
		return nil, err
	}

	placementProto, err := s.helper.GenerateProto(p)
	if err != nil {
		return nil, err
	}

	if s.opts.Dryrun() {
		s.logger.Info("this is a dryrun, the operation is not persisted")
		return p, nil
	}

	v, err := s.store.SetIfNotExists(
		s.key,
		placementProto,
	)

	if err != nil {
		return nil, err
	}

	return p.Clone().SetVersion(v), nil
}

func (s *storage) Delete() error {
	if s.opts.Dryrun() {
		s.logger.Info("this is a dryrun, the operation is not persisted")
		return nil
	}

	_, err := s.store.Delete(s.key)
	return err
}

func (s *storage) Placement() (placement.Placement, error) {
	p, _, err := s.helper.Placement()
	return p, err
}

func (s *storage) Watch() (placement.Watch, error) {
	w, err := s.store.Watch(s.key)
	if err != nil {
		return nil, err
	}
	return newPlacementWatch(w, s.opts), nil
}

func (s *storage) PlacementForVersion(version int) (placement.Placement, error) {
	return s.helper.PlacementForVersion(version)
}
