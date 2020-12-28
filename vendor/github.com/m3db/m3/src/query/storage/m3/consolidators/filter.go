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

package consolidators

import (
	"bytes"

	"github.com/m3db/m3/src/query/models"
	"github.com/m3db/m3/src/x/ident"
)

func filterTagIterator(
	iter ident.TagIterator,
	filters models.Filters,
) (bool, error) {
	shouldFilter := shouldFilterTagIterator(iter, filters)
	return shouldFilter, iter.Err()
}

func shouldFilterTagIterator(
	iter ident.TagIterator,
	filters models.Filters,
) bool {
	if len(filters) == 0 || iter.Remaining() == 0 {
		return false
	}

	// NB: rewind iterator for re-use.
	defer iter.Rewind()
	for iter.Next() {
		tag := iter.Current()

		name := tag.Name.Bytes()
		value := tag.Value.Bytes()
		for _, f := range filters {
			if !bytes.Equal(name, f.Name) {
				continue
			}

			// 0 length filters implies filtering for entire range.
			if len(f.Values) == 0 {
				return true
			}

			for _, filterValue := range f.Values {
				if bytes.Equal(filterValue, value) {
					return true
				}
			}
		}
	}

	return false
}

func filterNames(tags []CompletedTag, filters models.Filters) []CompletedTag {
	if len(filters) == 0 || len(tags) == 0 {
		return tags
	}

	filteredTags := tags[:0]
	for _, tag := range tags {
		skip := false
		for _, f := range filters {
			if len(f.Values) != 0 {
				// If this has filter values, it is not a name filter, and the result
				// is valid.
				continue
			}

			if bytes.Equal(tag.Name, f.Name) {
				skip = true
				break
			}
		}

		if !skip {
			filteredTags = append(filteredTags, tag)
		}
	}

	return filteredTags
}

func filterTags(tags []CompletedTag, filters models.Filters) []CompletedTag {
	if len(filters) == 0 || len(tags) == 0 {
		return tags
	}

	filteredTags := tags[:0]
	for _, tag := range tags {
		for _, f := range filters {
			if !bytes.Equal(tag.Name, f.Name) {
				continue
			}

			// NB: Name filter matches.
			if len(f.Values) == 0 {
				tag.Values = tag.Values[:0]
				break
			}

			filteredValues := tag.Values[:0]
			for _, value := range tag.Values {
				skip := false
				for _, filterValue := range f.Values {
					if bytes.Equal(filterValue, value) {
						skip = true
						break
					}
				}

				if !skip {
					filteredValues = append(filteredValues, value)
				}
			}

			tag.Values = filteredValues
			break
		}

		if len(tag.Values) == 0 {
			// NB: all values for this tag are invalid.
			continue
		}

		filteredTags = append(filteredTags, tag)
	}

	return filteredTags
}
