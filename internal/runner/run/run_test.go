package run_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetTerragruntInputsAsEnvVars(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		envVarsInOpts  map[string]string
		inputsInConfig map[string]any
		expected       map[string]string
		description    string
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
			inputsInConfig: map[string]any{"foo": "bar"},
			expected:       map[string]string{"TF_VAR_foo": "bar"},
		},
		{
			description:    "No env vars in opts, a few inputs",
			envVarsInOpts:  nil,
			inputsInConfig: map[string]any{"foo": "bar", "list": []int{1, 2, 3}, "map": map[string]any{"a": "b"}},
			expected:       map[string]string{"TF_VAR_foo": "bar", "TF_VAR_list": "[1,2,3]", "TF_VAR_map": `{"a":"b"}`},
		},
		{
			description:    "A few env vars in opts, a few inputs, no overlap",
			envVarsInOpts:  map[string]string{"foo": "bar", "something": "else"},
			inputsInConfig: map[string]any{"foo": "bar", "list": []int{1, 2, 3}, "map": map[string]any{"a": "b"}},
			expected:       map[string]string{"foo": "bar", "something": "else", "TF_VAR_foo": "bar", "TF_VAR_list": "[1,2,3]", "TF_VAR_map": `{"a":"b"}`},
		},
		{
			description:    "A few env vars in opts, a few inputs, with overlap",
			envVarsInOpts:  map[string]string{"foo": "bar", "TF_VAR_foo": "original", "TF_VAR_list": "original"},
			inputsInConfig: map[string]any{"foo": "bar", "list": []int{1, 2, 3}, "map": map[string]any{"a": "b"}},
			expected:       map[string]string{"foo": "bar", "TF_VAR_foo": "original", "TF_VAR_list": "original", "TF_VAR_map": `{"a":"b"}`},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest("mock-path-for-test.hcl")
			require.NoError(t, err)

			opts.Env = tc.envVarsInOpts

			cfg := &config.TerragruntConfig{Inputs: tc.inputsInConfig}

			l := logger.CreateLogger()
			require.NoError(t, run.SetTerragruntInputsAsEnvVars(l, opts, cfg))

			assert.Equal(t, tc.expected, opts.Env)
		})
	}
}

func TestTerragruntTerraformCodeCheck(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files       map[string]string
		description string
		valid       bool
	}{
		{
			description: "Directory with plain Terraform",
			files: map[string]string{
				"main.tf": `# Terraform file`,
			},
			valid: true,
		},
		{
			description: "Directory with plain OpenTofu",
			files: map[string]string{
				"main.tofu": `# OpenTofu file`,
			},
			valid: true,
		},
		{
			description: "Directory with plain Terraform and OpenTofu",
			files: map[string]string{
				"main.tf":   `# Terraform file`,
				"main.tofu": `# OpenTofu file`,
			},
			valid: true,
		},
		{
			description: "Directory with JSON formatted Terraform",
			files: map[string]string{
				"main.tf.json": `{"terraform": {"backend": {"s3": {}}}}`,
			},
			valid: true,
		},
		{
			description: "Directory with JSON formatted OpenTofu",
			files: map[string]string{
				"main.tofu.json": `{"terraform": {"backend": {"s3": {}}}}`,
			},
			valid: true,
		},
		{
			description: "Directory with JSON formatted Terraform and OpenTofu",
			files: map[string]string{
				"main.tf.json":   `{"terraform": {"backend": {"s3": {}}}}`,
				"main.tofu.json": `{"terraform": {"backend": {"s3": {}}}}`,
			},
			valid: true,
		},
		{
			description: "Directory with no Terraform or OpenTofu",
			files: map[string]string{
				"main.yaml": `# Not a terraform file`,
			},
			valid: false,
		},
		{
			description: "Directory with no files",
			files:       map[string]string{},
			valid:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			for filename, content := range tc.files {
				filePath := filepath.Join(tmpDir, filename)
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
			}

			opts, err := options.NewTerragruntOptionsForTest("mock-path-for-test.hcl")
			require.NoError(t, err)

			opts.WorkingDir = tmpDir

			err = run.CheckFolderContainsTerraformCode(opts)
			if (err != nil) && tc.valid {
				t.Error("valid terraform returned error")
			}

			if (err == nil) && !tc.valid {
				t.Error("invalid terraform did not return error")
			}
		})
	}
}

// Legacy retry tests removed; retries now handled via errors blocks

