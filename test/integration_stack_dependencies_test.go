//go:build !windows

package test_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/git"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

const (
	testFixtureStackDepsAutoInclude              = "fixtures/stacks/stack-dependencies-autoinclude"
	testFixtureStackDepsStackRef                 = "fixtures/stacks/stack-dependencies-stack-ref"
	testFixtureStackDepsBasic                    = "fixtures/stacks/stack-deps-basic"
	testFixtureStackDepsChain                    = "fixtures/stacks/stack-deps-chain"
	testFixtureStackDepsCrossStack               = "fixtures/stacks/stack-deps-cross-stack"
	testFixtureStackDepsTree                     = "fixtures/stacks/stack-deps-tree"
	testFixtureStackDepsAutoIncParserLimit       = "fixtures/stacks/stack-deps-autoinclude-parser-limit"
	testFixtureStackDepsAutoIncViaInclude        = "fixtures/stacks/stack-deps-autoinclude-via-include"
	testFixtureStackDepsAutoIncViaDynInclude     = "fixtures/stacks/stack-deps-autoinclude-via-dyn-include"
	testFixtureStackDepsAutoIncViaIncludeSuccess = "fixtures/stacks/stack-deps-autoinclude-via-include-success"
	testFixtureStackDepsAutoIncComplexSiblings   = "fixtures/stacks/stack-deps-autoinclude-complex-siblings"
	testFixtureStackDepsRunAllFuncsInNestedStack = "fixtures/stacks/stack-deps-runall-funcs-in-nested-stack"
	testFixtureStackDepsNoDeps                   = "fixtures/stacks/stack-deps-no-deps"
	testFixtureStackDepsMergePrecedence          = "fixtures/stacks/stack-deps-merge-precedence"
	testFixtureStackDepsArbitraryOverride        = "fixtures/stacks/stack-deps-arbitrary-override"
	testFixtureStackDepsArbitraryRetry           = "fixtures/stacks/stack-deps-arbitrary-retry"
	testFixtureStackDepsArbitraryFeature         = "fixtures/stacks/stack-deps-arbitrary-feature"
	testFixtureStackDepsArbitraryIgnore          = "fixtures/stacks/stack-deps-arbitrary-ignore"
	testFixtureStackDepsStackAutoInclude         = "fixtures/stacks/stack-deps-stack-autoinclude"
	testFixtureStackDepsStackAutoIncludeNested   = "fixtures/stacks/stack-deps-stack-autoinclude-nested"
	testFixtureStackDepsCrossLevelValues         = "fixtures/stacks/stack-deps-cross-level-values"
	testFixtureStackDepsStackAutoIncDepValues    = "fixtures/stacks/stack-deps-stack-autoinclude-dep-values"
	testFixtureStackDepsLocalsReadConfigDep      = "fixtures/stacks/stack-deps-locals-readconfig-dep"
	testFixtureStackDepsRemoteStateDep           = "fixtures/stacks/stack-deps-remote-state-dep"
	testFixtureStackDepsNestedRemoteStateDep     = "fixtures/stacks/stack-deps-nested-remote-state-dep"
	testFixtureStackDepsDupDependency            = "fixtures/stacks/stack-deps-dup-dependency"
	testFixtureStackDepsDepMockMerge             = "fixtures/stacks/stack-deps-dep-mock-merge"
	testFixtureStackDepsDisabledAutoIncDep       = "fixtures/stacks/stack-deps-disabled-autoinclude-dep"
	testFixtureStackDepsNestedUnitDep            = "fixtures/stacks/stack-deps-nested-unit-dep"
	testFixtureStackDepsApplyNoMocks             = "fixtures/stacks/stack-deps-apply-no-mocks"
	testFixtureStackDepsStackAutoIncOverride     = "fixtures/stacks/stack-deps-stack-autoinclude-override"
	testFixtureStackDepsStackAutoIncLocalPath    = "fixtures/stacks/stack-deps-stack-autoinclude-local-path"
	testFixtureStackDepsMockLocal                = "fixtures/stacks/stack-deps-mock-local"
	testFixtureStackDepsAutoIncValuesResolved    = "fixtures/stacks/stack-deps-autoinclude-values-resolved"
	testFixtureStackDepsAutoIncFuncs             = "fixtures/stacks/stack-deps-autoinclude-funcs"
	testFixtureStackDepsValuesSiblingAutoInc     = "fixtures/stacks/stack-deps-values-sibling-autoinclude"
	testFixtureStackDepsStackValuesLocals        = "fixtures/stacks/stack-deps-stack-values-locals"
	testFixtureStackDepsHCLValidateAutoInc       = "fixtures/stacks/stack-deps-hclvalidate-autoinclude"
	testFixtureStackDepsAutoIncObjectKey         = "fixtures/stacks/stack-deps-autoinclude-object-key"
)

// TestStackDepsAutoIncludeGenerationAndDAG tests parsing, autoinclude generation,
// and DAG dependency extraction in a single flow.
func TestStackDepsAutoIncludeGenerationAndDAG(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoInclude)
	liveDir := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoInclude, "live")
	liveDir, err := filepath.EvalSymlinks(liveDir)
	require.NoError(t, err)

	stackFile := filepath.Join(liveDir, "terragrunt.stack.hcl")

	srcBytes, err := os.ReadFile(stackFile)
	require.NoError(t, err)

	result, err := inthclparse.ParseStackFile(vfs.NewOSFS(), &inthclparse.ParseStackFileInput{
		Src:      srcBytes,
		Filename: stackFile,
		StackDir: liveDir,
	})
	require.NoError(t, err)
	require.Len(t, result.Units, 2)

	resolved, ok := result.AutoIncludes[inthclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok, "app should have autoinclude")
	require.Len(t, resolved.Dependencies, 1)
	assert.Equal(t, "vpc", resolved.Dependencies[0].Name)

	appDir := filepath.Join(liveDir, inthclparse.StackDir, "app")

	err = inthclparse.GenerateAutoIncludeFile(vfs.NewOSFS(), resolved, appDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	autoIncludePath := filepath.Join(appDir, inthclparse.AutoIncludeFile)
	require.FileExists(t, autoIncludePath)

	generated, err := os.ReadFile(autoIncludePath)
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(t, content, "Generated by Terragrunt")
	assert.Contains(t, content, `dependency "vpc"`)
	assert.Contains(t, content, "../vpc")
	assert.Contains(t, content, "mock_outputs_allowed_terraform_commands")
	assert.Contains(t, content, "mock-vpc-id")
	// A stack local renders at generate time, including inside the inputs zone.
	assert.Contains(t, content, `"test"`, "a stack local inside inputs renders at generate time")
	assert.NotContains(t, content, "local.env", "a stack local must not be left verbatim")
	assert.Contains(t, content, "dependency.vpc.outputs.vpc_id")

	// Verify DAG sees the dependency
	depPaths, depErr := inthclparse.AutoIncludeDependencyPaths(vfs.NewOSFS(), appDir)
	require.NoError(t, depErr)
	require.Len(t, depPaths, 1)

	vpcDir := filepath.Join(liveDir, inthclparse.StackDir, "vpc")
	assert.Equal(t, vpcDir, depPaths[0])
}

// TestStackDepsMockLocalResolvesLocal pins, end to end, that an autoinclude dependency's mock_outputs resolves
// stack-level locals to literals at generate time. The run step confirms the generated stack plans cleanly while
// the unit's own inputs keep the dependency.* references for the unit run.
func TestStackDepsMockLocalResolvesLocal(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsMockLocal)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsMockLocal)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsMockLocal)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	generated, err := os.ReadFile(filepath.Join(rootPath, inthclparse.StackDir, "iam", inthclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)

	// Dependency path: config_path = unit.account.path resolves to the sibling unit at generate time.
	assert.Contains(t, content, `"../account"`, "the dependency config_path (unit.<name>.path) must resolve at generate time")

	// Dependency mock outputs: stack-level locals are generate-time-knowable, so they resolve to literals here.
	assert.Contains(t, content, `"my-account"`, "a local in mock_outputs must be resolved at generate time")
	assert.Contains(t, content, `"eu-west-1"`, "a local in mock_outputs must be resolved at generate time")
	assert.NotContains(t, content, "local.account", "a stack-level local must not be left literal")
	assert.NotContains(t, content, "values.region", "values.* must not appear in the generated file")

	// The autoinclude only contributes the mock dependency; inputs live in the unit's own terragrunt.hcl.
	assert.NotContains(t, content, "inputs", "the generated autoinclude must contain only the mock dependency, not inputs")

	// End to end: the unit's own inputs consume the dependency mock outputs, so the stack must plan cleanly.
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "the generated stack must plan; stderr=%s", stderr)
	assert.NotContains(t, stderr, "no variable named", "the generated stack must reference no undefined variables")
}

// TestStackDepsAutoIncludeResolvesObjectKey verifies, end to end, that an interpolated object key in an autoinclude resolves at stack generate time even when the object's value defers to dependency.*, so no stack-level reference leaks into the generated unit.
func TestStackDepsAutoIncludeResolvesObjectKey(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncObjectKey)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncObjectKey)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncObjectKey)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	generated, err := os.ReadFile(filepath.Join(rootPath, inthclparse.StackDir, "app", inthclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(t, content, `"pre_key"`, "an interpolated object key must resolve at generate time")
	assert.NotContains(t, content, "local.prefix", "an interpolated object key must not leak a stack-level reference into the generated unit")
	assert.Contains(t, content, "dependency.vpc.outputs.id", "the dependency reference stays verbatim for the unit")
	assert.Contains(t, content, "pre_mock", "an interpolated key inside a dependency block attribute must resolve at generate time")

	// End to end: the generated unit must evaluate (a leaked stack reference would fail here), with the resolved key carrying the mocked dependency output through to the unit's planned outputs.
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "the generated stack must plan; stderr=%s", stderr)
	assert.Contains(t, stdout, "pre_key=mock-vpc-id",
		"the resolved object key must carry the mocked dependency output into the unit inputs")
}

