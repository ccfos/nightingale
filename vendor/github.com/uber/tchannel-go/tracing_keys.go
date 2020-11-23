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
	"strings"
	"sync"
)

// tracingKeyPrefix is used to prefix all keys used by the OpenTracing Tracer to represent
// its trace context and baggage. The prefixing is done in order to distinguish tracing
// headers from the actual application headers and to hide the former from the user code.
const tracingKeyPrefix = "$tracing$"

// tracingKeyMappingSize is the maximum number of tracing key mappings we cache.
const tracingKeyMappingSize = 100

type tracingKeysMapping struct {
	sync.RWMutex
	mapping map[string]string
	mapper  func(key string) string
}

var tracingKeyEncoding = &tracingKeysMapping{
	mapping: make(map[string]string),
	mapper: func(key string) string {
		return tracingKeyPrefix + key
	},
}

var tracingKeyDecoding = &tracingKeysMapping{
	mapping: make(map[string]string),
	mapper: func(key string) string {
		return key[len(tracingKeyPrefix):]
	},
}

func (m *tracingKeysMapping) mapAndCache(key string) string {
	m.RLock()
	v, ok := m.mapping[key]
	m.RUnlock()
	if ok {
		return v
	}
	m.Lock()
	defer m.Unlock()
	if v, ok := m.mapping[key]; ok {
		return v
	}
	mappedKey := m.mapper(key)
	if len(m.mapping) < tracingKeyMappingSize {
		m.mapping[key] = mappedKey
	}
	return mappedKey
}

type tracingHeadersCarrier map[string]string

// Set implements Set() of opentracing.TextMapWriter
func (c tracingHeadersCarrier) Set(key, val string) {
	prefixedKey := tracingKeyEncoding.mapAndCache(key)
	c[prefixedKey] = val
}

// ForeachKey conforms to the TextMapReader interface.
func (c tracingHeadersCarrier) ForeachKey(handler func(key, val string) error) error {
	for k, v := range c {
		if !strings.HasPrefix(k, tracingKeyPrefix) {
			continue
		}
		noPrefixKey := tracingKeyDecoding.mapAndCache(k)
		if err := handler(noPrefixKey, v); err != nil {
			return err
		}
	}
	return nil
}

func (c tracingHeadersCarrier) RemoveTracingKeys() {
	for key := range c {
		if strings.HasPrefix(key, tracingKeyPrefix) {
			delete(c, key)
		}
	}
}
