package test_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testExcludeComprehensive = "fixtures/exclude/comprehensive"
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

func TestExcludeBlockBehavior(t *testing.T) {
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

			cleanupTerraformFolder(t, testExcludeComprehensive)
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

			runs, err := report.ParseJSONRunsFromFile(reportFile)
			require.NoError(t, err, "Failed to parse report file")

			for unitName, expected := range tc.expectedUnits {
				run := runs.FindByName(unitName)
				require.NotNil(t, run, "unit %s not found in report. Found: %v", unitName, runs.Names())
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
					t.Fatalf("Unexpected result %q for unit %s", expected.result, unitName)
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
