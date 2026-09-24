//go:build tf

package test_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testExcludeComprehensive       = "fixtures/exclude/comprehensive"
	testExcludeNullOrUnknownString = "fixtures/exclude/null-or-unknown-string"
)

// expectedResult defines the expected outcome for a unit in a test case.
type expectedResult struct {
	result string
	reason string
}

// excludeTestCase defines a single test case for the exclude block behavior.
type excludeTestCase struct {
	expectedUnits   map[string]expectedResult
	name            string
	command         string
	workingDir      string
	featureFlags    []string
	runAll          bool
	expectEarlyExit bool
	expectRuns      bool
}

func TestTFExcludeBlockBehavior(t *testing.T) {
	t.Parallel()

	testCases := []*excludeTestCase{
		// ========== Run --all Mode Tests ==========
		{
			name:    "run_all_basic_exclusion",
			command: "plan",
			runAll:  true,
			expectedUnits: map[string]expectedResult{
				"always-excluded": {result: "excluded", reason: "exclude block"},
				"never-excluded":  {result: "succeeded"},
				"normal-unit":     {result: "succeeded"},
			},
		},
		{
			name:    "run_all_action_specific_plan",
			command: "plan",
			runAll:  true,
			expectedUnits: map[string]expectedResult{
				"exclude-plan-only":  {result: "excluded", reason: "exclude block"},
				"exclude-apply-only": {result: "succeeded"},
			},
		},
		{
			name:    "run_all_action_specific_apply",
			command: "apply -auto-approve",
			runAll:  true,
			expectedUnits: map[string]expectedResult{
				"exclude-plan-only":  {result: "succeeded"},
				"exclude-apply-only": {result: "excluded", reason: "exclude block"},
			},
		},
		{
			name:    "run_all_all_except_output_apply",
			command: "apply -auto-approve",
			runAll:  true,
			expectedUnits: map[string]expectedResult{
				"exclude-all-except-output": {result: "excluded", reason: "exclude block"},
			},
		},
		{
			name:    "run_all_all_except_output_cmd",
			command: "output",
			runAll:  true,
			expectedUnits: map[string]expectedResult{
				"exclude-all-except-output": {result: "succeeded"},
			},
		},
		{
			name:    "run_all_ignores_no_run",
			command: "plan",
			runAll:  true,
			expectedUnits: map[string]expectedResult{
				"no-run-true":  {result: "excluded", reason: "exclude block"},
				"no-run-false": {result: "excluded", reason: "exclude block"},
				"normal-unit":  {result: "succeeded"},
			},
		},
		{
			name:         "run_all_feature_flag_true",
			command:      "plan",
			runAll:       true,
			featureFlags: []string{"exclude=true"},
			expectedUnits: map[string]expectedResult{
				"conditional-flag": {result: "excluded", reason: "exclude block"},
			},
		},
		{
			name:         "run_all_feature_flag_false",
			command:      "plan",
			runAll:       true,
			featureFlags: []string{"exclude=false"},
			expectedUnits: map[string]expectedResult{
				"conditional-flag": {result: "succeeded"},
			},
		},
		{
			name:         "run_all_exclude_dependencies_true",
			command:      "plan",
			runAll:       true,
			featureFlags: []string{"exclude=true", "exclude_deps=true"},
			expectedUnits: map[string]expectedResult{
				"with-dep": {result: "excluded", reason: "exclude block"},
				"dep-unit": {result: "excluded", reason: "exclude block"},
			},
		},
		{
			name:         "run_all_exclude_dependencies_false",
			command:      "plan",
			runAll:       true,
			featureFlags: []string{"exclude=true", "exclude_deps=false"},
			expectedUnits: map[string]expectedResult{
				"with-dep": {result: "excluded", reason: "exclude block"},
				"dep-unit": {result: "succeeded"},
			},
		},

		// ========== Single Unit Mode Tests ==========
		// Single-unit mode uses stderr to verify behavior since reports are not generated
		{
			name:            "single_no_run_true_early_exit",
			command:         "plan",
			runAll:          false,
			workingDir:      "no-run-true",
			expectEarlyExit: true,
		},
		{
			name:       "single_no_run_false_runs",
			command:    "plan",
			runAll:     false,
			workingDir: "no-run-false",
			expectRuns: true,
		},
		{
			name:       "single_no_run_not_set_runs",
			command:    "plan",
			runAll:     false,
			workingDir: "no-run-not-set",
			expectRuns: true,
		},
		{
			name:       "single_action_mismatch_runs",
			command:    "apply -auto-approve",
			runAll:     false,
			workingDir: "action-mismatch",
			expectRuns: true,
		},
		{
			name:            "single_conditional_no_run_excluded",
			command:         "plan",
			runAll:          false,
			workingDir:      "conditional-no-run",
			expectEarlyExit: true,
		},
		{
			name:         "single_conditional_no_run_runs",
			command:      "plan",
			runAll:       false,
			workingDir:   "conditional-no-run",
			featureFlags: []string{"enable_unit=true"},
			expectRuns:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testExcludeComprehensive)
			tmpEnvPath := helpers.CopyEnvironment(t, testExcludeComprehensive)

			var rootPath string
			if tc.runAll {
				rootPath = filepath.Join(tmpEnvPath, testExcludeComprehensive)
			} else {
				rootPath = filepath.Join(tmpEnvPath, testExcludeComprehensive, tc.workingDir)
			}

			reportFile := filepath.Join(t.TempDir(), "report.json")

			cmd := buildExcludeTestCommand(tc, rootPath, reportFile)

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			if !tc.runAll {
				if tc.expectEarlyExit {
					assert.Contains(t, stderr, "Early exit in terragrunt unit")
					assert.Contains(t, stderr, "due to exclude block with no_run = true")
				}

				if tc.expectRuns {
					assert.NotContains(t, stderr, "Early exit in terragrunt unit")
					assert.NotContains(t, stderr, "due to exclude block with no_run = true")
				}

				return
			}

			runs, err := report.ParseJSONRunsFromFile(vfs.NewOSFS(), reportFile)
			require.NoError(t, err, "Failed to parse report file")

			for unitName, expected := range tc.expectedUnits {
				run := runs.FindByName(unitName)
				require.NotNil(
					t,
					run,
					"unit %s not found in report. Found: %v",
					unitName,
					runs.Names(),
				)
				assert.Equal(
					t,
					expected.result,
					run.Result,
					"unit %s: expected result %q, got %q",
					unitName,
					expected.result,
					run.Result,
				)

				switch expected.result {
				case "excluded":
					require.NotEmpty(
						t,
						expected.reason,
						"test bug: excluded unit %s must specify expected reason",
						unitName,
					)
					require.NotNil(
						t,
						run.Reason,
						"unit %s: expected reason %q but got nil",
						unitName,
						expected.reason,
					)
					assert.Equal(
						t,
						expected.reason,
						*run.Reason,
						"unit %s: expected reason %q, got %q",
						unitName,
						expected.reason,
						*run.Reason,
					)
				case "succeeded":
					assert.Nil(
						t,
						run.Reason,
						"unit %s: succeeded units should not have a reason, got %v",
						unitName,
						run.Reason,
					)
				default:
					require.FailNowf(t, "unexpected result", "Unexpected result %q for unit %s", expected.result, unitName)
				}
			}
		})
	}
}