// TestStackDepsAutoIncludeResolvesValuesReference verifies that a values.* reference inside an autoinclude resolves
// against the stack file's terragrunt.values.hcl at stack generate time, baking the literal into the generated unit.
func TestStackDepsAutoIncludeResolvesValuesReference(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncValuesResolved)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncValuesResolved)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncValuesResolved)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	generated, err := os.ReadFile(filepath.Join(rootPath, inthclparse.StackDir, "app", inthclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(t, content, `"us-east-1"`, "values.* must resolve to its literal from the stack values at generate time")
	assert.NotContains(t, content, "values.region", "values.* must not be left verbatim in the generated unit")
	// A directory function resolves in the stack file's context at generate time: baked to a literal (not left
	// verbatim) and pointing at the stack directory, not the generated unit's .terragrunt-stack subdirectory.
	assert.NotContains(t, content, "get_terragrunt_dir(", "a directory function must resolve at generate time, not stay verbatim")
	assert.NotContains(t, content, inthclparse.StackDir, "a directory function must resolve in the stack file's context, not the generated unit's directory")
}

// TestStackDepsAutoIncludeFunctionsAndDeps covers, end to end, how an autoinclude treats functions and dependencies:
// a function call with no dependency.* reference (read_terragrunt_config in config_path, run_cmd in inputs) resolves
// in the stack file context at generate time, while a dependency.*.outputs.* reference stays verbatim and resolves
// inside the generated unit. The mock feeds the deferred dependency output at plan time.
func TestStackDepsAutoIncludeFunctionsAndDeps(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncFuncs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncFuncs)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncFuncs)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	generated, err := os.ReadFile(filepath.Join(rootPath, inthclparse.StackDir, "app", inthclparse.AutoIncludeFile))
	require.NoError(t, err)

	content := string(generated)

	// read_terragrunt_config in config_path is a generate-time function: it is evaluated in the stack file
	// context, so config_path is the resolved sibling path, not the function call.
	assert.Contains(t, content, `"../data"`, "config_path from read_terragrunt_config must resolve at generate time")
	assert.NotContains(t, content, "read_terragrunt_config", "read_terragrunt_config in config_path must be evaluated at generate, not deferred")

	// In inputs, a function call with no dependency.* reference resolves at generate time, while a dependency
	// output reference stays verbatim for unit-time evaluation.
	assert.Contains(t, content, `"hi-from-unit"`, "a function call with no dependency reference must resolve at generate time")
	assert.NotContains(t, content, "run_cmd(", "a resolvable function in inputs must not be left verbatim")
	assert.Contains(t, content, "dependency.data.outputs.value", "a dependency output in inputs must stay verbatim")

	// End to end: the dependency mock feeds the deferred input, the generate-time run_cmd result is baked in, and the stack plans cleanly.
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "the generated stack must plan; stderr=%s", stderr)
	assert.Contains(t, stdout, "mock-data:hi-from-unit",
		"the dependency mock and the generate-time run_cmd result must both feed the unit inputs")
}

func TestStackDepsAutoIncludeSymlink(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoInclude)
	liveDir := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoInclude, "live")

	symlinkDir := filepath.Join(tmpEnvPath, "symlinked-live")
	require.NoError(t, os.Symlink(liveDir, symlinkDir))

	stackFile := filepath.Join(symlinkDir, "terragrunt.stack.hcl")

	srcBytes, err := os.ReadFile(stackFile)
	require.NoError(t, err)

	result, err := inthclparse.ParseStackFile(vfs.NewOSFS(), &inthclparse.ParseStackFileInput{
		Src:      srcBytes,
		Filename: stackFile,
		StackDir: symlinkDir,
	})
	require.NoError(t, err)
	require.Len(t, result.Units, 2)

	resolved, ok := result.AutoIncludes[inthclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)
	require.Len(t, resolved.Dependencies, 1)
}

// TestStackDepsDAGWithoutAutoInclude verifies that units without autoinclude
// files return no extra dependency paths.
func TestStackDepsDAGWithoutAutoInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoInclude)
	liveDir := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoInclude, "live")

	stackFile := filepath.Join(liveDir, "terragrunt.stack.hcl")

	srcBytes, err := os.ReadFile(stackFile)
	require.NoError(t, err)

	result, err := inthclparse.ParseStackFile(vfs.NewOSFS(), &inthclparse.ParseStackFileInput{
		Src:      srcBytes,
		Filename: stackFile,
		StackDir: liveDir,
	})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[inthclparse.AutoIncludeKey("unit", "app")]
	require.True(t, ok)

	appDir := filepath.Join(liveDir, inthclparse.StackDir, "app")

	err = inthclparse.GenerateAutoIncludeFile(vfs.NewOSFS(), resolved, appDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	// vpc unit has no autoinclude: should return no deps
	vpcDir := filepath.Join(liveDir, inthclparse.StackDir, "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0755))

	vpcDeps, vpcErr := inthclparse.AutoIncludeDependencyPaths(vfs.NewOSFS(), vpcDir)
	require.NoError(t, vpcErr)
	assert.Empty(t, vpcDeps)
}

// TestStackDepsDAGExpandsStackToUnits verifies that when a dependency
// points to a stack directory, the stack is expanded to constituent unit paths.
func TestStackDepsDAGExpandsStackToUnits(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsStackRef)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsStackRef)
	liveDir := filepath.Join(tmpEnvPath, testFixtureStackDepsStackRef, "live")
	liveDir, err := filepath.EvalSymlinks(liveDir)
	require.NoError(t, err)

	stackFile := filepath.Join(liveDir, "terragrunt.stack.hcl")

	srcBytes, err := os.ReadFile(stackFile)
	require.NoError(t, err)

	result, err := inthclparse.ParseStackFile(vfs.NewOSFS(), &inthclparse.ParseStackFileInput{
		Src:      srcBytes,
		Filename: stackFile,
		StackDir: liveDir,
	})
	require.NoError(t, err)

	resolved, ok := result.AutoIncludes[inthclparse.AutoIncludeKey("unit", "app_stack_dep")]
	require.True(t, ok)

	appDir := filepath.Join(liveDir, inthclparse.StackDir, "app-stack-dep")

	err = inthclparse.GenerateAutoIncludeFile(vfs.NewOSFS(), resolved, appDir, srcBytes, resolved.EvalCtx)
	require.NoError(t, err)

	depPaths, depErr := inthclparse.AutoIncludeDependencyPaths(vfs.NewOSFS(), appDir)
	require.NoError(t, depErr)
	require.Len(t, depPaths, 1)

	stackDir := depPaths[0]

	nestedStackSrc := filepath.Join(tmpEnvPath, testFixtureStackDepsStackRef, "catalog", "stacks", "networking", "terragrunt.stack.hcl")

	require.NoError(t, os.MkdirAll(stackDir, 0755))

	nestedContent, err := os.ReadFile(nestedStackSrc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), nestedContent, 0644))

	l := logger.CreateLogger()
	ctx, pctx := configbridge.NewParsingContext(t.Context(), l, options.NewTerragruntOptions())

	unitPaths, err := inthclparse.UnitPathsFromStackDir(vfs.NewOSFS(), stackDir, stackDepsFuncsFor(ctx, l, pctx))
	require.NoError(t, err)
	require.Len(t, unitPaths, 2, "networking stack should expand to 2 unit paths")

	expectedVPC := filepath.Join(stackDir, inthclparse.StackDir, "vpc")
	expectedSubnets := filepath.Join(stackDir, inthclparse.StackDir, "subnets")

	assert.Contains(t, unitPaths, expectedVPC)
	assert.Contains(t, unitPaths, expectedSubnets)
}

// TestStackDepsUnitPathsFromNestedOnlyStack expands a stack whose file declares only nested stacks and expects the nested units to be returned.
func TestStackDepsUnitPathsFromNestedOnlyStack(t *testing.T) {
	t.Parallel()

	root := helpers.TmpDirWOSymlinks(t)

	// A generated parent stack dir whose stack file declares only a nested stack.
	writeStackDepsFile(t, root, "terragrunt.stack.hcl", `stack "more" {
  source = "./more"
  path   = "more"
}
`)

	// The nested stack, one level deeper, holds the only real unit.
	writeStackDepsFile(t, root, filepath.Join(inthclparse.StackDir, "more", "terragrunt.stack.hcl"), `unit "deep" {
  source = "./deep"
  path   = "deep"
}
`)

	l := logger.CreateLogger()
	ctx, pctx := configbridge.NewParsingContext(t.Context(), l, options.NewTerragruntOptions())

	unitPaths, err := inthclparse.UnitPathsFromStackDir(vfs.NewOSFS(), root, stackDepsFuncsFor(ctx, l, pctx))
	require.NoError(t, err)

	deep := filepath.Join(root, inthclparse.StackDir, "more", inthclparse.StackDir, "deep")
	// The fixture has exactly one real unit; assert the full slice so DAG over-expansion fails the test.
	assert.ElementsMatch(t, []string{deep}, unitPaths, "a stack-of-stacks dependency must expand to exactly the nested units")
}

// TestStackDepsUnitPathsFromMissingStackFile returns no paths and no error when the directory has no stack file.
func TestStackDepsUnitPathsFromMissingStackFile(t *testing.T) {
	t.Parallel()

	root := helpers.TmpDirWOSymlinks(t)

	l := logger.CreateLogger()
	ctx, pctx := configbridge.NewParsingContext(t.Context(), l, options.NewTerragruntOptions())

	unitPaths, err := inthclparse.UnitPathsFromStackDir(vfs.NewOSFS(), root, stackDepsFuncsFor(ctx, l, pctx))
	require.NoError(t, err)
	assert.Empty(t, unitPaths, "a directory without a stack file expands to no unit paths")
}

// TestStackDepsE2EBasic runs the full end-to-end flow with 2 units:
// stack generate -> run --all apply -> verify outputs -> run --all destroy.
func TestStackDepsE2EBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsBasic, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	autoIncludePath := filepath.Join(rootPath, inthclparse.StackDir, "unit-w-inputs", "terragrunt.autoinclude.hcl")
	require.FileExists(t, autoIncludePath)

	content, err := os.ReadFile(autoIncludePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `dependency "unit_w_outputs"`)
	assert.Contains(t, string(content), "../unit-w-outputs")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	inputPath := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "unit-w-inputs"), "input.txt")

	inputContent, err := os.ReadFile(inputPath)
	require.NoError(t, err)
	assert.Equal(t, "Received: Hello!", string(inputContent))

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
}

