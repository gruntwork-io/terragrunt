package test_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureHCLFilter = "fixtures/hcl-filter"
)

func TestHCLFormatCheckWithFilter(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	// Create a temporary directory for this test case
	helpers.CleanupTerraformFolder(t, testFixtureHCLFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHCLFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHCLFilter, "fmt")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		filterArgs  []string
		expectError bool
		errorAs     error
	}{
		// Path-based filtering
		{
			name:        "path-based: recursive in needs-formatting",
			filterArgs:  []string{"./needs-formatting/**"},
			expectError: true,
			errorAs: format.FileNeedsFormattingError{
				Path: filepath.Join(rootPath, "needs-formatting/nested/deep/web/terragrunt.hcl"),
			},
		},
		{
			name:       "path-based: specific directory with hcl file",
			filterArgs: []string{"./already-formatted/app1/**"},
		},
		{
			name:        "path-based: nested recursive",
			filterArgs:  []string{"./needs-formatting/nested/deep/**"},
			expectError: true,
			errorAs: format.FileNeedsFormattingError{
				Path: filepath.Join(rootPath, "needs-formatting/nested/api/terragrunt.hcl"),
			},
		},
		{
			name:       "path-based: wrapped path",
			filterArgs: []string{"{./already-formatted/app2/**}"},
		},
		{
			name:        "path-based: stack files with .stack.hcl extension",
			filterArgs:  []string{"./stacks/**"},
			expectError: true,
			errorAs: format.FileNeedsFormattingError{
				Path: filepath.Join(rootPath, "stacks/needs-formatting/stack1/terragrunt.stack.hcl"),
			},
		},

		// Negation with path filters
		{
			name:        "negation: exclude path '!./needs-formatting/**'",
			filterArgs:  []string{"!./needs-formatting/**"},
			expectError: true,
			errorAs: format.FileNeedsFormattingError{
				Path: filepath.Join(rootPath, "stacks/needs-formatting/stack1/terragrunt.stack.hcl"),
			},
		},
		{
			name:        "negation: exclude stack files by name",
			filterArgs:  []string{"!./**stack.hcl"},
			expectError: true,
			errorAs: format.FileNeedsFormattingError{
				Path: filepath.Join(rootPath, "needs-formatting/nested/deep/web/terragrunt.hcl"),
			},
		},

		// Intersection (refinement)
		{
			name:        "intersection: path AND filename",
			filterArgs:  []string{"./needs-formatting/** | ./**/terragrunt.hcl"},
			expectError: true,
			errorAs: format.FileNeedsFormattingError{
				Path: filepath.Join(rootPath, "needs-formatting/nested/deep/web/terragrunt.hcl"),
			},
		},
		{
			name:       "intersection: path AND negated filename",
			filterArgs: []string{"./stacks/** | !./**/stack1/*"},
		},

		// Union (multiple filters)
		{
			name:        "union: multiple paths",
			filterArgs:  []string{"./needs-formatting/db/**", "./already-formatted/app1/**"},
			expectError: true,
			errorAs: format.FileNeedsFormattingError{
				Path: filepath.Join(rootPath, "needs-formatting/db/terragrunt.hcl"),
			},
		},

		// Attribute filters
		{
			name:        "error: name=*.stack.hcl requires HCL parsing",
			filterArgs:  []string{"name=*.stack.hcl"},
			expectError: true,
			errorAs:     filter.FilterQueryRequiresHCLParsingError{Query: "name=*.stack.hcl"},
		},
		{
			name:        "error: type=unit requires HCL parsing",
			filterArgs:  []string{"type=unit"},
			expectError: true,
			errorAs:     filter.FilterQueryRequiresHCLParsingError{Query: "type=unit"},
		},
		{
			name:        "error: type=stack requires HCL parsing",
			filterArgs:  []string{"type=stack"},
			expectError: true,
			errorAs:     filter.FilterQueryRequiresHCLParsingError{Query: "type=stack"},
		},
		{
			name:        "error: external=true requires HCL parsing",
			filterArgs:  []string{"external=true"},
			expectError: true,
			errorAs:     filter.FilterQueryRequiresHCLParsingError{Query: "external=true"},
		},
		{
			name:        "error: intersection with type filter",
			filterArgs:  []string{"./needs-formatting/** | type=unit"},
			expectError: true,
			errorAs:     filter.FilterQueryRequiresHCLParsingError{Query: "./needs-formatting/** | type=unit"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Build filter arguments
			filterStr := ""
			for _, filter := range tc.filterArgs {
				filterStr += fmt.Sprintf(" --filter '%s'", filter)
			}

			cmd := fmt.Sprintf(
				"terragrunt hcl fmt %s --check --working-dir %s",
				filterStr,
				rootPath,
			)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectError {
				// Command should fail with expected error
				require.Error(t, err, "Expected command to fail but it succeeded")
				assert.ErrorAs(t, err, &tc.errorAs)
				assert.Equal(t, tc.errorAs.Error(), err.Error())
			} else {
				// Command should succeed
				require.NoError(
					t,
					err,
					"Expected command to succeed but got error: %v\nstdout: %s\nstderr: %s",
					err, stdout, stderr,
				)
			}
		})
	}
}

