package hclparse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/function"
)

// noFuncs is the factory returning an empty HCL function map shared by tests that
// exercise only literal stack attributes, parse errors, or panic contracts. Any HCL
// function call against it resolves to "function not found", which is the
// intended outcome for these tests.
func noFuncs(string) (map[string]function.Function, error) {
	return map[string]function.Function{}, nil
}

func TestBuildComponentRefMapExposesPath(t *testing.T) {
	t.Parallel()

	got := hclparse.BuildComponentRefMap([]hclparse.ComponentRef{
		{Name: "networking", Path: ".terragrunt-stack/networking"},
	})

	networking := got.AsValueMap()["networking"].AsValueMap()
	assert.Equal(t, ".terragrunt-stack/networking", networking["path"].AsString())
	_, hasName := networking["name"]
	assert.False(t, hasName, "ref object should not expose .name")
}

func TestUnitPathsFromStackDir(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}

unit "db" {
  source = "../units/db"
  path   = "db"
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.NoError(t, err)
	require.Len(t, paths, 2)
	assert.Contains(t, paths[0], ".terragrunt-stack")
	assert.Contains(t, paths[1], ".terragrunt-stack")
}

func TestUnitPathsFromStackDir_RecursesNestedStacks(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	// A stack whose file declares only a nested stack, no direct units.
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`stack "more" {
  source = "."
  path   = "more"
}
`), 0644))
	// The nested stack, one level deeper, holds the only unit.
	require.NoError(t, vfs.WriteFile(fs, "/test/.terragrunt-stack/more/terragrunt.stack.hcl", []byte(`unit "deep" {
  source = "."
  path   = "deep"
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join("/test", ".terragrunt-stack", "more", ".terragrunt-stack", "deep")}, paths)
}

// TestUnitPathsFromStackDir_MergesStackAutoInclude pins that discovery folds a sibling
// terragrunt.autoinclude.stack.hcl into expansion, so a unit injected by a stack-level autoinclude
// produces a DAG edge the same way a full stack parse materializes it.
func TestUnitPathsFromStackDir_MergesStackAutoInclude(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}
`), 0644))
	// A stack-level autoinclude beside the stack file injects an extra unit.
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.autoinclude.stack.hcl", []byte(`unit "injected" {
  source = "../units/injected"
  path   = "injected"
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.NoError(t, err)
	require.Len(t, paths, 2, "the autoinclude-injected unit must expand alongside the stack file's own unit")
	assert.Contains(t, paths, filepath.Join("/test", ".terragrunt-stack", "vpc"))
	assert.Contains(t, paths, filepath.Join("/test", ".terragrunt-stack", "injected"))
}

