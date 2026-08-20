//go:build tf

package test_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFFilterFlagWithRunAllGitFilter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		filterQuery        string
		description        string
		expectedUnits      []string
		ignoredUnits       []string
		expectedExcluded   []string
		filterAllowDestroy bool
		expectError        bool
	}{
		{
			name:               "git filter discovers modified, created, and removed units and excludes untouched",
			filterQuery:        "[HEAD~1...HEAD]",
			filterAllowDestroy: false,
			expectedUnits:      []string{"unit-to-be-created", "unit-to-be-modified"},
			ignoredUnits:       []string{"unit-to-be-untouched"},
			expectedExcluded:   []string{"unit-to-be-removed"},
			expectError:        false,
			description:        "Git filter should discover units that were created, modified, or removed between commits, and exclude untouched units. Removed unit should be excluded without --filter-allow-destroy",
		},
		{
			name:               "git filter with --filter-allow-destroy includes removed unit",
			filterQuery:        "[HEAD~1...HEAD]",
			filterAllowDestroy: true,
			expectedUnits: []string{
				"unit-to-be-created",
				"unit-to-be-modified",
				"unit-to-be-removed",
			},
			ignoredUnits:     []string{"unit-to-be-untouched"},
			expectedExcluded: []string{},
			expectError:      false,
			description:      "Git filter with --filter-allow-destroy should include removed unit for destroy operations",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			runner := helpers.InitTestGitRunner(t, tmpDir)

			// Create three units initially using helper
			unitToBeModifiedDir := filepath.Join(tmpDir, "unit-to-be-modified")
			unitToBeRemovedDir := filepath.Join(tmpDir, "unit-to-be-removed")
			unitToBeUntouchedDir := filepath.Join(tmpDir, "unit-to-be-untouched")

			unitToBeModifiedHCLPath := createTestUnit(
				t,
				unitToBeModifiedDir,
				`# Unit to be modified`,
			)
			_ = createTestUnit(t, unitToBeRemovedDir, `# Unit to be removed`)
			_ = createTestUnit(t, unitToBeUntouchedDir, `# Unit to be untouched`)

			// Initial commit
			require.NoError(t, runner.Add(t.Context(), "."))
			require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

			// Modify the unit to be modified
			err := os.WriteFile(unitToBeModifiedHCLPath, []byte(`# Unit modified`), 0644)
			require.NoError(t, err)

			// Remove the unit to be removed (delete the directory)
			err = os.RemoveAll(unitToBeRemovedDir)
			require.NoError(t, err)

			// Add a unit to be created
			unitToBeCreatedDir := filepath.Join(tmpDir, "unit-to-be-created")
			_ = createTestUnit(t, unitToBeCreatedDir, `# Unit created`)

			// Do nothing to the unit to be untouched

			// Commit the modification and removal in a single commit
			require.NoError(t, runner.Add(t.Context(), "."))
			require.NoError(t, runner.Commit(t.Context(), "Create, modify, and remove units"))

			// Run terragrunt run --all --filter with git filter
			// Note: We use 'plan' command which should work even without terraform init
			// Note: --experiment-mode enables the filter-flag experiment required for --filter
			cmd := "terragrunt run --all --no-color --experiment-mode --working-dir " + tmpDir + " --filter '" + tc.filterQuery + "' --report-file " + helpers.ReportFile

			if tc.filterAllowDestroy {
				cmd += " --filter-allow-destroy"
			}

			cmd += " -- plan"

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				// For run commands, we expect some output even if terraform isn't fully initialized
				// The key is that the command should execute and process the filtered units
				if err != nil {
					// If there's an error, it might be because terraform isn't initialized
					// but we should still see that the filter worked (units were discovered)
					// Let's check if the error is about terraform init or similar
					if !strings.Contains(stderr, "terraform") && !strings.Contains(stderr, "tofu") {
						// Unexpected error
						require.NoError(
							t,
							err,
							"Unexpected error for filter query: %s\nstdout: %s\nstderr: %s",
							tc.filterQuery,
							stdout,
							stderr,
						)
					}
				}

				// Verify the report file exists
				reportFilePath := filepath.Join(tmpDir, helpers.ReportFile)
				assert.FileExists(t, reportFilePath, "Report file should exist")

				// Read and parse the report file
				runs, err := report.ParseJSONRunsFromFile(reportFilePath)
				require.NoError(t, err, "Should be able to parse report JSON")

				// Create a map of unit names to records for easier lookup
				// The report contains full paths, so we extract the unit name from the path
				recordsByUnit := make(map[string]*report.JSONRun)

				for i := range runs {
					run := &runs[i]
					fullPath := run.Name
					// Extract unit name from path (e.g., "unit-to-be-created" from "/tmp/.../unit-to-be-created")
					baseName := filepath.Base(fullPath)
					recordsByUnit[baseName] = run
					// Also store by full path for fallback
					recordsByUnit[fullPath] = run
					// Store by any part of the path that matches our unit pattern
					parts := strings.SplitSeq(fullPath, string(filepath.Separator))
					for part := range parts {
						if strings.HasPrefix(part, "unit-to-be-") {
							recordsByUnit[part] = run
						}
					}
				}

				// Verify expected units are in the report and not excluded
				for _, expectedUnit := range tc.expectedUnits {
					run, found := recordsByUnit[expectedUnit]
					if !found {
						// Try to find by partial match
						for name, rec := range recordsByUnit {
							if strings.Contains(name, expectedUnit) {
								run = rec
								found = true

								break
							}
						}
					}

					require.True(
						t,
						found,
						"Expected unit '%s' should be in report. Found units: %v",
						expectedUnit,
						getJSONRunNames(recordsByUnit),
					)
					assert.NotEqual(
						t,
						"excluded",
						run.Result,
						"Expected unit '%s' should not be excluded",
						expectedUnit,
					)
				}

				// Verify excluded units are NOT in the report
				for _, excludedUnit := range tc.ignoredUnits {
					found := false

					for name := range recordsByUnit {
						if strings.Contains(name, excludedUnit) {
							found = true
							break
						}
					}

					assert.False(
						t,
						found,
						"Excluded unit '%s' should NOT be in report",
						excludedUnit,
					)
				}

				// Verify expected excluded units are in the report but marked as excluded
				for _, excludedUnit := range tc.expectedExcluded {
					run, found := recordsByUnit[excludedUnit]
					if !found {
						// Try to find by partial match
						for name, rec := range recordsByUnit {
							if strings.Contains(name, excludedUnit) {
								run = rec
								found = true

								break
							}
						}
					}

					require.True(
						t,
						found,
						"Expected excluded unit '%s' should be in report",
						excludedUnit,
					)
					assert.Equal(
						t,
						"excluded",
						run.Result,
						"Unit '%s' should be marked as excluded",
						excludedUnit,
					)
				}
			}
		})
	}
}

