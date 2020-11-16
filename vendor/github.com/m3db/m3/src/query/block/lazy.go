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

package block

type lazyBlock struct {
	block Block
	opts  LazyOptions
}

// NewLazyBlock creates a lazy block wrapping another block with lazy options.
func NewLazyBlock(block Block, opts LazyOptions) Block {
	return &lazyBlock{
		block: block,
		opts:  opts,
	}
}

func (b *lazyBlock) Info() BlockInfo {
	return NewWrappedBlockInfo(BlockLazy, b.block.Info())
}

func (b *lazyBlock) Close() error { return b.block.Close() }

func (b *lazyBlock) Meta() Metadata {
	mt := b.opts.MetaTransform()
	return mt(b.block.Meta())
}

// StepIter returns a StepIterator
func (b *lazyBlock) StepIter() (StepIter, error) {
	iter, err := b.block.StepIter()
	if err != nil {
		return nil, err
	}

	return &lazyStepIter{
		it:   iter,
		opts: b.opts,
	}, nil
}

type lazyStepIter struct {
	it   StepIter
	opts LazyOptions
}

func (it *lazyStepIter) Close()         { it.it.Close() }
func (it *lazyStepIter) Err() error     { return it.it.Err() }
func (it *lazyStepIter) StepCount() int { return it.it.StepCount() }
func (it *lazyStepIter) Next() bool     { return it.it.Next() }

func (it *lazyStepIter) SeriesMeta() []SeriesMeta {
	mt := it.opts.SeriesMetaTransform()
	return mt(it.it.SeriesMeta())
}

func (it *lazyStepIter) Current() Step {
	var (
		c        = it.it.Current()
		tt, vt   = it.opts.TimeTransform(), it.opts.ValueTransform()
		stepVals = c.Values()
	)

	vals := make([]float64, 0, len(stepVals))
	for _, val := range stepVals {
		vals = append(vals, vt(val))
	}

	return ColStep{
		time:   tt(c.Time()),
		values: vals,
	}
}

func (b *lazyBlock) SeriesIter() (SeriesIter, error) {
	seriesIter, err := b.block.SeriesIter()
	if err != nil {
		return nil, err
	}

	return &lazySeriesIter{
		it:   seriesIter,
		opts: b.opts,
	}, nil
}

type lazySeriesIter struct {
	it   SeriesIter
	opts LazyOptions
}

func (it *lazySeriesIter) Close()           { it.it.Close() }
func (it *lazySeriesIter) Err() error       { return it.it.Err() }
func (it *lazySeriesIter) SeriesCount() int { return it.it.SeriesCount() }
func (it *lazySeriesIter) Next() bool       { return it.it.Next() }

func (it *lazySeriesIter) SeriesMeta() []SeriesMeta {
	mt := it.opts.SeriesMetaTransform()
	return mt(it.it.SeriesMeta())
}

func (it *lazySeriesIter) Current() UnconsolidatedSeries {
	var (
		c      = it.it.Current()
		values = c.datapoints
		tt, vt = it.opts.TimeTransform(), it.opts.ValueTransform()
	)

	for i, v := range values {
		c.datapoints[i].Timestamp = tt(v.Timestamp)
		c.datapoints[i].Value = vt(v.Value)
	}

	return c
}

func (b *lazyBlock) MultiSeriesIter(
	concurrency int,
) ([]SeriesIterBatch, error) {
	batches, err := b.block.MultiSeriesIter(concurrency)
	if err != nil {
		return nil, err
	}

	for i, batch := range batches {
		batches[i].Iter = &lazySeriesIter{
			it:   batch.Iter,
			opts: b.opts,
		}
	}

	return batches, err
}
