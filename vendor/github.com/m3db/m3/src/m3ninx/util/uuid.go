// Copyright (c) 2018 Uber Technologies, Inc.
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

package util

import (
	"encoding/base64"
	"errors"

	"github.com/satori/go.uuid"
)

var errUUIDForbidden = errors.New("generating UUIDs is forbidden")

var encodedLen = base64.StdEncoding.EncodedLen(uuid.Size)

// NewUUIDFn is a function for creating new UUIDs.
type NewUUIDFn func() ([]byte, error)

// NewUUID returns a new UUID.
func NewUUID() ([]byte, error) {
	// TODO: V4 UUIDs are randomly generated. It would be more efficient to instead
	// use time-based UUIDs so the prefixes of the UUIDs are similar. V1 UUIDs use
	// the current timestamp and the server's MAC address but the latter isn't
	// guaranteed to be unique since we may have multiple processes running on the
	// same host. Elasticsearch uses Flake IDs which ensure uniqueness by requiring
	// an initial coordination step and we may want to consider doing the same.
	uuid := uuid.NewV4().Bytes()

	buf := make([]byte, encodedLen)
	base64.StdEncoding.Encode(buf, uuid)
	return buf, nil
}

// NewUUIDForbidden is NewUUIDFn which always returns an error in the case that
// UUIDs are forbidden.
func NewUUIDForbidden() ([]byte, error) {
	return nil, errUUIDForbidden
}
