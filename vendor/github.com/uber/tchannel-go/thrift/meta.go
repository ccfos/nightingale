// Copyright (c) 2015 Uber Technologies, Inc.

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

package thrift

import (
	"errors"
	"runtime"
	"strings"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift/gen-go/meta"
)

// HealthFunc is the interface for custom health endpoints.
// ok is whether the service health is OK, and message is optional additional information for the health result.
type HealthFunc func(ctx Context) (ok bool, message string)

// HealthRequestType is the type of health check.
type HealthRequestType int

const (
	// Process health checks are used to check whether the process is up
	// and should almost always return true immediately.
	Process HealthRequestType = iota

	// Traffic health checks are used to check whether the process should
	// receive traffic. This can be used to keep a process running, but
	// not receiving health checks (e.g., during process warm-up).
	Traffic
)

// HealthRequest is optional parametres for a health request.
type HealthRequest struct {
	// Type is the type of health check being requested.
	Type HealthRequestType
}

// HealthRequestFunc is a health check function that includes parameters
// about the health check.
type HealthRequestFunc func(Context, HealthRequest) (ok bool, message string)

// healthHandler implements the default health check enpoint.
type metaHandler struct {
	healthFn HealthRequestFunc
}

// newMetaHandler return a new HealthHandler instance.
func newMetaHandler() *metaHandler {
	return &metaHandler{healthFn: defaultHealth}
}

// Health returns true as default Health endpoint.
func (h *metaHandler) Health(ctx Context, req *meta.HealthRequest) (*meta.HealthStatus, error) {
	ok, message := h.healthFn(ctx, metaReqToReq(req))
	if message == "" {
		return &meta.HealthStatus{Ok: ok}, nil
	}
	return &meta.HealthStatus{Ok: ok, Message: &message}, nil
}

func (h *metaHandler) ThriftIDL(ctx Context) (*meta.ThriftIDLs, error) {
	// TODO(prashant): Add thriftIDL to the generated code.
	return nil, errors.New("unimplemented")
}

func (h *metaHandler) VersionInfo(ctx Context) (*meta.VersionInfo, error) {
	return &meta.VersionInfo{
		Language:        "go",
		LanguageVersion: strings.TrimPrefix(runtime.Version(), "go"),
		Version:         tchannel.VersionInfo,
	}, nil
}

func defaultHealth(ctx Context, r HealthRequest) (bool, string) {
	return true, ""
}

func (h *metaHandler) setHandler(f HealthRequestFunc) {
	h.healthFn = f
}

func metaReqToReq(r *meta.HealthRequest) HealthRequest {
	if r == nil {
		return HealthRequest{}
	}

	return HealthRequest{
		Type: HealthRequestType(r.GetType()),
	}
}
