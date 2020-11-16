package lightstep

import (
	"errors"

	"github.com/opentracing/opentracing-go"
)

// PropagatorStack provides a Propagator interface that supports
// multiple propagators per format.
type PropagatorStack struct {
	propagators []Propagator
}

// PushPropagator adds a Propagator to a list of configured propagators
func (stack *PropagatorStack) PushPropagator(p Propagator) {
	stack.propagators = append(stack.propagators, p)
}

// Inject iterates through configured propagators and calls
// their Inject functions
func (stack PropagatorStack) Inject(
	spanContext opentracing.SpanContext,
	opaqueCarrier interface{},
) error {
	if len(stack.propagators) == 0 {
		return errors.New("No valid propagator configured")
	}
	for _, propagator := range stack.propagators {
		propagator.Inject(spanContext, opaqueCarrier)
	}
	return nil
}

// Extract iterates through configured propagators and
// returns the first successfully extracted context
func (stack PropagatorStack) Extract(
	opaqueCarrier interface{},
) (opentracing.SpanContext, error) {
	if len(stack.propagators) == 0 {
		return nil, errors.New("No valid propagator configured")
	}

	for _, propagator := range stack.propagators {
		context, err := propagator.Extract(opaqueCarrier)
		if err == nil {
			return context, nil
		}
	}

	return nil, errors.New("No valid propagator configured")
}
