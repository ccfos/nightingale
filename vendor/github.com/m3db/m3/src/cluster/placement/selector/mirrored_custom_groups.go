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

package selector

import (
	"fmt"

	"github.com/m3db/m3/src/cluster/placement"
	"github.com/m3db/m3/src/x/errors"

	"go.uber.org/zap"
)

type mirroredCustomGroupSelector struct {
	instanceIDToGroupID InstanceGroupIDFunc
	logger              *zap.Logger
	opts            placement.Options
}

// InstanceGroupIDFunc maps an instance to its mirrored group.
type InstanceGroupIDFunc func(inst placement.Instance) (string, error)

// NewMapInstanceGroupIDFunc creates a simple lookup function for an instances group, which
// looks up the group ID for an instance by the instance ID.
func NewMapInstanceGroupIDFunc(instanceToGroup map[string]string) InstanceGroupIDFunc {
	return func(inst placement.Instance) (string, error) {
		gid, ok := instanceToGroup[inst.ID()]
		if !ok {
			return "", fmt.Errorf(
				"instance %s doesn't have a corresponding group in ID to group map",
				inst.ID(),
			)
		}
		return gid, nil
	}
}

// NewMirroredCustomGroupSelector constructs a placement.InstanceSelector which assigns shardsets
// according to their group ID (provided by instanceToGroupID). That is, instances with the
// same group ID are assigned the same shardset ID, and will receive the same mirrored traffic.
func NewMirroredCustomGroupSelector(
	instanceToGroupID InstanceGroupIDFunc,
	opts placement.Options,
) placement.InstanceSelector {
	return &mirroredCustomGroupSelector{
		logger:              opts.InstrumentOptions().Logger(),
		instanceIDToGroupID: instanceToGroupID,
		opts:                opts,
	}
}

func (e *mirroredCustomGroupSelector) SelectInitialInstances(
	candidates []placement.Instance,
	rf int,
) ([]placement.Instance, error) {
	return e.selectInstances(
		candidates,
		placement.NewPlacement().SetReplicaFactor(rf),
		true,
	)
}

func (e *mirroredCustomGroupSelector) SelectAddingInstances(
	candidates []placement.Instance,
	p placement.Placement,
) ([]placement.Instance, error) {
	return e.selectInstances(candidates, p, e.opts.AddAllCandidates())
}

// SelectReplaceInstances attempts to find a replacement instance in the same group
// for each of the leavingInstances
func (e *mirroredCustomGroupSelector) SelectReplaceInstances(
	candidates []placement.Instance,
	leavingInstanceIDs []string,
	p placement.Placement,
) ([]placement.Instance, error) {
	candidates, err := getValidCandidates(p, candidates, e.opts)
	if err != nil {
	    return nil, err
	}

	// find a replacement for each leaving instance.
	candidatesByGroup, err := e.groupInstancesByID(candidates)
	if err != nil {
		return nil, err
	}

	leavingInstances, err := getLeavingInstances(p, leavingInstanceIDs)
	if err != nil {
		return nil, err
	}

	replacementGroups := make([]mirroredReplacementGroup, 0, len(leavingInstances))
	for _, leavingInstance := range leavingInstances {
		// try to find an instance in the same group as the leaving instance.
		groupID, err := e.getGroup(leavingInstance)
		if err != nil {
			return nil, err
		}

		replacementGroup := candidatesByGroup[groupID]
		if len(replacementGroup) == 0 {
			return nil, newErrNoValidReplacement(leavingInstance.ID(), groupID)
		}

		replacementNode := replacementGroup[len(replacementGroup)-1]
		candidatesByGroup[groupID] = replacementGroup[:len(replacementGroup)-1]

		replacementGroups = append(
			replacementGroups,
			mirroredReplacementGroup{
				Leaving:     leavingInstance,
				Replacement: replacementNode,
			})
	}

	return assignShardsetIDsToReplacements(leavingInstanceIDs, replacementGroups)
}

type mirroredReplacementGroup struct {
	Leaving     placement.Instance
	Replacement placement.Instance
}

// selectInstances does the actual work of the class. It groups candidate instances by their
// group ID, and assigns them shardsets.
// N.B. (amains): addAllInstances is a parameter here (instead of using e.opts) because it
// only applies to the SelectAddingInstances case.
func (e *mirroredCustomGroupSelector) selectInstances(
	candidates []placement.Instance,
	p placement.Placement,
	addAllInstances bool,
) ([]placement.Instance, error) {
	candidates, err := getValidCandidates(p, candidates, e.opts)
	if err != nil {
		return nil, err
	}

	groups, err := e.groupWithRF(candidates, p.ReplicaFactor())
	if err != nil {
		return nil, err
	}

	// no groups => no instances
	if len(groups) == 0 {
		return nil, nil
	}

	if !addAllInstances {
		groups = groups[:1]
	}
	return assignShardsetsToGroupedInstances(groups, p), nil
}

func (e *mirroredCustomGroupSelector) groupWithRF(
	candidates []placement.Instance,
	rf int) ([][]placement.Instance, error) {
	byGroupID, err := e.groupInstancesByID(candidates)
	if err != nil {
		return nil, err
	}

	groups := make([][]placement.Instance, 0, len(byGroupID))
	// validate and convert to slice
	for groupID, group := range byGroupID {
		if len(group) > rf {
			fullGroup := group
			group = group[:rf]

			var droppedIDs []string
			for _, dropped := range fullGroup[rf:] {
				droppedIDs = append(droppedIDs, dropped.ID())
			}
			e.logger.Warn(
				"mirroredCustomGroupSelector: found more hosts than RF in group; "+
					"using only RF hosts",
				zap.Strings("droppedIDs", droppedIDs),
				zap.String("groupID", groupID),
			)
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (e *mirroredCustomGroupSelector) groupInstancesByID(candidates []placement.Instance) (map[string][]placement.Instance, error) {
	byGroupID := make(map[string][]placement.Instance)
	for _, candidate := range candidates {
		groupID, err := e.getGroup(candidate)
		if err != nil {
			return nil, err
		}

		byGroupID[groupID] = append(byGroupID[groupID], candidate)
	}
	return byGroupID, nil
}

// small wrapper around e.instanceIDToGroupID providing context on error.
func (e *mirroredCustomGroupSelector) getGroup(inst placement.Instance) (string, error) {
	groupID, err := e.instanceIDToGroupID(inst)
	if err != nil {
		return "", errors.Wrapf(err, "finding group for %s", inst.ID())
	}
	return groupID, nil
}

func newErrNoValidReplacement(leavingInstID string, groupID string) error {
	return fmt.Errorf(
		"leaving instance %s has no valid replacements in the same group (%s)",
		leavingInstID,
		groupID,
	)
}