func TestTFFilterFlagWithRunAllGitFilterRemovedUnitDestroyFlag(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)

	unitToBeRemovedDir := filepath.Join(tmpDir, "unit-to-be-removed")
	err := os.MkdirAll(unitToBeRemovedDir, 0755)
	require.NoError(t, err)

	terragruntHCL := `# Unit to be removed
terraform {
  source = "."
}
`
	err = os.WriteFile(
		filepath.Join(unitToBeRemovedDir, "terragrunt.hcl"),
		[]byte(terragruntHCL),
		0644,
	)
	require.NoError(t, err)

	mainTF := `resource "null_resource" "test" {
  triggers = {
    test = "value"
  }
}
`
	err = os.WriteFile(filepath.Join(unitToBeRemovedDir, "main.tf"), []byte(mainTF), 0644)
	require.NoError(t, err)

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit with unit"))

	// Apply the unit so that it shows up in state first.
	cmd := "terragrunt run --non-interactive --all --no-color --report-file report.json --working-dir " + tmpDir + " -- apply"

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	runs := helpers.ReadReport(t, tmpDir, "report.json")
	assert.NotNil(t, runs.FindByName("unit-to-be-removed"),
		"unit-to-be-removed should be discovered and run")

	assert.Contains(
		t,
		stdout,
		"Apply complete! Resources: 1 added",
		"unit-to-be-removed should be applied",
	)

	err = os.RemoveAll(unitToBeRemovedDir)
	require.NoError(t, err)

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Remove unit"))

	cmd = "terragrunt run --non-interactive --all --no-color --working-dir " + tmpDir +
		" --filter '[HEAD~1]' --filter-allow-destroy -- plan"

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	combinedOutput := stdout + stderr

	assert.Contains(
		t,
		combinedOutput,
		"unit-to-be-removed",
		"Removed unit should be discovered and processed",
	)

	// Check for destroy-related output. The message "No changes. No objects need to be destroyed"
	// is what Terraform outputs when plan -destroy is run but there's no state to destroy.
	// This is expected when using worktrees with local state (state is in original dir, not worktree).
	// The important thing is that the -destroy flag was passed, which we verify by checking for
	// this specific message that only appears with -destroy flag.
	hasDestroyFlag := strings.Contains(combinedOutput, "to destroy") ||
		strings.Contains(combinedOutput, "No objects need to be destroyed") ||
		strings.Contains(combinedOutput, "will be destroyed")

	assert.True(
		t,
		hasDestroyFlag,
		"Removed unit should be planned with -destroy flag. Output should contain 'to destroy', 'No objects need to be destroyed', or 'will be destroyed'. "+
			"Current output:\nstdout: %s\nstderr: %s",
		stdout,
		stderr,
	)
}

