//go:build !windows && tf

package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTFStackDepsMockLocalResolvesLocal pins, end to end, that an autoinclude dependency's mock_outputs resolves
// stack-level locals to literals at generate time. The run step confirms the generated stack plans cleanly while
// the unit's own inputs keep the dependency.* references for the unit run.
func TestTFStackDepsMockLocalResolvesLocal(t *testing.T) {
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

	generated, err := os.ReadFile(
		filepath.Join(rootPath, inthclparse.StackDir, "iam", inthclparse.AutoIncludeFile),
	)
	require.NoError(t, err)

	content := string(generated)

	// Dependency path: config_path = unit.account.path resolves to the sibling unit at generate time.
	assert.Contains(
		t,
		content,
		`"../account"`,
		"the dependency config_path (unit.<name>.path) must resolve at generate time",
	)

	// Dependency mock outputs: stack-level locals are generate-time-knowable, so they resolve to literals here.
	assert.Contains(
		t,
		content,
		`"my-account"`,
		"a local in mock_outputs must be resolved at generate time",
	)
	assert.Contains(
		t,
		content,
		`"eu-west-1"`,
		"a local in mock_outputs must be resolved at generate time",
	)
	assert.NotContains(t, content, "local.account", "a stack-level local must not be left literal")
	assert.NotContains(
		t,
		content,
		"values.region",
		"values.* must not appear in the generated file",
	)

	// The autoinclude only contributes the mock dependency; inputs live in the unit's own terragrunt.hcl.
	assert.NotContains(
		t,
		content,
		"inputs",
		"the generated autoinclude must contain only the mock dependency, not inputs",
	)

	// End to end: the unit's own inputs consume the dependency mock outputs, so the stack must plan cleanly.
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "the generated stack must plan; stderr=%s", stderr)
	assert.NotContains(
		t,
		stderr,
		"no variable named",
		"the generated stack must reference no undefined variables",
	)
}

// TestTFStackDepsAutoIncludeResolvesObjectKey verifies, end to end, that an interpolated object key in an autoinclude resolves at stack generate time even when the object's value defers to dependency.*, so no stack-level reference leaks into the generated unit.
func TestTFStackDepsAutoIncludeResolvesObjectKey(t *testing.T) {
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

	generated, err := os.ReadFile(
		filepath.Join(rootPath, inthclparse.StackDir, "app", inthclparse.AutoIncludeFile),
	)
	require.NoError(t, err)

	content := string(generated)
	assert.Contains(
		t,
		content,
		`"pre_key"`,
		"an interpolated object key must resolve at generate time",
	)
	assert.NotContains(
		t,
		content,
		"local.prefix",
		"an interpolated object key must not leak a stack-level reference into the generated unit",
	)
	assert.Contains(
		t,
		content,
		"dependency.vpc.outputs.id",
		"the dependency reference stays verbatim for the unit",
	)
	assert.Contains(
		t,
		content,
		"pre_mock",
		"an interpolated key inside a dependency block attribute must resolve at generate time",
	)

	// End to end: the generated unit must evaluate (a leaked stack reference would fail here), with the resolved key carrying the mocked dependency output through to the unit's planned outputs.
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "the generated stack must plan; stderr=%s", stderr)
	assert.Contains(t, stdout, "pre_key=mock-vpc-id",
		"the resolved object key must carry the mocked dependency output into the unit inputs")
}

// TestTFStackDepsAutoIncludeFunctionsAndDeps covers, end to end, how an autoinclude treats functions and dependencies:
// a function call with no dependency.* reference (read_terragrunt_config in config_path, run_cmd in inputs) resolves
// in the stack file context at generate time, while a dependency.*.outputs.* reference stays verbatim and resolves
// inside the generated unit. The mock feeds the deferred dependency output at plan time.
func TestTFStackDepsAutoIncludeFunctionsAndDeps(t *testing.T) {
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

	generated, err := os.ReadFile(
		filepath.Join(rootPath, inthclparse.StackDir, "app", inthclparse.AutoIncludeFile),
	)
	require.NoError(t, err)

	content := string(generated)

	// read_terragrunt_config in config_path is a generate-time function: it is evaluated in the stack file
	// context, so config_path is the resolved sibling path, not the function call.
	assert.Contains(
		t,
		content,
		`"../data"`,
		"config_path from read_terragrunt_config must resolve at generate time",
	)
	assert.NotContains(
		t,
		content,
		"read_terragrunt_config",
		"read_terragrunt_config in config_path must be evaluated at generate, not deferred",
	)

	// In inputs, a function call with no dependency.* reference resolves at generate time, while a dependency
	// output reference stays verbatim for unit-time evaluation.
	assert.Contains(
		t,
		content,
		`"hi-from-unit"`,
		"a function call with no dependency reference must resolve at generate time",
	)
	assert.NotContains(
		t,
		content,
		"run_cmd(",
		"a resolvable function in inputs must not be left verbatim",
	)
	assert.Contains(
		t,
		content,
		"dependency.data.outputs.value",
		"a dependency output in inputs must stay verbatim",
	)

	// End to end: the dependency mock feeds the deferred input, the generate-time run_cmd result is baked in, and the stack plans cleanly.
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(t, err, "the generated stack must plan; stderr=%s", stderr)
	assert.Contains(t, stdout, "mock-data:hi-from-unit",
		"the dependency mock and the generate-time run_cmd result must both feed the unit inputs")
}

