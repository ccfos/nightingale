# Changelog

## [Pending Release](https://github.com/lightstep/lightstep-tracer-go/compare/v0.18.1...HEAD)

## [v0.18.1](https://github.com/lightstep/lightstep-tracer-go/compare/v0.18.0...v0.18.1)
* Adding support for configuring custom propagators

## [v0.18.0](https://github.com/lightstep/lightstep-tracer-go/compare/v0.17.1...v0.18.0)
* Adding support for B3 headers [#224](https://github.com/lightstep/lightstep-tracer-go/issues/224)
* Fix OpenCensus to LighStep ID conversion
* Added new constructor `CreateTracer` to propagate errors [#226](https://github.com/lightstep/lightstep-tracer-go/issues/226)

## [v0.17.1](https://github.com/lightstep/lightstep-tracer-go/compare/v0.17.0...v0.17.1)
* Fixes [#219](https://github.com/lightstep/lightstep-tracer-go/issues/219) so that there is no longer a data race when reading and writing from spans

## [v0.17.0](https://github.com/lightstep/lightstep-tracer-go/compare/v0.16.0...v0.17.0)
* Migrate dependency management from dep to gomod
* Lazy loggers can emit 0 -> N log entries instead of just one.
* Refactor Collector Client to allow for user-defined transports.

## [v0.16.0](https://github.com/lightstep/lightstep-tracer-go/compare/v0.15.6...v0.16.0)
* Thrift transport is eliminated
* If no transport is specified, UseHTTP is now the default (was UseGRPC)
* Requires go >= 1.7
* Update to gogo-1.2.1, no longer using Google protobuf
* Imports `context` via the standard library instead of `golang.org/x/net/context`
* Fixes [#182](https://github.com/lightstep/lightstep-tracer-go/issues/182), so that `StartSpan` can now take `SpanReference`s to non-LightStep `SpanContext`s use
* Adds experimental OpenCensus exporter
* Access-Tokens are passed as headers to support improved load balancing
* Support for custom TLS certificates.

## [v0.15.6](https://github.com/lightstep/lightstep-tracer-go/compare/v0.15.5...v0.15.6)

* Minor update to `sendspan` to make it easier to pick which transport protocol to use.
* Add a new field to Options: DialOptions. These allow setting custom grpc dial options when using grpc.
  * This is necessary to have customer balancers or interceptors.
  * DialOptions shouldn't be set unless it is needed, it will default correctly.
* Added a new field to Endpoint: Scheme. Scheme can be used to override the default schemes (http/https) or set a custom scheme (for grpc).
  * This is necessary for using custom grpc resolvers.
  * If callers are using struct construction without field names (i.e. Endpoint{"host", port, ...}), they will need to add a new field (scheme = "").
  * Scheme shouldn't be set unless it needs to overridden, it will default correctly.

## [v0.15.5](https://github.com/lightstep/lightstep-tracer-go/compare/v0.15.4...v0.15.5)
* Internal performance optimizations and a bug fix for issue [#161](https://github.com/lightstep/lightstep-tracer-go/issues/161)

## [v0.15.4](https://github.com/lightstep/lightstep-tracer-go/compare/v0.15.3...v0.15.4)
* This change affects LightStep's internal testing, not a functional change.

## [v0.15.3](https://github.com/lightstep/lightstep-tracer-go/compare/v0.15.2...v0.15.3)
* Adds compatibility for io.Writer and io.Reader in Inject/Extract, as required by Open Tracing.

## [v0.15.2](https://github.com/lightstep/lightstep-tracer-go/compare/v0.15.1...v0.15.2)
* Adds lightstep.GetLightStepReporterID.

## [v0.15.1](https://github.com/lightstep/lightstep-tracer-go/compare/v0.15.0...v0.15.1)
* Adds Gopkg.toml

## [v0.15.0](https://github.com/lightstep/lightstep-tracer-go/compare/v0.14.0...v0.15.0)
* We are replacing the internal diagnostic logging with a more flexible “Event” framework. This enables you to track and understand tracer problems with metrics and logging tools of your choice.
* We are also changed the types of the Close() and Flush() methods to take a context parameter to support cancellation. These changes are *not* backwards compatible and *you will need to update your instrumentation*. We are providing a NewTracerv0_14() method that is a drop-in replacement for the previous version.

## [v0.14.0](https://github.com/lightstep/lightstep-tracer-go/compare/v0.13.0...v0.14.0)
* Flush buffer syncronously on Close.
* Flush twice if a flush is already in flight.
* Remove gogo in favor of golang/protobuf.
* Requires grpc-go >= 1.4.0.

## [v0.13.0](https://github.com/lightstep/lightstep-tracer-go/compare/v0.12.0...v0.13.0) 
* BasicTracer has been removed.
* Tracer now takes a SpanRecorder as an option.
* Tracer interface now includes Close and Flush.
* Tests redone with ginkgo/gomega.

## [v0.12.0](https://github.com/lightstep/lightstep-tracer-go/compare/v0.11.0...v0.12.0)
* Added CloseTracer function to flush and close a lightstep recorder.

## [v0.11.0](https://github.com/lightstep/lightstep-tracer-go/compare/v0.10.0...v0.11.0)
* Thrift transport is now deprecated, gRPC is the default.
