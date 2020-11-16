// Package lightstep implements the LightStep OpenTracing client for Go.
package lightstep

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
)

// Tracer extends the `opentracing.Tracer` interface with methods for manual
// flushing and closing. To access these methods, you can take the global
// tracer and typecast it to a `lightstep.Tracer`. As a convenience, the
// lightstep package provides static functions which perform the typecasting.
type Tracer interface {
	opentracing.Tracer

	// Close flushes and then terminates the LightStep collector
	Close(context.Context)
	// Flush sends all spans currently in the buffer to the LighStep collector
	Flush(context.Context)
	// Options gets the Options used in New() or NewWithOptions().
	Options() Options
	// Disable prevents the tracer from recording spans or flushing
	Disable()
}

// Implements the `Tracer` interface. Buffers spans and forwards to a Lightstep collector.
type tracerImpl struct {
	//////////////////////////////////////////////////////////////
	// IMMUTABLE IMMUTABLE IMMUTABLE IMMUTABLE IMMUTABLE IMMUTABLE
	//////////////////////////////////////////////////////////////

	// Note: there may be a desire to update some of these fields
	// at runtime, in which case suitable changes may be needed
	// for variables accessed during Flush.

	reporterID uint64 // the LightStep tracer guid
	opts       Options

	// report loop management
	closeOnce               sync.Once
	closeReportLoopChannel  chan struct{}
	reportLoopClosedChannel chan struct{}

	converter   *protoConverter
	accessToken string
	attributes  map[string]string

	//////////////////////////////////////////////////////////
	// MUTABLE MUTABLE MUTABLE MUTABLE MUTABLE MUTABLE MUTABLE
	//////////////////////////////////////////////////////////

	// the following fields are modified under `lock`.
	lock sync.Mutex

	// Remote service that will receive reports.
	client     collectorClient
	connection Connection

	// Two buffers of data.
	buffer   reportBuffer
	flushing reportBuffer

	// Flush state.
	flushingLock      sync.Mutex
	reportInFlight    bool
	lastReportAttempt time.Time

	// Meta Event Reporting can be enabled at tracer creation or on-demand by satellite
	metaEventReportingEnabled bool
	// Set to true on first report
	firstReportHasRun bool

	// We allow our remote peer to disable this instrumentation at any
	// time, turning all potentially costly runtime operations into
	// no-ops.
	//
	// TODO this should use atomic load/store to test disabled
	// prior to taking the lock, do please.
	disabled bool

	// Map of propagators used to determine the correct propagator to use
	// based on the format passed into Inject/Extract. Supports one
	// propagator for each of the formats: TextMap, HTTPHeaders, Binary
	propagators map[opentracing.BuiltinFormat]Propagator
}

// NewTracer creates and starts a new Lightstep Tracer.
// In case of error, we emit event and return nil.
func NewTracer(opts Options) Tracer {
	tr, err := CreateTracer(opts)
	if err != nil {
		emitEvent(newEventStartError(err))
		return nil
	}
	return tr
}

// CreateTracer creates and starts a new Lightstep Tracer.
// It is meant to replace NewTracer which does not propagate the error.
func CreateTracer(opts Options) (Tracer, error) {
	if err := opts.Initialize(); err != nil {
		return nil, fmt.Errorf("init; err: %v", err)
	}

	attributes := map[string]string{}
	for k, v := range opts.Tags {
		attributes[k] = fmt.Sprint(v)
	}
	// Don't let the GrpcOptions override these values. That would be confusing.
	attributes[TracerPlatformKey] = TracerPlatformValue
	attributes[TracerPlatformVersionKey] = runtime.Version()
	attributes[TracerVersionKey] = TracerVersionValue

	now := time.Now()
	impl := &tracerImpl{
		opts:                    opts,
		reporterID:              genSeededGUID(),
		buffer:                  newSpansBuffer(opts.MaxBufferedSpans),
		flushing:                newSpansBuffer(opts.MaxBufferedSpans),
		closeReportLoopChannel:  make(chan struct{}),
		reportLoopClosedChannel: make(chan struct{}),
		converter:               newProtoConverter(opts),
		accessToken:             opts.AccessToken,
		attributes:              attributes,
	}

	impl.buffer.setCurrent(now)

	var err error
	impl.client, err = newCollectorClient(opts)
	if err != nil {
		return nil, fmt.Errorf("create collector client; err: %v", err)
	}

	conn, err := impl.client.ConnectClient()
	if err != nil {
		return nil, err
	}
	impl.connection = conn

	// set meta reporting to defined option
	impl.metaEventReportingEnabled = opts.MetaEventReportingEnabled
	impl.firstReportHasRun = false

	go impl.reportLoop()

	impl.propagators = map[opentracing.BuiltinFormat]Propagator{
		opentracing.TextMap:     LightStepPropagator,
		opentracing.HTTPHeaders: LightStepPropagator,
		opentracing.Binary:      BinaryPropagator,
	}
	for builtin, propagator := range opts.Propagators {
		impl.propagators[builtin] = propagator
	}

	return impl, nil
}

