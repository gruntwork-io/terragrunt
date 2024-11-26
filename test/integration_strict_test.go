package test_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictMode(t *testing.T) {
	t.Parallel()

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
			expectedStderr: "The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.",
			expectedError:  nil,
		},
		{
			name:           "plan-all with plan-all strict control",
			controls:       []string{"plan-all"},
			strictMode:     false,
			expectedStderr: "",
			expectedError:  strict.StrictControls[strict.PlanAll].Error,
		},
		{
			name:           "plan-all with multiple strict controls",
			controls:       []string{"plan-all", "apply-all"},
			strictMode:     false,
			expectedStderr: "",
			expectedError:  strict.StrictControls[strict.PlanAll].Error,
		},
		{
			name:           "plan-all with strict mode",
			controls:       []string{},
			strictMode:     true,
			expectedStderr: "",
			expectedError:  strict.StrictControls[strict.PlanAll].Error,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEmptyState)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureEmptyState)

			args := "--terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir " + rootPath
			if tt.strictMode {
				args = "--strict-mode " + args
			}

			for _, control := range tt.controls {
				args = " --strict-control " + control + " " + args
			}

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan-all "+args)

			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}

			assert.Contains(t, stderr, tt.expectedStderr)
		})
	}
}

func TestTerragruntTerraformOutputJson(t *testing.T) {
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInitError)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureInitError)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply --no-color --terragrunt-json-log --terragrunt-tf-logs-to-json --terragrunt-non-interactive --terragrunt-working-dir "+testPath)
	require.Error(t, err)

	// Sometimes, this is the error returned by AWS.
	if !strings.Contains(stderr, "Error: Failed to get existing workspaces: operation error S3: ListObjectsV2, https response error StatusCode: 301") {
		assert.Regexp(t, stderr, `"msg":".*`+regexp.QuoteMeta("Initializing the backend..."))
	}

	// check if output can be extracted in json
	jsonStrings := strings.Split(stderr, "\n")
	for _, jsonString := range jsonStrings {
		if len(jsonString) == 0 {
			continue
		}
		var output map[string]interface{}
		err = json.Unmarshal([]byte(jsonString), &output)
		require.NoErrorf(t, err, "Failed to parse json %s", jsonString)
		assert.NotNil(t, output["level"])
		assert.NotNil(t, output["time"])
	}
}
