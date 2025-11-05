package test_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureStrictBareInclude = "fixtures/strict-bare-include"
)

// TestRootTerragruntHCLStrictMode uses globally mutated state to determine if strict mode has already
// been triggered, so we don't run it in parallel.
//
//nolint:paralleltest,tparallel
func TestRootTerragruntHCLStrictMode(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureFindParentWithDeprecatedRoot)

	testCases := []struct {
		expectedError  error
		name           string
		expectedStderr string
		controls       []string
		strictMode     bool
	}{
		{
			name:           "root terragrunt.hcl",
			strictMode:     false,
			expectedStderr: "Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern",
		},
		{
			name:          "root terragrunt.hcl with root-terragrunt-hcl strict control",
			controls:      []string{"root-terragrunt-hcl"},
			strictMode:    false,
			expectedError: errors.New("Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern"),
		},
		// we cannot test `-strict-mode` flag, since we cannot know at which strict control TG will output the error.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFindParentWithDeprecatedRoot)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureFindParentWithDeprecatedRoot, "app")

			args := "--non-interactive --log-level debug --working-dir " + rootPath
			if tc.strictMode {
				args = "--strict-mode " + args
			}

			for _, control := range tc.controls {
				args = " --strict-control " + control + " " + args
			}

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run "+args+" -- plan")

			if tc.expectedError != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError.Error())
			} else {
				require.NoError(t, err)
			}

			assert.Contains(t, stderr, tc.expectedStderr)
		})
	}
}

// TestBareIncludeStrictMode uses globally mutated state to determine if strict mode has already
// been triggered, so we don't run it in parallel.
//
//nolint:paralleltest,tparallel
func TestBareIncludeStrictMode(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureStrictBareInclude)

	testCases := []struct {
		expectedError error
		name          string
		controls      []string
		strictMode    bool
	}{
		{
			name:          "bare include with no strict mode or control",
			controls:      []string{},
			strictMode:    false,
			expectedError: nil,
		},
		{
			name:          "bare include with bare-include strict control",
			controls:      []string{"bare-include"},
			strictMode:    false,
			expectedError: errors.New("Using an `include` block without a label is deprecated. Please use the `include` block with a label instead."),
		},
		{
			name:          "bare include with strict mode",
			controls:      []string{},
			strictMode:    true,
			expectedError: errors.New("Using an `include` block without a label is deprecated. Please use the `include` block with a label instead."),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStrictBareInclude)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureStrictBareInclude)

			args := "init --non-interactive --log-level trace --working-dir " + rootPath
			if tc.strictMode {
				args = "--strict-mode " + args
			}

			for _, control := range tc.controls {
				args = " --strict-control " + control + " " + args
			}

			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt "+args)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectedError.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
