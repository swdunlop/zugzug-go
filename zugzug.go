// Copyright (c) 2023, Scott W. Dunlop
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

package zugzug

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/swdunlop/zugzug-go/zug"
)

// Main will assemble a configuration of tasks that can be performed with the provided options, and then run them based
// on the command line arguments.
func Main(options ...Option) {
	err := runMain(options...)
	if err != nil {
		println(`!!`, err.Error())
		os.Exit(1)
	}
}

func runMain(options ...Option) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	cfg, err := New(options...)
	if err != nil {
		return err
	}
	return cfg.Run(ctx, os.Args[1:]...)
}

// New will assemble a configuration of tasks that can be run based on context.
func New(options ...Option) (Interface, error) {
	cfg := &config{
		tasks: make(map[string]any),
		usage: make(map[string]string),
	}
	cfg.addTask(`help`, zug.Alias(`help`, zug.New(cfg.showUsage)))
	for _, option := range options {
		option.apply(cfg)
		if cfg.err != nil {
			return nil, cfg.err
		}
	}
	return cfg, nil
}

// Tasks specify a set of tasks that can be run by a Zugzug configuration and can be provided as an option to New and
// Main.
type Tasks []struct {
	Name string                      // if empty, the name of the function will be used
	Fn   func(context.Context) error // the task function
	Use  string                      // if non-empty, explains what the task does
}

func (seq Tasks) apply(cfg *config) {
	for _, it := range seq {
		task := zug.New(it.Fn)
		name := it.Name
		if name != `` {
			task = zug.Alias(it.Name, task)
		} else {
			name = task.(zug.NamedTask).TaskName()
			name = strings.TrimSuffix(name, `-fm`) // common for methods converted to functions
		}
		name = sanitize(name)
		if name == `` {
			panic(fmt.Errorf(`all zugzug tasks must have a name`))
		}
		cfg.addTask(name, task)
		if it.Use != `` {
			cfg.addUsage(name, it.Use)
		}
	}
}

type config struct {
	tasks map[string]any
	usage map[string]string
	err   error
}

func (cfg *config) Run(ctx context.Context, args ...string) error {
	if len(args) == 0 {
		args = []string{`help`}
	}
	tasks := make([]any, 0, len(args))
	for _, arg := range args {
		task, ok := cfg.tasks[strings.ToLower(arg)]
		if !ok {
			return fmt.Errorf(`unknown task %q`, arg)
		}
		tasks = append(tasks, task)
	}
	return zug.Run(ctx, tasks...)
}

func (cfg *config) addTask(name string, task zug.Task) {
	cfg.tasks[name] = task
}

func (cfg *config) addUsage(name, use string) {
	cfg.usage[name] = use
}

func (cfg *config) showUsage(ctx context.Context) error {
	argv0 := os.Args[0] // TODO: let the user override this
	argv0 = strings.TrimSuffix(argv0, `.exe`)
	if ix := strings.LastIndexByte(argv0, filepath.Separator); ix >= 0 {
		argv0 = argv0[ix+1:]
	}

	topics := make([]string, 0, len(cfg.usage))
	for topic := range cfg.usage {
		topics = append(topics, topic)
	}
	sort.Strings(topics)

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, `USAGE:`)
	for _, topic := range topics {
		usage := cfg.usage[topic]
		fmt.Fprintf(tw, "  %s %s \t%s\n", argv0, topic, usage)
	}
	return nil
}

type Interface interface {
	Run(ctx context.Context, args ...string) error
}

type fnOption func(*config)

func (fn fnOption) apply(cfg *config) { fn(cfg) }

type Option interface {
	apply(*config)
}

func sanitize(name string) string {
	// TODO: normalize whitespace.
	ix := strings.LastIndexByte(name, '.')
	if ix >= 0 {
		name = name[ix+1:]
	}
	name = rxFirstCap.ReplaceAllString(name, "${1}-${2}")
	name = rxAllCap.ReplaceAllString(name, "${1}-${2}")
	name = strings.Join(rxWord.FindAllString(name, -1), `-`)
	return strings.ToLower(name)
}

var (
	rxFirstCap = regexp.MustCompile(`(.)([\p{Lu}][\p{Ll}]+)`)
	rxAllCap   = regexp.MustCompile(`([\p{Ll}0-9])([\p{Lu}])`)
	rxWord     = regexp.MustCompile(`[\pL0-9]+`)
)
