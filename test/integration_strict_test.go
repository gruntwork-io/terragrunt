package test_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
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
		name           string
		controls       []string
		strictMode     bool
		expectedStderr string
		expectedError  error
	}{
		{
			name:           "plan-all",
			controls:       []string{},
			strictMode:     false,
			expectedStderr: fmt.Sprintf(strict.NewControls().Find(strict.DeprecatedCommands).WarnFmt, "plan-all", "terragrunt run-all plan"),
			expectedError:  nil,
		},
		{
			name:           "plan-all with plan-all strict control",
			controls:       []string{"deprecated-commands"},
			strictMode:     false,
			expectedStderr: "",
			expectedError:  errors.Errorf(strict.NewControls().Find(strict.DeprecatedCommands).ErrorFmt, "plan-all", "terragrunt run-all plan"),
		},
		{
			name:           "plan-all with strict mode",
			controls:       []string{},
			strictMode:     true,
			expectedStderr: "",
			expectedError:  errors.New("is no longer supported"),
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEmptyState)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureEmptyState)

			args := "--terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir " + rootPath
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
		name           string
		controls       []string
		strictMode     bool
		expectedStderr string
		expectedError  error
	}{
		{
			name:           "root terragrunt.hcl",
			strictMode:     false,
			expectedStderr: strict.NewControls().Find(strict.RootTerragruntHCL).WarnFmt,
		},
		{
			name:          "root terragrunt.hcl with root-terragrunt-hcl strict control",
			controls:      []string{"root-terragrunt-hcl"},
			strictMode:    false,
			expectedError: errors.New(strict.NewControls().Find(strict.RootTerragruntHCL).ErrorFmt),
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
