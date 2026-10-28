// Package venv defines the root virtualized environment threaded from the
// Terragrunt binary entrypoint down through the CLI and its commands.
//
// A [Venv] bundles the side-effect handles every layer below the CLI needs
// to do its work: [vfs.FS] for filesystem reads and writes, [vexec.Exec]
// for spawning subprocesses, [vhttp.Client] for outbound HTTP,
// [vsops.Decrypter] for SOPS decryption, the shell environment variables and
// platform handles read at startup, the OS signals a run waits on, the stdin
// reader, the console characteristics output adapts to, and the stdout/stderr
// writers. Production code constructs the real bundle once at the top via
// [OSVenv]; tests construct an in-memory bundle and drive the full CLI through
// it.
//
// This is the one Venv type threaded through the codebase. A package may
// define its own local Venv only when its handle set genuinely differs
// from what this bundle carries.
package venv

import (
	"context"
	"io"
	"net"
	"os"
	"runtime"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"golang.org/x/term"
)

// Venv is the root virtualized environment. It carries the filesystem,
// process-execution, HTTP, SOPS-decryption, browser, environment-variable,
// platform, and writer handles that every Terragrunt operation needs. Env is shared by
// reference across the run and mutated in place as provider-cache, hook, and
// inputs contributions resolve. Writers is held as a pointer so per-call
// overrides via [writer.Writers.WithWriter] and [writer.Writers.WithErrWriter]
// produce fresh pointers without mutating the caller's value; never mutate its
// fields in place, since shallow-copied Venvs share the pointer.
//
// Stdin is the one console input for the whole run: Terragrunt's own prompts
// read it, and a spawned command inherits it. Nothing between them may buffer
// it. Read-ahead held in a buffer is invisible to every other consumer, so a
// prompt that grabbed more than its line strands the rest, and handing a
// buffered reader to os/exec makes it copy the stream into the child's pipe to
// EOF whether the child reads or not, which is enough for one incidental
// `tofu -version` to swallow the whole input.
type Venv struct {
	FS       vfs.FS
	Exec     vexec.Exec
	HTTP     vhttp.Client
	Sops     vsops.Decrypter
	Browser  vbrowser.Opener
	Listen   Listener
	Signals  signal.NotifierFunc
	Stdin    io.Reader
	Env      map[string]string
	Platform *Platform
	Terminal *Terminal
	Writers  *writer.Writers
}

// Listener opens a socket to serve on. Terragrunt listens for one thing, the
// provider-cache server that the tofu subprocess fetches providers from, so
// production supplies the real dialer and an in-memory bundle supplies one that
// refuses: a run with no subprocess has nothing to serve.
type Listener func(ctx context.Context, network, addr string) (net.Listener, error)

// WithStdin returns a copy of v whose subprocess standard input is r. This is
// the stream a spawned command inherits, not the one Terragrunt prompts from;
// see [Venv] for why they are separate.
func (v *Venv) WithStdin(r io.Reader) *Venv {
	c := *v
	c.Stdin = r

	return &c
}

// WithWriter returns a copy of v whose primary writer is w. The copy gets
// a fresh Writers pointer so the caller's venv is untouched.
func (v *Venv) WithWriter(w io.Writer) *Venv {
	c := *v
	c.Writers = c.Writers.WithWriter(w)

	return &c
}

// WithErrWriter returns a copy of v whose error writer is w. The copy gets
// a fresh Writers pointer so the caller's venv is untouched.
func (v *Venv) WithErrWriter(w io.Writer) *Venv {
	c := *v
	c.Writers = c.Writers.WithErrWriter(w)

	return &c
}

// WithExec returns a copy of v whose process executor is exec.
func (v *Venv) WithExec(exec vexec.Exec) *Venv {
	c := *v
	c.Exec = exec

	return &c
}

// WithHandler returns a copy of v whose executor is an in-memory exec driven
// by h, for the in-memory test bundles this package serves.
func (v *Venv) WithHandler(h vexec.Handler) *Venv {
	c := *v
	c.Exec = vexec.NewMemExec(h)

	return &c
}

// WithHTTP returns a copy of v whose outbound HTTP client is c.
func (v *Venv) WithHTTP(c vhttp.Client) *Venv {
	cp := *v
	cp.HTTP = c

	return &cp
}

// WithBrowser returns a copy of v whose browser opener is o.
func (v *Venv) WithBrowser(o vbrowser.Opener) *Venv {
	c := *v
	c.Browser = o

	return &c
}

// WithSignals returns a copy of v whose OS signals are delivered by n.
func (v *Venv) WithSignals(n signal.NotifierFunc) *Venv {
	c := *v
	c.Signals = n

	return &c
}

// WithSops returns a copy of v whose SOPS decrypter is d.
func (v *Venv) WithSops(d vsops.Decrypter) *Venv {
	c := *v
	c.Sops = d

	return &c
}

// WithFS returns a copy of v backed by fsys.
func (v *Venv) WithFS(fsys vfs.FS) *Venv {
	c := *v
	c.FS = fsys

	return &c
}

