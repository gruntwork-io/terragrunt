package venv_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVenvRequireFS pins the FS contract: the zero Venv panics with the
// sentinel, a populated Venv passes.
func TestVenvRequireFS(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, venv.ErrVenvFSUnset, func() {
		(&venv.Venv{}).RequireFS()
	})

	assert.NotPanics(t, func() {
		(&venv.Venv{FS: vfs.NewOSFS()}).RequireFS()
	})
}

// TestVenvRequireExec pins the Exec contract. A Venv with FS but no Exec
// must still panic; only a populated Exec satisfies the check.
func TestVenvRequireExec(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, venv.ErrVenvExecUnset, func() {
		(&venv.Venv{FS: vfs.NewOSFS()}).RequireExec()
	})

	assert.NotPanics(t, func() {
		(&venv.Venv{Exec: vexec.NewOSExec()}).RequireExec()
	})
}

func TestVenvWriterBuildersIsolateCopies(t *testing.T) {
	t.Parallel()

	original := venv.OSVenv()
	originalWriter := original.Writers.Writer
	originalErrWriter := original.Writers.ErrWriter

	got := original.WithWriter(io.Discard).WithErrWriter(io.Discard)

	require.NotSame(t, original.Writers, got.Writers)
	assert.Equal(t, io.Discard, got.Writers.Writer)
	assert.Equal(t, io.Discard, got.Writers.ErrWriter)
	assert.Equal(t, originalWriter, original.Writers.Writer)
	assert.Equal(t, originalErrWriter, original.Writers.ErrWriter)
}

func TestVenvPlatformRequirements(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, venv.ErrVenvFSUnset, func() {
		(&venv.Venv{}).RequireFS()
	})
	assert.PanicsWithValue(t, venv.ErrVenvEnvUnset, func() {
		venv.RequireEnvMap(nil)
	})
	assert.NotPanics(t, func() {
		venv.RequireEnvMap(map[string]string{})
	})
	assert.PanicsWithValue(t, venv.ErrVenvGOOSUnset, func() {
		(&venv.Venv{}).RequireGOOS()
	})
	assert.PanicsWithValue(t, venv.ErrVenvGOARCHUnset, func() {
		(&venv.Venv{}).RequireGOARCH()
	})
	assert.PanicsWithValue(t, venv.ErrVenvUserHomeDirUnset, func() {
		(&venv.Venv{}).RequireUserHomeDir()
	})
	assert.PanicsWithValue(t, venv.ErrVenvUserConfigDirUnset, func() {
		(&venv.Venv{}).RequireUserConfigDir()
	})
	assert.PanicsWithValue(t, venv.ErrVenvPlatformUnset, func() {
		(&venv.Venv{}).RequirePlatform()
	})
	assert.PanicsWithValue(t, venv.ErrVenvWritersUnset, func() {
		(&venv.Venv{}).RequireWriters()
	})
	assert.PanicsWithValue(t, venv.ErrVenvListenUnset, func() {
		(&venv.Venv{}).RequireListen()
	})
	assert.PanicsWithValue(t, venv.ErrVenvSignalsUnset, func() {
		(&venv.Venv{}).RequireSignals()
	})
	assert.PanicsWithValue(t, venv.ErrVenvStdinUnset, func() {
		(&venv.Venv{}).RequireStdin()
	})
	assert.PanicsWithValue(t, venv.ErrVenvWritersUnset, func() {
		// A non-nil Writers whose fields are nil is the case a bare nil check
		// would wave through, and the one that fails furthest from its cause.
		(&venv.Venv{Writers: &writer.Writers{}}).RequireWriters()
	})
}

