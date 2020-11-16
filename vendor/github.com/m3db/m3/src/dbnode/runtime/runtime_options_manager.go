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

package runtime

import (
	"fmt"

	xclose "github.com/m3db/m3/src/x/close"
	xwatch "github.com/m3db/m3/src/x/watch"
)

type optionsManager struct {
	watchable xwatch.Watchable
}

// NewOptionsManager creates a new runtime options manager
func NewOptionsManager() OptionsManager {
	watchable := xwatch.NewWatchable()
	watchable.Update(NewOptions())
	return &optionsManager{watchable: watchable}
}

func (w *optionsManager) Update(value Options) error {
	if err := value.Validate(); err != nil {
		return err
	}
	w.watchable.Update(value)
	return nil
}

func (w *optionsManager) Get() Options {
	return w.watchable.Get().(Options)
}

func (w *optionsManager) RegisterListener(
	listener OptionsListener,
) xclose.SimpleCloser {
	_, watch, _ := w.watchable.Watch()

	// We always initialize the watchable so always read
	// the first notification value
	<-watch.C()

	// Deliver the current runtime options
	listener.SetRuntimeOptions(watch.Get().(Options))

	// Spawn a new goroutine that will terminate when the
	// watchable terminates on the close of the runtime options manager
	go func() {
		for range watch.C() {
			listener.SetRuntimeOptions(watch.Get().(Options))
		}
	}()

	return watch
}

func (w *optionsManager) Close() {
	w.watchable.Close()
}

// NewNoOpOptionsManager returns a no-op options manager that cannot
// be updated and does not spawn backround goroutines (useful for globals
// in test files).
func NewNoOpOptionsManager(opts Options) OptionsManager {
	return noOpOptionsManager{opts: opts}
}

type noOpOptionsManager struct {
	opts Options
}

func (n noOpOptionsManager) Update(value Options) error {
	return fmt.Errorf("no-op options manager cannot update options")
}

func (n noOpOptionsManager) Get() Options {
	return n.opts
}

func (n noOpOptionsManager) RegisterListener(
	listener OptionsListener,
) xclose.SimpleCloser {
	// noOpOptionsManager never changes its options, not worth
	// registering listener
	return noOpCloser{}
}

func (n noOpOptionsManager) Close() {
}

type noOpCloser struct{}

func (n noOpCloser) Close() {

}
