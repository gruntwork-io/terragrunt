package test_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		errorAs     error
		name        string
		filterArgs  []string
		expectError bool
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
				Path: filepath.Join(rootPath, "needs-formatting/nested/deep/web/terragrunt.hcl"),
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
			errorAs:     filter.FilterQueryRequiresDiscoveryError{Query: "name=*.stack.hcl"},
		},
		{
			name:        "error: type=unit requires HCL parsing",
			filterArgs:  []string{"type=unit"},
			expectError: true,
			errorAs:     filter.FilterQueryRequiresDiscoveryError{Query: "type=unit"},
		},
		{
			name:        "error: type=stack requires HCL parsing",
			filterArgs:  []string{"type=stack"},
			expectError: true,
			errorAs:     filter.FilterQueryRequiresDiscoveryError{Query: "type=stack"},
		},
		{
			name:        "error: external=true requires HCL parsing",
			filterArgs:  []string{"external=true"},
			expectError: true,
			errorAs:     filter.FilterQueryRequiresDiscoveryError{Query: "external=true"},
		},
		{
			name:        "error: intersection with type filter",
			filterArgs:  []string{"./needs-formatting/** | type=unit"},
			expectError: true,
			errorAs:     filter.FilterQueryRequiresDiscoveryError{Query: "./needs-formatting/** | type=unit"},
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

				require.ErrorContains(t, err, tc.errorAs.Error())
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

func TestHCLValidateWithFilter(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	testCases := []struct {
		name         string
		filterArgs   []string
		expectErrors bool
	}{
		// Path-based filtering
		{
			name:       "path-based: valid files only",
			filterArgs: []string{"./valid**"},
		},
		{
			name:       "path-based: nested recursive valid",
			filterArgs: []string{"./valid/nested/deep/**"},
		},

		// Attribute-based filtering
		{
			name:         "attribute-based: type=unit",
			filterArgs:   []string{"type=unit"},
			expectErrors: true, // includes all units, including syntax-error and semantic-error
		},
		{
			name:         "attribute-based: type=stack",
			filterArgs:   []string{"type=stack"},
			expectErrors: true, // includes all stacks, including syntax-error stacks
		},

		// Negation
		{
			name:       "negation: exclude all error directories",
			filterArgs: []string{"!./syntax-error/**", "!./semantic-error/**", "!type=stack"},
		},
		{
			name:         "negation: exclude stacks",
			filterArgs:   []string{"!type=stack"},
			expectErrors: true, // includes all units, including error units
		},

		// Intersection (refinement)
		{
			name:       "intersection: valid AND type=unit",
			filterArgs: []string{"./valid** | type=unit"},
		},
		{
			name:       "intersection: valid AND NOT db",
			filterArgs: []string{"./valid** | !name=db"},
		},
		{
			name:       "intersection: stacks/valid only",
			filterArgs: []string{"./**stack** | ./**valid**"},
		},

		// Union (multiple filters)
		{
			name:       "union: valid files OR valid stacks",
			filterArgs: []string{"./valid**", "./stacks/valid/**"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureHCLFilter)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHCLFilter)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureHCLFilter, "validate")
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			// Build filter arguments with proper quoting
			filterStr := ""
			for _, filter := range tc.filterArgs {
				filterStr += fmt.Sprintf(" --filter '%s'", filter)
			}

			cmd := fmt.Sprintf("terragrunt hcl validate%s --working-dir %s", filterStr, rootPath)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			if tc.expectErrors {
				require.Error(t, err, "Expected validation to find errors for test case: %s", tc.name)
				assert.NotEmpty(t, stdout, "Expected validation errors in output for test case: %s", tc.name)
			} else {
				require.NoError(t, err, "Expected validation to succeed but got error for test case: %s\nstdout: %s\nstderr: %s", tc.name, stdout, stderr)
				assert.Empty(t, stderr, "Expected no errors but got stderr for test case: %s\nstderr: %s", tc.name, stderr)
			}
		})
	}
}

func TestHCLFormatFilterIntegration(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHCLFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHCLFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHCLFilter, "fmt")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --json --filter './needs-formatting/**' --working-dir "+rootPath)
	require.NoError(t, err)
	assert.Empty(t, stderr)

	type foundComponents struct {
		Path string `json:"path"`
		Type string `json:"type"`

		Contents []byte `json:"contents,omitempty"`
	}

	var components []foundComponents

	err = json.Unmarshal([]byte(stdout), &components)
	require.NoError(t, err)

	for _, component := range components {
		basename := "terragrunt.hcl"
		if component.Type == "stack" {
			basename = "terragrunt.stack.hcl"
		}

		filename := filepath.Join(rootPath, component.Path, basename)
		content, readErr := os.ReadFile(filename)
		require.NoError(t, readErr)

		component.Contents = content
	}

	checkCmd := "terragrunt hcl format --filter './needs-formatting/**' --check --working-dir " + rootPath

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, checkCmd)
	require.Error(t, err, "Expected --check to find unformatted files")

	formatCmd := "terragrunt hcl format --filter './needs-formatting/**' --working-dir " + rootPath

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, formatCmd)
	require.NoError(t, err, "Format command should succeed")

	for _, component := range components {
		basename := "terragrunt.hcl"
		if component.Type == "stack" {
			basename = "terragrunt.stack.hcl"
		}

		filename := filepath.Join(rootPath, component.Path, basename)
		content, err := os.ReadFile(filename)
		require.NoError(t, err)
		assert.NotEqual(t, content, component.Contents)
	}
}
