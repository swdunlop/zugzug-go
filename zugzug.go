// Copyright (c) 2023, Scott W. Dunlop
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

// Package zugzug makes it easy to bind a set of Zug tasks as subcommands of a command line program.
package zugzug

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/swdunlop/zugzug-go/zug"
	"github.com/swdunlop/zugzug-go/zug/console"
	"github.com/swdunlop/zugzug-go/zug/parser"
)

// Main will assemble a configuration of tasks that can be performed with the provided options, and then run them based
// on the command line arguments.
func Main(options ...Option) {
	err := runMain(options...)
	if err == nil {
		return
	}

	var exit Exit
	if errors.As(err, &exit) {
		os.Exit(int(exit))
	}
	println(`!!`, err.Error())
	os.Exit(1)
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
		tasks:       make([]boundTask, 0, 32),
		defaultTask: `help`,
	}
	cfg.bindTask(zug.Alias(`help`, zug.New(cfg.provideHelp)), cfg, nil, ``)
	for _, option := range options {
		option.apply(cfg)
		if cfg.err != nil {
			return nil, cfg.err
		}
	}
	return cfg, nil
}

// Default specifies the default task if no arguments are provided.
func Default(taskName string) Option {
	return fnOption(func(cfg *config) { cfg.defaultTask = taskName })
}

// Tasks specify a set of tasks that can be run by a Zugzug configuration and can be provided as an option to New and
// Main.
type Tasks []struct {
	Name     string                      // if empty, the name of the function will be used
	Fn       func(context.Context) error // the task function
	Use      string                      // if non-empty, explains what the task does
	Parser   Parser                      // if non-nil this will be used to parse additional arguments and flags
	Settings Settings                    // will be configured using the console environment
}

func (seq Tasks) apply(cfg *config) {
	for _, it := range seq {
		var task zug.NamedTask
		name := it.Name
		if name != `` {
			task = zug.Alias(it.Name, zug.New(it.Fn))
		} else {
			task = zug.New(it.Fn).(zug.NamedTask)
			name = task.TaskName()
			name = strings.TrimSuffix(name, `-fm`) // common for methods converted to functions
			name = sanitize(name)
			task = zug.Alias(name, task)
		}
		if name == `` {
			panic(fmt.Errorf(`all zugzug tasks must have a name`))
		}
		cfg.bindTask(task, it.Parser, it.Settings, it.Use)
	}
}

// Helper describes an interface that may be implemented by a parser or task to explain its arguments and flags.  This
// is implemented by zug/parser.  This should return text like "foo bar [-f foo] file1 fileN...\nFLAGS:\n  -f
// .."
//
// Zugzug will prefer the parser's help to the task.
type Helper interface {
	// Help will return a string explaining the arguments and flags for the named command.
	Help(name string) string
}

type Parser = parser.Interface

type config struct {
	with        []contextHook
	parserHooks []parserHook
	tasks       []boundTask
	err         error
	topics      []string
	defaultTask string
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
		args = []string{cfg.defaultTask}
	} else {
		switch args[0] {
		case `--help`, `-h`:
			args[0] = `help`
		}
	}

	type job struct {
		ctx  context.Context
		task zug.Task
	}
	var jobs []job

	lookupEnv := envLookup(ctx)

	for len(args) > 0 {
		// TODO: support for using "--" to separate arguments from the command and its flags.
		task := cfg.match(args...)
		if task == nil {
			return fmt.Errorf(`unknown command %q; try "help" for a list of commands`, strings.Join(args, ` `))
		}
		if task.settings != nil {
			err := task.settings.Apply(lookupEnv)
			if err != nil {
				return fmt.Errorf(`%w in %q`, err, strings.Join(task.name, ` `))
			}
		}
		args = args[len(task.name):]
		taskCtx := ctx
		if task.parser == nil && len(cfg.parserHooks) > 0 {
			task.parser = parser.New() // shim in a parser.
		}
		if task.parser != nil {
			var err error
			for _, hook := range cfg.parserHooks {
				hook(task.parser)
			}
			taskCtx, err = task.parser.Parse(ctx, cfg.baseCommandName()+` `+strings.Join(task.name, ` `), args)
			if err != nil {
				return err
			}
			if taskCtx == nil {
				return cfg.explainTopic(ctx, strings.Join(task.name, ` `))
			}
			args = nil // we assume the parser has consumed all arguments
		}
		for _, with := range task.with {
			taskCtx = with(taskCtx)
		}
		if len(args) > 0 {
			switch args[0] {
			case `--help`, `-h`:
				// stop planning work, give the user help.
				return cfg.explainTopic(ctx, strings.Join(task.name, ` `))
			}
		}
		jobs = append(jobs, job{ctx: taskCtx, task: task.task})
	}

	for _, job := range jobs {
		err := zug.Run(job.ctx, job.task)
		if err != nil {
			return err
		}
	}

	return nil
}

type runConfig struct {
	ctx  context.Context
	task *boundTask
}

func (cfg *config) provideHelp(ctx context.Context) error {
	if topic, ok := ctx.Value(ctxHelpTopic{}).(string); ok {
		return cfg.explainTopic(ctx, topic)
	}

	argv0 := cfg.baseCommandName()
	tw := tabwriter.NewWriter(console.From(ctx).Stderr(), 0, 0, 2, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, `COMMANDS:`)
	for _, topic := range cfg.topics {
		if topic == `help` {
			continue
		}
		task := cfg.matchStr(topic)
		if task == nil {
			continue
		}
		usage := task.use
		if ix := strings.IndexByte(usage, '\n'); ix > 0 {
			usage = usage[:ix]
		}
		usage = strings.TrimSuffix(usage, "\r")
		fmt.Fprintf(tw, "  %s %s \t%s\n", argv0, topic, usage)
	}

	hasSettings := false
	for _, task := range cfg.tasks {
		if len(task.settings) > 0 {
			hasSettings = true
			break
		}
	}
	if !hasSettings {
		return nil
	}

	fmt.Fprintln(tw, "\nSETTINGS:")
	explained := make(map[string]struct{}, len(cfg.tasks))
	for _, task := range cfg.tasks {
		for _, it := range task.settings {
			if _, ok := explained[it.Name]; ok {
				continue
			}
			explained[it.Name] = struct{}{}
			fmt.Fprintln(tw, settingExplanation(it.Name, it.Use, get(it.Var)))
		}
	}
	return nil
}

