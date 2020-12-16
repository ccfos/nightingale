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
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/uber-go/tally"
	customtransport "github.com/uber-go/tally/m3/customtransports"
	m3thrift "github.com/uber-go/tally/m3/thrift"
	"github.com/uber-go/tally/m3/thriftudp"
	"github.com/uber-go/tally/thirdparty/github.com/apache/thrift/lib/go/thrift"
)

// Protocol describes a M3 thrift transport protocol.
type Protocol int

// Compact and Binary represent the compact and
// binary thrift protocols respectively.
const (
	Compact Protocol = iota
	Binary
)

const (
	// ServiceTag is the name of the M3 service tag.
	ServiceTag = "service"
	// EnvTag is the name of the M3 env tag.
	EnvTag = "env"
	// HostTag is the name of the M3 host tag.
	HostTag = "host"
	// DefaultMaxQueueSize is the default M3 reporter queue size.
	DefaultMaxQueueSize = 4096
	// DefaultMaxPacketSize is the default M3 reporter max packet size.
	DefaultMaxPacketSize = int32(1440)
	// DefaultHistogramBucketIDName is the default histogram bucket ID tag name
	DefaultHistogramBucketIDName = "bucketid"
	// DefaultHistogramBucketName is the default histogram bucket name tag name
	DefaultHistogramBucketName = "bucket"
	// DefaultHistogramBucketTagPrecision is the default
	// precision to use when formatting the metric tag
	// with the histogram bucket bound values.
	DefaultHistogramBucketTagPrecision = uint(6)

	emitMetricBatchOverhead    = 19
	minMetricBucketIDTagLength = 4
)

// Initialize max vars in init function to avoid lint error.
var (
	maxInt64   int64
	maxFloat64 float64
)

func init() {
	maxInt64 = math.MaxInt64
	maxFloat64 = math.MaxFloat64
}

type metricType int

const (
	counterType metricType = iota + 1
	timerType
	gaugeType
)

var (
	errNoHostPorts   = errors.New("at least one entry for HostPorts is required")
	errCommonTagSize = errors.New("common tags serialized size exceeds packet size")
	errAlreadyClosed = errors.New("reporter already closed")
)

// Reporter is an M3 reporter.
type Reporter interface {
	tally.CachedStatsReporter
	io.Closer
}

// reporter is a metrics backend that reports metrics to a local or
// remote M3 collector, metrics are batched together and emitted
// via either thrift compact or binary protocol in batch UDP packets.
type reporter struct {
	client          *m3thrift.M3Client
	curBatch        *m3thrift.MetricBatch
	curBatchLock    sync.Mutex
	calc            *customtransport.TCalcTransport
	calcProto       thrift.TProtocol
	calcLock        sync.Mutex
	commonTags      map[*m3thrift.MetricTag]bool
	freeBytes       int32
	processors      sync.WaitGroup
	resourcePool    *resourcePool
	bucketIDTagName string
	bucketTagName   string
	bucketValFmt    string

	status reporterStatus
	metCh  chan sizedMetric
}

type reporterStatus struct {
	sync.RWMutex
	closed bool
}

// Options is a set of options for the M3 reporter.
type Options struct {
	HostPorts                   []string
	Service                     string
	Env                         string
	CommonTags                  map[string]string
	IncludeHost                 bool
	Protocol                    Protocol
	MaxQueueSize                int
	MaxPacketSizeBytes          int32
	HistogramBucketIDName       string
	HistogramBucketName         string
	HistogramBucketTagPrecision uint
}

