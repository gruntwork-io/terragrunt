package output

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestNestUnitOutputs_SingleUnit(t *testing.T) {
	t.Parallel()

	flat := map[string]map[string]cty.Value{
		"app": {
			"name": cty.StringVal("myapp"),
		},
	}

	result, err := nestUnitOutputs(flat)
	require.NoError(t, err)
	assert.Contains(t, result, "app")
}

func TestNestUnitOutputs_NestedPath(t *testing.T) {
	t.Parallel()

	flat := map[string]map[string]cty.Value{
		"stack.unit": {
			"value": cty.StringVal("test"),
		},
	}

	result, err := nestUnitOutputs(flat)
	require.NoError(t, err)

	stackMap, ok := result["stack"].(map[string]any)
	require.True(t, ok, "stack should be a nested map")
	assert.Contains(t, stackMap, "unit")
}

func TestNestUnitOutputs_DeeplyNested(t *testing.T) {
	t.Parallel()

	flat := map[string]map[string]cty.Value{
		"a.b.c": {
			"out": cty.NumberIntVal(42),
		},
	}

	result, err := nestUnitOutputs(flat)
	require.NoError(t, err)

	aMap, ok := result["a"].(map[string]any)
	require.True(t, ok)

	bMap, ok := aMap["b"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, bMap, "c")
}

func TestNestUnitOutputs_MultipleUnitsUnderSameStack(t *testing.T) {
	t.Parallel()

	flat := map[string]map[string]cty.Value{
		"stack.app1": {
			"port": cty.NumberIntVal(8080),
		},
		"stack.app2": {
			"port": cty.NumberIntVal(9090),
		},
	}

	result, err := nestUnitOutputs(flat)
	require.NoError(t, err)

	stackMap, ok := result["stack"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, stackMap, "app1")
	assert.Contains(t, stackMap, "app2")
}

func TestNestUnitOutputs_EmptyOutputs(t *testing.T) {
	t.Parallel()

	flat := map[string]map[string]cty.Value{
		"excluded": {},
	}

	result, err := nestUnitOutputs(flat)
	require.NoError(t, err)
	assert.Contains(t, result, "excluded")
}

func TestNestUnitOutputs_EmptyInput(t *testing.T) {
	t.Parallel()

	result, err := nestUnitOutputs(map[string]map[string]cty.Value{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNestUnitOutputs_ConflictingPath(t *testing.T) {
	t.Parallel()

	flat := map[string]map[string]cty.Value{
		"stack": {
			"val": cty.StringVal("leaf"),
		},
		"stack.child": {
			"val": cty.StringVal("nested"),
		},
	}

	_, err := nestUnitOutputs(flat)
	assert.Error(t, err)
}

func TestDiscoverExcludedPaths_ExcludedUnit(t *testing.T) {
	t.Parallel()

	dir := tmpDir(t)
	unitDir := filepath.Join(dir, "excluded-unit")

	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`
exclude {
  if      = true
  actions = ["output"]
}
`), 0644))

	opts := &options.TerragruntOptions{
		WorkingDir:       dir,
		RootWorkingDir:   dir,
		TerraformCommand: "output",
	}

	result := discoverExcludedPaths(context.Background(), testLogger(), opts, dir)
	assert.True(t, result[unitDir], "unit with output in actions should be excluded")
}

func TestDiscoverExcludedPaths_NotExcludedUnit(t *testing.T) {
	t.Parallel()

	dir := tmpDir(t)
	unitDir := filepath.Join(dir, "normal-unit")

	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`
terraform {
  source = "."
}
`), 0644))

	opts := &options.TerragruntOptions{
		WorkingDir:       dir,
		RootWorkingDir:   dir,
		TerraformCommand: "output",
	}

	result := discoverExcludedPaths(context.Background(), testLogger(), opts, dir)
	assert.False(t, result[unitDir], "unit without exclude block should not be excluded")
}

