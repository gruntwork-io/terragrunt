//go:build tf

package test_test

import (
	"path/filepath"
	"strings"
	"testing"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureStrictBareInclude      = "fixtures/strict-bare-include"
	testFixtureStrictBareIncludeUnits = "fixtures/strict-bare-include-units"
)

// TestTFRootTerragruntHCLStrictMode uses globally mutated state to determine if strict mode has already
// been triggered.
//
//nolint:paralleltest // strict mode is triggered through global state
func TestTFRootTerragruntHCLStrictMode(t *testing.T) {
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
			name:       "root terragrunt.hcl with root-terragrunt-hcl strict control",
			controls:   []string{"root-terragrunt-hcl"},
			strictMode: false,
			expectedError: errors.New(
				"Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern",
			),
		},
		// we cannot test `-strict-mode` flag, since we cannot know at which strict control TG will output the error.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFindParentWithDeprecatedRoot)
			rootPath := filepath.Join(tmpEnvPath, testFixtureFindParentWithDeprecatedRoot, "app")

			args := "--non-interactive --log-level debug --working-dir " + rootPath
			if tc.strictMode {
				args = "--strict-mode " + args
			}

			for _, control := range tc.controls {
				args = " --strict-control " + control + " " + args
			}

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt run "+args+" -- plan",
			)

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

// TestTFBareIncludeStrictMode uses globally mutated state to determine if strict mode has already
// been triggered.
//
//nolint:paralleltest // strict mode is triggered through global state
func TestTFBareIncludeStrictMode(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureStrictBareInclude)

	bareInclude := bareIncludeControl(t)

	testCases := []struct {
		expectedError error
		name          string
		controls      []string
		strictMode    bool
		wantWarning   bool
	}{
		{
			name:          "bare include with no strict mode or control",
			controls:      []string{},
			strictMode:    false,
			expectedError: nil,
			wantWarning:   true,
		},
		{
			name:          "bare include with bare-include strict control",
			controls:      []string{"bare-include"},
			strictMode:    false,
			expectedError: bareInclude.Error,
		},
		{
			name:          "bare include with strict mode",
			controls:      []string{},
			strictMode:    true,
			expectedError: bareInclude.Error,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStrictBareInclude)
			rootPath := filepath.Join(tmpEnvPath, testFixtureStrictBareInclude)

			args := "init --non-interactive --working-dir " + rootPath
			if tc.strictMode {
				args = "--strict-mode " + args
			}

			for _, control := range tc.controls {
				args = " --strict-control " + control + " " + args
			}

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt "+args)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectedError.Error())
			} else {
				require.NoError(t, err)
			}

			if tc.wantWarning {
				assert.Contains(t, stderr, bareInclude.Warning)
				return
			}

			assert.NotContains(t, stderr, bareInclude.Warning)
		})
	}
}

// TestTFBareIncludeWarnsOnce pins that units including the same file with a bare include log one deprecation
// warning per run.
func TestTFBareIncludeWarnsOnce(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStrictBareIncludeUnits)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStrictBareIncludeUnits)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- init",
	)
	require.NoError(t, err)

	assert.Equal(t, 1, strings.Count(stderr, bareIncludeControl(t).Warning))
}

// bareIncludeControl returns the bare-include strict control Terragrunt registers.
func bareIncludeControl(t *testing.T) *controls.Control {
	t.Helper()

	ctrl, ok := controls.New().Find(controls.BareInclude).(*controls.Control)
	require.True(t, ok)

	return ctrl
}
