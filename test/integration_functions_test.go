package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureStartswith              = "fixtures/startswith"
	testFixtureTimecmp                 = "fixtures/timecmp"
	testFixtureTimecmpInvalidTimestamp = "fixtures/timecmp-errors/invalid-timestamp"
	testFixtureEndswith                = "fixtures/endswith"
	testFixtureStrcontains             = "fixtures/strcontains"
	testFixtureGetRepoRoot             = "fixtures/get-repo-root"
	testFixtureGetWorkingDir           = "fixtures/get-working-dir"
	testFixtureRelativeIncludeCmd      = "fixtures/relative-include-cmd"
	testFixturePathRelativeFromInclude = "fixtures/get-path/path_relative_from_include"
	testFixtureGetPathFromRepoRoot     = "fixtures/get-path/get-path-from-repo-root"
	testFixtureGetPathToRepoRoot       = "fixtures/get-path/get-path-to-repo-root"
	testFixtureGetPlatform             = "fixtures/get-platform"
)

func TestStartsWith(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStartswith)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStartswith)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStartswith)

	helpers.RunTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	validateOutput(t, outputs, "startswith1", true)
	validateOutput(t, outputs, "startswith2", false)
	validateOutput(t, outputs, "startswith3", true)
	validateOutput(t, outputs, "startswith4", false)
	validateOutput(t, outputs, "startswith5", true)
	validateOutput(t, outputs, "startswith6", false)
	validateOutput(t, outputs, "startswith7", true)
	validateOutput(t, outputs, "startswith8", false)
	validateOutput(t, outputs, "startswith9", false)
}

func TestTimeCmp(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureTimecmp)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTimecmp)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureTimecmp)

	helpers.RunTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	validateOutput(t, outputs, "timecmp1", float64(0))
	validateOutput(t, outputs, "timecmp2", float64(0))
	validateOutput(t, outputs, "timecmp3", float64(1))
	validateOutput(t, outputs, "timecmp4", float64(-1))
	validateOutput(t, outputs, "timecmp5", float64(-1))
	validateOutput(t, outputs, "timecmp6", float64(1))
}

func TestTimeCmpInvalidTimestamp(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureTimecmpInvalidTimestamp)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTimecmpInvalidTimestamp)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureTimecmpInvalidTimestamp)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

	expectedError := `not a valid RFC3339 timestamp: missing required time introducer 'T'`
	require.ErrorContains(t, err, expectedError)
}

func TestEndsWith(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureEndswith)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEndswith)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureEndswith)

	helpers.RunTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	validateOutput(t, outputs, "endswith1", true)
	validateOutput(t, outputs, "endswith2", false)
	validateOutput(t, outputs, "endswith3", true)
	validateOutput(t, outputs, "endswith4", false)
	validateOutput(t, outputs, "endswith5", true)
	validateOutput(t, outputs, "endswith6", false)
	validateOutput(t, outputs, "endswith7", true)
	validateOutput(t, outputs, "endswith8", false)
	validateOutput(t, outputs, "endswith9", false)
}

func TestStrContains(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStrcontains)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStrcontains)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStrcontains)

	helpers.RunTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	validateOutput(t, outputs, "o1", true)
	validateOutput(t, outputs, "o2", false)
}

func TestGetRepoRootCaching(t *testing.T) {
	t.Parallel()
	helpers.CleanupTerraformFolder(t, testFixtureGetRepoRoot)
	tmpEnvPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureGetRepoRoot))
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetRepoRoot)

	gitOutput, err := exec.Command("git", "init", rootPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(gitOutput))
	}

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	output := fmt.Sprintf("%s %s", stdout, stderr)
	count := strings.Count(output, "git show-toplevel result")
	assert.Equal(t, 1, count)
}

