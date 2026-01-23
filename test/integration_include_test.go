package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	includeDeepFixturePath       = "fixtures/include-deep/"
	includeDeepFixtureChildPath  = "child"
	includeFixturePath           = "fixtures/include/"
	includeShallowFixturePath    = "stage/my-app"
	includeNoMergeFixturePath    = "qa/my-app"
	includeExposeFixturePath     = "fixtures/include-expose/"
	includeChildFixturePath      = "child"
	includeMultipleFixturePath   = "fixtures/include-multiple/"
	includeRunAllFixturePath     = "fixtures/include-runall/"
	rootTerragruntHCLFixturePath = "fixtures/root-terragrunt-hcl-regression/"
)

func TestTerragruntWorksWithIncludeLocals(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, includeExposeFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, includeExposeFixturePath)
	tmpEnvPath = filepath.Join(tmpEnvPath, includeExposeFixturePath)

	files, err := os.ReadDir(tmpEnvPath)
	require.NoError(t, err)

	testCases := []string{}

	for _, finfo := range files {
		if finfo.IsDir() {
			testCases = append(testCases, finfo.Name())
		}
	}

	for _, tc := range testCases {
		t.Run(filepath.Base(tc), func(t *testing.T) {
			t.Parallel()

			childPath := filepath.Join(tmpEnvPath, tc, includeChildFixturePath)
			helpers.CleanupTerraformFolder(t, childPath)
			helpers.RunTerragrunt(t, "terragrunt run --all --queue-include-external --non-interactive --working-dir "+childPath+" -- apply -auto-approve")

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}
			err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr)
			require.NoError(t, err)

			outputs := map[string]helpers.TerraformOutput{}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
			assert.Equal(t, "us-west-1-test", outputs["region"].Value.(string))
		})
	}
}
func TestTerragruntWorksWithIncludeLocalsWithFilter(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, includeExposeFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, includeExposeFixturePath)
	tmpEnvPath = filepath.Join(tmpEnvPath, includeExposeFixturePath)

	files, err := os.ReadDir(tmpEnvPath)
	require.NoError(t, err)

	testCases := []string{}

	for _, finfo := range files {
		if finfo.IsDir() {
			testCases = append(testCases, finfo.Name())
		}
	}

	for _, tc := range testCases {
		t.Run(filepath.Base(tc), func(t *testing.T) {
			t.Parallel()

			childPath := filepath.Join(tmpEnvPath, tc, includeChildFixturePath)
			helpers.CleanupTerraformFolder(t, childPath)
			helpers.RunTerragrunt(t, "terragrunt run --all --filter '{./**}...' --non-interactive --working-dir "+childPath+" -- apply -auto-approve")

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}
			err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr)
			require.NoError(t, err)

			outputs := map[string]helpers.TerraformOutput{}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
			assert.Equal(t, "us-west-1-test", outputs["region"].Value.(string))
		})
	}
}

func TestTerragruntFilterReadingRestrictsSet(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, includeRunAllFixturePath)
	modulePath := filepath.Join(rootPath, includeRunAllFixturePath)
	helpers.CleanupTerraformFolder(t, modulePath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --non-interactive --working-dir "+modulePath+" --filter 'reading=alpha.hcl'",
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, "alpha")
	assert.NotContains(t, stdout, "beta")
	assert.NotContains(t, stdout, "charlie")
}

func TestTerragruntRunAllModulesWithPrefix(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, includeRunAllFixturePath)
	modulePath := filepath.Join(rootPath, includeRunAllFixturePath)
	helpers.CleanupTerraformFolder(t, modulePath)

	// Retry to handle intermittent failures due to network issues on CICD
	retry.DoWithRetry(t, "Run all modules with prefix verification", 3, 0, func() (string, error) {
		helpers.CleanupTerraformFolder(t, modulePath)

		stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
			t,
			"terragrunt run --all plan --non-interactive --tf-forward-stdout --working-dir "+modulePath,
		)
		if err != nil {
			return "", fmt.Errorf("command failed: %w", err)
		}

		// Check if all expected outputs are present
		hasAlpha := strings.Contains(stdout, "alpha")
		hasBeta := strings.Contains(stdout, "beta")
		hasCharlie := strings.Contains(stdout, "charlie")

		if !hasAlpha || !hasBeta || !hasCharlie {
			return "", fmt.Errorf("missing outputs: alpha=%v, beta=%v, charlie=%v", hasAlpha, hasBeta, hasCharlie)
		}

		// All outputs present, verify prefixes
		stdoutLines := strings.SplitSeq(stderr, "\n")
		for line := range stdoutLines {
			if strings.Contains(line, "alpha") && !strings.Contains(line, "prefix=a") {
				return "", fmt.Errorf("alpha found but wrong prefix in line: %s", line)
			}

			if strings.Contains(line, "beta") && !strings.Contains(line, "prefix=b") {
				return "", fmt.Errorf("beta found but wrong prefix in line: %s", line)
			}

			if strings.Contains(line, "charlie") && !strings.Contains(line, "prefix=c") {
				return "", fmt.Errorf("charlie found but wrong prefix in line: %s", line)
			}
		}

		return "Success", nil
	})
}

