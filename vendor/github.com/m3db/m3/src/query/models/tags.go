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

package models

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/m3db/m3/src/metrics/generated/proto/metricpb"
	xerrors "github.com/m3db/m3/src/x/errors"

	"github.com/cespare/xxhash/v2"
)

var (
	errNoTags = errors.New("no tags")
)

// NewTags builds a tags with the given size and tag options.
func NewTags(size int, opts TagOptions) Tags {
	if opts == nil {
		opts = NewTagOptions()
	}

	return Tags{
		// Todo: Pool these
		Tags: make([]Tag, 0, size),
		Opts: opts,
	}
}

// EmptyTags returns empty tags with a default tag options.
func EmptyTags() Tags {
	return NewTags(0, nil)
}

// LastComputedID returns the last computed ID; this should only be
// used when it is guaranteed that no tag transforms take place between calls.
func (t *Tags) LastComputedID() []byte {
	if t.id == nil {
		t.id = t.ID()
	}

	return t.id
}

// ID returns a byte slice representation of the tags, using the generation
// strategy from the tag options.
func (t Tags) ID() []byte {
	return id(t)
}

func (t Tags) tagSubset(keys [][]byte, include bool) Tags {
	tags := NewTags(t.Len(), t.Opts)
	for _, tag := range t.Tags {
		found := false
		for _, k := range keys {
			if bytes.Equal(tag.Name, k) {
				found = true
				break
			}
		}

		if found == include {
			tags = tags.AddTag(tag)
		}
	}

	return tags
}

// TagsWithoutKeys returns only the tags which do not have the given keys.
func (t Tags) TagsWithoutKeys(excludeKeys [][]byte) Tags {
	return t.tagSubset(excludeKeys, false)
}

// TagsWithKeys returns only the tags which have the given keys.
func (t Tags) TagsWithKeys(includeKeys [][]byte) Tags {
	return t.tagSubset(includeKeys, true)
}

// WithoutName copies the tags excluding the name tag.
func (t Tags) WithoutName() Tags {
	return t.TagsWithoutKeys([][]byte{t.Opts.MetricName()})
}

// Get returns the value for the tag with the given name.
func (t Tags) Get(key []byte) ([]byte, bool) {
	for _, tag := range t.Tags {
		if bytes.Equal(tag.Name, key) {
			return tag.Value, true
		}
	}

	return nil, false
}

// Clone returns a copy of the tags.
func (t Tags) Clone() Tags {
	// TODO: Pool these
	clonedTags := make([]Tag, t.Len())
	for i, tag := range t.Tags {
		clonedTags[i] = tag.Clone()
	}

	return Tags{
		Tags: clonedTags,
		Opts: t.Opts,
	}
}

// AddTag is used to add a single tag and maintain sorted order.
func (t Tags) AddTag(tag Tag) Tags {
	t.Tags = append(t.Tags, tag)
	return t.Normalize()
}

// AddTagWithoutNormalizing is used to add a single tag.
func (t Tags) AddTagWithoutNormalizing(tag Tag) Tags {
	t.Tags = append(t.Tags, tag)
	return t
}

// SetName sets the metric name.
func (t Tags) SetName(value []byte) Tags {
	return t.AddOrUpdateTag(Tag{Name: t.Opts.MetricName(), Value: value})
}

// Name gets the metric name.
func (t Tags) Name() ([]byte, bool) {
	return t.Get(t.Opts.MetricName())
}

// SetBucket sets the bucket tag value.
func (t Tags) SetBucket(value []byte) Tags {
	return t.AddOrUpdateTag(Tag{Name: t.Opts.BucketName(), Value: value})
}

// Bucket gets the bucket tag value.
func (t Tags) Bucket() ([]byte, bool) {
	return t.Get(t.Opts.BucketName())
}

// AddTags is used to add a list of tags and maintain sorted order.
func (t Tags) AddTags(tags []Tag) Tags {
	t.Tags = append(t.Tags, tags...)
	return t.Normalize()
}

