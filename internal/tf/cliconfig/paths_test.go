package cliconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The virtual locations the injected platform reports.
const (
	testHome    = "/virtual/home"
	testAppData = "/virtual/appdata"
	testXDG     = "/virtual/config"
)

// errStatFailed stands in for a filesystem that cannot answer whether a path exists.
var errStatFailed = errors.New("stat failed")

// statErrorFS fails Stat for one exact path and delegates every other call.
type statErrorFS struct {
	vfs.FS
	failPath string
}

func (fsys statErrorFS) Stat(name string) (os.FileInfo, error) {
	if name == fsys.failPath {
		return nil, &os.PathError{Op: "stat", Path: name, Err: errStatFailed}
	}

	return fsys.FS.Stat(name)
}

// pathsVenv builds an in-memory venv reporting goos, home, and env.
func pathsVenv(goos, home string, env map[string]string) *venv.Venv {
	if env == nil {
		env = map[string]string{}
	}

	return venvtest.New().
		WithGOOS(goos).
		WithUserHomeDir(func() (string, error) { return home, nil }).
		WithEnv(env)
}

// TestUserConfigOverride: the env vars are read in OpenTofu's order, and neither being set is not an override.
func TestUserConfigOverride(t *testing.T) {
	t.Parallel()

	tc := []struct {
		env      map[string]string
		name     string
		wantPath string
		wantEnv  string
	}{
		{name: "unset"},
		{
			name:     "TF_CLI_CONFIG_FILE",
			env:      map[string]string{cliconfig.EnvNameTFCLIConfigFile: "/virtual/custom.tofurc"},
			wantPath: "/virtual/custom.tofurc",
			wantEnv:  cliconfig.EnvNameTFCLIConfigFile,
		},
		{
			name:     "TERRAFORM_CONFIG",
			env:      map[string]string{cliconfig.EnvNameTerraformConfig: "/virtual/terraform.rc"},
			wantPath: "/virtual/terraform.rc",
			wantEnv:  cliconfig.EnvNameTerraformConfig,
		},
		{
			name: "TF_CLI_CONFIG_FILE wins",
			env: map[string]string{
				cliconfig.EnvNameTFCLIConfigFile: "/virtual/custom.tofurc",
				cliconfig.EnvNameTerraformConfig: "/virtual/terraform.rc",
			},
			wantPath: "/virtual/custom.tofurc",
			wantEnv:  cliconfig.EnvNameTFCLIConfigFile,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path, envName := cliconfig.UserConfigOverride(pathsVenv("linux", testHome, tt.env))
			assert.Equal(t, tt.wantPath, path)
			assert.Equal(t, tt.wantEnv, envName)
		})
	}
}

// TestUserConfigCandidates: the default file locations follow OpenTofu's platform-specific search order.
func TestUserConfigCandidates(t *testing.T) {
	t.Parallel()

	tc := []struct {
		env  map[string]string
		name string
		goos string
		home string
		want []string
	}{
		{
			name: "unix dotfiles",
			goos: "linux",
			home: testHome,
			want: []string{filepath.Join(testHome, ".tofurc"), filepath.Join(testHome, ".terraformrc")},
		},
		{
			name: "unix XDG appended when set",
			goos: "linux",
			home: testHome,
			env:  map[string]string{"XDG_CONFIG_HOME": testXDG},
			want: []string{
				filepath.Join(testHome, ".tofurc"),
				filepath.Join(testHome, ".terraformrc"),
				filepath.Join(testXDG, "opentofu", "tofurc"),
			},
		},
		{
			name: "unix without a home reads XDG only",
			goos: "linux",
			env:  map[string]string{"XDG_CONFIG_HOME": testXDG},
			want: []string{filepath.Join(testXDG, "opentofu", "tofurc")},
		},
		{
			name: "unix without a home or XDG has no candidate",
			goos: "linux",
		},
		{
			name: "windows reads APPDATA only",
			goos: "windows",
			home: testHome,
			env:  map[string]string{"APPDATA": testAppData, "XDG_CONFIG_HOME": testXDG},
			want: []string{filepath.Join(testAppData, "tofu.rc"), filepath.Join(testAppData, "terraform.rc")},
		},
		{
			name: "windows without APPDATA has no candidate",
			goos: "windows",
			home: testHome,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cliconfig.UserConfigCandidates(pathsVenv(tt.goos, tt.home, tt.env)))
		})
	}
}

// TestUserConfigDir: the config directory prefers the legacy directory, then XDG, then the legacy default.
func TestUserConfigDir(t *testing.T) {
	t.Parallel()

	tc := []struct {
		env         map[string]string
		name        string
		goos        string
		home        string
		existingDir string
		want        string
	}{
		{
			name:        "existing legacy directory wins",
			goos:        "linux",
			home:        testHome,
			env:         map[string]string{"XDG_CONFIG_HOME": testXDG},
			existingDir: filepath.Join(testHome, ".terraform.d"),
			want:        filepath.Join(testHome, ".terraform.d"),
		},
		{
			name: "absent legacy directory falls back to XDG",
			goos: "linux",
			home: testHome,
			env:  map[string]string{"XDG_CONFIG_HOME": testXDG},
			want: filepath.Join(testXDG, "opentofu"),
		},
		{
			name: "no XDG falls back to the legacy default",
			goos: "linux",
			home: testHome,
			want: filepath.Join(testHome, ".terraform.d"),
		},
		{
			name: "no home and no XDG has no directory",
			goos: "linux",
		},
		{
			name: "windows reads APPDATA",
			goos: "windows",
			home: testHome,
			env:  map[string]string{"APPDATA": testAppData},
			want: filepath.Join(testAppData, "terraform.d"),
		},
		{
			name: "windows without APPDATA has no directory",
			goos: "windows",
			home: testHome,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := pathsVenv(tt.goos, tt.home, tt.env)
			if tt.existingDir != "" {
				require.NoError(t, v.FS.MkdirAll(tt.existingDir, 0o755))
			}

			dir, err := cliconfig.UserConfigDir(v)
			require.NoError(t, err)
			assert.Equal(t, tt.want, dir)
		})
	}
}

// TestUserConfigDirStatFailure: a directory whose presence cannot be determined is an error, not a fallback.
func TestUserConfigDirStatFailure(t *testing.T) {
	t.Parallel()

	legacy := filepath.Join(testHome, ".terraform.d")
	v := pathsVenv("linux", testHome, map[string]string{"XDG_CONFIG_HOME": testXDG})
	v = v.WithFS(statErrorFS{FS: v.FS, failPath: legacy})

	_, err := cliconfig.UserConfigDir(v)
	require.ErrorIs(t, err, errStatFailed, "an unreadable config directory must not silently fall through to XDG")
}