func TestGetRepoRoot(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetRepoRoot)
	tmpEnvPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureGetRepoRoot))
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetRepoRoot)

	output, err := exec.Command("git", "init", rootPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}
	helpers.RunTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	repoRoot, ok := outputs["repo_root"]

	assert.True(t, ok)
	assert.Regexp(t, "/tmp/terragrunt-.*/fixtures/get-repo-root", repoRoot.Value)
}

func TestGetWorkingDirBuiltInFunc(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetWorkingDir)
	tmpEnvPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureGetWorkingDir))
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetWorkingDir)

	output, err := exec.Command("git", "init", rootPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}
	helpers.RunTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	workingDir, ok := outputs["working_dir"]

	expectedWorkingDir := filepath.Join(rootPath, util.TerragruntCacheDir)
	curWalkStep := 0

	err = filepath.Walk(expectedWorkingDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.IsDir() {
				return err
			}

			expectedWorkingDir = path

			if curWalkStep == 2 {
				return filepath.SkipDir
			}
			curWalkStep++

			return nil
		})
	require.NoError(t, err)

	assert.True(t, ok)
	assert.Equal(t, expectedWorkingDir, workingDir.Value)
}

func TestPathRelativeToIncludeInvokedInCorrectPathFromChild(t *testing.T) {
	t.Parallel()

	appPath := path.Join(testFixtureRelativeIncludeCmd, "app")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt version --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir "+appPath, &stdout, &stderr)
	require.NoError(t, err)
	output := stdout.String()
	assert.Equal(t, 1, strings.Count(output, "path_relative_to_inclue: app\n"))
	assert.Equal(t, 0, strings.Count(output, "path_relative_to_inclue: .\n"))
}

func TestPathRelativeFromInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixturePathRelativeFromInclude)
	tmpEnvPath, err := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixturePathRelativeFromInclude))
	require.NoError(t, err)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePathRelativeFromInclude, "lives/dev")
	basePath := util.JoinPath(rootPath, "base")
	clusterPath := util.JoinPath(rootPath, "cluster")

	output, err := exec.Command("git", "init", tmpEnvPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}

	helpers.RunTerragrunt(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+clusterPath)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	val, hasVal := outputs["some_output"]
	assert.True(t, hasVal)
	assert.Equal(t, "something else", val.Value)

	// try to destroy module and check if warning is printed in output, also test `get_parent_terragrunt_dir()` func in the parent terragrunt config.
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+basePath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Detected dependent modules:\n"+clusterPath)
}

func TestGetPathFromRepoRoot(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetPathFromRepoRoot)
	tmpEnvPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureGetPathFromRepoRoot))
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetPathFromRepoRoot)

	output, err := exec.Command("git", "init", tmpEnvPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}

	helpers.RunTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	pathFromRoot, hasPathFromRoot := outputs["path_from_root"]

	assert.True(t, hasPathFromRoot)
	assert.Equal(t, testFixtureGetPathFromRepoRoot, pathFromRoot.Value)
}

func TestGetPathToRepoRoot(t *testing.T) {
	t.Parallel()

	tmpEnvPath, _ := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureGetPathToRepoRoot))
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetPathToRepoRoot)
	helpers.CleanupTerraformFolder(t, rootPath)

	output, err := exec.Command("git", "init", tmpEnvPath).CombinedOutput()
	if err != nil {
		t.Fatalf("Error initializing git repo: %v\n%s", err, string(output))
	}
	helpers.RunTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	expectedToRoot, err := filepath.Rel(rootPath, tmpEnvPath)
	require.NoError(t, err)

	for name, expected := range map[string]string{
		"path_to_root":    expectedToRoot,
		"path_to_modules": filepath.Join(expectedToRoot, "modules"),
	} {
		value, hasValue := outputs[name]

		assert.True(t, hasValue)
		assert.Equal(t, expected, value.Value)
	}
}

func TestGetPlatform(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetPlatform)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetPlatform)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetPlatform)

	helpers.RunTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	platform, hasPlatform := outputs["platform"]
	assert.True(t, hasPlatform)
	assert.Equal(t, runtime.GOOS, platform.Value)
}
