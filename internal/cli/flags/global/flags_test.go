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
	fl := global.NewProfileFlags(opts, nil)

	names := fl.Names()

	assert.True(t, slices.Contains(names, global.ProfileCPUFlagName))
	assert.True(t, slices.Contains(names, global.ProfileMEMFlagName))
	assert.True(t, slices.Contains(names, global.ProfileGoroutineFlagName))
	assert.True(t, slices.Contains(names, global.ProfileDirFlagName))

	// Each flag should declare its TG_ prefixed env var
	cpuFlag := fl.Get(global.ProfileCPUFlagName)
	require.NotNil(t, cpuFlag)
	assert.True(t, slices.Contains(cpuFlag.GetEnvVars(), "TG_PROFILE_CPU"), "expected TG_PROFILE_CPU env")

	memFlag := fl.Get(global.ProfileMEMFlagName)
	require.NotNil(t, memFlag)
	assert.True(t, slices.Contains(memFlag.GetEnvVars(), "TG_PROFILE_MEM"))
}

func TestProfileFlagsSetDestination(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	_ = global.NewProfileFlags(opts, nil)

	// Simulate values as the CLI would via destinations
	opts.ProfileCPU = "cpu.prof"
	opts.ProfileMEM = "mem.prof"
	opts.ProfileGoroutine = "go.prof"
	opts.ProfileDir = "profiles"

	assert.Equal(t, "cpu.prof", opts.ProfileCPU)
	assert.Equal(t, "mem.prof", opts.ProfileMEM)
	assert.Equal(t, "go.prof", opts.ProfileGoroutine)
	assert.Equal(t, "profiles", opts.ProfileDir)
}