// TestStackDepsRemoteStateDependency pins that a dependency output referenced
// inside a remote_state config (the dependency block lives only in the
// generated autoinclude) resolves during run --all plan instead of failing
// with an unknown "dependency" variable.
func TestStackDepsRemoteStateDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsRemoteStateDep)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsRemoteStateDep)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsRemoteStateDep, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "run --all plan must resolve dependency output inside remote_state; stderr=%s", stderr)

	// The mock output fake-val must resolve inside the generated backend, producing the key fake-val.tfstate.
	backendPath := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "unit-w-inputs"), "backend.tf")
	backend, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Contains(t, string(backend), "fake-val.tfstate",
		"dependency output must resolve inside remote_state from the autoinclude mock")
}

// TestStackDepsNestedRemoteStateDependency covers a nested stack-of-stacks
// (stacks -> sandbox-1 -> roles) where the roles unit references an autoinclude-injected
// dependency output in both a remote_state block and a generate block, driven by
// `terragrunt stack run plan`.
func TestStackDepsNestedRemoteStateDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsNestedRemoteStateDep)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsNestedRemoteStateDep)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsNestedRemoteStateDep)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "stacks")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack run plan --non-interactive --working-dir "+rootPath)
	require.NoError(t, err, "stack run plan must resolve the autoinclude dependency in remote_state; stderr=%s", stderr)

	// The mock account name must resolve inside the nested roles unit's generated backend.
	rolesDir := filepath.Join(rootPath, inthclparse.StackDir, "sandbox-1", inthclparse.StackDir, "roles_hcl")
	backendPath := helpers.FindCachedFile(t, rolesDir, "backend.tf")
	backend, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Contains(t, string(backend), "mock-account/roles.tfstate",
		"dependency output must resolve inside remote_state in a nested stack")
}

// TestStackDepsNestedUnitAutoIncludeDependency covers a nested stack whose unit autoinclude depends on
// a sibling unit via unit.X.path, with the dependency output consumed through inputs. The generated
// dependency config_path must account for the nested .terragrunt-stack directory (../data, not one
// level too high), and run --all plan must resolve it.
func TestStackDepsNestedUnitAutoIncludeDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsNestedUnitDep)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsNestedUnitDep)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsNestedUnitDep)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	// vpc is generated at core/.terragrunt-stack/vpc and data at core/.terragrunt-stack/data, so the
	// dependency must resolve to ../data through the nested .terragrunt-stack directory.
	vpcDir := filepath.Join(rootPath, inthclparse.StackDir, "core", inthclparse.StackDir, "vpc")
	autoInclude, err := os.ReadFile(filepath.Join(vpcDir, inthclparse.AutoIncludeFile))
	require.NoError(t, err)
	assert.Contains(t, string(autoInclude), `"../data"`,
		"the nested-stack dependency must resolve to the sibling unit through .terragrunt-stack, not one level too high")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "a nested-stack unit autoinclude dependency must resolve at run time; stderr=%s", stderr)
	assert.NotContains(t, stderr, "does not contain a terragrunt.hcl",
		"the dependency path must include the nested .terragrunt-stack segment")
}

// TestStackDepsAutoIncludeOverridesUnitDependency covers the same-name dependency conflict case:
// when a unit declares its own dependency block AND the autoinclude declares a dependency
// of the same name, the autoinclude block wins by name (shallow merge, like a default include),
// so dependency.x.outputs.v resolves to the autoinclude's mock value, not the unit's.
func TestStackDepsAutoIncludeOverridesUnitDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsDupDependency)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsDupDependency)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsDupDependency, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "autoinclude dependency must override the unit's same-name block; stderr=%s", stderr)

	backendPath := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "y"), "backend.tf")
	backend, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Contains(t, string(backend), "from-autoinclude.tfstate",
		"autoinclude dependency (by name) must override the unit's own dependency path")
}

// TestStackDepsAutoIncludeReplacesUnitDependency verifies the shallow-merge contract for a same-name
// dependency conflict: when a unit and its autoinclude both declare dependency "x", the autoinclude
// block REPLACES the unit's wholesale (it is not deep-merged). So the autoinclude's mock outputs are the
// ones that resolve, the conflicting "common" key takes the autoinclude value, and the unit-only key
// (from_unit) no longer exists, exactly as a default include behaves.
func TestStackDepsAutoIncludeReplacesUnitDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsDepMockMerge)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsDepMockMerge)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsDepMockMerge, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "the autoinclude dependency must replace the unit's same-name block (shallow); stderr=%s", stderr)

	// Backend key is "absent-autoinclude-common": the autoinclude's block fully replaced the unit's, so
	// from_unit no longer exists (try() falls back to "absent") and the conflicting "common" key resolves
	// to the autoinclude value. A deep merge would keep from_unit and yield "unitval-autoinclude-common",
	// so this assertion fails if foldSiblingAutoIncludeDeps reverts to a deep merge (both still plan).
	backendPath := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "y"), "backend.tf")
	backend, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Contains(t, string(backend), "absent-autoinclude-common.tfstate",
		"the autoinclude dependency must replace the unit's same-name block, not deep-merge it")
}

// TestStackDepsAutoIncludeDisabledDependencyCreatesNoEdge is a regression test: a dependency declared
// in an autoinclude with enabled = false must not become a run-DAG edge. The disabled dependency points
// at a nonexistent path, so a run that followed it would fail with a missing terragrunt.hcl error. The
// partial-parse merge drops disabled blocks, and discovery must not re-add them from the raw autoinclude.
func TestStackDepsAutoIncludeDisabledDependencyCreatesNoEdge(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsDisabledAutoIncDep)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsDisabledAutoIncDep)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsDisabledAutoIncDep, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "a disabled autoinclude dependency must not create a run-DAG edge to its nonexistent path; stderr=%s", stderr)
	assert.NotContains(t, stderr, "nonexistent-in-tree", "the disabled dependency path must not enter the run graph")
}

// TestStackDepsAutoIncludeDependencyAppliesWithoutMockOutputs verifies that run --all apply succeeds and
// reads the dependency's real output when an autoinclude-injected dependency defines no mock_outputs: the
// run queue applies the dependency first, so the dependent never needs a mock at apply time.
func TestStackDepsAutoIncludeDependencyAppliesWithoutMockOutputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsApplyNoMocks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsApplyNoMocks)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsApplyNoMocks, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")
	require.NoError(t, err, "run --all apply must apply the dependency first and read its real output even with no mock_outputs; stderr=%s", stderr)

	// The consumer marker must hold the producer's REAL output (no mock exists), proving the queue applied
	// the producer first and the dependent read live state rather than failing on a missing output.
	marker := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "consumer"), "marker.txt")

	content, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, "consumer received: real-producer-output", string(content))
}

// TestStackDepsE2EChain runs a 3-level dependency chain end-to-end:
// unit_a -> unit_b -> unit_c
// Verifies chained output propagation and correct apply/destroy ordering.
func TestStackDepsE2EChain(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsChain)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsChain)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsChain, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	// Verify autoinclude generated for unit-b and unit-a but not unit-c
	require.FileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "unit-b", "terragrunt.autoinclude.hcl"))
	require.FileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "unit-a", "terragrunt.autoinclude.hcl"))
	assert.NoFileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "unit-c", "terragrunt.autoinclude.hcl"))

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	// Verify unit-a received chained output: from-b(from-c)
	markerA := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "unit-a"), "marker.txt")

	contentA, err := os.ReadFile(markerA)
	require.NoError(t, err)
	assert.Equal(t, "unit-a received: from-b(from-c)", string(contentA))

	// Verify unit-b received: from-c
	markerB := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "unit-b"), "marker.txt")

	contentB, err := os.ReadFile(markerB)
	require.NoError(t, err)
	assert.Equal(t, "unit-b received: from-c", string(contentB))

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")

	// Destroy must remove the marker files produced by apply.
	assert.NoFileExists(t, markerA)
	assert.NoFileExists(t, markerB)
}

// TestStackDepsE2ECrossStack tests stack generation with cross-stack dependencies:
// a "network" stack (containing vpc + subnets) and an "app" unit depending on
// the entire network stack via stack.network.path.
func TestStackDepsE2ECrossStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsCrossStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsCrossStack)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsCrossStack)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	err = runner.WithWorkDir(gitPath).Init(t.Context())
	require.NoError(t, err)

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	autoIncludePath := filepath.Join(rootPath, inthclparse.StackDir, "app", "terragrunt.autoinclude.hcl")
	require.FileExists(t, autoIncludePath)

	content, err := os.ReadFile(autoIncludePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `dependency "network"`)
	assert.Contains(t, string(content), "../network")

	// Verify network stack units were generated
	networkStackDir := filepath.Join(rootPath, inthclparse.StackDir, "network", inthclparse.StackDir)
	assert.DirExists(t, filepath.Join(networkStackDir, "vpc"))
	assert.DirExists(t, filepath.Join(networkStackDir, "subnets"))

	// Verify DAG sees the dependency
	appDir := filepath.Join(rootPath, inthclparse.StackDir, "app")
	depPaths, depErr := inthclparse.AutoIncludeDependencyPaths(vfs.NewOSFS(), appDir)
	require.NoError(t, depErr)
	require.Len(t, depPaths, 1)
	assert.Equal(t, filepath.Join(rootPath, inthclparse.StackDir, "network"), depPaths[0])

	// Apply the whole tree: the network stack's units run first, then app consumes the
	// real aggregated output dependency.network.outputs.vpc.vpc_id (not the mock).
	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	markerPath := helpers.FindCachedFile(t, appDir, "marker.txt")
	markerContent, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	assert.Equal(t, "app received: vpc-cross-stack", string(markerContent),
		"app must receive the network stack's real vpc output, not the mock")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
}