func TestTFFilterFlagWithExplicitStacksGitFilter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		filterQuery        string
		description        string
		expectedUnits      []string
		ignoredUnits       []string
		expectedExcluded   []string
		filterAllowDestroy bool
		expectError        bool
	}{
		{
			name:               "git filter discovers units from modified, created, and removed stacks and excludes untouched",
			filterQuery:        "[HEAD~1...HEAD]",
			filterAllowDestroy: false,
			expectedUnits: []string{
				"unit-to-be-added",
				"unit-to-be-modified",
				"unit-to-be-created-1",
				"unit-to-be-created-2",
			},
			ignoredUnits: []string{
				"unit-to-be-untouched",
			},
			expectedExcluded: []string{
				"unit-to-be-removed-from-stack",
			},
			expectError: false,
			description: "Git filter should discover units from stacks that were created, modified, or removed between commits, and exclude untouched stacks. Units from removed stack should be excluded without --filter-allow-destroy",
		},
		{
			name:               "git filter with --filter-allow-destroy includes units from removed stack",
			filterQuery:        "[HEAD~1...HEAD]",
			filterAllowDestroy: true,
			expectedUnits: []string{
				"unit-to-be-added",
				"unit-to-be-modified",
				"unit-to-be-created-1",
				"unit-to-be-created-2",
				"unit-to-be-removed-from-stack",
			},
			ignoredUnits: []string{
				"unit-to-be-untouched",
			},
			expectedExcluded: []string{},
			expectError:      false,
			description:      "Git filter with --filter-allow-destroy should include units from removed stack for destroy operations",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			runner := helpers.InitTestGitRunner(t, tmpDir)

			// Create a catalog of units that will be referenced by stacks
			legacyUnitDir := filepath.Join(tmpDir, "catalog", "units", "legacy")
			err := os.MkdirAll(legacyUnitDir, 0755)
			require.NoError(t, err)
			_ = createTestUnit(t, legacyUnitDir, `# Legacy unit`)

			modernUnitDir := filepath.Join(tmpDir, "catalog", "units", "modern")
			err = os.MkdirAll(modernUnitDir, 0755)
			require.NoError(t, err)
			_ = createTestUnit(t, modernUnitDir, `# Modern unit`)

			// Create initial stacks
			stackToBeModifiedDir := filepath.Join(tmpDir, "live", "stack-to-be-modified")
			err = os.MkdirAll(stackToBeModifiedDir, 0755)
			require.NoError(t, err)

			stackToBeRemovedDir := filepath.Join(tmpDir, "live", "stack-to-be-removed")
			err = os.MkdirAll(stackToBeRemovedDir, 0755)
			require.NoError(t, err)

			stackToBeUntouchedDir := filepath.Join(tmpDir, "live", "stack-to-be-untouched")
			err = os.MkdirAll(stackToBeUntouchedDir, 0755)
			require.NoError(t, err)

			// Initial stack file contents
			initialStackContent := `unit "unit-to-be-modified" {
	source = "${get_repo_root()}/catalog/units/legacy"
	path   = "unit-to-be-modified"
}

unit "unit-to-be-removed-from-stack" {
	source = "${get_repo_root()}/catalog/units/legacy"
	path   = "unit-to-be-removed-from-stack"
}
`

			untouchedStackContent := `unit "unit-to-be-untouched" {
	source = "${get_repo_root()}/catalog/units/legacy"
	path   = "unit-to-be-untouched"
}
`

			// Write initial stack files
			err = os.WriteFile(
				filepath.Join(stackToBeModifiedDir, "terragrunt.stack.hcl"),
				[]byte(initialStackContent),
				0644,
			)
			require.NoError(t, err)

			err = os.WriteFile(
				filepath.Join(stackToBeRemovedDir, "terragrunt.stack.hcl"),
				[]byte(initialStackContent),
				0644,
			)
			require.NoError(t, err)

			err = os.WriteFile(
				filepath.Join(stackToBeUntouchedDir, "terragrunt.stack.hcl"),
				[]byte(untouchedStackContent),
				0644,
			)
			require.NoError(t, err)

			// Initial commit
			require.NoError(t, runner.Add(t.Context(), "."))
			require.NoError(t, runner.Commit(t.Context(), "Initial commit with stacks"))

			// Modify the stack-to-be-modified: add a unit, modify a unit, remove a unit
			modifiedStackContent := `unit "unit-to-be-added" {
	source = "${get_repo_root()}/catalog/units/modern"
	path   = "unit-to-be-added"
}

unit "unit-to-be-modified" {
	source = "${get_repo_root()}/catalog/units/modern"
	path   = "unit-to-be-modified"
}
`
			err = os.WriteFile(
				filepath.Join(stackToBeModifiedDir, "terragrunt.stack.hcl"),
				[]byte(modifiedStackContent),
				0644,
			)
			require.NoError(t, err)

			// Remove the stack-to-be-removed
			err = os.RemoveAll(stackToBeRemovedDir)
			require.NoError(t, err)

			// Add a new stack
			stackToBeCreatedDir := filepath.Join(tmpDir, "live", "stack-to-be-created")
			err = os.MkdirAll(stackToBeCreatedDir, 0755)
			require.NoError(t, err)

			newStackContent := `unit "unit-to-be-created-1" {
	source = "${get_repo_root()}/catalog/units/modern"
	path   = "unit-to-be-created-1"
}

unit "unit-to-be-created-2" {
	source = "${get_repo_root()}/catalog/units/modern"
	path   = "unit-to-be-created-2"
}
`
			err = os.WriteFile(
				filepath.Join(stackToBeCreatedDir, "terragrunt.stack.hcl"),
				[]byte(newStackContent),
				0644,
			)
			require.NoError(t, err)

			// Leave stack-to-be-untouched unchanged

			// Commit the changes
			require.NoError(t, runner.Add(t.Context(), "."))
			require.NoError(t, runner.Commit(t.Context(), "Modify, create, and remove stacks"))

			// Run terragrunt run --all --filter with git filter
			cmd := "terragrunt run --all --no-color --experiment-mode --working-dir " + tmpDir + " --filter '" + tc.filterQuery + "' --report-file " + helpers.ReportFile

			if tc.filterAllowDestroy {
				cmd += " --filter-allow-destroy"
			}

			cmd += " -- plan"

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				require.Error(t, err, "Expected error for filter query: %s", tc.filterQuery)
				assert.NotEmpty(t, stderr, "Expected error message in stderr")
			} else {
				// For run commands, we expect some output even if terraform isn't fully initialized
				// The key is that the command should execute and process the filtered units
				if err != nil {
					// If there's an error, it might be because terraform isn't initialized
					// but we should still see that the filter worked (units were discovered)
					// Let's check if the error is about terraform init or similar
					if !strings.Contains(stderr, "terraform") && !strings.Contains(stderr, "tofu") {
						// Unexpected error
						require.NoError(
							t,
							err,
							"Unexpected error for filter query: %s\nstdout: %s\nstderr: %s",
							tc.filterQuery,
							stdout,
							stderr,
						)
					}
				}

				// Verify the report file exists
				reportFilePath := filepath.Join(tmpDir, helpers.ReportFile)
				assert.FileExists(t, reportFilePath, "Report file should exist")

				// Read and parse the report file
				runs, err := report.ParseJSONRunsFromFile(reportFilePath)
				require.NoError(t, err, "Should be able to parse report JSON")

				// Create a map of unit names to records for easier lookup
				// The report contains full paths, so we extract the unit name from the path
				recordsByUnit := make(map[string]*report.JSONRun)

				for i := range runs {
					run := &runs[i]
					fullPath := run.Name
					// Extract unit name from path
					// Paths might be like: /tmp/.../live/stack-to-be-modified/.terragrunt-stack/unit-to-be-added
					baseName := filepath.Base(fullPath)
					recordsByUnit[baseName] = run
					// Also store by full path for fallback
					recordsByUnit[fullPath] = run
					// Store by any part of the path that matches our unit pattern
					parts := strings.SplitSeq(fullPath, string(filepath.Separator))
					for part := range parts {
						if strings.HasPrefix(part, "unit-to-be-") {
							recordsByUnit[part] = run
						}
					}
				}

				// Verify expected units are in the report and not excluded
				for _, expectedUnit := range tc.expectedUnits {
					run, found := recordsByUnit[expectedUnit]
					if !found {
						// Try to find by partial match
						for name, rec := range recordsByUnit {
							if strings.Contains(name, expectedUnit) {
								run = rec
								found = true

								break
							}
						}
					}

					require.True(
						t,
						found,
						"Expected unit '%s' should be in report. Found units: %v",
						expectedUnit,
						getJSONRunNames(recordsByUnit),
					)
					assert.NotEqual(
						t,
						"excluded",
						run.Result,
						"Expected unit '%s' should not be excluded",
						expectedUnit,
					)
				}

				// Verify excluded units are NOT in the report
				for _, excludedUnit := range tc.ignoredUnits {
					found := false

					for name := range recordsByUnit {
						if strings.Contains(name, excludedUnit) {
							found = true
							break
						}
					}

					assert.False(
						t,
						found,
						"Excluded unit '%s' should NOT be in report",
						excludedUnit,
					)
				}

				// Verify expected excluded units are in the report but marked as excluded
				for _, excludedUnit := range tc.expectedExcluded {
					run, found := recordsByUnit[excludedUnit]
					if !found {
						// Try to find by partial match
						for name, rec := range recordsByUnit {
							if strings.Contains(name, excludedUnit) {
								run = rec
								found = true

								break
							}
						}
					}

					require.True(
						t,
						found,
						"Expected excluded unit '%s' should be in report",
						excludedUnit,
					)
					assert.Equal(
						t,
						"excluded",
						run.Result,
						"Unit '%s' should be marked as excluded",
						excludedUnit,
					)
				}
			}
		})
	}
}

