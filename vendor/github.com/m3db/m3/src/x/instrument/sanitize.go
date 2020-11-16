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

package instrument

import (
	"fmt"
	"strings"

	"github.com/uber-go/tally"
	"github.com/uber-go/tally/m3"
	"github.com/uber-go/tally/prometheus"
)

// MetricSanitizationType is a type of sanitizer to use for metrics.
type MetricSanitizationType int

const (
	// NoMetricSanitization performs no metric sanitization.
	NoMetricSanitization MetricSanitizationType = iota

	// M3MetricSanitization performs M3 metric sanitization.
	M3MetricSanitization

	// PrometheusMetricSanitization performs Prometheus metric sanitization.
	PrometheusMetricSanitization

	// defaultMetricSanitization is the default metric sanitization.
	defaultMetricSanitization = NoMetricSanitization
)

var (
	validMetricSanitizationTypes = []MetricSanitizationType{
		NoMetricSanitization,
		M3MetricSanitization,
		PrometheusMetricSanitization,
	}
)

func (t MetricSanitizationType) String() string {
	switch t {
	case NoMetricSanitization:
		return "none"
	case M3MetricSanitization:
		return "m3"
	case PrometheusMetricSanitization:
		return "prometheus"
	}
	return "unknown"
}

// UnmarshalYAML unmarshals a MetricSanitizationType into a valid type from string.
func (t *MetricSanitizationType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	if str == "" {
		*t = defaultMetricSanitization
		return nil
	}
	strs := make([]string, 0, len(validMetricSanitizationTypes))
	for _, valid := range validMetricSanitizationTypes {
		if str == valid.String() {
			*t = valid
			return nil
		}
		strs = append(strs, "'"+valid.String()+"'")
	}
	return fmt.Errorf("invalid MetricSanitizationType '%s' valid types are: %s",
		str, strings.Join(strs, ", "))
}

// NewOptions returns a new set of sanitization options for the sanitization type.
func (t *MetricSanitizationType) NewOptions() *tally.SanitizeOptions {
	switch *t {
	case NoMetricSanitization:
		return nil
	case M3MetricSanitization:
		return &m3.DefaultSanitizerOpts
	case PrometheusMetricSanitization:
		return &prometheus.DefaultSanitizerOpts
	}
	return nil
}
