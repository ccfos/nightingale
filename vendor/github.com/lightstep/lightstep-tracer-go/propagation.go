package lightstep

import "github.com/opentracing/opentracing-go"

// Propagator provides the ability to inject/extract different
// formats of span information. Currently supported: ls, b3
type Propagator interface {
	Inject(opentracing.SpanContext, interface{}) error
	Extract(interface{}) (opentracing.SpanContext, error)
}
