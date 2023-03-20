// Copyright (c) 2023, Scott W. Dunlop
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"context"

	zugzug "github.com/swdunlop/zugzug-go"
	"github.com/swdunlop/zugzug-go/zug"
	"github.com/swdunlop/zugzug-go/zug/console"
)

func main() {
	// Main will take over the program, running tasks based on the command line arguments.
	zugzug.Main(zugzug.Tasks{
		{Fn: Check, Use: `runs all of the checks`},
		{Fn: CheckSpelling, Use: `checks spelling`},
		{Fn: CheckLinks, Use: `checks links`},
		{Fn: GenerateHTML, Use: `generates HTML`},
		{Fn: ListGoSources, Use: `finds Go source files`},
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
