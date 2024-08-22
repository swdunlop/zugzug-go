// Copyright (c) 2023, Scott W. Dunlop
// All rights reserved.
//
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree.

package console

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/swdunlop/zugzug-go/zug"
	"github.com/swdunlop/zugzug-go/zug/console/indent"
)

// Print will print the provided arguments to the console's stdout using fmt.Println.
func Print(ctx context.Context, args ...interface{}) error {
	_, err := fmt.Fprintln(from(ctx).stdout, args...)
	return err
}

// Printf will print the provided arguments to the console's stdout using fmt.Printf.
func Printf(ctx context.Context, format string, args ...interface{}) error {
	_, err := fmt.Fprintf(from(ctx).stdout, format, args...)
	return err
}

// PrintError will print the provided arguments to the console's stderr using fmt.Println.
func PrintError(ctx context.Context, args ...interface{}) error {
	_, err := fmt.Fprintln(from(ctx).stderr, args...)
	return err
}

// PrintErrorf will print the provided arguments to the console's stderr using fmt.Printf.
func Errorf(ctx context.Context, format string, args ...interface{}) error {
	_, err := fmt.Fprintf(from(ctx).stderr, format, args...)
	return err
}

// Do will return a Zug task that will run the specified command.
func Do(name string, args ...string) zug.NamedTask {
	return zug.Alias(name, zug.New(func(ctx context.Context) error {
		return Run(ctx, name, args...)
	})).(zug.NamedTask)
}

// Eval will run the provided command with the provided arguments, returning the output and error if any.
func Eval(ctx context.Context, name string, args ...string) (string, error) {
	var buf bytes.Buffer
	err := from(ctx).withCommand(ctx, name, args, func(cmd *exec.Cmd) error {
		cmd.Stdout = &buf
		return cmd.Run()
	})
	return buf.String(), err
}

// Run will run the provided command with the provided arguments, returning the error if any.
func Run(ctx context.Context, name string, args ...string) (err error) {
	return from(ctx).withCommand(ctx, name, args, func(cmd *exec.Cmd) error {
		return cmd.Run()
	})
}

func (cfg *config) withCommand(ctx context.Context, name string, args []string, do func(*exec.Cmd) error) (err error) {
	cmd := cfg.command(ctx, name, args...)
	var buf []byte
	if cfg.verbosityValue != silentVerbosity {
		buf = make([]byte, 0, 256)
		buf = append(buf, ">> "...)
		buf = AppendCommand(buf, cmd)
		buf = append(buf, '\n')
	}
	switch cfg.verbosityValue {
	case normalVerbosity, quietVerbosity:
		var stderr bytes.Buffer
		if cfg.verbosityValue == normalVerbosity {
			cfg.stderr.Write(buf)
		} else {
			stderr.Write(buf) // only show the command if it fails.
		}
		stderr.Grow(256)
		oldStderr := cmd.Stderr
		cmd.Stderr = indent.Writer(&stderr, `   `)
		defer func() {
			if err != nil {
				fmt.Fprintln(&stderr, `!!`, err)
				stderr.WriteTo(oldStderr)
			}
		}()
	case verboseVerbosity:
		cfg.stderr.Write(buf)
		defer func() {
			if err != nil {
				fmt.Fprintln(cfg.stderr, `!!`, err)
			}
		}()
		cmd.Stderr = indent.Writer(cmd.Stderr, `   `)
	case silentVerbosity:
		cmd.Stderr = io.Discard
	}

	err = do(cmd)
	return
}

//	func Console() console.Option {
//		return console.Hook(func(ctx context.Context, cmd *exec.Cmd) func(error) error {
//			if !Quiet || Verbose {
//				console.PrintError(ctx, "..", console.FormatCommand(cmd))
//			}
//			if Verbose {
//				return nil
//			}
//			var buf bytes.Buffer
//			buf.Grow(128)
//			if Quiet && !Verbose {
//				console.PrintError(ctx, "..", console.FormatCommand(cmd))
//			}
//			out := indent.Writer(&buf, "   ")
//				if cmd.Stderr == nil {
//					cmd.Stderr = out
//				} else {
//					cmd.Stderr = io.MultiWriter(out, cmd.Stderr)
//				}
//				if cmd.Stdout == nil {
//					cmd.Stdout = out
//				} else {
//					cmd.Stdout = io.MultiWriter(out, cmd.Stdout)
//				}
//				return func(err error) error {
//					if err != nil {
//						_, _ = io.Copy(os.Stderr, &buf)
//					}
//					return err
//				}
//			})
//		}
//
// Command returns a new command with the provided name and arguments with Dir, Stdout, Stderr, Stdin and Environment
// configured by the console.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	return from(ctx).command(ctx, name, args...)
}

func (cfg *config) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cfg.dir
	cmd.Stdout = cfg.stdout
	cmd.Stderr = cfg.stderr
	cmd.Stdin = cfg.stdin
	cmd.Env = cfg.env
	return cmd
}