// buildExcludeTestCommand constructs the terragrunt command for a test case.
func buildExcludeTestCommand(tc *excludeTestCase, rootPath, reportFile string) string {
	if tc.runAll {
		cmd := fmt.Sprintf(
			"terragrunt run --all --non-interactive --working-dir %s "+
				"--report-file %s --report-format json",
			rootPath,
			reportFile,
		)

		var cmdSB strings.Builder
		for _, flag := range tc.featureFlags {
			cmdSB.WriteString(" --feature " + flag)
		}

		cmd += cmdSB.String()

		cmd += " -- " + tc.command

		return cmd
	}

	cmd := fmt.Sprintf(
		"terragrunt %s --non-interactive --working-dir %s",
		tc.command,
		rootPath,
	)

	var cmdSB strings.Builder
	for _, flag := range tc.featureFlags {
		cmdSB.WriteString(" --feature " + flag)
	}

	cmd += cmdSB.String()

	return cmd
}

// TestTFExcludeBlockFeatureFlagDefaultInDependency tests that when a dependency unit
// defines feature flags with defaults and uses them in an exclude block, the dependent
// unit can still parse the dependency's config without errors.
// This reproduces https://github.com/gruntwork-io/terragrunt/issues/4395
func TestTFExcludeBlockFeatureFlagDefaultInDependency(t *testing.T) {
	t.Parallel()

	testFixturePath := "fixtures/exclude/dependency-feature-flags"
	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath, "dependent-unit")

	cmd := "terragrunt plan --non-interactive --working-dir " + rootPath

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	// The bug causes: "Error: Attempt to get attribute from null value"
	// when parsing the dependency's exclude block that uses feature flags
	assert.NotContains(t, stderr, "Attempt to get attribute from null value",
		"Feature flag defaults should be available when parsing dependency configs")
	require.NoError(
		t,
		err,
		"terragrunt plan should succeed when dependency has feature flags in exclude block",
	)
}

