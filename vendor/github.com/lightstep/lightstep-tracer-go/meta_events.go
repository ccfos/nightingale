package lightstep

// keys for ls meta span events
const LSMetaEvent_MetaEventKey = "lightstep.meta_event"
const LSMetaEvent_PropagationFormatKey = "lightstep.propagation_format"
const LSMetaEvent_TraceIdKey = "lightstep.trace_id"
const LSMetaEvent_SpanIdKey = "lightstep.span_id"
const LSMetaEvent_TracerGuidKey = "lightstep.tracer_guid"

// operation names for ls meta span events
const LSMetaEvent_ExtractOperation = "lightstep.extract_span"
const LSMetaEvent_InjectOperation = "lightstep.inject_span"
const LSMetaEvent_SpanStartOperation = "lightstep.span_start"
const LSMetaEvent_SpanFinishOperation = "lightstep.span_finish"
const LSMetaEvent_TracerCreateOperation = "lightstep.tracer_create"
