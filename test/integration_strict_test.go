package test_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStrictMode uses globally mutated state to determine if strict mode has already
// been triggered, so we don't run it in parallel.
//
//nolint:paralleltest,tparallel
func TestStrictMode(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureEmptyState)

	tc := []struct {
		expectedError  error
		name           string
		expectedStderr string
		controls       []string
		strictMode     bool
	}{
		{
			name:           "plan-all",
			controls:       []string{},
			strictMode:     false,
			expectedStderr: "The `plan-all` command is deprecated and will be removed in a future version of Terragrunt. Use `terragrunt run-all plan` instead.",
			expectedError:  nil,
		},
		{
			name:           "plan-all with plan-all strict control",
			controls:       []string{"deprecated-commands"},
			strictMode:     false,
			expectedStderr: "",
			expectedError:  errors.New("The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead."),
		},
		{
			name:           "plan-all with strict mode",
			controls:       []string{},
			strictMode:     true,
			expectedStderr: "",
			expectedError:  errors.New("The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead."),
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEmptyState)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureEmptyState)

			args := "--non-interactive --log-level trace --working-dir " + rootPath
			if tt.strictMode {
				args = "--strict-mode " + args
			}

			for _, control := range tt.controls {
				args = " --strict-control " + control + " " + args
			}

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan-all "+args)

			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}

			assert.Contains(t, stderr, tt.expectedStderr)
		})
	}
}

// TestRootTerragruntHCLStrictMode uses globally mutated state to determine if strict mode has already
// been triggered, so we don't run it in parallel.
//
//nolint:paralleltest,tparallel
func TestRootTerragruntHCLStrictMode(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureFindParentWithDeprecatedRoot)

	tc := []struct {
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

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFindParentWithDeprecatedRoot)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureFindParentWithDeprecatedRoot, "app")

			args := "--non-interactive --log-level debug --working-dir " + rootPath
			if tt.strictMode {
				args = "--strict-mode " + args
			}

			for _, control := range tt.controls {
				args = " --strict-control " + control + " " + args
			}

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --experiment cli-redesign "+args+" -- plan")

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}

			assert.Contains(t, stderr, tt.expectedStderr)
		})
	}
}