func (tracer *tracerImpl) Options() Options {
	return tracer.opts
}

func (tracer *tracerImpl) StartSpan(
	operationName string,
	sso ...opentracing.StartSpanOption,
) opentracing.Span {
	return newSpan(operationName, tracer, sso)
}

func (tracer *tracerImpl) Inject(sc opentracing.SpanContext, format interface{}, carrier interface{}) error {
	if tracer.opts.MetaEventReportingEnabled {
		opentracing.StartSpan(LSMetaEvent_InjectOperation,
			opentracing.Tag{Key: LSMetaEvent_MetaEventKey, Value: true},
			opentracing.Tag{Key: LSMetaEvent_TraceIdKey, Value: sc.(SpanContext).TraceID},
			opentracing.Tag{Key: LSMetaEvent_SpanIdKey, Value: sc.(SpanContext).SpanID},
			opentracing.Tag{Key: LSMetaEvent_PropagationFormatKey, Value: format}).
			Finish()
	}

	builtin, ok := format.(opentracing.BuiltinFormat)
	if !ok {
		return opentracing.ErrUnsupportedFormat
	}
	return tracer.propagators[builtin].Inject(sc, carrier)
}

func (tracer *tracerImpl) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	if tracer.opts.MetaEventReportingEnabled {
		opentracing.StartSpan(LSMetaEvent_ExtractOperation,
			opentracing.Tag{Key: LSMetaEvent_MetaEventKey, Value: true},
			opentracing.Tag{Key: LSMetaEvent_PropagationFormatKey, Value: format}).
			Finish()
	}
	builtin, ok := format.(opentracing.BuiltinFormat)
	if !ok {
		return nil, opentracing.ErrUnsupportedFormat
	}
	return tracer.propagators[builtin].Extract(carrier)
}

func (tracer *tracerImpl) reconnectClient(now time.Time) {
	conn, err := tracer.client.ConnectClient()
	if err != nil {
		emitEvent(newEventConnectionError(err))
	} else {
		tracer.lock.Lock()
		oldConn := tracer.connection
		tracer.connection = conn
		tracer.lock.Unlock()

		oldConn.Close()
	}
}

// Close flushes and then terminates the LightStep collector. Close may only be
// called once; subsequent calls to Close are no-ops.
func (tracer *tracerImpl) Close(ctx context.Context) {
	tracer.closeOnce.Do(func() {
		// notify report loop that we are closing
		close(tracer.closeReportLoopChannel)
		select {
		case <-tracer.reportLoopClosedChannel:
			tracer.Flush(ctx)
		case <-ctx.Done():
			return
		}

		// now its safe to close the connection
		tracer.lock.Lock()
		conn := tracer.connection
		tracer.connection = nil
		tracer.lock.Unlock()

		if conn != nil {
			err := conn.Close()
			if err != nil {
				emitEvent(newEventConnectionError(err))
			}
		}
	})
}

// RecordSpan records a finished Span.
func (tracer *tracerImpl) RecordSpan(raw RawSpan) {
	tracer.lock.Lock()

	// Early-out for disabled runtimes
	if tracer.disabled {
		tracer.lock.Unlock()
		return
	}

	tracer.buffer.addSpan(raw)
	tracer.lock.Unlock()

	if tracer.opts.Recorder != nil {
		tracer.opts.Recorder.RecordSpan(raw)
	}
}

// Flush sends all buffered data to the collector.
func (tracer *tracerImpl) Flush(ctx context.Context) {
	tracer.flushingLock.Lock()
	defer tracer.flushingLock.Unlock()

	if errorEvent := tracer.preFlush(); errorEvent != nil {
		emitEvent(errorEvent)
		return
	}

	if tracer.opts.MetaEventReportingEnabled && !tracer.firstReportHasRun {
		opentracing.StartSpan(LSMetaEvent_TracerCreateOperation,
			opentracing.Tag{Key: LSMetaEvent_MetaEventKey, Value: true},
			opentracing.Tag{Key: LSMetaEvent_TracerGuidKey, Value: tracer.reporterID}).
			Finish()
		tracer.firstReportHasRun = true
	}

	ctx, cancel := context.WithTimeout(ctx, tracer.opts.ReportTimeout)
	defer cancel()

	protoReq := tracer.converter.toReportRequest(
		tracer.reporterID,
		tracer.attributes,
		tracer.accessToken,
		&tracer.flushing,
	)
	req, err := tracer.client.Translate(protoReq)
	if err != nil {
		errorEvent := newEventFlushError(err, FlushErrorTranslate)
		emitEvent(errorEvent)
		// call postflush to prevent the tracer from going into an invalid state.
		emitEvent(tracer.postFlush(errorEvent))
		return
	}

	var reportErrorEvent *eventFlushError
	resp, err := tracer.client.Report(ctx, req)
	if err != nil {
		reportErrorEvent = newEventFlushError(err, FlushErrorTransport)
	} else if len(resp.GetErrors()) > 0 {
		reportErrorEvent = newEventFlushError(fmt.Errorf(resp.GetErrors()[0]), FlushErrorReport)
	}

	if reportErrorEvent != nil {
		emitEvent(reportErrorEvent)
	}
	emitEvent(tracer.postFlush(reportErrorEvent))

	if err == nil && resp.DevMode() {
		tracer.metaEventReportingEnabled = true
	}

	if err == nil && !resp.DevMode() {
		tracer.metaEventReportingEnabled = false
	}

	if err == nil && resp.Disable() {
		tracer.Disable()
	}
}

