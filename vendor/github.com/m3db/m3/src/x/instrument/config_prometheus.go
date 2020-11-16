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

package instrument

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	prom "github.com/m3db/prometheus_client_golang/prometheus"
	"github.com/m3db/prometheus_client_golang/prometheus/promhttp"
	dto "github.com/m3db/prometheus_client_model/go"
	extprom "github.com/prometheus/client_golang/prometheus"
	"github.com/uber-go/tally/prometheus"
)

// PrometheusConfiguration is a configuration for a Prometheus reporter.
type PrometheusConfiguration struct {
	// HandlerPath if specified will be used instead of using the default
	// HTTP handler path "/metrics".
	HandlerPath string `yaml:"handlerPath"`

	// ListenAddress if specified will be used instead of just registering the
	// handler on the default HTTP serve mux without listening.
	ListenAddress string `yaml:"listenAddress"`

	// TimerType is the default Prometheus type to use for Tally timers.
	TimerType string `yaml:"timerType"`

	// DefaultHistogramBuckets if specified will set the default histogram
	// buckets to be used by the reporter.
	DefaultHistogramBuckets []prometheus.HistogramObjective `yaml:"defaultHistogramBuckets"`

	// DefaultSummaryObjectives if specified will set the default summary
	// objectives to be used by the reporter.
	DefaultSummaryObjectives []prometheus.SummaryObjective `yaml:"defaultSummaryObjectives"`

	// OnError specifies what to do when an error either with listening
	// on the specified listen address or registering a metric with the
	// Prometheus. By default the registerer will panic.
	OnError string `yaml:"onError"`
}

// HistogramObjective is a Prometheus histogram bucket.
// See: https://godoc.org/github.com/prometheus/client_golang/prometheus#HistogramOpts
type HistogramObjective struct {
	Upper float64 `yaml:"upper"`
}

// SummaryObjective is a Prometheus summary objective.
// See: https://godoc.org/github.com/prometheus/client_golang/prometheus#SummaryOpts
type SummaryObjective struct {
	Percentile   float64 `yaml:"percentile"`
	AllowedError float64 `yaml:"allowedError"`
}

// PrometheusConfigurationOptions allows some programatic options, such as using a
// specific registry and what error callback to register.
type PrometheusConfigurationOptions struct {
	// Registry if not nil will specify the specific registry to use
	// for registering metrics.
	Registry *prom.Registry
	// ExternalRegistries if set (with combination of a specified Registry)
	// will also
	ExternalRegistries []PrometheusExternalRegistry
	// HandlerListener is the listener to register the server handler on.
	HandlerListener net.Listener
	// HandlerOpts is the reporter HTTP handler options, not specifying will
	// use defaults.
	HandlerOpts promhttp.HandlerOpts
	// OnError allows for customization of what to do when a metric
	// registration error fails, the default is to panic.
	OnError func(e error)
}

// PrometheusExternalRegistry is an external Prometheus registry
// to also expose as part of the handler.
type PrometheusExternalRegistry struct {
	// Registry is the external prometheus registry to list.
	Registry *extprom.Registry
	// SubScope will add a prefix to all metric names exported by
	// this registry.
	SubScope string
}

// NewReporter creates a new M3 Prometheus reporter from this configuration.
func (c PrometheusConfiguration) NewReporter(
	configOpts PrometheusConfigurationOptions,
) (prometheus.Reporter, error) {
	var opts prometheus.Options

	if configOpts.Registry != nil {
		opts.Registerer = configOpts.Registry
	}

	if configOpts.OnError != nil {
		opts.OnRegisterError = configOpts.OnError
	} else {
		switch c.OnError {
		case "stderr":
			opts.OnRegisterError = func(err error) {
				fmt.Fprintf(os.Stderr, "tally prometheus reporter error: %v\n", err)
			}
		case "log":
			opts.OnRegisterError = func(err error) {
				log.Printf("tally prometheus reporter error: %v\n", err)
			}
		case "none":
			opts.OnRegisterError = func(err error) {}
		default:
			opts.OnRegisterError = func(err error) {
				panic(err)
			}
		}
	}

	switch c.TimerType {
	case "summary":
		opts.DefaultTimerType = prometheus.SummaryTimerType
	case "histogram":
		opts.DefaultTimerType = prometheus.HistogramTimerType
	}

	if len(c.DefaultHistogramBuckets) > 0 {
		values := make([]float64, 0, len(c.DefaultHistogramBuckets))
		for _, value := range c.DefaultHistogramBuckets {
			values = append(values, value.Upper)
		}
		opts.DefaultHistogramBuckets = values
	}

	if len(c.DefaultSummaryObjectives) > 0 {
		values := make(map[float64]float64, len(c.DefaultSummaryObjectives))
		for _, value := range c.DefaultSummaryObjectives {
			values[value.Percentile] = value.AllowedError
		}
		opts.DefaultSummaryObjectives = values
	}

	reporter := prometheus.NewReporter(opts)

	path := "/metrics"
	if handlerPath := strings.TrimSpace(c.HandlerPath); handlerPath != "" {
		path = handlerPath
	}

	handler := reporter.HTTPHandler()
	if configOpts.Registry != nil {
		gatherer := newMultiGatherer(configOpts.Registry, configOpts.ExternalRegistries)
		handler = promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
	}

	addr := strings.TrimSpace(c.ListenAddress)
	if addr == "" && configOpts.HandlerListener == nil {
		// If address not specified and server not specified, register
		// on default mux.
		http.Handle(path, handler)
	} else {
		mux := http.NewServeMux()
		mux.Handle(path, handler)

		listener := configOpts.HandlerListener
		if listener == nil {
			// Address must be specified if server was nil.
			var err error
			listener, err = net.Listen("tcp", addr)
			if err != nil {
				return nil, fmt.Errorf(
					"prometheus handler listen address error: %v", err)
			}
		}

		go func() {
			server := &http.Server{Handler: mux}
			if err := server.Serve(listener); err != nil {
				opts.OnRegisterError(err)
			}
		}()
	}

	return reporter, nil
}