// With will derive a new context with the provided options.
func With(ctx context.Context, options ...Option) context.Context {
	cfg := *from(ctx)
	for _, option := range options {
		option(&cfg)
	}
	return context.WithValue(ctx, ctxConsole{}, &cfg)
}

// From will return the console from the provided context or the default console.  The first time From or With is
// called, the default console is initialized from os.Stdout, os.Stderr, and os.Environ.
func From(ctx context.Context) Interface {
	return from(ctx)
}

// from returns either the console from the provided context or the default console.
func from(ctx context.Context) *config {
	if cfg, ok := ctx.Value(ctxConsole{}).(*config); ok {
		return cfg
	}
	initDefaultConfigOnce.Do(initDefaultConfig)
	return &defaultConfig
}

type ctxConsole struct{}

// Interface describes the interface managed by the console package.
type Interface interface {
	// Dir returns the current working directory.
	Dir() string

	// Env returns the current environment.
	Env() []string

	// Stdout returns the current stdout writer.
	Stdout() io.Writer

	// Stderr returns the current stderr writer.
	Stderr() io.Writer

	// verbosity returns the current verbosity for Run and Eval.  (This does not affect Command.)
	//
	// This defaults to printing the commands that are run, but not relaying stderr.
	verbosity() verbosity
}

// TeeStdout copies output sent to stdout to the specified writer before sending it to the original stdout.
func TeeStdout(w io.Writer) Option {
	return func(cfg *config) { cfg.stdout = io.MultiWriter(w, cfg.stdout) }
}

// TeeStderr copies output sent to stderr to the specified writer before sending it to the original stderr.
func TeeStderr(w io.Writer) Option {
	return func(cfg *config) { cfg.stderr = io.MultiWriter(w, cfg.stderr) }
}

// TeeStdin copies input read from stdin to the specified writer.
func TeeStdin(w io.Writer) Option {
	return func(cfg *config) { cfg.stdin = io.TeeReader(cfg.stdin, w) }
}

// Indent indents the stdout and stderr by the specified prefix using indent.Writer
func Indent(str string) Option {
	return func(cfg *config) {
		cfg.stdout = indent.Writer(cfg.stdout, str)
		cfg.stderr = indent.Writer(cfg.stderr, str)
	}
}

// Silent specifies that the console must never produce output for any reason.
func Silent() Option {
	return func(cfg *config) { cfg.verbosityValue = silentVerbosity }
}

// Quiet specifies that the console should not produce output to stderr unless there is an error.
func Quiet() Option {
	return func(cfg *config) { cfg.verbosityValue = quietVerbosity }
}

// Verbose specifies that the console should produce output to stderr that describes what it is doing.
func Verbose() Option {
	return func(cfg *config) { cfg.verbosityValue = verboseVerbosity }
}

const (
	normalVerbosity = verbosity(iota)
	verboseVerbosity
	quietVerbosity
	silentVerbosity
)

type verbosity int

// Silent is true if stderr should not get any output for any reason.
func (v verbosity) Silent() bool { return v == silentVerbosity }

// Quiet is true if stderr should not get any output if there is no error.
func (v verbosity) Quiet() bool { return v == quietVerbosity }

// Verbose is true if stderr should get all output.
func (v verbosity) Verbose() bool { return v == verboseVerbosity }

// Apply applies one or more options as an option.
func Apply(options ...Option) Option {
	return func(cfg *config) {
		for _, option := range options {
			option(cfg)
		}
	}
}

// Stdout specifies the writer to use for console output.
func Stdout(stdout io.Writer) Option {
	return func(cfg *config) { cfg.stdout = stdout }
}

// Stderr specifies the writer to use for console error output.
func Stderr(stderr io.Writer) Option {
	return func(cfg *config) { cfg.stderr = stderr }
}

// Stdin specifies the reader to use for console input.
func Stdin(stdin io.Reader) Option {
	return func(cfg *config) { cfg.stdin = stdin }
}

// FullEnv specifies the environment to use for the console, without appending to the previous environment.
func FullEnv(env []string) Option {
	return func(cfg *config) { cfg.env = env }
}

// Env adds the provided environment variables to the console environment, use FullEnv to specify the full environment
// without appending.
func Env(env ...string) Option {
	return func(cfg *config) { cfg.env = append(cfg.env, env...) }
}

// Dir specifies the working directory for the console.
func Dir(dir string) Option {
	return func(cfg *config) { cfg.dir = dir }
}

// An Option affects the configuration of a console.
type Option func(*config)

// initDefaultConfig is run by "from" when we need a config and there isn't one in the context.  We do this as late as
// possible in case the process updates stdout, stderr or environ.  We do not default stdin because some other part of
// the process might use it.
func initDefaultConfig() {
	defaultConfig = config{
		stdout: os.Stdout,
		stderr: os.Stderr,
		env:    os.Environ(),
	}
}

var (
	initDefaultConfigOnce sync.Once
	defaultConfig         config
)

type config struct {
	dir            string
	stdout, stderr io.Writer
	stdin          io.Reader
	env            []string
	verbosityValue verbosity
}

