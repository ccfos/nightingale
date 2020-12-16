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
	"fmt"

	athrift "github.com/apache/thrift/lib/go/thrift"
	gen "github.com/uber/tchannel-go/thrift/gen-go/meta"
)

// Interfaces for the service and client for the services defined in the IDL.

// tchanMeta is the interface that defines the server handler and client interface.
type tchanMeta interface {
	Health(ctx Context, req *gen.HealthRequest) (*gen.HealthStatus, error)
	ThriftIDL(ctx Context) (*gen.ThriftIDLs, error)
	VersionInfo(ctx Context) (*gen.VersionInfo, error)
}

// Implementation of a client and service handler.

type tchanMetaClient struct {
	thriftService string
	client        TChanClient
}

func newTChanMetaClient(client TChanClient) tchanMeta {
	return &tchanMetaClient{
		"Meta",
		client,
	}
}

func (c *tchanMetaClient) Health(ctx Context, req *gen.HealthRequest) (*gen.HealthStatus, error) {
	var resp gen.MetaHealthResult
	args := gen.MetaHealthArgs{
		Hr: req,
	}
	success, err := c.client.Call(ctx, c.thriftService, "health", &args, &resp)
	if err == nil && !success {
	}

	return resp.GetSuccess(), err
}

func (c *tchanMetaClient) ThriftIDL(ctx Context) (*gen.ThriftIDLs, error) {
	var resp gen.MetaThriftIDLResult
	args := gen.MetaThriftIDLArgs{}
	success, err := c.client.Call(ctx, c.thriftService, "thriftIDL", &args, &resp)
	if err == nil && !success {
	}

	return resp.GetSuccess(), err
}

func (c *tchanMetaClient) VersionInfo(ctx Context) (*gen.VersionInfo, error) {
	var resp gen.MetaVersionInfoResult
	args := gen.MetaVersionInfoArgs{}
	success, err := c.client.Call(ctx, c.thriftService, "versionInfo", &args, &resp)
	if err == nil && !success {
	}

	return resp.GetSuccess(), err
}

type tchanMetaServer struct {
	handler tchanMeta
}

func newTChanMetaServer(handler tchanMeta) TChanServer {
	return &tchanMetaServer{
		handler,
	}
}

func (s *tchanMetaServer) Service() string {
	return "Meta"
}

func (s *tchanMetaServer) Methods() []string {
	return []string{
		"health",
		"thriftIDL",
		"versionInfo",
	}
}

func (s *tchanMetaServer) Handle(ctx Context, methodName string, protocol athrift.TProtocol) (bool, athrift.TStruct, error) {
	switch methodName {
	case "health":
		return s.handleHealth(ctx, protocol)
	case "thriftIDL":
		return s.handleThriftIDL(ctx, protocol)
	case "versionInfo":
		return s.handleVersionInfo(ctx, protocol)

	default:
		return false, nil, fmt.Errorf("method %v not found in service %v", methodName, s.Service())
	}
}

func (s *tchanMetaServer) handleHealth(ctx Context, protocol athrift.TProtocol) (bool, athrift.TStruct, error) {
	var req gen.MetaHealthArgs
	var res gen.MetaHealthResult

	if err := req.Read(protocol); err != nil {
		return false, nil, err
	}

	r, err :=
		s.handler.Health(ctx, req.Hr)

	if err != nil {
		return false, nil, err
	}
	res.Success = r

	return err == nil, &res, nil
}

func (s *tchanMetaServer) handleThriftIDL(ctx Context, protocol athrift.TProtocol) (bool, athrift.TStruct, error) {
	var req gen.MetaThriftIDLArgs
	var res gen.MetaThriftIDLResult

	if err := req.Read(protocol); err != nil {
		return false, nil, err
	}

	r, err :=
		s.handler.ThriftIDL(ctx)

	if err != nil {
		return false, nil, err
	}
	res.Success = r

	return err == nil, &res, nil
}

func (s *tchanMetaServer) handleVersionInfo(ctx Context, protocol athrift.TProtocol) (bool, athrift.TStruct, error) {
	var req gen.MetaVersionInfoArgs
	var res gen.MetaVersionInfoResult

	if err := req.Read(protocol); err != nil {
		return false, nil, err
	}

	r, err :=
		s.handler.VersionInfo(ctx)

	if err != nil {
		return false, nil, err
	}
	res.Success = r

	return err == nil, &res, nil
}