func newMultiGatherer(
	primary *prom.Registry,
	ext []PrometheusExternalRegistry,
) prom.Gatherer {
	return &multiGatherer{
		primary: primary,
		ext:     ext,
	}
}

var _ prom.Gatherer = (*multiGatherer)(nil)

type multiGatherer struct {
	primary *prom.Registry
	ext     []PrometheusExternalRegistry
}

func (g *multiGatherer) Gather() ([]*dto.MetricFamily, error) {
	results, err := g.primary.Gather()
	if err != nil {
		return nil, err
	}

	if len(g.ext) == 0 {
		return results, nil
	}

	for _, secondary := range g.ext {
		gathered, err := secondary.Registry.Gather()
		if err != nil {
			return nil, err
		}

		for _, elem := range gathered {
			entry := &dto.MetricFamily{
				Name:   elem.Name,
				Help:   elem.Help,
				Metric: make([]*dto.Metric, 0, len(elem.Metric)),
			}

			if secondary.SubScope != "" && entry.Name != nil {
				scopedName := fmt.Sprintf("%s_%s", secondary.SubScope, *entry.Name)
				entry.Name = &scopedName
			}

			if v := elem.Type; v != nil {
				metricType := dto.MetricType(*v)
				entry.Type = &metricType
			}

			for _, metricElem := range elem.Metric {
				metricEntry := &dto.Metric{
					Label:       make([]*dto.LabelPair, 0, len(metricElem.Label)),
					TimestampMs: metricElem.TimestampMs,
				}

				if v := metricElem.Gauge; v != nil {
					metricEntry.Gauge = &dto.Gauge{
						Value: v.Value,
					}
				}

				if v := metricElem.Counter; v != nil {
					metricEntry.Counter = &dto.Counter{
						Value: v.Value,
					}
				}

				if v := metricElem.Summary; v != nil {
					metricEntry.Summary = &dto.Summary{
						SampleCount: v.SampleCount,
						SampleSum:   v.SampleSum,
						Quantile:    make([]*dto.Quantile, 0, len(v.Quantile)),
					}

					for _, quantileElem := range v.Quantile {
						quantileEntry := &dto.Quantile{
							Quantile: quantileElem.Quantile,
							Value:    quantileElem.Value,
						}
						metricEntry.Summary.Quantile =
							append(metricEntry.Summary.Quantile, quantileEntry)
					}
				}

				if v := metricElem.Untyped; v != nil {
					metricEntry.Untyped = &dto.Untyped{
						Value: v.Value,
					}
				}

				if v := metricElem.Histogram; v != nil {
					metricEntry.Histogram = &dto.Histogram{
						SampleCount: v.SampleCount,
						SampleSum:   v.SampleSum,
						Bucket:      make([]*dto.Bucket, 0, len(v.Bucket)),
					}

					for _, bucketElem := range v.Bucket {
						bucketEntry := &dto.Bucket{
							CumulativeCount: bucketElem.CumulativeCount,
							UpperBound:      bucketElem.UpperBound,
						}
						metricEntry.Histogram.Bucket =
							append(metricEntry.Histogram.Bucket, bucketEntry)
					}
				}

				for _, labelElem := range metricElem.Label {
					labelEntry := &dto.LabelPair{
						Name:  labelElem.Name,
						Value: labelElem.Value,
					}

					metricEntry.Label = append(metricEntry.Label, labelEntry)
				}

				entry.Metric = append(entry.Metric, metricEntry)
			}

			results = append(results, entry)
		}
	}

	return results, nil
}
