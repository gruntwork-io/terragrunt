package terraform

import (
	"testing"

	goerrors "github.com/go-errors/errors"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
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

			require.NoError(t, SetTerragruntInputsAsEnvVars(opts, cfg))

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
			err = CheckFolderContainsTerraformCode(opts)
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

func TestErrorRetryableOnStdoutError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{".*error.*"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	retryable := isRetryable("error is here", "", errors.WithStackTrace(goerrors.New("dummy error")), tgOptions)
	require.True(t, retryable, "The error should have retried")
}

func TestErrorMultipleRetryableOnStderrError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{"no match", ".*error.*"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	retryable := isRetryable("", "error is here", errors.WithStackTrace(goerrors.New("dummy error")), tgOptions)
	require.True(t, retryable, "The error should have retried")
}

func TestEmptyRetryablesOnStderrError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	retryable := isRetryable("", "error is here", errors.WithStackTrace(goerrors.New("dummy error")), tgOptions)
	require.False(t, retryable, "The error should not have retried, the list of retryable errors was empty")
}

func TestErrorRetryableOnStderrError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{".*error.*"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	retryable := isRetryable("", "error is here", errors.WithStackTrace(goerrors.New("dummy error")), tgOptions)
	require.True(t, retryable, "The error should have retried")
}

func TestErrorNotRetryableOnStdoutError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{"not the error"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	retryable := isRetryable("error is here", "", errors.WithStackTrace(goerrors.New("dummy error")), tgOptions)
	require.False(t, retryable, "The error should not retry")
}

func TestErrorNotRetryableOnStderrError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{"not the error"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	retryable := isRetryable("", "error is here", errors.WithStackTrace(goerrors.New("dummy error")), tgOptions)
	require.False(t, retryable, "The error should not retry")
}

func TestErrorNotRetryableOnStderrWithoutError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{".*"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	retryable := isRetryable("", "error is here", nil, tgOptions)
	require.False(t, retryable, "The error should not retry")
}

func TestAutoRetryFalseDisablesRetry(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{".*"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = false

	retryable := isRetryable("", "error is here", nil, tgOptions)
	require.False(t, retryable, "The error should not retry")
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

func TestToTerraformEnvVars(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		description string
		vars        map[string]interface{}
		expected    map[string]string
	}{
		{
			"empty",
			map[string]interface{}{},
			map[string]string{},
		},
		{
			"string value",
			map[string]interface{}{"foo": "bar"},
			map[string]string{"TF_VAR_foo": `bar`},
		},
		{
			"int value",
			map[string]interface{}{"foo": 42},
			map[string]string{"TF_VAR_foo": `42`},
		},
		{
			"bool value",
			map[string]interface{}{"foo": true},
			map[string]string{"TF_VAR_foo": `true`},
		},
		{
			"list value",
			map[string]interface{}{"foo": []string{"a", "b", "c"}},
			map[string]string{"TF_VAR_foo": `["a","b","c"]`},
		},
		{
			"map value",
			map[string]interface{}{"foo": map[string]interface{}{"a": "b", "c": "d"}},
			map[string]string{"TF_VAR_foo": `{"a":"b","c":"d"}`},
		},
		{
			"nested map value",
			map[string]interface{}{"foo": map[string]interface{}{"a": []int{1, 2, 3}, "b": "c", "d": map[string]interface{}{"e": "f"}}},
			map[string]string{"TF_VAR_foo": `{"a":[1,2,3],"b":"c","d":{"e":"f"}}`},
		},
		{
			"multiple values",
			map[string]interface{}{"str": "bar", "int": 42, "bool": false, "list": []int{1, 2, 3}, "map": map[string]interface{}{"a": "b"}},
			map[string]string{"TF_VAR_str": `bar`, "TF_VAR_int": `42`, "TF_VAR_bool": `false`, "TF_VAR_list": `[1,2,3]`, "TF_VAR_map": `{"a":"b"}`},
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase
		t.Run(testCase.description, func(t *testing.T) {
			actual, err := toTerraformEnvVars(testCase.vars)
			require.NoError(t, err)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}
