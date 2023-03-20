// Copyright (c) 2023, Scott W. Dunlop
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

package worker

import (
	"context"
	"reflect"
	"sync"
)

// Run will run the provided task in its context, returning the error if any.
func Run(ctx context.Context, id uint, fn func(context.Context) error) error {
	return from(ctx).run(ctx, id, fn)
}

// With derives a new context with the provided options.
func With(ctx context.Context, options ...Option) context.Context {
	w0 := from(ctx)
	w0.control.Lock()
	defer w0.control.Unlock()
	w := w0
	for _, option := range options {
		w = option(w)
	}
	return context.WithValue(ctx, ctxWorker{}, &w)
}

// EmptyState provides an option with no state tracking.  Any task started in the previous context can be run again
// in the new context.
func EmptyState() Option {
	return func(prev *worker) *worker {
		var next worker
		next.state = make(map[resultID]*result)
		return &next
	}
}

// LocalState provides an option that isolates state tracking.  Any task started in the previous context cannot be
// run again in the new context -- instead the result of the previous task will be returned.
func LocalState() Option {
	return func(prev *worker) *worker {
		var next worker
		next.state = make(map[resultID]*result, len(prev.state))
		for id, state := range prev.state {
			next.state[id] = state
		}
		return &next
	}
}

type Option func(*worker) *worker

func from(ctx context.Context) *worker {
	w, ok := ctx.Value(ctxWorker{}).(*worker)
	if ok {
		return w
	}
	return globalWorker
}

var globalWorker = newWorker()

func newWorker() *worker {
	return &worker{
		state: make(map[resultID]*result),
	}
}

type worker struct {
	control sync.Mutex
	state   map[resultID]*result
}

func (w *worker) run(ctx context.Context, id uint, fn func(context.Context) error) error {
	pc := reflect.ValueOf(fn).Pointer()
	rid := resultID{id, pc}
	w.control.Lock()
	state, ok := w.state[rid]
	if !ok {
		state = &result{}
		w.state[rid] = state
	}
	w.control.Unlock()
	state.once.Do(func() { state.err = fn(ctx) })
	return state.err
}

type resultID struct {
	id uint
	pc uintptr
}

type result struct {
	once sync.Once
	err  error
}

type ctxWorker struct{}
