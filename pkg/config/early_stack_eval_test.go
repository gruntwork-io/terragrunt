package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// terragruntFuncNames lists every Terragrunt HCL function exposed by the full
// production eval context. Kept in sync with createTerragruntEvalContext;
// TestEarlyStackParseFunctions_CoversAllTerragruntFunctions guards parity.
var terragruntFuncNames = []string{
	config.FuncNameFindInParentFolders,
	config.FuncNamePathRelativeToInclude,
	config.FuncNamePathRelativeFromInclude,
	config.FuncNameGetEnv,
	config.FuncNameRunCmd,
	config.FuncNameReadTerragruntConfig,
	config.FuncNameGetPlatform,
	config.FuncNameGetRepoRoot,
	config.FuncNameGetPathFromRepoRoot,
	config.FuncNameGetPathToRepoRoot,
	config.FuncNameGetTerragruntDir,
	config.FuncNameGetOriginalTerragruntDir,
	config.FuncNameGetTerraformCommand,
	config.FuncNameGetTerraformCLIArgs,
	config.FuncNameGetParentTerragruntDir,
	config.FuncNameGetAWSAccountAlias,
	config.FuncNameGetAWSAccountID,
	config.FuncNameGetAWSCallerIdentityArn,
	config.FuncNameGetAWSCallerIdentityUserID,
	config.FuncNameGetTerraformCommandsThatNeedVars,
	config.FuncNameGetTerraformCommandsThatNeedLocking,
	config.FuncNameGetTerraformCommandsThatNeedInput,
	config.FuncNameGetTerraformCommandsThatNeedParallelism,
	config.FuncNameSopsDecryptFile,
	config.FuncNameGetTerragruntSourceCLIFlag,
	config.FuncNameGetDefaultRetryableErrors,
	config.FuncNameReadTfvarsFile,
	config.FuncNameGetWorkingDir,
	config.FuncNameStartsWith,
	config.FuncNameEndsWith,
	config.FuncNameStrContains,
	config.FuncNameTimeCmp,
	config.FuncNameMarkAsRead,
	config.FuncNameMarkGlobAsRead,
	config.FuncNameConstraintCheck,
	config.FuncNameDeepMerge,
}

// newStackParsePctx builds a minimal ParsingContext sufficient for
// EarlyStackParseFunctions calls in unit tests. Tests that need specific opts
// state (Env, TerraformCommand, etc.) mutate fields after construction.
func newStackParsePctx(t *testing.T, baseDir string) *config.ParsingContext {
	t.Helper()

	_, pctx := config.NewParsingContext(t.Context(), logger.CreateLogger())
	pctx.TerragruntConfigPath = filepath.Join(baseDir, "terragrunt.hcl")
	pctx.WorkingDir = baseDir
	pctx.MaxFoldersToCheck = 100

	return pctx
}

func TestEarlyStackParseFunctions_CoversAllTerragruntFunctions(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	funcs, err := config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), baseDir, newStackParsePctx(t, baseDir))
	require.NoError(t, err)

	for _, name := range terragruntFuncNames {
		_, ok := funcs[name]
		assert.Truef(t, ok, "early stack parse functions missing %q", name)
	}
}

func TestEarlyStackParseFunctions_PureEvaluatesNormally(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	funcs, err := config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), baseDir, newStackParsePctx(t, baseDir))
	require.NoError(t, err)

	got, err := funcs[config.FuncNameStartsWith].Call([]cty.Value{cty.StringVal("foobar"), cty.StringVal("foo")})
	require.NoError(t, err)
	assert.Equal(t, cty.True, got)
}

func TestEarlyStackParseFunctions_GetEnvReadsPctxEnv(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	pctx := newStackParsePctx(t, baseDir)
	pctx.Venv.Env = map[string]string{"PLAN_KEY": "plan_value"}

	funcs, err := config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), baseDir, pctx)
	require.NoError(t, err)

	got, err := funcs[config.FuncNameGetEnv].Call([]cty.Value{cty.StringVal("PLAN_KEY")})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("plan_value"), got)
}

func TestEarlyStackParseFunctions_GetTerragruntDirReturnsBaseDir(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	funcs, err := config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), baseDir, newStackParsePctx(t, baseDir))
	require.NoError(t, err)

	got, err := funcs[config.FuncNameGetTerragruntDir].Call(nil)
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal(baseDir), got)
}

// TestEarlyStackParseFunctions_GetWorkingDirOverride verifies the
// stack-file-specific override: the production impl re-parses the current
// config to compute a Terraform source URL (which stack files don't carry).
// The override returns the stack directory instead.
func TestEarlyStackParseFunctions_GetWorkingDirOverride(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	funcs, err := config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), baseDir, newStackParsePctx(t, baseDir))
	require.NoError(t, err)

	got, err := funcs[config.FuncNameGetWorkingDir].Call(nil)
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal(baseDir), got)
}

// TestStackParseFunctionsFrom_OverridesWithoutMutating verifies the helper overrides get_working_dir to the
// stack directory, preserves other functions, and does not mutate the input map.
func TestStackParseFunctionsFrom_OverridesWithoutMutating(t *testing.T) {
	t.Parallel()

	sentinel := function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func([]cty.Value, cty.Type) (cty.Value, error) { return cty.StringVal("PROD"), nil },
	})
	input := map[string]function.Function{
		config.FuncNameGetWorkingDir: sentinel,
		"other":                      sentinel,
	}

	out := config.StackParseFunctionsFrom(input, "/base/dir")

	got, err := out[config.FuncNameGetWorkingDir].Call(nil)
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("/base/dir"), got, "get_working_dir must be overridden to the stack dir")

	assert.Contains(t, out, "other", "other functions must be preserved")

	gotIn, err := input[config.FuncNameGetWorkingDir].Call(nil)
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("PROD"), gotIn, "the input map must not be mutated")
}

