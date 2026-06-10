package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tfInitCommand is the terraform command these autoinclude tests resolve dependency mock outputs against.
const tfInitCommand = "init"

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

	// An autoinclude that references an undefined local so its parse fails and mergeAutoIncludeIfPresent returns (nil, err).
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
	pctx.OriginalTerraformCommand = tfInitCommand

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err, "the autoinclude dependency must resolve config_path against the autoinclude's own locals")
	require.NotNil(t, parsed)
	require.NotNil(t, parsed.RemoteState)

	// Resolution of the mock output proves the autoinclude dependency decoded with its own local.target.
	assert.Equal(t, marker, parsed.RemoteState.BackendConfig["path"])
}

// TestFoldSiblingAutoIncludeDeps_FoldsIncludeInheritedDeps pins that foldSiblingAutoIncludeDeps folds in
// dependency blocks the autoinclude inherits through its OWN include blocks, not only blocks declared
// directly in the autoinclude file. The autoinclude declares no dependency of its own; it inherits "foo"
// from an included base.hcl via merge_strategy = "deep". Without that fold the unit's remote_state
// reference to dependency.foo would not resolve.
func TestFoldSiblingAutoIncludeDeps_FoldsIncludeInheritedDeps(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	// The dependency target dir.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "foo"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo", config.DefaultTerragruntConfigPath), []byte(``), 0644))

	const marker = "include-inherited-dep-marker"

	// The unit reads a dependency output but declares no dependency itself.
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

	// The included base lives in its OWN dir (not beside the autoinclude) and declares the dependency
	// the unit relies on. config_path is absolute so it resolves the same regardless of parse dir.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "base"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "base", "base.hcl"), []byte(`
dependency "foo" {
  config_path  = "`+filepath.ToSlash(filepath.Join(tmpDir, "foo"))+`"
  skip_outputs = true
  mock_outputs = {
    val = "`+marker+`"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`), 0644))

	// The autoinclude declares NO dependency of its own; it inherits "foo" through a deep include.
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
include "base" {
  path           = "`+filepath.Join(tmpDir, "base", "base.hcl")+`"
  merge_strategy = "deep"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx.OriginalTerraformCommand = tfInitCommand

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err, "a dependency inherited through the autoinclude's own include must be folded into the unit")
	require.NotNil(t, parsed)
	require.NotNil(t, parsed.RemoteState)

	// Resolution of the mock output proves the include-inherited dependency was folded.
	assert.Equal(t, marker, parsed.RemoteState.BackendConfig["path"])
}

// TestMergeAutoInclude_SameDirIncludeDoesNotRecurse pins that an autoinclude which includes a file in
// its OWN directory terminates instead of infinitely recursing the autoinclude merge. Parsing the
// included sibling must not re-merge the sibling autoinclude; without the skip-on-pulled-in-files guard
// this parse stack-overflows.
func TestMergeAutoInclude_SameDirIncludeDoesNotRecurse(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, os.WriteFile(cfgPath, []byte(`
inputs = {
  from_unit = "unit-value"
}
`), 0644))

	// A sibling file in the SAME directory as the autoinclude.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "common.hcl"), []byte(`
inputs = {
  from_common = "common-value"
}
`), 0644))

	// The autoinclude includes that same-dir sibling, the shape that previously recursed forever.
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
include "common" {
  path           = "`+filepath.ToSlash(filepath.Join(tmpDir, "common.hcl"))+`"
  merge_strategy = "deep"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	l := logger.CreateLogger()

	require.NotPanics(t, func() {
		parsed, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
		require.NoError(t, err, "a same-dir autoinclude include must terminate, not recurse")
		require.NotNil(t, parsed)
		// The included sibling's inputs still flow through the autoinclude into the unit.
		require.Contains(t, parsed.Inputs, "from_common", "the included sibling's inputs must merge through the autoinclude")
		require.Contains(t, parsed.Inputs, "from_unit")
	})
}

// TestFoldSiblingAutoIncludeDeps_PulledInFileDoesNotFoldForeignAutoInclude pins that a file pulled in by
// an autoinclude's own include (here b/base.hcl in a DIFFERENT directory, whose own sibling autoinclude
// declares a foreign "leak" dependency) does not leak that foreign dependency into the unit. The unit
// must end with the autoinclude's own "wanted" dependency and NOT "leak". It asserts the resulting
// dependency set (cache-order independent), covering the skipAutoIncludeMerge guard on the fold path.
func TestFoldSiblingAutoIncludeDeps_PulledInFileDoesNotFoldForeignAutoInclude(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Dependency targets for the wanted and foreign dependencies.
	for _, name := range []string{"wanted-target", "leak-target"} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, name), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, name, config.DefaultTerragruntConfigPath), []byte(``), 0644))
	}

	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
inputs = { from_unit = "a" }
`), 0644))

	// A's autoinclude declares a WANTED dependency and deep-includes a base file in a different dir.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "b"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, config.DefaultAutoIncludeFile), []byte(`
