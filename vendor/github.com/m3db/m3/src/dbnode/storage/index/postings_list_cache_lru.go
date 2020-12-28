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

package index

import (
	"container/list"
	"errors"

	"github.com/m3db/m3/src/m3ninx/postings"

	"github.com/pborman/uuid"
)

// PostingsListLRU implements a non-thread safe fixed size LRU cache of postings lists
// that were resolved by running a given query against a particular segment for a given
// field and pattern type (term vs regexp). Normally a key in the LRU would look like:
//
// type key struct {
//    segmentUUID uuid.UUID
//    field       string
//    pattern     string
//    patternType PatternType
// }
//
// However, some of the postings lists that we will store in the LRU have a fixed lifecycle
// because they reference mmap'd byte slices which will eventually be unmap'd. To prevent
// these postings lists that point to unmap'd regions from remaining in the LRU, we want to
// support the ability to efficiently purge the LRU of any postings list that belong to a
// given segment. This isn't technically required for correctness as once a segment has been
// closed, its old postings list in the LRU will never be accessed again (since they are only
// addressable by that segments UUID), but we purge them from the LRU before closing the segment
// anyways as an additional safety precaution.
//
// Instead of adding additional tracking on-top of an existing generic LRU, we've created a
// specialized LRU that instead of having a single top-level map pointing into the linked-list,
// has a two-level map where the top level map is keyed by segment UUID and the second level map
// is keyed by the field/pattern/patternType.
//
// As a result, when a segment is ready to be closed, they can call into the cache with their
// UUID and we can efficiently remove all the entries corresponding to that segment from the
// LRU. The specialization has the additional nice property that we don't need to allocate everytime
// we add an item to the LRU due to the interface{} conversion.
type postingsListLRU struct {
	size      int
	evictList *list.List
	items     map[uuid.Array]map[key]*list.Element
}

// entry is used to hold a value in the evictList.
type entry struct {
	uuid         uuid.UUID
	key          key
	postingsList postings.List
}

type key struct {
	field       string
	pattern     string
	patternType PatternType
}

// newPostingsListLRU constructs an LRU of the given size.
func newPostingsListLRU(size int) (*postingsListLRU, error) {
	if size <= 0 {
		return nil, errors.New("Must provide a positive size")
	}

	return &postingsListLRU{
		size:      size,
		evictList: list.New(),
		items:     make(map[uuid.Array]map[key]*list.Element),
	}, nil
}

// Add adds a value to the cache. Returns true if an eviction occurred.
func (c *postingsListLRU) Add(
	segmentUUID uuid.UUID,
	field string,
	pattern string,
	patternType PatternType,
	pl postings.List,
) (evicted bool) {
	newKey := newKey(field, pattern, patternType)
	// Check for existing item.
	uuidArray := segmentUUID.Array()
	if uuidEntries, ok := c.items[uuidArray]; ok {
		if ent, ok := uuidEntries[newKey]; ok {
			// If it already exists, just move it to the front. This avoids storing
			// the same item in the LRU twice which is important because the maps
			// can only point to one entry at a time and we use them for purges. Also,
			// it saves space by avoiding storing duplicate values.
			c.evictList.MoveToFront(ent)
			ent.Value.(*entry).postingsList = pl
			return false
		}
	}

	// Add new item.
	var (
		ent = &entry{
			uuid:         segmentUUID,
			key:          newKey,
			postingsList: pl,
		}
		entry = c.evictList.PushFront(ent)
	)
	if queries, ok := c.items[uuidArray]; ok {
		queries[newKey] = entry
	} else {
		c.items[uuidArray] = map[key]*list.Element{
			newKey: entry,
		}
	}

	evict := c.evictList.Len() > c.size
	// Verify size not exceeded.
	if evict {
		c.removeOldest()
	}
	return evict
}

// Get looks up a key's value from the cache.
func (c *postingsListLRU) Get(
	segmentUUID uuid.UUID,
	field string,
	pattern string,
	patternType PatternType,
) (postings.List, bool) {
	newKey := newKey(field, pattern, patternType)
	uuidArray := segmentUUID.Array()
	if uuidEntries, ok := c.items[uuidArray]; ok {
		if ent, ok := uuidEntries[newKey]; ok {
			c.evictList.MoveToFront(ent)
			return ent.Value.(*entry).postingsList, true
		}
	}

	return nil, false
}

// Remove removes the provided key from the cache, returning if the
// key was contained.
func (c *postingsListLRU) Remove(
	segmentUUID uuid.UUID,
	field string,
	pattern string,
	patternType PatternType,
) bool {
	newKey := newKey(field, pattern, patternType)
	uuidArray := segmentUUID.Array()
	if uuidEntries, ok := c.items[uuidArray]; ok {
		if ent, ok := uuidEntries[newKey]; ok {
			c.removeElement(ent)
			return true
		}
	}

	return false
}

func (c *postingsListLRU) PurgeSegment(segmentUUID uuid.UUID) {
	if uuidEntries, ok := c.items[segmentUUID.Array()]; ok {
		for _, ent := range uuidEntries {
			c.removeElement(ent)
		}
	}
}

// Len returns the number of items in the cache.
func (c *postingsListLRU) Len() int {
	return c.evictList.Len()
}

// removeOldest removes the oldest item from the cache.
func (c *postingsListLRU) removeOldest() {
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
	}
}

// removeElement is used to remove a given list element from the cache
func (c *postingsListLRU) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	entry := e.Value.(*entry)

	if patterns, ok := c.items[entry.uuid.Array()]; ok {
		delete(patterns, entry.key)
		if len(patterns) == 0 {
			delete(c.items, entry.uuid.Array())
		}
	}
}

func newKey(field, pattern string, patternType PatternType) key {
	return key{field: field, pattern: pattern, patternType: patternType}
}
