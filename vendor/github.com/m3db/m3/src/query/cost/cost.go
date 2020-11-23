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

package cost

import (
	"errors"
	"fmt"

	"github.com/m3db/m3/src/x/cost"
)

const (
	// BlockLevel identifies per-block enforcers.
	BlockLevel = "block"
	// QueryLevel identifies per-query enforcers.
	QueryLevel = "query"
	// GlobalLevel identifies global enforcers.
	GlobalLevel = "global"
)

// ChainedEnforcer is a cost.Enforcer implementation which tracks resource usage implements cost.Enforcer to enforce
// limits on multiple resources at once, linked together in a tree.
type ChainedEnforcer interface {
	cost.Enforcer

	// Child creates a new ChainedEnforcer which rolls up to this one.
	Child(resourceName string) ChainedEnforcer

	// Close indicates that all resources have been returned for this
	// ChainedEnforcer. It should inform all parent enforcers that the
	// resources have been freed.
	Close()
}

type noopChainedReporter struct{}

func (noopChainedReporter) ReportCost(_ cost.Cost)    {}
func (noopChainedReporter) ReportCurrent(_ cost.Cost) {}
func (noopChainedReporter) ReportOverLimit(_ bool)    {}
func (noopChainedReporter) OnChildClose(_ cost.Cost)  {}
func (noopChainedReporter) OnClose(_ cost.Cost)       {}

var noopChainedReporterInstance = noopChainedReporter{}

// ChainedReporter is a listener for chainedEnforcer methods, which listens to Close events in addition to
// events used by cost.EnforcerReporter.
type ChainedReporter interface {
	cost.EnforcerReporter

	// OnChildClose is called whenever a child of this reporter's chainedEnforcer is released.
	OnChildClose(currentCost cost.Cost)

	// OnClose is called whenever this reporter's chainedEnforcer is released.
	OnClose(currentCost cost.Cost)
}

// chainedEnforcer is the actual implementation of ChainedEnforcer.
type chainedEnforcer struct {
	resourceName string
	local        cost.Enforcer
	parent       *chainedEnforcer
	models       []cost.Enforcer
	reporter     ChainedReporter
}

var noopChainedEnforcer = mustNoopChainedEnforcer()

func mustNoopChainedEnforcer() ChainedEnforcer {
	rtn, err := NewChainedEnforcer("", []cost.Enforcer{cost.NoopEnforcer()})
	if err != nil {
		panic(err.Error())
	}

	return rtn
}

// NoopChainedEnforcer returns a chainedEnforcer which enforces no limits and does no reporting.
func NoopChainedEnforcer() ChainedEnforcer {
	return noopChainedEnforcer
}

// NewChainedEnforcer constructs a chainedEnforcer which creates children using the provided models.
// models[0] enforces this instance; models[1] enforces the first level of children, and so on.
func NewChainedEnforcer(rootResourceName string, models []cost.Enforcer) (ChainedEnforcer, error) {
	if len(models) == 0 {
		return nil, errors.New("must provide at least one Enforcer instance for a chainedEnforcer")
	}

	local := models[0]

	return &chainedEnforcer{
		resourceName: rootResourceName,
		parent:       nil, // root has nil parent
		local:        local,
		models:       models[1:],
		reporter:     upcastReporterOrNoop(local.Reporter()),
	}, nil
}

func upcastReporterOrNoop(r cost.EnforcerReporter) ChainedReporter {
	if r, ok := r.(ChainedReporter); ok {
		return r
	}

	return noopChainedReporterInstance
}

// Add adds the given cost both to this enforcer and any parents, working recursively until the root is reached.
// The most local error is preferred.
func (ce *chainedEnforcer) Add(c cost.Cost) cost.Report {
	if ce.parent == nil {
		return ce.wrapLocalResult(ce.local.Add(c))
	}

	localR := ce.local.Add(c)
	globalR := ce.parent.Add(c)

	// check our local limit first
	if localR.Error != nil {
		return ce.wrapLocalResult(localR)
	}

	// check the global limit
	if globalR.Error != nil {
		return globalR
	}

	return localR
}

func (ce *chainedEnforcer) wrapLocalResult(localR cost.Report) cost.Report {
	if localR.Error != nil {
		return cost.Report{
			Cost:  localR.Cost,
			Error: fmt.Errorf("exceeded %s limit: %s", ce.resourceName, localR.Error.Error()),
		}
	}
	return localR
}

// Child creates a new chainedEnforcer whose resource consumption rolls up into this instance.
func (ce *chainedEnforcer) Child(resourceName string) ChainedEnforcer {
	// no more models; just return a noop default. TODO: this could be a panic case? Technically speaking it's
	// misconfiguration.
	if len(ce.models) == 0 {
		return NoopChainedEnforcer()
	}

	newLocal := ce.models[0]
	return &chainedEnforcer{
		resourceName: resourceName,
		parent:       ce,
		// make sure to clone the local enforcer, so that we're using an
		// independent instance with the same configuration.
		local:    newLocal.Clone(),
		models:   ce.models[1:],
		reporter: upcastReporterOrNoop(newLocal.Reporter()),
	}
}

// Clone on a chainedEnforcer is a noop--TODO: implement?
func (ce *chainedEnforcer) Clone() cost.Enforcer {
	return ce
}

// State returns the local state of this enforcer (ignoring anything further up the chain).
func (ce *chainedEnforcer) State() (cost.Report, cost.Limit) {
	return ce.local.State()
}

// Close releases all resources tracked by this enforcer back to the global enforcer
func (ce *chainedEnforcer) Close() {
	r, _ := ce.local.State()
	ce.reporter.OnClose(r.Cost)

	if ce.parent != nil {
		parentR, _ := ce.parent.State()
		ce.parent.reporter.OnChildClose(parentR.Cost)
	}

	ce.Add(-r.Cost)
}

func (ce *chainedEnforcer) Limit() cost.Limit {
	return ce.local.Limit()
}

func (ce *chainedEnforcer) Reporter() cost.EnforcerReporter {
	return ce.local.Reporter()
}