// TestUnitPathsFromStackDir_StackAutoIncludePathReferencesSiblingRef pins that discovery resolves an
// autoinclude block whose path references a base unit.<name>.path, matching the full stack parse. Discovery
// must publish the base component refs before decoding the autoinclude, or it fails with an undefined unit
// variable for a config the full parse accepts.
func TestUnitPathsFromStackDir_StackAutoIncludePathReferencesSiblingRef(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "anchor" {
  source = "."
  path   = "anchor"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.autoinclude.stack.hcl", []byte(`unit "vpc" {
  source = "."
  path   = "${unit.anchor.path}-vpc"
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.NoError(t, err, "an autoinclude path referencing a base unit.<name>.path must resolve during discovery")
	assert.Len(t, paths, 2, "both the base anchor unit and the injected vpc unit must expand")
}

// TestUnitPathsFromStackDir_RecursesTwoStackLevels pins that discovery recurses through two levels of nested
// stacks (a stack whose generated stack references another stack), so a unit two levels deep enumerates with
// BOTH .terragrunt-stack segments. This guards the deep-nested case where a unit was reported one level too
// high (e.g. core/data instead of core/.terragrunt-stack/data).
func TestUnitPathsFromStackDir_RecursesTwoStackLevels(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))

	// Top stack references the eks stack.
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`stack "eks" {
  source = "."
  path   = "eks"
}
`), 0644))

	// The generated eks stack references the core stack and declares a sibling unit.
	require.NoError(t, vfs.WriteFile(fs, "/test/.terragrunt-stack/eks/terragrunt.stack.hcl", []byte(`stack "core" {
  source = "."
  path   = "core"
}

unit "cluster" {
  source = "."
  path   = "cluster"
}
`), 0644))

	// The generated core stack declares two units.
	require.NoError(t, vfs.WriteFile(fs, "/test/.terragrunt-stack/eks/.terragrunt-stack/core/terragrunt.stack.hcl", []byte(`unit "data" {
  source = "."
  path   = "data"
}

unit "vpc" {
  source = "."
  path   = "vpc"
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.NoError(t, err)

	clusterPath := filepath.Join("/test", ".terragrunt-stack", "eks", ".terragrunt-stack", "cluster")
	dataPath := filepath.Join("/test", ".terragrunt-stack", "eks", ".terragrunt-stack", "core", ".terragrunt-stack", "data")
	vpcPath := filepath.Join("/test", ".terragrunt-stack", "eks", ".terragrunt-stack", "core", ".terragrunt-stack", "vpc")

	assert.ElementsMatch(t, []string{clusterPath, dataPath, vpcPath}, paths,
		"a unit two stack levels deep must enumerate through both .terragrunt-stack segments, not one level too high")
}

// TestUnitPathsFromStackDir_RecursesStackAutoIncludeInjectedStack pins that a stack injected by a
// stack-level autoinclude is recursed into, so its nested units also produce DAG edges.
func TestUnitPathsFromStackDir_RecursesStackAutoIncludeInjectedStack(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "vpc" {
  source = "."
  path   = "vpc"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.autoinclude.stack.hcl", []byte(`stack "more" {
  source = "."
  path   = "more"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/.terragrunt-stack/more/terragrunt.stack.hcl", []byte(`unit "deep" {
  source = "."
  path   = "deep"
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.NoError(t, err)
	assert.Contains(t, paths, filepath.Join("/test", ".terragrunt-stack", "vpc"))
	assert.Contains(t, paths, filepath.Join("/test", ".terragrunt-stack", "more", ".terragrunt-stack", "deep"), "a stack injected by the autoinclude must be recursed into")
}

// TestUnitPathsFromStackDir_StackAutoIncludeDepValuesRejected pins that discovery applies the same
// dep-values backstop as the full parse: a stack autoinclude whose injected unit values reference
// dependency outputs is rejected, not silently expanded into DAG edges.
func TestUnitPathsFromStackDir_StackAutoIncludeDepValuesRejected(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "base" {
  source = "."
  path   = "base"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.autoinclude.stack.hcl", []byte(`unit "extra" {
  source = "."
  path   = "extra"
  values = {
    v = dependency.foo.outputs.bar
  }
}
`), 0644))

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.Error(t, err, "a stack autoinclude whose injected values reference dependency outputs must be rejected by discovery")

	var typed hclparse.StackAutoIncludeDependencyValuesError
	require.ErrorAs(t, err, &typed, "discovery must surface the same typed dep-values error as the full parse")
}

// TestUnitPathsFromStackDir_StackAutoIncludeSameNameOverrides pins that discovery overrides a base
// unit wholesale when the autoinclude injects a unit with the same name, so the injected path wins and
// the base path is dropped, matching the full stack parse.
func TestUnitPathsFromStackDir_StackAutoIncludeSameNameOverrides(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "vpc" {
  source = "."
  path   = "vpc"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.autoinclude.stack.hcl", []byte(`unit "vpc" {
  source = "."
  path   = "vpc-injected"
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.NoError(t, err, "a same-name injected unit must override the base unit, not raise a duplicate-name error")

	assert.Equal(t, []string{filepath.Join("/test", ".terragrunt-stack", "vpc-injected")}, paths,
		"the injected unit overrides the base unit wholesale, so only the injected path remains")
}

// TestUnitPathsFromStackDir_StackFileDuplicateNameRejected pins that a duplicate unit name within the
// base stack file itself is still rejected; override only collapses base-vs-autoinclude collisions.
func TestUnitPathsFromStackDir_StackFileDuplicateNameRejected(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "vpc" {
  source = "."
  path   = "vpc"
}

unit "vpc" {
  source = "."
  path   = "vpc-again"
}
`), 0644))

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.Error(t, err, "a duplicate unit name within the base stack file must be rejected by discovery")

	var typed hclparse.DuplicateUnitNameError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, "vpc", typed.Name)
}

// TestUnitPathsFromStackDir_StackFileDuplicateNameRejectedEvenWhenOverridden pins that a duplicate name in
// the base stack file is rejected even when the autoinclude also targets that name, so the override merge
// cannot mask a base-file duplicate by collapsing both base entries into the single override.
func TestUnitPathsFromStackDir_StackFileDuplicateNameRejectedEvenWhenOverridden(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "vpc" {
  source = "."
  path   = "vpc"
}

unit "vpc" {
  source = "."
  path   = "vpc-again"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.autoinclude.stack.hcl", []byte(`unit "vpc" {
  source = "."
  path   = "vpc-injected"
}
`), 0644))

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.Error(t, err, "a base-file duplicate must be rejected even when the autoinclude overrides that name")

	var typed hclparse.DuplicateUnitNameError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, "vpc", typed.Name)
}

// TestUnitPathsFromStackDir_StackAutoIncludeDuplicateNameRejected pins that a duplicate name within the
// autoinclude file itself is rejected, mirroring the base-file rejection, rather than silently collapsing
// the two same-name blocks into one via last-writer-wins.
func TestUnitPathsFromStackDir_StackAutoIncludeDuplicateNameRejected(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "base" {
  source = "."
  path   = "base"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.autoinclude.stack.hcl", []byte(`unit "extra" {
  source = "."
  path   = "extra"
}

unit "extra" {
  source = "."
  path   = "extra-dup"
}
`), 0644))

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.Error(t, err, "a duplicate name within the autoinclude file must be rejected, not silently collapsed")

	var typed hclparse.DuplicateUnitNameError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, "extra", typed.Name)
}

// TestUnitPathsFromStackDir_StackAutoIncludeLocalsRejected pins that discovery rejects a stack
// autoinclude that defines top-level locals, matching the full parse.
func TestUnitPathsFromStackDir_StackAutoIncludeLocalsRejected(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "base" {
  source = "."
  path   = "base"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.autoinclude.stack.hcl", []byte(`locals {
  x = "y"
}

unit "extra" {
  source = "."
  path   = "extra"
}
`), 0644))

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.Error(t, err, "a stack autoinclude defining top-level locals must be rejected by discovery")
	assert.Contains(t, err.Error(), "locals", "the rejection must identify the unsupported locals block")
}

// TestUnitPathsFromStackDir_StackAutoIncludeStrayContentRejected pins that discovery rejects a stack
// autoinclude carrying stray top-level content (here a generate block) that the full parse also
// rejects, instead of silently absorbing it. Only unit and stack blocks are allowed at the top level.
func TestUnitPathsFromStackDir_StackAutoIncludeStrayContentRejected(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "base" {
  source = "."
  path   = "base"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.autoinclude.stack.hcl", []byte(`generate "stray" {
  path      = "stray.tf"
  if_exists = "overwrite"
  contents  = ""
}

unit "extra" {
  source = "."
  path   = "extra"
}
`), 0644))

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.Error(t, err, "a stack autoinclude carrying stray top-level content must be rejected by discovery, matching the full parse")
}

// TestUnitPathsFromStackDir_FuncFactoryRebuiltPerNestedDir pins the dir-scoping
// contract: the factory is invoked once per visited stack dir, each time with that
// dir, so dir-sensitive functions resolve against the nested dir, not the top dir.
func TestUnitPathsFromStackDir_FuncFactoryRebuiltPerNestedDir(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`stack "more" {
  source = "."
  path   = "more"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/.terragrunt-stack/more/terragrunt.stack.hcl", []byte(`unit "deep" {
  source = "."
  path   = "deep"
}
`), 0644))

	var seenDirs []string

	funcsFor := func(dir string) (map[string]function.Function, error) {
		seenDirs = append(seenDirs, dir)
		return map[string]function.Function{}, nil
	}

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test", funcsFor)
	require.NoError(t, err)

	nested := filepath.Join("/test", ".terragrunt-stack", "more")
	assert.Equal(t, []string{"/test", nested}, seenDirs, "the factory must be rebuilt for the top dir and the nested dir")
}

// TestUnitPathsFromStackDir_NilFuncsFactoryMapPanics pins that a factory returning a nil map is a programming error and panics with a clear message.
func TestUnitPathsFromStackDir_NilFuncsFactoryMapPanics(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "vpc" {
  source = "."
  path   = "vpc"
}
`), 0644))

	nilMapFactory := func(string) (map[string]function.Function, error) {
		return nil, nil
	}

	assert.PanicsWithValue(t,
		`hclparse.UnitPathsFromStackDir: funcsFor returned a nil map (stackDir="/test")`,
		func() { _, _ = hclparse.UnitPathsFromStackDir(fs, "/test", nilMapFactory) },
	)
}

func TestUnitPathsFromStackDir_CycleTerminates(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	// A nested stack whose path escapes back to its own directory; without the
	// visited guard the expansion would recurse forever.
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`stack "loop" {
  source = "."
  path   = ".."
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestUnitPathsFromStackDir_DepthCapReturnsError(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	// Build a chain deeper than the recursion cap, every level a distinct path so the
	// visited set never collapses it; only the depth cap can stop the recursion.
	dir := "/test"
	for range 1002 {
		require.NoError(t, fs.MkdirAll(dir, 0755))
		require.NoError(t, vfs.WriteFile(fs, filepath.Join(dir, "terragrunt.stack.hcl"), []byte(`stack "next" {
  source = "."
  path   = "next"
}
`), 0644))
		dir = filepath.Join(dir, ".terragrunt-stack", "next")
	}

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.Error(t, err)

	var depthErr hclparse.StackRecursionDepthExceededError
	require.ErrorAs(t, err, &depthErr)
}

func TestUnitPathsFromStackDir_WithIncludedUnits(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test/includes", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/includes/units.hcl", []byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}
`), 0644))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`
include "units" {
  path = "./includes/units.hcl"
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Contains(t, paths[0], filepath.Join(hclparse.StackDir, "vpc"))
}

func TestUnitPathsFromStackDir_PathWithUnsupportedFunctionReturnsError(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = get_repo_root()
}
`), 0644))

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get_repo_root")
}