func TestTFFilterFlagMinimizesParsing(t *testing.T) {
	t.Parallel()

	t.Run("single unit filter", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsing)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsing)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsing)

		// Run with filter targeting only target-unit
		// This will parse target-unit and its dependency (dependency-unit) for outputs,
		// but only target-unit will be run and appear in the report
		// The excluded units with land-mine configs should NOT be parsed
		cmd := "terragrunt run --all plan --no-color --experiment-mode --working-dir " + rootPath + " --filter './target-unit' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// Command should succeed
		require.NoError(t, err)

		// Verify no errors from land-mine units in stderr
		assert.NotContains(t, stderr, "excluded-unit-1", "excluded-unit-1 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-2", "excluded-unit-2 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-3", "excluded-unit-3 should not be parsed")

		// Verify that dependency-unit is still being parsed
		assert.Contains(t, stderr, "dependency-unit", "dependency-unit should be parsed")

		// Verify the report file exists and parse it
		reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
		require.FileExists(t, reportFilePath, "Report file should exist")
		runs, err := report.ParseJSONRunsFromFile(reportFilePath)
		require.NoError(t, err, "Should be able to parse report JSON")

		names := runs.Names()

		require.True(t, slices.ContainsFunc(names, func(name string) bool {
			return strings.Contains(name, "target-unit")
		}), "target-unit should be in report. Found units: %v", names)

		for _, excludedUnit := range []string{"excluded-unit-1", "excluded-unit-2", "excluded-unit-3"} {
			assert.False(t, slices.ContainsFunc(names, func(name string) bool {
				return strings.Contains(name, excludedUnit)
			}), "Excluded unit '%s' should NOT be in report", excludedUnit)
		}
	})

	t.Run("multiple units filter", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsing)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsing)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsing)

		// Run with filter targeting both target-unit and dependency-unit (OR semantics)
		cmd := "terragrunt run --all plan --no-color --experiment-mode --working-dir " + rootPath + " --filter './target-unit' --filter './dependency-unit' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// Command should succeed - if land-mines were parsed, we'd get errors
		require.NoError(t, err)

		// Verify no errors from land-mine units in stderr
		assert.NotContains(t, stderr, "excluded-unit-1", "excluded-unit-1 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-2", "excluded-unit-2 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-3", "excluded-unit-3 should not be parsed")

		// Verify the report file exists and parse it
		reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
		require.FileExists(t, reportFilePath)

		runs, err := report.ParseJSONRunsFromFile(reportFilePath)
		require.NoError(t, err, "Should be able to parse report JSON")

		names := runs.Names()

		for _, expected := range []string{"target-unit", "dependency-unit"} {
			require.True(t, slices.ContainsFunc(names, func(name string) bool {
				return strings.Contains(name, expected)
			}), "%s should be in report. Found units: %v", expected, names)
		}

		for _, excludedUnit := range []string{"excluded-unit-1", "excluded-unit-2", "excluded-unit-3"} {
			assert.False(t, slices.ContainsFunc(names, func(name string) bool {
				return strings.Contains(name, excludedUnit)
			}), "Excluded unit '%s' should NOT be in report", excludedUnit)
		}
	})

	t.Run("positive and negative filter", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsing)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsing)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsing)

		// excluded-unit-{2,3} match neither expression; classifier must early-exclude them or their land-mine run_cmd fires.
		cmd := "terragrunt run --all plan --no-color --experiment-mode --working-dir " + rootPath +
			" --filter './target-unit' --filter '!./excluded-unit-1' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		require.NoError(t, err)

		assert.NotContains(t, stderr, "excluded-unit-1", "excluded-unit-1 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-2", "excluded-unit-2 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-3", "excluded-unit-3 should not be parsed")

		reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
		require.FileExists(t, reportFilePath)

		runs, err := report.ParseJSONRunsFromFile(reportFilePath)
		require.NoError(t, err, "Should be able to parse report JSON")

		names := runs.Names()

		require.True(t, slices.ContainsFunc(names, func(name string) bool {
			return strings.Contains(name, "target-unit")
		}), "target-unit should be in report. Found units: %v", names)

		for _, excludedUnit := range []string{"excluded-unit-1", "excluded-unit-2", "excluded-unit-3"} {
			assert.False(t, slices.ContainsFunc(names, func(name string) bool {
				return strings.Contains(name, excludedUnit)
			}), "Excluded unit '%s' should NOT be in report", excludedUnit)
		}
	})

	t.Run("graph filter with negation", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsing)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsing)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsing)

		// Pins step-1 ordering: a simple negation must short-circuit before graph dep traversal forces parsing.
		cmd := "terragrunt run --all plan --no-color --experiment-mode --working-dir " + rootPath +
			" --filter './target-unit...' --filter '!./excluded-unit-1' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		require.NoError(t, err)

		assert.NotContains(t, stderr, "excluded-unit-1", "excluded-unit-1 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-2", "excluded-unit-2 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-3", "excluded-unit-3 should not be parsed")

		reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
		require.FileExists(t, reportFilePath)

		runs, err := report.ParseJSONRunsFromFile(reportFilePath)
		require.NoError(t, err, "Should be able to parse report JSON")

		names := runs.Names()

		for _, expected := range []string{"target-unit", "dependency-unit"} {
			require.True(t, slices.ContainsFunc(names, func(name string) bool {
				return strings.Contains(name, expected)
			}), "%s should be in report. Found units: %v", expected, names)
		}

		for _, excludedUnit := range []string{"excluded-unit-1", "excluded-unit-2", "excluded-unit-3"} {
			assert.False(t, slices.ContainsFunc(names, func(name string) bool {
				return strings.Contains(name, excludedUnit)
			}), "Excluded unit '%s' should NOT be in report", excludedUnit)
		}
	})

	t.Run("negated graph target still parses for traversal", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsing)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsing)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsing)

		// Graph traversal requires parsing the target to discover deps; negation only scopes the final result.
		// Verifies minimization still applies to the unrelated land-mine units.
		cmd := "terragrunt run --all plan --no-color --experiment-mode --working-dir " + rootPath +
			" --filter './excluded-unit-1...' --filter '!./excluded-unit-1' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		require.Error(t, err)
		assert.Contains(
			t,
			stderr,
			"excluded-unit-1",
			"excluded-unit-1 must be parsed because it is the graph target",
		)
		assert.NotContains(t, stderr, "excluded-unit-2", "excluded-unit-2 should not be parsed")
		assert.NotContains(t, stderr, "excluded-unit-3", "excluded-unit-3 should not be parsed")
	})

	t.Run("destroy without graph filter", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsingDestroy)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsingDestroy)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsingDestroy)

		// Run destroy with filter targeting only unit-a
		// This should only parse unit-a, NOT all units in the repository
		// The land-mine units should NOT be parsed
		cmd := "terragrunt run --all destroy --non-interactive --no-color --experiment-mode --working-dir " + rootPath + " --filter './unit-a' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// Command should succeed - if land-mines were parsed, we'd get errors
		require.NoError(t, err)

		// Verify no errors from land-mine units in stderr
		assert.NotContains(
			t,
			stderr,
			"landmine-unit-1",
			"landmine-unit-1 should not be parsed during destroy",
		)
		assert.NotContains(
			t,
			stderr,
			"landmine-unit-2",
			"landmine-unit-2 should not be parsed during destroy",
		)

		// Verify the report file exists and parse it
		reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
		require.FileExists(t, reportFilePath)

		runs, err := report.ParseJSONRunsFromFile(reportFilePath)
		require.NoError(t, err, "Should be able to parse report JSON")

		names := runs.Names()

		require.True(t, slices.ContainsFunc(names, func(name string) bool {
			return strings.Contains(name, "unit-a")
		}), "unit-a should be in report. Found units: %v", names)

		for _, excludedUnit := range []string{"landmine-unit-1", "landmine-unit-2"} {
			assert.False(t, slices.ContainsFunc(names, func(name string) bool {
				return strings.Contains(name, excludedUnit)
			}), "Excluded unit '%s' should NOT be in report", excludedUnit)
		}
	})

	t.Run("destroy with graph filter", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureMinimizeParsingDestroy)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMinimizeParsingDestroy)
		rootPath := filepath.Join(tmpEnvPath, testFixtureMinimizeParsingDestroy)

		// Run destroy with graph filter targeting unit-a
		// Graph filters explicitly request dependency discovery, so this is expected behavior
		// The land-mine units should still NOT be parsed (they're not dependencies)
		cmd := "terragrunt run --all destroy --non-interactive --no-color --experiment-mode --working-dir " + rootPath + " --filter '{./unit-a}...' --report-file " + helpers.ReportFile
		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

		// Command should succeed - if land-mines were parsed, we'd get errors
		// Note: destroy might fail for other reasons (e.g., no state), but it shouldn't fail due to parsing land-mines
		require.NoError(t, err)

		// Verify no errors from land-mine units in stderr
		assert.NotContains(
			t,
			stderr,
			"landmine-unit-1",
			"landmine-unit-1 should not be parsed during destroy with graph filter",
		)
		assert.NotContains(
			t,
			stderr,
			"landmine-unit-2",
			"landmine-unit-2 should not be parsed during destroy with graph filter",
		)

		// Verify the report file exists and parse it
		reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
		require.FileExists(t, reportFilePath)

		runs, err := report.ParseJSONRunsFromFile(reportFilePath)
		require.NoError(t, err, "Should be able to parse report JSON")

		names := runs.Names()

		require.True(t, slices.ContainsFunc(names, func(name string) bool {
			return strings.Contains(name, "unit-a")
		}), "unit-a should be in report. Found units: %v", names)

		for _, excludedUnit := range []string{"landmine-unit-1", "landmine-unit-2"} {
			assert.False(t, slices.ContainsFunc(names, func(name string) bool {
				return strings.Contains(name, excludedUnit)
			}), "Excluded unit '%s' should NOT be in report", excludedUnit)
		}
	})
}

