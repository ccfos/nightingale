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
	"container/heap"
	"errors"
	"fmt"
	"math"

	"github.com/m3db/m3/src/cluster/placement"

	"go.uber.org/zap"
)

var (
	errNoValidMirrorInstance = errors.New("no valid instance for mirror placement in the candidate list")
)

// mirroredPortSelector groups instances by their port--see NewPortMirroredSelector for details.
type mirroredPortSelector struct {
	opts   placement.Options
	logger *zap.Logger
}

// NewPortMirroredSelector returns a placement.InstanceSelector which creates groups of instances
// by their port number and assigns a shardset to each group, taking isolation groups into account
// while creating groups. This is the default behavior used by NewInstanceSelector if IsMirrored
// is true.
func NewPortMirroredSelector(opts placement.Options) placement.InstanceSelector {
	return &mirroredPortSelector{
		opts:   opts,
		logger: opts.InstrumentOptions().Logger(),
	}
}

// SelectInitialInstances tries to make as many groups as possible from
// the candidate instances to make the initial placement.
func (f *mirroredPortSelector) SelectInitialInstances(
	candidates []placement.Instance,
	rf int,
) ([]placement.Instance, error) {
	candidates, err := getValidCandidates(
		placement.NewPlacement(),
		candidates,
		f.opts,
	)
	if err != nil {
		return nil, err
	}

	weightToHostMap, err := groupHostsByWeight(candidates)
	if err != nil {
		return nil, err
	}

	var groups = make([][]placement.Instance, 0, len(candidates))
	for _, hosts := range weightToHostMap {
		groupedHosts, ungrouped := groupHostsWithIsolationGroupCheck(hosts, rf)
		if len(ungrouped) != 0 {
			for _, host := range ungrouped {
				f.logger.Warn("could not group",
					zap.String("host", host.name),
					zap.String("isolationGroup", host.isolationGroup),
					zap.Uint32("weight", host.weight))
			}
		}
		if len(groupedHosts) == 0 {
			continue
		}

		groupedInstances, err := groupInstancesByHostPort(groupedHosts)
		if err != nil {
			return nil, err
		}

		groups = append(groups, groupedInstances...)
	}

	if len(groups) == 0 {
		return nil, errNoValidMirrorInstance
	}

	return assignShardsetsToGroupedInstances(groups, placement.NewPlacement()), nil
}

// SelectAddingInstances tries to make just one group of hosts from
// the candidate instances to be added to the placement.
func (f *mirroredPortSelector) SelectAddingInstances(
	candidates []placement.Instance,
	p placement.Placement,
) ([]placement.Instance, error) {
	candidates, err := getValidCandidates(p, candidates, f.opts)
	if err != nil {
		return nil, err
	}

	weightToHostMap, err := groupHostsByWeight(candidates)
	if err != nil {
		return nil, err
	}

	var groups = make([][]placement.Instance, 0, len(candidates))
	for _, hosts := range weightToHostMap {
		groupedHosts, _ := groupHostsWithIsolationGroupCheck(hosts, p.ReplicaFactor())
		if len(groupedHosts) == 0 {
			continue
		}

		if !f.opts.AddAllCandidates() {
			// When AddAllCandidates option is disabled, we will only add
			// one pair of hosts into the placement.
			groups, err = groupInstancesByHostPort(groupedHosts[:1])
			if err != nil {
				return nil, err
			}

			break
		}

		newGroups, err := groupInstancesByHostPort(groupedHosts)
		if err != nil {
			return nil, err
		}
		groups = append(groups, newGroups...)
	}

	if len(groups) == 0 {
		return nil, errNoValidMirrorInstance
	}

	return assignShardsetsToGroupedInstances(groups, p), nil
}

