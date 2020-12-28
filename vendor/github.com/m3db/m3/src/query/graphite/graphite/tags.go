// Copyright (c) 2019 Uber Technologies, Inc.
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

package graphite

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/unsafe"
)

const (
	// graphiteFormat is the format for graphite metric tag names, which will be
	// represented as tag/value pairs in M3.
	// NB: stats.gauges.donkey.kong.barrels would become the following tag set:
	// {__g0__: stats}
	// {__g1__: gauges}
	// {__g2__: donkey}
	// {__g3__: kong}
	// {__g4__: barrels}
	graphiteFormat = "__g%d__"

	// Number of pre-formatted key names to generate in the init() function.
	numPreFormattedTagNames = 128

	// MatchAllPattern that is used to match all metrics.
	MatchAllPattern = ".*"
)

var (
	// Should never be modified after init().
	preFormattedTagNames [][]byte
	// Should never be modified after init().
	preFormattedTagNameIDs []ident.ID

	// Prefix is the prefix for graphite metrics
	Prefix = []byte("__g")

	// Suffix is the suffix for graphite metrics
	suffix = []byte("__")
)

func init() {
	for i := 0; i < numPreFormattedTagNames; i++ {
		name := generateTagName(i)
		preFormattedTagNames = append(preFormattedTagNames, name)
		preFormattedTagNameIDs = append(preFormattedTagNameIDs, ident.BytesID(name))
	}
}

// TagName gets a preallocated or generate a tag name for the given graphite
// path index.
func TagName(idx int) []byte {
	if idx < len(preFormattedTagNames) {
		return preFormattedTagNames[idx]
	}

	return []byte(fmt.Sprintf(graphiteFormat, idx))
}

// TagNameID gets a preallocated or generate a tag name ID for the given graphite
// path index.
func TagNameID(idx int) ident.ID {
	if idx < len(preFormattedTagNameIDs) {
		return preFormattedTagNameIDs[idx]
	}

	return ident.StringID(fmt.Sprintf(graphiteFormat, idx))
}

func generateTagName(idx int) []byte {
	return []byte(fmt.Sprintf(graphiteFormat, idx))
}

// TagIndex returns the index given the tag.
func TagIndex(tag []byte) (int, bool) {
	if !bytes.HasPrefix(tag, Prefix) ||
		!bytes.HasSuffix(tag, suffix) {
		return 0, false
	}
	start := len(Prefix)
	end := len(tag) - len(suffix)
	indexStr := unsafe.String(tag[start:end])
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return 0, false
	}
	return index, true
}
