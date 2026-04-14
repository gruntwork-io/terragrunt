package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeAutoInclude_NoFile(t *testing.T) {
	t.Parallel()

	// When there is no terragrunt.autoinclude.hcl, parsing should work normally.
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, os.WriteFile(cfgPath, []byte(`
terraform {
  source = "."
}

inputs = {
  name = "original"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Original inputs should be preserved
	assert.Equal(t, "original", parsed.Inputs["name"])
}

func TestMergeAutoInclude_WithFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	// Unit config with some inputs
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
terraform {
  source = "."
}

inputs = {
  name = "from-unit"
  keep = "unit-value"
}
`), 0644))

	// Autoinclude with overlapping inputs — should win on conflicts
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
inputs = {
  name = "from-autoinclude"
  extra = "autoinclude-value"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Autoinclude wins on conflict
	assert.Equal(t, "from-autoinclude", parsed.Inputs["name"])
	// Autoinclude adds new keys
	assert.Equal(t, "autoinclude-value", parsed.Inputs["extra"])
	// Unit value preserved when no conflict
	assert.Equal(t, "unit-value", parsed.Inputs["keep"])
}

func TestMergeAutoInclude_ExperimentDisabled(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, os.WriteFile(cfgPath, []byte(`
terraform {
  source = "."
}

inputs = {
  name = "original"
}
`), 0644))

	// Autoinclude exists but experiment is NOT enabled
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
inputs = {
  name = "should-not-merge"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	// NOT enabling StackDependencies experiment

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Autoinclude should NOT be merged
	assert.Equal(t, "original", parsed.Inputs["name"])
}