// SelectReplaceInstances for mirror supports replacing multiple instances from one host.
// Two main use cases:
// 1, find a new host from a pool of hosts to replace a host in the placement.
// 2, back out of a replacement, both leaving and adding host are still in the placement.
func (f *mirroredPortSelector) SelectReplaceInstances(
	candidates []placement.Instance,
	leavingInstanceIDs []string,
	p placement.Placement,
) ([]placement.Instance, error) {
	candidates, err := getValidCandidates(p, candidates, f.opts)
	if err != nil {
		return nil, err
	}

	leavingInstances, err := getLeavingInstances(p, leavingInstanceIDs)
	if err != nil {
		return nil, err
	}

	// Validate leaving instances.
	var (
		h     host
		ssIDs = make(map[uint32]struct{}, len(leavingInstances))
	)
	for _, instance := range leavingInstances {
		if h.name == "" {
			h = newHost(instance.Hostname(), instance.IsolationGroup(), instance.Weight())
		}

		err := h.addInstance(instance.Port(), instance)
		if err != nil {
			return nil, err
		}
		ssIDs[instance.ShardSetID()] = struct{}{}
	}

	weightToHostMap, err := groupHostsByWeight(candidates)
	if err != nil {
		return nil, err
	}

	hosts, ok := weightToHostMap[h.weight]
	if !ok {
		return nil, fmt.Errorf("could not find instances with weight %d in the candidate list", h.weight)
	}

	// Find out the isolation groups that are already in the same shard set id with the leaving instances.
	var conflictIGs = make(map[string]struct{})
	for _, instance := range p.Instances() {
		if _, ok := ssIDs[instance.ShardSetID()]; !ok {
			continue
		}
		if instance.Hostname() == h.name {
			continue
		}
		if instance.IsLeaving() {
			continue
		}

		conflictIGs[instance.IsolationGroup()] = struct{}{}
	}

	var replacementGroups []mirroredReplacementGroup
	for _, candidateHost := range hosts {
		if candidateHost.name == h.name {
			continue
		}

		if _, ok := conflictIGs[candidateHost.isolationGroup]; ok {
			continue
		}

		groups, err := groupInstancesByHostPort([][]host{[]host{h, candidateHost}})
		if err != nil {
			f.logger.Warn("could not match up candidate host with target host",
				zap.String("candidate", candidateHost.name),
				zap.String("target", h.name),
				zap.Error(err))
			continue
		}

		for _, group := range groups {
			if len(group) != 2 {
				return nil, fmt.Errorf(
					"unexpected length of instance group for replacement: %d",
					len(group),
				)
			}

			replacementGroup := mirroredReplacementGroup{}

			// search for leaving + replacement in the group (don't assume anything about the order)
			for _, inst := range group {
				if inst.Hostname() == h.name {
					replacementGroup.Leaving = inst
				} else if inst.Hostname() == candidateHost.name {
					replacementGroup.Replacement = inst
				}
			}
			if replacementGroup.Replacement == nil {
				return nil, fmt.Errorf(
					"programming error: failed to find replacement instance for host %s in group",
					candidateHost.name,
				)
			}
			if replacementGroup.Leaving == nil {
				return nil, fmt.Errorf(
					"programming error: failed to find leaving instance for host %s in group",
					h.name,
				)
			}

			replacementGroups = append(
				replacementGroups,
				replacementGroup,
			)
		}

		// Successfully grouped candidate with the host in placement.
		break
	}

	if len(replacementGroups) == 0 {
		return nil, errNoValidMirrorInstance
	}

	return assignShardsetIDsToReplacements(leavingInstanceIDs, replacementGroups)
}

