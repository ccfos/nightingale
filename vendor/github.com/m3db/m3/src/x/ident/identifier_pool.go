// Copyright (c) 2016 Uber Technologies, Inc.
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

package ident

import (
	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/pool"
)

const (
	defaultCapacityOptions    = 16
	defaultMaxCapacityOptions = 32
)

// PoolOptions is a set of pooling options.
type PoolOptions struct {
	IDPoolOptions           pool.ObjectPoolOptions
	TagsPoolOptions         pool.ObjectPoolOptions
	TagsCapacity            int
	TagsMaxCapacity         int
	TagsIteratorPoolOptions pool.ObjectPoolOptions
}

func (o PoolOptions) defaultsIfNotSet() PoolOptions {
	if o.IDPoolOptions == nil {
		o.IDPoolOptions = pool.NewObjectPoolOptions()
	}
	if o.TagsPoolOptions == nil {
		o.TagsPoolOptions = pool.NewObjectPoolOptions()
	}
	if o.TagsCapacity == 0 {
		o.TagsCapacity = defaultCapacityOptions
	}
	if o.TagsMaxCapacity == 0 {
		o.TagsMaxCapacity = defaultMaxCapacityOptions
	}
	if o.TagsIteratorPoolOptions == nil {
		o.TagsIteratorPoolOptions = pool.NewObjectPoolOptions()
	}
	return o
}

// NewPool constructs a new simple Pool.
func NewPool(
	bytesPool pool.CheckedBytesPool,
	opts PoolOptions,
) Pool {
	opts = opts.defaultsIfNotSet()

	p := &simplePool{
		bytesPool: bytesPool,
		pool:      pool.NewObjectPool(opts.IDPoolOptions),
		tagArrayPool: newTagArrayPool(tagArrayPoolOpts{
			Options:     opts.TagsPoolOptions,
			Capacity:    opts.TagsCapacity,
			MaxCapacity: opts.TagsMaxCapacity,
		}),
		itersPool: pool.NewObjectPool(opts.TagsIteratorPoolOptions),
	}
	p.pool.Init(func() interface{} {
		return &id{pool: p}
	})
	p.tagArrayPool.Init()
	p.itersPool.Init(func() interface{} {
		return newTagSliceIter(Tags{}, nil, p)
	})

	return p
}

type simplePool struct {
	bytesPool    pool.CheckedBytesPool
	pool         pool.ObjectPool
	tagArrayPool tagArrayPool
	itersPool    pool.ObjectPool
}

func (p *simplePool) GetBinaryID(ctx context.Context, v checked.Bytes) ID {
	id := p.BinaryID(v)
	ctx.RegisterFinalizer(id)
	return id
}

func (p *simplePool) BinaryID(v checked.Bytes) ID {
	id := p.pool.Get().(*id)
	v.IncRef()
	id.pool, id.data = p, v
	return id
}

func (p *simplePool) GetBinaryTag(
	ctx context.Context,
	name checked.Bytes,
	value checked.Bytes,
) Tag {
	return Tag{
		Name:  TagName(p.GetBinaryID(ctx, name)),
		Value: TagValue(p.GetBinaryID(ctx, value)),
	}
}

func (p *simplePool) BinaryTag(
	name checked.Bytes,
	value checked.Bytes,
) Tag {
	return Tag{
		Name:  TagName(p.BinaryID(name)),
		Value: TagValue(p.BinaryID(value)),
	}
}

func (p *simplePool) GetStringID(ctx context.Context, v string) ID {
	id := p.StringID(v)
	ctx.RegisterFinalizer(id)
	return id
}

func (p *simplePool) StringID(v string) ID {
	data := p.bytesPool.Get(len(v))
	data.IncRef()
	data.AppendAll([]byte(v))
	data.DecRef()

	return p.BinaryID(data)
}

func (p *simplePool) GetTagsIterator(c context.Context) TagsIterator {
	iter := p.itersPool.Get().(*tagSliceIter)
	c.RegisterCloser(iter)
	return iter
}

func (p *simplePool) TagsIterator() TagsIterator {
	return p.itersPool.Get().(*tagSliceIter)
}

func (p *simplePool) Tags() Tags {
	return Tags{
		values: p.tagArrayPool.Get(),
		pool:   p,
	}
}

func (p *simplePool) Put(v ID) {
	p.pool.Put(v)
}

func (p *simplePool) PutTag(t Tag) {
	p.Put(t.Name)
	p.Put(t.Value)
}

func (p *simplePool) PutTags(t Tags) {
	p.tagArrayPool.Put(t.values)
}

func (p *simplePool) PutTagsIterator(iter TagsIterator) {
	iter.Reset(Tags{})
	p.itersPool.Put(iter)
}

func (p *simplePool) GetStringTag(ctx context.Context, name string, value string) Tag {
	return Tag{
		Name:  TagName(p.GetStringID(ctx, name)),
		Value: TagValue(p.GetStringID(ctx, value)),
	}
}

func (p *simplePool) StringTag(name string, value string) Tag {
	return Tag{
		Name:  TagName(p.StringID(name)),
		Value: TagValue(p.StringID(value)),
	}
}

func (p *simplePool) Clone(existing ID) ID {
	var (
		id      = p.pool.Get().(*id)
		data    = existing.Bytes()
		newData = p.bytesPool.Get(len(data))
	)

	newData.IncRef()
	newData.AppendAll(data)

	id.pool, id.data = p, newData

	return id
}

func (p *simplePool) CloneTag(t Tag) Tag {
	return Tag{
		Name:  p.Clone(t.Name),
		Value: p.Clone(t.Value),
	}
}

func (p *simplePool) CloneTags(t Tags) Tags {
	tags := p.tagArrayPool.Get()[:0]
	for _, tag := range t.Values() {
		tags = append(tags, p.CloneTag(tag))
	}
	return Tags{
		values: tags,
		pool:   p,
	}
}