// AddOrUpdateTag is used to add a single tag and maintain sorted order,
// or to replace the value of an existing tag.
func (t Tags) AddOrUpdateTag(tag Tag) Tags {
	tags := t.Tags
	for i, tt := range tags {
		if bytes.Equal(tag.Name, tt.Name) {
			tags[i].Value = tag.Value
			return t
		}
	}

	return t.AddTag(tag)
}

// AddTagsIfNotExists is used to add a list of tags with unique names
// and maintain sorted order.
func (t Tags) AddTagsIfNotExists(tags []Tag) Tags {
	for _, tt := range tags {
		t = t.addIfMissingTag(tt)
	}

	return t.Normalize()
}

// addIfMissingTag is used to add a single tag and maintain sorted order,
// or to replace the value of an existing tag.
func (t Tags) addIfMissingTag(tag Tag) Tags {
	tags := t.Tags
	for _, tt := range tags {
		if bytes.Equal(tag.Name, tt.Name) {
			return t
		}
	}

	return t.AddTag(tag)
}

// Add is used to add two tag structures and maintain sorted order.
func (t Tags) Add(other Tags) Tags {
	t.Tags = append(t.Tags, other.Tags...)
	return t.Normalize()
}

// Ensure Tags implements sort interface.
var _ sort.Interface = Tags{}

func (t Tags) Len() int      { return len(t.Tags) }
func (t Tags) Swap(i, j int) { t.Tags[i], t.Tags[j] = t.Tags[j], t.Tags[i] }
func (t Tags) Less(i, j int) bool {
	return bytes.Compare(t.Tags[i].Name, t.Tags[j].Name) == -1
}

// Ensure sortableTagsNumericallyAsc implements sort interface.
var _ sort.Interface = sortableTagsNumericallyAsc{}

type sortableTagsNumericallyAsc Tags

func (t sortableTagsNumericallyAsc) Len() int { return len(t.Tags) }
func (t sortableTagsNumericallyAsc) Swap(i, j int) {
	t.Tags[i], t.Tags[j] = t.Tags[j], t.Tags[i]
}
func (t sortableTagsNumericallyAsc) Less(i, j int) bool {
	iName, jName := t.Tags[i].Name, t.Tags[j].Name
	lenDiff := len(iName) - len(jName)
	if lenDiff < 0 {
		return true
	}

	if lenDiff > 0 {
		return false
	}

	return bytes.Compare(iName, jName) == -1
}

// Normalize normalizes the tags by sorting them in place.
// In the future, it might also ensure other things like uniqueness.
func (t Tags) Normalize() Tags {
	if t.Opts.IDSchemeType() == TypeGraphite {
		// Graphite tags are sorted numerically rather than lexically.
		sort.Sort(sortableTagsNumericallyAsc(t))
	} else {
		sort.Sort(t)
	}

	return t
}

// Validate will validate there are tag values, and the
// tags are ordered and there are no duplicates.
func (t Tags) Validate() error {
	// Wrap call to validate to make sure a validation error
	// is always an invalid parameters error so we return bad request
	// instead of internal server error at higher in the stack.
	if err := t.validate(); err != nil {
		return xerrors.NewInvalidParamsError(err)
	}
	return nil
}

