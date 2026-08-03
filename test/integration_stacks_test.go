package test_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureStacksBasic                     = "fixtures/stacks/basic"
	testFixtureStacksLocals                    = "fixtures/stacks/locals"
	testFixtureStacksRemote                    = "fixtures/stacks/remote"
	testFixtureStacksInputs                    = "fixtures/stacks/inputs"
	testFixtureStacksOutputs                   = "fixtures/stacks/outputs"
	testFixtureStacksUnitValues                = "fixtures/stacks/unit-values"
	testFixtureStacksLocalsError               = "fixtures/stacks/errors/locals-error"
	testFixtureStacksUnitEmptyPath             = "fixtures/stacks/errors/unit-empty-path"
	testFixtureStacksEmptyPath                 = "fixtures/stacks/errors/stack-empty-path"
	testFixtureStackAbsolutePath               = "fixtures/stacks/errors/absolute-path"
	testFixtureStackRelativePathOutsideOfStack = "fixtures/stacks/errors/relative-path-outside-of-stack"
	testFixtureStackNotExist                   = "fixtures/stacks/errors/not-existing-path"
	testFixtureStackValidationUnitPath         = "fixtures/stacks/errors/validation-unit"
	testFixtureStackValidationStackPath        = "fixtures/stacks/errors/validation-stack"
	testFixtureStackIncorrectSource            = "fixtures/stacks/errors/incorrect-source"
	testFixtureStacksUnknownValueError         = "fixtures/stacks/errors/unknown-value"
	testFixtureNoStack                         = "fixtures/stacks/no-stack"
	testFixtureNestedStacks                    = "fixtures/stacks/nested"
	testFixtureNestedStackFilter               = "fixtures/stacks/nested-stack-filter"
	testFixtureStackValues                     = "fixtures/stacks/stack-values"
	testFixtureStackDependencies               = "fixtures/stacks/dependencies"
	testFixtureStackSourceMap                  = "fixtures/stacks/source-map"
	testFixtureNoStackNoDir                    = "fixtures/stacks/no-stack-dir"
	testFixtureMultipleStacks                  = "fixtures/stacks/multiple-stacks"
	testFixtureReadStack                       = "fixtures/stacks/read-stack"
	testFixtureStackSelfInclude                = "fixtures/stacks/self-include"
	testFixtureStackNestedOutputs              = "fixtures/stacks/nested-outputs"
	testFixtureStackNoValidation               = "fixtures/stacks/no-validation"
	testFixtureStackTerragruntDir              = "fixtures/stacks/terragrunt-dir"
	testFixtureStacksAllNoStackDir             = "fixtures/stacks/all-no-stack-dir"
	testFixtureStackNoDotTerragruntStackOutput = "fixtures/stacks/no-dot-terragrunt-stack-output"
	testFixtureStackFindInParentFolders        = "fixtures/stacks/find-in-parent-folders"
	testFixtureStackOriginalTerragruntDir      = "fixtures/stacks/get-original-terragrunt-dir"
	testFixtureStackVersionConstraints         = "fixtures/stacks/version-constraints"
	testFixtureStackCoexistHclAndStack         = "fixtures/stacks/coexist-hcl-and-stack"
	testFixtureStackExcludeOutput              = "fixtures/stacks/exclude-output"
	testFixtureStacksOutputsParallel           = "fixtures/stacks/outputs-parallel"
)

func TestStacksGenerateBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksBasic, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	path := filepath.Join(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestNestedStacksGenerate(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	gitPath := filepath.Join(tmpEnvPath, testFixtureNestedStacks)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := filepath.Join(gitPath, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)
	require.NoError(t, err)

	// Check that logs contain stack generation messages
	assert.Contains(t, stderr, "Generating stack prod from ./terragrunt.stack.hcl")
	assert.Contains(t, stderr, "Generating stack dev from ./terragrunt.stack.hcl")
	assert.Contains(
		t,
		stderr,
		"Generating unit prod-api from ./.terragrunt-stack/prod/terragrunt.stack.hcl",
	)
	assert.Contains(
		t,
		stderr,
		"Generating unit dev-web from ./.terragrunt-stack/dev/terragrunt.stack.hcl",
	)

	path := filepath.Join(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksGenerateErrorOnCoexistingHclAndStackFiles(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackCoexistHclAndStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackCoexistHclAndStack)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackCoexistHclAndStack)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := filepath.Join(gitPath, "non-prod", "dev")

	// Create the conflicting terragrunt.hcl alongside terragrunt.stack.hcl in temp copy.
	// Not kept on disk in the fixture to avoid breaking `terragrunt find` from repo root.
	require.NoError(t, os.WriteFile(filepath.Join(rootPath, "terragrunt.hcl"), []byte(""), 0644))

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)
	require.Error(t, err)

	var coexistErr discovery.CoexistenceError
	require.ErrorAs(t, err, &coexistErr)
}

