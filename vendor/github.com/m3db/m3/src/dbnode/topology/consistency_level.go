// Copyright (c) 2018 Uber Technologies, Inc.
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

package topology

import (
	"errors"
	"fmt"
	"strings"
)

// ConsistencyLevel is the consistency level for cluster operations
type ConsistencyLevel int

// nolint: varcheck, unused
const (
	consistencyLevelNone ConsistencyLevel = iota

	// ConsistencyLevelOne corresponds to a single node participating
	// for an operation to succeed
	ConsistencyLevelOne

	// ConsistencyLevelMajority corresponds to the majority of nodes participating
	// for an operation to succeed
	ConsistencyLevelMajority

	// ConsistencyLevelAll corresponds to all nodes participating
	// for an operation to succeed
	ConsistencyLevelAll
)

// String returns the consistency level as a string
func (l ConsistencyLevel) String() string {
	switch l {
	case consistencyLevelNone:
		return none
	case ConsistencyLevelOne:
		return one
	case ConsistencyLevelMajority:
		return majority
	case ConsistencyLevelAll:
		return all
	}
	return unknown
}

var validConsistencyLevels = []ConsistencyLevel{
	consistencyLevelNone,
	ConsistencyLevelOne,
	ConsistencyLevelMajority,
	ConsistencyLevelAll,
}

var (
	errConsistencyLevelUnspecified = errors.New("consistency level not specified")
	errConsistencyLevelInvalid     = errors.New("consistency level invalid")
)

// ValidConsistencyLevels returns a copy of valid consistency levels
// to avoid callers mutating the set of valid read consistency levels
func ValidConsistencyLevels() []ConsistencyLevel {
	result := make([]ConsistencyLevel, len(validConsistencyLevels))
	copy(result, validConsistencyLevels)
	return result
}

// ValidateConsistencyLevel returns nil when consistency level is valid,
// otherwise it returns an error
func ValidateConsistencyLevel(v ConsistencyLevel) error {
	for _, level := range validConsistencyLevels {
		if level == v {
			return nil
		}
	}
	return errConsistencyLevelInvalid
}

// UnmarshalYAML unmarshals an ConnectConsistencyLevel into a valid type from string.
func (l *ConsistencyLevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	if str == "" {
		return errConsistencyLevelUnspecified
	}
	strs := make([]string, 0, len(validConsistencyLevels))
	for _, valid := range validConsistencyLevels {
		if str == valid.String() {
			*l = valid
			return nil
		}
		strs = append(strs, "'"+valid.String()+"'")
	}
	return fmt.Errorf("invalid ConsistencyLevel '%s' valid types are: %s",
		str, strings.Join(strs, ", "))
}

// ConnectConsistencyLevel is the consistency level for connecting to a cluster
type ConnectConsistencyLevel int

const (
	// ConnectConsistencyLevelAny corresponds to connecting to any number of nodes for a given shard
	// set, this strategy will attempt to connect to all, then the majority, then one and then none.
	ConnectConsistencyLevelAny ConnectConsistencyLevel = iota

	// ConnectConsistencyLevelNone corresponds to connecting to no nodes for a given shard set
	ConnectConsistencyLevelNone

	// ConnectConsistencyLevelOne corresponds to connecting to a single node for a given shard set
	ConnectConsistencyLevelOne

	// ConnectConsistencyLevelMajority corresponds to connecting to the majority of nodes for a given shard set
	ConnectConsistencyLevelMajority

	// ConnectConsistencyLevelAll corresponds to connecting to all of the nodes for a given shard set
	ConnectConsistencyLevelAll
)

// String returns the consistency level as a string
func (l ConnectConsistencyLevel) String() string {
	switch l {
	case ConnectConsistencyLevelAny:
		return any
	case ConnectConsistencyLevelNone:
		return none
	case ConnectConsistencyLevelOne:
		return one
	case ConnectConsistencyLevelMajority:
		return majority
	case ConnectConsistencyLevelAll:
		return all
	}
	return unknown
}

var validConnectConsistencyLevels = []ConnectConsistencyLevel{
	ConnectConsistencyLevelAny,
	ConnectConsistencyLevelNone,
	ConnectConsistencyLevelOne,
	ConnectConsistencyLevelMajority,
	ConnectConsistencyLevelAll,
}