// TestStackDepsStackValuesInLocals pins that run-queue expansion of a stack-dir
// dependency resolves values.* referenced in the target stack's locals from the
// generated terragrunt.values.hcl sitting next to the generated terragrunt.stack.hcl,
// instead of failing with an unknown "values" variable (gruntwork-io/terragrunt#5663).
func TestStackDepsStackValuesInLocals(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsStackValuesLocals)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsStackValuesLocals)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsStackValuesLocals)

	// The child stack uses get_repo_root() for unit sources, so the fixture copy must be a git repo.
	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	err = runner.WithWorkDir(gitPath).Init(t.Context())
	require.NoError(t, err)

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	// Generation wrote the child stack's values file and its locals consumed it: the
	// generated unit dir carries the env prefix from values.env.
	networkDir := filepath.Join(rootPath, inthclparse.StackDir, "network")
	require.FileExists(t, filepath.Join(networkDir, "terragrunt.values.hcl"))
	require.DirExists(t, filepath.Join(networkDir, inthclparse.StackDir, "dev-vpc"))

	// Run-queue expansion of app's stack-dir dependency re-evaluates the child stack's
	// locals; it must load the sibling values file rather than fail on values.env.
	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
}

// Regression: a non-literal expression (here, format()) in an unrelated unit must not block autoinclude resolution. Generation succeeds and the autoinclude file is produced for the unit that declares it.
func TestStackDepsAutoIncludeWithFormatInOtherUnit(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncParserLimit)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncParserLimit)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncParserLimit, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+rootPath)

	require.NoError(t, err, "non-literal expressions in unrelated units must not block autoinclude generation")
	require.FileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "subnet", inthclparse.AutoIncludeFile))
}

// Regression: a unit using `values.X` references in a sibling unit must not block autoinclude generation on a different unit.
func TestStackDepsAutoIncludeWithValuesRefInOtherUnit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	stackDir := filepath.Join(tmpDir, "live")
	require.NoError(t, os.MkdirAll(stackDir, 0755))

	for _, unitName := range []string{"account", "idp", "roles"} {
		unitDir := filepath.Join(tmpDir, "catalog", "units", unitName)
		require.NoError(t, os.MkdirAll(unitDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte("inputs = {}\n"), 0644))
	}

	valuesPath := filepath.Join(stackDir, "terragrunt.values.hcl")
	require.NoError(t, os.WriteFile(valuesPath, []byte(`
account = "acme"
region  = "us-east-1"
idps    = ["okta"]
roles   = ["admin"]
`), 0644))

	stackHCL := `
unit "account" {
  source = "../catalog/units/account"
  path   = "account_hcl"
  values = {
    account = values.account
    region  = values.region
  }
}

unit "idps" {
  source = "../catalog/units/idp"
  path   = "idps_hcl"
  values = {
    identityProviders = values.idps
    region            = values.region
  }
}

unit "roles" {
  source = "../catalog/units/roles"
  path   = "roles_hcl"
  values = {
    roles  = values.roles
    region = values.region
  }
  autoinclude {
    dependency "account" {
      config_path = unit.account.path
    }
  }
}
`
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackHCL), 0644))

	_, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+stackDir)
	require.NoError(t, err, "values references in unrelated units must not block autoinclude generation on roles unit")

	require.FileExists(t, filepath.Join(stackDir, inthclparse.StackDir, "roles_hcl", inthclparse.AutoIncludeFile))
	require.NoFileExists(t, filepath.Join(stackDir, inthclparse.StackDir, "account_hcl", inthclparse.AutoIncludeFile))
	require.NoFileExists(t, filepath.Join(stackDir, inthclparse.StackDir, "idps_hcl", inthclparse.AutoIncludeFile))
}

// Companion contract: parser-incompatible HCL without an autoinclude block still generates successfully (silent skip is allowed only when the user has nothing to lose).
func TestStackDepsParserLimitOKWithoutAutoInclude(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	stackDir := filepath.Join(tmpDir, "live")
	require.NoError(t, os.MkdirAll(stackDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "catalog", "units", "vpc"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "catalog", "units", "vpc", "terragrunt.hcl"), []byte("inputs = {}\n"), 0644))

	stackHCL := `
unit "vpc" {
  source = "../catalog/units/vpc"
  path   = format("%s", "vpc")
}
`
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackHCL), 0644))

	_, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+stackDir)
	require.NoError(t, err, "stack generate must succeed without autoinclude even if production-only expressions appear in source/path attributes")

	require.DirExists(t, filepath.Join(stackDir, inthclparse.StackDir, "vpc"))
	require.NoFileExists(t, filepath.Join(stackDir, inthclparse.StackDir, "vpc", "terragrunt.autoinclude.hcl"))
}

// Root has only an `include` block (literal path); autoinclude lives in the included file along with non-literal HCL expressions in unrelated units. Generation must succeed.
func TestStackDepsAutoIncludePassesViaInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncViaInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncViaInclude)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncViaInclude, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+rootPath)

	require.NoError(t, err, "autoinclude in an included file must succeed even when the included file contains non-literal expressions in other units")
	require.FileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "subnet", inthclparse.AutoIncludeFile))
}

// Root's include.path is an HCL expression (format()) that the stack-dependencies parser must resolve with the production function context. The included file declares autoinclude, so generation must succeed.
func TestStackDepsAutoIncludePassesViaDynamicInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncViaDynInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncViaDynInclude)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncViaDynInclude, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+rootPath)

	require.NoError(t, err, "autoinclude reachable via an expression-based include path must succeed")
	require.FileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "subnet", inthclparse.AutoIncludeFile))
}

// Root has only an `include`; the included file declares parser-compatible autoinclude. Generation must use the included file's bytes when slicing expressions, otherwise mock_outputs/inputs come out garbled or empty.
func TestStackDepsAutoIncludeViaIncludePreservesContent(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncViaIncludeSuccess)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncViaIncludeSuccess)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncViaIncludeSuccess, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+rootPath)
	require.NoError(t, err)

	autoIncludePath := filepath.Join(rootPath, inthclparse.StackDir, "subnet", inthclparse.AutoIncludeFile)
	require.FileExists(t, autoIncludePath)

	generated, err := os.ReadFile(autoIncludePath)
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(t, content, `dependency "vpc"`)
	assert.Contains(t, content, "mock_outputs_allowed_terraform_commands")
	assert.Contains(t, content, "shared-mock-id")
	assert.Contains(t, content, "dependency.vpc.outputs.id")
}

// findComponentTypeUnit is the find --json component type for a unit.
const findComponentTypeUnit = "unit"