dependency "wanted" {
  config_path  = "`+filepath.ToSlash(filepath.Join(tmpDir, "wanted-target"))+`"
  skip_outputs = true
  mock_outputs = { val = "wanted" }
  mock_outputs_allowed_terraform_commands = ["init"]
}

include "base" {
  path           = "`+filepath.Join(tmpDir, "b", "base.hcl")+`"
  merge_strategy = "deep"
}
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "b", "base.hcl"), []byte(`
inputs = { from_base = "b" }
`), 0644))

	// The base directory has its OWN sibling autoinclude declaring a foreign dependency that must not leak.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "b", config.DefaultAutoIncludeFile), []byte(`
dependency "leak" {
  config_path  = "`+filepath.ToSlash(filepath.Join(tmpDir, "leak-target"))+`"
  skip_outputs = true
  mock_outputs = { val = "leaked" }
  mock_outputs_allowed_terraform_commands = ["init"]
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx.OriginalTerraformCommand = tfInitCommand

	parsed, err := config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	depNames := make([]string, 0, len(parsed.TerragruntDependencies))
	for _, dep := range parsed.TerragruntDependencies {
		depNames = append(depNames, dep.Name)
	}

	assert.Contains(t, depNames, "wanted", "the autoinclude's own declared dependency must still be folded into the unit")
	assert.NotContains(t, depNames, "leak", "a file pulled in by an autoinclude must not leak its own sibling autoinclude's dependency into the unit")
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
	pctx.OriginalTerraformCommand = tfInitCommand

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
	pctx.OriginalTerraformCommand = tfInitCommand

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, autoIncludePath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)
}

// Full parse must SHALLOW-merge inputs exactly like a default include: top-level keys from the unit and
// the autoinclude combine, but an overlapping nested map is replaced wholesale by the autoinclude (the
// winner), NOT deep-merged. This pins the shallow contract: under deep merge the unit's nested key would
// also survive, so this test fails if the merge is deep.
func TestMergeAutoIncludeShallowInputMerge(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, os.WriteFile(cfgPath, []byte(`
inputs = {
  tags      = { a = "1" }
  unit_only = "u"
}
`), 0644))

	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
inputs = {
  tags    = { b = "2" }
  ai_only = "x"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Top-level keys from both sides survive (shallow merge combines the top-level inputs map).
	assert.Equal(t, "u", parsed.Inputs["unit_only"], "a unit-only top-level input key must survive")
	assert.Equal(t, "x", parsed.Inputs["ai_only"], "an autoinclude-only top-level input key must survive")

	// The overlapping nested map is replaced wholesale by the autoinclude, not deep-merged.
	tags, ok := parsed.Inputs["tags"].(map[string]any)
	require.True(t, ok, "tags input should be a map")
	assert.Equal(t, "2", tags["b"], "autoinclude wins: its tags map replaces the unit's")
	_, hasA := tags["a"]
	assert.False(t, hasA, "shallow merge replaces the whole nested map; the unit's nested key must NOT survive")
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
	pctx.OriginalTerraformCommand = tfInitCommand
	pctx = pctx.WithDecodeList(config.DependencyBlock, config.RemoteStateBlock).WithSkipOutputsResolution()

	l := logger.CreateLogger()

	parsed, err := config.PartialParseConfigFile(ctx, pctx, l, autoIncludePath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed)
}

