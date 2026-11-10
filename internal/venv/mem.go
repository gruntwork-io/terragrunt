package venv

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// ErrNoListen is what the in-memory bundle's listener returns. A caller whose
// subject serves on a socket says so by supplying its own; until it does,
// binding a port fails rather than quietly claiming one on the machine.
var ErrNoListen = errors.New("venv: listening is not permitted")

// Cache and temp roots for the in-memory bundle. They sit under a name no real
// machine uses, so code that accidentally runs them against the OS filesystem
// fails loudly instead of writing into the invoking user's directories.
var (
	memCacheDir  = memRoot("/memvenv/cache")
	memConfigDir = memRoot("/memvenv/config")
	memTempDir   = memRoot("/memvenv/tmp")
	memWorkDir   = memRoot("/memvenv/work")
)

// memPID is a fixed process id, so a crash report or log line carrying it
// compares equal between runs.
const memPID = 1

const memUID = 1000

// MemVenv returns an in-memory [Venv]: a fail-closed exec, an in-memory
// filesystem, a fail-closed no-network HTTP client, a mem SOPS decrypter
// yielding empty cleartext, a fail-closed browser opener, a signal notifier
// that never delivers, an empty (non-nil) environment, deterministic platform
// handles, an empty console reader, a console that is no stream's terminal
// and has no width, and both writers wired to [io.Discard]. Refine it with the
// fluent With methods.
//
// Reporting no terminal is what a piped or captured run reports, so color and
// wrapping stay off and output does not shift with the terminal the process
// happens to run under.
//
// A caller whose subject runs a command supplies a handler through
// [Venv.WithHandler]. Until it does, the command fails with [vexec.ErrNoSpawn]
// rather than reporting success with empty output, which a caller reading the
// command's stdout would take as a real answer. The browser refuses on the
// same principle, so a test suite never launches a window on the machine
// running it.
func MemVenv() *Venv {
	return &Venv{
		Exec:    vexec.NewNoSpawnExec(),
		FS:      vfs.NewMemMapFS(),
		HTTP:    vhttp.NewNoNetworkClient(),
		Sops:    vsops.NewMemDecrypter(func(map[string]string, string, string) ([]byte, error) { return nil, nil }),
		Browser: vbrowser.NewNoBrowserOpener(),
		Listen: func(context.Context, string, string) (net.Listener, error) {
			return nil, ErrNoListen
		},
		Signals: func(context.Context, signal.NotifyFunc, ...os.Signal) {},
		Stdin:   strings.NewReader(""),
		Env:     map[string]string{},
		Platform: &Platform{
			UserHomeDir: func() (string, error) {
				return "", nil
			},
			UserCacheDir: func() (string, error) {
				return memCacheDir, nil
			},
			UserConfigDir: func() (string, error) {
				return memConfigDir, nil
			},
			TempDir: func() string {
				return memTempDir
			},
			Getwd: func() (string, error) {
				return memWorkDir, nil
			},
			GetPID: func() int {
				return memPID
			},
			GetUID: func() int {
				return memUID
			},
			ReplaceEnviron: func(map[string]string) error {
				return nil
			},
			GOOS:   runtime.GOOS,
			GOARCH: runtime.GOARCH,
		},
		Terminal: &Terminal{
			StdinIsTTY:  func() bool { return false },
			StdoutIsTTY: func() bool { return false },
			StderrIsTTY: func() bool { return false },
			Width:       func() int { return 0 },
		},
		Writers: &writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}
}

// memRoot returns a slash-rooted path as the absolute path the host resolves
// it to. On Windows "/memvenv" alone is not absolute, so it takes the volume
// of the working directory, which is also where code that runs filepath.Abs
// on the same value lands.
func memRoot(path string) string {
	abs, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		panic("venv: resolving in-memory root " + path + ": " + err.Error())
	}

	return abs
}
