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

package runtime

import (
	"fmt"

	"github.com/m3db/m3/src/cluster/kv"
	"github.com/m3db/m3/src/x/watch"

	"go.uber.org/zap"
)

// Value is a value that can be updated during runtime.
type Value interface {
	watch.Value

	// Key is the key associated with value.
	Key() string
}

// UnmarshalFn unmarshals a kv value and extracts its payload.
type UnmarshalFn func(value kv.Value) (interface{}, error)

// ProcessFn processes a value.
type ProcessFn func(value interface{}) error

type value struct {
	watch.Value

	key         string
	store       kv.Store
	opts        Options
	log         *zap.Logger
	unmarshalFn UnmarshalFn
	processFn   ProcessFn
	updateFn    watch.ProcessFn

	currValue kv.Value
}

// NewValue creates a new value.
func NewValue(
	key string,
	opts Options,
) Value {
	v := &value{
		key:         key,
		store:       opts.KVStore(),
		opts:        opts,
		log:         opts.InstrumentOptions().Logger(),
		unmarshalFn: opts.UnmarshalFn(),
		processFn:   opts.ProcessFn(),
	}
	v.updateFn = v.update
	v.initValue()
	return v
}

func (v *value) initValue() {
	valueOpts := watch.NewOptions().
		SetInstrumentOptions(v.opts.InstrumentOptions()).
		SetInitWatchTimeout(v.opts.InitWatchTimeout()).
		SetNewUpdatableFn(v.newUpdatableFn).
		SetGetUpdateFn(v.getUpdateFn).
		SetProcessFn(v.updateFn).
		SetKey(v.key)
	v.Value = watch.NewValue(valueOpts)
}

func (v *value) Key() string { return v.key }

func (v *value) newUpdatableFn() (watch.Updatable, error) {
	return v.store.Watch(v.key)
}

func (v *value) getUpdateFn(updatable watch.Updatable) (interface{}, error) {
	return updatable.(kv.ValueWatch).Get(), nil
}

func (v *value) update(value interface{}) error {
	update := value.(kv.Value)
	if v.currValue != nil && !update.IsNewer(v.currValue) {
		v.log.Warn("ignore kv update with version which is not newer than the version of the current value",
			zap.Int("new version", update.Version()),
			zap.Int("current version", v.currValue.Version()))
		return nil
	}
	v.log.Info("received kv update",
		zap.Int("version", update.Version()),
		zap.String("key", v.key))
	latest, err := v.unmarshalFn(update)
	if err != nil {
		return fmt.Errorf("error unmarshalling value for version %d: %v", update.Version(), err)
	}
	if err := v.processFn(latest); err != nil {
		return err
	}
	v.currValue = update
	return nil
}
