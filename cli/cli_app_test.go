package cli

import (
	"os"
	"path/filepath"
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

func TestMissingRunAllArguments(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.TerraformCommand = ""

	err = runAll(tgOptions)
	require.Error(t, err)
	_, ok := errors.Unwrap(err).(MissingCommand)
	assert.True(t, ok)
}

// Run a benchmark on runGraphDependencies for all fixtures possible.
// This should reveal regression on execution time due to new, changed or removed features.
func BenchmarkRunGraphDependencies(b *testing.B) {
	// Setup
	b.StopTimer()
	cwd, err := os.Getwd()
	require.NoError(b, err)

	testDir := "../test"

	fixtureDirs := []struct {
		description          string
		workingDir           string
		usePartialParseCache bool
	}{
		{"PartialParseBenchmarkRegressionCaching", "fixture-regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl", true},
		{"PartialParseBenchmarkRegressionNoCache", "fixture-regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl", false},
		{"PartialParseBenchmarkRegressionIncludesCaching", "fixture-regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl", true},
		{"PartialParseBenchmarkRegressionIncludesNoCache", "fixture-regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl", false},
	}

	// Run benchmarks
	for _, fixture := range fixtureDirs {
		b.Run(fixture.description, func(b *testing.B) {
			workingDir := filepath.Join(cwd, testDir, fixture.workingDir)
			terragruntOptions, err := options.NewTerragruntOptionsForTest(workingDir)
			if fixture.usePartialParseCache {
				terragruntOptions.UsePartialParseConfigCache = true
			} else {
				terragruntOptions.UsePartialParseConfigCache = false
			}
			require.NoError(b, err)

			b.ResetTimer()
			b.StartTimer()
			err = runGraphDependencies(terragruntOptions)
			b.StopTimer()
			require.NoError(b, err)
		})
	}
}
