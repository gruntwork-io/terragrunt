package cli

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/require"
)

func TestTerragruntTerraformCodeCheck(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description string
		workingDir  string
		valid       bool
	}{
		{
			description: "Directory with plain Terraform",
			workingDir:  "test-fixtures/dir-with-terraform",
			valid:       true,
		},
		{
			description: "Directory with JSON formatted Terraform",
			workingDir:  "test-fixtures/dir-with-terraform-json",
			valid:       true,
		},
		{
			description: "Directory with no Terraform",
			workingDir:  "test-fixtures/dir-with-no-terraform",
			valid:       false,
		},
		{
			description: "Directory with no files",
			workingDir:  "test-fixtures/dir-with-no-files",
			valid:       false,
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase
		testFunc := func(t *testing.T) {
			opts, err := options.NewTerragruntOptionsForTest("mock-path-for-test.hcl")
			require.NoError(t, err)
			opts.WorkingDir = testCase.workingDir
			err = checkFolderContainsTerraformCode(opts)
			if (err != nil) && testCase.valid {
				t.Error("valid terraform returned error")
			}

			if (err == nil) && !testCase.valid {
				t.Error("invalid terraform did not return error")
			}
		}
		t.Run(testCase.description, testFunc)
	}
}

func TestTerragruntHandlesCatastrophicTerraformFailure(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Use a path that doesn't exist to induce error
	tgOptions.TerraformPath = "i-dont-exist"
	err = runTerraformWithRetry(tgOptions)
	require.Error(t, err)
}
