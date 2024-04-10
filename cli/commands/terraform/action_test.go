package terraform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/sirupsen/logrus"
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

	out := &shell.CmdOutput{
		Stdout: "",
		Stderr: "error is here",
	}

	retryable := isRetryable(tgOptions, out)
	require.True(t, retryable, "The error should have retried")
}

func TestErrorMultipleRetryableOnStderrError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{"no match", ".*error.*"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	out := &shell.CmdOutput{
		Stdout: "",
		Stderr: "error is here",
	}

	retryable := isRetryable(tgOptions, out)
	require.True(t, retryable, "The error should have retried")
}

func TestEmptyRetryablesOnStderrError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	out := &shell.CmdOutput{
		Stdout: "",
		Stderr: "error is here",
	}

	retryable := isRetryable(tgOptions, out)
	require.False(t, retryable, "The error should not have retried, the list of retryable errors was empty")
}

func TestErrorRetryableOnStderrError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{".*error.*"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	out := &shell.CmdOutput{
		Stdout: "",
		Stderr: "error is here",
	}

	retryable := isRetryable(tgOptions, out)
	require.True(t, retryable, "The error should have retried")
}

func TestErrorNotRetryableOnStdoutError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{"not the error"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	out := &shell.CmdOutput{
		Stdout: "error is here",
		Stderr: "",
	}

	retryable := isRetryable(tgOptions, out)
	require.False(t, retryable, "The error should not retry")
}

func TestErrorNotRetryableOnStderrError(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	retryableErrors := []string{"not the error"}
	tgOptions.RetryableErrors = retryableErrors
	tgOptions.AutoRetry = true

	out := &shell.CmdOutput{
		Stdout: "",
		Stderr: "error is here",
	}

	retryable := isRetryable(tgOptions, out)
	require.False(t, retryable, "The error should not retry")
}

func TestTerragruntHandlesCatastrophicTerraformFailure(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Use a path that doesn't exist to induce error
	tgOptions.TerraformPath = "i-dont-exist"
	err = runTerraformWithRetry(context.Background(), tgOptions)
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

func TestFilterTerraformExtraArgs(t *testing.T) {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	workingDir = filepath.ToSlash(workingDir)

	temporaryFile := createTempFile(t)

	testCases := []struct {
		options      *options.TerragruntOptions
		extraArgs    config.TerraformExtraArguments
		expectedArgs []string
	}{
		// Standard scenario
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"apply", "plan", "destroy"}, []string{}, []string{}),
			[]string{"--foo", "bar"},
		},
		// optional existing var file
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"apply", "plan"}, []string{}, []string{temporaryFile}),
			[]string{"--foo", "bar", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// required var file + optional existing var file
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"apply", "plan"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// non existing required var file + non existing optional var file
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"apply", "plan"}, []string{"required.tfvars"}, []string{"optional.tfvars"}),
			[]string{"--foo", "bar", "-var-file=required.tfvars"},
		},
		// plan providing a folder, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"plan", workingDir}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// apply providing a folder, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"apply", workingDir}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "-var='key=value'"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "-var-file=test.tfvars", "-var='key=value'", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// apply providing a file, no var files included
		{
			mockCmdOptions(t, workingDir, []string{"apply", temporaryFile}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "foo"},
		},

		// apply providing no params, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// apply with some parameters, providing a file => no var files included
		{
			mockCmdOptions(t, workingDir, []string{"apply", "-no-color", "-foo", temporaryFile}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "foo"},
		},
		// destroy providing a folder, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"destroy", workingDir}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "-var='key=value'"}, []string{"plan", "destroy"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "-var-file=test.tfvars", "-var='key=value'", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// destroy providing a file, no var files included
		{
			mockCmdOptions(t, workingDir, []string{"destroy", temporaryFile}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "destroy"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "foo"},
		},

		// destroy providing no params, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"destroy"}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "destroy"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// destroy with some parameters, providing a file => no var files included
		{
			mockCmdOptions(t, workingDir, []string{"destroy", "-no-color", "-foo", temporaryFile}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "destroy"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "foo"},
		},

		// Command not included in commands list
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"plan", "destroy"}, []string{"required.tfvars"}, []string{"optional.tfvars"}),
			[]string{},
		},
	}
	for _, testCase := range testCases {
		config := config.TerragruntConfig{
			Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{testCase.extraArgs}},
		}

		out := filterTerraformExtraArgs(testCase.options, &config)

		assert.Equal(t, testCase.expectedArgs, out)
	}

}

var defaultLogLevel = util.GetDefaultLogLevel()

func mockCmdOptions(t *testing.T, workingDir string, terraformCliArgs []string) *options.TerragruntOptions {
	o := mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, terraformCliArgs, true, "", false, false, defaultLogLevel, false)
	return o
}

func mockExtraArgs(arguments, commands, requiredVarFiles, optionalVarFiles []string) config.TerraformExtraArguments {
	a := config.TerraformExtraArguments{
		Name:             "test",
		Arguments:        &arguments,
		Commands:         commands,
		RequiredVarFiles: &requiredVarFiles,
		OptionalVarFiles: &optionalVarFiles,
	}

	return a
}

func mockOptions(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, includeExternalDependencies bool, logLevel logrus.Level, debug bool) *options.TerragruntOptions {
	opts, err := options.NewTerragruntOptionsForTest(terragruntConfigPath)
	if err != nil {
		t.Fatalf("error: %v\n", errors.WithStackTrace(err))
	}

	opts.WorkingDir = workingDir
	opts.TerraformCliArgs = terraformCliArgs
	opts.NonInteractive = nonInteractive
	opts.Source = terragruntSource
	opts.IgnoreDependencyErrors = ignoreDependencyErrors
	opts.IncludeExternalDependencies = includeExternalDependencies
	opts.Logger.Level = logLevel
	opts.Debug = debug

	return opts
}

func createTempFile(t *testing.T) string {
	tmpFile, err := os.CreateTemp("", "")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s\n", err.Error())
	}

	return filepath.ToSlash(tmpFile.Name())
}
