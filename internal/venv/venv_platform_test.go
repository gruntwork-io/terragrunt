package venv_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// TestVenvCacheTempAndEnvironBuilders pins the builder contract for the
// platform handles the other builder tests leave out: the copy carries the
// new handle, the rest of the platform is kept, and the receiver's platform
// is untouched.
func TestVenvCacheTempAndEnvironBuilders(t *testing.T) {
	t.Parallel()

	wantReplaceErr := errors.New("replace failed")
	original := &venv.Venv{Platform: &venv.Platform{GOOS: "plan9", GOARCH: "mips"}}

	got := original.
		WithUserCacheDir(func() (string, error) { return "/cache", nil }).
		WithTempDir(func() string { return "/scratch" }).
		WithReplaceEnviron(func(map[string]string) error { return wantReplaceErr })

	require.NotNil(t, got.Platform)
	require.NotSame(t, original.Platform, got.Platform)

	cacheDir, err := got.Platform.UserCacheDir()
	require.NoError(t, err)
	assert.Equal(t, "/cache", cacheDir)
	assert.Equal(t, "/scratch", got.Platform.TempDir())
	require.ErrorIs(t, got.Platform.ReplaceEnviron(map[string]string{}), wantReplaceErr)
	assert.Equal(t, "plan9", got.Platform.GOOS)
	assert.Equal(t, "mips", got.Platform.GOARCH)

	assert.Nil(t, original.Platform.UserCacheDir)
	assert.Nil(t, original.Platform.TempDir)
	assert.Nil(t, original.Platform.ReplaceEnviron)
}

// TestVenvRequireHandles pins the Require contracts not covered elsewhere: a
// missing handle panics with its sentinel, including a platform that is set
// but lacks the one handle asked for, and a populated handle passes.
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

// TestEnviron pins that Environ renders KEY=VALUE entries in sorted order, so
// the result does not vary with map iteration, and that it inverts
// ParseEnviron.
func TestEnviron(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		env  map[string]string
		name string
		want []string
	}{
		{
			name: "empty map",
			env:  map[string]string{},
			want: []string{},
		},
		{
			name: "entries are sorted",
			env:  map[string]string{"ZED": "last", "ALPHA": "first", "MID": "middle"},
			want: []string{"ALPHA=first", "MID=middle", "ZED=last"},
		},
		{
			name: "value with separator and empty value",
			env:  map[string]string{"URL": "https://example.com/?a=b", "EMPTY": ""},
			want: []string{"EMPTY=", "URL=https://example.com/?a=b"},
		},
		{
			name: "windows per-drive key",
			env:  map[string]string{`=C:`: `C:\Users\alice`, "PATH": `C:\bin`},
			want: []string{`=C:=C:\Users\alice`, `PATH=C:\bin`},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := venv.Environ(tc.env)

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.env, venv.ParseEnviron(got))
		})
	}
}

// TestOSVenvReplaceEnviron pins the production ReplaceEnviron: it clears the
// process environment before setting the new one, and reports a variable the
// OS refuses. It replaces the real process environment, so it runs alone and
// puts the original environment back when it is done.
func TestOSVenvReplaceEnviron(t *testing.T) {
	replace := venv.OSVenv().Platform.ReplaceEnviron
	original := venv.ParseEnviron(os.Environ())

	t.Cleanup(func() {
		// The Windows per-drive working-directory variables ("=C:") are
		// maintained by the OS, not set through the environment API.
		restore := make(map[string]string, len(original))

		for name, value := range original {
			if !strings.HasPrefix(name, "=") {
				restore[name] = value
			}
		}

		assert.NoError(t, replace(restore))
	})

	t.Setenv("TG_VENV_TEST_STALE", "stale")

	want := map[string]string{
		"TG_VENV_TEST_A": "one",
		"TG_VENV_TEST_B": "two=three",
	}

	require.NoError(t, replace(want))
	assert.Equal(t, want, venv.ParseEnviron(os.Environ()))

	err := replace(map[string]string{"TG_VENV_TEST\x00BAD": "value"})
	require.Error(t, err)
	assert.ErrorContains(t, err, "in the process environment")
}

// TestOSVenvTerminalWithoutTerminal pins the production Terminal handles when
// no stream is a terminal, as in a pipe, a file, or a CI log: every probe
// reports false and the width is 0. It swaps the process-wide standard
// streams, so it runs alone.
//
//nolint:paralleltest // swaps the process-wide os.Stdin, os.Stdout and os.Stderr
func TestOSVenvTerminalWithoutTerminal(t *testing.T) {
	file := createStreamFile(t)
	swapStdStreams(t, file, file, file)

	terminal := venv.OSVenv().Terminal

	require.NotNil(t, terminal)
	assert.False(t, terminal.StdinIsTTY())
	assert.False(t, terminal.StdoutIsTTY())
	assert.False(t, terminal.StderrIsTTY())
	assert.Zero(t, terminal.Width())
}

// createStreamFile returns a regular file, which no terminal probe accepts,
// closed when the test ends.
func createStreamFile(t *testing.T) *os.File {
	t.Helper()

	file, err := os.Create(filepath.Join(t.TempDir(), "stream"))
	require.NoError(t, err)

	t.Cleanup(func() { file.Close() })

	return file
}

// swapStdStreams points os.Stdin, os.Stdout and os.Stderr at the given files
// until the test ends, then restores the originals.
func swapStdStreams(t *testing.T, stdin, stdout, stderr *os.File) {
	t.Helper()

	origStdin, origStdout, origStderr := os.Stdin, os.Stdout, os.Stderr

	t.Cleanup(func() {
		os.Stdin, os.Stdout, os.Stderr = origStdin, origStdout, origStderr
	})

	os.Stdin, os.Stdout, os.Stderr = stdin, stdout, stderr
}
