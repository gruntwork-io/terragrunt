package venv_test

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
)

func TestOSVenvProvidesPlatformHandles(t *testing.T) {
	t.Parallel()

	v := venv.OSVenv()

	require.NotNil(t, v.Platform)
	assert.Equal(t, runtime.GOOS, v.Platform.GOOS)
	assert.Equal(t, runtime.GOARCH, v.Platform.GOARCH)
	assert.NotNil(t, v.Platform.UserHomeDir)
	assert.NotNil(t, v.Platform.UserConfigDir)
}

func TestVenvPlatformBuilders(t *testing.T) {
	t.Parallel()

	wantHomeErr := errors.New("home lookup failed")
	homeDir := func() (string, error) { return "", wantHomeErr }
	configDir := func() (string, error) { return "/tmp/config", nil }
	original := venv.OSVenv()

	got := original.WithGOOS("plan9").WithGOARCH("mips").WithUserHomeDir(homeDir).WithUserConfigDir(configDir)

	require.NotNil(t, got.Platform)
	assert.Equal(t, "plan9", got.Platform.GOOS)
	assert.Equal(t, "mips", got.Platform.GOARCH)
	_, err := got.Platform.UserHomeDir()
	require.ErrorIs(t, err, wantHomeErr)
	gotConfigDir, err := got.Platform.UserConfigDir()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/config", gotConfigDir)
	assert.Equal(t, runtime.GOOS, original.Platform.GOOS)
	assert.Equal(t, runtime.GOARCH, original.Platform.GOARCH)
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
		(&venv.Venv{}).WithUserConfigDir(func() (string, error) { return "", nil })
	})
	assert.PanicsWithValue(t, venv.ErrVenvPlatformUnset, func() {
		(&venv.Venv{}).WithTempDir(func() string { return "" })
	})
	assert.PanicsWithValue(t, venv.ErrVenvPlatformUnset, func() {
		(&venv.Venv{}).WithUserCacheDir(func() (string, error) { return "", nil })
	})
	assert.PanicsWithValue(t, venv.ErrVenvPlatformUnset, func() {
		(&venv.Venv{}).WithReplaceEnviron(func(map[string]string) error { return nil })
	})
}

// TestVenvCacheTempAndEnvironBuilders checks each builder sets its handle on a copy, not the original.
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

// TestOSVenvReplaceEnviron replaces the process environment, so it is not parallel and restores it on cleanup.
func TestOSVenvReplaceEnviron(t *testing.T) {
	replace := venv.OSVenv().Platform.ReplaceEnviron
	original := venv.ParseEnviron(os.Environ())

	t.Cleanup(func() {
		// Windows manages the per-drive "=C:" variables itself, so they are not restored.
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