// TestTFStackDepsE2EBasic runs the full end-to-end flow with 2 units:
// stack generate -> run --all apply -> verify outputs -> run --all destroy.
func TestTFStackDepsE2EBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsBasic, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	autoIncludePath := filepath.Join(
		rootPath,
		inthclparse.StackDir,
		"unit-w-inputs",
		"terragrunt.autoinclude.hcl",
	)
	require.FileExists(t, autoIncludePath)

	content, err := os.ReadFile(autoIncludePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `dependency "unit_w_outputs"`)
	assert.Contains(t, string(content), "../unit-w-outputs")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	inputPath := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "unit-w-inputs"),
		"input.txt",
	)

	inputContent, err := os.ReadFile(inputPath)
	require.NoError(t, err)
	assert.Equal(t, "Received: Hello!", string(inputContent))

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)
}

// TestTFStackDepsRemoteStateDependency pins that a dependency output referenced
// inside a remote_state config (the dependency block lives only in the
// generated autoinclude) resolves during run --all plan instead of failing
// with an unknown "dependency" variable.
func TestTFStackDepsRemoteStateDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsRemoteStateDep)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsRemoteStateDep)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsRemoteStateDep, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(
		t,
		err,
		"run --all plan must resolve dependency output inside remote_state; stderr=%s",
		stderr,
	)

	// The mock output fake-val must resolve inside the generated backend, producing the key fake-val.tfstate.
	backendPath := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "unit-w-inputs"),
		"backend.tf",
	)
	backend, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Contains(t, string(backend), "fake-val.tfstate",
		"dependency output must resolve inside remote_state from the autoinclude mock")
}

// TestTFStackDepsNestedRemoteStateDependency covers a nested stack-of-stacks
// (stacks -> sandbox-1 -> roles) where the roles unit references an autoinclude-injected
// dependency output in both a remote_state block and a generate block, driven by
// `terragrunt stack run plan`.
func TestTFStackDepsNestedRemoteStateDependency(t *testing.T) {
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
	require.NoError(
		t,
		err,
		"stack run plan must resolve the autoinclude dependency in remote_state; stderr=%s",
		stderr,
	)

	// The mock account name must resolve inside the nested roles unit's generated backend.
	rolesDir := filepath.Join(
		rootPath,
		inthclparse.StackDir,
		"sandbox-1",
		inthclparse.StackDir,
		"roles_hcl",
	)
	backendPath := helpers.FindCachedFile(t, rolesDir, "backend.tf")
	backend, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Contains(t, string(backend), "mock-account/roles.tfstate",
		"dependency output must resolve inside remote_state in a nested stack")
}

// TestTFStackDepsNestedUnitAutoIncludeDependency covers a nested stack whose unit autoinclude depends on
// a sibling unit via unit.X.path, with the dependency output consumed through inputs. The generated
// dependency config_path must account for the nested .terragrunt-stack directory (../data, not one
// level too high), and run --all plan must resolve it.
func TestTFStackDepsNestedUnitAutoIncludeDependency(t *testing.T) {
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
	assert.Contains(
		t,
		string(autoInclude),
		`"../data"`,
		"the nested-stack dependency must resolve to the sibling unit through .terragrunt-stack, not one level too high",
	)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(
		t,
		err,
		"a nested-stack unit autoinclude dependency must resolve at run time; stderr=%s",
		stderr,
	)
	assert.NotContains(t, stderr, "does not contain a terragrunt.hcl",
		"the dependency path must include the nested .terragrunt-stack segment")
}

