package parser

import (
	"context"
	"time"

	"github.com/spf13/pflag"
	"github.com/swdunlop/zugzug-go"
)

// Custom returns a zugzug.Parser that does not really parse -- it just captures the arguments for later access using
// Args with the context.  The provided usage is provided as literal usage if `--help` is invoked.
func Custom() zugzug.Parser {
	return custom{}
}

type custom struct{}

// Parse implements zugzug.Parser.
func (custom) Parse(ctx context.Context, name string, arguments []string) (context.Context, error) {
	return context.WithValue(ctx, ctxArgs{}, arguments), nil
}

// Args returns the unparsed arguments from the parser.
func Args(ctx context.Context) []string {
	return ctx.Value(ctxArgs{}).([]string)
}

// New constructs a new parser using pflag flags.
func New(options ...Option) zugzug.Parser {
	var cfg config
	cfg.options = append(cfg.options, options...)
	return &cfg
}

// Apply applies a series of options as an option.
func Apply(options ...Option) Option {
	return func(fs *FlagSet) {
		for _, option := range options {
			option(fs)
		}
	}
}

// TODO: count.

// String applies FlagSet.StringVarP as an Option to add a string flag with shorthand.
func String(p *string, name, shorthand string, usage string) Option {
	return func(fs *FlagSet) { fs.StringVarP(p, name, shorthand, *p, usage) }
}

// Int applies FlagSet.IntVarP as an Option to add a int flag with shorthand.
func Int(p *int, name, shorthand string, usage string) Option {
	return func(fs *FlagSet) { fs.IntVarP(p, name, shorthand, *p, usage) }
}

// Bool applies FlagSet.BoolVarP as an Option to add a bool flag with shorthand.
func Bool(p *bool, name, shorthand string, usage string) Option {
	return func(fs *FlagSet) { fs.BoolVarP(p, name, shorthand, *p, usage) }
}

// Uint applies FlagSet.UintVarP as an Option to add a uint flag with shorthand.
func Uint(p *uint, name, shorthand string, usage string) Option {
	return func(fs *FlagSet) { fs.UintVarP(p, name, shorthand, *p, usage) }
}

// Time uses Var to add a time flag expecting the provided format.  The format is the same as that used in
// time.ParseInLocation with location set to time.Local.
func Time(p *time.Time, name, shorthand string, format string, usage string) Option {
	return func(fs *FlagSet) { fs.VarP(timeFlag{p, format, time.Local}, name, shorthand, usage) }
}

// UTCTime uses Var to add a time flag expecting the provided format.  The format is the same as that used in
// time.ParseInLocation with location set to time.UTC.
func UTCTime(p *time.Time, name, shorthand string, format string, usage string) Option {
	return func(fs *FlagSet) { fs.VarP(timeFlag{p, format, time.UTC}, name, shorthand, usage) }
}

// Duration uses Var to add a duration flag with shorthand.  The time syntax is the same as used by time.ParseDuration
func Duration(p *time.Duration, name, shorthand string, usage string) Option {
	return func(fs *FlagSet) { fs.VarP(durationFlag{p}, name, shorthand, usage) }
}

// StringSlice applies FlagSet.StringSliceVarP as an Option to add a slice of strings flag with shorthand.
func StringSlice(p *[]string, name, shorthand string, usage string) Option {
	return func(fs *FlagSet) { fs.StringSliceVarP(p, name, shorthand, *p, usage) }
}

// Var applies FlagSet.VarP as an Option to add a variable flag with shorthand.
func Var(p Value, name, shorthand string, usage string) Option {
	return func(fs *FlagSet) { fs.VarP(p, name, shorthand, usage) }
}

// An Option configures the provided flag set in advance of Usage or Parse.
type Option func(*FlagSet)

type config struct {
	options []Option
}

// Parse implements zugzug.Parser.
func (cfg *config) Parse(ctx context.Context, name string, arguments []string) (context.Context, error) {
	fs := cfg.flagset(name)
	err := fs.Parse(arguments)
	switch err {
	case nil:
		return context.WithValue(ctx, ctxArgs{}, fs.Args()), nil
	case pflag.ErrHelp:
		return nil, zugzug.Exit(1)
	default:
		return nil, err
	}
}

type ctxArgs struct{}

// flagset composes a new flagset with the provided name and applies options.
func (cfg *config) flagset(name string) *FlagSet {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	for _, opt := range cfg.options {
		opt(fs)
	}
	return fs
}

// Value is an alias for pflag.Value from github.com/spf13/pflag
type Value = pflag.Value

// FlagSet is an alias for pflag.FlagSet from github.com/spf13/pflag
type FlagSet = pflag.FlagSet

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
