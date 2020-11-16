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

package m3

import (
	"github.com/uber-go/tally"
	customtransport "github.com/uber-go/tally/m3/customtransports"
	m3thrift "github.com/uber-go/tally/m3/thrift"
	"github.com/uber-go/tally/thirdparty/github.com/apache/thrift/lib/go/thrift"
)

const (
	batchPoolSize   = 10
	metricPoolSize  = DefaultMaxQueueSize
	valuePoolSize   = DefaultMaxQueueSize
	timerPoolSize   = DefaultMaxQueueSize
	tagPoolSize     = DefaultMaxQueueSize
	counterPoolSize = DefaultMaxQueueSize
	gaugePoolSize   = DefaultMaxQueueSize
	protoPoolSize   = 10
)

type resourcePool struct {
	batchPool   *tally.ObjectPool
	metricPool  *tally.ObjectPool
	tagPool     *tally.ObjectPool
	valuePool   *tally.ObjectPool
	counterPool *tally.ObjectPool
	gaugePool   *tally.ObjectPool
	timerPool   *tally.ObjectPool
	protoPool   *tally.ObjectPool
}

func newResourcePool(protoFac thrift.TProtocolFactory) *resourcePool {
	batchPool := tally.NewObjectPool(batchPoolSize)
	batchPool.Init(func() interface{} {
		return m3thrift.NewMetricBatch()
	})

	metricPool := tally.NewObjectPool(metricPoolSize)
	metricPool.Init(func() interface{} {
		return m3thrift.NewMetric()
	})

	tagPool := tally.NewObjectPool(tagPoolSize)
	tagPool.Init(func() interface{} {
		return m3thrift.NewMetricTag()
	})

	valuePool := tally.NewObjectPool(valuePoolSize)
	valuePool.Init(func() interface{} {
		return m3thrift.NewMetricValue()
	})

	counterPool := tally.NewObjectPool(counterPoolSize)
	counterPool.Init(func() interface{} {
		return m3thrift.NewCountValue()
	})

	gaugePool := tally.NewObjectPool(gaugePoolSize)
	gaugePool.Init(func() interface{} {
		return m3thrift.NewGaugeValue()
	})

	timerPool := tally.NewObjectPool(timerPoolSize)
	timerPool.Init(func() interface{} {
		return m3thrift.NewTimerValue()
	})

	protoPool := tally.NewObjectPool(protoPoolSize)
	protoPool.Init(func() interface{} {
		return protoFac.GetProtocol(&customtransport.TCalcTransport{})
	})

	return &resourcePool{
		batchPool:   batchPool,
		metricPool:  metricPool,
		tagPool:     tagPool,
		valuePool:   valuePool,
		counterPool: counterPool,
		gaugePool:   gaugePool,
		timerPool:   timerPool,
		protoPool:   protoPool,
	}
}

func (r *resourcePool) getBatch() *m3thrift.MetricBatch {
	o := r.batchPool.Get()
	return o.(*m3thrift.MetricBatch)
}

func (r *resourcePool) getMetric() *m3thrift.Metric {
	o := r.metricPool.Get()
	return o.(*m3thrift.Metric)
}

func (r *resourcePool) getTagList() map[*m3thrift.MetricTag]bool {
	return map[*m3thrift.MetricTag]bool{}
}

func (r *resourcePool) getTag() *m3thrift.MetricTag {
	o := r.tagPool.Get()
	return o.(*m3thrift.MetricTag)
}

func (r *resourcePool) getValue() *m3thrift.MetricValue {
	o := r.valuePool.Get()
	return o.(*m3thrift.MetricValue)
}

func (r *resourcePool) getCount() *m3thrift.CountValue {
	o := r.counterPool.Get()
	return o.(*m3thrift.CountValue)
}

func (r *resourcePool) getGauge() *m3thrift.GaugeValue {
	o := r.gaugePool.Get()
	return o.(*m3thrift.GaugeValue)
}

func (r *resourcePool) getTimer() *m3thrift.TimerValue {
	o := r.timerPool.Get()
	return o.(*m3thrift.TimerValue)
}

func (r *resourcePool) getProto() thrift.TProtocol {
	o := r.protoPool.Get()
	return o.(thrift.TProtocol)
}

func (r *resourcePool) releaseProto(proto thrift.TProtocol) {
	calc := proto.Transport().(*customtransport.TCalcTransport)
	calc.ResetCount()
	r.protoPool.Put(proto)
}

func (r *resourcePool) releaseBatch(batch *m3thrift.MetricBatch) {
	batch.CommonTags = nil
	for _, metric := range batch.Metrics {
		r.releaseMetric(metric)
	}
	batch.Metrics = nil
	r.batchPool.Put(batch)
}

func (r *resourcePool) releaseMetricValue(metVal *m3thrift.MetricValue) {
	if metVal.IsSetCount() {
		metVal.Count.I64Value = nil
		r.counterPool.Put(metVal.Count)
		metVal.Count = nil
	} else if metVal.IsSetGauge() {
		metVal.Gauge.I64Value = nil
		metVal.Gauge.DValue = nil
		r.gaugePool.Put(metVal.Gauge)
		metVal.Gauge = nil
	} else if metVal.IsSetTimer() {
		metVal.Timer.I64Value = nil
		metVal.Timer.DValue = nil
		r.timerPool.Put(metVal.Timer)
		metVal.Timer = nil
	}
	r.valuePool.Put(metVal)
}

func (r *resourcePool) releaseMetrics(mets []*m3thrift.Metric) {
	for _, m := range mets {
		r.releaseMetric(m)
	}
}

func (r *resourcePool) releaseShallowMetrics(mets []*m3thrift.Metric) {
	for _, m := range mets {
		r.releaseShallowMetric(m)
	}
}

func (r *resourcePool) releaseMetric(metric *m3thrift.Metric) {
	metric.Name = ""
	// Release Tags
	for tag := range metric.Tags {
		tag.TagName = ""
		tag.TagValue = nil
		r.tagPool.Put(tag)
	}
	metric.Tags = nil

	r.releaseShallowMetric(metric)
}

func (r *resourcePool) releaseShallowMetric(metric *m3thrift.Metric) {
	metric.Name = ""
	metric.Tags = nil
	metric.Timestamp = nil

	metVal := metric.MetricValue
	if metVal == nil {
		r.metricPool.Put(metric)
		return
	}

	r.releaseMetricValue(metVal)
	metric.MetricValue = nil

	r.metricPool.Put(metric)
}
