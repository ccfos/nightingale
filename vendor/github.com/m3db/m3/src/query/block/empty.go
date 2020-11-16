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

type emptyBlock struct {
	meta Metadata
}

// NewEmptyBlock creates an empty block with the given metadata.
func NewEmptyBlock(meta Metadata) Block {
	return &emptyBlock{meta: meta}
}

func (b *emptyBlock) Close() error { return nil }

func (b *emptyBlock) Info() BlockInfo {
	return NewBlockInfo(BlockEmpty)
}

func (b *emptyBlock) Meta() Metadata {
	return b.meta
}

func (b *emptyBlock) StepIter() (StepIter, error) {
	return &emptyStepIter{steps: b.meta.Bounds.Steps()}, nil
}

type emptyStepIter struct {
	steps int
}

func (it *emptyStepIter) Close()                   {}
func (it *emptyStepIter) Err() error               { return nil }
func (it *emptyStepIter) StepCount() int           { return it.steps }
func (it *emptyStepIter) SeriesMeta() []SeriesMeta { return []SeriesMeta{} }
func (it *emptyStepIter) Next() bool               { return false }
func (it *emptyStepIter) Current() Step            { return nil }

func (b *emptyBlock) SeriesIter() (SeriesIter, error) {
	return &emptySeriesIter{}, nil
}

type emptySeriesIter struct{}

func (it *emptySeriesIter) Close()                        {}
func (it *emptySeriesIter) Err() error                    { return nil }
func (it *emptySeriesIter) SeriesCount() int              { return 0 }
func (it *emptySeriesIter) SeriesMeta() []SeriesMeta      { return []SeriesMeta{} }
func (it *emptySeriesIter) Next() bool                    { return false }
func (it *emptySeriesIter) Current() UnconsolidatedSeries { return UnconsolidatedSeries{} }

func (b *emptyBlock) MultiSeriesIter(
	concurrency int,
) ([]SeriesIterBatch, error) {
	batch := make([]SeriesIterBatch, concurrency)
	for i := range batch {
		batch[i] = SeriesIterBatch{
			Size: 1,
			Iter: &emptySeriesIter{},
		}
	}

	return batch, nil
}
