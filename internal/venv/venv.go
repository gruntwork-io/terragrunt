// Package venv defines the root virtualized environment threaded from the
// Terragrunt binary entrypoint down through the CLI and its commands.
//
// A [Venv] bundles the side-effect handles every layer below the CLI needs
// to do its work: [vfs.FS] for filesystem reads and writes, [vexec.Exec]
// for spawning subprocesses, [vhttp.Client] for outbound HTTP,
// [vsops.Decrypter] for SOPS decryption, the shell environment variables and
// platform handles read at startup, the stdin reader, the console
// characteristics output adapts to, and the stdout/stderr writers. Production
// code constructs the real bundle once at the top via [OSVenv]; tests
// construct an in-memory bundle and drive the full CLI through it.
//
// This is the one Venv type threaded through the codebase. A package may
// define its own local Venv only when its handle set genuinely differs
// from what this bundle carries.
package venv

import (
	"context"
	"errors"
	"io"
	"maps"
	"net"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"golang.org/x/term"
)

// ErrVenvEnvUnset is the panic value [Venv.RequireEnv] raises when Env is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Env rather than a runtime condition.
var ErrVenvEnvUnset = errors.New("venv.Venv.Env is required but unset")

// ErrVenvEnvNil is the panic value [Venv.WithEnv] raises when its env
// argument is nil. Every caller builds the map it passes, so a nil points
// at a caller bug rather than a runtime condition.
var ErrVenvEnvNil = errors.New("venv.Venv.WithEnv: env must not be nil")

// ErrVenvFSUnset is the panic value [Venv.RequireFS] raises when FS is nil.
// Production callers build the Venv through [OSVenv], so it points at a test
// that forgot to set FS rather than a runtime condition.
var ErrVenvFSUnset = errors.New("venv.Venv.FS is required but unset")

// ErrVenvExecUnset is the panic value [Venv.RequireExec] raises when Exec is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Exec rather than a runtime condition.
var ErrVenvExecUnset = errors.New("venv.Venv.Exec is required but unset")

// ErrVenvHTTPUnset is the panic value [Venv.RequireHTTP] raises when HTTP is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set HTTP rather than a runtime condition.
var ErrVenvHTTPUnset = errors.New("venv.Venv.HTTP is required but unset")

// ErrVenvBrowserUnset is the panic value [Venv.RequireBrowser] raises when
// Browser is nil. Production callers build the Venv through [OSVenv], so it
// points at a test that forgot to set Browser rather than a runtime condition.
var ErrVenvBrowserUnset = errors.New("venv.Venv.Browser is required but unset")

// ErrVenvStdinUnset is the panic value [Venv.RequireStdin] raises when Stdin is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Stdin rather than a runtime condition.
var ErrVenvStdinUnset = errors.New("venv.Venv.Stdin is required but unset")

// ErrVenvTerminalUnset is the panic value [Venv.RequireTerminal] raises when
// Terminal is nil. Production callers build the Venv through [OSVenv], so it
// points at a test that forgot to set Terminal rather than a runtime condition.
var ErrVenvTerminalUnset = errors.New("venv.Venv.Terminal is required but unset")

// ErrVenvWritersUnset is the panic value [Venv.RequireWriters] raises when
// Writers, or either writer it carries, is nil. Production callers build the
// Venv through [OSVenv], so it points at a test that forgot to set Writers
// rather than a runtime condition.
var ErrVenvWritersUnset = errors.New("venv.Venv.Writers is required but unset")

// ErrVenvListenUnset is the panic value [Venv.RequireListen] raises when Listen
// is nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Listen rather than a runtime condition.
var ErrVenvListenUnset = errors.New("venv.Venv.Listen is required but unset")

// ErrVenvPlatformUnset is the panic value [Venv.RequirePlatform] raises when
// Platform is nil. Production callers build the Venv through [OSVenv], so it
// points at a test that forgot to set Platform rather than a runtime condition.
var ErrVenvPlatformUnset = errors.New("venv.Venv.Platform is required but unset")

// ErrVenvGOOSUnset is the panic value [Venv.RequireGOOS] raises when GOOS is empty.
var ErrVenvGOOSUnset = errors.New("venv.Venv.Platform.GOOS is required but unset")

