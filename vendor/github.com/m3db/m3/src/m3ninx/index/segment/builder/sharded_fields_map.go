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

package builder

type shardedFieldsMap struct {
	data []*fieldsMap
}

func newShardedFieldsMap(
	numShards int,
	shardInitialCapacity int,
) *shardedFieldsMap {
	data := make([]*fieldsMap, 0, numShards)
	for i := 0; i < numShards; i++ {
		data = append(data, newFieldsMap(fieldsMapOptions{
			InitialSize: shardInitialCapacity,
		}))
	}
	return &shardedFieldsMap{
		data: data,
	}
}

func (s *shardedFieldsMap) ShardedGet(
	shard int,
	k []byte,
) (*terms, bool) {
	return s.data[shard].Get(k)
}

func (s *shardedFieldsMap) ShardedSetUnsafe(
	shard int,
	k []byte,
	v *terms,
	opts fieldsMapSetUnsafeOptions,
) {
	s.data[shard].SetUnsafe(k, v, opts)
}

// ResetTermsSets keeps fields around but resets the terms set for each one.
func (s *shardedFieldsMap) ResetTermsSets() {
	for _, fieldMap := range s.data {
		for _, entry := range fieldMap.Iter() {
			entry.Value().reset()
		}
	}
}