func (cfg *config) explainTopic(ctx context.Context, topic string) error {
	task := cfg.matchStr(topic)
	if task == nil {
		return fmt.Errorf(`no help available for "%q"`, topic)
	}
	argv0 := cfg.baseCommandName()
	if helper, ok := task.parser.(Helper); ok {
		_ = console.PrintError(ctx, helper.Help(argv0+` `+topic))
	} else if helper, ok := task.task.(Helper); ok {
		_ = console.PrintError(ctx, helper.Help(argv0+` `+topic))
	} else {
		_ = console.PrintError(ctx, `COMMAND:`, argv0, topic)
	}

	if len(task.settings) > 0 {
		tw := tabwriter.NewWriter(console.From(ctx).Stderr(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, `SETTINGS:`)
		for _, it := range task.settings {
			fmt.Fprintln(tw, settingExplanation(it.Name, it.Use, get(it.Var)))
		}
		_ = tw.Flush()
	}

	return nil
}

func settingExplanation(name, use, value string) string {
	if value == `` {
		return fmt.Sprintf("  %s \t%s", name, use)
	} else {
		return fmt.Sprintf("  %s \t%s (default: %q)", name, use, value)
	}
}

func (cfg *config) baseCommandName() string {
	argv0 := os.Args[0] // TODO: let the user override this
	argv0 = strings.TrimSuffix(argv0, `.exe`)
	if ix := strings.LastIndexByte(argv0, filepath.Separator); ix >= 0 {
		argv0 = argv0[ix+1:]
	}
	return argv0
}

func (cfg *config) bindTask(task zug.NamedTask, parser Parser, settings Settings, use string) {
	nameStr := strings.TrimSpace(task.TaskName())
	var nameSeq []string
	if nameStr != `` {
		nameSeq = rxSpace.Split(nameStr, -1)
		nameStr = strings.Join(nameSeq, ` `)
		cfg.topics = append(cfg.topics, nameStr)
	}

	cfg.tasks = append(cfg.tasks, boundTask{
		with:     append([]contextHook{}, cfg.with...),
		name:     nameSeq,
		task:     task,
		parser:   parser,
		settings: settings,
		use:      use,
	})
}

var rxSpace = regexp.MustCompile(`\s+`)

// matchStr returns the named task that matches the provided arguments, or nil if none match.
func (cfg *config) matchStr(name string) *boundTask {
	return cfg.match(strings.Split(name, ` `)...)
}

// match returns the named task that matches the provided arguments, or nil if none match.
func (cfg *config) match(args ...string) *boundTask {
	for i := range cfg.tasks {
		task := &cfg.tasks[i]
		if task.matches(args...) {
			return task
		}
	}
	return nil
}

type boundTask struct {
	with     []contextHook
	name     []string
	task     zug.NamedTask
	use      string
	parser   Parser
	settings Settings
}

// matches returns true if the args[:len(task.name)] matches task.name.
func (t *boundTask) matches(args ...string) bool {
	if len(args) < len(t.name) {
		return false
	}
	for i, name := range t.name {
		if name != args[i] {
			return false
		}
	}
	return true
}

// Interface describes the interface produced by New for running a task using a list of arguments.
type Interface interface {
	// Run will select a task based on the provided arguments.  If that task specifies a parser, its parser will be
	// provided with any remaining arguments.  Otherwise, Run will use the next argument to select another task, and
	// so on.
	Run(ctx context.Context, args ...string) error
}

// Console specifies console options.
func Console(options ...console.Option) Option {
	return fnOption(func(cfg *config) {
		cfg.with = append(cfg.with, func(ctx context.Context) context.Context {
			return console.With(ctx, options...)
		})
	})
}

// Verbosity specifies control of console verbosity for tasks whose parsers support parser.BoolFlagger.  This adds flags for
// "-v / --verbose", "-q / --quiet", and "-s / --silent".
func Verbosity() Option {
	return fnOption(func(cfg *config) {
		var (
			quiet   = false
			verbose = false
			silent  = false
		)
		cfg.parserHooks = append(cfg.parserHooks, func(fs parser.Interface) {
			bf, ok := fs.(parser.BoolFlagger)
			if !ok {
				return
			}
			bf.BoolFlag(&verbose, `verbose`, `v`, `logs commands their stderr to stderr`)
			bf.BoolFlag(&quiet, `quiet`, `q`, `only logs commands and their stderr to stderr if they fail`)
			bf.BoolFlag(&silent, `silent`, `s`, `suppresses command logging entirely`)
		})
		cfg.with = append(cfg.with, func(ctx context.Context) context.Context {
			switch {
			case verbose:
				ctx = console.With(ctx, console.Verbose())
			case quiet:
				ctx = console.With(ctx, console.Quiet())
			case silent:
				ctx = console.With(ctx, console.Silent())
			}
			return ctx
		})
	})
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

// Exit, when returned as an error by a task to Main, causes it to exit with a specific error code but not print
// any error.
type Exit int

func (ex Exit) Error() string { return fmt.Sprint(`exit code `, int(ex)) }

type parserHook func(parser.Interface)

type contextHook func(context.Context) context.Context
