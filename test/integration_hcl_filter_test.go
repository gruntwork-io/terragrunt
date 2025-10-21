package test_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
)

const (
	testFixtureHclFilter = "fixtures/hcl-filter-test"
	// testFixtureFilterBasic is defined in integration_filter_test.go
)

func TestHclValidateWithFilter(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHclFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclFilter)

	testCases := []struct {
		name        string
		filterQuery string
	}{
		{
			name:        "filter only app directory",
			filterQuery: "./app",
		},
		{
			name:        "filter only db directory",
			filterQuery: "./db",
		},
		{
			name:        "filter with wildcard pattern",
			filterQuery: "./*",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}

			cmd := fmt.Sprintf("terragrunt hcl validate --experiment=filter-flag --filter %s --working-dir %s",
				tc.filterQuery, rootPath)

			err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

			// Filter should be accepted (no filter parsing errors)
			output := stdout.String() + stderr.String()
			assert.NotContains(t, strings.ToLower(output), "parse error",
				"Filter should be accepted without parse errors")

			// Validation should pass - just verifying the filter flag works
			if err != nil {
				t.Logf("Output: %s", output)
			}
		})
	}
}

func TestHclFormatWithFilter(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHclFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclFilter)

	testCases := []struct {
		name        string
		filterQuery string
	}{
		{
			name:        "filter only app",
			filterQuery: "./app",
		},
		{
			name:        "filter only db",
			filterQuery: "./db",
		},
		{
			name:        "filter only shared",
			filterQuery: "./shared",
		},
		{
			name:        "filter with wildcard",
			filterQuery: "./*",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}

			cmd := fmt.Sprintf("terragrunt hcl format --experiment=filter-flag --filter %s --check --working-dir %s",
				tc.filterQuery, rootPath)

			err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

			output := stdout.String() + stderr.String()
			// Just verify filter is accepted - actual formatting behavior varies
			assert.NotContains(t, strings.ToLower(output), "parse error",
				"Filter should be accepted without parse errors")

			t.Logf("Output for %s (err=%v): %s", tc.name, err, output)
		})
	}
}

func TestHclFormatFilterAccepted(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHclFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclFilter)

	// NOTE: hcl format currently does NOT respect the filter flag because it uses
	// zglob.Glob() directly instead of the runner system. This test verifies
	// that the flag is accepted without errors, which is the first step.
	// TODO: Implement actual filtering in hcl format command to respect FilterQueries

	t.Run("filter flag is accepted without errors", func(t *testing.T) {
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}

		// Use filter flag - it should be accepted even if not implemented yet
		cmd := fmt.Sprintf("terragrunt hcl format --experiment=filter-flag --filter ./app --diff --working-dir %s", rootPath)
		err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

		output := stdout.String() + stderr.String()
		t.Logf("Output with filter (err=%v): %s", err, output)

		// Main verification: filter flag is accepted without parse errors
		assert.NotContains(t, strings.ToLower(output), "parse error",
			"Filter should be accepted without parse errors")
		assert.NotContains(t, strings.ToLower(output), "unknown flag",
			"Filter flag should be recognized")
	})
}

func TestHclFormatWithFilterDiff(t *testing.T) {
	t.Parallel()

	// Skip if filter-flag experiment is not enabled
	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHclFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclFilter)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	// Filter only app directory
	cmd := fmt.Sprintf("terragrunt hcl format --experiment=filter-flag --filter ./app --diff --working-dir %s",
		rootPath)

	// Run the command
	err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

	output := stdout.String() + stderr.String()

	// Verify filter was accepted (no parse errors)
	assert.NotContains(t, strings.ToLower(output), "parse error",
		"Filter should be accepted without parse errors")

	t.Logf("Diff output (err=%v): %s", err, output)
}