func TestTFFilterFlagAutoEnablesAll(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		cmd           string
		expectedUnits []string
	}{
		{
			name: "filter flag without --all processes multiple units",
			cmd:  "terragrunt run --no-color --filter './**' --report-file " + helpers.ReportFile + " plan",
			expectedUnits: []string{
				"a-dependent",
				"b-dependency",
				"c-mixed-deps",
				"d-dependencies-only",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFilterDAG)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFilterDAG)
			rootPath := filepath.Join(tmpEnvPath, testFixtureFilterDAG)

			cmd := tc.cmd + " --working-dir " + rootPath
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			// Verify the report file exists
			reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
			assert.FileExists(t, reportFilePath)

			r, err := report.ParseJSONRunsFromFile(reportFilePath)
			require.NoError(t, err)

			runs := r.Names()

			// Verify expected units are in the report
			assert.ElementsMatch(t, tc.expectedUnits, runs)
		})
	}
}

// TestTFOutDirWithGitFilter verifies that --out-dir works correctly with git-based filters.
// This is a regression test for https://github.com/gruntwork-io/terragrunt/issues/5287
// The bug was that plan files were written to the temporary git worktree directory
// instead of the specified --out-dir path.
func TestTFOutDirWithGitFilter(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	outDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)

	// Create initial unit
	unitDir := filepath.Join(tmpDir, "unit-initial")
	err := os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	// Create terragrunt.hcl
	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`# Initial unit`), 0644)
	require.NoError(t, err)

	// Create main.tf with a simple null resource
	err = os.WriteFile(filepath.Join(unitDir, "main.tf"), []byte(`
resource "null_resource" "test" {}
`), 0644)
	require.NoError(t, err)

	// Initial commit
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	// Create a new unit (this will be detected by the git filter)
	newUnitDir := filepath.Join(tmpDir, "unit-new")
	err = os.MkdirAll(newUnitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(newUnitDir, "terragrunt.hcl"), []byte(`# New unit`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(newUnitDir, "main.tf"), []byte(`
resource "null_resource" "test" {}
`), 0644)
	require.NoError(t, err)

	// Commit the new unit
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Add new unit"))

	// Run terragrunt with --out-dir and git filter
	// The bug was that plan files went to /tmp/terragrunt-worktree-... instead of outDir
	cmd := "terragrunt run --all --no-color --experiment-mode --non-interactive --working-dir " + tmpDir +
		" --out-dir " + outDir + " --filter '[HEAD~1...HEAD]' -- plan"

	helpers.RunTerragrunt(t, cmd)

	// Verify plan files are in outDir, NOT in a worktree path
	// The key assertion: plan files should be in outDir/unit-new/
	// NOT in /tmp/terragrunt-worktree-*/unit-new/
	files, err := filepath.Glob(filepath.Join(outDir, "**", "*.tfplan"))
	if err != nil {
		// Glob with ** doesn't work on all systems, try a walk
		files = []string{}
		_ = filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
			require.NoError(t, err)

			if strings.HasSuffix(path, ".tfplan") {
				files = append(files, path)
			}

			return nil
		})
	}

	// Should have at least 1 plan file in outDir
	assert.NotEmpty(t, files, "Expected plan files in outDir %s", outDir)

	// None of the files should be in a worktree path
	for _, file := range files {
		assert.NotContains(t, file, "terragrunt-worktree",
			"Plan file %s should not be in a worktree directory", file)
		assert.True(t, strings.HasPrefix(file, outDir),
			"Plan file %s should be in outDir %s", file, outDir)
	}
}

