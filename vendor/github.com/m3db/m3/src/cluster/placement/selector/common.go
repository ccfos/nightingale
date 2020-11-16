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

package selector

import (
	"errors"

	"github.com/m3db/m3/src/cluster/placement"
)

var (
	errNoValidCandidateInstance = errors.New("no new instances found in the valid zone")
)

// getValidCandidates finds the instances that are not already in
// the placement and in the valid zone.
func getValidCandidates(
	p placement.Placement,
	candidates []placement.Instance,
	opts placement.Options,
) ([]placement.Instance, error) {
	var instances = make([]placement.Instance, 0, len(candidates))
	for _, instance := range candidates {
		instanceInPlacement, exist := p.Instance(instance.ID())
		if !exist {
			instances = append(instances, instance)
			continue
		}
		if instanceInPlacement.IsLeaving() {
			instances = append(instances, instanceInPlacement)
		}
	}

	instances = filterZones(p, instances, opts)
	if len(instances) == 0 {
		return nil, errNoValidCandidateInstance
	}

	return instances, nil
}

func filterZones(
	p placement.Placement,
	candidates []placement.Instance,
	opts placement.Options,
) []placement.Instance {
	if len(candidates) == 0 {
		return []placement.Instance{}
	}

	var validZone string
	if opts != nil {
		validZone = opts.ValidZone()
		if opts.AllowAllZones() {
			return candidates
		}
	}
	if validZone == "" && len(p.Instances()) > 0 {
		validZone = p.Instances()[0].Zone()
	}

	validInstances := make([]placement.Instance, 0, len(candidates))
	for _, instance := range candidates {
		if validZone == instance.Zone() {
			validInstances = append(validInstances, instance)
		}
	}
	return validInstances
}
