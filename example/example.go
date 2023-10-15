// Copyright (c) 2023, Scott W. Dunlop
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"context"
	"fmt"
	"time"

	zugzug "github.com/swdunlop/zugzug-go"
	"github.com/swdunlop/zugzug-go/zug"
	"github.com/swdunlop/zugzug-go/zug/console"
	"github.com/swdunlop/zugzug-go/zug/parser"
)

func main() {
	// Main will take over the program, running tasks based on the command line arguments.
	zugzug.Main(zugzug.Tasks{
		{Fn: Check, Use: `runs all of the checks`},
		{Fn: CheckSpelling, Use: `checks spelling`},
		{Fn: CheckLinks, Use: `checks links`},
		{Fn: GenerateHTML, Use: `generates HTML`},
		{Fn: ListGoSources, Use: `finds Go source files`},
		{Fn: Sleep, Use: `sleeps for a certain amount of time`, Parser: parser.New(
			parser.Duration(&sleepTime, `duration`, `t`, `how long to sleep`),
		)},
		{Name: `ql`, Fn: RunQL, Use: `runs the QL command line utility`, Parser: parser.Custom()},
	})
}

// task groups all of the methods we want to expose as Zug tasks.
type task struct{}

func Check(ctx context.Context) error {
	return zug.Run(ctx,
		CheckSpelling,
		CheckLinks,
	)
}

func GenerateHTML(ctx context.Context) error {
	return console.Print(ctx, `generating HTML`)
}

func CheckSpelling(ctx context.Context) error {
	return console.Print(ctx, `checking spelling`)
}

func CheckLinks(ctx context.Context) error {
	return console.Print(ctx, `checking links`)
}

func ListGoSources(ctx context.Context) error {
	return console.Run(ctx, `find`, `.`, `-name`, `*.go`)
}

func RunQL(ctx context.Context) error {
	args := append([]string{`run`, `github.com/cznic/ql/ql@latest`}, parser.Args(ctx)...)
	return console.Run(ctx, `go`, args...)
}

func Sleep(ctx context.Context) error {
	c2, cancel := context.WithTimeout(ctx, sleepTime)
	defer cancel()
	<-c2.Done()
	if c2.Err() != nil {
		return fmt.Errorf(`sleep interrupted`)
	}
	return nil
}

var sleepTime = time.Second