// TestTFOutDirPlanKeyMatchesFilesystemDiscovery verifies that a unit is mirrored under
// --out-dir at the same path whether a Git filter or a plain filesystem walk selected it,
// so a plan taken with a filter can be applied by a run without one.
func TestTFOutDirPlanKeyMatchesFilesystemDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	outDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)

	// A working directory below the root of the repository is where the two ways of
	// selecting a unit used to disagree.
	workingDir := filepath.Join(tmpDir, "live")
	unitDir := filepath.Join(workingDir, "unit")
	err := os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`# Unit`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "main.tf"), []byte(`
resource "null_resource" "test" {}
`), 0644)
	require.NoError(t, err)

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	err = os.WriteFile(filepath.Join(unitDir, "main.tf"), []byte(`
resource "null_resource" "test" {}

resource "null_resource" "second" {}
`), 0644)
	require.NoError(t, err)

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Modify unit"))

	cmd := "terragrunt run --all --no-color --experiment-mode --non-interactive --working-dir " + workingDir +
		" --out-dir " + outDir + " --filter '[HEAD~1...HEAD]' -- plan"

	helpers.RunTerragrunt(t, cmd)

	require.FileExists(t, filepath.Join(outDir, "unit", "tfplan.tfplan"))
	require.NoFileExists(t, filepath.Join(outDir, "live", "unit", "tfplan.tfplan"))

	cmd = "terragrunt run --all --no-color --non-interactive --working-dir " + workingDir +
		" --out-dir " + outDir + " -- apply"

	helpers.RunTerragrunt(t, cmd)
}