// NewReporter creates a new M3 reporter.
func NewReporter(opts Options) (Reporter, error) {
	if opts.MaxQueueSize <= 0 {
		opts.MaxQueueSize = DefaultMaxQueueSize
	}
	if opts.MaxPacketSizeBytes <= 0 {
		opts.MaxPacketSizeBytes = DefaultMaxPacketSize
	}
	if opts.HistogramBucketIDName == "" {
		opts.HistogramBucketIDName = DefaultHistogramBucketIDName
	}
	if opts.HistogramBucketName == "" {
		opts.HistogramBucketName = DefaultHistogramBucketName
	}
	if opts.HistogramBucketTagPrecision == 0 {
		opts.HistogramBucketTagPrecision = DefaultHistogramBucketTagPrecision
	}

	// Create M3 thrift client
	var trans thrift.TTransport
	var err error
	if len(opts.HostPorts) == 0 {
		err = errNoHostPorts
	} else if len(opts.HostPorts) == 1 {
		trans, err = thriftudp.NewTUDPClientTransport(opts.HostPorts[0], "")
	} else {
		trans, err = thriftudp.NewTMultiUDPClientTransport(opts.HostPorts, "")
	}
	if err != nil {
		return nil, err
	}

	var protocolFactory thrift.TProtocolFactory
	if opts.Protocol == Compact {
		protocolFactory = thrift.NewTCompactProtocolFactory()
	} else {
		protocolFactory = thrift.NewTBinaryProtocolFactoryDefault()
	}

	client := m3thrift.NewM3ClientFactory(trans, protocolFactory)
	resourcePool := newResourcePool(protocolFactory)

	// Create common tags
	tags := resourcePool.getTagList()
	for k, v := range opts.CommonTags {
		tags[createTag(resourcePool, k, v)] = true
	}
	if opts.CommonTags[ServiceTag] == "" {
		if opts.Service == "" {
			return nil, fmt.Errorf("%s common tag is required", ServiceTag)
		}
		tags[createTag(resourcePool, ServiceTag, opts.Service)] = true
	}
	if opts.CommonTags[EnvTag] == "" {
		if opts.Env == "" {
			return nil, fmt.Errorf("%s common tag is required", EnvTag)
		}
		tags[createTag(resourcePool, EnvTag, opts.Env)] = true
	}
	if opts.IncludeHost {
		if opts.CommonTags[HostTag] == "" {
			hostname, err := os.Hostname()
			if err != nil {
				return nil, fmt.Errorf("error resolving host tag: %v", err)
			}
			tags[createTag(resourcePool, HostTag, hostname)] = true
		}
	}

	// Calculate size of common tags
	batch := resourcePool.getBatch()
	batch.CommonTags = tags
	batch.Metrics = []*m3thrift.Metric{}
	proto := resourcePool.getProto()
	batch.Write(proto)
	calc := proto.Transport().(*customtransport.TCalcTransport)
	numOverheadBytes := emitMetricBatchOverhead + calc.GetCount()
	calc.ResetCount()

	freeBytes := opts.MaxPacketSizeBytes - numOverheadBytes
	if freeBytes <= 0 {
		return nil, errCommonTagSize
	}

	r := &reporter{
		client:          client,
		curBatch:        batch,
		calc:            calc,
		calcProto:       proto,
		commonTags:      tags,
		freeBytes:       freeBytes,
		resourcePool:    resourcePool,
		bucketIDTagName: opts.HistogramBucketIDName,
		bucketTagName:   opts.HistogramBucketName,
		bucketValFmt:    "%." + strconv.Itoa(int(opts.HistogramBucketTagPrecision)) + "f",
		metCh:           make(chan sizedMetric, opts.MaxQueueSize),
	}

	r.processors.Add(1)
	go r.process()

	return r, nil
}

// AllocateCounter implements tally.CachedStatsReporter.
func (r *reporter) AllocateCounter(
	name string, tags map[string]string,
) tally.CachedCount {
	return r.allocateCounter(name, tags)
}

func (r *reporter) allocateCounter(
	name string, tags map[string]string,
) cachedMetric {
	counter := r.newMetric(name, tags, counterType)
	size := r.calculateSize(counter)
	return cachedMetric{counter, r, size}
}

// AllocateGauge implements tally.CachedStatsReporter.
func (r *reporter) AllocateGauge(
	name string, tags map[string]string,
) tally.CachedGauge {
	gauge := r.newMetric(name, tags, gaugeType)
	size := r.calculateSize(gauge)
	return cachedMetric{gauge, r, size}
}

// AllocateTimer implements tally.CachedStatsReporter.
func (r *reporter) AllocateTimer(
	name string, tags map[string]string,
) tally.CachedTimer {
	timer := r.newMetric(name, tags, timerType)
	size := r.calculateSize(timer)
	return cachedMetric{timer, r, size}
}