// TestHCLValidateWithFilter tests the hcl validate command with various filter expressions
func TestHCLValidateWithFilter(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	testCases := []struct {
		name            string
		filterArgs      []string
		expectedInclude []string
		expectedExclude []string
		expectErrors    bool
	}{
		// Name-based filtering
		{
			name:            "name-based: exact match 'web'",
			filterArgs:      []string{"web"},
			expectedInclude: []string{"web"},
			expectedExclude: []string{"api", "db"},
			expectErrors:    false,
		},
		{
			name:            "name-based: exact match 'invalid-char'",
			filterArgs:      []string{"invalid-char"},
			expectedInclude: []string{"invalid-char"},
			expectedExclude: []string{"web", "api"},
			expectErrors:    true,
		},

		// Path-based filtering
		{
			name:            "path-based: valid files only",
			filterArgs:      []string{"./valid/**"},
			expectedInclude: []string{"valid/nested/deep/web", "valid/nested/api", "valid/db"},
			expectedExclude: []string{"syntax-error", "semantic-error"},
			expectErrors:    false,
		},
		{
			name:            "path-based: syntax-error files",
			filterArgs:      []string{"./syntax-error/**"},
			expectedInclude: []string{"syntax-error/invalid-char", "syntax-error/invalid-key"},
			expectedExclude: []string{"valid/", "semantic-error/"},
			expectErrors:    true,
		},
		{
			name:            "path-based: semantic-error files",
			filterArgs:      []string{"./semantic-error/**"},
			expectedInclude: []string{"semantic-error/incomplete-block", "semantic-error/missing-value"},
			expectedExclude: []string{"valid/", "syntax-error/"},
			expectErrors:    true,
		},
		{
			name:            "path-based: nested recursive valid",
			filterArgs:      []string{"./valid/nested/deep/**"},
			expectedInclude: []string{"valid/nested/deep/web"},
			expectedExclude: []string{"valid/nested/api", "valid/db"},
			expectErrors:    false,
		},

		// Attribute-based filtering
		{
			name:            "attribute-based: type=unit",
			filterArgs:      []string{"type=unit"},
			expectedInclude: []string{"terragrunt.hcl"},
			expectedExclude: []string{".stack.hcl"},
			expectErrors:    true, // Will include error files
		},
		{
			name:            "attribute-based: type=stack",
			filterArgs:      []string{"type=stack"},
			expectedInclude: []string{".stack.hcl"},
			expectedExclude: []string{"valid/nested/deep/web", "valid/nested/api", "valid/db"},
			expectErrors:    true, // stack2 has syntax error
		},

		// Negation
		{
			name:            "negation: exclude syntax errors",
			filterArgs:      []string{"!./syntax-error/**"},
			expectedInclude: []string{"valid/", "semantic-error/"},
			expectedExclude: []string{"syntax-error/"},
			expectErrors:    true, // semantic errors still present
		},
		{
			name:            "negation: exclude all errors",
			filterArgs:      []string{"!./syntax-error/**", "!./semantic-error/**"},
			expectedInclude: []string{"valid/nested/deep/web", "valid/db"},
			expectedExclude: []string{"syntax-error/", "semantic-error/"},
			expectErrors:    false,
		},
		{
			name:            "negation: exclude stacks",
			filterArgs:      []string{"!type=stack"},
			expectedInclude: []string{"web", "api"},
			expectedExclude: []string{".stack.hcl"},
			expectErrors:    true, // unit errors present
		},

		// Intersection (refinement)
		{
			name:            "intersection: valid AND type=unit",
			filterArgs:      []string{"./valid/** | type=unit"},
			expectedInclude: []string{"valid/nested/deep/web", "valid/nested/api", "valid/db"},
			expectedExclude: []string{"syntax-error/", "semantic-error/"},
			expectErrors:    false,
		},
		{
			name:            "intersection: valid AND NOT db",
			filterArgs:      []string{"./valid/** | !name=db"},
			expectedInclude: []string{"valid/nested/deep/web", "valid/nested/api"},
			expectedExclude: []string{"valid/db"},
			expectErrors:    false,
		},
		{
			name:            "intersection: stacks AND valid",
			filterArgs:      []string{"./stacks/** | ./stacks/valid/**"},
			expectedInclude: []string{"stacks/valid/stack1"},
			expectedExclude: []string{"stacks/syntax-error/stack2"},
			expectErrors:    false,
		},

		// Union (multiple filters)
		{
			name:            "union: web OR api",
			filterArgs:      []string{"web", "api"},
			expectedInclude: []string{"web", "api"},
			expectedExclude: []string{"db"},
			expectErrors:    false,
		},
		{
			name:            "union: valid OR stacks/valid",
			filterArgs:      []string{"./valid/**", "./stacks/valid/**"},
			expectedInclude: []string{"valid/nested/deep/web", "stacks/valid/stack1"},
			expectedExclude: []string{"syntax-error/", "semantic-error/", "stacks/syntax-error"},
			expectErrors:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureHCLFilter)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHCLFilter)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureHCLFilter, "validate")

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}

			// Build filter arguments
			filterStr := ""
			for _, filter := range tc.filterArgs {
				filterStr += fmt.Sprintf(" --filter '%s'", filter)
			}

			cmd := fmt.Sprintf("terragrunt hcl validate%s --working-dir %s",
				filterStr, rootPath)

			err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

			output := stdout.String() + stderr.String()

			// Check if errors were expected
			if tc.expectErrors {
				assert.Error(t, err, "Expected validation errors but command succeeded")
			} else {
				require.NoError(t, err, "Expected no validation errors but got: %s", output)
			}

			// Verify inclusion: files that should be processed are mentioned
			for _, expectedPath := range tc.expectedInclude {
				assert.Contains(t, output, expectedPath,
					"Expected output to contain '%s' but it was not found. Output:\n%s", expectedPath, output)
			}

			// Verify exclusion: files that should NOT be processed are NOT mentioned
			for _, excludedPath := range tc.expectedExclude {
				assert.NotContains(t, output, excludedPath,
					"Expected output to NOT contain '%s' but it was found. Output:\n%s", excludedPath, output)
			}
		})
	}
}

