// Copyright (c) 2023, Scott W. Dunlop
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

package parser

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/pflag"
)

// Custom returns a Parser that does not really parse -- it just captures the arguments for later access using
// Args with the context.  The provided usage is provided as literal usage if `--help` is invoked.
func Custom() Interface {
	return custom{}
}

type custom struct{}

// Parse implements Parser.
func (custom) Parse(ctx context.Context, name string, arguments []string) (context.Context, error) {
	return context.WithValue(ctx, ctxArgs{}, arguments), nil
}

// Args returns the unparsed arguments from the parser.
func Args(ctx context.Context) []string {
	return ctx.Value(ctxArgs{}).([]string)
}

// New constructs a new parser using pflag flags.
func New(options ...Option) BoolFlagger {
	var cfg config
	for _, opt := range options {
		opt(&cfg)
	}
	return &cfg
}

// Apply applies a series of options as an option.
func Apply(options ...Option) Option {
	return func(cfg *config) {
		for _, option := range options {
			option(cfg)
		}
	}
}

// TODO: count.

// String applies FlagSet.StringVarP as an Option to add a string flag with shorthand.
func String(p *string, name, shorthand string, usage string) Option {
	return func(cfg *config) {
		cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) { fs.StringVarP(p, name, shorthand, *p, usage) })
	}
}

// Int applies FlagSet.IntVarP as an Option to add a int flag with shorthand.
func Int(p *int, name, shorthand string, usage string) Option {
	return func(cfg *config) {
		cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) { fs.IntVarP(p, name, shorthand, *p, usage) })
	}
}

// Bool applies FlagSet.BoolVarP as an Option to add a bool flag with shorthand.
func Bool(p *bool, name, shorthand string, usage string) Option {
	return func(cfg *config) {
		cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) { fs.BoolVarP(p, name, shorthand, *p, usage) })
	}
}

// Uint applies FlagSet.UintVarP as an Option to add a uint flag with shorthand.
func Uint(p *uint, name, shorthand string, usage string) Option {
	return func(cfg *config) {
		cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) { fs.UintVarP(p, name, shorthand, *p, usage) })
	}
}

// Float applies FlagSet.FloatVarP as an Option to add a float flag with shorthand.
func Float(p *float64, name, shorthand string, usage string) Option {
	return func(cfg *config) {
		cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) { fs.Float64VarP(p, name, shorthand, *p, usage) })
	}
}

// Time uses Var to add a time flag expecting the provided format.  The format is the same as that used in
// time.ParseInLocation with location set to time.Local.
func Time(p *time.Time, name, shorthand string, format string, usage string) Option {
	return func(cfg *config) {
		cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) { fs.VarP(timeFlag{p, format, time.Local}, name, shorthand, usage) })
	}
}

// UTCTime uses Var to add a time flag expecting the provided format.  The format is the same as that used in
// time.ParseInLocation with location set to time.UTC.
func UTCTime(p *time.Time, name, shorthand string, format string, usage string) Option {
	return func(cfg *config) {
		cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) { fs.VarP(timeFlag{p, format, time.UTC}, name, shorthand, usage) })
	}
}

// Duration uses Var to add a duration flag with shorthand.  The time syntax is the same as used by time.ParseDuration
func Duration(p *time.Duration, name, shorthand string, usage string) Option {
	return func(cfg *config) {
		cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) { fs.VarP(durationFlag{p}, name, shorthand, usage) })
	}
}

// StringSlice applies FlagSet.StringSliceVarP as an Option to add a slice of strings flag with shorthand.
func StringSlice(p *[]string, name, shorthand string, usage string) Option {
	return func(cfg *config) {
		cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) { fs.StringSliceVarP(p, name, shorthand, *p, usage) })
	}
}

// Var applies FlagSet.VarP as an Option to add a variable flag with shorthand.
func Var(p Value, name, shorthand string, usage string) Option {
	return func(cfg *config) {
		cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) { fs.VarP(p, name, shorthand, usage) })
	}
}

// Arg documents a positional argument for the help output.
func Arg(name, usage string, required bool) Option {
	return func(cfg *config) {
		cfg.positionals = append(cfg.positionals, positionalArg{name, usage, required})
	}
}

// Example documents a usage example for the help output.
func Example(example, description string) Option {
	return func(cfg *config) {
		cfg.examples = append(cfg.examples, exampleArg{example, description})
	}
}

// An Option configures the parser.
type Option func(*config)

type positionalArg struct {
	name     string
	usage    string
	required bool
}

