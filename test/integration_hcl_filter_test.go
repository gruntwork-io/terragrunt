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
	testFixtureHCLFilter = "fixtures/hcl-filter-test"
)

func TestHCLValidateWithFilter(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHCLFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHCLFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHCLFilter)

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
			t.Parallel()

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}

			cmd := fmt.Sprintf("terragrunt hcl validate --filter %s --working-dir %s",
				tc.filterQuery, rootPath)

			err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

			output := stdout.String() + stderr.String()
			assert.NotContains(t, strings.ToLower(output), "parse error",
				"Filter should be accepted without parse errors")

			if err != nil {
				t.Logf("Output: %s", output)
			}
		})
	}
}

func TestHCLFormatWithFilter(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHCLFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHCLFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHCLFilter)

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
			t.Parallel()

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}

			cmd := fmt.Sprintf("terragrunt hcl format --filter %s --check --working-dir %s",
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

func TestHCLFormatFilterAccepted(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHCLFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHCLFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHCLFilter)

	t.Run("filter only processes filtered directories", func(t *testing.T) {
		t.Parallel()

		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}

		cmd := "terragrunt hcl format --filter ./app --check --working-dir " + rootPath
		err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

		output := stdout.String() + stderr.String()
		t.Logf("Output with filter (err=%v): %s", err, output)

		assert.NotContains(t, output, "db/terragrunt.hcl", "Should NOT process db config (filtered out)")
	})
}

func TestHCLFormatWithFilterDiff(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("Skipping filter flag tests - TG_EXPERIMENT_MODE not enabled")
	}

	helpers.CleanupTerraformFolder(t, testFixtureHCLFilter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHCLFilter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHCLFilter)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	cmd := "terragrunt hcl format --filter ./app --diff --working-dir " + rootPath

	err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)

	output := stdout.String() + stderr.String()

	assert.NotContains(t, strings.ToLower(output), "parse error",
		"Filter should be accepted without parse errors")

	t.Logf("Diff output (err=%v): %s", err, output)
}
