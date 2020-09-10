package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetTerragruntInputsAsEnvVars(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		description    string
		envVarsInOpts  map[string]string
		inputsInConfig map[string]interface{}
		expected       map[string]string
	}{
		{
			description:    "No env vars in opts, no inputs",
			envVarsInOpts:  nil,
			inputsInConfig: nil,
			expected:       map[string]string{},
		},
		{
			description:    "A few env vars in opts, no inputs",
			envVarsInOpts:  map[string]string{"foo": "bar"},
			inputsInConfig: nil,
			expected:       map[string]string{"foo": "bar"},
		},
		{
			description:    "No env vars in opts, one input",
			envVarsInOpts:  nil,
			inputsInConfig: map[string]interface{}{"foo": "bar"},
			expected:       map[string]string{"TF_VAR_foo": "bar"},
		},
		{
			description:    "No env vars in opts, a few inputs",
			envVarsInOpts:  nil,
			inputsInConfig: map[string]interface{}{"foo": "bar", "list": []int{1, 2, 3}, "map": map[string]interface{}{"a": "b"}},
			expected:       map[string]string{"TF_VAR_foo": "bar", "TF_VAR_list": "[1,2,3]", "TF_VAR_map": `{"a":"b"}`},
		},
		{
			description:    "A few env vars in opts, a few inputs, no overlap",
			envVarsInOpts:  map[string]string{"foo": "bar", "something": "else"},
			inputsInConfig: map[string]interface{}{"foo": "bar", "list": []int{1, 2, 3}, "map": map[string]interface{}{"a": "b"}},
			expected:       map[string]string{"foo": "bar", "something": "else", "TF_VAR_foo": "bar", "TF_VAR_list": "[1,2,3]", "TF_VAR_map": `{"a":"b"}`},
		},
		{
			description:    "A few env vars in opts, a few inputs, with overlap",
			envVarsInOpts:  map[string]string{"foo": "bar", "TF_VAR_foo": "original", "TF_VAR_list": "original"},
			inputsInConfig: map[string]interface{}{"foo": "bar", "list": []int{1, 2, 3}, "map": map[string]interface{}{"a": "b"}},
			expected:       map[string]string{"foo": "bar", "TF_VAR_foo": "original", "TF_VAR_list": "original", "TF_VAR_map": `{"a":"b"}`},
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase
		t.Run(testCase.description, func(t *testing.T) {
			opts, err := options.NewTerragruntOptionsForTest("mock-path-for-test.hcl")
			require.NoError(t, err)
			opts.Env = testCase.envVarsInOpts

			cfg := &config.TerragruntConfig{Inputs: testCase.inputsInConfig}

			require.NoError(t, setTerragruntInputsAsEnvVars(opts, cfg))

			assert.Equal(t, testCase.expected, opts.Env)
		})
	}
}

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
	_, err = runTerraformWithRetry(tgOptions)
	require.Error(t, err)
}

func TestTerragruntTerraformErrorsRequiringInit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		description string
		output      string
		err         error
		options     *options.TerragruntOptions
		matches     bool
	}{
		{
			description: "Does not match as auto retry and auto init are disabled",
			err:         errors.New("A a-test-error ocurred"),
			options:     &options.TerragruntOptions{},
			matches:     false,
		},
		{
			description: "Does not match as auto init is disabled",
			err:         errors.New("A a-test-error ocurred"),
			options: &options.TerragruntOptions{
				AutoInit:  false,
				AutoRetry: true,
			},
		},
		{
			description: "Does not match as auto retry is disabled",
			err:         errors.New("A a-test-error ocurred"),
			options: &options.TerragruntOptions{
				AutoInit:  true,
				AutoRetry: false,
			},
			matches: false,
		},
		{
			description: "Does not match with no error",
			options: &options.TerragruntOptions{
				AutoInit:  true,
				AutoRetry: true,
			},
			matches: false,
		},
		{
			description: "Does not match an error from an empty list",
			output:      "This is an error message",
			err:         errors.New("A a-test-error ocurred"),
			options: &options.TerragruntOptions{
				AutoInit:            true,
				AutoRetry:           true,
				ErrorsRequiringInit: []string{},
			},
			matches: false,
		},
		{
			description: "Does not match an error from the provided list using regular expression",
			output:      "This is an error message",
			err:         errors.New("A a-test-error ocurred"),
			options: &options.TerragruntOptions{
				AutoInit:  true,
				AutoRetry: true,
				ErrorsRequiringInit: []string{
					"(?s).*some-error.*",
					"(?s).*a-test-error.*",
					"(?s).*other-error.*",
				},
			},
			matches: false,
		},
		{
			description: "Does not match an error from the provided list using bare string",
			output:      "This is an error message",
			err:         errors.New("A a-test-error ocurred"),
			options: &options.TerragruntOptions{
				AutoInit:  true,
				AutoRetry: true,
				ErrorsRequiringInit: []string{
					"some-error",
					"a-test-error",
					"other-error",
				},
			},
			matches: false,
		},
		{
			description: "Matches an error from the provided list using regular expression",
			output:      "This is a-test-error message",
			err:         errors.New("A a-test-error ocurred"),
			options: &options.TerragruntOptions{
				AutoInit:  true,
				AutoRetry: true,
				ErrorsRequiringInit: []string{
					"(?s).*some-error.*",
					"(?s).*a-test-error.*",
					"(?s).*other-error.*",
				},
			},
			matches: true,
		},
		{
			description: "Matches an error from the provided list using bare string",
			output:      "This is a-test-error message",
			err:         errors.New("A a-test-error ocurred"),
			options: &options.TerragruntOptions{
				AutoInit:  true,
				AutoRetry: true,
				ErrorsRequiringInit: []string{
					"some-error",
					"a-test-error",
					"other-error",
				},
			},
			matches: true,
		},
		{
			description: "Matches a multi-line error from the provided list using regular expression",
			output:      fmt.Sprintln("This is\na-test-error\nmessage"),
			err:         errors.New("A a-test-error ocurred"),
			options: &options.TerragruntOptions{
				AutoInit:  true,
				AutoRetry: true,
				ErrorsRequiringInit: []string{
					"(?s).*some-error.*",
					"(?s).*a-test-error.*",
					"(?s).*other-error.*",
				},
			},
			matches: true,
		},
		{
			description: "Matches a multi-line error with ANSI colors from the provided list using regular expression",
			output:      fmt.Sprintln("\x1b[31mThis is\x1b[0m\n\x1b[32ma-test-error\x1b[0m\n\x1b[34mmessage\x1b[0m"),
			err:         errors.New("A a-test-error ocurred"),
			options: &options.TerragruntOptions{
				AutoInit:  true,
				AutoRetry: true,
				ErrorsRequiringInit: []string{
					"(?s).*some-error.*",
					"(?s).*a-test-error.*",
					"(?s).*other-error.*",
				},
			},
			matches: true,
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		t.Run(testCase.description, func(t *testing.T) {
			actual := isErrorRequiringInit(testCase.output, testCase.err, testCase.options)
			assert.Equal(t, testCase.matches, actual)
		})
	}
}