// RequireFS panics with [ErrVenvFSUnset] when FS is nil. Functions that
// touch the filesystem call this as their first statement so a missing
// handle panics at the offending call site instead of inside an unrelated
// stack frame.
func (v *Venv) RequireFS() {
	if v.FS == nil {
		panic(ErrVenvFSUnset)
	}
}

// RequireExec panics with [ErrVenvExecUnset] when Exec is nil. Functions
// that spawn subprocesses call this as their first statement so a missing
// handle panics at the offending call site instead of inside an unrelated
// stack frame.
func (v *Venv) RequireExec() {
	if v.Exec == nil {
		panic(ErrVenvExecUnset)
	}
}

// RequireHTTP panics with [ErrVenvHTTPUnset] when HTTP is nil. Functions
// that probe over HTTP call this as their first statement so a missing
// handle panics at the offending call site instead of inside an unrelated
// stack frame.
func (v *Venv) RequireHTTP() {
	if v.HTTP == nil {
		panic(ErrVenvHTTPUnset)
	}
}

// RequireBrowser panics with [ErrVenvBrowserUnset] when Browser is nil.
func (v *Venv) RequireBrowser() {
	if v.Browser == nil {
		panic(ErrVenvBrowserUnset)
	}
}

// RequireWriters panics with [ErrVenvWritersUnset] when Writers, or either
// writer it carries, is nil. A nil writer reaching a subprocess or a formatter
// panics wherever the write happens, which is nowhere near the caller that
// dropped it, so functions that hand the writers onwards assert them first.
func (v *Venv) RequireWriters() {
	if v.Writers == nil || v.Writers.Writer == nil || v.Writers.ErrWriter == nil {
		panic(ErrVenvWritersUnset)
	}
}

// RequireStdin panics with [ErrVenvStdinUnset] when Stdin is nil. Functions
// that hand a subprocess its standard input call this as their first statement
// so a missing handle panics at the offending call site instead of inside an
// unrelated stack frame.
func (v *Venv) RequireStdin() {
	if v.Stdin == nil {
		panic(ErrVenvStdinUnset)
	}
}

// RequireListen panics with [ErrVenvListenUnset] when Listen is nil. Functions
// that open a socket call this as their first statement so a missing handle
// panics at the offending call site instead of inside an unrelated stack frame.
func (v *Venv) RequireListen() {
	if v.Listen == nil {
		panic(ErrVenvListenUnset)
	}
}

// RequireSignals panics with [ErrVenvSignalsUnset] when Signals is nil.
// Functions that wait on an OS signal call this as their first statement so a
// missing handle panics at the offending call site instead of inside an
// unrelated stack frame.
func (v *Venv) RequireSignals() {
	if v.Signals == nil {
		panic(ErrVenvSignalsUnset)
	}
}

// OSVenv builds the production [Venv]: the real OS filesystem, the real OS
// process executor, the real outbound HTTP client, platform handles, a
// snapshot of the OS environment, and stdin/stdout/stderr wired to the real
// OS streams.
//
// It returns a *[Venv] so the bundle is threaded by pointer through every
// downstream call: small parameter, no copying. Shallow-copying a
// pointed-to [Venv] (via `local := *v`) still shares the Env map with the
// original, so callers must go through [Venv.WithEnvCloned] before mutating
// environment variables; writer swaps stay independent because
// [writer.Writers.WithWriter] returns a fresh copy.
func OSVenv() *Venv {
	return &Venv{
		FS:      vfs.NewOSFS(),
		Exec:    vexec.NewOSExec(),
		HTTP:    vhttp.NewOSClient(),
		Sops:    vsops.NewOSDecrypter(),
		Browser: vbrowser.NewOSOpener(),
		Listen:  (&net.ListenConfig{}).Listen,
		Signals: signal.NotifierWithContext,
		Stdin:   os.Stdin,
		Env:     ParseEnviron(os.Environ()),
		Platform: &Platform{
			UserHomeDir:    os.UserHomeDir,
			UserCacheDir:   os.UserCacheDir,
			UserConfigDir:  os.UserConfigDir,
			TempDir:        os.TempDir,
			Getwd:          os.Getwd,
			GetPID:         os.Getpid,
			ReplaceEnviron: replaceOSEnviron,
			GOOS:           runtime.GOOS,
			GOARCH:         runtime.GOARCH,
		},
		Terminal: &Terminal{
			StdinIsTTY:           func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
			StdoutIsTTY:          func() bool { return term.IsTerminal(int(os.Stdout.Fd())) },
			StderrIsTTY:          func() bool { return term.IsTerminal(int(os.Stderr.Fd())) },
			StderrHandlesEscapes: osStderrHandlesEscapes,
			Width:                osTerminalWidth,
			ErrWidth:             osTerminalErrWidth,
		},
		Writers: &writer.Writers{Writer: os.Stdout, ErrWriter: os.Stderr},
	}
}