// TestTFOutDirPlanKeyForStackUnits verifies that units generated by a changed stack are
// mirrored under --out-dir relative to the working directory, the same as the units a
// filesystem walk finds.
func TestTFOutDirPlanKeyForStackUnits(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	outDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)

	_ = createTestUnit(t, filepath.Join(tmpDir, "catalog", "units", "unit"), `# Unit`)

	stackDir := filepath.Join(tmpDir, "live", "stack")
	err := os.MkdirAll(stackDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(`unit "first" {
	source = "${get_repo_root()}/catalog/units/unit"
	path   = "first"
}
`), 0644)
	require.NoError(t, err)

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	err = os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(`unit "first" {
	source = "${get_repo_root()}/catalog/units/unit"
	path   = "first"
}

unit "second" {
	source = "${get_repo_root()}/catalog/units/unit"
	path   = "second"
}
`), 0644)
	require.NoError(t, err)

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Add a unit to the stack"))

	cmd := "terragrunt run --all --no-color --experiment-mode --non-interactive --working-dir " +
		filepath.Join(tmpDir, "live") +
		" --out-dir " + outDir + " --filter '[HEAD~1...HEAD]' -- plan"

	helpers.RunTerragrunt(t, cmd)

	require.FileExists(
		t,
		filepath.Join(outDir, "stack", ".terragrunt-stack", "second", "tfplan.tfplan"),
	)
	require.NoFileExists(
		t,
		filepath.Join(outDir, "live", "stack", ".terragrunt-stack", "second", "tfplan.tfplan"),
	)
}

func TestTFDestroyWithOutDirGitFilter(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	outDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)

	// create unit to destroy
	unitDir := filepath.Join(tmpDir, "unit-to-destroy")
	err := os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`# Unit to destroy`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "main.tf"), []byte(`
resource "null_resource" "test" {}
`), 0644)
	require.NoError(t, err)

	// Initial commit
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	// do initial apply
	cmd := "terragrunt run --all --no-color --non-interactive --working-dir " + tmpDir +
		" -- apply"
	helpers.RunTerragrunt(t, cmd)

	// remove unit to trigger destruction
	err = os.RemoveAll(unitDir)
	require.NoError(t, err)

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Unit removal"))

	cmd = "terragrunt run --all --no-color --experiment-mode --non-interactive --working-dir " + tmpDir +
		" --out-dir " + outDir + " --filter-allow-destroy --filter '[HEAD~1...HEAD]' -- plan"

	helpers.RunTerragrunt(t, cmd)

	// check creation of plan files
	var planFiles []string

	err = filepath.Walk(outDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".tfplan") {
			planFiles = append(planFiles, path)
		}

		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, planFiles)

	// Bug regression test

	cmd = "terragrunt run --all --no-color --non-interactive --working-dir " + tmpDir +
		" --out-dir " + outDir + " --filter-allow-destroy --filter '[HEAD~1...HEAD]' -- apply"

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	output := stdout + stderr
	require.NotContains(t, output, "Too many command line arguments")
	require.NotContains(t, output, "Expected at most one positional argument")
}

// TestTFDestroyWithOutDirGitFilterDependentsWithRacing is a regression test for
// https://github.com/gruntwork-io/terragrunt/issues/6319.
// It verifies that --filter-allow-destroy works correctly when the filter uses
// the ...[from...to] dependents syntax (not just the plain [from...to] syntax).
func TestTFDestroyWithOutDirGitFilterDependentsWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	outDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)

	err := os.WriteFile(
		filepath.Join(tmpDir, ".gitignore"),
		[]byte(".terragrunt-cache/\n.terraform/\n.terraform.lock.hcl\n"),
		0644,
	)
	require.NoError(t, err)

	// create unit to destroy
	unitDir := filepath.Join(tmpDir, "unit-to-destroy")
	err = os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`# Unit to destroy`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "main.tf"), []byte(`
resource "null_resource" "test" {}
`), 0644)
	require.NoError(t, err)

	// create unit-a which depends on unit-to-destroy.
	// This is necessary so that discoverDependentsUpstream finds unit-a as a
	// dependent and calls processUpstreamCandidate, which exercises the
	// shallow-copy path in DiscoveryContext.Copy().
	unitADir := filepath.Join(tmpDir, "unit-a")
	err = os.MkdirAll(unitADir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitADir, "terragrunt.hcl"), []byte(`
dependency "b" {
  config_path = "../unit-to-destroy"
  mock_outputs = {}
  mock_outputs_allowed_terraform_commands = ["plan", "apply"]
}
`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitADir, "main.tf"), []byte(`
resource "null_resource" "unit_a" {}
`), 0644)
	require.NoError(t, err)

	// Initial commit
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	// do initial apply
	cmd := "terragrunt run --all --no-color --non-interactive --working-dir " + tmpDir +
		" -- apply"
	helpers.RunTerragrunt(t, cmd)

	// remove unit to trigger destruction
	err = os.RemoveAll(unitDir)
	require.NoError(t, err)

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Unit removal"))

	// Use ...[HEAD~1...HEAD] (dependents syntax) instead of plain [HEAD~1...HEAD].
	// The graph expression wrapper must not strip the -destroy flag from the
	// deleted unit's DiscoveryContext.
	cmd = "terragrunt run --all --no-color --experiment-mode --non-interactive --working-dir " + tmpDir +
		" --out-dir " + outDir + " --filter-allow-destroy --filter '...[HEAD~1...HEAD]' -- plan"

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	output := stdout + stderr
	require.NotContains(t, output, "Too many command line arguments")
	require.NotContains(t, output, "Expected at most one positional argument")

	// check creation of plan files
	var planFiles []string

	err = filepath.Walk(outDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".tfplan") {
			planFiles = append(planFiles, path)
		}

		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, planFiles)
}

