// Copyright (c) 2019 Uber Technologies, Inc.
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

package m3

// Configuration is a configuration for a M3 reporter.
type Configuration struct {
	// HostPort is the host and port of the M3 server.
	HostPort string `yaml:"hostPort" validate:"nonzero"`

	// HostPorts are the host and port of the M3 server.
	HostPorts []string `yaml:"hostPorts"`

	// Service is the service tag to that this client emits.
	Service string `yaml:"service" validate:"nonzero"`

	// Env is the env tag to use that this client emits.
	Env string `yaml:"env" validate:"nonzero"`

	// CommonTags are tags that are common for all metrics this client emits.
	CommonTags map[string]string `yaml:"tags" `

	// Queue is the maximum metric queue size of client.
	Queue int `yaml:"queue"`

	// PacketSize is the maximum packet size for a batch of metrics.
	PacketSize int32 `yaml:"packetSize"`

	// IncludeHost is whether or not to include host tag.
	IncludeHost bool `yaml:"includeHost"`
}

// NewReporter creates a new M3 reporter from this configuration.
func (c Configuration) NewReporter() (Reporter, error) {
	hostPorts := c.HostPorts
	if len(hostPorts) == 0 {
		hostPorts = []string{c.HostPort}
	}
	return NewReporter(Options{
		HostPorts:          hostPorts,
		Service:            c.Service,
		Env:                c.Env,
		CommonTags:         c.CommonTags,
		MaxQueueSize:       c.Queue,
		MaxPacketSizeBytes: c.PacketSize,
		IncludeHost:        c.IncludeHost,
	})
}
