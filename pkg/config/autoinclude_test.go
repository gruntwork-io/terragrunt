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

// A malformed sibling autoinclude makes the merge helper return an error; ParseConfigFile must surface that error without nil-panicking in handleInclude.
func TestMergeAutoInclude_MalformedSiblingDoesNotPanic(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	// The parent lives in a subdirectory so the sibling autoinclude does not also apply to it.
	parentDir := filepath.Join(tmpDir, "parent")
	require.NoError(t, os.MkdirAll(parentDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(parentDir, "root.hcl"), []byte(`
inputs = {
  parent = "from-root"
}
`), 0644))

	// A unit with a resolvable include so TrackInclude is set and the post-merge handleInclude branch dereferences config on a shallow merge.
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
include "root" {
  path           = "`+filepath.Join(parentDir, "root.hcl")+`"
  merge_strategy = "shallow"
}

inputs = {
  name = "from-unit"
}
`), 0644))

	// An autoinclude that references an undefined local so its parse fails and mergeAutoIncludeDeepIfPresent returns (nil, err).
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
inputs = {
  broken = local.does_not_exist
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	l := logger.CreateLogger()

	// The call must return the parse error, never panic on a nil config.
	require.NotPanics(t, func() {
		_, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
		require.Error(t, err, "a malformed sibling autoinclude must surface as an error")
	})
}

// The autoinclude dependency block must resolve its config_path against the autoinclude's OWN locals.
// The unit declares no such local, so the unit-locals-only decode would fail to resolve local.target.
func TestFoldSiblingAutoIncludeDeps_UsesAutoIncludeOwnLocals(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	// The valid dependency target named by the autoinclude's own local.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "foo"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo", config.DefaultTerragruntConfigPath), []byte(``), 0644))

	const marker = "autoinclude-local-marker"

	// The unit defines NO target local and reads a dependency output the autoinclude wires in.
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

	// The autoinclude declares its own local that names the real target dir and feeds config_path.
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
locals {
  target = "./foo"
}

dependency "foo" {
  config_path  = local.target
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
	require.NoError(t, err, "the autoinclude dependency must resolve config_path against the autoinclude's own locals")
	require.NotNil(t, parsed)
	require.NotNil(t, parsed.RemoteState)

	// Resolution of the mock output proves the autoinclude dependency decoded with its own local.target.
	assert.Equal(t, marker, parsed.RemoteState.BackendConfig["path"])
}

// The autoinclude exclude block must win over the unit's exclude in BOTH full and partial parse.
func TestMergeAutoInclude_ExcludeAutoIncludeWinsBothParsePaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	// The unit declares an exclude with one action set.
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
exclude {
  if      = true
  actions = ["plan"]
}
`), 0644))

	// The autoinclude declares a different exclude that must win.
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
exclude {
  if      = true
  actions = ["apply"]
}
`), 0644))

	l := logger.CreateLogger()

	ctxFull, pctxFull := newTestParsingContext(t, cfgPath)
	pctxFull.Experiments.EnableExperiment(experiment.StackDependencies)

	parsedFull, err := config.ParseConfigFile(ctxFull, pctxFull, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsedFull)
	require.NotNil(t, parsedFull.Exclude)
	assert.Equal(t, []string{"apply"}, parsedFull.Exclude.Actions, "autoinclude exclude must win in full parse")

	ctxPartial, pctxPartial := newTestParsingContext(t, cfgPath)
	pctxPartial.Experiments.EnableExperiment(experiment.StackDependencies)
	pctxPartial = pctxPartial.WithDecodeList(config.ExcludeBlock).WithSkipOutputsResolution()

	parsedPartial, err := config.PartialParseConfigFile(ctxPartial, pctxPartial, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsedPartial)
	require.NotNil(t, parsedPartial.Exclude)
	assert.Equal(t, []string{"apply"}, parsedPartial.Exclude.Actions, "autoinclude exclude must win in partial parse, matching full parse")
}

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

// Full parse must deep-merge nested input maps from the unit and the autoinclude, keeping keys from both sides.
func TestMergeAutoIncludeDeepInputMerge(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, os.WriteFile(cfgPath, []byte(`
inputs = {
  tags = {
    a = "1"
  }
}
`), 0644))

	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
inputs = {
  tags = {
    b = "2"
  }
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	tags, ok := parsed.Inputs["tags"].(map[string]any)
	require.True(t, ok, "tags input should be a map")
	assert.Equal(t, "1", tags["a"], "unit nested key must survive deep merge")
	assert.Equal(t, "2", tags["b"], "autoinclude nested key must survive deep merge")
}

// The autoinclude scalar must win over the unit scalar for the same key in both full and partial parse.
func TestMergeAutoIncludeDirectionAutoIncludeWins(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, os.WriteFile(cfgPath, []byte(`
terraform {
  source = "from-unit"
}