func TestEarlyStackParseFunctions_GetTerraformCommandReadsPctx(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	pctx := newStackParsePctx(t, baseDir)
	pctx.TerraformCommand = "plan"

	funcs, err := config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), baseDir, pctx)
	require.NoError(t, err)

	got, err := funcs[config.FuncNameGetTerraformCommand].Call(nil)
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("plan"), got)
}

func TestEarlyStackParseFunctions_GetTerraformCLIArgsReadsPctx(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	pctx := newStackParsePctx(t, baseDir)
	pctx.TerraformCliArgs = iacargs.New()
	pctx.TerraformCliArgs.InsertArguments(0, "-auto-approve")

	funcs, err := config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), baseDir, pctx)
	require.NoError(t, err)

	got, err := funcs[config.FuncNameGetTerraformCLIArgs].Call(nil)
	require.NoError(t, err)
	require.Equal(t, cty.List(cty.String), got.Type())
	assert.Equal(t, cty.StringVal("-auto-approve"), got.AsValueSlice()[0])
}

func TestEarlyStackParseFunctions_MarkAsReadAppendsToPctxFilesRead(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	pctx := newStackParsePctx(t, baseDir)
	pctx.FilesRead = config.NewFilesRead()

	funcs, err := config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), baseDir, pctx)
	require.NoError(t, err)

	got, err := funcs[config.FuncNameMarkAsRead].Call([]cty.Value{cty.StringVal("inputs.yaml")})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("inputs.yaml"), got)
	assert.Equal(t, []string{filepath.Join(baseDir, "inputs.yaml")}, pctx.FilesRead.Paths())
}

// TestEarlyStackParseFunctions_PathRelativeToIncludeFallback verifies the
// no-include fallback: a stack file with no TrackInclude chain returns "."
// from path_relative_to_include rather than erroring.
func TestEarlyStackParseFunctions_PathRelativeToIncludeFallback(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	funcs, err := config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), baseDir, newStackParsePctx(t, baseDir))
	require.NoError(t, err)

	got, err := funcs[config.FuncNamePathRelativeToInclude].Call(nil)
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("."), got)
}

// TestEarlyStackParseFunctions_FindInParentFoldersResolvesDuringDiscovery pins
// that find_in_parent_folders resolves end-to-end against a real FS fixture
// through UnitPathsFromStackDir.
func TestEarlyStackParseFunctions_FindInParentFoldersResolvesDuringDiscovery(t *testing.T) {
	t.Parallel()

	// find_in_parent_folders walks the real OS filesystem; the fixture must live on disk.
	tmpRoot := t.TempDir()
	stackDir := filepath.Join(tmpRoot, "live", "stack")
	require.NoError(t, os.MkdirAll(stackDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpRoot, "root.hcl"), []byte(`# marker`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = basename(dirname(find_in_parent_folders("root.hcl")))
}
`), 0644))

	pctx := newStackParsePctx(t, stackDir)
	funcsFor := func(dir string) (map[string]function.Function, error) {
		return config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), dir, pctx)
	}

	paths, err := inthclparse.UnitPathsFromStackDir(vfs.NewOSFS(), stackDir, funcsFor)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	// basename(dirname(.../root.hcl)) == basename(tmpRoot); resolve symlinks
	// (on macOS, /var is a symlink to /private/var) before comparing.
	resolvedRoot, err := filepath.EvalSymlinks(tmpRoot)
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(resolvedRoot), filepath.Base(paths[0]))
}

// TestEarlyStackParseFunctions_RunCmdExecutesDuringDiscovery pins that
// run_cmd executes during stack discovery and its stdout is consumed as the
// unit's path attribute.
func TestEarlyStackParseFunctions_RunCmdExecutesDuringDiscovery(t *testing.T) {
	t.Parallel()

	tmpRoot := t.TempDir()
	stackDir := filepath.Join(tmpRoot, "live", "stack")
	require.NoError(t, os.MkdirAll(stackDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = run_cmd("--terragrunt-quiet", "echo", "computed-path")
}
`), 0644))

	pctx := newStackParsePctx(t, stackDir)
	funcsFor := func(dir string) (map[string]function.Function, error) {
		return config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), dir, pctx)
	}

	paths, err := inthclparse.UnitPathsFromStackDir(vfs.NewOSFS(), stackDir, funcsFor)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.True(t, strings.HasSuffix(paths[0], "computed-path"), "expected path to end with 'computed-path', got %q", paths[0])
}

func TestEarlyStackParseFunctions_TerraformStdlibIncluded(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	funcs, err := config.EarlyStackParseFunctions(t.Context(), logger.CreateLogger(), baseDir, newStackParsePctx(t, baseDir))
	require.NoError(t, err)

	// `format` is a Terraform stdlib function. Its presence proves the stdlib
	// scope is still merged into the early-stack-parse function set.
	_, ok := funcs["format"]
	assert.True(t, ok, "terraform stdlib (format) should be present alongside the tg functions")
}