func TestUnitPathsFromStackDir_NotAStack(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.NoError(t, err)
	assert.Nil(t, paths)
}

func TestUnitPathsFromStackDir_Nonexistent(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/nonexistent", noFuncs)
	require.NoError(t, err)
	assert.Nil(t, paths)
}

func TestUnitPathsFromStackDir_MalformedReturnsError(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "x" { source = "." `), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test", noFuncs)
	require.Error(t, err)
	assert.Nil(t, paths)

	var fpe hclparse.FileParseError
	require.ErrorAs(t, err, &fpe)
}

func TestParseStackFileFromPath(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`
unit "app" {
  source = "../units/app"
  path   = "app"
}
`), 0644))

	result, err := hclparse.ParseStackFileFromPath(fs, "/test")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Units, 1)
	assert.Equal(t, "app", result.Units[0].Name)
}

func TestParseStackFileFromPath_NoFile(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))

	result, err := hclparse.ParseStackFileFromPath(fs, "/test")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// ParseStackFileFromPath is strict: passing a regular file produces an error. Callers that may receive non-directory paths (e.g. discovery) must filter them upstream.
func TestParseStackFileFromPath_StackDirIsFileReturnsError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "another-name.hcl")
	require.NoError(t, os.WriteFile(filePath, []byte(`# regular file, not a directory`), 0644))

	result, err := hclparse.ParseStackFileFromPath(vfs.NewOSFS(), filePath)
	require.Error(t, err)
	assert.Nil(t, result)

	var readErr hclparse.FileReadError
	require.ErrorAs(t, err, &readErr)
	// On macOS, t.TempDir() returns paths under /var/folders/... where /var is a symlink to /private/var; util.ResolvePath follows it, so resolve our side too before comparing.
	resolvedFilePath, evalErr := filepath.EvalSymlinks(filePath)
	require.NoError(t, evalErr)
	assert.Equal(t, filepath.Join(resolvedFilePath, "terragrunt.stack.hcl"), readErr.FilePath)
}

func TestParseStackFileFromPath_Symlink(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create the real stack directory
	realDir := filepath.Join(tmpDir, "real-stack")
	require.NoError(t, os.MkdirAll(realDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(realDir, "terragrunt.stack.hcl"), []byte(`
unit "app" {
  source = "../units/app"
  path   = "app"
}
`), 0644))

	// Create a symlink pointing to the real directory
	symlinkDir := filepath.Join(tmpDir, "symlinked-stack")
	require.NoError(t, os.Symlink(realDir, symlinkDir))

	// Parse via the symlink - should work the same as via real path
	result, err := hclparse.ParseStackFileFromPath(vfs.NewOSFS(), symlinkDir)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Units, 1)
	assert.Equal(t, "app", result.Units[0].Name)
}