inputs = {
  shared = "from-unit"
}
`), 0644))

	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
terraform {
  source = "from-autoinclude"
}

inputs = {
  shared = "from-autoinclude"
}
`), 0644))

	l := logger.CreateLogger()

	ctxFull, pctxFull := newTestParsingContext(t, cfgPath)
	pctxFull.Experiments.EnableExperiment(experiment.StackDependencies)

	parsedFull, err := config.ParseConfigFile(ctxFull, pctxFull, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsedFull)
	assert.Equal(t, "from-autoinclude", parsedFull.Inputs["shared"], "autoinclude scalar must win in full parse")
	require.NotNil(t, parsedFull.Terraform)
	require.NotNil(t, parsedFull.Terraform.Source)
	assert.Equal(t, "from-autoinclude", *parsedFull.Terraform.Source, "autoinclude terraform source must win in full parse")

	ctxPartial, pctxPartial := newTestParsingContext(t, cfgPath)
	pctxPartial.Experiments.EnableExperiment(experiment.StackDependencies)
	pctxPartial = pctxPartial.WithDecodeList(config.TerraformSource).WithSkipOutputsResolution()

	parsedPartial, err := config.PartialParseConfigFile(ctxPartial, pctxPartial, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsedPartial)
	require.NotNil(t, parsedPartial.Terraform)
	require.NotNil(t, parsedPartial.Terraform.Source)
	assert.Equal(t, "from-autoinclude", *parsedPartial.Terraform.Source, "autoinclude terraform source must win in partial parse")
}

// Partial parse must surface remote_state, feature, and errors blocks declared only in the autoinclude when those sections are in the decode list.
func TestPartialParseAutoIncludeEffectiveMerge(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	// Unit declares none of the blocks under test.
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
inputs = {
  name = "from-unit"
}
`), 0644))

	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = "from-autoinclude"
  }
}

feature "from_autoinclude" {
  default = true
}

errors {
  retry "from_autoinclude" {
    retryable_errors = [".*transient.*"]
    max_attempts       = 2
    sleep_interval_sec = 1
  }
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx = pctx.WithDecodeList(
		config.DependencyBlock,
		config.RemoteStateBlock,
		config.FeatureFlagsBlock,
		config.ErrorsBlock,
	).WithSkipOutputsResolution()

	l := logger.CreateLogger()

	parsed, err := config.PartialParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	require.NotNil(t, parsed.RemoteState, "autoinclude remote_state must appear in partial parse")
	assert.Equal(t, "from-autoinclude", parsed.RemoteState.BackendConfig["path"])

	require.NotNil(t, parsed.FeatureFlags, "autoinclude feature block must appear in partial parse")

	foundFeature := false

	for _, ff := range parsed.FeatureFlags {
		if ff.Name == "from_autoinclude" {
			foundFeature = true
		}
	}

	assert.True(t, foundFeature, "autoinclude feature must be present")

	require.NotNil(t, parsed.Errors, "autoinclude errors block must appear in partial parse")
	require.Len(t, parsed.Errors.Retry, 1)
	assert.Equal(t, "from_autoinclude", parsed.Errors.Retry[0].Label)
}

// A dependency declared in both the unit and the autoinclude must be folded once in partial parse, never duplicated.
func TestPartialParseAutoIncludeDependencyNoDoubleCount(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "foo"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo", config.DefaultTerragruntConfigPath), []byte(``), 0644))

	require.NoError(t, os.WriteFile(cfgPath, []byte(`
dependency "foo" {
  config_path  = "./foo"
  skip_outputs = true
}
`), 0644))

	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
dependency "foo" {
  config_path  = "./foo"
  skip_outputs = true
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx = pctx.WithDecodeList(config.DependencyBlock).WithSkipOutputsResolution()

	l := logger.CreateLogger()

	parsed, err := config.PartialParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	names := map[string]int{}

	for _, dep := range parsed.TerragruntDependencies {
		names[dep.Name]++
	}

	assert.Equal(t, 1, names["foo"], "dependency foo must not be double-counted after autoinclude merge")

	require.NotNil(t, parsed.Dependencies)

	paths := map[string]int{}

	for _, p := range parsed.Dependencies.Paths {
		paths[p]++
	}

	for p, count := range paths {
		assert.Equal(t, 1, count, "dependency path %s must not be duplicated", p)
	}
}

// Recursion guard: partial parsing the autoinclude file directly must return normally instead of recursing back into the autoinclude merge.
func TestPartialParseConfigAutoIncludeFileDirectlyDoesNotRecurse(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// A sibling target directory so the dependency config_path is valid.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "foo"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo", config.DefaultTerragruntConfigPath), []byte(``), 0644))

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
	pctx = pctx.WithDecodeList(config.DependencyBlock, config.RemoteStateBlock).WithSkipOutputsResolution()

	l := logger.CreateLogger()

	parsed, err := config.PartialParseConfigFile(ctx, pctx, l, autoIncludePath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)
}
