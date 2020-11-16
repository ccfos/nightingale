# lightstep-tracer-go

[![Circle CI](https://circleci.com/gh/lightstep/lightstep-tracer-go.svg?style=shield)](https://circleci.com/gh/lightstep/lightstep-tracer-go)
[![MIT license](http://img.shields.io/badge/license-MIT-blue.svg)](http://opensource.org/licenses/MIT)
[![GoDoc](https://godoc.org/github.com/lightstep/lightstep-tracer-go?status.svg)](https://godoc.org/github.com/lightstep/lightstep-tracer-go)

The LightStep distributed tracing library for Go.

**Looking for the LightStep OpenCensus exporter? Check out the [`lightstepoc` package](./lightstepoc).**

## Installation

```
$ go get 'github.com/lightstep/lightstep-tracer-go'
```

## API Documentation

Godoc: https://godoc.org/github.com/lightstep/lightstep-tracer-go

## Initialization: Starting a new tracer
To initialize a tracer, configure it with a valid Access Token and optional tuning parameters. Register the tracer as the OpenTracing global tracer so that it will become available to your installed instrumentation libraries.

```go
import (
  "github.com/opentracing/opentracing-go"
  "github.com/lightstep/lightstep-tracer-go"
)

func main() {
  lightstepTracer := lightstep.NewTracer(lightstep.Options{
    AccessToken: "YourAccessToken",
  })

  opentracing.SetGlobalTracer(lightstepTracer)
}
```

## Instrumenting Code: Using the OpenTracing API

All instrumentation should be done through the OpenTracing API, rather than using the lightstep tracer type directly. For API documentation and advice on instrumentation in general, see the opentracing godocs and the opentracing website.

- https://godoc.org/github.com/opentracing/opentracing-go
- http://opentracing.io

## Flushing and Closing: Managing the tracer lifecycle

As part of managing your application lifecycle, the lightstep tracer extends the `opentracing.Tracer` interface with methods for manual flushing and closing. To access these methods, you can take the global tracer and typecast it to a `lightstep.Tracer`. As a convenience, the lightstep package provides static methods which perform the typecasting.

```go
import (
  "context"
  "github.com/opentracing/opentracing-go"
  "github.com/lightstep/lightstep-tracer-go"
)

func shutdown(ctx context.Context) {
  // access the running tracer
  tracer := opentracing.GlobalTracer()
    
  // typecast from opentracing.Tracer to lightstep.Tracer
  lsTracer, ok := tracer.(lightstep.Tracer)
  if (!ok) { 
    return 
  }
  lsTracer.Close(ctx)

  // or use static methods
  lightstep.Close(ctx, tracer)
}
```

## Event Handling: Observing the LightStep tracer
In order to connect diagnostic information from the lightstep tracer into an application's logging and metrics systems, inject an event handler using the `OnEvent` static method. Events may be typecast to check for errors or specific events such as status reports.

```go
import (
  "example/logger"
  "example/metrics"
  "github.com/lightstep/lightstep-tracer-go"
)

logAndMetricsHandler := func(event lightstep.Event){
  switch event := event.(type) {
  case EventStatusReport:
    metrics.Count("tracer.dropped_spans", event.DroppedSpans())
  case ErrorEvent:
    logger.Error("LS Tracer error: %s", event)
  default:
    logger.Info("LS Tracer info: %s", event)
  }
}

func main() {
  // setup event handler first to catch startup errors
  lightstep.SetGlobalEventHandler(logAndMetricsHandler)
  
  lightstepTracer := lightstep.NewTracer(lightstep.Options{
    AccessToken: "YourAccessToken",
  })

  opentracing.SetGlobalTracer(lightstepTracer)
}
```

Event handlers will receive events from any active tracers, as well as errors in static functions. It is suggested that you set up event handling before initializing your tracer to catch any errors on initialization.

## Advanced Configuration: Transport and Serialization Protocols

By following the above configuration, the tracer will send information to LightStep using HTTP and Protocol Buffers which is the recommended configuration. If there are no specific transport protocol needs you have, there is no need to change this default.

There are two options for transport protocols:

- [Protocol Buffers](https://developers.google.com/protocol-buffers/) over HTTP - The recommended and default solution.
- [Protocol Buffers](https://developers.google.com/protocol-buffers/) over [GRPC](https://grpc.io/) - This is a more advanced solution that might be desirable if you already have gRPC networking configured.

You can configure which transport protocol the tracer uses using the `UseGRPC` and `UseHttp` flags in the options.