// ErrVenvGOARCHUnset is the panic value [Venv.RequireGOARCH] raises when GOARCH is empty.
var ErrVenvGOARCHUnset = errors.New("venv.Venv.Platform.GOARCH is required but unset")

// ErrVenvUserHomeDirUnset is the panic value [Venv.RequireUserHomeDir] raises
// when UserHomeDir is nil.
var ErrVenvUserHomeDirUnset = errors.New("venv.Venv.Platform.UserHomeDir is required but unset")

// ErrVenvUserCacheDirUnset is the panic value [Venv.RequireUserCacheDir] raises
// when UserCacheDir is nil.
var ErrVenvUserCacheDirUnset = errors.New("venv.Venv.Platform.UserCacheDir is required but unset")

// ErrVenvTempDirUnset is the panic value [Venv.RequireTempDir] raises when
// TempDir is nil.
var ErrVenvTempDirUnset = errors.New("venv.Venv.Platform.TempDir is required but unset")

// Platform carries the operating-system handles used below the CLI boundary.
type Platform struct {
	UserHomeDir  func() (string, error)
	UserCacheDir func() (string, error)
	TempDir      func() string
	Getwd        func() (string, error)
	GetPID       func() int
	GOOS         string
	GOARCH       string
}

// Terminal reports the console a run's output is adapting to: whether each
// standard stream is attached to a terminal, and how wide that terminal is.
// Width reports 0 when no width is available, which callers read as "do not
// wrap" or replace with a default of their own.
type Terminal struct {
	StdinIsTTY  func() bool
	StdoutIsTTY func() bool
	StderrIsTTY func() bool
	Width       func() int
}

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

// WithGOOS returns a copy of v whose operating-system identifier is goos.
func (v *Venv) WithGOOS(goos string) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.GOOS = goos

	c := *v
	c.Platform = &platform

	return &c
}

// WithGOARCH returns a copy of v whose architecture identifier is goarch.
func (v *Venv) WithGOARCH(goarch string) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.GOARCH = goarch

	c := *v
	c.Platform = &platform

	return &c
}

// WithUserHomeDir returns a copy of v whose home-directory lookup is userHomeDir.
func (v *Venv) WithUserHomeDir(userHomeDir func() (string, error)) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.UserHomeDir = userHomeDir

	c := *v
	c.Platform = &platform

	return &c
}

// WithTempDir returns a copy of v whose temp-directory lookup is tempDir.
func (v *Venv) WithTempDir(tempDir func() string) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.TempDir = tempDir

	c := *v
	c.Platform = &platform

	return &c
}

// WithEnv returns a copy of v whose shell environment is env. It panics
// with [ErrVenvEnvNil] on a nil env so the result always satisfies
// [Venv.RequireEnv].
func (v *Venv) WithEnv(env map[string]string) *Venv {
	if env == nil {
		panic(ErrVenvEnvNil)
	}

	c := *v
	c.Env = env

	return &c
}

// WithEnvCloned returns a copy of v whose Env is an independent clone. Fan-out
// paths that process units one at a time hand each unit a clone so
// per-unit mutations (obtained credentials, TF_VAR_* contributions) never
// leak into sibling units. It panics with [ErrVenvEnvUnset], per
// [Venv.RequireEnv], when Env is nil.
func (v *Venv) WithEnvCloned() *Venv {
	v.RequireEnv()

	return v.WithEnv(maps.Clone(v.Env))
}

// RequireEnv panics with [ErrVenvEnvUnset] when Env is nil, guarding
// functions that write into the shared environment.
func (v *Venv) RequireEnv() {
	if v.Env == nil {
		panic(ErrVenvEnvUnset)
	}
}

