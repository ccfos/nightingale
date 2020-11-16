// Copyright (c) 2017 Uber Technologies, Inc.
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

package kvconfig

const (
	// NamespacesKey is the KV config key for the runtime configuration
	// specifying the namespaces configured.
	NamespacesKey = "m3db.node.namespaces"

	// BootstrapperKey is the KV config key for the runtime configuration
	// specifying the set of bootstrappers as a string array.
	BootstrapperKey = "m3db.node.bootstrapper"

	// ClusterNewSeriesInsertLimitKey is the KV config key for the runtime
	// configuration specifying a hard limit for a cluster new series insertions.
	ClusterNewSeriesInsertLimitKey = "m3db.node.cluster-new-series-insert-limit"

	// EncodersPerBlockLimitKey is the KV config key for the runtime
	// configuration specifying a hard limit on the number of active encoders
	// per block.
	EncodersPerBlockLimitKey = "m3db.node.encoders-per-block-limit"

	// ClientBootstrapConsistencyLevel is the KV config key for the runtime
	// configuration specifying the client bootstrap consistency level
	ClientBootstrapConsistencyLevel = "m3db.client.bootstrap-consistency-level"

	// ClientReadConsistencyLevel is the KV config key for the runtime
	// configuration specifying the client read consistency level
	ClientReadConsistencyLevel = "m3db.client.read-consistency-level"

	// ClientWriteConsistencyLevel is the KV config key for the runtime
	// configuration specifying the client write consistency level
	ClientWriteConsistencyLevel = "m3db.client.write-consistency-level"
)