// assignShardsetIDsToReplacements assigns the shardset of each leaving instance to each replacement
// instance. The output is ordered in the order of leavingInstanceIDs.
func assignShardsetIDsToReplacements(
	leavingInstanceIDs []string,
	groups []mirroredReplacementGroup,
) ([]placement.Instance, error) {
	if len(groups) != len(leavingInstanceIDs) {
		return nil, fmt.Errorf(
			"failed to find %d replacement instances to replace %d leaving instances",
			len(groups), len(leavingInstanceIDs),
		)
	}
	// The groups returned from the groupInstances() might not be the same order as
	// the instances in leavingInstanceIDs. We need to reorder them to the same order
	// as leavingInstanceIDs.
	var res = make([]placement.Instance, len(groups))
	for _, group := range groups {
		idx := findIndex(leavingInstanceIDs, group.Leaving.ID())
		if idx == -1 {
			return nil, fmt.Errorf(
				"could not find instance id: '%s' in leaving instances", group.Leaving.ID())
		}

		res[idx] = group.Replacement.SetShardSetID(group.Leaving.ShardSetID())
	}
	return res, nil
}

func getLeavingInstances(
	p placement.Placement,
	leavingInstanceIDs []string,
) ([]placement.Instance, error) {
	leavingInstances := make([]placement.Instance, 0, len(leavingInstanceIDs))
	for _, id := range leavingInstanceIDs {
		leavingInstance, exist := p.Instance(id)
		if !exist {
			return nil, errInstanceAbsent
		}
		leavingInstances = append(leavingInstances, leavingInstance)
	}
	return leavingInstances, nil
}

func findIndex(ids []string, id string) int {
	for i := range ids {
		if ids[i] == id {
			return i
		}
	}
	// Unexpected.
	return -1
}

func groupHostsByWeight(candidates []placement.Instance) (map[uint32][]host, error) {
	var (
		uniqueHosts      = make(map[string]host, len(candidates))
		weightToHostsMap = make(map[uint32][]host, len(candidates))
	)
	for _, instance := range candidates {
		hostname := instance.Hostname()
		weight := instance.Weight()
		h, ok := uniqueHosts[hostname]
		if !ok {
			h = newHost(hostname, instance.IsolationGroup(), weight)
			uniqueHosts[hostname] = h
			weightToHostsMap[weight] = append(weightToHostsMap[weight], h)
		}
		err := h.addInstance(instance.Port(), instance)
		if err != nil {
			return nil, err
		}
	}
	return weightToHostsMap, nil
}

// groupHostsWithIsolationGroupCheck looks at the isolation groups of the given hosts
// and try to make as many groups as possible. The hosts in each group
// must come from different isolation groups.
func groupHostsWithIsolationGroupCheck(hosts []host, rf int) (groups [][]host, ungrouped []host) {
	if len(hosts) < rf {
		// When the number of hosts is less than rf, no groups can be made.
		return nil, hosts
	}

	var (
		uniqIGs = make(map[string]*group, len(hosts))
		rh      = groupsByNumHost(make([]*group, 0, len(hosts)))
	)
	for _, h := range hosts {
		r, ok := uniqIGs[h.isolationGroup]
		if !ok {
			r = &group{
				isolationGroup: h.isolationGroup,
				hosts:          make([]host, 0, rf),
			}

			uniqIGs[h.isolationGroup] = r
			rh = append(rh, r)
		}
		r.hosts = append(r.hosts, h)
	}

	heap.Init(&rh)

	// For each group, always prefer to find one host from the largest isolation group
	// in the heap. After a group is filled, push all the checked isolation groups back
	// to the heap so they can be used for the next group.
	groups = make([][]host, 0, int(math.Ceil(float64(len(hosts))/float64(rf))))
	for rh.Len() >= rf {
		// When there are more than rf isolation groups available, try to make a group.
		seenIGs := make(map[string]*group, rf)
		g := make([]host, 0, rf)
		for i := 0; i < rf; i++ {
			r := heap.Pop(&rh).(*group)
			// Move the host from the isolation group to the group.
			// The isolation groups in the heap always have at least one host.
			g = append(g, r.hosts[len(r.hosts)-1])
			r.hosts = r.hosts[:len(r.hosts)-1]
			seenIGs[r.isolationGroup] = r
		}
		if len(g) == rf {
			groups = append(groups, g)
		}
		for _, r := range seenIGs {
			if len(r.hosts) > 0 {
				heap.Push(&rh, r)
			}
		}
	}

	ungrouped = make([]host, 0, rh.Len())
	for _, r := range rh {
		ungrouped = append(ungrouped, r.hosts...)
	}
	return groups, ungrouped
}