func TestToTerraformEnvVars(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		vars        map[string]any
		expected    map[string]string
		description string
	}{
		{
			description: "empty",
			vars:        map[string]any{},
			expected:    map[string]string{},
		},
		{
			description: "string value",
			vars:        map[string]any{"foo": "bar"},
			expected:    map[string]string{"TF_VAR_foo": `bar`},
		},
		{
			description: "int value",
			vars:        map[string]any{"foo": 42},
			expected:    map[string]string{"TF_VAR_foo": `42`},
		},
		{
			description: "bool value",
			vars:        map[string]any{"foo": true},
			expected:    map[string]string{"TF_VAR_foo": `true`},
		},
		{
			description: "list value",
			vars:        map[string]any{"foo": []string{"a", "b", "c"}},
			expected:    map[string]string{"TF_VAR_foo": `["a","b","c"]`},
		},
		{
			description: "map value",
			vars:        map[string]any{"foo": map[string]any{"a": "b", "c": "d"}},
			expected:    map[string]string{"TF_VAR_foo": `{"a":"b","c":"d"}`},
		},
		{
			description: "nested map value",
			vars:        map[string]any{"foo": map[string]any{"a": []int{1, 2, 3}, "b": "c", "d": map[string]any{"e": "f"}}},
			expected:    map[string]string{"TF_VAR_foo": `{"a":[1,2,3],"b":"c","d":{"e":"f"}}`},
		},
		{
			description: "multiple values",
			vars:        map[string]any{"str": "bar", "int": 42, "bool": false, "list": []int{1, 2, 3}, "map": map[string]any{"a": "b"}},
			expected:    map[string]string{"TF_VAR_str": `bar`, "TF_VAR_int": `42`, "TF_VAR_bool": `false`, "TF_VAR_list": `[1,2,3]`, "TF_VAR_map": `{"a":"b"}`},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest("")
			require.NoError(t, err)

			l := logger.CreateLogger()
			actual, err := run.ToTerraformEnvVars(l, opts, tc.vars)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestFilterTerraformExtraArgs(t *testing.T) {
	t.Parallel()

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
			[]string{"--foo", "bar", "-var-file=" + temporaryFile},
		},
		// required var file + optional existing var file
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"apply", "plan"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "-var-file=required.tfvars", "-var-file=" + temporaryFile},
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
			[]string{"--foo", "bar", "-var-file=required.tfvars", "-var-file=" + temporaryFile},
		},
		// apply providing a folder, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"apply", workingDir}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "-var='key=value'"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "-var-file=test.tfvars", "-var='key=value'", "-var-file=required.tfvars", "-var-file=" + temporaryFile},
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
			[]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo", "-var-file=required.tfvars", "-var-file=" + temporaryFile},
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
			[]string{"--foo", "-var-file=test.tfvars", "-var='key=value'", "-var-file=required.tfvars", "-var-file=" + temporaryFile},
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
			[]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo", "-var-file=required.tfvars", "-var-file=" + temporaryFile},
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
	for _, tc := range testCases {
		config := config.TerragruntConfig{
			Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{tc.extraArgs}},
		}

		l := logger.CreateLogger()
		out := run.FilterTerraformExtraArgs(l, tc.options, &config)

		assert.Equal(t, tc.expectedArgs, out)
	}
}

var defaultLogLevel = log.DebugLevel

func mockCmdOptions(t *testing.T, workingDir string, terraformCliArgs []string) *options.TerragruntOptions {
	t.Helper()

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

func mockOptions(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, includeExternalDependencies bool, _ log.Level, debug bool) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(terragruntConfigPath)
	if err != nil {
		t.Fatalf("error: %v\n", errors.New(err))
	}

	opts.WorkingDir = workingDir
	opts.TerraformCliArgs = terraformCliArgs
	opts.NonInteractive = nonInteractive
	opts.Source = terragruntSource
	opts.IgnoreDependencyErrors = ignoreDependencyErrors
	opts.IncludeExternalDependencies = includeExternalDependencies
	opts.Debug = debug

	return opts
}

func createTempFile(t *testing.T) string {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s\n", err.Error())
	}

	return filepath.ToSlash(tmpFile.Name())
}

func TestShouldCopyLockFile(t *testing.T) {
	t.Parallel()

	type args struct {
		terraformConfig *config.TerraformConfig
		args            []string
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "init without terraform config",
			args: args{
				args: []string{"init"},
			},
			want: true,
		},
		{
			name: "providers lock without terraform config",
			args: args{
				args: []string{"providers", "lock"},
			},
			want: true,
		},
		{
			name: "providers schema without terraform config",
			args: args{
				args: []string{"providers", "schema"},
			},
			want: false,
		},
		{
			name: "plan without terraform config",
			args: args{
				args: []string{"plan"},
			},
			want: false,
		},
		{
			name: "init with empty terraform config",
			args: args{
				args:            []string{"init"},
				terraformConfig: &config.TerraformConfig{},
			},
			want: true,
		},
		{
			name: "init with CopyTerraformLockFile enabled",
			args: args{
				args: []string{"init"},
				terraformConfig: &config.TerraformConfig{
					CopyTerraformLockFile: &[]bool{true}[0],
				},
			},
			want: true,
		},
		{
			name: "init with CopyTerraformLockFile disabled",
			args: args{
				args: []string{"init"},
				terraformConfig: &config.TerraformConfig{
					CopyTerraformLockFile: &[]bool{false}[0],
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equalf(t, tt.want, run.ShouldCopyLockFile(tt.args.args, tt.args.terraformConfig), "shouldCopyLockFile(%v, %v)", tt.args.args, tt.args.terraformConfig)
		})
	}
}