// A unit partial-parsed BEFORE its sibling autoinclude exists must not return a stale cached result
// once the autoinclude is created in-process: the cache key folds the autoinclude existence+content.
func TestPartialParseAutoIncludeCacheNotStaleOnCreate(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	// A sibling target directory so the autoinclude dependency config_path is valid.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "producer"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "producer", config.DefaultTerragruntConfigPath), []byte(``), 0644))

	// The unit declares none of the blocks under test; the autoinclude supplies them.
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
inputs = {
  name = "from-unit"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	// A shared config cache so the second parse can see (and would otherwise reuse) the first entry.
	ctx = context.WithValue(ctx, config.TerragruntConfigCacheContextKey, cache.NewCache[*config.TerragruntConfig]("test-config-cache"))
	pctx.UsePartialParseConfigCache = true
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx = pctx.WithDecodeList(config.DependencyBlock, config.RemoteStateBlock).WithSkipOutputsResolution()

	l := logger.CreateLogger()

	// First parse populates the cache while no autoinclude exists.
	before, err := config.PartialParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, before)
	assert.Nil(t, before.RemoteState, "no autoinclude yet: no remote_state should be merged")

	// Create the autoinclude AFTER the cache was populated: a dependency plus a remote_state that
	// references that dependency (the real generated unit-level autoinclude shape).
	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
dependency "producer" {
  config_path  = "./producer"
  skip_outputs = true
}

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
`), 0644))

	// Second parse must reflect the freshly created autoinclude, not the stale pre-autoinclude entry.
	after, err := config.PartialParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, after)
	require.NotNil(t, after.RemoteState, "the newly created autoinclude must be merged, not a stale cache hit")
	assert.Equal(t, "from-autoinclude", after.RemoteState.BackendConfig["path"])

	foundDep := false

	for _, dep := range after.TerragruntDependencies {
		if dep.Name == "producer" {
			foundDep = true
		}
	}

	assert.True(t, foundDep, "the autoinclude dependency must be folded after creation")
}

// A unit partial-parsed with a present autoinclude, then re-parsed after the autoinclude is EDITED
// in-process, must reflect the edit rather than the first cached result.
func TestPartialParseAutoIncludeCacheNotStaleOnEdit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

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
    path = "first"
  }
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	// A shared config cache so the second parse can see (and would otherwise reuse) the first entry.
	ctx = context.WithValue(ctx, config.TerragruntConfigCacheContextKey, cache.NewCache[*config.TerragruntConfig]("test-config-cache"))
	pctx.UsePartialParseConfigCache = true
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx = pctx.WithDecodeList(config.RemoteStateBlock).WithSkipOutputsResolution()

	l := logger.CreateLogger()

	first, err := config.PartialParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NotNil(t, first.RemoteState)
	assert.Equal(t, "first", first.RemoteState.BackendConfig["path"])

	// Edit the autoinclude in-process; the changed content must change the cache key.
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = "second"
  }
}
`), 0644))

	second, err := config.PartialParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, second)
	require.NotNil(t, second.RemoteState)
	assert.Equal(t, "second", second.RemoteState.BackendConfig["path"], "the edited autoinclude must win, not the stale cached result")
}

// A dependencies block path that overlaps a DISABLED dependency block of the same path must survive a
// sibling autoinclude merge. The merge rebuilds the block derived path list through
// dependencyBlocksToModuleDependencies, which skips disabled blocks, so the path is classified as
// dependencies-block-only and preserved instead of being dropped as if a dependency block still carried it.
func TestMergeAutoInclude_DependenciesBlockEdgeSurvivesDisabledDependencyOverlap(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	for _, name := range []string{"foo", "bar"} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, name), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, name, config.DefaultTerragruntConfigPath), []byte(``), 0644))
	}

	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
dependencies {
  paths = ["./foo"]
}

