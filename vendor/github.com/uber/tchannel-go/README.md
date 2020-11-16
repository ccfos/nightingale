# TChannel [![GoDoc][doc-img]][doc] [![Build Status][ci-img]][ci] [![Coverage Status][cov-img]][cov]

[TChannel][tchan-spec] is a multiplexing and framing protocol for RPC calls.
tchannel-go is a Go implementation of the protocol, including client libraries
for [Hyperbahn][hyperbahn].

If you'd like to start by writing a small Thrift and TChannel service, check
out [this guide](guide/Thrift_Hyperbahn.md). For a less opinionated setup, see
the [contribution guidelines](CONTRIBUTING.md).

## Overview

TChannel is a network protocol that supports:

 * A request/response model,
 * Multiplexing multiple requests across the same TCP socket,
 * Out-of-order responses,
 * Streaming requests and responses,
 * Checksummed frames,
 * Transport of arbitrary payloads,
 * Easy implementation in many languages, and
 * Redis-like performance.

This protocol is intended to run on datacenter networks for inter-process
communication.

## Protocol

TChannel frames have a fixed-length header and 3 variable-length fields. The
underlying protocol does not assign meaning to these fields, but the included
client/server implementation uses the first field to represent a unique
endpoint or function name in an RPC model.  The next two fields can be used for
arbitrary data. Some suggested way to use the 3 fields are:

* URI path + HTTP method and headers as JSON + body, or
* Function name + headers + thrift/protobuf.

Note, however, that the only encoding supported by TChannel is UTF-8.  If you
want JSON, you'll need to stringify and parse outside of TChannel.

This design supports efficient routing and forwarding: routers need to parse
the first or second field, but can forward the third field without parsing.

There is no notion of client and server in this system. Every TChannel instance
is capable of making and receiving requests, and thus requires a unique port on
which to listen. This requirement may change in the future.

See the [protocol specification][tchan-proto-spec] for more details.

## Examples

 - [ping](examples/ping): A simple ping/pong example using raw TChannel.
 - [thrift](examples/thrift): A Thrift server/client example.
 - [keyvalue](examples/keyvalue): A keyvalue Thrift service with separate server and client binaries.

<hr>
This project is released under the [MIT License](LICENSE.md).

[doc-img]: https://godoc.org/github.com/uber/tchannel-go?status.svg
[doc]: https://godoc.org/github.com/uber/tchannel-go
[ci-img]: https://travis-ci.com/uber/tchannel-go.svg?branch=master
[ci]: https://travis-ci.com/uber/tchannel-go
[cov-img]: https://coveralls.io/repos/uber/tchannel-go/badge.svg?branch=master&service=github
[cov]: https://coveralls.io/github/uber/tchannel-go?branch=master
[tchan-spec]: http://tchannel.readthedocs.org/en/latest/
[tchan-proto-spec]: http://tchannel.readthedocs.org/en/latest/protocol/
[hyperbahn]: https://github.com/uber/hyperbahn