type exampleArg struct {
	example     string
	description string
}

type config struct {
	flagOptions []func(*pflag.FlagSet)
	positionals []positionalArg
	examples    []exampleArg
}

// Parse implements Parser.
func (cfg *config) Parse(ctx context.Context, name string, arguments []string) (context.Context, error) {
	fs := cfg.flagset(name)
	err := fs.Parse(arguments)
	switch err {
	case nil:
		return context.WithValue(ctx, ctxArgs{}, fs.Args()), nil
	case pflag.ErrHelp:
		return nil, nil
	default:
		return nil, err
	}
}

// Help implements zugzug.Helper by explaining the flags and positional arguments configured by the parser.
func (cfg *config) Help(name string) string {
	fs := cfg.flagset(name)
	var buf strings.Builder
	buf.WriteString(`COMMAND: `)
	buf.WriteString(name)
	if fs.HasFlags() {
		buf.WriteString(` [flags]`)
	}
	if len(cfg.positionals) > 0 {
		for _, p := range cfg.positionals {
			if p.required {
				fmt.Fprintf(&buf, " <%s>", p.name)
			} else {
				fmt.Fprintf(&buf, " [%s]", p.name)
			}
		}
	} else {
		buf.WriteString(` [argument...]`)
	}
	buf.WriteString("\n")
	if len(cfg.positionals) > 0 {
		buf.WriteString("\nARGUMENTS:\n")
		tw := tabwriter.NewWriter(&buf, 2, 0, 2, ' ', 0)
		for _, p := range cfg.positionals {
			req := ""
			if p.required {
				req = " (required)"
			}
			_, _ = fmt.Fprintf(tw, "  %s\t%s%s\n", p.name, p.usage, req)
		}
		_ = tw.Flush()
	}
	if fs.HasFlags() {
		buf.WriteString("\nFLAGS:\n")
		buf.WriteString(fs.FlagUsagesWrapped(118))
	}
	if len(cfg.examples) > 0 {
		buf.WriteString("\nEXAMPLES:\n")
		tw := tabwriter.NewWriter(&buf, 2, 0, 2, ' ', 0)
		for _, e := range cfg.examples {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\n", e.example, e.description)
		}
		_ = tw.Flush()
	}
	return buf.String()
}

// BoolFlag implements zugzug.BoolFlagger, allowing zugzug to add global boolean flags like -q / --quiet
func (cfg *config) BoolFlag(p *bool, name, shorthand string, usage string) {
	cfg.flagOptions = append(cfg.flagOptions, func(fs *pflag.FlagSet) {
		fs.BoolVarP(p, name, shorthand, *p, usage)
	})
}

type ctxArgs struct{}

// flagset composes a new flagset with the provided name and applies options.
func (cfg *config) flagset(name string) *pflag.FlagSet {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.Usage = func() {} // do nothing, we will return nil, nil instead.
	for _, opt := range cfg.flagOptions {
		opt(fs)
	}
	return fs
}

// Value is an alias for pflag.Value from github.com/spf13/pflag
type Value = pflag.Value

type timeFlag struct {
	p      *time.Time
	layout string
	loc    *time.Location
}

// Set implements pflag.Value.
func (f timeFlag) Set(str string) error {
	t, err := time.ParseInLocation(f.layout, str, f.loc)
	if err == nil {
		*f.p = t
	}
	return err
}

// String implements pflag.Value.
func (f timeFlag) String() string {
	if f.p.IsZero() {
		return ``
	}
	return f.p.Format(f.layout)
}

// Type implements pflag.Value.
func (f timeFlag) Type() string {
	return `string`
}

type durationFlag struct {
	p *time.Duration
}

// Set implements pflag.Value.
func (f durationFlag) Set(str string) error {
	d, err := time.ParseDuration(str)
	if err == nil {
		*f.p = d
	}
	return err
}

// String implements pflag.Value.
func (f durationFlag) String() string {
	if *f.p == 0 {
		return ``
	}
	return f.p.String()
}

// Type implements pflag.Value.
func (f durationFlag) Type() string {
	return `string`
}

// Interface describes the parser interface provided by this package.
type Interface interface {
	// Parse will parse arguments for flags or return nil, nil if help is requested.
	Parse(ctx context.Context, name string, arguments []string) (context.Context, error)
}

// BoolFlagger is an optional interface that is implemented by parser.New that lets zugzug add flags before it parses.
type BoolFlagger interface {
	Interface
	BoolFlag(p *bool, name, shorthand, usage string)
}