// TestTFStackDepsAutoIncludeOverridesUnitDependency covers the same-name dependency conflict case:
// when a unit declares its own dependency block AND the autoinclude declares a dependency
// of the same name, the autoinclude block wins by name (shallow merge, like a default include),
// so dependency.x.outputs.v resolves to the autoinclude's mock value, not the unit's.
func TestTFStackDepsAutoIncludeOverridesUnitDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsDupDependency)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsDupDependency)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsDupDependency, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(
		t,
		err,
		"autoinclude dependency must override the unit's same-name block; stderr=%s",
		stderr,
	)

	backendPath := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "y"),
		"backend.tf",
	)
	backend, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Contains(t, string(backend), "from-autoinclude.tfstate",
		"autoinclude dependency (by name) must override the unit's own dependency path")
}

// TestTFStackDepsAutoIncludeReplacesUnitDependency verifies the shallow-merge contract for a same-name
// dependency conflict: when a unit and its autoinclude both declare dependency "x", the autoinclude
// block REPLACES the unit's wholesale (it is not deep-merged). So the autoinclude's mock outputs are the
// ones that resolve, the conflicting "common" key takes the autoinclude value, and the unit-only key
// (from_unit) no longer exists, exactly as a default include behaves.
func TestTFStackDepsAutoIncludeReplacesUnitDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsDepMockMerge)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsDepMockMerge)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsDepMockMerge, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(
		t,
		err,
		"the autoinclude dependency must replace the unit's same-name block (shallow); stderr=%s",
		stderr,
	)

	// Backend key is "absent-autoinclude-common": the autoinclude's block fully replaced the unit's, so
	// from_unit no longer exists (try() falls back to "absent") and the conflicting "common" key resolves
	// to the autoinclude value. A deep merge would keep from_unit and yield "unitval-autoinclude-common",
	// so this assertion fails if foldSiblingAutoIncludeDeps reverts to a deep merge (both still plan).
	backendPath := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "y"),
		"backend.tf",
	)
	backend, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Contains(t, string(backend), "absent-autoinclude-common.tfstate",
		"the autoinclude dependency must replace the unit's same-name block, not deep-merge it")
}

// TestTFStackDepsAutoIncludeDisabledDependencyCreatesNoEdge is a regression test: a dependency declared
// in an autoinclude with enabled = false must not become a run-DAG edge. The disabled dependency points
// at a nonexistent path, so a run that followed it would fail with a missing terragrunt.hcl error. The
// partial-parse merge drops disabled blocks, and discovery must not re-add them from the raw autoinclude.
func TestTFStackDepsAutoIncludeDisabledDependencyCreatesNoEdge(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsDisabledAutoIncDep)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsDisabledAutoIncDep)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsDisabledAutoIncDep, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(
		t,
		err,
		"a disabled autoinclude dependency must not create a run-DAG edge to its nonexistent path; stderr=%s",
		stderr,
	)
	assert.NotContains(
		t,
		stderr,
		"nonexistent-in-tree",
		"the disabled dependency path must not enter the run graph",
	)
}

// TestTFStackDepsAutoIncludeDependencyAppliesWithoutMockOutputs verifies that run --all apply succeeds and
// reads the dependency's real output when an autoinclude-injected dependency defines no mock_outputs: the
// run queue applies the dependency first, so the dependent never needs a mock at apply time.
func TestTFStackDepsAutoIncludeDependencyAppliesWithoutMockOutputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsApplyNoMocks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsApplyNoMocks)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsApplyNoMocks, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")
	require.NoError(
		t,
		err,
		"run --all apply must apply the dependency first and read its real output even with no mock_outputs; stderr=%s",
		stderr,
	)

	// The consumer marker must hold the producer's REAL output (no mock exists), proving the queue applied
	// the producer first and the dependent read live state rather than failing on a missing output.
	marker := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "consumer"),
		"marker.txt",
	)

	content, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, "consumer received: real-producer-output", string(content))
}

