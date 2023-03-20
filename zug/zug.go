// Copyright (c) 2023, Scott W. Dunlop
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

package zug

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/swdunlop/zugzug-go/zug/worker"
)

// WithLocalState will provide a new context with local state table for tracking whether a task has been run.  This
// is useful for enabling running tasks multiple times.
func WithLocalState(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxState{}, make(map[any]bool))
}

type ctxState struct{}

// Run will run each of the tasks in the order they are specified, sequentially, waiting until they complete and
// and stopping at the first failure.
func Run(ctx context.Context, tasks ...any) error {
	actualTasks, err := toTasks(tasks)
	if err != nil {
		return err
	}
	for _, task := range actualTasks {
		err := runTask(ctx, task)
		if err.Err != nil {
			return err
		}
	}
	return nil
}

// Start is similar to Run, but will run each task in parallel, waiting until they complete and returning Errors if
// any failed.
func Start(ctx context.Context, tasks ...any) error {
	actualTasks, err := toTasks(tasks)
	if err != nil {
		return err
	}
	taskErrors := make(Errors, len(actualTasks))
	var wg sync.WaitGroup
	wg.Add(len(actualTasks))
	for i, t := range actualTasks {
		go func(i int, t Task) {
			defer wg.Done()
			taskErrors[i] = runTask(ctx, t)
		}(i, t)
	}
	wg.Wait()
	n := 0
	for _, err := range taskErrors {
		if err.Err != nil {
			taskErrors[n] = err
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return taskErrors[:n]
}

// Alias will rename a task, which is useful if you have multiple tasks created by New but want to give them different
// names.
func Alias(name string, task Task) Task { return aliasTask{name: name, task: task} }

type aliasTask struct {
	name string
	task Task
}

func (t aliasTask) RunTask(ctx context.Context) error { return t.task.RunTask(ctx) }
func (t aliasTask) TaskName() string                  { return t.name }

// New will create a new task wrapped around the provided function and protected by a sync.Once from concurrency and
// repeated execution.
func New(task func(context.Context) error) Task { return &wrapTask{fn: task} }

type wrapTask struct {
	fn   func(context.Context) error
	once sync.Once
	err  error
}

func (t *wrapTask) RunTask(ctx context.Context) error {
	t.once.Do(func() { t.err = t.fn(ctx) })
	return t.err
}

func (t *wrapTask) TaskName() string {
	return fnTaskName(t.fn)
}

func toTasks(tasks []any) ([]Task, error) {
	actualTasks := make([]Task, len(tasks))
	for i, t := range tasks {
		task, err := toTask(t)
		if err != nil {
			return nil, err
		}
		actualTasks[i] = task
	}
	return actualTasks, nil
}

func toTask(t any) (Task, error) {
	if t, ok := t.(Task); ok {
		return t, nil
	}

	rv := reflect.ValueOf(t)
	if rv.Kind() != reflect.Func {
		return nil, fmt.Errorf(`cannot convert %v to a task`, rv.Type())
	}
	pc := rv.Pointer()

	switch t := t.(type) {
	case func(context.Context) error:
		return fnTask{pc, t}, nil
	case func() error:
		return fnTask{pc, func(context.Context) error { return t() }}, nil
	case func():
		return fnTask{pc, func(context.Context) error { t(); return nil }}, nil
	default:
		return nil, fmt.Errorf(`cannot convert function %v to a task`, fnTaskName(t))
	}
}

type fnTask struct {
	pc uintptr
	fn func(context.Context) error
}

func (t fnTask) RunTask(ctx context.Context) error {
	return worker.Run(ctx, 0, t.fn)
}

func (t fnTask) TaskName() string { return pcTaskName(t.pc) }

func fnTaskName(fn any) string {
	return pcTaskName(reflect.ValueOf(fn).Pointer())
}

func pcTaskName(pc uintptr) string {
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return ``
	}
	name := fn.Name()
	name = strings.TrimSuffix(name, "-fm")
	return name
}

// Errors is a slice of Errors that can be used as an error.
type Errors []Error

// condense removes all nil errors from the slice and returns nil if there are no errors.
func (errs Errors) condense() error {
	n := 0
	for _, err := range errs {
		if err.Err != nil {
			errs[n] = err
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return errs[:n]
}

// Error implements the error interface by returning an empty string if there are no errors, the first
// error if there was only one, or a string indicating the number of errors if there were more.
func (errs Errors) Error() string {
	switch len(errs) {
	case 0:
		return ``
	case 1:
		return errs[0].Error()
	default:
		return fmt.Sprintf(`%v errors`, len(errs))
	}
}

func runTask(ctx context.Context, task Task) (e Error) {
	var err error
	if debug {
		e.Task = taskName(task)
	} else {
		defer func() {
			switch r := recover().(type) {
			case nil:
				if err != nil {
					e = Error{taskName(task), err}
				}
			case error:
				e = Error{taskName(task), r}
			default:
				e = Error{taskName(task), fmt.Errorf(`%v`, r)}
			}
		}()
	}
	err = task.RunTask(ctx)
	return
}

var debug = true

// An Error is an error that knows which task it came from.
type Error struct {
	Task string
	Err  error
}

// Error implements the error interface by returning a string indicating the "task: error".
func (err Error) Error() string {
	return fmt.Sprintf(`%v: %v`, err.Task, err.Err)
}

func taskName(t Task) string {
	if nt, ok := t.(NamedTask); ok {
		return nt.TaskName()
	}
	return ``
}

// A NamedTask is a Task that has a name -- when Zug returns an Error, it will use this name to identify where the
// error occurred.
type NamedTask interface {
	Task
	TaskName() string
}

// A Task is something that can be run by Zug.  Tasks are expected to either be idempotent or to be able to prevent
// repeated (and possibly concurrent) execution.  It is usually easier to give Zug a function that optionally takes
// a context.Context and optionally returns an error, since Zug will automatically wrap it with a sync.Once based
// on the function address and name.
type Task interface {
	RunTask(context.Context) error
}