dependency "foo" {
  config_path = "./foo"
  enabled     = false
}
`), 0644))

	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
dependency "bar" {
  config_path = "./bar"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx = pctx.WithDecodeList(config.DependenciesBlock, config.DependencyBlock).WithSkipOutputsResolution()

	l := logger.CreateLogger()

	parsed, err := config.PartialParseConfigFile(ctx, pctx, l, cfgPath, nil)
	require.NoError(t, err)
	require.NotNil(t, parsed.Dependencies)
	assert.Contains(t, parsed.Dependencies.Paths, "./foo",
		"a dependencies block path overlapping a disabled dependency block must survive the autoinclude merge")
}

// TestParseTerragruntConfig_ReadsStackAutoIncludeFile pins that read_terragrunt_config can read a
// generated terragrunt.autoinclude.stack.hcl. The file is a stack-file fragment holding unit and
// stack blocks, so it must decode through the same stack parsing path as terragrunt.stack.hcl and
// return its components, not fail the strict unit-config decode.
func TestParseTerragruntConfig_ReadsStackAutoIncludeFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// The generated stack-level autoinclude lives beside a nested stack's terragrunt.stack.hcl.
	stackDir := filepath.Join(tmpDir, "stack")
	require.NoError(t, os.MkdirAll(stackDir, 0755))

	autoIncludePath := filepath.Join(stackDir, config.DefaultAutoIncludeStackFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
unit "db" {
  source = "../catalog/units/db"
  path   = "db"
}

stack "networking" {
  source = "../catalog/stacks/networking"
  path   = "networking"
}
`), 0644))

	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	l := logger.CreateLogger()

	cfgCty, err := config.ParseTerragruntConfig(ctx, pctx, l, autoIncludePath, nil)
	require.NoError(t, err, "a stack-level autoinclude file must decode through the stack parsing path")

	cfgMap, err := ctyhelper.ParseCtyValueToMap(cfgCty)
	require.NoError(t, err)

	unitsMap, ok := cfgMap[config.MetadataUnit].(map[string]any)
	require.True(t, ok, "the decoded autoinclude must expose its unit blocks")

	dbUnit, ok := unitsMap["db"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "../catalog/units/db", dbUnit["source"])
	assert.Equal(t, "db", dbUnit["path"])

	stacksMap, ok := cfgMap[config.MetadataStack].(map[string]any)
	require.True(t, ok, "the decoded autoinclude must expose its stack blocks")

	netStack, ok := stacksMap["networking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "../catalog/stacks/networking", netStack["source"])
	assert.Equal(t, "networking", netStack["path"])
}

// TestParseTerragruntConfig_ReadsUnitAutoIncludeFile pins that read_terragrunt_config reads a
// generated unit-level terragrunt.autoinclude.hcl through the regular unit-config path, returning
// its dependency blocks and inputs.
func TestParseTerragruntConfig_ReadsUnitAutoIncludeFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// The dependency target the autoinclude's dependency block points at.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "vpc"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "vpc", config.DefaultTerragruntConfigPath), []byte(``), 0644))

	autoIncludePath := filepath.Join(tmpDir, config.DefaultAutoIncludeFile)
	require.NoError(t, os.WriteFile(autoIncludePath, []byte(`
dependency "vpc" {
  config_path  = "./vpc"
  skip_outputs = true
  mock_outputs = {
    vpc_id = "mock-vpc-id"
  }
  mock_outputs_allowed_terraform_commands = ["init"]
}

inputs = {
  env = "test"
}
`), 0644))

	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)
	pctx.OriginalTerraformCommand = tfInitCommand

	l := logger.CreateLogger()

	cfgCty, err := config.ParseTerragruntConfig(ctx, pctx, l, autoIncludePath, nil)
	require.NoError(t, err, "a unit-level autoinclude file must read through the unit-config path")

	cfgMap, err := ctyhelper.ParseCtyValueToMap(cfgCty)
	require.NoError(t, err)

	inputs, ok := cfgMap[config.MetadataInputs].(map[string]any)
	require.True(t, ok, "the decoded autoinclude must expose its inputs")
	assert.Equal(t, "test", inputs["env"])

	deps, ok := cfgMap[config.MetadataDependency].(map[string]any)
	require.True(t, ok, "the decoded autoinclude must expose its dependency blocks")

	vpcDep, ok := deps["vpc"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "./vpc", vpcDep["config_path"])
}

// TestValidateStackAutoIncludes_ReportsMalformedAutoInclude pins that the strict autoinclude parse
// behind `hcl validate` reports a malformed autoinclude block (here a locals block, which is
// explicitly rejected) that the lenient ParseStackConfig decode lets through into Remain.
func TestValidateStackAutoIncludes_ReportsMalformedAutoInclude(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	stackPath := filepath.Join(tmpDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(stackPath, []byte(`
unit "app" {
  source = "./units/app"
  path   = "app"

  autoinclude {
    locals {
      env = "dev"
    }
  }
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, stackPath)
	pctx.Experiments.EnableExperiment(experiment.StackDependencies)

	l := logger.CreateLogger()

	stackCfg, err := config.ReadStackConfigFile(ctx, l, pctx, stackPath, nil)
	require.NoError(t, err, "the lenient stack decode must keep accepting the malformed autoinclude block")

	err = config.ValidateStackAutoIncludes(ctx, l, pctx, stackPath, stackCfg, nil)
	require.Error(t, err, "the strict autoinclude parse must report the locals block")

	var stageErr config.AutoIncludeParserStageError
	require.ErrorAs(t, err, &stageErr)

	var localsErr inthclparse.AutoIncludeLocalsBlockError
	require.ErrorAs(t, err, &localsErr)
	assert.Equal(t, "app", localsErr.Component)
}
