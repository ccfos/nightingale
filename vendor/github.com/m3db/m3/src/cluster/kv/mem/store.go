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

package mem

import (
	"errors"
	"sync"

	"github.com/m3db/m3/src/cluster/kv"

	"github.com/golang/protobuf/proto"
)

// NewStore returns a new in-process store that can be used for testing
func NewStore() kv.TxnStore {
	return &store{
		values:     make(map[string][]*value),
		watchables: make(map[string]kv.ValueWatchable),
	}
}

// NewValue returns a new fake Value around the given proto
func NewValue(vers int, msg proto.Message) kv.Value {
	data, _ := proto.Marshal(msg)
	return &value{
		version: vers,
		data:    data,
	}
}

// NewValueWithData returns a new fake Value around the given data
func NewValueWithData(vers int, data []byte) kv.Value {
	return &value{
		version: vers,
		data:    data,
	}
}

type value struct {
	version  int
	revision int
	data     []byte
}

func (v value) Version() int                      { return v.version }
func (v value) Unmarshal(msg proto.Message) error { return proto.Unmarshal(v.data, msg) }
func (v value) IsNewer(other kv.Value) bool {
	otherValue, ok := other.(*value)
	if !ok {
		return v.version > other.Version()
	}
	if v.revision == otherValue.revision {
		return v.version > other.Version()
	}
	return v.revision > otherValue.revision
}

type store struct {
	sync.RWMutex

	revision   int
	values     map[string][]*value
	watchables map[string]kv.ValueWatchable
}

func (s *store) Get(key string) (kv.Value, error) {
	s.RLock()
	defer s.RUnlock()

	return s.getWithLock(key)
}

func (s *store) getWithLock(key string) (kv.Value, error) {
	val, ok := s.values[key]
	if !ok {
		return nil, kv.ErrNotFound
	}

	if len(val) == 0 {
		return nil, kv.ErrNotFound
	}

	return val[len(val)-1], nil
}

func (s *store) Watch(key string) (kv.ValueWatch, error) {
	s.Lock()
	val := s.values[key]

	watchable, ok := s.watchables[key]
	if !ok {
		watchable = kv.NewValueWatchable()
		s.watchables[key] = watchable
	}
	s.Unlock()

	if !ok && len(val) != 0 {
		watchable.Update(val[len(val)-1])
	}

	_, watch, _ := watchable.Watch()
	return watch, nil
}

func (s *store) Set(key string, val proto.Message) (int, error) {
	s.Lock()
	defer s.Unlock()

	return s.setWithLock(key, val)
}

func (s *store) setWithLock(key string, val proto.Message) (int, error) {
	data, err := proto.Marshal(val)
	if err != nil {
		return 0, err
	}

	lastVersion := 0
	vals := s.values[key]

	if len(vals) != 0 {
		lastVersion = vals[len(vals)-1].version
	}
	newVersion := lastVersion + 1
	s.updateInternalWithLock(key, newVersion, data)
	return newVersion, nil
}

func (s *store) SetIfNotExists(key string, val proto.Message) (int, error) {
	data, err := proto.Marshal(val)
	if err != nil {
		return 0, err
	}

	s.Lock()
	defer s.Unlock()

	if _, exists := s.values[key]; exists {
		return 0, kv.ErrAlreadyExists
	}

	s.updateInternalWithLock(key, 1, data)
	return 1, nil
}

func (s *store) CheckAndSet(key string, version int, val proto.Message) (int, error) {
	data, err := proto.Marshal(val)
	if err != nil {
		return 0, err
	}

	s.Lock()
	defer s.Unlock()

	lastVersion := 0
	vals, exists := s.values[key]
	if exists && len(vals) != 0 {
		lastVersion = vals[len(vals)-1].version
	}

	if version != lastVersion {
		return 0, kv.ErrVersionMismatch
	}

	newVersion := version + 1
	s.updateInternalWithLock(key, newVersion, data)
	return newVersion, nil
}

func (s *store) updateInternalWithLock(key string, newVersion int, data []byte) {
	s.revision++
	fv := &value{
		version:  newVersion,
		revision: s.revision,
		data:     data,
	}
	s.values[key] = append(s.values[key], fv)
	s.updateWatchable(key, fv)
}

func (s *store) Delete(key string) (kv.Value, error) {
	s.Lock()
	defer s.Unlock()

	val, ok := s.values[key]
	if !ok {
		return nil, kv.ErrNotFound
	}

	prev := val[len(val)-1]
	s.updateWatchable(key, nil)
	delete(s.values, key)
	return prev, nil
}

func (s *store) History(key string, from, to int) ([]kv.Value, error) {
	if from <= 0 || to <= 0 || from > to {
		return nil, errors.New("bad request")
	}

	if from == to {
		return nil, nil
	}

	s.RLock()
	defer s.RUnlock()

	vals, ok := s.values[key]
	if !ok {
		return nil, kv.ErrNotFound
	}

	l := len(vals)
	if l == 0 {
		return nil, kv.ErrNotFound
	}

	var res []kv.Value
	for i := from; i < to; i++ {
		idx := i - 1
		if idx >= 0 && idx < l {
			res = append(res, vals[idx])
		}
	}

	return res, nil
}

// NB(cw) When there is an error in one of the ops, the finished ops will not be rolled back
func (s *store) Commit(conditions []kv.Condition, ops []kv.Op) (kv.Response, error) {
	s.Lock()
	defer s.Unlock()

	for _, condition := range conditions {
		if condition.CompareType() != kv.CompareEqual || condition.TargetType() != kv.TargetVersion {
			return nil, errors.New("invalid condition")
		}

		v, err := s.getWithLock(condition.Key())
		expectedVersion := condition.Value().(int)
		if err != nil {
			if err == kv.ErrNotFound && expectedVersion == 0 {
				continue
			}
			return nil, err
		}

		if expectedVersion != v.Version() {
			return nil, kv.ErrConditionCheckFailed
		}
	}

	oprs := make([]kv.OpResponse, len(ops))
	for i, op := range ops {
		if op.Type() != kv.OpSet {
			return nil, errors.New("invalid op")
		}
		opSet := op.(kv.SetOp)

		v, err := s.setWithLock(opSet.Key(), opSet.Value)
		if err != nil {
			return nil, err
		}

		oprs[i] = kv.NewOpResponse(op).SetValue(v)
	}

	return kv.NewResponse().SetResponses(oprs), nil
}

// updateWatchable updates all subscriptions for the given key. It assumes
// the fakeStore write lock is acquired outside of this call
func (s *store) updateWatchable(key string, newVal kv.Value) {
	if watchable, ok := s.watchables[key]; ok {
		watchable.Update(newVal)
	}
}
