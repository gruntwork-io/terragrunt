package global_test

import (
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileFlagsRegistration(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	profileFlags := global.NewProfileFlags(opts, nil)

	testCases := []struct {
		flagName string
		envName  string
	}{
		{flagName: global.ProfileCPUFlagName, envName: "TG_PROFILE_CPU"},
		{flagName: global.ProfileMemFlagName, envName: "TG_PROFILE_MEM"},
		{flagName: global.ProfileGoroutineFlagName, envName: "TG_PROFILE_GOROUTINE"},
		{flagName: global.ProfileDirFlagName, envName: "TG_PROFILE_DIR"},
	}

	for _, tc := range testCases {
		flag := profileFlags.Get(tc.flagName)
		require.NotNil(t, flag, "flag %s must be registered", tc.flagName)
		assert.True(t, slices.Contains(flag.GetEnvVars(), tc.envName), "flag %s must declare the %s env var", tc.flagName, tc.envName)
	}
}

func TestProfileFlagsParse(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()

	err := global.NewProfileFlags(opts, nil).Parse([]string{
		"--profile-cpu=cpu.prof",
		"--profile-mem=mem.prof",
		"--profile-goroutine=goroutine.prof",
		"--profile-dir=profiles",
	})
	require.NoError(t, err)

	assert.Equal(t, "cpu.prof", opts.ProfileCPU)
	assert.Equal(t, "mem.prof", opts.ProfileMem)
	assert.Equal(t, "goroutine.prof", opts.ProfileGoroutine)
	assert.Equal(t, "profiles", opts.ProfileDir)
}

func TestProfileFlagsParseSpaceForm(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()

	err := global.NewProfileFlags(opts, nil).Parse([]string{"--profile-cpu", "cpu.prof"})
	require.NoError(t, err)

	assert.Equal(t, "cpu.prof", opts.ProfileCPU)
}