// TestTFStackDepsE2EChain runs a 3-level dependency chain end-to-end:
// unit_a -> unit_b -> unit_c
// Verifies chained output propagation and correct apply/destroy ordering.
func TestTFStackDepsE2EChain(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsChain)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsChain)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsChain, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	// Verify autoinclude generated for unit-b and unit-a but not unit-c
	require.FileExists(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "unit-b", "terragrunt.autoinclude.hcl"),
	)
	require.FileExists(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "unit-a", "terragrunt.autoinclude.hcl"),
	)
	assert.NoFileExists(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "unit-c", "terragrunt.autoinclude.hcl"),
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	// Verify unit-a received chained output: from-b(from-c)
	markerA := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "unit-a"),
		"marker.txt",
	)

	contentA, err := os.ReadFile(markerA)
	require.NoError(t, err)
	assert.Equal(t, "unit-a received: from-b(from-c)", string(contentA))

	// Verify unit-b received: from-c
	markerB := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "unit-b"),
		"marker.txt",
	)

	contentB, err := os.ReadFile(markerB)
	require.NoError(t, err)
	assert.Equal(t, "unit-b received: from-c", string(contentB))

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)

	// Destroy must remove the marker files produced by apply.
	assert.NoFileExists(t, markerA)
	assert.NoFileExists(t, markerB)
}

// TestTFStackDepsE2ECrossStack tests stack generation with cross-stack dependencies:
// a "network" stack (containing vpc + subnets) and an "app" unit depending on
// the entire network stack via stack.network.path.
func TestTFStackDepsE2ECrossStack(t *testing.T) {
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

	autoIncludePath := filepath.Join(
		rootPath,
		inthclparse.StackDir,
		"app",
		"terragrunt.autoinclude.hcl",
	)
	require.FileExists(t, autoIncludePath)

	content, err := os.ReadFile(autoIncludePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `dependency "network"`)
	assert.Contains(t, string(content), "../network")

	// Verify network stack units were generated
	networkStackDir := filepath.Join(
		rootPath,
		inthclparse.StackDir,
		"network",
		inthclparse.StackDir,
	)
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
	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	markerPath := helpers.FindCachedFile(t, appDir, "marker.txt")
	markerContent, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	assert.Equal(t, "app received: vpc-cross-stack", string(markerContent),
		"app must receive the network stack's real vpc output, not the mock")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)
}

// TestTFStackDepsTransitiveStackDirDependency checks that a transitive dependency on a stack directory resolves.
func TestTFStackDepsTransitiveStackDirDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsTransitiveStackDir)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsTransitiveStackDir)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackDepsTransitiveStackDir)

	helpers.CreateGitRepo(t, gitPath)

	rootPath := filepath.Join(gitPath, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")
	require.NoError(
		t,
		err,
		"transitive dependency on a stack directory must resolve; stderr=%s",
		stderr,
	)
	assert.NotContains(t, stderr, "does not contain a terragrunt.hcl")
}

// TestTFStackDepsStackValuesInLocals pins that run-queue expansion of a stack-dir
// dependency resolves values.* referenced in the target stack's locals from the
// generated terragrunt.values.hcl sitting next to the generated terragrunt.stack.hcl,
// instead of failing with an unknown "values" variable (gruntwork-io/terragrunt#5663).
func TestTFStackDepsStackValuesInLocals(t *testing.T) {
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
	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan",
	)
}

// TestTFStackDepsE2ETree runs apply/destroy on the multi-level dependency tree
// and verifies output propagation through all levels.
func TestTFStackDepsE2ETree(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsTree)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsTree)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsTree, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	// Verify unit-b received outputs from D and E
	markerB := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "unit-b"),
		"marker.txt",
	)

	contentB, err := os.ReadFile(markerB)
	require.NoError(t, err)
	assert.Equal(t, "unit-b(from-d,from-e)", string(contentB))

	// Verify unit-a received outputs from B and C
	markerA := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "unit-a"),
		"marker.txt",
	)

	contentA, err := os.ReadFile(markerA)
	require.NoError(t, err)
	assert.Equal(t, "unit-a(from-b(from-d,from-e),from-c)", string(contentA))

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)

	// Destroy must remove the marker files produced by apply.
	assert.NoFileExists(t, markerA)
	assert.NoFileExists(t, markerB)
}

