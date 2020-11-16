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

// Adapted from: https://raw.githubusercontent.com/blevesearch/bleve/master/index/scorch/segment/regexp.go

package regexp

import (
	"regexp/syntax"

	vregexp "github.com/m3dbx/vellum/regexp"
)

// ParseRegexp parses the provided regexp pattern into an equivalent matching automaton, and
// corresponding keys to bound prefix beginning and end during the FST search.
func ParseRegexp(pattern string) (a *vregexp.Regexp, prefixBeg, prefixEnd []byte, err error) {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, nil, nil, err
	}
	return ParsedRegexp(pattern, parsed)
}

// ParsedRegexp uses the pre-parsed regexp pattern and creates an equivalent matching automaton, and
// corresponding keys to bound prefix beginning and end during the FST search.
func ParsedRegexp(pattern string, parsed *syntax.Regexp) (a *vregexp.Regexp, prefixBeg, prefixEnd []byte, err error) {
	re, err := vregexp.NewParsedWithLimit(pattern, parsed, vregexp.DefaultLimit)
	if err != nil {
		return nil, nil, nil, err
	}

	prefix := LiteralPrefix(parsed)
	if prefix != "" {
		prefixBeg := []byte(prefix)
		prefixEnd := IncrementBytes(prefixBeg)
		return re, prefixBeg, prefixEnd, nil
	}

	return re, nil, nil, nil
}

// LiteralPrefix returns the literal prefix given the parse tree for a regexp
func LiteralPrefix(s *syntax.Regexp) string {
	// traverse the left-most branch in the parse tree as long as the
	// node represents a concatenation
	for s != nil && s.Op == syntax.OpConcat {
		if len(s.Sub) < 1 {
			return ""
		}

		s = s.Sub[0]
	}

	if s.Op == syntax.OpLiteral {
		return string(s.Rune)
	}

	return "" // no literal prefix
}

// IncrementBytes increments the provided bytes to the next word boundary.
func IncrementBytes(in []byte) []byte {
	rv := make([]byte, len(in))
	copy(rv, in)
	for i := len(rv) - 1; i >= 0; i-- {
		rv[i] = rv[i] + 1
		if rv[i] != 0 {
			return rv // didn't overflow, so stop
		}
	}
	return nil // overflowed
}
