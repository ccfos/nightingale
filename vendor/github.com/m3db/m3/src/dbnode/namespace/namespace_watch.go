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

package namespace

import (
	"errors"
	"sync"
	"time"

	"github.com/m3db/m3/src/x/instrument"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

var (
	errAlreadyWatching = errors.New("database is already watching namespace updates")
	errNotWatching     = errors.New("database is not watching for namespace updates")
)

type dbNamespaceWatch struct {
	sync.Mutex

	watching bool
	watch    Watch
	doneCh   chan struct{}
	closedCh chan struct{}

	update NamespaceUpdater

	log            *zap.Logger
	metrics        dbNamespaceWatchMetrics
	reportInterval time.Duration
}

type dbNamespaceWatchMetrics struct {
	activeNamespaces tally.Gauge
	updates          tally.Counter
}

func newWatchMetrics(
	scope tally.Scope,
) dbNamespaceWatchMetrics {
	nsScope := scope.SubScope("namespace-watch")
	return dbNamespaceWatchMetrics{
		activeNamespaces: nsScope.Gauge("active"),
		updates:          nsScope.Counter("updates"),
	}
}

func NewNamespaceWatch(
	update NamespaceUpdater,
	w Watch,
	iopts instrument.Options,
) NamespaceWatch {
	scope := iopts.MetricsScope()
	return &dbNamespaceWatch{
		watch:          w,
		update:         update,
		log:            iopts.Logger(),
		metrics:        newWatchMetrics(scope),
		reportInterval: iopts.ReportInterval(),
	}
}

func (w *dbNamespaceWatch) Start() error {
	w.Lock()
	defer w.Unlock()

	if w.watching {
		return errAlreadyWatching
	}

	w.watching = true
	w.doneCh = make(chan struct{}, 1)
	w.closedCh = make(chan struct{}, 1)

	go w.startWatch()

	return nil
}

func (w *dbNamespaceWatch) startWatch() {
	reportClosingCh := make(chan struct{}, 1)
	reportClosedCh := make(chan struct{}, 1)
	go func() {
		ticker := time.NewTicker(w.reportInterval)
		defer func() {
			ticker.Stop()
			close(reportClosedCh)
		}()
		for {
			select {
			case <-ticker.C:
				w.reportMetrics()
			case <-reportClosingCh:
				return
			}
		}
	}()

	defer func() {
		// Issue closing signal to report channel
		close(reportClosingCh)
		// Wait for report channel to close
		<-reportClosedCh
		// Signal all closed
		close(w.closedCh)
	}()

	for {
		select {
		case <-w.doneCh:
			return
		case _, ok := <-w.watch.C():
			if !ok {
				return
			}

			if !w.isWatching() {
				return
			}

			w.metrics.updates.Inc(1)
			newMap := w.watch.Get()
			w.log.Info("received update from kv namespace watch")
			if err := w.update(newMap); err != nil {
				w.log.Error("failed to update owned namespaces",
					zap.Error(err))
			}
		}
	}
}

func (w *dbNamespaceWatch) isWatching() bool {
	w.Lock()
	defer w.Unlock()
	return w.watching
}

func (w *dbNamespaceWatch) reportMetrics() {
	m := w.watch.Get()
	if m == nil {
		w.metrics.activeNamespaces.Update(0)
		return
	}
	w.metrics.activeNamespaces.Update(float64(len(m.Metadatas())))
}

func (w *dbNamespaceWatch) Stop() error {
	w.Lock()
	defer w.Unlock()

	if !w.watching {
		return errNotWatching
	}

	w.watching = false
	close(w.doneCh)
	<-w.closedCh

	return nil
}

func (w *dbNamespaceWatch) Close() error {
	w.Lock()
	watching := w.watching
	w.Unlock()
	if watching {
		return w.Stop()
	}
	return nil
}
