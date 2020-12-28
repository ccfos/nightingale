// Copyright (c) 2017 Uber Technologies, Inc.
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

package aggregation

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/m3db/m3/src/metrics/metric"
	"github.com/m3db/m3/src/x/pool"
)

// QuantileTypeStringFn returns the type string for a quantile value.
type QuantileTypeStringFn func(quantile float64) []byte

// TypeStringTransformFn transforms the type string.
type TypeStringTransformFn func(typeString []byte) []byte

// TypesOptions provides a set of options for aggregation types.
type TypesOptions interface {
	// Read-Write methods.

	// SetDefaultCounterAggregationTypes sets the default aggregation types for counters.
	SetDefaultCounterAggregationTypes(value Types) TypesOptions

	// DefaultCounterAggregationTypes returns the default aggregation types for counters.
	DefaultCounterAggregationTypes() Types

	// SetDefaultTimerAggregationTypes sets the default aggregation types for timers.
	SetDefaultTimerAggregationTypes(value Types) TypesOptions

	// DefaultTimerAggregationTypes returns the default aggregation types for timers.
	DefaultTimerAggregationTypes() Types

	// SetDefaultGaugeAggregationTypes sets the default aggregation types for gauges.
	SetDefaultGaugeAggregationTypes(value Types) TypesOptions

	// DefaultGaugeAggregationTypes returns the default aggregation types for gauges.
	DefaultGaugeAggregationTypes() Types

	// SetQuantileTypeStringFn sets the quantile type string function for timers.
	SetQuantileTypeStringFn(value QuantileTypeStringFn) TypesOptions

	// QuantileTypeStringFn returns the quantile type string function for timers.
	QuantileTypeStringFn() QuantileTypeStringFn

	// SetCounterTypeStringTransformFn sets the transformation function for counter type strings.
	SetCounterTypeStringTransformFn(value TypeStringTransformFn) TypesOptions

	// CounterTypeStringTransformFn returns the transformation function for counter type strings.
	CounterTypeStringTransformFn() TypeStringTransformFn

	// SetTimerTypeStringTransformFn sets the transformation function for timer type strings.
	SetTimerTypeStringTransformFn(value TypeStringTransformFn) TypesOptions

	// TimerTypeStringTransformFn returns the transformation function for timer type strings.
	TimerTypeStringTransformFn() TypeStringTransformFn

	// SetGaugeTypeStringTransformFn sets the transformation function for gauge type strings.
	SetGaugeTypeStringTransformFn(value TypeStringTransformFn) TypesOptions

	// GaugeTypeStringTransformFn returns the transformation function for gauge type strings.
	GaugeTypeStringTransformFn() TypeStringTransformFn

	// SetTypesPool sets the aggregation types pool.
	SetTypesPool(pool TypesPool) TypesOptions

	// TypesPool returns the aggregation types pool.
	TypesPool() TypesPool

	// SetQuantilesPool sets the timer quantiles pool.
	SetQuantilesPool(pool pool.FloatsPool) TypesOptions

	// QuantilesPool returns the timer quantiles pool.
	QuantilesPool() pool.FloatsPool

	// Read only methods.

	// TypeStringForCounter returns the type string for the aggregation type for counters.
	TypeStringForCounter(value Type) []byte

	// TypeStringForTimer returns the type string for the aggregation type for timers.
	TypeStringForTimer(value Type) []byte

	// TypeStringForGauge returns the type string for the aggregation type for gauges.
	TypeStringForGauge(value Type) []byte

	// TypeForCounter returns the aggregation type for given counter type string.
	TypeForCounter(value []byte) Type

	// TypeForTimer returns the aggregation type for given timer type string.
	TypeForTimer(value []byte) Type

	// TypeForGauge returns the aggregation type for given gauge type string.
	TypeForGauge(value []byte) Type

	// Quantiles returns the quantiles for timers.
	Quantiles() []float64

	// IsContainedInDefaultAggregationTypes checks if the given aggregation type is
	// contained in the default aggregation types for the metric type.
	IsContainedInDefaultAggregationTypes(at Type, mt metric.Type) bool
}

