// Package vexec provides a virtual process-execution abstraction for testing and production use.
// It wraps os/exec to provide a consistent, injectable interface for running external commands.
package vexec

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// Sentinel errors returned by the in-memory backend for misuse of the Cmd
// lifecycle. All are exported so callers can match them with errors.Is.
var (
	// ErrAlreadyStarted is returned when Start is called on a Cmd that has
	// already been started.
	ErrAlreadyStarted = errors.New("vexec: command already started")
	// ErrNotStarted is returned when Wait is called before Start.
	ErrNotStarted = errors.New("vexec: Wait called before Start")
	// ErrAlreadyWaited is returned when Wait is called more than once.
	ErrAlreadyWaited = errors.New("vexec: Wait called more than once")
	// ErrStdoutAlreadySet is returned from Output or CombinedOutput when
	// Stdout has been set via SetStdout.
	ErrStdoutAlreadySet = errors.New("vexec: Stdout already set")
	// ErrStderrAlreadySet is returned from CombinedOutput when Stderr has
	// been set via SetStderr.
	ErrStderrAlreadySet = errors.New("vexec: Stderr already set")
	// ErrProcessNotStarted is returned from Signal before Start.
	ErrProcessNotStarted = errors.New("vexec: process not started")
)

// Exec is the process-execution interface used throughout the codebase.
// It provides an abstraction over real and in-memory command execution.
type Exec interface {
	// Command prepares a command handle bound to ctx. The command is not
	// started until Run, Start, Output, or CombinedOutput is called.
	Command(ctx context.Context, name string, args ...string) Cmd
	// LookPath searches for an executable named file, mirroring exec.LookPath.
	LookPath(file string) (string, error)
}

// Cmd is a handle for a single command invocation.
//
// A Cmd may only be used once: after Run, Output, CombinedOutput, or Wait
// returns, subsequent calls to those methods return an error.
type Cmd interface {
	SetStdin(r io.Reader)
	SetStdout(w io.Writer)
	SetStderr(w io.Writer)
	SetEnv(env []string)
	SetDir(dir string)

	Run() error
	Start() error
	Wait() error
	Output() ([]byte, error)
	CombinedOutput() ([]byte, error)

	// ProcessState returns the post-exit state, or nil if the command has not
	// finished. The in-memory backend always returns nil; callers that need
	// exit codes should use ExitCode on the returned error instead.
	ProcessState() *os.ProcessState

	// SetCancel registers a function invoked once when the command's context
	// is canceled, before Wait returns. Mirrors exec.Cmd.Cancel. If unset,
	// the OS backend kills the process and the in-memory backend does nothing.
	SetCancel(fn func() error)

	// Signal sends sig to the running process, or returns ErrProcessNotStarted
	// if Start has not been called. The in-memory backend always returns nil.
	Signal(sig os.Signal) error
}

// ExitCoder is implemented by errors that carry a process exit code. The real
// backend returns *exec.ExitError, which already satisfies this interface;
// the in-memory backend returns a value that satisfies it too.
type ExitCoder interface {
	error
	ExitCode() int
}

// ExitCode extracts an exit code from err. It returns 0 if err is nil, or -1
// if err does not carry an exit code.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}

	if ec, ok := errors.AsType[ExitCoder](err); ok {
		return ec.ExitCode()
	}

	return -1
}

// Output runs name with args using e and returns its standard output.
func Output(e Exec, ctx context.Context, name string, args ...string) ([]byte, error) {
	return e.Command(ctx, name, args...).Output()
}

// CombinedOutput runs name with args using e and returns its combined
// standard output and standard error.
func CombinedOutput(e Exec, ctx context.Context, name string, args ...string) ([]byte, error) {
	return e.Command(ctx, name, args...).CombinedOutput()
}

// Run runs name with args using e and waits for it to complete.
func Run(e Exec, ctx context.Context, name string, args ...string) error {
	return e.Command(ctx, name, args...).Run()
}

// NewOSExec returns an Exec backed by the real operating system via os/exec.
func NewOSExec() Exec {
	return &osExec{}
}

type osExec struct{}

func (osExec) Command(ctx context.Context, name string, args ...string) Cmd {
	return &osCmd{cmd: exec.CommandContext(ctx, name, args...)}
}

func (osExec) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

type osCmd struct {
	cmd *exec.Cmd
}

func (c *osCmd) SetStdin(r io.Reader)  { c.cmd.Stdin = r }
func (c *osCmd) SetStdout(w io.Writer) { c.cmd.Stdout = w }
func (c *osCmd) SetStderr(w io.Writer) { c.cmd.Stderr = w }
func (c *osCmd) SetEnv(env []string)   { c.cmd.Env = env }
func (c *osCmd) SetDir(dir string)     { c.cmd.Dir = dir }

func (c *osCmd) SetCancel(fn func() error) { c.cmd.Cancel = fn }

func (c *osCmd) Signal(sig os.Signal) error {
	if c.cmd.Process == nil {
		return ErrProcessNotStarted
	}

	return c.cmd.Process.Signal(sig)
}

func (c *osCmd) Run() error                      { return c.cmd.Run() }
func (c *osCmd) Start() error                    { return c.cmd.Start() }
func (c *osCmd) Wait() error                     { return c.cmd.Wait() }
func (c *osCmd) Output() ([]byte, error)         { return c.cmd.Output() }
func (c *osCmd) CombinedOutput() ([]byte, error) { return c.cmd.CombinedOutput() }
func (c *osCmd) ProcessState() *os.ProcessState  { return c.cmd.ProcessState }