// RequireEnvMap panics with [ErrVenvEnvUnset] when env is nil. A nil map reads
// as empty rather than failing, so a function handed one would resolve every
// lookup to "" and report a run that simply found no environment. Callers that
// take an environment as a parameter assert it here.
func RequireEnvMap(env map[string]string) {
	if env == nil {
		panic(ErrVenvEnvUnset)
	}
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

// RequireTerminal panics with [ErrVenvTerminalUnset] when Terminal is nil.
// Functions that size or color their output call this as their first
// statement so a missing handle panics at the offending call site instead of
// inside an unrelated stack frame.
func (v *Venv) RequireTerminal() {
	if v.Terminal == nil {
		panic(ErrVenvTerminalUnset)
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

// RequirePlatform panics with [ErrVenvPlatformUnset] when Platform is nil.
// The platform builders call this so that refining one handle on a Venv that
// carries no platform at all fails at the builder rather than silently
// producing a platform whose other handles are nil.
func (v *Venv) RequirePlatform() {
	if v.Platform == nil {
		panic(ErrVenvPlatformUnset)
	}
}

// RequireGOOS panics with [ErrVenvGOOSUnset] when GOOS is empty.
func (v *Venv) RequireGOOS() {
	if v.Platform == nil || v.Platform.GOOS == "" {
		panic(ErrVenvGOOSUnset)
	}
}

// RequireGOARCH panics with [ErrVenvGOARCHUnset] when GOARCH is empty. The two
// halves of a platform identifier have to come from the same place: a run that
// took its GOOS from the venv and its architecture from the binary would name
// a target that exists on no machine.
func (v *Venv) RequireGOARCH() {
	if v.Platform == nil || v.Platform.GOARCH == "" {
		panic(ErrVenvGOARCHUnset)
	}
}

// RequireUserHomeDir panics with [ErrVenvUserHomeDirUnset] when UserHomeDir is nil.
func (v *Venv) RequireUserHomeDir() {
	if v.Platform == nil || v.Platform.UserHomeDir == nil {
		panic(ErrVenvUserHomeDirUnset)
	}
}

// RequireUserCacheDir panics with [ErrVenvUserCacheDirUnset] when UserCacheDir is nil.
func (v *Venv) RequireUserCacheDir() {
	if v.Platform == nil || v.Platform.UserCacheDir == nil {
		panic(ErrVenvUserCacheDirUnset)
	}
}

// RequireTempDir panics with [ErrVenvTempDirUnset] when TempDir is nil.
func (v *Venv) RequireTempDir() {
	if v.Platform == nil || v.Platform.TempDir == nil {
		panic(ErrVenvTempDirUnset)
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
		Stdin:   os.Stdin,
		Env:     ParseEnviron(os.Environ()),
		Platform: &Platform{
			UserHomeDir:  os.UserHomeDir,
			UserCacheDir: os.UserCacheDir,
			TempDir:      os.TempDir,
			Getwd:        os.Getwd,
			GetPID:       os.Getpid,
			GOOS:         runtime.GOOS,
			GOARCH:       runtime.GOARCH,
		},
		Terminal: &Terminal{
			StdinIsTTY:  func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
			StdoutIsTTY: func() bool { return term.IsTerminal(int(os.Stdout.Fd())) },
			StderrIsTTY: func() bool { return term.IsTerminal(int(os.Stderr.Fd())) },
			Width:       osTerminalWidth,
		},
		Writers: &writer.Writers{Writer: os.Stdout, ErrWriter: os.Stderr},
	}
}

// osTerminalWidth reports the real terminal's width, and 0 when stdout is not
// a terminal (a pipe, a file, a CI log). Callers that need a number either way
// substitute their own default for the 0.
func osTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}

	return width
}

// Environ turns an environment map back into os.Environ-style KEY=VALUE
// entries, sorted so the result does not vary with map iteration order. It is
// the inverse of [ParseEnviron].
func Environ(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}

	slices.Sort(out)

	return out
}

// ParseEnviron turns os.Environ-style KEY=VALUE entries into a map, splitting
// on the first "=" after the leading byte. That leading byte is skipped so the
// Windows per-drive working-directory variables, whose names begin with "="
// (e.g. "=C:"), keep their names intact. Entries without a separator are dropped.
func ParseEnviron(environ []string) map[string]string {
	out := make(map[string]string, len(environ))

	for _, entry := range environ {
		if entry == "" {
			continue
		}

		i := strings.IndexByte(entry[1:], '=')
		if i < 0 {
			continue
		}

		out[entry[:i+1]] = entry[i+2:]
	}

	return out
}