var (
	defaultDefaultCounterAggregationTypes = Types{
		Sum,
	}
	defaultDefaultTimerAggregationTypes = Types{
		Sum,
		SumSq,
		Mean,
		Min,
		Max,
		Count,
		Stdev,
		Median,
		P50,
		P95,
		P99,
	}
	defaultDefaultGaugeAggregationTypes = Types{
		Last,
	}
	defaultTypeStringsMap = map[Type][]byte{
		Last:   []byte("last"),
		Sum:    []byte("sum"),
		SumSq:  []byte("sum_sq"),
		Mean:   []byte("mean"),
		Min:    []byte("lower"),
		Max:    []byte("upper"),
		Count:  []byte("count"),
		Stdev:  []byte("stdev"),
		Median: []byte("median"),
	}
)

type options struct {
	defaultCounterAggregationTypes Types
	defaultTimerAggregationTypes   Types
	defaultGaugeAggregationTypes   Types
	quantileTypeStringFn           QuantileTypeStringFn
	counterTypeStringTransformFn   TypeStringTransformFn
	timerTypeStringTransformFn     TypeStringTransformFn
	gaugeTypeStringTransformFn     TypeStringTransformFn
	aggTypesPool                   TypesPool
	quantilesPool                  pool.FloatsPool

	counterTypeStrings [][]byte
	timerTypeStrings   [][]byte
	gaugeTypeStrings   [][]byte
	quantiles          []float64
}

// NewTypesOptions returns a default TypesOptions.
func NewTypesOptions() TypesOptions {
	o := &options{
		defaultCounterAggregationTypes: defaultDefaultCounterAggregationTypes,
		defaultGaugeAggregationTypes:   defaultDefaultGaugeAggregationTypes,
		defaultTimerAggregationTypes:   defaultDefaultTimerAggregationTypes,
		quantileTypeStringFn:           defaultQuantileTypeStringFn,
		counterTypeStringTransformFn:   NoOpTransform,
		timerTypeStringTransformFn:     NoOpTransform,
		gaugeTypeStringTransformFn:     NoOpTransform,
	}
	o.initPools()
	o.computeAllDerived()
	return o
}

func (o *options) initPools() {
	o.aggTypesPool = NewTypesPool(nil)
	o.aggTypesPool.Init(func() Types {
		return make(Types, 0, len(ValidTypes))
	})

	o.quantilesPool = pool.NewFloatsPool(nil, nil)
	o.quantilesPool.Init()
}

func (o *options) SetDefaultCounterAggregationTypes(aggTypes Types) TypesOptions {
	opts := *o
	opts.defaultCounterAggregationTypes = aggTypes
	opts.computeAllDerived()
	return &opts
}

func (o *options) DefaultCounterAggregationTypes() Types {
	return o.defaultCounterAggregationTypes
}

func (o *options) SetDefaultTimerAggregationTypes(aggTypes Types) TypesOptions {
	opts := *o
	opts.defaultTimerAggregationTypes = aggTypes
	opts.computeAllDerived()
	return &opts
}

func (o *options) DefaultTimerAggregationTypes() Types {
	return o.defaultTimerAggregationTypes
}

func (o *options) SetDefaultGaugeAggregationTypes(aggTypes Types) TypesOptions {
	opts := *o
	opts.defaultGaugeAggregationTypes = aggTypes
	opts.computeAllDerived()
	return &opts
}

func (o *options) DefaultGaugeAggregationTypes() Types {
	return o.defaultGaugeAggregationTypes
}

func (o *options) SetQuantileTypeStringFn(value QuantileTypeStringFn) TypesOptions {
	opts := *o
	opts.quantileTypeStringFn = value
	opts.computeAllDerived()
	return &opts
}

func (o *options) QuantileTypeStringFn() QuantileTypeStringFn {
	return o.quantileTypeStringFn
}

func (o *options) SetCounterTypeStringTransformFn(value TypeStringTransformFn) TypesOptions {
	opts := *o
	opts.counterTypeStringTransformFn = value
	opts.computeAllDerived()
	return &opts
}

func (o *options) CounterTypeStringTransformFn() TypeStringTransformFn {
	return o.counterTypeStringTransformFn
}

func (o *options) SetTimerTypeStringTransformFn(value TypeStringTransformFn) TypesOptions {
	opts := *o
	opts.timerTypeStringTransformFn = value
	opts.computeAllDerived()
	return &opts
}

func (o *options) TimerTypeStringTransformFn() TypeStringTransformFn {
	return o.timerTypeStringTransformFn
}

func (o *options) SetGaugeTypeStringTransformFn(value TypeStringTransformFn) TypesOptions {
	opts := *o
	opts.gaugeTypeStringTransformFn = value
	opts.computeAllDerived()
	return &opts
}

