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

package namespace

import (
	"fmt"
	"sync"

	xclose "github.com/m3db/m3/src/x/close"
	"github.com/m3db/m3/src/x/ident"
	xwatch "github.com/m3db/m3/src/x/watch"

	"go.uber.org/zap"
)

func newSchemaHistoryNotFoundError(nsIDStr string) error {
	return &schemaHistoryNotFoundError{nsIDStr}
}

type schemaHistoryNotFoundError struct {
	nsIDStr string
}

func (s *schemaHistoryNotFoundError) Error() string {
	return fmt.Sprintf("schema history is not found for %s", s.nsIDStr)
}

type schemaRegistry struct {
	sync.RWMutex

	protoEnabled bool
	logger       *zap.Logger
	registry     map[string]xwatch.Watchable
}

func NewSchemaRegistry(protoEnabled bool, logger *zap.Logger) SchemaRegistry {
	return newSchemaRegistry(protoEnabled, logger)
}

func newSchemaRegistry(protoEnabled bool, logger *zap.Logger) SchemaRegistry {
	return &schemaRegistry{
		protoEnabled: protoEnabled,
		logger:       logger,
		registry:     make(map[string]xwatch.Watchable),
	}
}

func (sr *schemaRegistry) SetSchemaHistory(id ident.ID, history SchemaHistory) error {
	if !sr.protoEnabled {
		if sr.logger != nil {
			sr.logger.Warn("proto is not enabled, can not update schema registry",
				zap.Stringer("namespace", id))
		}
		return nil
	}

	if newSchema, ok := history.GetLatest(); !ok {
		return fmt.Errorf("can not set empty schema history for %v", id.String())
	} else if sr.logger != nil {
		sr.logger.Info("proto is enabled, setting schema",
			zap.Stringer("namespace", id),
			zap.String("version", newSchema.DeployId()))
	}

	sr.Lock()
	defer sr.Unlock()

	// TODO [haijun] use generated map for optimized map lookup.
	current, ok := sr.registry[id.String()]
	if ok {
		if !history.Extends(current.Get().(SchemaHistory)) {
			return fmt.Errorf("can not update schema registry to one that does not extends the existing one")
		}
	} else {
		sr.registry[id.String()] = xwatch.NewWatchable()
	}

	sr.registry[id.String()].Update(history)
	return nil
}

func (sr *schemaRegistry) GetLatestSchema(id ident.ID) (SchemaDescr, error) {
	if !sr.protoEnabled {
		return nil, nil
	}

	nsIDStr := id.String()
	history, err := sr.getSchemaHistory(nsIDStr)
	if err != nil {
		return nil, err
	}
	schema, ok := history.GetLatest()
	if !ok {
		return nil, fmt.Errorf("schema history is empty for namespace %v", nsIDStr)
	}
	return schema, nil
}

func (sr *schemaRegistry) GetSchema(id ident.ID, schemaId string) (SchemaDescr, error) {
	if !sr.protoEnabled {
		return nil, nil
	}

	nsIDStr := id.String()
	history, err := sr.getSchemaHistory(nsIDStr)
	if err != nil {
		return nil, err
	}
	schema, ok := history.Get(schemaId)
	if !ok {
		return nil, fmt.Errorf("schema of version %v is not found for namespace %v", schemaId, nsIDStr)
	}
	return schema, nil
}

func (sr *schemaRegistry) getSchemaHistory(nsIDStr string) (SchemaHistory, error) {
	sr.RLock()
	defer sr.RUnlock()

	history, ok := sr.registry[nsIDStr]
	if !ok {
		return nil, newSchemaHistoryNotFoundError(nsIDStr)
	}
	return history.Get().(SchemaHistory), nil
}

func (sr *schemaRegistry) RegisterListener(
	nsID ident.ID,
	listener SchemaListener,
) (xclose.SimpleCloser, error) {
	if !sr.protoEnabled {
		return nil, nil
	}

	nsIDStr := nsID.String()
	sr.RLock()
	defer sr.RUnlock()

	watchable, ok := sr.registry[nsIDStr]
	if !ok {
		return nil, fmt.Errorf("schema not found for namespace: %v", nsIDStr)
	}

	_, watch, _ := watchable.Watch()

	// We always initialize the watchable so always read
	// the first notification value
	<-watch.C()

	// Deliver the current schema
	listener.SetSchemaHistory(watchable.Get().(SchemaHistory))

	// Spawn a new goroutine that will terminate when the
	// watchable terminates on the close of the runtime options manager
	go func() {
		for range watch.C() {
			listener.SetSchemaHistory(watchable.Get().(SchemaHistory))
		}
	}()

	return watch, nil
}

func (sr *schemaRegistry) Close() {
	sr.Lock()
	defer sr.Unlock()
	for _, w := range sr.registry {
		w.Close()
	}
}