func TestStacksGenerateLocals(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksLocals)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksLocals)
	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpEnvPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksLocals, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)
}

func TestStacksGenerateLocalsError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksLocalsError)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksLocalsError)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksLocalsError)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)
	require.Error(t, err)
}

func TestStacksRunParseErrorNotSilentlySkipped(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksUnknownValueError)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksUnknownValueError)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksUnknownValueError, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack run plan --non-interactive --working-dir "+rootPath,
	)

	// Command should fail with parsing error, not silently skip the unit
	require.Error(t, err)
	assert.Contains(t, stderr, "missing_var")
}

func TestStacksGenerateRemote(t *testing.T) {
	t.Parallel()

	mirror := helpers.NewGitServer(t)
	helpers.CleanupTerraformFolder(t, testFixtureStacksRemote)
	tmpEnvPath := mirror.RenderFixture(testFixtureStacksRemote)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksRemote)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	path := filepath.Join(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksNoGenerate(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksBasic, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	path := filepath.Join(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	// clean .terragrunt-stack contents
	entries, err := os.ReadDir(path)
	require.NoError(t, err)

	for _, entry := range entries {
		err = os.RemoveAll(filepath.Join(path, entry.Name()))
		require.NoError(t, err)
	}

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack run apply --no-stack-generate --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, "No units discovered. Creating an empty runner.")
}

func TestStackCleanRecursively(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	gitPath := filepath.Join(tmpEnvPath, testFixtureNestedStacks)
	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	live := filepath.Join(gitPath, "live")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+live,
	)
	require.NoError(t, err)

	liveV2 := filepath.Join(gitPath, "live-v2")
	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+liveV2,
	)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack clean --working-dir "+gitPath,
	)
	require.NoError(t, err)

	assert.NoDirExists(t, filepath.Join(live, ".terragrunt-stack"))
	assert.NoDirExists(t, filepath.Join(liveV2, ".terragrunt-stack"))

	assert.Contains(t, stderr, "Deleting stack directory: live/.terragrunt-stack")
	assert.Contains(t, stderr, "Deleting stack directory: live-v2/.terragrunt-stack")
}

func TestStacksUnitEmptyPathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksUnitEmptyPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksUnitEmptyPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksUnitEmptyPath, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)
	require.Error(t, err)

	message := err.Error()
	// check for app1 and app2 empty path error
	assert.Contains(t, message, "unit 'app1_empty_path' has empty path")
	assert.Contains(t, message, "unit 'app2_empty_path' has empty path")
	assert.NotContains(t, message, "unit 'app3_not_empty_path' has empty path")
}

func TestStackStackEmptyPathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksEmptyPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksEmptyPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksEmptyPath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)
	require.Error(t, err)

	message := err.Error()
	assert.Contains(t, message, "stack 'prod' has empty path")
}

func TestStackValuesGeneration(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackValues)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackValues)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackValues)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := filepath.Join(gitPath, "live")
	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	path := filepath.Join(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	// check that is generated terragrunt.values.hcl
	valuesPath := filepath.Join(path, "dev", "terragrunt.values.hcl")
	assert.FileExists(t, valuesPath)
}

func TestStacksGenerateParallelism(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDependencies)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDependencies)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDependencies, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --parallelism 10 --working-dir "+rootPath)

	path := filepath.Join(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)
}

func TestStacksGenerateAbsolutePathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackAbsolutePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackAbsolutePath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackAbsolutePath, "live")

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --log-level debug --working-dir "+rootPath,
	)

	require.Error(t, err)
}

func TestStacksGenerateIncorrectSource(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackIncorrectSource)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackIncorrectSource)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackIncorrectSource, "live")

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --log-level debug --working-dir "+rootPath,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch unit api")
}

func TestStacksGenerateRelativePathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackRelativePathOutsideOfStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackRelativePathOutsideOfStack)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackRelativePathOutsideOfStack, "live")

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --log-level debug --working-dir "+rootPath,
	)

	require.Error(t, err)

	assert.Contains(t, err.Error(), "app1 destination path")
	assert.Contains(t, err.Error(), "is outside of the stack directory")
}

func TestStacksGenerateNoStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNoStack)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoStack)
	gitPath := filepath.Join(tmpEnvPath, testFixtureNoStack)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := filepath.Join(gitPath, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	validateNoStackDirs(t, rootPath)
}

func TestStacksNoStackDirDirectoryCreated(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNoStackNoDir)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoStackNoDir)
	rootPath := filepath.Join(tmpEnvPath, testFixtureNoStackNoDir, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	path := filepath.Join(rootPath, ".terragrunt-stack")
	// validate that the stack directory is not created
	assert.NoDirExists(t, path)
}

func TestStacksGeneratePrintWarning(t *testing.T) {
	t.Parallel()

	rootPath := helpers.TmpDirWOSymlinks(t)
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)
	assert.Contains(t, stderr, "No stack files found")
	require.NoError(t, err)
}

func TestStacksNotExistingPathError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackNotExist)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackNotExist)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackNotExist)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)
	require.Error(t, err)
}

func TestStacksGenerateMultipleStacks(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureMultipleStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMultipleStacks)
	rootPath := filepath.Join(tmpEnvPath, testFixtureMultipleStacks)

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	devStack := filepath.Join(rootPath, "dev", ".terragrunt-stack")
	validateStackDir(t, devStack)

	liveStack := filepath.Join(rootPath, "live", ".terragrunt-stack")
	validateStackDir(t, liveStack)
}

func TestStackUnitValidation(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackValidationUnitPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackValidationUnitPath)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackValidationUnitPath)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := filepath.Join(gitPath, "live")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --no-stack-validate --working-dir "+rootPath,
	)
	require.NoError(t, err)

	liveStack := filepath.Join(rootPath, ".terragrunt-stack")
	validateStackDir(t, liveStack)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed for unit v1")
	assert.Contains(
		t,
		err.Error(),
		"expected unit to generate with terragrunt.hcl file at root of generated directory",
	)
}

func TestStackValidation(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackValidationStackPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackValidationStackPath)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackValidationStackPath)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(gitPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	rootPath := filepath.Join(gitPath, "live")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --no-stack-validate --working-dir "+rootPath,
	)
	require.NoError(t, err)

	liveStack := filepath.Join(rootPath, ".terragrunt-stack")
	validateStackDir(t, liveStack)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "validation failed for stack stack-v1")
	assert.Contains(
		t,
		err.Error(),
		"expected stack to generate with terragrunt.stack.hcl file at root of generated directory",
	)
}

// validateNoStackDirs check if the directories outside of stack are created and contain test files
func validateNoStackDirs(t *testing.T, rootPath string) {
	t.Helper()

	stackConfig := filepath.Join(rootPath, "stack-config")
	assert.DirExists(t, stackConfig)

	unitConfig := filepath.Join(rootPath, "unit-config")
	assert.DirExists(t, unitConfig)

	configPath := filepath.Join(stackConfig, "config.txt")
	assert.FileExists(t, configPath)

	configPath = filepath.Join(unitConfig, "config.txt")
	assert.FileExists(t, configPath)

	secondStackUnitConfigDir := filepath.Join(
		rootPath,
		".terragrunt-stack",
		"dev",
		"second-stack-unit-config",
	)
	secondStackUnitConfig := filepath.Join(secondStackUnitConfigDir, "config.txt")

	assert.DirExists(t, secondStackUnitConfigDir)
	assert.FileExists(t, secondStackUnitConfig)
}

// check if the stack directory is created and contains files.
func validateStackDir(t *testing.T, path string) {
	t.Helper()
	assert.DirExists(t, path)

	// check that path is not empty directory
	entries, err := os.ReadDir(path)
	require.NoError(t, err, "Failed to read directory contents")

	hasSubdirectories := false

	for _, entry := range entries {
		if entry.IsDir() {
			hasSubdirectories = true

			break
		}
	}

	assert.True(
		t,
		hasSubdirectories,
		"The .terragrunt-stack directory should contain at least one subdirectory",
	)
}