// preFlush handles lock-protected data manipulation before flushing
func (tracer *tracerImpl) preFlush() *eventFlushError {
	tracer.lock.Lock()
	defer tracer.lock.Unlock()

	if tracer.disabled {
		return newEventFlushError(errFlushFailedTracerClosed, FlushErrorTracerDisabled)
	}

	if tracer.connection == nil {
		return newEventFlushError(errFlushFailedTracerClosed, FlushErrorTracerClosed)
	}

	now := time.Now()
	tracer.buffer, tracer.flushing = tracer.flushing, tracer.buffer
	tracer.reportInFlight = true
	tracer.flushing.setFlushing(now)
	tracer.buffer.setCurrent(now)
	tracer.lastReportAttempt = now
	return nil
}

// postFlush handles lock-protected data manipulation after flushing
func (tracer *tracerImpl) postFlush(flushEventError *eventFlushError) *eventStatusReport {
	tracer.lock.Lock()
	defer tracer.lock.Unlock()

	tracer.reportInFlight = false

	statusReportEvent := newEventStatusReport(
		tracer.flushing.reportStart,
		tracer.flushing.reportEnd,
		len(tracer.flushing.rawSpans),
		int(tracer.flushing.droppedSpanCount+tracer.buffer.droppedSpanCount),
		int(tracer.flushing.logEncoderErrorCount+tracer.buffer.logEncoderErrorCount),
	)

	if flushEventError == nil {
		tracer.flushing.clear()
		return statusReportEvent
	}

	switch flushEventError.State() {
	case FlushErrorTranslate:
		// When there's a translation error, we do not want to retry.
		tracer.flushing.clear()
	default:
		// Restore the records that did not get sent correctly
		tracer.buffer.mergeFrom(&tracer.flushing)
	}

	statusReportEvent.SetSentSpans(0)

	return statusReportEvent
}

func (tracer *tracerImpl) Disable() {
	tracer.lock.Lock()
	if tracer.disabled {
		tracer.lock.Unlock()
		return
	}
	tracer.disabled = true
	tracer.buffer.clear()
	tracer.lock.Unlock()

	emitEvent(newEventTracerDisabled())
}

// Every MinReportingPeriod the reporting loop wakes up and checks to see if
// either (a) the Runtime's max reporting period is about to expire (see
// maxReportingPeriod()), (b) the number of buffered log records is
// approaching kMaxBufferedLogs, or if (c) the number of buffered span records
// is approaching kMaxBufferedSpans. If any of those conditions are true,
// pending data is flushed to the remote peer. If not, the reporting loop waits
// until the next cycle. See Runtime.maybeFlush() for details.
//
// This could alternatively be implemented using flush channels and so forth,
// but that would introduce opportunities for client code to block on the
// runtime library, and we want to avoid that at all costs (even dropping data,
// which can certainly happen with high data rates and/or unresponsive remote
// peers).

func (tracer *tracerImpl) shouldFlushLocked(now time.Time) bool {
	if now.Add(tracer.opts.MinReportingPeriod).Sub(tracer.lastReportAttempt) > tracer.opts.ReportingPeriod {
		return true
	} else if tracer.buffer.isHalfFull() {
		return true
	}
	return false
}

func (tracer *tracerImpl) reportLoop() {
	tickerChan := time.Tick(tracer.opts.MinReportingPeriod)
	for {
		select {
		case <-tickerChan:
			now := time.Now()

			tracer.lock.Lock()
			disabled := tracer.disabled
			reconnect := !tracer.reportInFlight && tracer.client.ShouldReconnect()
			shouldFlush := tracer.shouldFlushLocked(now)
			tracer.lock.Unlock()

			if disabled {
				return
			}
			if shouldFlush {
				tracer.Flush(context.Background())
			}
			if reconnect {
				tracer.reconnectClient(now)
			}
		case <-tracer.closeReportLoopChannel:
			close(tracer.reportLoopClosedChannel)
			return
		}
	}
}