// TestVenvHandleBuildersReturnCopies pins the builder contract: the returned
// copy carries the new handle, the receiver keeps none of them.
func TestVenvHandleBuildersReturnCopies(t *testing.T) {
	t.Parallel()

	memFS := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(memFS, "/tracer.txt", []byte("tracer"), 0o644))

	wantSopsErr := errors.New("decrypt failed")
	wantBrowserErr := errors.New("browser opened")
	echo := vexec.Handler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte(inv.Name + " " + strings.Join(inv.Args, " "))}
	})

	testCases := []struct {
		build  func(v *venv.Venv) *venv.Venv
		verify func(t *testing.T, got *venv.Venv)
		name   string
	}{
		{
			name:  "WithFS",
			build: func(v *venv.Venv) *venv.Venv { return v.WithFS(memFS) },
			verify: func(t *testing.T, got *venv.Venv) {
				t.Helper()

				data, err := vfs.ReadFile(got.FS, "/tracer.txt")
				require.NoError(t, err)
				assert.Equal(t, "tracer", string(data))
			},
		},
		{
			name:   "WithExec",
			build:  func(v *venv.Venv) *venv.Venv { return v.WithExec(vexec.NewMemExec(echo)) },
			verify: assertExecEchoes,
		},
		{
			name:   "WithHandler",
			build:  func(v *venv.Venv) *venv.Venv { return v.WithHandler(echo) },
			verify: assertExecEchoes,
		},
		{
			name: "WithSops",
			build: func(v *venv.Venv) *venv.Venv {
				return v.WithSops(vsops.NewMemDecrypter(func(map[string]string, string, string) ([]byte, error) {
					return nil, wantSopsErr
				}))
			},
			verify: func(t *testing.T, got *venv.Venv) {
				t.Helper()

				_, err := got.Sops.DecryptFile(map[string]string{}, "/secrets.yaml", "yaml")
				require.ErrorIs(t, err, wantSopsErr)
			},
		},
		{
			name: "WithHTTP",
			build: func(v *venv.Venv) *venv.Venv {
				return v.WithHTTP(vhttp.NewMemClient(teapotHandler))
			},
			verify: func(t *testing.T, got *venv.Venv) {
				t.Helper()

				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", nil)
				require.NoError(t, err)

				resp, err := got.HTTP.Do(req)
				require.NoError(t, err)

				defer resp.Body.Close()

				assert.Equal(t, http.StatusTeapot, resp.StatusCode)
			},
		},
		{
			name: "WithBrowser",
			build: func(v *venv.Venv) *venv.Venv {
				return v.WithBrowser(vbrowser.NewMemOpener(func(_ context.Context, rawURL string) error {
					return fmt.Errorf("%w: %s", wantBrowserErr, rawURL)
				}))
			},
			verify: func(t *testing.T, got *venv.Venv) {
				t.Helper()

				err := got.Browser.Open(t.Context(), "https://example.com/login")
				require.ErrorIs(t, err, wantBrowserErr)
				assert.ErrorContains(t, err, "https://example.com/login")
			},
		},
		{
			name: "WithSignals",
			build: func(v *venv.Venv) *venv.Venv {
				return v.WithSignals(func(_ context.Context, notifyFn signal.NotifyFunc, _ ...os.Signal) {
					notifyFn(os.Interrupt)
				})
			},
			verify: func(t *testing.T, got *venv.Venv) {
				t.Helper()

				var received os.Signal

				got.Signals(t.Context(), func(sig os.Signal) { received = sig }, os.Interrupt)

				assert.Equal(t, os.Interrupt, received)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := &venv.Venv{}
			got := tc.build(original)

			require.NotSame(t, original, got)
			assert.Nil(t, original.FS)
			assert.Nil(t, original.Exec)
			assert.Nil(t, original.Sops)
			assert.Nil(t, original.HTTP)
			assert.Nil(t, original.Browser)
			assert.Nil(t, original.Signals)
			tc.verify(t, got)
		})
	}
}

// TestVenvWithStdinSharesOneStream pins the stdin contract: every derived venv
// reads the one stream, unwrapped, so a second consumer resumes exactly where
// the first stopped, and a later replacement swaps the stream for that copy
// alone.
func TestVenvWithStdinSharesOneStream(t *testing.T) {
	t.Parallel()

	original := &venv.Venv{}
	stdin := strings.NewReader("first\nsecond\n")
	got := original.WithStdin(stdin)

	assert.Nil(t, original.Stdin)
	require.Same(t, stdin, got.Stdin)

	derived := got.WithFS(vfs.NewMemMapFS())
	require.Same(t, got.Stdin, derived.Stdin)

	replaced := got.WithStdin(strings.NewReader("third\n"))
	require.NotSame(t, got.Stdin, replaced.Stdin)
	require.Same(t, stdin, got.Stdin)
}

