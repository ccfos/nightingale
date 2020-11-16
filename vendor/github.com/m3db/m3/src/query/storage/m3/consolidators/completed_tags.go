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

package consolidators

import (
	"bytes"
	"errors"
	"sort"
	"sync"

	"github.com/m3db/m3/src/query/block"
	"github.com/m3db/m3/src/query/models"
)

type completeTagsResultBuilder struct {
	sync.RWMutex
	nameOnly    bool
	metadata    block.ResultMetadata
	tagBuilders map[string]completedTagBuilder

	filters models.Filters
}

// NewCompleteTagsResultBuilder creates a new complete tags result builder.
func NewCompleteTagsResultBuilder(
	nameOnly bool,
	opts models.TagOptions,
) CompleteTagsResultBuilder {
	return &completeTagsResultBuilder{
		nameOnly: nameOnly,
		metadata: block.NewResultMetadata(),
		filters:  opts.Filters(),
	}
}

func (b *completeTagsResultBuilder) Add(tagResult *CompleteTagsResult) error {
	b.Lock()
	defer b.Unlock()

	nameOnly := b.nameOnly
	if nameOnly != tagResult.CompleteNameOnly {
		return errors.New("incoming tag result has mismatched type")
	}

	completedTags := tagResult.CompletedTags
	if b.tagBuilders == nil {
		b.tagBuilders = make(map[string]completedTagBuilder, len(completedTags))
	}

	b.metadata = b.metadata.CombineMetadata(tagResult.Metadata)
	if nameOnly {
		completedTags = filterNames(completedTags, b.filters)
		for _, tag := range completedTags {
			b.tagBuilders[string(tag.Name)] = completedTagBuilder{}
		}

		return nil
	}

	completedTags = filterTags(completedTags, b.filters)
	for _, tag := range completedTags {
		if builder, exists := b.tagBuilders[string(tag.Name)]; exists {
			builder.add(tag.Values)
		} else {
			builder := completedTagBuilder{}
			builder.add(tag.Values)
			b.tagBuilders[string(tag.Name)] = builder
		}
	}

	return nil
}

type completedTagsByName []CompletedTag

func (s completedTagsByName) Len() int      { return len(s) }
func (s completedTagsByName) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s completedTagsByName) Less(i, j int) bool {
	return bytes.Compare(s[i].Name, s[j].Name) == -1
}

func (b *completeTagsResultBuilder) Build() CompleteTagsResult {
	b.RLock()
	defer b.RUnlock()

	result := make([]CompletedTag, 0, len(b.tagBuilders))
	if b.nameOnly {
		for name := range b.tagBuilders {
			result = append(result, CompletedTag{
				Name:   []byte(name),
				Values: [][]byte{},
			})
		}

		sort.Sort(completedTagsByName(result))
		return CompleteTagsResult{
			CompleteNameOnly: true,
			CompletedTags:    result,
			Metadata:         b.metadata,
		}
	}

	for name, builder := range b.tagBuilders {
		result = append(result, CompletedTag{
			Name:   []byte(name),
			Values: builder.build(),
		})
	}

	sort.Sort(completedTagsByName(result))
	return CompleteTagsResult{
		CompleteNameOnly: false,
		CompletedTags:    result,
		Metadata:         b.metadata,
	}
}

type completedTagBuilder struct {
	seenMap map[string]struct{}
}

func (b *completedTagBuilder) add(values [][]byte) {
	if b.seenMap == nil {
		b.seenMap = make(map[string]struct{}, len(values))
	}

	for _, val := range values {
		b.seenMap[string(val)] = struct{}{}
	}
}

type tagValuesByName [][]byte

func (s tagValuesByName) Len() int      { return len(s) }
func (s tagValuesByName) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s tagValuesByName) Less(i, j int) bool {
	return bytes.Compare(s[i], s[j]) == -1
}

func (b *completedTagBuilder) build() [][]byte {
	result := make([][]byte, 0, len(b.seenMap))
	for v := range b.seenMap {
		result = append(result, []byte(v))
	}

	sort.Sort(tagValuesByName(result))
	return result
}