var (
	errClusterConnectConsistencyLevelUnspecified = errors.New("cluster connect consistency level not specified")
	errClusterConnectConsistencyLevelInvalid     = errors.New("cluster connect consistency level invalid")
)

// ValidConnectConsistencyLevels returns a copy of valid consistency levels
// to avoid callers mutating the set of valid read consistency levels
func ValidConnectConsistencyLevels() []ConnectConsistencyLevel {
	result := make([]ConnectConsistencyLevel, len(validConnectConsistencyLevels))
	copy(result, validConnectConsistencyLevels)
	return result
}

// ValidateConnectConsistencyLevel returns nil when consistency level is valid,
// otherwise it returns an error
func ValidateConnectConsistencyLevel(v ConnectConsistencyLevel) error {
	for _, level := range validConnectConsistencyLevels {
		if level == v {
			return nil
		}
	}
	return errClusterConnectConsistencyLevelInvalid
}

// UnmarshalYAML unmarshals an ConnectConsistencyLevel into a valid type from string.
func (l *ConnectConsistencyLevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	if str == "" {
		return errClusterConnectConsistencyLevelUnspecified
	}
	strs := make([]string, 0, len(validConnectConsistencyLevels))
	for _, valid := range validConnectConsistencyLevels {
		if str == valid.String() {
			*l = valid
			return nil
		}
		strs = append(strs, "'"+valid.String()+"'")
	}
	return fmt.Errorf("invalid ConnectConsistencyLevel '%s' valid types are: %s",
		str, strings.Join(strs, ", "))
}

// ReadConsistencyLevel is the consistency level for reading from a cluster
type ReadConsistencyLevel int

const (
	// ReadConsistencyLevelNone corresponds to reading from no nodes
	ReadConsistencyLevelNone ReadConsistencyLevel = iota

	// ReadConsistencyLevelOne corresponds to reading from a single node
	ReadConsistencyLevelOne

	// ReadConsistencyLevelUnstrictMajority corresponds to reading from the majority of nodes
	// but relaxing the constraint when it cannot be met, falling back to returning success when
	// reading from at least a single node after attempting reading from the majority of nodes
	ReadConsistencyLevelUnstrictMajority

	// ReadConsistencyLevelMajority corresponds to reading from the majority of nodes
	ReadConsistencyLevelMajority

	// ReadConsistencyLevelUnstrictAll corresponds to reading from all nodes
	// but relaxing the constraint when it cannot be met, falling back to returning success when
	// reading from at least a single node after attempting reading from all of nodes
	ReadConsistencyLevelUnstrictAll

	// ReadConsistencyLevelAll corresponds to reading from all of the nodes
	ReadConsistencyLevelAll
)

// String returns the consistency level as a string
func (l ReadConsistencyLevel) String() string {
	switch l {
	case ReadConsistencyLevelNone:
		return none
	case ReadConsistencyLevelOne:
		return one
	case ReadConsistencyLevelUnstrictMajority:
		return unstrictMajority
	case ReadConsistencyLevelMajority:
		return majority
	case ReadConsistencyLevelUnstrictAll:
		return unstrictAll
	case ReadConsistencyLevelAll:
		return all
	}
	return unknown
}

var validReadConsistencyLevels = []ReadConsistencyLevel{
	ReadConsistencyLevelNone,
	ReadConsistencyLevelOne,
	ReadConsistencyLevelUnstrictMajority,
	ReadConsistencyLevelMajority,
	ReadConsistencyLevelUnstrictAll,
	ReadConsistencyLevelAll,
}

var (
	errReadConsistencyLevelUnspecified = errors.New("read consistency level not specified")
	errReadConsistencyLevelInvalid     = errors.New("read consistency level invalid")
)

// ValidReadConsistencyLevels returns a copy of valid consistency levels
// to avoid callers mutating the set of valid read consistency levels
func ValidReadConsistencyLevels() []ReadConsistencyLevel {
	result := make([]ReadConsistencyLevel, len(validReadConsistencyLevels))
	copy(result, validReadConsistencyLevels)
	return result
}

