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
	"strings"
	"text/tabwriter"

	"github.com/swdunlop/zugzug-go/zug"
	"github.com/swdunlop/zugzug-go/zug/console"
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
		tasks:   make(map[string]any),
		parsers: make(map[string]Parser),
		usage:   make(map[string]string),
	}
	cfg.addTask(`help`, zug.Alias(`help`, zug.New(cfg.provideHelp)), cfg)
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
	Name  string                      // if empty, the name of the function will be used
	Fn    func(context.Context) error // the task function
	Use   string                      // if non-empty, explains what the task does
	Parse Parser                      // if non-nil this will be used to parse additional arguments and flags
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
		cfg.addTask(name, task, it.Parse)
		if it.Use != `` {
			cfg.addUsage(name, it.Use)
		}
	}
}

type Parser interface {
	// Parse will parse arguments for flags or return nil, nil if help is requested.
	Parse(ctx context.Context, name string, arguments []string) (context.Context, error)

	// Usage will provide information about how the parser uses arguments.
	Usage(name string) string
}

type config struct {
	tasks   map[string]any
	parsers map[string]Parser
	err     error
	topics  []string
	usage   map[string]string
}

func (cfg *config) Parse(ctx context.Context, _ string, args []string) (context.Context, error) {
	if len(args) == 0 {
		return ctx, nil
	}
	if len(args) > 1 {
		return nil, fmt.Errorf(`cannot provide help for more than one argument`)
	}
	return context.WithValue(ctx, ctxHelpTopic{}, args[0]), nil
}

func (cfg *config) Usage(name string) string { return name + ` help [topic]` }

type ctxHelpTopic struct{}

func (cfg *config) Run(ctx context.Context, args ...string) error {
	if len(args) == 0 {
		args = []string{`help`}
	} else {
		switch args[0] {
		case `--help`, `-h`:
			args[0] = `help`
		default:
			if len(args) > 1 {
				switch args[1] {
				case `--help`, `-h`:
					args = []string{`help`, args[0]}
				}
			}
		}
	}

	var runs []runConfig
	for len(args) > 0 {
		var run runConfig
		var err error
		run, args, err = cfg.run(ctx, args...)
		if err != nil {
			return err
		}
		runs = append(runs, run)
	}

	for _, run := range runs {
		err := zug.Run(run.ctx, run.task)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cfg *config) run(ctx context.Context, args ...string) (runConfig, []string, error) {
	arg0, args := args[0], args[1:]
	taskName := strings.ToLower(arg0)
	task, ok := cfg.tasks[taskName]
	if !ok {
		return runConfig{}, nil, fmt.Errorf(`unknown task %q`, arg0)
	}

	fs := cfg.parsers[taskName]
	if fs == nil {
		return runConfig{ctx, task}, args, nil
	}
	ctx, err := fs.Parse(ctx, arg0, args)
	if err != nil {
		return runConfig{}, args, err
	}
	if ctx != nil {
		return runConfig{ctx, task}, nil, nil
	}
	return runConfig{}, []string{`help`, taskName}, nil
}

type runConfig struct {
	ctx  context.Context
	task any
}

func (cfg *config) addTask(name string, task zug.Task, parser Parser) {
	cfg.topics = append(cfg.topics, name)
	cfg.tasks[name] = task
	cfg.parsers[name] = parser
}

func (cfg *config) addUsage(name, use string) {
	cfg.usage[name] = use
}

func (cfg *config) provideHelp(ctx context.Context) error {
	if topic, ok := ctx.Value(ctxHelpTopic{}).(string); ok {
		return cfg.explainTopic(ctx, topic)
	}

	argv0 := cfg.baseCommandName()
	tw := tabwriter.NewWriter(console.From(ctx).Stderr(), 0, 0, 2, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, `USAGE:`)
	for _, topic := range cfg.topics {
		if topic == `help` {
			continue
		}
		usage := cfg.usage[topic]
		if ix := strings.IndexByte(usage, '\n'); ix > 0 {
			usage = usage[:ix]
		}
		usage = strings.TrimSuffix(usage, "\r")
		fmt.Fprintf(tw, "  %s %s \t%s\n", argv0, topic, usage)
	}
	return nil
}

func (cfg *config) explainTopic(ctx context.Context, topic string) error {
	usage := cfg.usage[topic]
	if usage == `` {
		return fmt.Errorf(`no help available for "%q"`, topic)
	}
	argv0 := cfg.baseCommandName()
	err := console.PrintError(ctx, argv0, topic)
	if err != nil {
		return err
	}
	console.PrintError(ctx, usage)
	if err != nil {
		return err
	}
	return nil
}

func (cfg *config) baseCommandName() string {
	argv0 := os.Args[0] // TODO: let the user override this
	argv0 = strings.TrimSuffix(argv0, `.exe`)
	if ix := strings.LastIndexByte(argv0, filepath.Separator); ix >= 0 {
		argv0 = argv0[ix+1:]
	}
	return argv0
}

// Interface describes the interface produced by New for running a task using a list of arguments.
type Interface interface {
	// Run will select a task based on the provided arguments.  If that task specifies a parser, its parser will be
	// provided with any remaining arguments.  Otherwise, Run will use the next argument to select another task, and
	// so on.
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