// Uses OSFS because MemMapFS does not faithfully reproduce os.Symlink semantics that util.ResolvePath relies on.
func TestUnitPathsFromStackDir_Symlink(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	realDir := filepath.Join(tmpDir, "real-stack")
	require.NoError(t, os.MkdirAll(realDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(realDir, "terragrunt.stack.hcl"), []byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}
`), 0644))

	symlinkDir := filepath.Join(tmpDir, "symlinked-stack")
	require.NoError(t, os.Symlink(realDir, symlinkDir))

	paths, err := hclparse.UnitPathsFromStackDir(vfs.NewOSFS(), symlinkDir, noFuncs)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	// Path should resolve to the real directory, not the symlink path.
	assert.Contains(t, paths[0], "real-stack")
	assert.NotContains(t, paths[0], "symlinked-stack")
}

func TestParseStackFile_WithInclude(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test/includes", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/includes/extra.stack.hcl", []byte(`
unit "monitoring" {
  source = "../catalog/units/monitoring"
  path   = "monitoring"
}
`), 0644))

	// Create the main stack file
	mainSrc := `
include "extra" {
  path = "/test/includes/extra.stack.hcl"
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: []byte(mainSrc), Filename: "/test/terragrunt.stack.hcl", StackDir: "/test"})
	require.NoError(t, err)

	// Should have both units: vpc from main + monitoring from include
	require.Len(t, result.Units, 2)

	names := []string{result.Units[0].Name, result.Units[1].Name}
	assert.Contains(t, names, "vpc")
	assert.Contains(t, names, "monitoring")
}

