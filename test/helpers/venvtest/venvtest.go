// Package venvtest builds in-memory [venv.Venv] values for tests. New seeds
// the mem defaults; callers refine individual handles with venv.Venv's fluent
// With methods (WithHandler, WithExec, WithFS, WithSops, WithEnv, WithGOOS,
// WithUserHomeDir). Production code builds venvs through [venv.OSVenv] instead.
package venvtest

import (
	"bufio"
	"io"
	"runtime"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// Cache and temp roots for the in-memory bundle. They sit under a name no real
// machine uses, so a test that accidentally runs them against the OS filesystem
// fails loudly instead of writing into the invoking user's directories.
const (
	memCacheDir = "/venvtest/cache"
	memTempDir  = "/venvtest/tmp"
)

// New returns an in-memory venv: a fail-closed exec, an in-memory filesystem,
// a fail-closed no-network HTTP client, a mem SOPS decrypter yielding empty
// cleartext, an empty (non-nil) environment, deterministic platform handles,
// an empty console reader, and both writers wired to [io.Discard]. Refine it
// with venv.Venv's fluent With methods.
//
// A test whose subject runs a command supplies a handler through WithHandler.
// Until it does, the command fails with [vexec.ErrNoSpawn] rather than
// reporting success with empty output, which a caller reading the command's
// stdout would take as a real answer.
func New() *venv.Venv {
	return &venv.Venv{
		Exec:   vexec.NewNoSpawnExec(),
		FS:     vfs.NewMemMapFS(),
		HTTP:   vhttp.NewNoNetworkClient(),
		Sops:   vsops.NewMemDecrypter(func(string, string) ([]byte, error) { return nil, nil }),
		Reader: bufio.NewReader(strings.NewReader("")),
		Env:    map[string]string{},
		Platform: &venv.Platform{
			UserHomeDir: func() (string, error) {
				return "", nil
			},
			UserCacheDir: func() (string, error) {
				return memCacheDir, nil
			},
			TempDir: func() string {
				return memTempDir
			},
			GOOS: runtime.GOOS,
		},
		Writers: &writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}
}

// NewWithOSFS returns [New] with the real filesystem swapped in, for tests
// whose fixtures live on disk but which need nothing else live.
func NewWithOSFS() *venv.Venv {
	return New().WithFS(vfs.NewOSFS())
}

// NewOSWithEmptyEnv returns the OS-backed bundle with an empty environment, so
// a variable set on the machine running the suite cannot reach the code under
// test. Tests that drive the real filesystem and real subprocesses need it;
// prefer [NewWithOSFS] when only the filesystem has to be real.
func NewOSWithEmptyEnv() *venv.Venv {
	return venv.OSVenv().WithEnv(map[string]string{})
}