// findComponent is the JSON structure returned by terragrunt find --json.
type findComponent struct {
	Type         string   `json:"type"`
	Path         string   `json:"path"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// TestStackDepsFindJSON verifies that terragrunt find --json --dag --dependencies
// correctly shows stack dependency relationships from autoinclude.
func TestStackDepsFindJSON(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsBasic, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt find --json --dag --dependencies --working-dir "+rootPath)
	require.NoError(t, err)

	var components []findComponent

	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	// Find the unit-w-inputs component and verify its dependency
	foundInputs := false
	foundOutputs := false

	for _, c := range components {
		if c.Type != findComponentTypeUnit {
			continue
		}

		if filepath.Base(c.Path) == "unit-w-inputs" {
			foundInputs = true

			require.Len(t, c.Dependencies, 1)
			assert.Contains(t, c.Dependencies[0], "unit-w-outputs")
		}

		if filepath.Base(c.Path) == "unit-w-outputs" {
			foundOutputs = true

			assert.Empty(t, c.Dependencies)
		}
	}

	require.True(t, foundInputs, "unit-w-inputs should be in find output")
	require.True(t, foundOutputs, "unit-w-outputs should be in find output")
}

// TestStackDepsFindDAG verifies that terragrunt find --dag lists units in
// dependency order: unit-w-outputs before unit-w-inputs.
func TestStackDepsFindDAG(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsBasic, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt find --dag --working-dir "+rootPath)
	require.NoError(t, err)

	outputsIdx := strings.Index(stdout, "unit-w-outputs")
	inputsIdx := strings.Index(stdout, "unit-w-inputs")

	require.NotEqual(t, -1, outputsIdx, "unit-w-outputs should be in output")
	require.NotEqual(t, -1, inputsIdx, "unit-w-inputs should be in output")
	assert.Less(t, outputsIdx, inputsIdx, "unit-w-outputs should appear before unit-w-inputs in DAG order")
}

// TestStackDepsListLong verifies that terragrunt list --long --dependencies --dag
// shows dependency columns with stack dependency relationships.
func TestStackDepsListLong(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsBasic, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt list --long --dependencies --dag --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Dependencies")

	found := false

	for line := range strings.SplitSeq(stdout, "\n") {
		if !strings.Contains(line, "unit-w-inputs") {
			continue
		}

		found = true

		assert.Contains(t, line, "unit-w-outputs",
			"unit-w-inputs row should show unit-w-outputs as dependency")
	}

	require.True(t, found, "expected a list row mentioning unit-w-inputs")
}

// TestStackDepsListTree verifies that terragrunt list --tree --dag
// shows a tree structure reflecting dependency relationships.
func TestStackDepsListTree(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsBasic, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt list --tree --dag --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "unit-w-outputs")
	assert.Contains(t, stdout, "unit-w-inputs")

	outputsIdx := strings.Index(stdout, "unit-w-outputs")
	inputsIdx := strings.Index(stdout, "unit-w-inputs")

	assert.Less(t, outputsIdx, inputsIdx)
}

// TestStackDepsFindChain verifies find --json --dag --dependencies with a
// 3-level dependency chain: unit_a -> unit_b -> unit_c.
func TestStackDepsFindChain(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsChain)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsChain)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsChain, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt find --json --dag --dependencies --working-dir "+rootPath)
	require.NoError(t, err)

	var components []findComponent

	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	depsByPath := make(map[string][]string)

	for _, c := range components {
		depsByPath[c.Path] = c.Dependencies
	}

	var unitAPath, unitBPath, unitCPath string

	for path := range depsByPath {
		switch {
		case filepath.Base(path) == "unit-a":
			unitAPath = path
		case filepath.Base(path) == "unit-b":
			unitBPath = path
		case filepath.Base(path) == "unit-c":
			unitCPath = path
		}
	}

	require.NotEmpty(t, unitAPath, "unit-a should be in output")
	require.NotEmpty(t, unitBPath, "unit-b should be in output")
	require.NotEmpty(t, unitCPath, "unit-c should be in output")

	require.Len(t, depsByPath[unitAPath], 1)
	assert.Equal(t, unitBPath, depsByPath[unitAPath][0])

	require.Len(t, depsByPath[unitBPath], 1)
	assert.Equal(t, unitCPath, depsByPath[unitBPath][0])

	assert.Empty(t, depsByPath[unitCPath])
}

// TestStackDepsFindTree verifies find --json --dag --dependencies with a
// multi-level dependency tree:
//
//	    A
//	   / \
//	  B   C
//	 / \
//	D   E
//
// D, E, C are leaves. B depends on D+E. A depends on B+C.
func TestStackDepsFindTree(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsTree)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsTree)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsTree, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt find --json --dag --dependencies --working-dir "+rootPath)
	require.NoError(t, err)

	var components []findComponent

	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	depsByPath := make(map[string][]string)

	for _, c := range components {
		depsByPath[c.Path] = c.Dependencies
	}

	var aPath, bPath, cPath, dPath, ePath string

	for path := range depsByPath {
		switch {
		case filepath.Base(path) == "unit-a":
			aPath = path
		case filepath.Base(path) == "unit-b":
			bPath = path
		case filepath.Base(path) == "unit-c":
			cPath = path
		case filepath.Base(path) == "unit-d":
			dPath = path
		case filepath.Base(path) == "unit-e":
			ePath = path
		}
	}

	require.NotEmpty(t, aPath, "unit-a should be in output")
	require.NotEmpty(t, bPath, "unit-b should be in output")
	require.NotEmpty(t, cPath, "unit-c should be in output")
	require.NotEmpty(t, dPath, "unit-d should be in output")
	require.NotEmpty(t, ePath, "unit-e should be in output")

	// A depends on B and C
	require.Len(t, depsByPath[aPath], 2)
	assert.Contains(t, depsByPath[aPath], bPath)
	assert.Contains(t, depsByPath[aPath], cPath)

	// B depends on D and E
	require.Len(t, depsByPath[bPath], 2)
	assert.Contains(t, depsByPath[bPath], dPath)
	assert.Contains(t, depsByPath[bPath], ePath)

	// C, D, E are leaves
	assert.Empty(t, depsByPath[cPath])
	assert.Empty(t, depsByPath[dPath])
	assert.Empty(t, depsByPath[ePath])
}

// TestStackDepsFindTreeDAGOrder verifies that find --dag with a multi-level
// tree outputs units in correct topological order: leaves before parents.
func TestStackDepsFindTreeDAGOrder(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsTree)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsTree)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsTree, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt find --dag --working-dir "+rootPath)
	require.NoError(t, err)

	idxA := strings.Index(stdout, "unit-a")
	idxB := strings.Index(stdout, "unit-b")
	idxC := strings.Index(stdout, "unit-c")
	idxD := strings.Index(stdout, "unit-d")
	idxE := strings.Index(stdout, "unit-e")

	require.NotEqual(t, -1, idxA)
	require.NotEqual(t, -1, idxB)
	require.NotEqual(t, -1, idxC)
	require.NotEqual(t, -1, idxD)
	require.NotEqual(t, -1, idxE)

	// D and E must appear before B (B depends on D+E)
	assert.Less(t, idxD, idxB, "unit-d should appear before unit-b")
	assert.Less(t, idxE, idxB, "unit-e should appear before unit-b")

	// B and C must appear before A (A depends on B+C)
	assert.Less(t, idxB, idxA, "unit-b should appear before unit-a")
	assert.Less(t, idxC, idxA, "unit-c should appear before unit-a")
}

// TestStackDepsE2ETree runs apply/destroy on the multi-level dependency tree
// and verifies output propagation through all levels.
func TestStackDepsE2ETree(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsTree)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsTree)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsTree, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	// Verify unit-b received outputs from D and E
	markerB := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "unit-b"), "marker.txt")

	contentB, err := os.ReadFile(markerB)
	require.NoError(t, err)
	assert.Equal(t, "unit-b(from-d,from-e)", string(contentB))

	// Verify unit-a received outputs from B and C
	markerA := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "unit-a"), "marker.txt")

	contentA, err := os.ReadFile(markerA)
	require.NoError(t, err)
	assert.Equal(t, "unit-a(from-b(from-d,from-e),from-c)", string(contentA))

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")

	// Destroy must remove the marker files produced by apply.
	assert.NoFileExists(t, markerA)
	assert.NoFileExists(t, markerB)
}

// TestStackDepsAutoIncludeWithLocalRefInOtherUnit pins that local.* references in unrelated units do not block autoinclude generation.
func TestStackDepsAutoIncludeWithLocalRefInOtherUnit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	stackDir := filepath.Join(tmpDir, "live")
	require.NoError(t, os.MkdirAll(stackDir, 0755))

	for _, unitName := range []string{"a", "b"} {
		unitDir := filepath.Join(tmpDir, "catalog", "units", unitName)
		require.NoError(t, os.MkdirAll(unitDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte("inputs = {}\n"), 0644))
	}

	stackHCL := `
locals {
  region = "us-east-1"
}

unit "a" {
  source = "../catalog/units/a"
  path   = "a"
  values = {
    region = local.region
  }
}

unit "b" {
  source = "../catalog/units/b"
  path   = "b"
  autoinclude {
    dependency "a" {
      config_path = unit.a.path
    }
  }
}
`
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackHCL), 0644))

	_, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+stackDir)
	require.NoError(t, err, "local.* references in unrelated units must not block autoinclude generation")

	require.FileExists(t, filepath.Join(stackDir, inthclparse.StackDir, "b", inthclparse.AutoIncludeFile))
}

// TestStackDepsAutoIncludeWithFunctionInSource pins that terragrunt function calls (e.g. get_terragrunt_dir()) in the source attribute of an unrelated unit do not block autoinclude generation.
func TestStackDepsAutoIncludeWithFunctionInSource(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncComplexSiblings)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncComplexSiblings)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncComplexSiblings, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+rootPath)
	require.NoError(t, err, "get_terragrunt_dir() in source plus local.* and values.* references in unrelated units must not block autoinclude generation")

	// Autoinclude must be generated only on the unit that declared it.
	require.FileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "roles", inthclparse.AutoIncludeFile))
	require.NoFileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "account", inthclparse.AutoIncludeFile))
	require.NoFileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "idps", inthclparse.AutoIncludeFile))
}

// TestStackDepsE2EAutoIncludeWithComplexSiblings is the end-to-end regression for stacks whose unrelated units use every HCL feature class that previously broke the simplified parser; the roles unit must observe the account unit's output through the generated autoinclude.
func TestStackDepsE2EAutoIncludeWithComplexSiblings(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncComplexSiblings)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncComplexSiblings)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncComplexSiblings, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	autoIncludePath := filepath.Join(rootPath, inthclparse.StackDir, "roles", inthclparse.AutoIncludeFile)
	require.FileExists(t, autoIncludePath, "roles unit must have its autoinclude file generated")

	autoIncludeContent, err := os.ReadFile(autoIncludePath)
	require.NoError(t, err)
	assert.Contains(t, string(autoIncludeContent), `dependency "account"`)
	assert.Contains(t, string(autoIncludeContent), "../account")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	markerPath := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "roles"), "marker.txt")

	markerContent, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	assert.Equal(t, "roles-received: account-output", string(markerContent), "roles must receive account's output via the generated autoinclude dependency")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
}

// Regression: `run --all` discovery must walk a generated nested stack file even when its unit `source` attribute contains terragrunt function calls.
func TestStackDepsRunAllWithFunctionsInNestedStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsRunAllFuncsInNestedStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsRunAllFuncsInNestedStack)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsRunAllFuncsInNestedStack, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, runErr := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, runErr, "run --all must succeed when the generated nested stack file contains terragrunt function calls; stderr=%s", stderr)
	assert.NotContains(t, stderr, "Function calls not allowed",
		"discovery must not surface 'Function calls not allowed' on generated nested stack files")

	stdout, _, findErr := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt find --json --dag --dependencies --working-dir "+rootPath)
	require.NoError(t, findErr)

	var components []findComponent
	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	foundNestedVPC := false

	for _, c := range components {
		if c.Type == findComponentTypeUnit && filepath.Base(c.Path) == "vpc" && strings.Contains(c.Path, filepath.Join("networking", inthclparse.StackDir, "vpc")) {
			foundNestedVPC = true
			break
		}
	}

	require.True(t, foundNestedVPC, "generated nested stack unit vpc must be present in find output")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	nestedVPCGen := filepath.Join(rootPath, inthclparse.StackDir, "networking", inthclparse.StackDir, "vpc")
	vpcOutput, _, outputErr := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt output -json vpc_id --working-dir "+nestedVPCGen)
	require.NoError(t, outputErr, "nested vpc unit must be applied by run --all discovery")
	assert.Contains(t, vpcOutput, "vpc-from-nested-stack")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
}

// TestStackDepsNestedSameNameWithCAS covers a stack named "foo" containing a unit also named "foo": CAS must rewrite the relative source in the catalog stack file without name collisions, and an external sibling unit "bar" depends on the nested foo unit via the supported `${stack.foo.path}/.terragrunt-stack/foo` hand-computed path.
func TestStackDepsNestedSameNameWithCAS(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Experiment mode is not enabled")
	}

	catalog := helpers.TmpDirWOSymlinks(t)
	liveDir := helpers.TmpDirWOSymlinks(t)

	writeStackDepsFile(t, catalog, "units/foo/main.tf", `resource "local_file" "marker" {
  content  = "Hello from unit foo inside stack foo!"
  filename = "${path.module}/output.txt"
}

output "val" {
  value = "from-stack-foo-unit-foo"
}
`)
	writeStackDepsFile(t, catalog, "units/foo/terragrunt.hcl", ``)

	writeStackDepsFile(t, catalog, "units/bar/main.tf", `variable "val" {
  type        = string
  description = "Value received from the nested foo unit"
}

resource "local_file" "marker" {
  content  = "Received: ${var.val}"
  filename = "${path.module}/input.txt"
}

output "received_val" {
  value = var.val
}
`)
	writeStackDepsFile(t, catalog, "units/bar/terragrunt.hcl", ``)

	writeStackDepsFile(t, catalog, "stacks/foo/terragrunt.stack.hcl", `unit "foo" {
  source = "../..//units/foo"
  path   = "foo"

  update_source_with_cas = true
}
`)

	liveStack := fmt.Sprintf(`stack "foo" {
  source = %s
  path   = "foo"

  update_source_with_cas = true
}

unit "bar" {
  source = %s
  path   = "bar"

  update_source_with_cas = true

  autoinclude {
    dependency "foo_unit" {
      config_path = "${stack.foo.path}/.terragrunt-stack/foo"

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "fake-val"
      }
    }

    inputs = {
      val = dependency.foo_unit.outputs.val
    }
  }
}
`,
		strconv.Quote(filepath.ToSlash(catalog)+"//stacks/foo"),
		strconv.Quote(filepath.ToSlash(catalog)+"//units/bar"),
	)
	require.NoError(t, os.WriteFile(filepath.Join(liveDir, "terragrunt.stack.hcl"), []byte(liveStack), 0644))

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+liveDir)

	nestedFooStackFile := filepath.Join(liveDir, inthclparse.StackDir, "foo", "terragrunt.stack.hcl")
	require.FileExists(t, nestedFooStackFile)

	nestedFooUnitTfgFile := filepath.Join(liveDir, inthclparse.StackDir, "foo", inthclparse.StackDir, "foo", "terragrunt.hcl")
	require.FileExists(t, nestedFooUnitTfgFile)

	nestedFooStackContent, err := os.ReadFile(nestedFooStackFile)
	require.NoError(t, err)
	assert.Contains(t, string(nestedFooStackContent), "cas::", "nested stack file should reference CAS after generation")
	assert.NotContains(t, string(nestedFooStackContent), "../..//units/foo", "relative source must be rewritten by CAS")

	barAutoInclude := filepath.Join(liveDir, inthclparse.StackDir, "bar", inthclparse.AutoIncludeFile)
	require.FileExists(t, barAutoInclude)

	content, err := os.ReadFile(barAutoInclude)
	require.NoError(t, err)
	assert.Contains(t, string(content), `dependency "foo_unit"`)
	assert.Contains(t, string(content), "dependency.foo_unit.outputs.val")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+liveDir+" -- apply -auto-approve")

	barInputPath := helpers.FindCachedFile(t, filepath.Join(liveDir, inthclparse.StackDir, "bar"), "input.txt")
	barInputContent, err := os.ReadFile(barInputPath)
	require.NoError(t, err)
	assert.Equal(t, "Received: from-stack-foo-unit-foo", string(barInputContent))

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+liveDir+" -- destroy -auto-approve")
}

// TestStackDepsRelativeCatalogSourceRewrittenByCAS pins the supported flow for relative source paths inside catalog stack files: under the cas + stack-dependencies experiments, a nested catalog stack file using `source = "../..//units/foo"` is rewritten by CAS to a cas:: reference, so the generated copy under .terragrunt-stack is self-contained and does not require any sidecar metadata.
func TestStackDepsRelativeCatalogSourceRewrittenByCAS(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Experiment mode is not enabled")
	}

	catalog := helpers.TmpDirWOSymlinks(t)
	liveDir := helpers.TmpDirWOSymlinks(t)

	writeStackDepsFile(t, catalog, "units/baz/main.tf", `output "value" { value = "baz" }`)
	writeStackDepsFile(t, catalog, "units/baz/terragrunt.hcl", ``)

	writeStackDepsFile(t, catalog, "stacks/inner/terragrunt.stack.hcl", `unit "baz" {
  source = "../..//units/baz"
  path   = "baz"

  update_source_with_cas = true
}
`)

	liveStack := fmt.Sprintf(`stack "inner" {
  source = %s
  path   = "inner"

  update_source_with_cas = true
}
`, strconv.Quote(filepath.ToSlash(catalog)+"//stacks/inner"))
	require.NoError(t, os.WriteFile(filepath.Join(liveDir, "terragrunt.stack.hcl"), []byte(liveStack), 0644))

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+liveDir)

	nestedStackFile := filepath.Join(liveDir, inthclparse.StackDir, "inner", "terragrunt.stack.hcl")
	require.FileExists(t, nestedStackFile)

	// CAS must rewrite the relative source in the copied catalog stack file so the generated copy is self-contained.
	body, err := os.ReadFile(nestedStackFile)
	require.NoError(t, err)
	assert.Contains(t, string(body), "cas::", "nested stack file must reference CAS after generation")
	assert.NotContains(t, string(body), "../..//units/baz", "relative source must be rewritten by CAS")

	// No sidecar file should be written next to the copied stack.
	assert.NoFileExists(t, filepath.Join(liveDir, inthclparse.StackDir, "inner", ".terragrunt-stack-origin"),
		"sidecar must not be written; CAS is the supported source-resolution path")

	// And the nested unit must materialize under the recursion target.
	require.FileExists(t, filepath.Join(liveDir, inthclparse.StackDir, "inner", inthclparse.StackDir, "baz", "terragrunt.hcl"))
}

// writeStackDepsFile writes body to root/rel, creating parent dirs as needed; test fails on any error.
func writeStackDepsFile(t *testing.T, root, rel, body string) {
	t.Helper()

	full := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
	require.NoError(t, os.WriteFile(full, []byte(body), 0644))
}

// TestStackDepsNoDependenciesBaseline verifies the baseline case: the experiment
// is enabled and the stack has multiple units but no autoinclude. Generation must
// emit no autoinclude files and the stack must apply/destroy.
func TestStackDepsNoDependenciesBaseline(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsNoDeps)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsNoDeps)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsNoDeps, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	// No autoinclude declared anywhere: generation must not emit autoinclude files.
	assert.NoFileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "alpha", inthclparse.AutoIncludeFile))
	assert.NoFileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "beta", inthclparse.AutoIncludeFile))

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	require.FileExists(t, helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "alpha"), "marker.txt"))
	require.FileExists(t, helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "beta"), "marker.txt"))

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
}

// TestStackDepsAutoIncludeWinsOnConflict pins the documented merge precedence:
// when the unit's own terragrunt.hcl and the autoinclude both set the same
// input, the autoinclude value wins.
func TestStackDepsAutoIncludeWinsOnConflict(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsMergePrecedence)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsMergePrecedence)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsMergePrecedence, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)
	require.FileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "target", inthclparse.AutoIncludeFile))

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	markerPath := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "target"), "marker.txt")
	markerContent, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	assert.Equal(t, "received: from-autoinclude", string(markerContent),
		"autoinclude value must win over the unit's own inputs on conflict")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
}

// TestStackDepsAutoIncludeArbitraryOverride verifies that an autoinclude may patch
// a unit with config beyond dependency/inputs. Here a generate block is injected;
// it must be preserved in the generated file and emit its file on apply.
func TestStackDepsAutoIncludeArbitraryOverride(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsArbitraryOverride)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsArbitraryOverride)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsArbitraryOverride, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	autoIncludePath := filepath.Join(rootPath, inthclparse.StackDir, "gen", inthclparse.AutoIncludeFile)
	require.FileExists(t, autoIncludePath)
	content, err := os.ReadFile(autoIncludePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `generate "injected"`,
		"non-dependency blocks in autoinclude must be preserved in the generated file")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	injectedPath := helpers.FindCachedFile(t, filepath.Join(rootPath, inthclparse.StackDir, "gen"), "injected.tf")
	require.FileExists(t, injectedPath, "generate block injected via autoinclude must produce its file")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
}

// TestStackDepsAutoIncludeArbitraryRetryBlock verifies that an autoinclude may inject a
// block other than dependency/generate (here an errors retry block); it must be preserved
// in the generated unit and merge into the unit's effective config.
func TestStackDepsAutoIncludeArbitraryRetryBlock(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsArbitraryRetry)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsArbitraryRetry)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsArbitraryRetry, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	unitDir := filepath.Join(rootPath, inthclparse.StackDir, "svc")
	autoIncludePath := filepath.Join(unitDir, inthclparse.AutoIncludeFile)
	require.FileExists(t, autoIncludePath)

	unitConfigPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	require.FileExists(t, unitConfigPath)

	// Parse the generated unit terragrunt.hcl with the stack-dependencies experiment on so the
	// sibling autoinclude merges in. The injected errors/retry block must appear in the unit's
	// effective merged config, not merely as text in the standalone generated autoinclude file.
	l := logger.CreateLogger()
	ctx, pctx := newStackDepsParsingContext(t, l, unitConfigPath)

	parsed, err := config.ParseConfigFile(ctx, pctx, l, unitConfigPath, nil)
	require.NoError(t, err, "unit config with a merged autoinclude errors block must parse as a valid config")
	require.NotNil(t, parsed.Errors, "errors block injected via autoinclude must merge into the unit's effective config")
	require.Len(t, parsed.Errors.Retry, 1)

	retry := parsed.Errors.Retry[0]
	assert.Equal(t, "transient", retry.Label)
	assert.Equal(t, 3, retry.MaxAttempts)
	assert.Equal(t, 5, retry.SleepIntervalSec)
	assert.Equal(t, []string{".*transient.*"}, retry.RetryableErrors)

	// The merged block must also survive a discovery-style partial parse: ErrorsBlock is in the
	// discovery decode list, so a partial parse of the unit must surface the same retry rule.
	partial := partialParseDiscovery(t, l, unitConfigPath)
	require.NotNil(t, partial.Errors, "merged errors block must survive a discovery partial parse")
	require.Len(t, partial.Errors.Retry, 1)
	assert.Equal(t, "transient", partial.Errors.Retry[0].Label)
	assert.Equal(t, 3, partial.Errors.Retry[0].MaxAttempts)
}

// TestStackDepsAutoIncludeFeatureBlock verifies that an autoinclude may inject a
// feature block (a block other than dependency/generate); it must be preserved in
// the generated unit and merge into the unit's effective config.
func TestStackDepsAutoIncludeFeatureBlock(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsArbitraryFeature)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsArbitraryFeature)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsArbitraryFeature, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	unitDir := filepath.Join(rootPath, inthclparse.StackDir, "svc")
	autoIncludePath := filepath.Join(unitDir, inthclparse.AutoIncludeFile)
	require.FileExists(t, autoIncludePath)

	unitConfigPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	require.FileExists(t, unitConfigPath)

	// Parse the generated unit terragrunt.hcl with the stack-dependencies experiment on so the
	// sibling autoinclude merges in. The injected feature block must appear in the unit's
	// effective merged config with the exact value.
	l := logger.CreateLogger()
	ctx, pctx := newStackDepsParsingContext(t, l, unitConfigPath)

	parsed, err := config.ParseConfigFile(ctx, pctx, l, unitConfigPath, nil)
	require.NoError(t, err, "unit config with a merged autoinclude feature block must parse as a valid config")
	require.Len(t, parsed.FeatureFlags, 1, "feature block injected via autoinclude must merge into the unit's effective config")

	flag := parsed.FeatureFlags[0]
	assert.Equal(t, "foo", flag.Name)
	require.NotNil(t, flag.Default, "feature flag default must decode")
	assert.True(t, flag.Default.RawEquals(cty.True), "feature flag default must decode to true")

	// The merged block must also survive a discovery-style partial parse: FeatureFlagsBlock is in
	// the discovery decode list, so a partial parse of the unit must surface the same feature flag.
	partial := partialParseDiscovery(t, l, unitConfigPath)
	require.Len(t, partial.FeatureFlags, 1, "merged feature block must survive a discovery partial parse")
	assert.Equal(t, "foo", partial.FeatureFlags[0].Name)
	require.NotNil(t, partial.FeatureFlags[0].Default)
	assert.True(t, partial.FeatureFlags[0].Default.RawEquals(cty.True))
}

// TestStackDepsAutoIncludeIgnoreBlock verifies that an autoinclude may inject an
// errors block with an ignore rule (a block other than dependency/generate); it must
// be preserved in the generated unit and merge into the unit's effective config.
func TestStackDepsAutoIncludeIgnoreBlock(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsArbitraryIgnore)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsArbitraryIgnore)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsArbitraryIgnore, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	unitDir := filepath.Join(rootPath, inthclparse.StackDir, "svc")
	autoIncludePath := filepath.Join(unitDir, inthclparse.AutoIncludeFile)
	require.FileExists(t, autoIncludePath)

	unitConfigPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	require.FileExists(t, unitConfigPath)

	// Parse the generated unit terragrunt.hcl with the stack-dependencies experiment on so the
	// sibling autoinclude merges in. The injected errors/ignore block must appear in the unit's
	// effective merged config with the exact values.
	l := logger.CreateLogger()
	ctx, pctx := newStackDepsParsingContext(t, l, unitConfigPath)

	parsed, err := config.ParseConfigFile(ctx, pctx, l, unitConfigPath, nil)
	require.NoError(t, err, "unit config with a merged autoinclude errors block must parse as a valid config")
	require.NotNil(t, parsed.Errors, "errors block injected via autoinclude must merge into the unit's effective config")
	require.Len(t, parsed.Errors.Ignore, 1)

	ignore := parsed.Errors.Ignore[0]
	assert.Equal(t, "bar", ignore.Label)
	assert.Equal(t, "Ignoring error bar", ignore.Message)
	assert.Equal(t, []string{".*bar.*"}, ignore.IgnorableErrors)
	require.Contains(t, ignore.Signals, "failed_bar")
	assert.True(t, ignore.Signals["failed_bar"].RawEquals(cty.True), "ignore signal must decode to true")

	// The merged block must also survive a discovery-style partial parse: ErrorsBlock is in the
	// discovery decode list, so a partial parse of the unit must surface the same ignore rule.
	partial := partialParseDiscovery(t, l, unitConfigPath)
	require.NotNil(t, partial.Errors, "merged errors block must survive a discovery partial parse")
	require.Len(t, partial.Errors.Ignore, 1)
	assert.Equal(t, "bar", partial.Errors.Ignore[0].Label)
	assert.Equal(t, []string{".*bar.*"}, partial.Errors.Ignore[0].IgnorableErrors)
}

// TestStackDepsMockOutputsAtPlan exercises mock_outputs functionally: with no
// prior apply, planning the dependent unit must succeed against the dependency's
// mock_outputs (allowed for "plan"), rather than failing on unavailable outputs.
func TestStackDepsMockOutputsAtPlan(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsBasic, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "plan must succeed using mock_outputs before any apply; stderr=%s", stderr)
}

// TestStackDepsStackLevelAutoInclude verifies stack-level autoinclude generation.
// A stack does not have dependencies (only units do), so a stack-block autoinclude
// injects valid terragrunt.stack.hcl content (here an extra unit). Generation must
// produce terragrunt.autoinclude.stack.hcl (not the unit filename) in the generated
// nested-stack directory, preserving that content, without breaking discovery of
// the nested units.
func TestStackDepsStackLevelAutoInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsStackAutoInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsStackAutoInclude)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsStackAutoInclude)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	stackAutoInc := filepath.Join(rootPath, inthclparse.StackDir, "networking", inthclparse.AutoIncludeStackFile)
	require.FileExists(t, stackAutoInc, "a stack-block autoinclude must generate terragrunt.autoinclude.stack.hcl")
	assert.NoFileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "networking", inthclparse.AutoIncludeFile),
		"stack autoinclude must not be written with the unit-level filename")

	content, err := os.ReadFile(stackAutoInc)
	require.NoError(t, err)
	assert.Contains(t, string(content), `unit "extra"`, "the injected stack content must be preserved in the generated file")

	// Generation must not break discovery: the nested vpc unit must still enumerate.
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt find --json --working-dir "+rootPath)
	require.NoError(t, err)

	var components []findComponent
	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	foundVPC := false

	for _, c := range components {
		if c.Type == findComponentTypeUnit && filepath.Base(c.Path) == "vpc" {
			foundVPC = true
			break
		}
	}

	assert.True(t, foundVPC, "nested vpc unit must be discoverable")
}

// TestStackDepsStackLevelAutoIncludeMergedIntoNestedStack verifies that a unit
// injected via a stack-level autoinclude materializes in the nested stack, once
// the generated terragrunt.autoinclude.stack.hcl is merged into the nested
// terragrunt.stack.hcl the same way a unit's terragrunt.autoinclude.hcl is merged
// into its terragrunt.hcl.
func TestStackDepsStackLevelAutoIncludeMergedIntoNestedStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsStackAutoInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsStackAutoInclude)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsStackAutoInclude)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	// The injected unit must be merged into the nested stack and generated.
	nestedStackDir := filepath.Join(rootPath, inthclparse.StackDir, "networking", inthclparse.StackDir)
	assert.DirExists(t, filepath.Join(nestedStackDir, "extra"),
		"the unit injected via the stack-level autoinclude must materialize in the nested stack")

	// The merged unit must also be discoverable, proving the merge feeds downstream
	// tooling (find/run), not just the on-disk directory copy.
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt find --json --working-dir "+rootPath)
	require.NoError(t, err)

	var components []findComponent
	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	foundExtra := false

	for _, c := range components {
		if c.Type == findComponentTypeUnit && filepath.Base(c.Path) == "extra" {
			foundExtra = true
			break
		}
	}

	assert.True(t, foundExtra, "the autoinclude-injected extra unit must be discoverable")
}

// TestStackDepsStackLevelAutoIncludeOverridesSameNameUnit verifies that when a stack-level
// autoinclude injects a unit whose name matches a unit already declared by the nested stack, the
// injected block overrides the base block wholesale (the generated unit is sourced from the
// autoinclude, not the nested stack), while an injected unit with a new name is appended.
func TestStackDepsStackLevelAutoIncludeOverridesSameNameUnit(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsStackAutoIncOverride)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsStackAutoIncOverride)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsStackAutoIncOverride)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	nestedStackDir := filepath.Join(rootPath, inthclparse.StackDir, "networking", inthclparse.StackDir)

	// The same-name vpc unit must be materialized from the autoinclude override source, not the base.
	vpcMain, err := os.ReadFile(filepath.Join(nestedStackDir, "vpc", "main.tf"))
	require.NoError(t, err, "the overridden vpc unit must be generated")
	assert.Contains(t, string(vpcMain), "vpc-override", "the injected unit must override the base unit wholesale")
	assert.NotContains(t, string(vpcMain), "vpc-base", "the base unit source must not survive the override")

	// The override is wholesale: the base block's nested unit-level autoinclude must NOT leak into the
	// overridden unit, so no terragrunt.autoinclude.hcl carrying the base inputs may be generated.
	leakedAutoInclude := filepath.Join(nestedStackDir, "vpc", inthclparse.AutoIncludeFile)
	assert.NoFileExists(t, leakedAutoInclude,
		"the base block's autoinclude must not leak into the overridden unit")

	// Pruning is surgical: a non-overridden sibling keeps its own unit-level autoinclude.
	siblingAutoInclude := filepath.Join(nestedStackDir, "sibling", inthclparse.AutoIncludeFile)
	assert.FileExists(t, siblingAutoInclude,
		"a non-overridden sibling must keep its own autoinclude")

	// The new-name unit injected alongside the override must still be appended.
	assert.DirExists(t, filepath.Join(nestedStackDir, "added"),
		"an injected unit with a new name must be appended, not dropped")

	// The merged result must feed discovery: exactly one vpc unit, plus the added unit.
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt find --json --working-dir "+rootPath)
	require.NoError(t, err)

	var components []findComponent
	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	vpcCount := 0
	foundAdded := false

	for _, c := range components {
		if c.Type != findComponentTypeUnit {
			continue
		}

		switch filepath.Base(c.Path) {
		case "vpc":
			vpcCount++
		case "added":
			foundAdded = true
		}
	}

	assert.Equal(t, 1, vpcCount, "the override must collapse the same-name unit to a single vpc component")
	assert.True(t, foundAdded, "the appended added unit must be discoverable")
}

// TestStackDepsStackLevelAutoIncludeOverridePathUsesLocal pins that stack generation succeeds when a
// sibling autoinclude injects a block whose path references the base stack's local. The override prune
// reads only block names, so it must not fail evaluating the injected path against the generate-path eval
// context (which has no local.* populated), keeping generation consistent with discovery and the full parse.
func TestStackDepsStackLevelAutoIncludeOverridePathUsesLocal(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsStackAutoIncLocalPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsStackAutoIncLocalPath)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsStackAutoIncLocalPath)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	// The injected unit's path resolved local.region to "eu", so it materializes at extra-eu.
	assert.DirExists(t, filepath.Join(rootPath, inthclparse.StackDir, "extra-eu"),
		"the injected unit path referencing local.region must resolve during generation")
	assert.NoDirExists(t, filepath.Join(rootPath, inthclparse.StackDir, "extra-${local.region}"),
		"the local reference must not be left unresolved in the generated path")
}

// TestStackDepsStackLevelAutoIncludeInjectsNestedStack verifies that a stack-level
// autoinclude can inject a nested stack (not just a unit). The injected stack must
// itself be re-discovered and expanded by the level-by-level generator, so its units
// materialize one level deeper than the autoinclude target.
func TestStackDepsStackLevelAutoIncludeInjectsNestedStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsStackAutoIncludeNested)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsStackAutoIncludeNested)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsStackAutoIncludeNested)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	// The autoinclude-injected "more" stack must merge into the networking stack and then
	// expand its own units a level deeper.
	moreStackDir := filepath.Join(rootPath, inthclparse.StackDir, "networking", inthclparse.StackDir, "more")
	assert.DirExists(t, moreStackDir,
		"the stack injected via the stack-level autoinclude must materialize in the nested stack")
	assert.DirExists(t, filepath.Join(moreStackDir, inthclparse.StackDir, "deep"),
		"the autoinclude-injected nested stack must expand its own units")
}

// TestStackDepsCrossLevelViaValues verifies a dependency between units at
// different stack levels. The parent passes unit.producer.path down to the child
// stack via values, and a unit inside the child stack consumes it as its
// autoinclude dependency config_path; the consumer must receive the producer's
// real output after apply.
func TestStackDepsCrossLevelViaValues(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsCrossLevelValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsCrossLevelValues)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsCrossLevelValues)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	consumerDir := filepath.Join(rootPath, inthclparse.StackDir, "child", inthclparse.StackDir, "consumer")
	autoInc := filepath.Join(consumerDir, inthclparse.AutoIncludeFile)
	require.FileExists(t, autoInc, "consumer in the child stack must get an autoinclude wired to the parent's producer")

	content, err := os.ReadFile(autoInc)
	require.NoError(t, err)
	assert.Contains(t, string(content), `dependency "producer"`)

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	inputPath := helpers.FindCachedFile(t, consumerDir, "input.txt")
	inputContent, err := os.ReadFile(inputPath)
	require.NoError(t, err)
	assert.Equal(t, "consumer received: produced-across-levels", string(inputContent),
		"consumer must receive the producer's output across stack levels")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
}

// TestStackDepsValuesRefWithSiblingAutoInclude reproduces the regression where a stack
// block's values referencing unit.<name>.path failed generation with "There is no
// variable named \"unit\"" whenever the same terragrunt.stack.hcl carried a sibling
// unit with an autoinclude block. Both the child stack's consumer (wired via values)
// and the sibling (wired via its own autoinclude) must receive the producer's output.
func TestStackDepsValuesRefWithSiblingAutoInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsValuesSiblingAutoInc)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsValuesSiblingAutoInc)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsValuesSiblingAutoInc)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	consumerDir := filepath.Join(rootPath, inthclparse.StackDir, "child", inthclparse.StackDir, "consumer")
	require.FileExists(t, filepath.Join(consumerDir, inthclparse.AutoIncludeFile),
		"consumer in the child stack must get an autoinclude wired to the parent's producer")

	siblingDir := filepath.Join(rootPath, inthclparse.StackDir, "sibling")
	require.FileExists(t, filepath.Join(siblingDir, inthclparse.AutoIncludeFile),
		"the sibling unit's own autoinclude must still generate")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	consumerInput, err := os.ReadFile(helpers.FindCachedFile(t, consumerDir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(t, "consumer received: produced-across-levels", string(consumerInput),
		"consumer must receive the producer's output across stack levels despite the sibling autoinclude")

	siblingInput, err := os.ReadFile(helpers.FindCachedFile(t, siblingDir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(t, "consumer received: produced-across-levels", string(siblingInput),
		"the sibling unit must receive the producer's output via its own autoinclude")

	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
}

// TestStackDepsStackAutoIncludeDepValuesIsClearError covers the unsupported cross-level pattern: a
// stack-level autoinclude declares a dependency block AND injects a unit whose values
// derive from that dependency's outputs. stack generate must fail with the clear typed
// error pointing at the supported cross-level pattern, not the low-level HCL diagnostic.
func TestStackDepsStackAutoIncludeDepValuesIsClearError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsStackAutoIncDepValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsStackAutoIncDepValues)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsStackAutoIncDepValues)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	_, _, runErr := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+rootPath)
	require.Error(t, runErr, "a stack autoinclude carrying a dependency consumed by injected values must fail generation")

	var typed inthclparse.StackAutoIncludeDependencyValuesError
	require.ErrorAs(t, runErr, &typed, "the failure must be the typed StackAutoIncludeDependencyValuesError")
	assert.Equal(t, "net", typed.StackName)
	assert.Equal(t, "extra", typed.UnitName)

	// The clear error must name the supported alternative, not the cryptic low-level diagnostic.
	msg := runErr.Error()
	assert.Contains(t, msg, "supported cross-level pattern")
	assert.Contains(t, msg, "declare the dependency inside the nested unit's own autoinclude")
	assert.NotContains(t, msg, "Unsupported block type")
	assert.NotContains(t, msg, "no variable named dependency")
}

// TestStackDepsLocalsReadConfigWithDep covers a unit whose generated config uses locals driven by
// read_terragrunt_config(find_in_parent_folders) and find_in_parent_folders, passes values from
// local.* and values.*, alongside a unit carrying an autoinclude with a dependency. stack generate
// must succeed.
func TestStackDepsLocalsReadConfigWithDep(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsLocalsReadConfigDep)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsLocalsReadConfigDep)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsLocalsReadConfigDep)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(gitPath).Init(t.Context()))

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err = filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	_, stderr, runErr := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+rootPath)
	require.NoError(t, runErr, "stack generate with locals/read_terragrunt_config and an autoinclude dependency must succeed: %s", stderr)

	rolesAutoInc := filepath.Join(rootPath, inthclparse.StackDir, "roles", inthclparse.AutoIncludeFile)
	require.FileExists(t, rolesAutoInc, "the roles unit must get its autoinclude with the dependency wired in")

	content, err := os.ReadFile(rolesAutoInc)
	require.NoError(t, err)
	assert.Contains(t, string(content), `dependency "account"`)
	// Match the attribute whitespace-tolerantly so the assertion does not break on formatter column-alignment changes.
	assert.Regexp(t, `config_path\s*=\s*"\.\./account"`, string(content),
		"unit.account.path must resolve into the autoinclude config_path even with locals/read_terragrunt_config in the unit configs")

	accountConfig := filepath.Join(rootPath, inthclparse.StackDir, "account", config.DefaultTerragruntConfigPath)
	require.FileExists(t, accountConfig, "the account unit must materialize")
	assert.NoFileExists(t, filepath.Join(rootPath, inthclparse.StackDir, "account", inthclparse.AutoIncludeFile),
		"the account unit declares no autoinclude, so none must be generated")
}

// TestStackDepsAutoIncludeUnknownTarget is the negative path: an autoinclude
// dependency whose config_path references an undeclared unit must fail stack
// generate with a clean error, not a panic.
func TestStackDepsAutoIncludeUnknownTarget(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	stackDir := filepath.Join(tmpDir, "live")
	require.NoError(t, os.MkdirAll(stackDir, 0755))

	unitDir := filepath.Join(tmpDir, "catalog", "units", "a")
	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte("inputs = {}\n"), 0644))

	stackHCL := `