func TestStackOriginalTerragruntDir(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackOriginalTerragruntDir)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackOriginalTerragruntDir)
	gitPath := filepath.Join(tmpEnvPath, testFixtureStackOriginalTerragruntDir)
	helpers.CreateGitRepo(t, gitPath)
	rootPath := filepath.Join(gitPath, "live")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+rootPath)

	var valuesFiles []string

	const (
		valuesFileName  = "terragrunt.values.hcl"
		dotStackDirName = ".terragrunt-stack"
		nestedUnitDirs  = "unit_dirs"
	)

	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)

		if d.IsDir() {
			return nil
		}

		if filepath.Base(path) == valuesFileName {
			valuesFiles = append(valuesFiles, path)
		}

		return nil
	})

	require.NoError(t, err)
	require.NotEmpty(t, valuesFiles)

	for _, valuesPath := range valuesFiles {
		content, readErr := os.ReadFile(valuesPath)
		require.NoError(t, readErr)

		before, _, ok := strings.Cut(
			valuesPath,
			string(os.PathSeparator)+dotStackDirName+string(os.PathSeparator),
		)
		if !ok {
			continue
		}

		expected := before

		isNoLocals := strings.Contains(valuesPath, "no-locals")
		isNestedReadConfig := strings.Contains(valuesPath, "read-config-nested")
		isNestedLocals := strings.Contains(valuesPath, "with-locals-nested")

		if isNoLocals && (isNestedReadConfig || isNestedLocals) {
			// In these fixtures we intentionally validate the "no locals + nested stacks" behavior whereby a user,
			// attempts to invoke the get_original_terragrunt_dir() function within the values block. Due to the order
			// of evaluation this scenario will resolve to the generated child stack directory rather than the parent
			// stack root. If users intend to acquire the parent stack directory at generate time they must do it from
			// the locals block either directly or in another config evaluated via read_terragrunt_config().
			expected = filepath.Join(expected, dotStackDirName, nestedUnitDirs)
		}

		expected = filepath.ToSlash(expected)

		assert.Contains(
			t,
			string(content),
			`stack_dir = "`+expected+`"`,
			"wrong stack_dir in %s",
			valuesPath,
		)
	}
}

func TestStackGenerateWithFilter(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStacks)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStacks)
	rootPath := filepath.Join(tmpEnvPath, testFixtureNestedStacks)
	liveDir := filepath.Join(rootPath, "live")

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --working-dir "+liveDir,
	)

	stackDir := filepath.Join(liveDir, ".terragrunt-stack")
	require.DirExists(t, stackDir)

	devDir := filepath.Join(stackDir, "dev", ".terragrunt-stack")
	require.DirExists(t, devDir)

	prodDir := filepath.Join(stackDir, "prod", ".terragrunt-stack")
	require.DirExists(t, prodDir)

	require.NoError(t, os.RemoveAll(stackDir))

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --working-dir "+liveDir+" --filter 'live | type=stack' --filter 'dev | type=stack'",
	)

	stackDir = filepath.Join(liveDir, ".terragrunt-stack")
	require.DirExists(t, stackDir)

	devDir = filepath.Join(stackDir, "dev", ".terragrunt-stack")
	require.DirExists(t, devDir)

	prodDir = filepath.Join(stackDir, "prod", ".terragrunt-stack")
	require.NoDirExists(t, prodDir)

	require.NoError(t, os.RemoveAll(stackDir))

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --working-dir "+liveDir+" --filter 'live | type=stack' --filter 'prod | type=stack'",
	)

	stackDir = filepath.Join(liveDir, ".terragrunt-stack")
	require.DirExists(t, stackDir)

	devDir = filepath.Join(stackDir, "dev", ".terragrunt-stack")
	require.NoDirExists(t, devDir)

	prodDir = filepath.Join(stackDir, "prod", ".terragrunt-stack")
	require.DirExists(t, prodDir)
}

// TestStackGenerateFilterNestedStacksTip verifies that a literal `<path> | type=stack`
// filter on a stack-of-stacks prints a tip on how to also generate the nested stacks.
func TestStackGenerateFilterNestedStacksTip(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStackFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStackFilter)
	rootPath := filepath.Join(tmpEnvPath, testFixtureNestedStackFilter)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)
	require.NoError(t, runner.Init(t.Context()))

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath+" --filter './stacks/first | type=stack'",
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, tips.StackNestedStacksNotGenerated)
	assert.Contains(t, stderr, "./stacks/first | type=stack")
	assert.Contains(t, stderr, "./stacks/first/** | type=stack")
}

// TestStackGenerateFilterRecursiveNoTip verifies the tip is not shown once the nested
// stacks are recursively generated (here via the suggested recursive glob filter).
func TestStackGenerateFilterRecursiveNoTip(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureNestedStackFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNestedStackFilter)
	rootPath := filepath.Join(tmpEnvPath, testFixtureNestedStackFilter)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(rootPath)
	require.NoError(t, runner.Init(t.Context()))

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath+
			" --filter './stacks/first | type=stack' --filter './stacks/first/** | type=stack'",
	)
	require.NoError(t, err)

	assert.NotContains(t, stderr, tips.StackNestedStacksNotGenerated)
}