// TestTFExcludeBlockFeatureFlagDefaultRunAll tests the run-all scenario where
// all units are parsed and one has feature flags with defaults in exclude blocks.
func TestTFExcludeBlockFeatureFlagDefaultRunAll(t *testing.T) {
	t.Parallel()

	testFixturePath := "fixtures/exclude/dependency-feature-flags"
	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)

	reportFile := filepath.Join(t.TempDir(), "report.json")

	cmd := fmt.Sprintf(
		"terragrunt run --all --non-interactive --working-dir %s --report-file %s --report-format json -- plan",
		rootPath,
		reportFile,
	)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	assert.NotContains(t, stderr, "Attempt to get attribute from null value",
		"Feature flag defaults should be available when parsing configs in run-all mode")
	require.NoError(
		t,
		err,
		"terragrunt run-all plan should succeed with feature flags in exclude blocks",
	)
}

// TestTFExcludeBlockNullOrUnknownStringDiscovery tests that discovery lists
// units whose exclude block reads a null or unknown string, where it used to
// panic.
func TestTFExcludeBlockNullOrUnknownStringDiscovery(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"find --dag", "list --dag"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testExcludeNullOrUnknownString)
			rootPath := filepath.Join(tmpEnvPath, testExcludeNullOrUnknownString)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt "+command+" --no-color --working-dir "+rootPath,
			)
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"dep", "null-string", "unknown-string"}, strings.Fields(stdout))
			assert.Contains(
				t,
				stderr,
				"Ignoring the exclude block in "+filepath.Join(rootPath, "unknown-string", "terragrunt.hcl"),
			)
		})
	}
}

// TestTFExcludeBlockNullOrUnknownStringFindJSON tests that find reports a null
// string in an exclude block the same way as a null bool, and omits an exclude
// block that reads a dependency output.
func TestTFExcludeBlockNullOrUnknownStringFindJSON(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeNullOrUnknownString)
	rootPath := filepath.Join(tmpEnvPath, testExcludeNullOrUnknownString)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt find --json --exclude --no-color --working-dir "+rootPath,
	)
	require.NoError(t, err)

	var components []struct {
		Exclude *config.ExcludeConfig `json:"exclude"`
		Path    string                `json:"path"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	excludes := map[string]*config.ExcludeConfig{}
	for _, component := range components {
		excludes[component.Path] = component.Exclude
	}

	assert.Equal(t, map[string]*config.ExcludeConfig{
		"dep":            nil,
		"null-string":    {Actions: []string{"all"}},
		"unknown-string": nil,
	}, excludes)
}

// TestTFExcludeBlockNullOrUnknownStringRunAll tests that run --all reports a
// null string in an exclude block as a unit error and runs the unit whose
// exclude block reads a dependency output, where both used to panic.
func TestTFExcludeBlockNullOrUnknownStringRunAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testExcludeNullOrUnknownString)
	tmpEnvPath := helpers.CopyEnvironment(t, testExcludeNullOrUnknownString)
	rootPath := filepath.Join(tmpEnvPath, testExcludeNullOrUnknownString)

	reportFile := filepath.Join(t.TempDir(), "report.json")

	cmd := fmt.Sprintf(
		"terragrunt run --all --non-interactive --working-dir %s --report-file %s --report-format json -- plan",
		rootPath,
		reportFile,
	)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.Error(t, err)
	assert.Contains(t, stderr, "null value is not allowed")
	assert.Contains(
		t,
		stderr,
		"Ignoring the exclude block in "+filepath.Join(rootPath, "unknown-string", "terragrunt.hcl"),
	)

	runs, err := report.ParseJSONRunsFromFile(vfs.NewOSFS(), reportFile)
	require.NoError(t, err)

	want := map[string]string{
		"dep":            "succeeded",
		"null-string":    "failed",
		"unknown-string": "succeeded",
	}
	require.Len(t, runs, len(want), "Found: %v", runs.Names())

	for unit, result := range want {
		run := runs.FindByName(unit)
		require.NotNil(t, run, "unit %s not found in report. Found: %v", unit, runs.Names())
		assert.Equal(t, result, run.Result, "unit %s", unit)
	}
}