// TestVenvRequireStdinAndHTTP pins the Stdin and HTTP contracts: the zero
// Venv panics with the sentinel, a populated Venv passes.
func TestVenvRequireStdinAndHTTP(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, venv.ErrVenvStdinUnset, func() {
		(&venv.Venv{FS: vfs.NewMemMapFS()}).RequireStdin()
	})

	assert.NotPanics(t, func() {
		(&venv.Venv{}).WithStdin(strings.NewReader("")).RequireStdin()
	})

	assert.PanicsWithValue(t, venv.ErrVenvHTTPUnset, func() {
		(&venv.Venv{FS: vfs.NewMemMapFS()}).RequireHTTP()
	})

	assert.NotPanics(t, func() {
		(&venv.Venv{HTTP: vhttp.NewMemClient(okHandler)}).RequireHTTP()
	})
}

// TestVenvRequireHandles checks each Require method panics when its handle is missing and passes when set.
func TestVenvRequireHandles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantPanic error
		require   func(v *venv.Venv)
		present   *venv.Venv
		name      string
		missing   []*venv.Venv
	}{
		{
			name:      "Browser",
			wantPanic: venv.ErrVenvBrowserUnset,
			require:   (*venv.Venv).RequireBrowser,
			missing:   []*venv.Venv{{}},
			present:   &venv.Venv{Browser: vbrowser.NewNoBrowserOpener()},
		},
		{
			name:      "Terminal",
			wantPanic: venv.ErrVenvTerminalUnset,
			require:   (*venv.Venv).RequireTerminal,
			missing:   []*venv.Venv{{}},
			present:   &venv.Venv{Terminal: &venv.Terminal{}},
		},
		{
			name:      "UserCacheDir",
			wantPanic: venv.ErrVenvUserCacheDirUnset,
			require:   (*venv.Venv).RequireUserCacheDir,
			missing:   []*venv.Venv{{}, {Platform: &venv.Platform{}}},
			present: &venv.Venv{Platform: &venv.Platform{
				UserCacheDir: func() (string, error) { return "/cache", nil },
			}},
		},
		{
			name:      "ReplaceEnviron",
			wantPanic: venv.ErrVenvReplaceEnvironUnset,
			require:   (*venv.Venv).RequireReplaceEnviron,
			missing:   []*venv.Venv{{}, {Platform: &venv.Platform{}}},
			present: &venv.Venv{Platform: &venv.Platform{
				ReplaceEnviron: func(map[string]string) error { return nil },
			}},
		},
		{
			name:      "TempDir",
			wantPanic: venv.ErrVenvTempDirUnset,
			require:   (*venv.Venv).RequireTempDir,
			missing:   []*venv.Venv{{}, {Platform: &venv.Platform{}}},
			present: &venv.Venv{Platform: &venv.Platform{
				TempDir: func() string { return "/scratch" },
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, v := range tc.missing {
				assert.PanicsWithValue(t, tc.wantPanic, func() { tc.require(v) })
			}

			assert.NotPanics(t, func() { tc.require(tc.present) })
		})
	}
}

func assertExecEchoes(t *testing.T, got *venv.Venv) {
	t.Helper()

	out, err := got.Exec.Command(t.Context(), "terraform", "version").Output()
	require.NoError(t, err)
	assert.Equal(t, "terraform version", string(out))
}

func okHandler(_ context.Context, _ *http.Request) (*http.Response, error) {
	return vhttp.Respond(http.StatusOK, nil, nil), nil
}

func teapotHandler(_ context.Context, _ *http.Request) (*http.Response, error) {
	return vhttp.Respond(http.StatusTeapot, nil, nil), nil
}