// Invocation describes a single command dispatched through the in-memory
// backend. Handlers inspect it to decide how to respond.
type Invocation struct {
	Stdin io.Reader
	Name  string
	Dir   string
	Args  []string
	Env   []string
}

// Result is the synthesized outcome of an in-memory command invocation.
// If Err is non-nil it is returned directly from Run/Start/Wait/Output;
// otherwise ExitCode is surfaced as an error satisfying ExitCoder when non-zero.
type Result struct {
	Err      error
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Handler processes an Invocation and returns a Result. It is invoked
// synchronously on the goroutine that calls Run/Start/Wait/Output.
type Handler func(ctx context.Context, inv Invocation) Result

// PathHandler resolves a binary name to a path for the in-memory backend.
// Returning a non-nil error causes LookPath to fail with that error.
type PathHandler func(file string) (string, error)

// MemExecOption configures an in-memory Exec.
type MemExecOption func(*memExec)

// WithLookPath overrides the default LookPath behavior of NewMemExec.
// By default, LookPath returns the input name unchanged.
func WithLookPath(p PathHandler) MemExecOption {
	return func(e *memExec) {
		e.lookPath = p
	}
}

// NewMemExec returns an Exec whose commands are dispatched to h instead of
// the operating system. It is intended for tests: h decides how each
// invocation should behave.
//
// h must not be nil. It is invoked synchronously; there is no implicit
// concurrency. LookPath returns the input name unchanged by default; use
// WithLookPath to customize.
func NewMemExec(h Handler, opts ...MemExecOption) Exec {
	if h == nil {
		panic("vexec: NewMemExec requires a non-nil Handler")
	}

	e := &memExec{handler: h}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

type memExec struct {
	handler  Handler
	lookPath PathHandler
}

func (e *memExec) Command(ctx context.Context, name string, args ...string) Cmd {
	return &memCmd{
		ctx:     ctx,
		handler: e.handler,
		name:    name,
		args:    args,
	}
}

func (e *memExec) LookPath(file string) (string, error) {
	if e.lookPath != nil {
		return e.lookPath(file)
	}

	return file, nil
}

// memCmd tracks a single in-memory invocation lifecycle.
type memCmd struct {
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	cancelErr  error
	ctx        context.Context
	handler    Handler
	cancel     func() error
	done       chan struct{}
	name       string
	dir        string
	args       []string
	env        []string
	result     Result
	cancelWG   sync.WaitGroup
	cancelOnce sync.Once
	started    bool
	waited     bool
}

func (c *memCmd) SetStdin(r io.Reader)  { c.stdin = r }
func (c *memCmd) SetStdout(w io.Writer) { c.stdout = w }
func (c *memCmd) SetStderr(w io.Writer) { c.stderr = w }
func (c *memCmd) SetEnv(env []string)   { c.env = env }
func (c *memCmd) SetDir(dir string)     { c.dir = dir }

func (c *memCmd) ProcessState() *os.ProcessState { return nil }

func (c *memCmd) SetCancel(fn func() error) { c.cancel = fn }

func (c *memCmd) Signal(os.Signal) error { return nil }

func (c *memCmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}

	return c.Wait()
}

func (c *memCmd) Start() error {
	if c.started {
		return ErrAlreadyStarted
	}

	c.started = true
	c.done = make(chan struct{})

	if c.cancel != nil {
		c.cancelWG.Go(func() {
			select {
			case <-c.ctx.Done():
				c.cancelOnce.Do(func() {
					c.cancelErr = c.cancel()
				})
			case <-c.done:
			}
		})
	}

	c.result = c.handler(c.ctx, c.invocation())
	close(c.done)
	c.cancelWG.Wait()

	return nil
}

func (c *memCmd) Wait() error {
	if !c.started {
		return ErrNotStarted
	}

	if c.waited {
		return ErrAlreadyWaited
	}

	c.waited = true

	if err := writeAll(c.stdout, c.result.Stdout); err != nil {
		return err
	}

	if err := writeAll(c.stderr, c.result.Stderr); err != nil {
		return err
	}

	if c.result.Err != nil {
		return c.result.Err
	}

	if c.result.ExitCode != 0 {
		return &exitError{code: c.result.ExitCode}
	}

	return c.cancelErr
}

func (c *memCmd) Output() ([]byte, error) {
	if c.stdout != nil {
		return nil, ErrStdoutAlreadySet
	}

	var buf bytes.Buffer

	c.stdout = &buf

	err := c.Run()

	return buf.Bytes(), err
}

func (c *memCmd) CombinedOutput() ([]byte, error) {
	if c.stdout != nil {
		return nil, ErrStdoutAlreadySet
	}

	if c.stderr != nil {
		return nil, ErrStderrAlreadySet
	}

	var buf bytes.Buffer

	c.stdout = &buf
	c.stderr = &buf

	err := c.Run()

	return buf.Bytes(), err
}

func (c *memCmd) invocation() Invocation {
	return Invocation{
		Name:  c.name,
		Args:  c.args,
		Env:   c.env,
		Dir:   c.dir,
		Stdin: c.stdin,
	}
}

func writeAll(w io.Writer, p []byte) error {
	if w == nil || len(p) == 0 {
		return nil
	}

	_, err := w.Write(p)

	return err
}

// exitError is returned from memCmd.Run/Wait when the handler reports a
// non-zero exit code without an explicit Err. It satisfies ExitCoder so
// callers can recover the exit code via errors.As.
type exitError struct {
	code int
}

func (e *exitError) Error() string { return "exit status " + strconv.Itoa(e.code) }
func (e *exitError) ExitCode() int { return e.code }