// TestStackGenerateDedupAtDiscoveryWithRacing guards intra-invocation duplicate-dispatch under -race.
func TestStackGenerateDedupAtDiscoveryWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	setupNestedStackFixture(t, tmpDir)

	liveDir := filepath.Join(tmpDir, "live")

	for range 2 {
		_, _, err := helpers.RunTerragruntCommandWithOutput(t,
			"terragrunt stack generate --working-dir "+liveDir)
		require.NoError(t, err)
	}

	verifyGeneratedUnits(t, filepath.Join(liveDir, ".terragrunt-stack"))
}

func TestStackGenerationWithNestedTopologyWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	setupNestedStackFixture(t, tmpDir)

	liveDir := filepath.Join(tmpDir, "live")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+liveDir,
	)
	require.NoError(t, err)

	stackDir := filepath.Join(liveDir, ".terragrunt-stack")
	require.DirExists(t, stackDir)

	foundFiles := findStackFiles(t, liveDir)
	require.NotEmpty(t, foundFiles, "Expected to find generated stack files")

	l := logger.CreateLogger()
	topology := generate.BuildStackTopology(l, foundFiles, liveDir)
	require.NotEmpty(t, topology, "Expected non-empty topology")

	levelCounts := make(map[int]int)
	for _, node := range topology {
		levelCounts[node.Level]++
	}

	t.Logf("Topology levels found: %v", levelCounts)

	assert.Len(t, levelCounts, 3, "Expected levels in nested topology")

	assert.Equal(t, 1, levelCounts[0], "Level 0 should have exactly 1 stack file")
	assert.Equal(t, 3, levelCounts[1], "Level 1 should have exactly 3 stack files")
	assert.Equal(t, 9, levelCounts[2], "Level 2 should have exactly 9 stack files")

	verifyGeneratedUnits(t, stackDir)

	// Run one more time just to be sure things don't break when running in a dirty directory
	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+liveDir,
	)
	require.NoError(t, err)
}

// setupNestedStackFixture creates a test fixture similar to testing-nested-stacks
func setupNestedStackFixture(t *testing.T, tmpDir string) {
	t.Helper()

	liveDir := filepath.Join(tmpDir, "live")
	stacksDir := filepath.Join(tmpDir, "stacks")
	unitsDir := filepath.Join(tmpDir, "units")

	require.NoError(t, os.MkdirAll(liveDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(stacksDir, "foo"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(stacksDir, "final"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(unitsDir, "final"), 0755))

	liveStackConfig := `stack "foo" {
  source = "../stacks/foo"
  path   = "foo"
}

stack "foo2" {
  source = "../stacks/foo"
  path   = "foo2"
}

stack "foo3" {
  source = "../stacks/foo"
  path   = "foo3"
}
`
	liveStackPath := filepath.Join(liveDir, config.DefaultStackFile)
	require.NoError(t, os.WriteFile(liveStackPath, []byte(liveStackConfig), 0644))

	fooStackConfig := `locals {
  final_stack = find_in_parent_folders("stacks/final")
}

stack "final" {
  source = local.final_stack
  path   = "final"
}

stack "final2" {
  source = local.final_stack
  path   = "final2"
}

stack "final3" {
  source = local.final_stack
  path   = "final3"
}
`
	fooStackPath := filepath.Join(stacksDir, "foo", config.DefaultStackFile)
	require.NoError(t, os.WriteFile(fooStackPath, []byte(fooStackConfig), 0644))

	finalStackConfig := `locals {
  final_unit = find_in_parent_folders("units/final")
}

unit "final" {
  source = local.final_unit
  path   = "final"
}
`
	finalStackPath := filepath.Join(stacksDir, "final", config.DefaultStackFile)
	require.NoError(t, os.WriteFile(finalStackPath, []byte(finalStackConfig), 0644))

	finalUnitPath := filepath.Join(unitsDir, "final", config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(finalUnitPath, []byte(``), 0644))

	finalMainTfPath := filepath.Join(unitsDir, "final", "main.tf")
	require.NoError(t, os.WriteFile(finalMainTfPath, []byte(``), 0644))
}

// verifyGeneratedUnits checks that some units were generated correctly
func verifyGeneratedUnits(t *testing.T, stackDir string) {
	t.Helper()

	var (
		unitDirs  []string
		stackDirs []string
	)

	err := filepath.WalkDir(stackDir, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.Name() == "terragrunt.hcl" {
			unitDir := filepath.Dir(path)
			unitDirs = append(unitDirs, unitDir)
		}

		if !info.IsDir() && info.Name() == "terragrunt.stack.hcl" {
			stackDir := filepath.Dir(path)
			stackDirs = append(stackDirs, stackDir)
		}

		return nil
	})
	require.NoError(t, err)

	require.Len(t, unitDirs, 9, "Expected exactly 9 generated units")
	require.Len(t, stackDirs, 12, "Expected exactly 12 generated stacks")
}

// findStackFiles recursively finds all terragrunt.stack.hcl files in a directory
func findStackFiles(t *testing.T, dir string) []string {
	t.Helper()

	var stackFiles []string

	err := filepath.WalkDir(dir, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, "terragrunt.stack.hcl") {
			stackFiles = append(stackFiles, path)
		}

		return nil
	})

	require.NoError(t, err)

	return stackFiles
}