// TestTFFilterCompoundNegation pins the left-to-right reading of "|" for a filter whose negation
// comes first. Asking for stacks outside _stacks selects nothing in this fixture, so the run
// finds no units. The run still has to succeed: the stack under _stacks holds no units and
// would fail to generate, which is what shows the filter reached stack generation too.
func TestTFFilterCompoundNegation(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExcludeByDefault)
	rootPath := filepath.Join(tmpEnvPath, testFixtureExcludeByDefault)

	helpers.CleanupTerraformFolder(t, rootPath)

	cmd := "terragrunt run --all --no-color --working-dir " + rootPath + " --filter '!_stacks | type=stack' -- plan"
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Contains(
		t,
		stderr,
		"No units discovered",
		"Filter asking only for stacks should select no units",
	)
}

// TestTFRunAllGitFilterMarkGlobAsReadDeleted verifies that `terragrunt run --all -- plan` selects a
// unit reading a glob-tracked file even when the diff deletes that file, instead of excluding the
// unit as a removal. In the deletion-only case the unit's only "change" is a file it reads
// disappearing, so the surviving unit must be planned on the HEAD side rather than excluded by the
// destroy-protection path. The modified/added cases and the combined cases
// (where another change could mask the bug) are covered as regressions alongside it.
func TestTFRunAllGitFilterMarkGlobAsReadDeleted(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		setupChange func(t *testing.T, configDir string)
		name        string
		description string
	}{
		{
			name: "deleted-only tracked file",
			setupChange: func(t *testing.T, configDir string) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(configDir, "item-a.yml")))
			},
			description: "deleting the only tracked file must still select the reading unit",
		},
		{
			name: "modified tracked file",
			setupChange: func(t *testing.T, configDir string) {
				t.Helper()
				require.NoError(
					t,
					os.WriteFile(
						filepath.Join(configDir, "item-a.yml"),
						[]byte("a: modified\n"),
						0644,
					),
				)
			},
			description: "modifying a tracked file must select the reading unit",
		},
		{
			name: "added tracked file",
			setupChange: func(t *testing.T, configDir string) {
				t.Helper()
				require.NoError(
					t,
					os.WriteFile(filepath.Join(configDir, "item-c.yml"), []byte("c: 3\n"), 0644),
				)
			},
			description: "adding a tracked file must select the reading unit",
		},
		{
			name: "deleted tracked file alongside a modification",
			setupChange: func(t *testing.T, configDir string) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(configDir, "item-a.yml")))
				require.NoError(
					t,
					os.WriteFile(
						filepath.Join(configDir, "item-b.yml"),
						[]byte("b: modified\n"),
						0644,
					),
				)
			},
			description: "deleting a tracked file alongside another change must still select the reading unit",
		},
		{
			name: "deleted tracked file alongside an addition",
			setupChange: func(t *testing.T, configDir string) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(configDir, "item-a.yml")))
				require.NoError(
					t,
					os.WriteFile(filepath.Join(configDir, "item-c.yml"), []byte("c: 3\n"), 0644),
				)
			},
			description: "deleting a tracked file alongside an addition must still select the reading unit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)
			runner := helpers.InitTestGitRunner(t, tmpDir)

			unitDir := filepath.Join(tmpDir, "unit-reads-config")
			require.NoError(t, os.MkdirAll(unitDir, 0755))
			require.NoError(t, os.WriteFile(
				filepath.Join(unitDir, "terragrunt.hcl"),
				[]byte(`locals {
  config_files = mark_glob_as_read("../config/{*.yaml,*.yml}")
}`), 0644))
			require.NoError(
				t,
				os.WriteFile(filepath.Join(unitDir, "main.tf"), []byte("# minimal"), 0644),
			)

			configDir := filepath.Join(tmpDir, "config")
			require.NoError(t, os.MkdirAll(configDir, 0755))
			require.NoError(
				t,
				os.WriteFile(filepath.Join(configDir, "item-a.yml"), []byte("a: 1\n"), 0644),
			)
			require.NoError(
				t,
				os.WriteFile(filepath.Join(configDir, "item-b.yml"), []byte("b: 2\n"), 0644),
			)

			require.NoError(t, runner.Add(t.Context(), "."))
			require.NoError(t, runner.Commit(t.Context(), "Baseline unit and tracked config files"))

			tc.setupChange(t, configDir)

			require.NoError(t, runner.Add(t.Context(), "."))
			require.NoError(t, runner.Commit(t.Context(), "Apply change to tracked config files"))

			helpers.CleanupTerraformFolder(t, tmpDir)

			reportFilePath := filepath.Join(tmpDir, helpers.ReportFile)
			cmd := "terragrunt run --all --non-interactive --no-color --working-dir " + tmpDir +
				" --filter '[HEAD~1...HEAD]' --report-file " + reportFilePath + " -- plan"

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			// A missing backend or IaC binary may fail the plan itself, but discovery and the report
			// still run; only an unrelated failure is unexpected.
			if err != nil && !strings.Contains(stderr, "terraform") &&
				!strings.Contains(stderr, "tofu") {
				require.NoError(t, err, "Unexpected error\nstdout: %s\nstderr: %s", stdout, stderr)
			}

			require.FileExists(t, reportFilePath, "Report file should exist at %s", reportFilePath)

			runs, parseErr := report.ParseJSONRunsFromFile(reportFilePath)
			require.NoError(t, parseErr, "Should be able to parse report JSON")

			var found bool

			runNames := make([]string, 0, len(runs))

			for i := range runs {
				run := &runs[i]
				runNames = append(runNames, filepath.Base(run.Name))

				if filepath.Base(run.Name) != "unit-reads-config" || run.Ref != "HEAD" {
					continue
				}

				found = true

				assert.NotEqual(t, "excluded", run.Result,
					"HEAD-side reading unit should not be excluded: %s", tc.description)
			}

			assert.True(
				t,
				found,
				"HEAD-side reading unit should be in the report (got: %v): %s",
				runNames,
				tc.description,
			)
		})
	}
}