func groupInstancesByHostPort(hostGroups [][]host) ([][]placement.Instance, error) {
	var instanceGroups = make([][]placement.Instance, 0, len(hostGroups))
	for _, hostGroup := range hostGroups {
		for port, instance := range hostGroup[0].portToInstance {
			instanceGroup := make([]placement.Instance, 0, len(hostGroup))
			instanceGroup = append(instanceGroup, instance)
			for _, otherHost := range hostGroup[1:] {
				otherInstance, ok := otherHost.portToInstance[port]
				if !ok {
					return nil, fmt.Errorf("could not find port %d on host %s", port, otherHost.name)
				}
				instanceGroup = append(instanceGroup, otherInstance)
			}
			instanceGroups = append(instanceGroups, instanceGroup)
		}
	}
	return instanceGroups, nil
}

// assignShardsetsToGroupedInstances is a helper for mirrored selectors, which assigns shardset
// IDs to the given groups.
func assignShardsetsToGroupedInstances(
	groups [][]placement.Instance,
	p placement.Placement,
) []placement.Instance {
	var (
		instances      = make([]placement.Instance, 0, p.ReplicaFactor()*len(groups))
		currShardSetID = p.MaxShardSetID() + 1
		ssID           uint32
	)
	for _, group := range groups {
		useNewSSID := shouldUseNewShardSetID(group, p)

		if useNewSSID {
			ssID = currShardSetID
			currShardSetID++
		}
		for _, instance := range group {
			if useNewSSID {
				instance = instance.SetShardSetID(ssID)
			}
			instances = append(instances, instance)
		}
	}
	return instances
}

func shouldUseNewShardSetID(
	group []placement.Instance,
	p placement.Placement,
) bool {
	var seenSSID *uint32
	for _, instance := range group {
		instanceInPlacement, exist := p.Instance(instance.ID())
		if !exist {
			return true
		}
		currentSSID := instanceInPlacement.ShardSetID()
		if seenSSID == nil {
			seenSSID = &currentSSID
			continue
		}
		if *seenSSID != currentSSID {
			return true
		}
	}
	return false
}

type host struct {
	name           string
	isolationGroup string
	weight         uint32
	portToInstance map[uint32]placement.Instance
}

func newHost(name, isolationGroup string, weight uint32) host {
	return host{
		name:           name,
		isolationGroup: isolationGroup,
		weight:         weight,
		portToInstance: make(map[uint32]placement.Instance),
	}
}

func (h host) addInstance(port uint32, instance placement.Instance) error {
	if h.weight != instance.Weight() {
		return fmt.Errorf("could not add instance %s to host %s, weight mismatch: %d and %d",
			instance.ID(), h.name, instance.Weight(), h.weight)
	}
	if h.isolationGroup != instance.IsolationGroup() {
		return fmt.Errorf("could not add instance %s to host %s, isolation group mismatch: %s and %s",
			instance.ID(), h.name, instance.IsolationGroup(), h.isolationGroup)
	}
	h.portToInstance[port] = instance
	return nil
}

type group struct {
	isolationGroup string
	hosts          []host
}

type groupsByNumHost []*group

func (h groupsByNumHost) Len() int {
	return len(h)
}

func (h groupsByNumHost) Less(i, j int) bool {
	return len(h[i].hosts) > len(h[j].hosts)
}

func (h groupsByNumHost) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *groupsByNumHost) Push(i interface{}) {
	r := i.(*group)
	*h = append(*h, r)
}

func (h *groupsByNumHost) Pop() interface{} {
	old := *h
	n := len(old)
	g := old[n-1]
	*h = old[0 : n-1]
	return g
}
