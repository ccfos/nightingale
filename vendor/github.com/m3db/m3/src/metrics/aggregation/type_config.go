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
	"fmt"

	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/pool"
)

// TypesConfiguration contains configuration for aggregation types.
type TypesConfiguration struct {
	// Default aggregation types for counter metrics.
	DefaultCounterAggregationTypes *Types `yaml:"defaultCounterAggregationTypes"`

	// Default aggregation types for timer metrics.
	DefaultTimerAggregationTypes *Types `yaml:"defaultTimerAggregationTypes"`

	// Default aggregation types for gauge metrics.
	DefaultGaugeAggregationTypes *Types `yaml:"defaultGaugeAggregationTypes"`

	// CounterTransformFnType configures the type string transformation function for counters.
	CounterTransformFnType *transformFnType `yaml:"counterTransformFnType"`

	// TimerTransformFnType configures the type string transformation function for timers.
	TimerTransformFnType *transformFnType `yaml:"timerTransformFnType"`

	// GaugeTransformFnType configures the type string transformation function for gauges.
	GaugeTransformFnType *transformFnType `yaml:"gaugeTransformFnType"`

	// Pool of aggregation types.
	AggregationTypesPool pool.ObjectPoolConfiguration `yaml:"aggregationTypesPool"`

	// Pool of quantile slices.
	QuantilesPool pool.BucketizedPoolConfiguration `yaml:"quantilesPool"`
}

// NewOptions creates a new Option.
func (c TypesConfiguration) NewOptions(instrumentOpts instrument.Options) (TypesOptions, error) {
	opts := NewTypesOptions()
	if c.DefaultCounterAggregationTypes != nil {
		opts = opts.SetDefaultCounterAggregationTypes(*c.DefaultCounterAggregationTypes)
	}
	if c.DefaultGaugeAggregationTypes != nil {
		opts = opts.SetDefaultGaugeAggregationTypes(*c.DefaultGaugeAggregationTypes)
	}
	if c.DefaultTimerAggregationTypes != nil {
		opts = opts.SetDefaultTimerAggregationTypes(*c.DefaultTimerAggregationTypes)
	}
	if c.CounterTransformFnType != nil {
		fn, err := c.CounterTransformFnType.TransformFn()
		if err != nil {
			return nil, err
		}
		opts = opts.SetCounterTypeStringTransformFn(fn)
	}
	if c.TimerTransformFnType != nil {
		fn, err := c.TimerTransformFnType.TransformFn()
		if err != nil {
			return nil, err
		}
		opts = opts.SetTimerTypeStringTransformFn(fn)
	}
	if c.GaugeTransformFnType != nil {
		fn, err := c.GaugeTransformFnType.TransformFn()
		if err != nil {
			return nil, err
		}
		opts = opts.SetGaugeTypeStringTransformFn(fn)
	}

	// Set aggregation types pool.
	scope := instrumentOpts.MetricsScope()
	iOpts := instrumentOpts.SetMetricsScope(scope.SubScope("aggregation-types-pool"))
	aggTypesPoolOpts := c.AggregationTypesPool.NewObjectPoolOptions(iOpts)
	aggTypesPool := NewTypesPool(aggTypesPoolOpts)
	opts = opts.SetTypesPool(aggTypesPool)
	aggTypesPool.Init(func() Types {
		return make(Types, 0, len(ValidTypes))
	})

	// Set quantiles pool.
	iOpts = instrumentOpts.SetMetricsScope(scope.SubScope("quantile-pool"))
	quantilesPool := pool.NewFloatsPool(
		c.QuantilesPool.NewBuckets(),
		c.QuantilesPool.NewObjectPoolOptions(iOpts),
	)
	opts = opts.SetQuantilesPool(quantilesPool)
	quantilesPool.Init()

	return opts, nil
}

type transformFnType string

var (
	noopTransformType   transformFnType = "noop"
	emptyTransformType  transformFnType = "empty"
	suffixTransformType transformFnType = "suffix"

	validTypes = []transformFnType{
		noopTransformType,
		emptyTransformType,
		suffixTransformType,
	}
)

func (t *transformFnType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	var validStrings []string
	for _, validType := range validTypes {
		validString := string(validType)
		if validString == str {
			*t = validType
			return nil
		}
		validStrings = append(validStrings, validString)
	}

	return fmt.Errorf("invalid transform type %s, valid types are: %v", str, validStrings)
}

func (t transformFnType) TransformFn() (TypeStringTransformFn, error) {
	switch t {
	case noopTransformType:
		return NoOpTransform, nil
	case emptyTransformType:
		return EmptyTransform, nil
	case suffixTransformType:
		return SuffixTransform, nil
	default:
		return nil, fmt.Errorf("invalid type string transform function type: %s", string(t))
	}
}
