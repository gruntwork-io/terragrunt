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

// Defensive test: a sibling terragrunt.autoinclude.stack.hcl (the stack-level filename) must NOT be merged into a unit's terragrunt.hcl. Stack-level autoincludes are handled by the stack parser path; merging here would defeat the point of the filename split.
func TestMergeAutoInclude_StackLevelFilenameNotMergedIntoUnit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, os.WriteFile(cfgPath, []byte(`
terraform {
  source = "."
}

inputs = {
  name = "from-unit"
}
`), 0644))

	// A sibling terragrunt.autoinclude.stack.hcl must be ignored by the unit-level merge path.
	stackAutoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeStackFile)
	require.NoError(t, os.WriteFile(stackAutoIncludePath, []byte(`
inputs = {
  name = "from-stack-autoinclude-must-not-merge"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	assert.Equal(t, "from-unit", parsed.Inputs["name"], "stack-level autoinclude must NOT be merged into the unit config")
}

// Partial parse with the experiment on must not fail when a remote_state references a dependency output whose dependency block lives only in the sibling autoinclude.
func TestPartialParseAutoIncludeRemoteStateDependencyPlaceholder(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	// Unit references a dependency output but declares no dependency block itself.
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = dependency.foo.outputs.bar
  }
}
`), 0644))

	// The dependency lives only in the sibling autoinclude.
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
dependency "foo" {
  config_path = "../foo"
  mock_outputs = {
    bar = "mocked"
  }
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx = pctx.WithDecodeList(config.DependencyBlock, config.RemoteStateBlock).WithSkipOutputsResolution()

	l := logger.CreateLogger()

	_, err := config.PartialParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
}

// Negative guard: with the experiment disabled the placeholder is not injected so the same partial parse still errors, proving the gate is not over-broad.
func TestPartialParseAutoIncludeRemoteStateDependencyExperimentDisabled(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, os.WriteFile(cfgPath, []byte(`
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = dependency.foo.outputs.bar
  }
}
`), 0644))

	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
dependency "foo" {
  config_path = "../foo"
  mock_outputs = {
    bar = "mocked"
  }
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	// NOT enabling StackDependencies experiment.
	pctx = pctx.WithDecodeList(config.DependencyBlock, config.RemoteStateBlock).WithSkipOutputsResolution()

	l := logger.CreateLogger()

	_, err := config.PartialParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.Error(t, err)
}

// A full parse must fold the autoinclude dependency block in so a remote_state config referencing its mock output resolves to the marker value.
func TestParseConfigAutoIncludeRemoteStateFoldsDependencyMockOutput(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	// A sibling target directory so the dependency config_path is valid.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "foo"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo", config.DefaultTerragruntConfigPath), []byte(``), 0644))

	const marker = "autoinclude-mock-marker"

	// Unit remote_state config references a dependency output but has no dependency block.
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = dependency.foo.outputs.val
  }
}
`), 0644))

	// The dependency lives only in the sibling autoinclude with a mock output.
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
dependency "foo" {
  config_path = "./foo"
  skip_outputs = true
  mock_outputs = {
    val = "`+marker+`"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx.OriginalTerraformCommand = "init"

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.NotNil(t, parsed.RemoteState)

	assert.Equal(t, marker, parsed.RemoteState.BackendConfig["path"])
}

// Recursion guard: parsing the autoinclude file directly must return normally instead of recursing back into the autoinclude merge.
func TestParseConfigAutoIncludeFileDirectlyDoesNotRecurse(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// A sibling target directory so the dependency config_path is valid.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "foo"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo", config.DefaultTerragruntConfigPath), []byte(``), 0644))

	// Parse the autoinclude file directly.
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
dependency "foo" {
  config_path = "./foo"
  skip_outputs = true
  mock_outputs = {
    bar = "mocked"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, autoIncludePath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx.OriginalTerraformCommand = "init"

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, autoIncludePath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)
}