func TestTerragruntWorksWithIncludeDeepMerge(t *testing.T) {
	t.Parallel()

	childPath := filepath.Join(includeDeepFixturePath, "child")
	helpers.CleanupTerraformFolder(t, childPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "mock", outputs["attribute"].Value.(string))
	assert.Equal(t, "new val", outputs["new_attribute"].Value.(string))
	assert.Equal(t, "old val", outputs["old_attribute"].Value.(string))
	assert.Equal(t, []any{"hello", "mock"}, outputs["list_attr"].Value.([]any))
	assert.Equal(t, map[string]any{"foo": "bar", "bar": "baz", "test": "new val"}, outputs["map_attr"].Value.(map[string]any))

	assert.Equal(
		t,
		map[string]any{
			"attribute":     "mock",
			"new_attribute": "new val",
			"old_attribute": "old val",
			"list_attr":     []any{"hello", "mock"},
			"map_attr": map[string]any{
				"foo": "bar",
				"bar": "baz",
			},
		},
		outputs["dep_out"].Value.(map[string]any),
	)
}

func TestTerragruntWorksWithMultipleInclude(t *testing.T) {
	t.Parallel()

	files, err := os.ReadDir(includeMultipleFixturePath)
	require.NoError(t, err)

	testCases := []string{}

	for _, finfo := range files {
		if finfo.IsDir() && filepath.Base(finfo.Name()) != "modules" {
			testCases = append(testCases, finfo.Name())
		}
	}

	for _, tc := range testCases {
		t.Run(filepath.Base(tc), func(t *testing.T) {
			t.Parallel()

			childPath := filepath.Join(includeMultipleFixturePath, tc, includeDeepFixtureChildPath)
			helpers.CleanupTerraformFolder(t, childPath)
			helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath)

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}
			err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr)
			require.NoError(t, err)

			outputs := map[string]helpers.TerraformOutput{}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
			validateMultipleIncludeTestOutput(t, outputs)
		})
	}
}

func TestTerragruntWorksWithRootTerragruntHCL(t *testing.T) {
	t.Parallel()

	// This is a regression test to ensure that users can still have a root terragrunt.hcl at the project root,
	// and run Terragrunt from that root, as long as they exclude that directory from the queue.
	//
	// In this fixture, the child units include the root config via:
	//   path = find_in_parent_folders("terragrunt.hcl")
	//
	// The key requirement is: excluding "." should prevent Terragrunt from trying to treat the root directory
	// itself as a unit, while still allowing the root terragrunt.hcl to be used for includes.
	tmpEnvPath := helpers.CopyEnvironment(t, rootTerragruntHCLFixturePath)
	rootPath := filepath.Join(tmpEnvPath, rootTerragruntHCLFixturePath)
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	reportFile := filepath.Join(rootPath, helpers.ReportFile)

	command := fmt.Sprintf(
		"terragrunt run --all --non-interactive --working-dir %s --queue-exclude-dir=. --report-file %s --report-format json -- plan",
		rootPath,
		reportFile,
	)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, command)
	require.NoError(t, err)

	// Validate the report file against the schema
	err = report.ValidateJSONReportFromFile(reportFile)
	require.NoError(t, err, "Report should pass schema validation")

	// Parse the report file to verify the correct units ran
	runs, err := report.ParseJSONRunsFromFile(reportFile)
	require.NoError(t, err, "Should be able to parse JSON report")

	runNames := runs.Names()

	// Verify exactly 3 child units ran (bar, baz, foo)
	assert.Len(t, runs, 3, "Expected exactly 3 units in report. Found: %v", runNames)

	// Verify each child unit was discovered and executed successfully
	barRun := runs.FindByName("bar")
	require.NotNil(t, barRun, "Expected bar unit to be in report. Found: %v", runNames)
	assert.Equal(t, "succeeded", barRun.Result)

	bazRun := runs.FindByName("baz")
	require.NotNil(t, bazRun, "Expected baz unit to be in report. Found: %v", runNames)
	assert.Equal(t, "succeeded", bazRun.Result)

	fooRun := runs.FindByName("foo")
	require.NotNil(t, fooRun, "Expected foo unit to be in report. Found: %v", runNames)
	assert.Equal(t, "succeeded", fooRun.Result)

	// Root should not be treated as a runnable unit (the "." directory should be excluded)
	rootRun := runs.FindByName(".")
	assert.Nil(t, rootRun, "Root directory should not be in the report as it should be excluded")
}

func validateMultipleIncludeTestOutput(t *testing.T, outputs map[string]helpers.TerraformOutput) {
	t.Helper()

	assert.Equal(t, "mock", outputs["attribute"].Value.(string))
	assert.Equal(t, "new val", outputs["new_attribute"].Value.(string))
	assert.Equal(t, "old val", outputs["old_attribute"].Value.(string))
	assert.Equal(t, []any{"hello", "mock", "foo"}, outputs["list_attr"].Value.([]any))
	assert.Equal(t, map[string]any{"foo": "bar", "bar": "baz", "test": "new val"}, outputs["map_attr"].Value.(map[string]any))

	assert.Equal(
		t,
		map[string]any{
			"attribute":     "mock",
			"new_attribute": "new val",
			"old_attribute": "old val",
			"list_attr":     []any{"hello", "mock", "foo"},
			"map_attr": map[string]any{
				"foo": "bar",
				"bar": "baz",
			},
		},
		outputs["dep_out"].Value.(map[string]any),
	)
}
