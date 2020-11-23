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

package strconv

// NB: for nicer table formatting.
const (
	// Indicates this rune does not need escaping.
	ff = false
	// Indicates this rune needs escaping.
	tt = true
)

// Determines valid characters that do not require escaping.
//
// NB: escape all control characters, `"`, and any character above `~`. This
// table loosely based on constants used in utf8.DecodeRune.
var escape = [256]bool{
	//   1   2   3   4   5   6   7   8   9   A   B   C   D   E   F
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0x00-0x0F
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0x10-0x1F
	ff, ff, tt, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0x20-0x2F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0x30-0x3F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0x40-0x4F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0x50-0x5F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0x60-0x6F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, tt, // 0x70-0x7F
	//   1   2   3   4   5   6   7   8   9   A   B   C   D   E   F
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0x80-0x8F
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0x90-0x9F
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0xA0-0xAF
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0xB0-0xBF
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0xC0-0xCF
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0xD0-0xDF
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0xE0-0xEF
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0xF0-0xFF
}

// NeedToEscape returns true if the byte slice contains characters that will
// need to be escaped when quoting the slice.
func NeedToEscape(bb []byte) bool {
	for _, b := range bb {
		if escape[b] {
			return true
		}
	}

	return false
}

// Determines valid alphanumeric characters.
var alphaNumeric = [256]bool{
	//   1   2   3   4   5   6   7   8   9   A   B   C   D   E   F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0x00-0x0F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0x10-0x1F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0x20-0x2F
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, ff, ff, ff, ff, ff, ff, // 0x30-0x3F
	ff, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0x40-0x4F
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, ff, ff, ff, ff, ff, // 0x50-0x5F
	ff, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, // 0x60-0x6F
	tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, tt, ff, ff, ff, ff, ff, // 0x70-0x7F
	//   1   2   3   4   5   6   7   8   9   A   B   C   D   E   F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0x80-0x8F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0x90-0x9F
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0xA0-0xAF
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0xB0-0xBF
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0xC0-0xCF
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0xD0-0xDF
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0xE0-0xEF
	ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, ff, // 0xF0-0xFF
}

// IsAlphaNumeric returns true if the given string is alpha numeric.
//
// NB: here this means that it contains only characters in [0-9A-Za-z])
func IsAlphaNumeric(str string) bool {
	for _, c := range str {
		if !alphaNumeric[c] {
			return false
		}
	}

	return true
}

// IsRuneAlphaNumeric returns true if the given rune is alpha numeric.
//
// NB: here this means that it contains only characters in [0-9A-Za-z])
func IsRuneAlphaNumeric(r rune) bool {
	return alphaNumeric[r]
}