// TestTFStackDepsE2EAutoIncludeWithComplexSiblings is the end-to-end regression for stacks whose unrelated units use every HCL feature class that previously broke the simplified parser; the roles unit must observe the account unit's output through the generated autoinclude.
func TestTFStackDepsE2EAutoIncludeWithComplexSiblings(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncComplexSiblings)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncComplexSiblings)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncComplexSiblings, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	autoIncludePath := filepath.Join(
		rootPath,
		inthclparse.StackDir,
		"roles",
		inthclparse.AutoIncludeFile,
	)
	require.FileExists(t, autoIncludePath, "roles unit must have its autoinclude file generated")

	autoIncludeContent, err := os.ReadFile(autoIncludePath)
	require.NoError(t, err)
	assert.Contains(t, string(autoIncludeContent), `dependency "account"`)
	assert.Contains(t, string(autoIncludeContent), "../account")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	markerPath := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "roles"),
		"marker.txt",
	)

	markerContent, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	assert.Equal(
		t,
		"roles-received: account-output",
		string(markerContent),
		"roles must receive account's output via the generated autoinclude dependency",
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)
}

// Regression: `run --all` discovery must walk a generated nested stack file even when its unit `source` attribute contains terragrunt function calls.
func TestTFStackDepsRunAllWithFunctionsInNestedStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsRunAllFuncsInNestedStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsRunAllFuncsInNestedStack)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsRunAllFuncsInNestedStack, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, runErr := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(
		t,
		runErr,
		"run --all must succeed when the generated nested stack file contains terragrunt function calls; stderr=%s",
		stderr,
	)
	assert.NotContains(t, stderr, "Function calls not allowed",
		"discovery must not surface 'Function calls not allowed' on generated nested stack files")

	stdout, _, findErr := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt find --json --dag --dependencies --working-dir "+rootPath)
	require.NoError(t, findErr)

	var components []findComponent
	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	foundNestedVPC := false

	for _, c := range components {
		if c.Type == findComponentTypeUnit && filepath.Base(c.Path) == "vpc" &&
			strings.Contains(c.Path, filepath.Join("networking", inthclparse.StackDir, "vpc")) {
			foundNestedVPC = true
			break
		}
	}

	require.True(
		t,
		foundNestedVPC,
		"generated nested stack unit vpc must be present in find output",
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	nestedVPCGen := filepath.Join(
		rootPath,
		inthclparse.StackDir,
		"networking",
		inthclparse.StackDir,
		"vpc",
	)
	vpcOutput, _, outputErr := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt output -json vpc_id --working-dir "+nestedVPCGen)
	require.NoError(t, outputErr, "nested vpc unit must be applied by run --all discovery")
	assert.Contains(t, vpcOutput, "vpc-from-nested-stack")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)
}

// TestTFStackDepsNoDependenciesBaseline verifies the baseline case: the experiment
// is enabled and the stack has multiple units but no autoinclude. Generation must
// emit no autoinclude files and the stack must apply/destroy.
func TestTFStackDepsNoDependenciesBaseline(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsNoDeps)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsNoDeps)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsNoDeps, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	// No autoinclude declared anywhere: generation must not emit autoinclude files.
	assert.NoFileExists(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "alpha", inthclparse.AutoIncludeFile),
	)
	assert.NoFileExists(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "beta", inthclparse.AutoIncludeFile),
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	require.FileExists(
		t,
		helpers.FindCachedFile(
			t,
			filepath.Join(rootPath, inthclparse.StackDir, "alpha"),
			"marker.txt",
		),
	)
	require.FileExists(
		t,
		helpers.FindCachedFile(
			t,
			filepath.Join(rootPath, inthclparse.StackDir, "beta"),
			"marker.txt",
		),
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)
}

// TestTFStackDepsAutoIncludeWinsOnConflict pins the documented merge precedence:
// when the unit's own terragrunt.hcl and the autoinclude both set the same
// input, the autoinclude value wins.
func TestTFStackDepsAutoIncludeWinsOnConflict(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsMergePrecedence)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsMergePrecedence)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsMergePrecedence, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)
	require.FileExists(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "target", inthclparse.AutoIncludeFile),
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	markerPath := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "target"),
		"marker.txt",
	)
	markerContent, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	assert.Equal(t, "received: from-autoinclude", string(markerContent),
		"autoinclude value must win over the unit's own inputs on conflict")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)
}

// TestTFStackDepsAutoIncludeArbitraryOverride verifies that an autoinclude may patch
// a unit with config beyond dependency/inputs. Here a generate block is injected;
// it must be preserved in the generated file and emit its file on apply.
func TestTFStackDepsAutoIncludeArbitraryOverride(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsArbitraryOverride)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsArbitraryOverride)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsArbitraryOverride, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	autoIncludePath := filepath.Join(
		rootPath,
		inthclparse.StackDir,
		"gen",
		inthclparse.AutoIncludeFile,
	)
	require.FileExists(t, autoIncludePath)
	content, err := os.ReadFile(autoIncludePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `generate "injected"`,
		"non-dependency blocks in autoinclude must be preserved in the generated file")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	injectedPath := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "gen"),
		"injected.tf",
	)
	require.FileExists(
		t,
		injectedPath,
		"generate block injected via autoinclude must produce its file",
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)
}