// ValidateReadConsistencyLevel returns nil when consistency level is valid,
// otherwise it returns an error
func ValidateReadConsistencyLevel(v ReadConsistencyLevel) error {
	for _, level := range validReadConsistencyLevels {
		if level == v {
			return nil
		}
	}
	return errReadConsistencyLevelInvalid
}

// UnmarshalYAML unmarshals an ConnectConsistencyLevel into a valid type from string.
func (l *ReadConsistencyLevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	if str == "" {
		return errReadConsistencyLevelUnspecified
	}
	strs := make([]string, 0, len(validReadConsistencyLevels))
	for _, valid := range validReadConsistencyLevels {
		if str == valid.String() {
			*l = valid
			return nil
		}
		strs = append(strs, "'"+valid.String()+"'")
	}
	return fmt.Errorf("invalid ReadConsistencyLevel '%s' valid types are: %s",
		str, strings.Join(strs, ", "))
}

// string constants, required to fix lint complaining about
// multiple occurrences of same literal string...
const (
	unknown          = "unknown"
	any              = "any"
	all              = "all"
	unstrictAll      = "unstrict_all"
	one              = "one"
	none             = "none"
	majority         = "majority"
	unstrictMajority = "unstrict_majority"
)

// WriteConsistencyAchieved returns a bool indicating whether or not we've received enough
// successful acks to consider a write successful based on the specified consistency level.
func WriteConsistencyAchieved(
	level ConsistencyLevel,
	majority, numPeers, numSuccess int,
) bool {
	switch level {
	case ConsistencyLevelAll:
		if numSuccess == numPeers { // Meets all
			return true
		}
		return false
	case ConsistencyLevelMajority:
		if numSuccess >= majority { // Meets majority
			return true
		}
		return false
	case ConsistencyLevelOne:
		if numSuccess > 0 { // Meets one
			return true
		}
		return false
	}
	panic(fmt.Errorf("unrecognized consistency level: %s", level.String()))
}

// ReadConsistencyTermination returns a bool to indicate whether sufficient
// responses (error/success) have been received, so that we're able to decide
// whether we will be able to satisfy the reuquest or not.
// NB: it is not the same as `readConsistencyAchieved`.
func ReadConsistencyTermination(
	level ReadConsistencyLevel,
	majority, remaining, success int32,
) bool {
	doneAll := remaining == 0
	switch level {
	case ReadConsistencyLevelOne, ReadConsistencyLevelNone:
		return success > 0 || doneAll
	case ReadConsistencyLevelMajority, ReadConsistencyLevelUnstrictMajority:
		return success >= majority || doneAll
	case ReadConsistencyLevelAll, ReadConsistencyLevelUnstrictAll:
		return doneAll
	}
	panic(fmt.Errorf("unrecognized consistency level: %s", level.String()))
}

// ReadConsistencyAchieved returns whether sufficient responses have been received
// to reach the desired consistency.
// NB: it is not the same as `readConsistencyTermination`.
func ReadConsistencyAchieved(
	level ReadConsistencyLevel,
	majority, numPeers, numSuccess int,
) bool {
	switch level {
	case ReadConsistencyLevelAll:
		return numSuccess == numPeers // Meets all
	case ReadConsistencyLevelMajority:
		return numSuccess >= majority // Meets majority
	case ReadConsistencyLevelOne, ReadConsistencyLevelUnstrictMajority, ReadConsistencyLevelUnstrictAll:
		return numSuccess > 0 // Meets one
	case ReadConsistencyLevelNone:
		return true // Always meets none
	}
	panic(fmt.Errorf("unrecognized consistency level: %s", level.String()))
}

// NumDesiredForReadConsistency returns the number of replicas that would ideally be used to
// satisfy the read consistency.
func NumDesiredForReadConsistency(level ReadConsistencyLevel, numReplicas, majority int) int {
	switch level {
	case ReadConsistencyLevelAll, ReadConsistencyLevelUnstrictAll:
		return numReplicas
	case ReadConsistencyLevelMajority, ReadConsistencyLevelUnstrictMajority:
		return majority
	case ReadConsistencyLevelOne:
		return 1
	case ReadConsistencyLevelNone:
		return 0
	}
	panic(fmt.Errorf("unrecognized consistency level: %s", level.String()))
}
