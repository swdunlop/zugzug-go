// Copyright (c) 2023, Scott W. Dunlop
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

package zugzug

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/swdunlop/zugzug-go/zug/console"
)

// Settings provide a way to configure data from an environment.
type Settings []struct {
	Var  any
	Name string
	Use  string
}

// Apply will resolve settings by name using the provided lookup function, stopping at the first error.
func (seq Settings) Apply(lookup func(string) (string, bool)) error {
	for _, it := range seq {
		if it.Name == `` {
			continue
		}
		if value, ok := lookup(it.Name); ok {
			if err := set(it.Var, value); err != nil {
				return err
			}
		}
	}
	return nil
}

// get will get the value of a variable as a string.
func get(target any) string {
	// target is likely a pointer to a value, so we need to dereference it
	targetValue := reflect.ValueOf(target)
	switch targetValue.Kind() {
	case reflect.Ptr, reflect.Interface:
		if targetValue.IsNil() {
			return ``
		}
		target = targetValue.Elem().Interface()
	}
	return fmt.Sprint(target)
}

// set will set the value of a variable.
func set(target any, value string) error {
	switch target := target.(type) {
	case *string:
		*target = value
	default:
		_, err := fmt.Sscan(value, target)
		if err != nil {
			return fmt.Errorf(`%w in %q`, err, target)
		}
	}
	return nil
}

// envLookup will compose a lookup function from the provided context.
func envLookup(ctx context.Context) func(string) (string, bool) {
	env := console.From(ctx).Env()
	table := make(map[string]string, len(env))
	for _, it := range env {
		if i := strings.IndexByte(it, '='); i > 0 {
			table[it[:i]] = it[i+1:]
		}
	}

	return func(name string) (string, bool) {
		str, ok := table[name]
		return str, ok
	}
}