// TestCASInStacksRejectsUpdateSourceWithCASWithNoCAS verifies that stack generation
// errors when a unit or stack block declares update_source_with_cas = true while
// --no-cas is set. CAS is enabled by default, so --no-cas is the way to disable it
// for this scenario.
func TestCASInStacksRejectsUpdateSourceWithCASWithNoCAS(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		stackConfig string
		wantName    string
	}{
		{
			name: "unit block",
			stackConfig: `unit "bar" {
  source = "../units/bar"
  path   = "bar"

  update_source_with_cas = true
}
`,
			wantName: `"bar"`,
		},
		{
			name: "stack block",
			stackConfig: `stack "nested" {
  source = "../nested"
  path   = "nested"

  update_source_with_cas = true
}
`,
			wantName: `"nested"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmp := helpers.TmpDirWOSymlinks(t)
			liveDir := filepath.Join(tmp, "live")
			require.NoError(t, os.MkdirAll(liveDir, 0755))
			require.NoError(
				t,
				os.WriteFile(
					filepath.Join(liveDir, "terragrunt.stack.hcl"),
					[]byte(tc.stackConfig),
					0644,
				),
			)

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt stack generate --no-cas --non-interactive --working-dir "+liveDir,
			)
			require.Error(t, err)

			combined := stderr + err.Error()
			assert.Contains(t, combined, "update_source_with_cas")
			assert.Contains(t, combined, tc.wantName)
		})
	}
}

// TestCASInStacksRejectsTerraformUpdateSourceWithCASWithNoCAS verifies that stack
// generation errors when a unit's terraform block declares update_source_with_cas = true
// while --no-cas is set. Unlike the unit/stack-block attribute (validated up front from the
// stack file), the terraform-block attribute lives in the unit's own terragrunt.hcl and is
// only visible once the source is materialized, so it is checked during generation. Without
// the check the relative source would be copied verbatim and silently fail to resolve.
func TestCASInStacksRejectsTerraformUpdateSourceWithCASWithNoCAS(t *testing.T) {
	t.Parallel()

	catalog := helpers.TmpDirWOSymlinks(t)
	liveDir := helpers.TmpDirWOSymlinks(t)

	writeFile := func(rel, body string) {
		full := filepath.Join(catalog, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0644))
	}

	writeFile("modules/baz/main.tf", ``)
	writeFile("units/bar/terragrunt.hcl", `terraform {
  source = "../..//modules/baz"

  update_source_with_cas = true
}
`)

	// The unit block itself does NOT set update_source_with_cas, so the only opt-in is the
	// terraform block inside the materialized unit.
	liveStack := fmt.Sprintf(`unit "bar" {
  source = %s
  path   = "bar"
}
`, strconv.Quote(filepath.ToSlash(catalog)+"//units/bar"))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(liveDir, "terragrunt.stack.hcl"), []byte(liveStack), 0644),
	)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --no-cas --non-interactive --working-dir "+liveDir,
	)
	require.Error(t, err)

	combined := stderr + err.Error()
	assert.Contains(t, combined, "update_source_with_cas")
	assert.Contains(t, combined, "terraform")
}

// readCachedFiles returns the contents of every file with the given name
// found inside a .terragrunt-cache directory under root.
func readCachedFiles(t *testing.T, root, fileName string) []string {
	t.Helper()

	var contents []string

	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || d.Name() != fileName {
			return nil
		}

		if !strings.Contains(path, ".terragrunt-cache") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		contents = append(contents, string(content))

		return nil
	}))

	return contents
}
