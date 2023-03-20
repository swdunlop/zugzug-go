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
	"sync"

	"github.com/swdunlop/zugzug-go/zug"
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
	cmd := Command(ctx, name, args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// Run will run the provided command with the provided arguments, returning the error if any.
func Run(ctx context.Context, name string, args ...string) error {
	return Command(ctx, name, args...).Run()
}

// Command returns a new command with the provided name and arguments with Dir, Stdout, Stderr, Stdin and Environment
// configured by the console.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cfg := from(ctx)
	cmd.Dir = cfg.dir
	cmd.Stdout = cfg.stdout
	cmd.Stderr = cfg.stderr
	cmd.Stdin = cfg.stdin
	cmd.Env = cfg.env
	return cmd
}

// With will derive a new context with the provided options.
func With(ctx context.Context, options ...Option) (context.Context, error) {
	cfg := *from(ctx)
	for _, option := range options {
		option(&cfg)
	}
	return context.WithValue(ctx, ctxConsole{}, &cfg), nil
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

type Interface interface {
	// Dir returns the current working directory.
	Dir() string

	// Env returns the current environment.
	Env() []string

	// Stdout returns the current stdout writer.
	Stdout() io.Writer

	// Stderr returns the current stderr writer.
	Stderr() io.Writer
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
}

func (c *config) Dir() string       { return c.dir }
func (c *config) Env() []string     { return c.env }
func (c *config) Stdout() io.Writer { return c.stdout }
func (c *config) Stderr() io.Writer { return c.stderr }
func (c *config) Stdin() io.Reader  { return c.stdin }