// AllocateHistogram implements tally.CachedStatsReporter.
func (r *reporter) AllocateHistogram(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
) tally.CachedHistogram {
	var (
		cachedValueBuckets    []cachedHistogramBucket
		cachedDurationBuckets []cachedHistogramBucket
	)
	bucketIDLen := len(strconv.Itoa(buckets.Len()))
	bucketIDLen = int(math.Max(float64(bucketIDLen),
		float64(minMetricBucketIDTagLength)))
	bucketIDLenStr := strconv.Itoa(bucketIDLen)
	bucketIDFmt := "%0" + bucketIDLenStr + "d"
	for i, pair := range tally.BucketPairs(buckets) {
		valueTags, durationTags :=
			make(map[string]string), make(map[string]string)
		for k, v := range tags {
			valueTags[k], durationTags[k] = v, v
		}

		idTagValue := fmt.Sprintf(bucketIDFmt, i)

		valueTags[r.bucketIDTagName] = idTagValue
		valueTags[r.bucketTagName] = fmt.Sprintf("%s-%s",
			r.valueBucketString(pair.LowerBoundValue()),
			r.valueBucketString(pair.UpperBoundValue()))

		cachedValueBuckets = append(cachedValueBuckets,
			cachedHistogramBucket{pair.UpperBoundValue(),
				pair.UpperBoundDuration(),
				r.allocateCounter(name, valueTags)})

		durationTags[r.bucketIDTagName] = idTagValue
		durationTags[r.bucketTagName] = fmt.Sprintf("%s-%s",
			r.durationBucketString(pair.LowerBoundDuration()),
			r.durationBucketString(pair.UpperBoundDuration()))

		cachedDurationBuckets = append(cachedDurationBuckets,
			cachedHistogramBucket{pair.UpperBoundValue(),
				pair.UpperBoundDuration(),
				r.allocateCounter(name, durationTags)})
	}
	return cachedHistogram{r, name, tags, buckets,
		cachedValueBuckets, cachedDurationBuckets}
}

func (r *reporter) valueBucketString(v float64) string {
	if v == math.MaxFloat64 {
		return "infinity"
	}
	if v == -math.MaxFloat64 {
		return "-infinity"
	}
	return fmt.Sprintf(r.bucketValFmt, v)
}

func (r *reporter) durationBucketString(d time.Duration) string {
	if d == 0 {
		return "0"
	}
	if d == time.Duration(math.MaxInt64) {
		return "infinity"
	}
	if d == time.Duration(math.MinInt64) {
		return "-infinity"
	}
	return d.String()
}

func (r *reporter) newMetric(
	name string,
	tags map[string]string,
	t metricType,
) *m3thrift.Metric {
	var (
		m      = r.resourcePool.getMetric()
		metVal = r.resourcePool.getValue()
	)
	m.Name = name
	if tags != nil {
		metTags := r.resourcePool.getTagList()
		for k, v := range tags {
			val := v
			metTag := r.resourcePool.getTag()
			metTag.TagName = k
			metTag.TagValue = &val
			metTags[metTag] = true
		}
		m.Tags = metTags
	} else {
		m.Tags = nil
	}
	m.Timestamp = &maxInt64

	switch t {
	case counterType:
		c := r.resourcePool.getCount()
		c.I64Value = &maxInt64
		metVal.Count = c
	case gaugeType:
		g := r.resourcePool.getGauge()
		g.DValue = &maxFloat64
		metVal.Gauge = g
	case timerType:
		t := r.resourcePool.getTimer()
		t.I64Value = &maxInt64
		metVal.Timer = t
	}
	m.MetricValue = metVal

	return m
}

func (r *reporter) calculateSize(m *m3thrift.Metric) int32 {
	r.calcLock.Lock()
	m.Write(r.calcProto)
	size := r.calc.GetCount()
	r.calc.ResetCount()
	r.calcLock.Unlock()
	return size
}

func (r *reporter) reportCopyMetric(
	m *m3thrift.Metric,
	size int32,
	t metricType,
	iValue int64,
	dValue float64,
) {
	copy := r.resourcePool.getMetric()
	copy.Name = m.Name
	copy.Tags = m.Tags
	timestampNano := time.Now().UnixNano()
	copy.Timestamp = &timestampNano
	copy.MetricValue = r.resourcePool.getValue()

	switch t {
	case counterType:
		c := r.resourcePool.getCount()
		c.I64Value = &iValue
		copy.MetricValue.Count = c
	case gaugeType:
		g := r.resourcePool.getGauge()
		g.DValue = &dValue
		copy.MetricValue.Gauge = g
	case timerType:
		t := r.resourcePool.getTimer()
		t.I64Value = &iValue
		copy.MetricValue.Timer = t
	}

	r.status.RLock()
	if !r.status.closed {
		select {
		case r.metCh <- sizedMetric{copy, size}:
		default:
		}
	}
	r.status.RUnlock()
}

