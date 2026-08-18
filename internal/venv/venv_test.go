package venv_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

func TestParseEnviron(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		want    map[string]string
		name    string
		environ []string
	}{
		{
			name:    "standard entries",
			environ: []string{"FOO=bar", "BAZ=qux"},
			want:    map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:    "value contains equals",
			environ: []string{"URL=https://example.com/?a=b"},
			want:    map[string]string{"URL": "https://example.com/?a=b"},
		},
		{
			name:    "empty value",
			environ: []string{"EMPTY="},
			want:    map[string]string{"EMPTY": ""},
		},
		{
			name:    "entry without separator is dropped",
			environ: []string{"NOSEP"},
			want:    map[string]string{},
		},
		{
			name:    "empty entry is dropped",
			environ: []string{"", "FOO=bar"},
			want:    map[string]string{"FOO": "bar"},
		},
		{
			name:    "windows per-drive key keeps leading equals",
			environ: []string{`=C:=C:\Users\alice`},
			want:    map[string]string{`=C:`: `C:\Users\alice`},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, venv.ParseEnviron(tc.environ))
		})
	}
}

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

// TestWithEnvRejectsNil pins the argument contract: WithEnv asserts a
// non-nil env instead of silently substituting an empty map, and
// WithEnvCloned asserts a non-nil receiver Env before cloning.
func TestWithEnvRejectsNil(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, venv.ErrVenvEnvNil, func() {
		(&venv.Venv{}).WithEnv(nil)
	})

	assert.PanicsWithValue(t, venv.ErrVenvEnvUnset, func() {
		(&venv.Venv{}).WithEnvCloned()
	})
}

func TestWithEnvClonedIsolatesMutations(t *testing.T) {
	t.Parallel()

	v := &venv.Venv{Env: map[string]string{"FOO": "bar"}}

	clone := v.WithEnvCloned()
	clone.Env["AWS_ACCESS_KEY_ID"] = "leaked"
	clone.Env["FOO"] = "changed"

	assert.Equal(t, map[string]string{"FOO": "bar"}, v.Env)

	v.Env["BAZ"] = "qux"

	assert.NotContains(t, clone.Env, "BAZ")
}

func TestOSVenvProvidesPlatformHandles(t *testing.T) {
	t.Parallel()

	v := venv.OSVenv()

	require.NotNil(t, v.Platform)
	assert.Equal(t, runtime.GOOS, v.Platform.GOOS)
	assert.Equal(t, runtime.GOARCH, v.Platform.GOARCH)
	assert.NotNil(t, v.Platform.UserHomeDir)
}

func TestVenvPlatformBuilders(t *testing.T) {
	t.Parallel()

	wantHomeErr := errors.New("home lookup failed")
	homeDir := func() (string, error) { return "", wantHomeErr }
	original := venv.OSVenv()

	got := original.WithGOOS("plan9").WithGOARCH("mips").WithUserHomeDir(homeDir)

	require.NotNil(t, got.Platform)
	assert.Equal(t, "plan9", got.Platform.GOOS)
	assert.Equal(t, "mips", got.Platform.GOARCH)
	_, err := got.Platform.UserHomeDir()
	require.ErrorIs(t, err, wantHomeErr)
	assert.Equal(t, runtime.GOOS, original.Platform.GOOS)
	assert.Equal(t, runtime.GOARCH, original.Platform.GOARCH)
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
	assert.PanicsWithValue(t, venv.ErrVenvPlatformUnset, func() {
		(&venv.Venv{}).RequirePlatform()
	})
	assert.PanicsWithValue(t, venv.ErrVenvWritersUnset, func() {
		(&venv.Venv{}).RequireWriters()
	})
	assert.PanicsWithValue(t, venv.ErrVenvListenUnset, func() {
		(&venv.Venv{}).RequireListen()
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

// TestVenvPlatformBuildersRequireAPlatform pins that refining one platform
// handle on a Venv carrying no platform fails at the builder. Filling in a
// blank platform instead would hand back a Venv whose other handles are nil,
// and the nil would surface somewhere else entirely.
func TestVenvPlatformBuildersRequireAPlatform(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, venv.ErrVenvPlatformUnset, func() {
		(&venv.Venv{}).WithGOOS("plan9")
	})
	assert.PanicsWithValue(t, venv.ErrVenvPlatformUnset, func() {
		(&venv.Venv{}).WithGOARCH("mips")
	})
	assert.PanicsWithValue(t, venv.ErrVenvPlatformUnset, func() {
		(&venv.Venv{}).WithUserHomeDir(func() (string, error) { return "", nil })
	})
	assert.PanicsWithValue(t, venv.ErrVenvPlatformUnset, func() {
		(&venv.Venv{}).WithTempDir(func() string { return "" })
	})
}

// TestVenvHandleBuildersReturnCopies pins the builder contract: the returned
// copy carries the new handle, the receiver keeps none of them.
func TestVenvHandleBuildersReturnCopies(t *testing.T) {
	t.Parallel()

	memFS := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(memFS, "/tracer.txt", []byte("tracer"), 0o644))

	wantSopsErr := errors.New("decrypt failed")
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

func assertExecEchoes(t *testing.T, got *venv.Venv) {
	t.Helper()

	out, err := got.Exec.Command(t.Context(), "terraform", "version").Output()
	require.NoError(t, err)
	assert.Equal(t, "terraform version", string(out))
}

func okHandler(_ context.Context, _ *http.Request) (*http.Response, error) {
	return vhttp.Respond(http.StatusOK, nil, nil), nil
}
