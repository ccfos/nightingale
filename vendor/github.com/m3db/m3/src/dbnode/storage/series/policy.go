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

package series

import (
	"errors"
	"fmt"
)

var (
	errCachePolicyUnspecified = errors.New("series cache policy unspecified")
)

// CachePolicy is the series cache policy.
type CachePolicy uint

const (
	// CacheNone specifies that no series will be cached by default.
	CacheNone CachePolicy = iota
	// CacheAll specifies that all series must be cached at all times
	// which requires loading all into cache on bootstrap and never
	// expiring series from memory until expired from retention.
	CacheAll
	// CacheRecentlyRead specifies that series that are recently read
	// must be cached, configurable by the namespace block expiry after
	// not accessed period.
	CacheRecentlyRead
	// CacheLRU specifies that series that are read will be cached
	// using an LRU of fixed capacity. Series that are least recently
	// used will be evicted first.
	CacheLRU

	// DefaultCachePolicy is the default cache policy.
	DefaultCachePolicy = CacheLRU
)

// ValidCachePolicies returns the valid series cache policies.
func ValidCachePolicies() []CachePolicy {
	return []CachePolicy{CacheNone, CacheAll, CacheRecentlyRead, CacheLRU}
}

func (p CachePolicy) String() string {
	switch p {
	case CacheNone:
		return "none"
	case CacheAll:
		return "all"
	case CacheRecentlyRead:
		return "recently_read"
	case CacheLRU:
		return "lru"
	}
	return "unknown"
}

// ValidateCachePolicy validates a cache policy.
func ValidateCachePolicy(v CachePolicy) error {
	validSeriesCachePolicy := false
	for _, valid := range ValidCachePolicies() {
		if valid == v {
			validSeriesCachePolicy = true
			break
		}
	}
	if !validSeriesCachePolicy {
		return fmt.Errorf("invalid series CachePolicy '%d' valid types are: %v",
			uint(v), ValidCachePolicies())
	}
	return nil
}

// ParseCachePolicy parses a CachePolicy from a string.
func ParseCachePolicy(str string) (CachePolicy, error) {
	var r CachePolicy
	if str == "" {
		return r, errCachePolicyUnspecified
	}
	for _, valid := range ValidCachePolicies() {
		if str == valid.String() {
			r = valid
			return r, nil
		}
	}
	return r, fmt.Errorf("invalid series CachePolicy '%s' valid types are: %v",
		str, ValidCachePolicies())
}

// UnmarshalYAML unmarshals an CachePolicy into a valid type from string.
func (p *CachePolicy) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	r, err := ParseCachePolicy(str)
	if err != nil {
		return err
	}
	*p = r
	return nil
}