unit "a" {
  source = "../catalog/units/a"
  path   = "a"

  autoinclude {
    dependency "missing" {
      config_path = unit.does_not_exist.path
    }
  }
}
`
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackHCL), 0644))

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+stackDir)
	require.Error(t, err, "autoinclude referencing an undeclared unit must fail generation")
	assert.NotContains(t, stderr, "panic", "the failure must be a clean error, not a panic")
}

// newStackDepsParsingContext builds a parsing context with the stack-dependencies experiment
// enabled so a unit's sibling terragrunt.autoinclude.hcl merges into the effective config.
func newStackDepsParsingContext(t *testing.T, l log.Logger, configPath string) (context.Context, *config.ParsingContext) {
	t.Helper()

	opts := options.NewTerragruntOptions()
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.StackDependencies))
	opts.TerragruntConfigPath = configPath

	return configbridge.NewParsingContext(t.Context(), l, opts)
}

// partialParseDiscovery partial parses a unit config the way discovery does, with the
// stack-dependencies experiment on and the discovery feature/errors blocks in the decode list,
// so the merged autoinclude block must still surface.
func partialParseDiscovery(t *testing.T, l log.Logger, configPath string) *config.TerragruntConfig {
	t.Helper()

	ctx, pctx := newStackDepsParsingContext(t, l, configPath)
	pctx = pctx.WithDecodeList(config.FeatureFlagsBlock, config.ErrorsBlock).WithSkipOutputsResolution()

	parsed, err := config.PartialParseConfigFile(ctx, pctx, l, configPath, nil)
	require.NoError(t, err, "discovery-style partial parse of the merged unit config must succeed")

	return parsed
}

// stackDepsFuncsFor builds a per-dir stack function factory so nested stacks resolve dir-sensitive functions against their own dir.
func stackDepsFuncsFor(ctx context.Context, l log.Logger, pctx *config.ParsingContext) inthclparse.StackFuncFactory {
	return func(dir string) (map[string]function.Function, error) {
		return config.EarlyStackParseFunctions(ctx, l, dir, pctx)
	}
}

// TestStackDepsHCLValidateReportsMalformedAutoInclude pins that `hcl validate` runs the strict
// autoinclude parse over stack configs: a malformed autoinclude block (a locals block inside
// autoinclude) fails validation instead of only failing later at `stack generate`.
func TestStackDepsHCLValidateReportsMalformedAutoInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsHCLValidateAutoInc)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsHCLValidateAutoInc)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsHCLValidateAutoInc, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	// hcl validate runs the same strict parse `stack generate` uses, so it must reject the malformed block.
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt hcl validate --working-dir "+rootPath)
	require.Error(t, err, "hcl validate must report the malformed autoinclude block")
}

// TestStackDepsHCLValidateAcceptsValidAutoInclude pins that the strict autoinclude pass added to
// `hcl validate` does not reject a well-formed autoinclude block.
func TestStackDepsHCLValidateAcceptsValidAutoInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoInclude)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoInclude)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoInclude, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt hcl validate --working-dir "+rootPath)
	require.NoError(t, err, "a well-formed autoinclude block must pass hcl validate")
}