func (t Tags) validate() error {
	n := t.Len()
	if n == 0 {
		return errNoTags
	}

	if t.Opts.IDSchemeType() == TypeGraphite {
		// Graphite tags are sorted numerically rather than lexically.
		tags := sortableTagsNumericallyAsc(t)
		for i, tag := range tags.Tags {
			if len(tag.Name) == 0 {
				return fmt.Errorf("tag name empty: index=%d", i)
			}
			if i == 0 {
				continue // Don't check order/unique attributes.
			}

			if !tags.Less(i-1, i) {
				return fmt.Errorf("graphite tags out of order: '%s' appears after"+
					" '%s', tags: %v", tags.Tags[i-1].Name, tags.Tags[i].Name, tags.Tags)
			}

			prev := tags.Tags[i-1]
			if bytes.Compare(prev.Name, tag.Name) == 0 {
				return fmt.Errorf("tags duplicate: '%s' appears more than once",
					tags.Tags[i-1].Name)
			}
		}
	} else {
		var (
			allowTagNameDuplicates = t.Opts.AllowTagNameDuplicates()
			allowTagValueEmpty     = t.Opts.AllowTagValueEmpty()
		)
		// Sorted alphanumerically otherwise, use bytes.Compare once for
		// both order and unique test.
		for i, tag := range t.Tags {
			if len(tag.Name) == 0 {
				return fmt.Errorf("tag name empty: index=%d", i)
			}
			if !allowTagValueEmpty && len(tag.Value) == 0 {
				return fmt.Errorf("tag value empty: index=%d, name=%s",
					i, t.Tags[i].Name)
			}
			if i == 0 {
				continue // Don't check order/unique attributes.
			}

			prev := t.Tags[i-1]
			cmp := bytes.Compare(prev.Name, t.Tags[i].Name)
			if cmp > 0 {
				return fmt.Errorf("tags out of order: '%s' appears after '%s', tags: %v",
					prev.Name, tag.Name, t.Tags)
			}
			if !allowTagNameDuplicates && cmp == 0 {
				return fmt.Errorf("tags duplicate: '%s' appears more than once in '%s'",
					prev.Name, t)
			}
		}
	}

	return nil
}

// Reset resets the tags for reuse.
func (t Tags) Reset() Tags {
	t.Tags = t.Tags[:0]
	return t
}

// HashedID returns the hashed ID for the tags.
func (t Tags) HashedID() uint64 {
	return xxhash.Sum64(t.ID())
}

// LastComputedHashedID returns the last computed hashed ID; this should only be
// used when it is guaranteed that no tag transforms take place between calls.
func (t *Tags) LastComputedHashedID() uint64 {
	if t.hashedID == 0 {
		t.hashedID = xxhash.Sum64(t.LastComputedID())
	}

	return t.hashedID
}

// Equals returns a boolean reporting whether the compared tags have the same
// values.
//
// NB: does not check that compared tags have the same underlying bytes.
func (t Tags) Equals(other Tags) bool {
	if t.Len() != other.Len() {
		return false
	}

	if !t.Opts.Equals(other.Opts) {
		return false
	}

	for i, t := range t.Tags {
		if !t.Equals(other.Tags[i]) {
			return false
		}
	}

	return true
}

var tagSeperator = []byte(", ")

// String returns the string representation of the tags.
func (t Tags) String() string {
	var sb strings.Builder
	for i, tt := range t.Tags {
		if i != 0 {
			sb.Write(tagSeperator)
		}
		sb.WriteString(tt.String())
	}
	return sb.String()
}

// TagsFromProto converts proto tags to models.Tags.
func TagsFromProto(pbTags []*metricpb.Tag) []Tag {
	tags := make([]Tag, 0, len(pbTags))
	for _, tag := range pbTags {
		tags = append(tags, Tag{
			Name:  tag.Name,
			Value: tag.Value,
		})
	}
	return tags
}

// ToProto converts the models.Tags to proto tags.
func (t Tag) ToProto() *metricpb.Tag {
	return &metricpb.Tag{
		Name:  t.Name,
		Value: t.Value,
	}
}

// String returns the string representation of the tag.
func (t Tag) String() string {
	return fmt.Sprintf("%s: %s", t.Name, t.Value)
}

// Equals returns a boolean indicating whether the provided tags are equal.
//
// NB: does not check that compared tags have the same underlying bytes.
func (t Tag) Equals(other Tag) bool {
	return bytes.Equal(t.Name, other.Name) && bytes.Equal(t.Value, other.Value)
}

// Clone returns a copy of the tag.
func (t Tag) Clone() Tag {
	// Todo: Pool these
	clonedName := make([]byte, len(t.Name))
	clonedVal := make([]byte, len(t.Value))
	copy(clonedName, t.Name)
	copy(clonedVal, t.Value)
	return Tag{
		Name:  clonedName,
		Value: clonedVal,
	}
}