func TestBuildComponentRefMap_Empty(t *testing.T) {
	t.Parallel()

	result := hclparse.BuildComponentRefMap(nil)
	assert.True(t, result.Type().IsObjectType())
}

func TestBuildComponentRefMap_WithRefs(t *testing.T) {
	t.Parallel()

	refs := []hclparse.ComponentRef{
		{Name: "vpc", Path: "vpc"},
		{Name: "app", Path: "app-service"},
	}

	result := hclparse.BuildComponentRefMap(refs)

	require.True(t, result.Type().IsObjectType())

	vpcVal := result.GetAttr("vpc")
	require.True(t, vpcVal.Type().IsObjectType())
	assert.Equal(t, "vpc", vpcVal.GetAttr("path").AsString())

	appVal := result.GetAttr("app")
	require.True(t, appVal.Type().IsObjectType())
	assert.Equal(t, "app-service", appVal.GetAttr("path").AsString())
}

// TestRedirectIntoStackDir pins the dependency-path hop: a path that drills into a nested stack's component
// (one .terragrunt-stack segment too high) is corrected to the real component directory, while an existing
// component, a missing target, and a no_dot_terragrunt_stack-style literal are left unchanged.
func TestRedirectIntoStackDir(t *testing.T) {
	t.Parallel()

	const (
		unitFile  = "terragrunt.hcl"
		stackFile = "terragrunt.stack.hcl"
	)

	testCases := []struct {
		name  string
		input string
		want  string
		files []string
	}{
		{
			name:  "existing unit dir is returned unchanged",
			files: []string{"/s/.terragrunt-stack/vpc/" + unitFile},
			input: "/s/.terragrunt-stack/vpc",
			want:  "/s/.terragrunt-stack/vpc",
		},
		{
			name:  "existing stack dir is returned unchanged",
			files: []string{"/s/.terragrunt-stack/core/" + stackFile},
			input: "/s/.terragrunt-stack/core",
			want:  "/s/.terragrunt-stack/core",
		},
		{
			name:  "drill into a nested unit hops through .terragrunt-stack",
			files: []string{"/s/core/.terragrunt-stack/vpc/" + unitFile},
			input: "/s/core/vpc",
			want:  "/s/core/.terragrunt-stack/vpc",
		},
		{
			name:  "drill into a nested sub-stack hops through .terragrunt-stack",
			files: []string{"/s/core/.terragrunt-stack/net/" + stackFile},
			input: "/s/core/net",
			want:  "/s/core/.terragrunt-stack/net",
		},
		{
			name:  "no component at either location returns the input unchanged",
			files: []string{"/s/core/" + stackFile},
			input: "/s/core/missing",
			want:  "/s/core/missing",
		},
		{
			name:  "a literal component wins over a hopped sibling",
			files: []string{"/s/core/vpc/" + unitFile, "/s/core/.terragrunt-stack/vpc/" + unitFile},
			input: "/s/core/vpc",
			want:  "/s/core/vpc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()

			for _, f := range tc.files {
				require.NoError(t, fs.MkdirAll(filepath.Dir(f), 0755))
				require.NoError(t, vfs.WriteFile(fs, f, []byte("# test"), 0644))
			}

			got := hclparse.RedirectIntoStackDir(fs, tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}