func (o *options) GaugeTypeStringTransformFn() TypeStringTransformFn {
	return o.gaugeTypeStringTransformFn
}

func (o *options) SetTypesPool(pool TypesPool) TypesOptions {
	opts := *o
	opts.aggTypesPool = pool
	return &opts
}

func (o *options) TypesPool() TypesPool {
	return o.aggTypesPool
}

func (o *options) SetQuantilesPool(pool pool.FloatsPool) TypesOptions {
	opts := *o
	opts.quantilesPool = pool
	return &opts
}

func (o *options) QuantilesPool() pool.FloatsPool {
	return o.quantilesPool
}

func (o *options) TypeStringForCounter(aggType Type) []byte {
	return o.counterTypeStrings[aggType.ID()]
}

func (o *options) TypeStringForTimer(aggType Type) []byte {
	return o.timerTypeStrings[aggType.ID()]
}

func (o *options) TypeStringForGauge(aggType Type) []byte {
	return o.gaugeTypeStrings[aggType.ID()]
}

func (o *options) TypeForCounter(value []byte) Type {
	return typeFor(value, o.counterTypeStrings)
}

func (o *options) TypeForTimer(value []byte) Type {
	return typeFor(value, o.timerTypeStrings)
}

func (o *options) TypeForGauge(value []byte) Type {
	return typeFor(value, o.gaugeTypeStrings)
}

func (o *options) Quantiles() []float64 {
	return o.quantiles
}

func (o *options) IsContainedInDefaultAggregationTypes(at Type, mt metric.Type) bool {
	var aggTypes Types
	switch mt {
	case metric.CounterType:
		aggTypes = o.DefaultCounterAggregationTypes()
	case metric.GaugeType:
		aggTypes = o.DefaultGaugeAggregationTypes()
	case metric.TimerType:
		aggTypes = o.DefaultTimerAggregationTypes()
	}
	return aggTypes.Contains(at)
}

func (o *options) computeAllDerived() {
	o.computeQuantiles()
	o.computeCounterTypeStrings()
	o.computeTimerTypeStrings()
	o.computeGaugeTypeStrings()
}

func (o *options) computeQuantiles() {
	o.quantiles, _ = o.DefaultTimerAggregationTypes().PooledQuantiles(o.QuantilesPool())
}

func (o *options) computeCounterTypeStrings() {
	o.counterTypeStrings = o.computeTypeStrings(o.counterTypeStringTransformFn)
}

func (o *options) computeTimerTypeStrings() {
	o.timerTypeStrings = o.computeTypeStrings(o.timerTypeStringTransformFn)
}

func (o *options) computeGaugeTypeStrings() {
	o.gaugeTypeStrings = o.computeTypeStrings(o.gaugeTypeStringTransformFn)
}

func (o *options) computeTypeStrings(transformFn TypeStringTransformFn) [][]byte {
	res := make([][]byte, maxTypeID+1)
	for aggType := range ValidTypes {
		var typeString []byte
		if typeStr, exist := defaultTypeStringsMap[aggType]; exist {
			typeString = typeStr
		} else {
			q, ok := aggType.Quantile()
			if ok {
				typeString = o.quantileTypeStringFn(q)
			}
		}
		transformed := transformFn(typeString)
		res[aggType.ID()] = transformed
	}
	return res
}

func typeFor(value []byte, typeStrings [][]byte) Type {
	for id, typeString := range typeStrings {
		if !bytes.Equal(value, typeString) {
			continue
		}
		if t := Type(id); t.IsValid() {
			return t
		}
	}
	return UnknownType
}

// By default we use e.g. "p50", "p95", "p99" for the 50th/95th/99th percentile.
func defaultQuantileTypeStringFn(quantile float64) []byte {
	str := strconv.FormatFloat(quantile*100, 'f', -1, 64)
	idx := strings.Index(str, ".")
	if idx != -1 {
		str = str[:idx] + str[idx+1:]
	}
	return []byte("p" + str)
}

// NoOpTransform returns the input byte slice as is.
func NoOpTransform(b []byte) []byte { return b }

// EmptyTransform transforms the input byte slice to an empty byte slice.
func EmptyTransform(b []byte) []byte { return nil }

// SuffixTransform transforms the input byte slice to a suffix by prepending
// a dot at the beginning.
func SuffixTransform(b []byte) []byte { return append([]byte("."), b...) }
