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
	"strings"

	"github.com/m3db/m3/src/query/graphite/errors"
)

const (
	// ValidIdentifierRunes contains all the runes allowed in a graphite identifier
	ValidIdentifierRunes = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789" +
		"$-_'|<>%#/:"
)

// GlobToRegexPattern converts a graphite-style glob into a regex pattern, with
// a boolean indicating if the glob is regexed or not
func GlobToRegexPattern(glob string) ([]byte, bool, error) {
	return globToRegexPattern(glob, GlobOptions{})
}

// ExtendedGlobToRegexPattern converts a graphite-style glob into a regex pattern
// with extended options
func ExtendedGlobToRegexPattern(glob string, opts GlobOptions) ([]byte, bool, error) {
	return globToRegexPattern(glob, opts)
}

// GlobOptions allows for matching everything
type GlobOptions struct {
	// AllowMatchAll allows for matching all text
	// including hierarchy separators with "**"
	AllowMatchAll bool `yaml:"allowMatchAll"`
}

type pattern struct {
	buff         bytes.Buffer
	eval         rune
	lastWriteLen int
}

func (p *pattern) String() string {
	return p.buff.String()
}

func (p *pattern) Evaluate(r rune) {
	p.eval = r
}

func (p *pattern) LastEvaluate() rune {
	return p.eval
}

func (p *pattern) WriteRune(r rune) {
	p.buff.WriteRune(r)
	p.lastWriteLen = 1
}

func (p *pattern) WriteString(str string) {
	p.buff.WriteString(str)
	p.lastWriteLen = len(str)
}

func (p *pattern) UnwriteLast() {
	p.buff.Truncate(p.buff.Len() - p.lastWriteLen)
	p.lastWriteLen = 0
}

func globToRegexPattern(glob string, opts GlobOptions) ([]byte, bool, error) {
	var (
		pattern  pattern
		escaping = false
		regexed  = false
	)

	groupStartStack := []rune{rune(0)} // rune(0) indicates pattern is not in a group
	for i, r := range glob {
		prevEval := pattern.LastEvaluate()
		pattern.Evaluate(r)

		if escaping {
			pattern.WriteRune(r)
			escaping = false
			continue
		}

		switch r {
		case '\\':
			escaping = true
			pattern.WriteRune('\\')
		case '.':
			// Match hierarchy separator
			pattern.WriteString("\\.+")
			regexed = true
		case '?':
			// Match anything except the hierarchy separator
			pattern.WriteString("[^\\.]")
			regexed = true
		case '*':
			if opts.AllowMatchAll && prevEval == '*' {
				pattern.UnwriteLast()
				pattern.WriteString(".*")
				regexed = true
			} else {
				// Match everything up to the next hierarchy separator
				pattern.WriteString("[^\\.]*")
				regexed = true
			}
		case '{':
			// Begin non-capturing group
			pattern.WriteString("(")
			groupStartStack = append(groupStartStack, r)
			regexed = true
		case '}':
			// End non-capturing group
			priorGroupStart := groupStartStack[len(groupStartStack)-1]
			if priorGroupStart != '{' {
				return nil, false, errors.NewInvalidParamsError(fmt.Errorf("invalid '}' at %d, no prior for '{' in %s", i, glob))
			}

			pattern.WriteRune(')')
			groupStartStack = groupStartStack[:len(groupStartStack)-1]
		case '[':
			// Begin character range
			pattern.WriteRune('[')
			groupStartStack = append(groupStartStack, r)
			regexed = true
		case ']':
			// End character range
			priorGroupStart := groupStartStack[len(groupStartStack)-1]
			if priorGroupStart != '[' {
				return nil, false, errors.NewInvalidParamsError(fmt.Errorf("invalid ']' at %d, no prior for '[' in %s", i, glob))
			}

			pattern.WriteRune(']')
			groupStartStack = groupStartStack[:len(groupStartStack)-1]
		case '<', '>', '\'', '$':
			pattern.WriteRune('\\')
			pattern.WriteRune(r)
		case ',':
			// Commas are part of the pattern if they appear in a group
			if groupStartStack[len(groupStartStack)-1] == '{' {
				pattern.WriteRune('|')
			} else {
				return nil, false, errors.NewInvalidParamsError(fmt.Errorf("invalid ',' outside of matching group at pos %d in %s", i, glob))
			}
		default:
			if !strings.ContainsRune(ValidIdentifierRunes, r) {
				return nil, false, errors.NewInvalidParamsError(fmt.Errorf("invalid character %c at pos %d in %s", r, i, glob))
			}
			pattern.WriteRune(r)
		}
	}

	if len(groupStartStack) > 1 {
		return nil, false, errors.NewInvalidParamsError(fmt.Errorf("unbalanced '%c' in %s", groupStartStack[len(groupStartStack)-1], glob))
	}

	return pattern.buff.Bytes(), regexed, nil
}
