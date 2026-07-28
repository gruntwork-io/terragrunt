package run_test

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
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
		userSet           bool
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
			userSet:           true,
			expectedUntouched: true,
		},
		{
			name:            "unit inside root gets a unit subdirectory",
			unitDir:         filepath.Join(rootDir, "app1"),
			configPath:      filepath.Join(rootDir, "app1", "terragrunt.hcl"),
			withProfileDir:  true,
			expectedRelPath: filepath.Join("app1", "tofu_cpu.prof"),
		},
		{
			name:            "nested unit keeps its relative layout",
			unitDir:         filepath.Join(rootDir, "prod", "app1"),
			configPath:      filepath.Join(rootDir, "prod", "app1", "terragrunt.hcl"),
			withProfileDir:  true,
			expectedRelPath: filepath.Join("prod", "app1", "tofu_cpu.prof"),
		},
		{
			name:            "unit at the root writes into the profile dir itself",
			unitDir:         rootDir,
			configPath:      filepath.Join(rootDir, "terragrunt.hcl"),
			withProfileDir:  true,
			expectedRelPath: "tofu_cpu.prof",
		},
		{
			name:            "dot-prefixed dir name inside root is treated as local",
			unitDir:         dotPrefixedDir,
			configPath:      filepath.Join(dotPrefixedDir, "terragrunt.hcl"),
			withProfileDir:  true,
			expectedRelPath: filepath.Join("..cache", "app1", "tofu_cpu.prof"),
		},
		{
			name:           "external unit gets a hash-suffixed dir under external",
			unitDir:        externalDir,
			configPath:     filepath.Join(externalDir, "terragrunt.hcl"),
			withProfileDir: true,
			expectedRelPath: filepath.Join(
				"external", "vpc-"+util.EncodeBase64Sha1(externalDir), "tofu_cpu.prof"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New()
			maps.Copy(v.Env, tc.presetEnv)

			opts := run.NewOptions()
			opts.RootWorkingDir = rootDir
			opts.UnitDir = tc.unitDir
			opts.OriginalTerragruntConfigPath = tc.configPath
			opts.TofuCPUProfileUserSet = tc.userSet

			if tc.withProfileDir {
				opts.ProfileDir = t.TempDir()
			}

			wantEnv := maps.Clone(v.Env)

			require.NoError(t, run.SetTofuCPUProfileEnv(logger.CreateLogger(), v, opts))

			if tc.expectedUntouched {
				assert.Equal(t, wantEnv, v.Env)

				return
			}

			expected := filepath.Join(opts.ProfileDir, tc.expectedRelPath)
			assert.Equal(t, expected, v.Env[tf.EnvNameTofuCPUProfile])

			info, err := v.FS.Stat(filepath.Dir(expected))
			require.NoError(t, err, "the tofu profile directory should be created")
			assert.True(t, info.IsDir())
		})
	}
}

func TestSetTofuCPUProfileEnvAutoInitGetsOwnProfile(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(string(filepath.Separator), "infra", "live")

	v := venvtest.New()

	opts := run.NewOptions()
	opts.RootWorkingDir = rootDir
	opts.UnitDir = filepath.Join(rootDir, "app1")
	opts.OriginalTerragruntConfigPath = filepath.Join(rootDir, "app1", "terragrunt.hcl")
	opts.ProfileDir = t.TempDir()
	opts.TerraformCommand = "plan"

	require.NoError(t, run.SetTofuCPUProfileEnv(logger.CreateLogger(), v, opts))
	planPath := v.Env[tf.EnvNameTofuCPUProfile]
	assert.Equal(t, "tofu_cpu_plan.prof", filepath.Base(planPath))

	opts.TerraformCommand = "init"

	require.NoError(t, run.SetTofuCPUProfileEnv(logger.CreateLogger(), v, opts))
	initPath := v.Env[tf.EnvNameTofuCPUProfile]
	assert.Equal(t, "tofu_cpu_init.prof", filepath.Base(initPath))
	assert.NotEqual(t, planPath, initPath, "auto-init and the main command must not share a profile file")
}

func TestSetTofuCPUProfileEnvUserPathInsideProfileDirRespected(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join(string(filepath.Separator), "infra", "live")

	v := venvtest.New()

	opts := run.NewOptions()
	opts.RootWorkingDir = rootDir
	opts.UnitDir = filepath.Join(rootDir, "app1")
	opts.OriginalTerragruntConfigPath = filepath.Join(rootDir, "app1", "terragrunt.hcl")
	opts.ProfileDir = t.TempDir()
	opts.TofuCPUProfileUserSet = true

	customPath := filepath.Join(opts.ProfileDir, "custom.prof")
	v.Env[tf.EnvNameTofuCPUProfile] = customPath

	require.NoError(t, run.SetTofuCPUProfileEnv(logger.CreateLogger(), v, opts))
	assert.Equal(t, customPath, v.Env[tf.EnvNameTofuCPUProfile],
		"a user-set TOFU_CPU_PROFILE must be respected even when it points inside the profile dir")
}

func TestSetTofuCPUProfileEnvExternalUnitsDoNotCollide(t *testing.T) {
	t.Parallel()

	profileDir := t.TempDir()
	rootDir := filepath.Join(string(filepath.Separator), "infra", "live")

	paths := make(map[string]struct{})

	for _, unitDir := range []string{
		filepath.Join(string(filepath.Separator), "team-a", "vpc"),
		filepath.Join(string(filepath.Separator), "team-b", "vpc"),
	} {
		v := venvtest.New()

		opts := run.NewOptions()
		opts.RootWorkingDir = rootDir
		opts.UnitDir = unitDir
		opts.OriginalTerragruntConfigPath = filepath.Join(unitDir, "terragrunt.hcl")
		opts.ProfileDir = profileDir

		require.NoError(t, run.SetTofuCPUProfileEnv(logger.CreateLogger(), v, opts))
		paths[v.Env[tf.EnvNameTofuCPUProfile]] = struct{}{}
	}

	assert.Len(t, paths, 2)
}

func TestSetTofuCPUProfileEnvDirCreationError(t *testing.T) {
	t.Parallel()

	profileDir := t.TempDir()
	rootDir := filepath.Join(string(filepath.Separator), "infra", "live")

	v := venvtest.New().WithFS(vfs.NewOSFS())
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(profileDir, "app1"), []byte("x"), 0o600))

	opts := run.NewOptions()
	opts.RootWorkingDir = rootDir
	opts.UnitDir = filepath.Join(rootDir, "app1")
	opts.OriginalTerragruntConfigPath = filepath.Join(rootDir, "app1", "terragrunt.hcl")
	opts.ProfileDir = profileDir

	err := run.SetTofuCPUProfileEnv(logger.CreateLogger(), v, opts)
	require.ErrorContains(t, err, "could not create tofu profile directory")
	assert.NotContains(t, v.Env, tf.EnvNameTofuCPUProfile)
}
