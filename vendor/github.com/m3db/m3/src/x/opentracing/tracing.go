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

package opentracing

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/m3db/m3/src/x/instrument"

	lightstep "github.com/lightstep/lightstep-tracer-go"
	"github.com/opentracing/opentracing-go"
	"github.com/uber-go/tally"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerzap "github.com/uber/jaeger-client-go/log/zap"
	jaegertally "github.com/uber/jaeger-lib/metrics/tally"
	"go.uber.org/zap"
)

const (
	spanTagBuildRevision  = "build.revision"
	spanTagBuildVersion   = "build.version"
	spanTagBuildBranch    = "build.branch"
	spanTagBuildDate      = "build.date"
	spanTagBuildTimeUnix  = "build.time_unix"
	spanTagBuildGoVersion = "build.go_version"
)

var (
	// TracingBackendJaeger indicates the Jaeger backend should be used.
	TracingBackendJaeger = "jaeger"
	// TracingBackendLightstep indices the LightStep backend should be used.
	TracingBackendLightstep = "lightstep"

	supportedBackends = []string{
		TracingBackendJaeger,
		TracingBackendLightstep,
	}

	tracerSpanTags = map[string]string{
		spanTagBuildRevision:  instrument.Revision,
		spanTagBuildBranch:    instrument.Branch,
		spanTagBuildVersion:   instrument.Version,
		spanTagBuildDate:      instrument.BuildDate,
		spanTagBuildTimeUnix:  instrument.BuildTimeUnix,
		spanTagBuildGoVersion: runtime.Version(),
	}
)

// TracingConfiguration configures an opentracing backend for m3query to use. Currently only jaeger is supported.
// Tracing is disabled if no backend is specified.
type TracingConfiguration struct {
	ServiceName string                  `yaml:"serviceName"`
	Backend     string                  `yaml:"backend"`
	Jaeger      jaegercfg.Configuration `yaml:"jaeger"`
	Lightstep   lightstep.Options       `yaml:"lightstep"`
}

// NewTracer returns a tracer configured with the configuration provided by this struct. The tracer's concrete
// type is determined by cfg.Backend. Currently only `"jaeger"` is supported. `""` implies
// disabled (NoopTracer).
func (cfg *TracingConfiguration) NewTracer(defaultServiceName string, scope tally.Scope, logger *zap.Logger) (opentracing.Tracer, io.Closer, error) {
	switch cfg.Backend {
	case "":
		return opentracing.NoopTracer{}, noopCloser{}, nil

	case TracingBackendJaeger:
		logger.Info("initializing Jaeger tracer")
		return cfg.newJaegerTracer(defaultServiceName, scope, logger)

	case TracingBackendLightstep:
		logger.Info("initializing LightStep tracer")
		return cfg.newLightstepTracer(defaultServiceName)

	default:
		return nil, nil, fmt.Errorf("unknown tracing backend: %s. Supported backends are: [%s]",
			cfg.Backend,
			strings.Join(supportedBackends, ","))
	}
}

func (cfg *TracingConfiguration) newJaegerTracer(defaultServiceName string, scope tally.Scope, logger *zap.Logger) (opentracing.Tracer, io.Closer, error) {
	if cfg.Jaeger.ServiceName == "" {
		cfg.Jaeger.ServiceName = defaultServiceName
	}

	for k, v := range tracerSpanTags {
		cfg.Jaeger.Tags = append(cfg.Jaeger.Tags, opentracing.Tag{
			Key:   k,
			Value: v,
		})
	}

	tracer, jaegerCloser, err := cfg.Jaeger.NewTracer(
		jaegercfg.Logger(jaegerzap.NewLogger(logger)),
		jaegercfg.Metrics(jaegertally.Wrap(scope)))

	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize jaeger: %s", err.Error())
	}

	return tracer, jaegerCloser, nil
}

func (cfg *TracingConfiguration) newLightstepTracer(serviceName string) (opentracing.Tracer, io.Closer, error) {
	if cfg.Lightstep.Tags == nil {
		cfg.Lightstep.Tags = opentracing.Tags{}
	}

	tags := cfg.Lightstep.Tags
	if _, ok := tags[lightstep.ComponentNameKey]; !ok {
		tags[lightstep.ComponentNameKey] = serviceName
	}

	for k, v := range tracerSpanTags {
		tags[k] = v
	}

	tracer, err := lightstep.CreateTracer(cfg.Lightstep)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create lightstep tracer: %v", err)
	}

	closer := &lightstepCloser{tracer: tracer}
	return tracer, closer, nil
}

type lightstepCloser struct {
	tracer lightstep.Tracer
}

func (l *lightstepCloser) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	l.tracer.Close(ctx)
	cancel()
	return ctx.Err()
}

type noopCloser struct{}

func (noopCloser) Close() error {
	return nil
}