func TestDiscoverExcludedPaths_IfFalse(t *testing.T) {
	t.Parallel()

	dir := tmpDir(t)
	unitDir := filepath.Join(dir, "conditional-unit")

	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`
exclude {
  if      = false
  actions = ["output"]
}
`), 0644))

	opts := &options.TerragruntOptions{
		WorkingDir:       dir,
		RootWorkingDir:   dir,
		TerraformCommand: "output",
	}

	result := discoverExcludedPaths(context.Background(), testLogger(), opts, dir)
	assert.False(t, result[unitDir], "unit with if=false should not be excluded")
}

func TestDiscoverExcludedPaths_ActionMismatch(t *testing.T) {
	t.Parallel()

	dir := tmpDir(t)
	unitDir := filepath.Join(dir, "plan-only")

	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`
exclude {
  if      = true
  actions = ["plan", "apply"]
}
`), 0644))

	opts := &options.TerragruntOptions{
		WorkingDir:       dir,
		RootWorkingDir:   dir,
		TerraformCommand: "output",
	}

	result := discoverExcludedPaths(context.Background(), testLogger(), opts, dir)
	assert.False(t, result[unitDir], "unit excluding plan/apply should not be excluded for output")
}

func TestDiscoverExcludedPaths_AllExceptOutput(t *testing.T) {
	t.Parallel()

	dir := tmpDir(t)
	unitDir := filepath.Join(dir, "all-except-output")

	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`
exclude {
  if      = true
  no_run  = true
  actions = ["all_except_output"]
}
`), 0644))

	opts := &options.TerragruntOptions{
		WorkingDir:       dir,
		RootWorkingDir:   dir,
		TerraformCommand: "output",
	}

	result := discoverExcludedPaths(context.Background(), testLogger(), opts, dir)
	assert.False(t, result[unitDir], "unit with all_except_output should not be excluded for output")
}

func TestDiscoverExcludedPaths_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := tmpDir(t)

	opts := &options.TerragruntOptions{
		WorkingDir:       dir,
		RootWorkingDir:   dir,
		TerraformCommand: "output",
	}

	result := discoverExcludedPaths(context.Background(), testLogger(), opts, dir)
	assert.Empty(t, result)
}

func TestDiscoverExcludedPaths_NonExistentDir(t *testing.T) {
	t.Parallel()

	opts := &options.TerragruntOptions{
		WorkingDir:       "/nonexistent",
		RootWorkingDir:   "/nonexistent",
		TerraformCommand: "output",
	}

	result := discoverExcludedPaths(context.Background(), testLogger(), opts, "/nonexistent")
	assert.NotNil(t, result, "should return empty map, not nil")
	assert.Empty(t, result)
}

func TestDiscoverExcludedPaths_MixedUnits(t *testing.T) {
	t.Parallel()

	dir := tmpDir(t)
	excludedDir := filepath.Join(dir, "excluded")
	normalDir := filepath.Join(dir, "normal")

	require.NoError(t, os.MkdirAll(excludedDir, 0755))
	require.NoError(t, os.MkdirAll(normalDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(excludedDir, "terragrunt.hcl"), []byte(`
exclude {
  if      = true
  actions = ["output"]
}
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(normalDir, "terragrunt.hcl"), []byte(`
terraform {
  source = "."
}
`), 0644))

	opts := &options.TerragruntOptions{
		WorkingDir:       dir,
		RootWorkingDir:   dir,
		TerraformCommand: "output",
	}

	result := discoverExcludedPaths(context.Background(), testLogger(), opts, dir)
	assert.True(t, result[excludedDir], "excluded unit should be in the map")
	assert.False(t, result[normalDir], "normal unit should not be in the map")
}

func TestStackOutput_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := tmpDir(t)

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = dir
	opts.RootWorkingDir = dir
	opts.TerraformCommand = "output"

	result, err := StackOutput(context.Background(), testLogger(), opts)
	require.NoError(t, err)
	assert.Equal(t, cty.NilVal, result, "empty dir should return NilVal")
}

func TestBuildWorktreesIfNeeded_NoFilters(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()

	wts, err := buildWorktreesIfNeeded(context.Background(), testLogger(), opts)
	require.NoError(t, err)
	assert.Nil(t, wts, "should return nil when no git filters")
}

func testLogger() log.Logger {
	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(format.NewFormatter(format.NewKeyValueFormatPlaceholders())))
}

func tmpDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	return resolved
}
