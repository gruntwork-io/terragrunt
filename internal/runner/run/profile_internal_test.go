package run

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetTofuCPUProfileEnv(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(string(filepath.Separator), "infra", "live")
	externalDir := filepath.Join(string(filepath.Separator), "infra", "modules", "vpc")
	dotPrefixedDir := filepath.Join(rootDir, "..cache", "app1")

	testCases := []struct {
		presetEnv         map[string]string
		name              string
		unitDir           string
		configPath        string
		expectedRelPath   string
		withProfileDir    bool
		expectedUntouched bool
	}{
		{
			name:              "no profile dir leaves env untouched",
			unitDir:           filepath.Join(rootDir, "app1"),
			configPath:        filepath.Join(rootDir, "app1", "terragrunt.hcl"),
			withProfileDir:    false,
			expectedUntouched: true,
		},
		{
			name:              "explicit TOFU_CPU_PROFILE is never overridden",
			unitDir:           filepath.Join(rootDir, "app1"),
			configPath:        filepath.Join(rootDir, "app1", "terragrunt.hcl"),
			presetEnv:         map[string]string{tf.EnvNameTofuCPUProfile: "custom.prof"},
			withProfileDir:    true,
			expectedUntouched: true,
		},
		{
			name:            "unit inside root gets a unit subdirectory",
			unitDir:         filepath.Join(rootDir, "app1"),
			configPath:      filepath.Join(rootDir, "app1", "terragrunt.hcl"),
			withProfileDir:  true,
			expectedRelPath: filepath.Join("app1", tofuCPUProfileName),
		},
		{
			name:            "nested unit keeps its relative layout",
			unitDir:         filepath.Join(rootDir, "prod", "app1"),
			configPath:      filepath.Join(rootDir, "prod", "app1", "terragrunt.hcl"),
			withProfileDir:  true,
			expectedRelPath: filepath.Join("prod", "app1", tofuCPUProfileName),
		},
		{
			name:            "unit at the root writes into the profile dir itself",
			unitDir:         rootDir,
			configPath:      filepath.Join(rootDir, "terragrunt.hcl"),
			withProfileDir:  true,
			expectedRelPath: tofuCPUProfileName,
		},
		{
			name:            "dot-prefixed dir name inside root is treated as local",
			unitDir:         dotPrefixedDir,
			configPath:      filepath.Join(dotPrefixedDir, "terragrunt.hcl"),
			withProfileDir:  true,
			expectedRelPath: filepath.Join("..cache", "app1", tofuCPUProfileName),
		},
		{
			name:           "external unit gets a hash-suffixed dir under external",
			unitDir:        externalDir,
			configPath:     filepath.Join(externalDir, "terragrunt.hcl"),
			withProfileDir: true,
			expectedRelPath: filepath.Join(
				"external", "vpc-"+util.EncodeBase64Sha1(externalDir), tofuCPUProfileName),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := NewOptions()
			opts.RootWorkingDir = rootDir
			opts.UnitDir = tc.unitDir
			opts.OriginalTerragruntConfigPath = tc.configPath
			opts.Env = map[string]string{}

			if tc.withProfileDir {
				opts.ProfileDir = t.TempDir()
			}

			for k, v := range tc.presetEnv {
				opts.Env[k] = v
			}

			wantEnv := maps.Clone(opts.Env)

			require.NoError(t, setTofuCPUProfileEnv(logger.CreateLogger(), opts))

			if tc.expectedUntouched {
				assert.Equal(t, wantEnv, opts.Env)

				return
			}

			expected := filepath.Join(opts.ProfileDir, tc.expectedRelPath)
			assert.Equal(t, expected, opts.Env[tf.EnvNameTofuCPUProfile])
			assert.DirExists(t, filepath.Dir(expected))
		})
	}
}

func TestSetTofuCPUProfileEnvExternalUnitsDoNotCollide(t *testing.T) {
	t.Parallel()

	profileDir := t.TempDir()
	rootDir := filepath.Join(string(filepath.Separator), "infra", "live")

	paths := make(map[string]bool)

	for _, unitDir := range []string{
		filepath.Join(string(filepath.Separator), "team-a", "vpc"),
		filepath.Join(string(filepath.Separator), "team-b", "vpc"),
	} {
		opts := NewOptions()
		opts.RootWorkingDir = rootDir
		opts.UnitDir = unitDir
		opts.OriginalTerragruntConfigPath = filepath.Join(unitDir, "terragrunt.hcl")
		opts.ProfileDir = profileDir
		opts.Env = map[string]string{}

		require.NoError(t, setTofuCPUProfileEnv(logger.CreateLogger(), opts))
		paths[opts.Env[tf.EnvNameTofuCPUProfile]] = true
	}

	assert.Len(t, paths, 2)
}

func TestSetTofuCPUProfileEnvDirCreationError(t *testing.T) {
	t.Parallel()

	profileDir := t.TempDir()
	rootDir := filepath.Join(string(filepath.Separator), "infra", "live")
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "app1"), []byte("x"), 0o600))

	opts := NewOptions()
	opts.RootWorkingDir = rootDir
	opts.UnitDir = filepath.Join(rootDir, "app1")
	opts.OriginalTerragruntConfigPath = filepath.Join(rootDir, "app1", "terragrunt.hcl")
	opts.ProfileDir = profileDir
	opts.Env = map[string]string{}

	err := setTofuCPUProfileEnv(logger.CreateLogger(), opts)
	require.ErrorContains(t, err, "could not create tofu profile directory")
	assert.NotContains(t, opts.Env, tf.EnvNameTofuCPUProfile)
}