// TestTFStackDepsMockOutputsAtPlan exercises mock_outputs functionally: with no
// prior apply, planning the dependent unit must succeed against the dependency's
// mock_outputs (allowed for "plan"), rather than failing on unavailable outputs.
func TestTFStackDepsMockOutputsAtPlan(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsBasic, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(
		t,
		err,
		"plan must succeed using mock_outputs before any apply; stderr=%s",
		stderr,
	)
}

// TestTFStackDepsCrossLevelViaValues verifies a dependency between units at
// different stack levels. The parent passes unit.producer.path down to the child
// stack via values, and a unit inside the child stack consumes it as its
// autoinclude dependency config_path; the consumer must receive the producer's
// real output after apply.
func TestTFStackDepsCrossLevelViaValues(t *testing.T) {
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

	consumerDir := filepath.Join(
		rootPath,
		inthclparse.StackDir,
		"child",
		inthclparse.StackDir,
		"consumer",
	)
	autoInc := filepath.Join(consumerDir, inthclparse.AutoIncludeFile)
	require.FileExists(
		t,
		autoInc,
		"consumer in the child stack must get an autoinclude wired to the parent's producer",
	)

	content, err := os.ReadFile(autoInc)
	require.NoError(t, err)
	assert.Contains(t, string(content), `dependency "producer"`)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	inputPath := helpers.FindCachedFile(t, consumerDir, "input.txt")
	inputContent, err := os.ReadFile(inputPath)
	require.NoError(t, err)
	assert.Equal(t, "consumer received: produced-across-levels", string(inputContent),
		"consumer must receive the producer's output across stack levels")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)
}

// TestTFStackDepsValuesRefWithSiblingAutoInclude reproduces the regression where a stack
// block's values referencing unit.<name>.path failed generation with "There is no
// variable named \"unit\"" whenever the same terragrunt.stack.hcl carried a sibling
// unit with an autoinclude block. Both the child stack's consumer (wired via values)
// and the sibling (wired via its own autoinclude) must receive the producer's output.
func TestTFStackDepsValuesRefWithSiblingAutoInclude(t *testing.T) {
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

	consumerDir := filepath.Join(
		rootPath,
		inthclparse.StackDir,
		"child",
		inthclparse.StackDir,
		"consumer",
	)
	require.FileExists(t, filepath.Join(consumerDir, inthclparse.AutoIncludeFile),
		"consumer in the child stack must get an autoinclude wired to the parent's producer")

	siblingDir := filepath.Join(rootPath, inthclparse.StackDir, "sibling")
	require.FileExists(t, filepath.Join(siblingDir, inthclparse.AutoIncludeFile),
		"the sibling unit's own autoinclude must still generate")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	consumerInput, err := os.ReadFile(helpers.FindCachedFile(t, consumerDir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(
		t,
		"consumer received: produced-across-levels",
		string(consumerInput),
		"consumer must receive the producer's output across stack levels despite the sibling autoinclude",
	)

	siblingInput, err := os.ReadFile(helpers.FindCachedFile(t, siblingDir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(t, "consumer received: produced-across-levels", string(siblingInput),
		"the sibling unit must receive the producer's output via its own autoinclude")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve",
	)
}

// TestTFStackDepsAutoIncludeOverridesConfigPathFromValues verifies that a unit dependency with
// config_path = values.vpc_path resolves when the stack autoinclude replaces that dependency
// with a different config_path and the values block omits vpc_path.
func TestTFStackDepsAutoIncludeOverridesConfigPathFromValues(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDepsAutoIncConfigPathValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDepsAutoIncConfigPathValues)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDepsAutoIncConfigPathValues, "live")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan")
	require.NoError(
		t,
		err,
		"autoinclude must override config_path = values.vpc_path without error; stderr=%s",
		stderr,
	)

	backendPath := helpers.FindCachedFile(
		t,
		filepath.Join(rootPath, inthclparse.StackDir, "app"),
		"backend.tf",
	)
	backend, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Contains(t, string(backend), "from-autoinclude.tfstate",
		"autoinclude dependency must override the unit's values.vpc_path config_path")
}