// Flush sends an empty sizedMetric to signal a flush.
func (r *reporter) Flush() {
	r.status.RLock()
	if !r.status.closed {
		r.metCh <- sizedMetric{}
	}
	r.status.RUnlock()
}

// Close waits for metrics to be flushed before closing the backend.
func (r *reporter) Close() (err error) {
	r.status.Lock()
	if r.status.closed {
		r.status.Unlock()
		return errAlreadyClosed
	}

	r.status.closed = true
	close(r.metCh)
	r.status.Unlock()

	r.processors.Wait()

	return nil
}

func (r *reporter) Capabilities() tally.Capabilities {
	return r
}

func (r *reporter) Reporting() bool {
	return true
}

func (r *reporter) Tagging() bool {
	return true
}

func (r *reporter) process() {
	mets := make([]*m3thrift.Metric, 0, (r.freeBytes / 10))
	bytes := int32(0)

	for smet := range r.metCh {
		if smet.m == nil {
			// Explicit flush requested
			if len(mets) > 0 {
				mets = r.flush(mets)
				bytes = 0
			}
			continue
		}

		if bytes+smet.size > r.freeBytes {
			mets = r.flush(mets)
			bytes = 0
		}

		mets = append(mets, smet.m)
		bytes += smet.size
	}

	if len(mets) > 0 {
		// Final flush
		r.flush(mets)
	}

	r.processors.Done()
}

func (r *reporter) flush(
	mets []*m3thrift.Metric,
) []*m3thrift.Metric {
	r.curBatchLock.Lock()
	r.curBatch.Metrics = mets
	r.client.EmitMetricBatch(r.curBatch)
	r.curBatch.Metrics = nil
	r.curBatchLock.Unlock()

	r.resourcePool.releaseShallowMetrics(mets)

	for i := range mets {
		mets[i] = nil
	}
	return mets[:0]
}

func createTag(
	pool *resourcePool,
	tagName, tagValue string,
) *m3thrift.MetricTag {
	tag := pool.getTag()
	tag.TagName = tagName
	if tagValue != "" {
		tag.TagValue = &tagValue
	}

	return tag
}

type cachedMetric struct {
	metric   *m3thrift.Metric
	reporter *reporter
	size     int32
}

func (c cachedMetric) ReportCount(value int64) {
	c.reporter.reportCopyMetric(c.metric, c.size, counterType, value, 0)
}

func (c cachedMetric) ReportGauge(value float64) {
	c.reporter.reportCopyMetric(c.metric, c.size, gaugeType, 0, value)
}

func (c cachedMetric) ReportTimer(interval time.Duration) {
	val := int64(interval)
	c.reporter.reportCopyMetric(c.metric, c.size, timerType, val, 0)
}

func (c cachedMetric) ReportSamples(value int64) {
	c.reporter.reportCopyMetric(c.metric, c.size, counterType, value, 0)
}

type noopMetric struct {
}

func (c noopMetric) ReportCount(value int64) {
}

func (c noopMetric) ReportGauge(value float64) {
}

func (c noopMetric) ReportTimer(interval time.Duration) {
}

func (c noopMetric) ReportSamples(value int64) {
}

type cachedHistogram struct {
	r                     *reporter
	name                  string
	tags                  map[string]string
	buckets               tally.Buckets
	cachedValueBuckets    []cachedHistogramBucket
	cachedDurationBuckets []cachedHistogramBucket
}

type cachedHistogramBucket struct {
	valueUpperBound    float64
	durationUpperBound time.Duration
	metric             cachedMetric
}

func (h cachedHistogram) ValueBucket(
	bucketLowerBound, bucketUpperBound float64,
) tally.CachedHistogramBucket {
	for _, b := range h.cachedValueBuckets {
		if b.valueUpperBound >= bucketUpperBound {
			return b.metric
		}
	}
	return noopMetric{}
}

func (h cachedHistogram) DurationBucket(
	bucketLowerBound, bucketUpperBound time.Duration,
) tally.CachedHistogramBucket {
	for _, b := range h.cachedDurationBuckets {
		if b.durationUpperBound >= bucketUpperBound {
			return b.metric
		}
	}
	return noopMetric{}
}

type sizedMetric struct {
	m    *m3thrift.Metric
	size int32
}