func (c *config) Dir() string          { return c.dir }
func (c *config) Env() []string        { return c.env }
func (c *config) Stdout() io.Writer    { return c.stdout }
func (c *config) Stderr() io.Writer    { return c.stderr }
func (c *config) Stdin() io.Reader     { return c.stdin }
func (c *config) verbosity() verbosity { return c.verbosityValue }

// FormatCommand wraps AppendCommand to return a string.
func FormatCommand(cmd *exec.Cmd) string {
	return string(AppendCommand(make([]byte, 0, 256), cmd))
}

// FormatCommandPath wraps AppendCommandPath to return a string.
func FormatCommandPath(path string) string {
	return string(AppendCommandPath(make([]byte, 0, 64), path))
}

// FormatArgs wraps AppendArgs to return a string.
func FormatArgs(args ...string) string {
	return string(AppendArgs(make([]byte, 0, 64), args...))
}

// FormatEnv wraps AppendEnv to return a string.
func FormatEnv(env ...string) string {
	return string(AppendEnv(make([]byte, 0, 64), env...))
}

// AppendCommand appends the specified command in POSIX shell format, using AppendCommandPath, AppendArgs and AppendEnv.
func AppendCommand(buf []byte, cmd *exec.Cmd) []byte {
	if env := variableEnv(novelEnv(cmd.Env...)...); len(env) > 0 {
		buf = appendEnv(buf, cmd.Env...)
		buf = append(buf, ' ')
	}

	buf = AppendCommandPath(buf, cmd.Path)

	if len(cmd.Args) > 0 {
		buf = append(buf, ' ')
		buf = AppendArgs(buf, cmd.Args[1:]...)
	}

	return buf
}

// AppendCommandPath appends the specified command path in POSIX shell format.  This will trim the command to just the filename if the
// executable could be found in $PATH.
func AppendCommandPath(buf []byte, path string) []byte {
	name := filepath.Base(path)
	if found, _ := exec.LookPath(name); found == path {
		path = name
	}
	return appendPOSIXValue(buf, path)
}

// AppendArgs appends the arguments in POSIX shell format -- this will leave arguments that have nonzero length and consist of
// alphanumeric characters unchanged, but it will wrap and escape other text.
func AppendArgs(buf []byte, args ...string) []byte {
	if len(args) == 0 {
		return buf
	}
	for _, arg := range args {
		buf = appendPOSIXValue(buf, arg)
		buf = append(buf, ' ')
	}
	// truncate off the trailing space.
	return buf[: len(buf)-1 : cap(buf)]
}

// AppendEnv appends the list of OS environment variables in POSIX shell format.  This will skip the shared prefix of env and os.Environ
// and environment variables that do not consist of alphanumeric variables followed by an = character.  Note that this still isn't fully
// POSIX compliant, since POSIX specifies that all environment variables must be uppercase but many programs use lowercase.
func AppendEnv(buf []byte, env ...string) []byte {
	return appendEnv(buf, variableEnv(novelEnv(env...)...)...)
}

// variableEnv returns env, skipping items that do not match rxValidEnv.
func variableEnv(env ...string) []string {
	vars := make([]string, 0, len(env))
	for _, str := range env {
		if rxValidEnv.MatchString(str) {
			vars = append(vars, str)
		}
	}
	return vars
}

var rxValidEnv = regexp.MustCompile(`^[A-Za-z0-9_]=`)

// novelEnv returns env, skipping items already present in os.Environ()
func novelEnv(env ...string) []string {
	global := os.Environ()
	n := len(env)
	if n > len(global) {
		n = len(global)
	}
	for i := 0; i < n; i++ {
		if env[i] != global[i] {
			return env[i:]
		}
	}
	return env[n:]
}

func appendEnv(buf []byte, env ...string) []byte {
	if len(env) == 0 {
		return buf
	}
	for _, env := range env {
		ofs := strings.IndexByte(env, '=')
		buf = append(buf, env[:ofs]...)
		buf = append(buf, '=')
		buf = appendPOSIXValue(buf, env)
		buf = append(buf, ' ')
	}
	// truncate off the trailing space.
	return buf[: len(buf)-1 : cap(buf)]
}

func appendPOSIXValue(buf []byte, str string) []byte {
	if rxValue.MatchString(str) {
		return append(buf, str...)
	}
	return appendPOSIXLiteral(buf, str)
}

func appendPOSIXLiteral(buf []byte, str string) []byte {
	buf = append(buf, '\'')
	for _, ch := range []byte(str) {
		switch ch {
		case '\r':
			buf = append(buf, '\r')
		case '\n':
			buf = append(buf, '\n')
		case '\t':
			buf = append(buf, '\t')
		case '\\', '\'':
			buf = append(buf, '\\', ch)
		default:
			buf = append(buf, ch)
		}
	}
	buf = append(buf, '\'')
	return buf
}

var rxCmd = regexp.MustCompile(`^[^ \r\n\t*?[\]{}|&;<>'` + "`" + `\\$#=!]+$`)
var rxValue = regexp.MustCompile(`^[^ \r\n\t*?[\]{}|&;<>'` + "`" + `\\$#!]+$`) // Note that = is okay here.