// TestHCLFormatFilterIntegration tests format command actually formats only filtered files
func TestHCLFormatFilterIntegration(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHCLFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHCLFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHCLFilter, "fmt")

	t.Run("format only needs-formatting directory", func(t *testing.T) {
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}

		// First check which files need formatting
		checkCmd := fmt.Sprintf("terragrunt hcl format --filter './needs-formatting/**' --check --working-dir %s", rootPath)
		checkErr := helpers.RunTerragruntCommand(t, checkCmd, &stdout, &stderr)

		// Should find files that need formatting
		assert.Error(t, checkErr, "Expected --check to find unformatted files")

		checkOutput := stdout.String() + stderr.String()
		assert.Contains(t, checkOutput, "needs-formatting", "Should detect needs-formatting files")

		// Now actually format them
		stdout.Reset()
		stderr.Reset()

		formatCmd := fmt.Sprintf("terragrunt hcl format --filter './needs-formatting/**' --working-dir %s", rootPath)
		err := helpers.RunTerragruntCommand(t, formatCmd, &stdout, &stderr)
		require.NoError(t, err, "Format command should succeed")

		formatOutput := stdout.String() + stderr.String()

		// Verify the files in needs-formatting were processed
		webPath := filepath.Join(rootPath, "needs-formatting", "nested", "deep", "web", "terragrunt.hcl")
		webContent, err := util.ReadFileAsString(webPath)
		require.NoError(t, err)

		// Check that formatting was applied (no extra spaces around =)
		assert.NotContains(t, webContent, "web_name =                    \"web-service\"",
			"File should be formatted")
		assert.Contains(t, webContent, "web_name", "File should still contain web_name")

		// Verify files in already-formatted were NOT changed
		assert.NotContains(t, formatOutput, "already-formatted",
			"Should not process already-formatted directory")
	})
}

// TestHCLValidateFilterErrorDetection tests that validate detects errors only in filtered files
func TestHCLValidateFilterErrorDetection(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHCLFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHCLFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHCLFilter, "validate")

	t.Run("validate only valid files - should pass", func(t *testing.T) {
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}

		cmd := fmt.Sprintf("terragrunt hcl validate --filter './valid/**' --working-dir %s", rootPath)
		err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

		output := stdout.String() + stderr.String()
		assert.NoError(t, err, "Valid files should pass validation. Output: %s", output)
		assert.Contains(t, output, "valid/", "Should process valid directory")
		assert.NotContains(t, strings.ToLower(output), "error", "Should not have errors")
	})

	t.Run("validate only syntax-error files - should fail", func(t *testing.T) {
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}

		cmd := fmt.Sprintf("terragrunt hcl validate --filter './syntax-error/**' --working-dir %s", rootPath)
		err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

		output := stdout.String() + stderr.String()
		assert.Error(t, err, "Syntax error files should fail validation")
		assert.Contains(t, output, "syntax-error", "Should process syntax-error directory")
	})

	t.Run("validate with negation - exclude errors", func(t *testing.T) {
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}

		cmd := fmt.Sprintf("terragrunt hcl validate --filter '!./syntax-error/**' --filter '!./semantic-error/**' --working-dir %s", rootPath)
		err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

		output := stdout.String() + stderr.String()
		assert.NoError(t, err, "Should pass when error files are excluded. Output: %s", output)
		assert.NotContains(t, output, "syntax-error", "Should not process syntax-error directory")
		assert.NotContains(t, output, "semantic-error", "Should not process semantic-error directory")
	})
}
