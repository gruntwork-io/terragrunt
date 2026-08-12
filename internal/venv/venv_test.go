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
	assert.NotNil(t, v.Platform.UserHomeDir)
}

func TestVenvPlatformBuilders(t *testing.T) {
	t.Parallel()

	wantHomeErr := errors.New("home lookup failed")
	homeDir := func() (string, error) { return "", wantHomeErr }
	original := venv.OSVenv()

	got := original.WithGOOS("plan9").WithUserHomeDir(homeDir)

	require.NotNil(t, got.Platform)
	assert.Equal(t, "plan9", got.Platform.GOOS)
	_, err := got.Platform.UserHomeDir()
	require.ErrorIs(t, err, wantHomeErr)
	assert.Equal(t, runtime.GOOS, original.Platform.GOOS)
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
	assert.PanicsWithValue(t, venv.ErrVenvGOOSUnset, func() {
		(&venv.Venv{}).RequireGOOS()
	})
	assert.PanicsWithValue(t, venv.ErrVenvUserHomeDirUnset, func() {
		(&venv.Venv{}).RequireUserHomeDir()
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
				return v.WithSops(vsops.NewMemDecrypter(func(string, string) ([]byte, error) {
					return nil, wantSopsErr
				}))
			},
			verify: func(t *testing.T, got *venv.Venv) {
				t.Helper()

				_, err := got.Sops.DecryptFile("/secrets.yaml", "yaml")
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

// TestVenvWithReaderBuffersOneStream pins the reader contract: the copy reads
// from r through a buffer every derived venv shares, so a second prompt resumes
// where the first stopped, and a later replacement leaves that buffer intact.
func TestVenvWithReaderBuffersOneStream(t *testing.T) {
	t.Parallel()

	original := &venv.Venv{}
	got := original.WithReader(strings.NewReader("first\nsecond\n"))

	assert.Nil(t, original.Reader)
	require.NotNil(t, got.Reader)

	line, err := got.Reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "first\n", line)
	assert.Positive(t, got.Reader.Buffered())

	derived := got.WithFS(vfs.NewMemMapFS())
	require.Same(t, got.Reader, derived.Reader)

	line, err = derived.Reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "second\n", line)

	replaced := got.WithReader(strings.NewReader("third\n"))
	require.NotSame(t, got.Reader, replaced.Reader)

	line, err = replaced.Reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "third\n", line)
}

// TestVenvRequireReaderAndHTTP pins the Reader and HTTP contracts: the zero
// Venv panics with the sentinel, a populated Venv passes.
func TestVenvRequireReaderAndHTTP(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, venv.ErrVenvReaderUnset, func() {
		(&venv.Venv{FS: vfs.NewMemMapFS()}).RequireReader()
	})

	assert.NotPanics(t, func() {
		(&venv.Venv{}).WithReader(strings.NewReader("")).RequireReader()
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
