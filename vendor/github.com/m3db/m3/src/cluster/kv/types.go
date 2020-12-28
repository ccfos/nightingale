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

package kv

import (
	"errors"

	"github.com/golang/protobuf/proto"
)

const (
	// UninitializedVersion is the version of an uninitialized kv value.
	UninitializedVersion = 0
)

var (
	// ErrVersionMismatch is returned when attempting a CheckAndSet and the
	// key is not at the provided version
	ErrVersionMismatch = errors.New("key is not at the specified version")

	// ErrAlreadyExists is returned when attempting a SetIfEmpty and the key
	// already has a value
	ErrAlreadyExists = errors.New("key already has a value")

	// ErrNotFound is returned when attempting a Get but no value is found for
	// the given key
	ErrNotFound = errors.New("key not found")

	// ErrUnknownTargetType is returned when an unknown TargetType is requested
	ErrUnknownTargetType = errors.New("unknown target type")

	// ErrUnknownCompareType is returned when an unknown CompareType is requested
	ErrUnknownCompareType = errors.New("unknown compare type")

	// ErrUnknownOpType is returned when an unknown OpType is requested
	ErrUnknownOpType = errors.New("unknown op type")

	// ErrConditionCheckFailed is returned when condition check failed
	ErrConditionCheckFailed = errors.New("condition check failed")
)

// A Value provides access to a versioned value in the configuration store
type Value interface {
	// Unmarshal retrieves the stored value
	Unmarshal(v proto.Message) error

	// Version returns the current version of the value
	Version() int

	// IsNewer returns if this Value is newer than the other Value
	IsNewer(other Value) bool
}

// ValueWatch provides updates to a Value
type ValueWatch interface {
	// C returns the notification channel
	C() <-chan struct{}
	// Get returns the latest version of the value
	Get() Value
	// Close stops watching for value updates
	Close()
}

// ValueWatchable can be watched for Value changes
type ValueWatchable interface {
	// Get returns the latest Value
	Get() Value
	// Watch returns the Value and a ValueWatch that will be notified on updates
	Watch() (Value, ValueWatch, error)
	// NumWatches returns the number of watches on the Watchable
	NumWatches() int
	// Update sets the Value and notify Watches
	Update(Value) error
	// IsClosed returns true if the Watchable is closed
	IsClosed() bool
	// Close stops watching for value updates
	Close()
}

// OverrideOptions provides a set of options to override the default configurations of a KV store.
type OverrideOptions interface {
	// Zone returns the zone of the KV store.
	Zone() string

	// SetZone sets the zone of the KV store.
	SetZone(value string) OverrideOptions

	// Namespace returns the namespace of the KV store.
	Namespace() string

	// SetNamespace sets the namespace of the KV store.
	SetNamespace(namespace string) OverrideOptions

	// Environment returns the environment of the KV store.
	Environment() string

	// SetEnvironment sets the environment of the KV store.
	SetEnvironment(env string) OverrideOptions

	// Validate validates the Options.
	Validate() error
}

// Store provides access to the configuration store
type Store interface {
	// Get retrieves the value for the given key
	Get(key string) (Value, error)

	// Watch adds a watch for value updates for given key. This is a non-blocking
	// call - a notification will be sent to ValueWatch.C() once a value is
	// available
	Watch(key string) (ValueWatch, error)

	// Set stores the value for the given key
	Set(key string, v proto.Message) (int, error)

	// SetIfNotExists sets the value for the given key only if no value already
	// exists
	SetIfNotExists(key string, v proto.Message) (int, error)

	// CheckAndSet stores the value for the given key if the current version
	// matches the provided version
	CheckAndSet(key string, version int, v proto.Message) (int, error)

	// Delete deletes a key in the store and returns the last value before deletion
	Delete(key string) (Value, error)

	// History returns the value for a key in version range [from, to)
	History(key string, from, to int) ([]Value, error)
}

// TargetType is the type of the comparison target in the condition
type TargetType int

// list of supported TargetTypes
const (
	TargetVersion TargetType = iota
)

// CompareType is the type of the comparison in the condition
type CompareType string

func (t CompareType) String() string {
	return string(t)
}

// list of supported CompareType
const (
	CompareEqual CompareType = "="
)

// Condition defines the prerequisite for a transaction
type Condition interface {
	// TargetType returns the type of the TargetType
	TargetType() TargetType
	// SetTargetType sets the type of the TargetType
	SetTargetType(t TargetType) Condition

	// CompareType returns the type of the CompareType
	CompareType() CompareType
	// SetCompareType sets the type of the CompareType
	SetCompareType(t CompareType) Condition

	// Key returns the key in the condition
	Key() string
	// SetKey sets the key in the condition
	SetKey(key string) Condition

	// Value returns the value for comparison
	Value() interface{}
	// SetValue sets the value for comparison
	SetValue(value interface{}) Condition
}

// OpType is the type of the operation
type OpType int

// list of supported OpTypes
const (
	OpSet OpType = iota
)

// Op is the operation to be performed in a transaction
type Op interface {
	// Type returns the type of the operation
	Type() OpType
	// SetType sets the type of the operation
	SetType(ot OpType) Op

	// Key returns the key used in the operation
	Key() string
	// SetKey sets the key in the operation
	SetKey(key string) Op
}

// OpResponse is the response of a transaction operation
type OpResponse interface {
	Op

	Value() interface{}
	SetValue(v interface{}) OpResponse
}

// Response captures the response of the transaction
type Response interface {
	Responses() []OpResponse
	SetResponses(oprs []OpResponse) Response
}

// TxnStore supports transactions on top of Store interface
type TxnStore interface {
	Store

	Commit([]Condition, []Op) (Response, error)
}
