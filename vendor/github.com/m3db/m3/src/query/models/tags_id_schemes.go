// Copyright (c) 2020 Uber Technologies, Inc.
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

package models

import (
	"github.com/m3db/m3/src/query/models/strconv"
	"github.com/m3db/m3/src/query/util/writer"
)

func id(t Tags) []byte {
	schemeType := t.Opts.IDSchemeType()
	if len(t.Tags) == 0 {
		if schemeType == TypeQuoted {
			return []byte("{}")
		}

		return []byte("")
	}

	switch schemeType {
	case TypeLegacy:
		return legacyID(t)
	case TypeQuoted:
		return quotedID(t)
	case TypePrependMeta:
		return prependMetaID(t)
	case TypeGraphite:
		return graphiteID(t)
	default:
		// Default to quoted meta
		// NB: realistically, schema defaults should be set by here.
		return quotedID(t)
	}
}

func legacyID(t Tags) []byte {
	// TODO: pool these bytes.
	id := make([]byte, idLen(t))
	idx := -1
	for _, tag := range t.Tags {
		idx += copy(id[idx+1:], tag.Name) + 1
		id[idx] = eq
		idx += copy(id[idx+1:], tag.Value) + 1
		id[idx] = sep
	}

	return id
}

func idLen(t Tags) int {
	idLen := 2 * t.Len() // account for separators
	for _, tag := range t.Tags {
		idLen += len(tag.Name)
		idLen += len(tag.Value)
	}

	return idLen
}

type tagEscaping struct {
	escapeName  bool
	escapeValue bool
}

func quotedID(t Tags) []byte {
	var (
		idLen        int
		needEscaping []tagEscaping
		l            int
		escape       tagEscaping
	)

	for i, tt := range t.Tags {
		l, escape = serializedLength(tt)
		idLen += l
		if escape.escapeName || escape.escapeValue {
			if needEscaping == nil {
				needEscaping = make([]tagEscaping, len(t.Tags))
			}

			needEscaping[i] = escape
		}
	}

	tagLength := 2 * len(t.Tags)
	idLen += tagLength + 1 // account for separators and brackets
	if needEscaping == nil {
		return quoteIDSimple(t, idLen)
	}

	// TODO: pool these bytes
	lastIndex := len(t.Tags) - 1
	id := make([]byte, idLen)
	id[0] = leftBracket
	idx := 1
	for i, tt := range t.Tags[:lastIndex] {
		idx = writeAtIndex(tt, id, needEscaping[i], idx)
		id[idx] = sep
		idx++
	}

	idx = writeAtIndex(t.Tags[lastIndex], id, needEscaping[lastIndex], idx)
	id[idx] = rightBracket
	return id
}

// adds quotes to tag values when no characters need escaping.
func quoteIDSimple(t Tags, length int) []byte {
	// TODO: pool these bytes.
	id := make([]byte, length)
	id[0] = leftBracket
	idx := 1
	lastIndex := len(t.Tags) - 1
	for _, tag := range t.Tags[:lastIndex] {
		idx += copy(id[idx:], tag.Name)
		id[idx] = eq
		idx++
		idx = strconv.QuoteSimple(id, tag.Value, idx)
		id[idx] = sep
		idx++
	}

	tag := t.Tags[lastIndex]
	idx += copy(id[idx:], tag.Name)
	id[idx] = eq
	idx++
	idx = strconv.QuoteSimple(id, tag.Value, idx)
	id[idx] = rightBracket

	return id
}

func writeAtIndex(t Tag, id []byte, escape tagEscaping, idx int) int {
	if escape.escapeName {
		idx = strconv.Escape(id, t.Name, idx)
	} else {
		idx += copy(id[idx:], t.Name)
	}

	id[idx] = eq
	idx++

	if escape.escapeValue {
		idx = strconv.Quote(id, t.Value, idx)
	} else {
		idx = strconv.QuoteSimple(id, t.Value, idx)
	}

	return idx
}

func serializedLength(t Tag) (int, tagEscaping) {
	var (
		idLen    int
		escaping tagEscaping
	)
	if strconv.NeedToEscape(t.Name) {
		idLen += strconv.EscapedLength(t.Name)
		escaping.escapeName = true
	} else {
		idLen += len(t.Name)
	}

	if strconv.NeedToEscape(t.Value) {
		idLen += strconv.QuotedLength(t.Value)
		escaping.escapeValue = true
	} else {
		idLen += len(t.Value) + 2
	}

	return idLen, escaping
}

func writeTagLengthMeta(dst []byte, lengths []int) int {
	idx := writer.WriteIntegers(dst, lengths, sep, 0)
	dst[idx] = finish
	return idx + 1
}

func prependMetaID(t Tags) []byte {
	l, metaLengths := prependMetaLen(t)
	// TODO: pool these bytes.
	id := make([]byte, l)
	idx := writeTagLengthMeta(id, metaLengths)
	for _, tag := range t.Tags {
		idx += copy(id[idx:], tag.Name)
		idx += copy(id[idx:], tag.Value)
	}

	return id
}

func prependMetaLen(t Tags) (int, []int) {
	idLen := 1 // account for separator
	tagLengths := make([]int, len(t.Tags)*2)
	for i, tag := range t.Tags {
		tagLen := len(tag.Name)
		tagLengths[2*i] = tagLen
		idLen += tagLen
		tagLen = len(tag.Value)
		tagLengths[2*i+1] = tagLen
		idLen += tagLen
	}

	prefixLen := writer.IntsLength(tagLengths)
	return idLen + prefixLen, tagLengths
}

func idLenGraphite(t Tags) int {
	idLen := t.Len() - 1 // account for separators
	for _, tag := range t.Tags {
		idLen += len(tag.Value)
	}

	return idLen
}

func graphiteID(t Tags) []byte {
	// TODO: pool these bytes.
	id := make([]byte, idLenGraphite(t))
	idx := 0
	lastIndex := len(t.Tags) - 1
	for _, tag := range t.Tags[:lastIndex] {
		idx += copy(id[idx:], tag.Value)
		id[idx] = graphiteSep
		idx++
	}

	copy(id[idx:], t.Tags[lastIndex].Value)
	return id
}
