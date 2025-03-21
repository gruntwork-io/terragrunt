package config_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestPathRelativeToInclude(t *testing.T) {
	t.Parallel()

	tc := []struct {
		include           map[string]config.IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
		params            []string
	}{
		{
			include:           nil,
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      ".",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../" + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "child",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "child",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../../../" + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "child/sub-child/sub-sub-child",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "child/sub-child/sub-sub-child",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "../child/sub-child",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../../" + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, "../child/sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "child/sub-child",
		},
		{
			include: map[string]config.IncludeConfig{
				"root":  {Path: "../../" + config.DefaultTerragruntConfigPath},
				"child": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath},
			},
			params:            []string{"child"},
			terragruntOptions: terragruntOptionsForTest(t, "../child/sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "../child/sub-child",
		},
	}

	for _, tt := range tc {
		trackInclude := getTrackIncludeFromTestData(tt.include, tt.params)
		ctx := config.NewParsingContext(context.Background(), tt.terragruntOptions).WithTrackInclude(trackInclude)
		actualPath, actualErr := config.PathRelativeToInclude(ctx, tt.params)
		require.NoError(t, actualErr, "For include %v and options %v, unexpected error: %v", tt.include, tt.terragruntOptions, actualErr)
		assert.Equal(t, tt.expectedPath, actualPath, "For include %v and options %v", tt.include, tt.terragruntOptions)
	}
}

func TestPathRelativeFromInclude(t *testing.T) {
	t.Parallel()

	tc := []struct {
		include           map[string]config.IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
		params            []string
	}{
		{
			include:           nil,
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      ".",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../" + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "..",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "..",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../../../" + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "../../..",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "../../..",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "../../other-child",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../../" + config.DefaultTerragruntConfigPath}},
			params:            nil,
			terragruntOptions: terragruntOptionsForTest(t, "../child/sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "../..",
		},
		{
			include: map[string]config.IncludeConfig{
				"root":  {Path: "../../" + config.DefaultTerragruntConfigPath},
				"child": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath},
			},
			params:            []string{"child"},
			terragruntOptions: terragruntOptionsForTest(t, "../child/sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      "../../other-child",
		},
	}

	for _, tt := range tc {
		trackInclude := getTrackIncludeFromTestData(tt.include, tt.params)
		ctx := config.NewParsingContext(context.Background(), tt.terragruntOptions).WithTrackInclude(trackInclude)
		actualPath, actualErr := config.PathRelativeFromInclude(ctx, tt.params)
		require.NoError(t, actualErr, "For include %v and options %v, unexpected error: %v", tt.include, tt.terragruntOptions, actualErr)
		assert.Equal(t, tt.expectedPath, actualPath, "For include %v and options %v", tt.include, tt.terragruntOptions)
	}
}

func TestRunCommand(t *testing.T) {
	t.Parallel()

	homeDir := os.Getenv("HOME")
	tc := []struct {
		expectedErr       error
		terragruntOptions *options.TerragruntOptions
		expectedOutput    string
		params            []string
	}{
		{
			params:            []string{"/bin/bash", "-c", "echo -n foo"},
			terragruntOptions: terragruntOptionsForTest(t, homeDir),
			expectedOutput:    "foo",
		},
		{
			params:            []string{"/bin/bash", "-c", "echo foo"},
			terragruntOptions: terragruntOptionsForTest(t, homeDir),
			expectedOutput:    "foo",
		},
		{
			params:            []string{"--terragrunt-quiet", "/bin/bash", "-c", "echo -n foo"},
			terragruntOptions: terragruntOptionsForTest(t, homeDir),
			expectedOutput:    "foo",
		},
		{
			params:            []string{"--terragrunt-quiet", "/bin/bash", "-c", "echo foo"},
			terragruntOptions: terragruntOptionsForTest(t, homeDir),
			expectedOutput:    "foo",
		},
		{
			params:            []string{"--terragrunt-global-cache", "/bin/bash", "-c", "echo foo"},
			terragruntOptions: terragruntOptionsForTest(t, homeDir),
			expectedOutput:    "foo",
		},
		{
			params:            []string{"--terragrunt-global-cache", "--terragrunt-quiet", "/bin/bash", "-c", "echo foo"},
			terragruntOptions: terragruntOptionsForTest(t, homeDir),
			expectedOutput:    "foo",
		},
		{
			params:            []string{"--terragrunt-quiet", "--terragrunt-global-cache", "/bin/bash", "-c", "echo foo"},
			terragruntOptions: terragruntOptionsForTest(t, homeDir),
			expectedOutput:    "foo",
		},
		{
			terragruntOptions: terragruntOptionsForTest(t, homeDir),
			expectedErr:       config.EmptyStringNotAllowedError("{run_cmd()}"),
		},
	}
	for _, tt := range tc {
		tt := tt

		t.Run(tt.terragruntOptions.TerragruntConfigPath, func(t *testing.T) {
			t.Parallel()

			ctx := config.NewParsingContext(context.Background(), tt.terragruntOptions)
			actualOutput, actualErr := config.RunCommand(ctx, tt.params)
			if tt.expectedErr != nil {
				if assert.Error(t, actualErr) {
					assert.IsType(t, tt.expectedErr, errors.Unwrap(actualErr))
				}
			} else {
				require.NoError(t, actualErr)
				assert.Equal(t, tt.expectedOutput, actualOutput)
			}
		})
	}
}

func absPath(t *testing.T, path string) string {
	t.Helper()

	out, err := filepath.Abs(path)
	require.NoError(t, err)
	return out
}

func TestFindInParentFolders(t *testing.T) {
	t.Parallel()

	tc := []struct {
		expectedErr       error
		terragruntOptions *options.TerragruntOptions
		name              string
		expectedPath      string
		params            []string
	}{
		{
			name:              "simple-lookup",
			params:            []string{"root.hcl"},
			terragruntOptions: terragruntOptionsForTest(t, "../test/fixtures/parent-folders/terragrunt-in-root/child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      absPath(t, "../test/fixtures/parent-folders/terragrunt-in-root/root.hcl"),
		},
		{
			name:              "nested-lookup",
			params:            []string{"root.hcl"},
			terragruntOptions: terragruntOptionsForTest(t, "../test/fixtures/parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      absPath(t, "../test/fixtures/parent-folders/terragrunt-in-root/root.hcl"),
		},
		{
			name:              "lookup-with-max-folders",
			params:            []string{"root.hcl"},
			terragruntOptions: terragruntOptionsForTestWithMaxFolders(t, "../test/fixtures/parent-folders/no-terragrunt-in-root/child/sub-child/"+config.DefaultTerragruntConfigPath, 3),
			expectedErr:       config.ParentFileNotFoundError{},
		},
		{
			name:              "multiple-terragrunt-in-parents",
			params:            []string{"root.hcl"},
			terragruntOptions: terragruntOptionsForTest(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      absPath(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/root.hcl"),
		},
		{
			name:              "multiple-terragrunt-in-parents-under-child",
			params:            []string{"root.hcl"},
			terragruntOptions: terragruntOptionsForTest(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      absPath(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/root.hcl"),
		},
		{
			name:              "multiple-terragrunt-in-parents-under-sub-child",
			params:            []string{"root.hcl"},
			terragruntOptions: terragruntOptionsForTest(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      absPath(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/sub-child/root.hcl"),
		},
		{
			name:              "parent-file-that-isnt-terragrunt",
			params:            []string{"foo.txt"},
			terragruntOptions: terragruntOptionsForTest(t, "../test/fixtures/parent-folders/other-file-names/child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      absPath(t, "../test/fixtures/parent-folders/other-file-names/foo.txt"),
		},
		{
			name:              "parent-file-that-isnt-terragrunt-in-another-subfolder",
			params:            []string{"common/foo.txt"},
			terragruntOptions: terragruntOptionsForTest(t, "../test/fixtures/parent-folders/in-another-subfolder/live/"+config.DefaultTerragruntConfigPath),
			expectedPath:      absPath(t, "../test/fixtures/parent-folders/in-another-subfolder/common/foo.txt"),
		},
		{
			name:              "parent-file-that-isnt-terragrunt-in-another-subfolder-with-params",
			params:            []string{"tfwork"},
			terragruntOptions: terragruntOptionsForTest(t, "../test/fixtures/parent-folders/with-params/tfwork/tg/"+config.DefaultTerragruntConfigPath),
			expectedPath:      absPath(t, "../test/fixtures/parent-folders/with-params/tfwork"),
		},
		{
			name:              "not-found",
			terragruntOptions: terragruntOptionsForTest(t, "/"),
			expectedErr:       config.ParentFileNotFoundError{},
		},
		{
			name:              "not-found-with-path",
			terragruntOptions: terragruntOptionsForTest(t, "/fake/path"),
			expectedErr:       config.ParentFileNotFoundError{},
		},
		{
			name:              "fallback",
			params:            []string{"foo.txt", "fallback.txt"},
			terragruntOptions: terragruntOptionsForTest(t, "/fake/path"),
			expectedPath:      "fallback.txt",
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := config.NewParsingContext(context.Background(), tt.terragruntOptions)
			actualPath, actualErr := config.FindInParentFolders(ctx, tt.params)
			if tt.expectedErr != nil {
				if assert.Error(t, actualErr) {
					assert.IsType(t, tt.expectedErr, errors.Unwrap(actualErr))
				}
			} else {
				require.NoError(t, actualErr)
				assert.Equal(t, tt.expectedPath, actualPath)
			}
		})
	}
}

func TestResolveTerragruntInterpolation(t *testing.T) {
	t.Parallel()

	tc := []struct {
		str               string
		include           *config.IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedOut       string
		expectedErr       string
	}{
		{
			"terraform { source = path_relative_to_include() }",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+config.DefaultTerragruntConfigPath),
			".",
			"",
		},
		{
			"terraform { source = path_relative_to_include() }",
			&config.IncludeConfig{Path: "../" + config.DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, "/root/child/"+config.DefaultTerragruntConfigPath),
			"child",
			"",
		},
		{
			"terraform { source = find_in_parent_folders(\"root.hcl\") }",
			nil,
			terragruntOptionsForTest(t, "../test/fixtures/parent-folders/terragrunt-in-root/child/sub-child/"+config.DefaultTerragruntConfigPath),
			absPath(t, "../test/fixtures/parent-folders/terragrunt-in-root/root.hcl"),
			"",
		},
		{
			"terraform { source = find_in_parent_folders(\"root.hcl\") }",
			nil,
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixtures/parent-folders/terragrunt-in-root/child/sub-child/"+config.DefaultTerragruntConfigPath, 1),
			"",
			"ParentFileNotFoundError",
		},
		{
			"terraform { source = find_in_parent_folders(\"root.hcl\") }",
			nil,
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixtures/parent-folders/no-terragrunt-in-root/child/sub-child/"+config.DefaultTerragruntConfigPath, 3),
			"",
			"ParentFileNotFoundError",
		},
	}

	for _, tt := range tc {
		// The following is necessary to make sure tt's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		tt := tt

		t.Run(fmt.Sprintf("%s--%s", tt.str, tt.terragruntOptions.TerragruntConfigPath), func(t *testing.T) {
			t.Parallel()

			ctx := config.NewParsingContext(context.Background(), tt.terragruntOptions)
			actualOut, actualErr := config.ParseConfigString(ctx, "mock-path-for-test.hcl", tt.str, tt.include)
			if tt.expectedErr != "" {
				require.Error(t, actualErr)
				assert.Contains(t, actualErr.Error(), tt.expectedErr)
			} else {
				require.NoError(t, actualErr)
				assert.NotNil(t, actualOut)
				assert.NotNil(t, actualOut.Terraform)
				assert.NotNil(t, actualOut.Terraform.Source)
				assert.Equal(t, tt.expectedOut, *actualOut.Terraform.Source)
			}
		})
	}
}

func TestResolveEnvInterpolationConfigString(t *testing.T) {
	t.Parallel()

	tc := []struct {
		str               string
		include           *config.IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedOut       string
		expectedErr       string
	}{
		{
			`iam_role = "foo/${get_env()}/bar"`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+config.DefaultTerragruntConfigPath),
			"",
			"InvalidGetEnvParamsError",
		},
		{
			`iam_role = "foo/${get_env("","")}/bar"`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+config.DefaultTerragruntConfigPath),
			"",
			"InvalidEnvParamNameError",
		},
		{
			`iam_role = get_env()`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+config.DefaultTerragruntConfigPath),
			"",
			"InvalidGetEnvParamsError",
		},
		{
			`iam_role = get_env("TEST_VAR_1", "TEST_VAR_2", "TEST_VAR_3")`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+config.DefaultTerragruntConfigPath),
			"",
			"InvalidGetEnvParamsError",
		},
		{
			`iam_role = get_env("TEST_ENV_TERRAGRUNT_VAR")`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+config.DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_VAR": "SOMETHING"}),
			"SOMETHING",
			"",
		},
		{
			`iam_role = get_env("SOME_VAR", "SOME_VALUE")`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+config.DefaultTerragruntConfigPath),
			"SOME_VALUE",
			"",
		},
		{
			`iam_role = "foo/${get_env("TEST_ENV_TERRAGRUNT_HIT","")}/bar"`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+config.DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"}),
			"foo//bar",
			"",
		},
		{
			`iam_role = "foo/${get_env("TEST_ENV_TERRAGRUNT_HIT","DEFAULT")}/bar"`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+config.DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"}),
			"foo/DEFAULT/bar",
			"",
		},
		{
			`iam_role = "foo/${get_env("TEST_ENV_TERRAGRUNT_VAR")}/bar"`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+config.DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_VAR": "SOMETHING"}),
			"foo/SOMETHING/bar",
			"",
		},
	}

	for _, tt := range tc {
		// The following is necessary to make sure tt's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		tt := tt
		t.Run(tt.str, func(t *testing.T) {
			t.Parallel()

			ctx := config.NewParsingContext(context.Background(), tt.terragruntOptions)
			actualOut, actualErr := config.ParseConfigString(ctx, "mock-path-for-test.hcl", tt.str, tt.include)
			if tt.expectedErr != "" {
				require.Error(t, actualErr)
				assert.Contains(t, actualErr.Error(), tt.expectedErr)
			} else {
				require.NoError(t, actualErr)
				assert.Equal(t, tt.expectedOut, actualOut.IamRole)
			}
		})
	}
}

func TestResolveCommandsInterpolationConfigString(t *testing.T) {
	t.Parallel()

	tc := []struct {
		str               string
		include           *config.IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedFooInput  []string
	}{
		{
			"inputs = { foo = get_terraform_commands_that_need_locking() }",
			nil,
			terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath),
			config.TerraformCommandsNeedLocking,
		},
		{
			`inputs = { foo = get_terraform_commands_that_need_vars() }`,
			nil,
			terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath),
			config.TerraformCommandsNeedVars,
		},
		{
			"inputs = { foo = get_terraform_commands_that_need_parallelism() }",
			nil,
			terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath),
			config.TerraformCommandsNeedParallelism,
		},
	}

	for _, tt := range tc {
		// The following is necessary to make sure tt's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		tt := tt

		t.Run(tt.str, func(t *testing.T) {
			t.Parallel()

			ctx := config.NewParsingContext(context.Background(), tt.terragruntOptions)
			actualOut, actualErr := config.ParseConfigString(ctx, "mock-path-for-test.hcl", tt.str, tt.include)
			require.NoError(t, actualErr, "For string '%s' include %v and options %v, unexpected error: %v", tt.str, tt.include, tt.terragruntOptions, actualErr)

			assert.NotNil(t, actualOut)

			inputs := actualOut.Inputs
			assert.NotNil(t, inputs)

			foo, containsFoo := inputs["foo"]
			assert.True(t, containsFoo)

			fooSlice := toStringSlice(t, foo)

			assert.EqualValues(t, tt.expectedFooInput, fooSlice, "For string '%s' include %v and options %v", tt.str, tt.include, tt.terragruntOptions)
		})
	}
}

func TestResolveCliArgsInterpolationConfigString(t *testing.T) {
	t.Parallel()

	for _, cliArgs := range [][]string{nil, {}, {"apply"}, {"plan", "-out=planfile"}} {
		opts := terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath)
		opts.TerraformCliArgs = cliArgs
		expectedFooInput := cliArgs
		// Expecting nil to be returned for get_terraform_cli_args() call for
		// either nil or empty array of input args
		if len(cliArgs) == 0 {
			expectedFooInput = nil
		}
		tt := struct {
			str               string
			include           *config.IncludeConfig
			terragruntOptions *options.TerragruntOptions
			expectedFooInput  []string
		}{
			"inputs = { foo = get_terraform_cli_args() }",
			nil,
			opts,
			expectedFooInput,
		}
		t.Run(tt.str, func(t *testing.T) {
			t.Parallel()

			ctx := config.NewParsingContext(context.Background(), tt.terragruntOptions)
			actualOut, actualErr := config.ParseConfigString(ctx, "mock-path-for-test.hcl", tt.str, tt.include)
			require.NoError(t, actualErr, "For string '%s' include %v and options %v, unexpected error: %v", tt.str, tt.include, tt.terragruntOptions, actualErr)

			assert.NotNil(t, actualOut)

			inputs := actualOut.Inputs
			assert.NotNil(t, inputs)

			foo, containsFoo := inputs["foo"]
			assert.True(t, containsFoo)

			fooSlice := toStringSlice(t, foo)
			assert.EqualValues(t, tt.expectedFooInput, fooSlice, "For string '%s' include %v and options %v", tt.str, tt.include, tt.terragruntOptions)
		})
	}
}

func toStringSlice(t *testing.T, value any) []string {
	t.Helper()

	if value == nil {
		return nil
	}

	asInterfaceSlice, isInterfaceSlice := value.([]any)
	assert.True(t, isInterfaceSlice)

	// TODO: See if this logic is desired
	if len(asInterfaceSlice) == 0 {
		return nil
	}

	var out = make([]string, 0, len(asInterfaceSlice))
	for _, item := range asInterfaceSlice {
		asStr, isStr := item.(string)
		assert.True(t, isStr)
		out = append(out, asStr)
	}

	return out
}

func TestGetTerragruntDirAbsPath(t *testing.T) {
	t.Parallel()
	workingDir, err := os.Getwd()
	require.NoError(t, err, "Could not get current working dir: %v", err)
	testGetTerragruntDir(t, "/foo/bar/terragrunt.hcl", filepath.VolumeName(workingDir)+"/foo/bar")
}

func TestGetTerragruntDirRelPath(t *testing.T) {
	t.Parallel()
	workingDir, err := os.Getwd()
	require.NoError(t, err, "Could not get current working dir: %v", err)
	workingDir = filepath.ToSlash(workingDir)

	testGetTerragruntDir(t, "foo/bar/terragrunt.hcl", workingDir+"/foo/bar")
}

func testGetTerragruntDir(t *testing.T, configPath string, expectedPath string) {
	t.Helper()

	terragruntOptions, err := options.NewTerragruntOptionsForTest(configPath)
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	ctx := config.NewParsingContext(context.Background(), terragruntOptions)
	actualPath, err := config.GetTerragruntDir(ctx)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expectedPath, actualPath)
}

func terragruntOptionsForTest(t *testing.T, configPath string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(configPath)
	if err != nil {
		t.Fatalf("Failed to create TerragruntOptions: %v", err)
	}
	return opts
}

func terragruntOptionsForTestWithMaxFolders(t *testing.T, configPath string, maxFoldersToCheck int) *options.TerragruntOptions {
	t.Helper()

	opts := terragruntOptionsForTest(t, configPath)
	opts.MaxFoldersToCheck = maxFoldersToCheck
	return opts
}

func terragruntOptionsForTestWithEnv(t *testing.T, configPath string, env map[string]string) *options.TerragruntOptions {
	t.Helper()

	opts := terragruntOptionsForTest(t, configPath)
	opts.Env = env
	return opts
}

func TestGetParentTerragruntDir(t *testing.T) {
	t.Parallel()

	currentDir, err := os.Getwd()
	require.NoError(t, err, "Could not get current working dir: %v", err)
	parentDir := filepath.ToSlash(filepath.Dir(currentDir))

	tc := []struct {
		include           map[string]config.IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
		params            []string
	}{
		{
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      helpers.RootFolder + "child",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../" + config.DefaultTerragruntConfigPath}},
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      helpers.RootFolder,
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      helpers.RootFolder,
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../../../" + config.DefaultTerragruntConfigPath}},
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      helpers.RootFolder,
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      helpers.RootFolder,
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath}},
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      filepath.VolumeName(parentDir) + "/other-child",
		},
		{
			include:           map[string]config.IncludeConfig{"": {Path: "../../" + config.DefaultTerragruntConfigPath}},
			terragruntOptions: terragruntOptionsForTest(t, "../child/sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      parentDir,
		},
		{
			include: map[string]config.IncludeConfig{
				"root":  {Path: "../../" + config.DefaultTerragruntConfigPath},
				"child": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath},
			},
			params:            []string{"child"},
			terragruntOptions: terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+config.DefaultTerragruntConfigPath),
			expectedPath:      filepath.VolumeName(parentDir) + "/other-child",
		},
	}

	for _, tt := range tc {
		trackInclude := getTrackIncludeFromTestData(tt.include, tt.params)
		ctx := config.NewParsingContext(context.Background(), tt.terragruntOptions).WithTrackInclude(trackInclude)
		actualPath, actualErr := config.GetParentTerragruntDir(ctx, tt.params)
		require.NoError(t, actualErr, "For include %v and options %v, unexpected error: %v", tt.include, tt.terragruntOptions, actualErr)
		assert.Equal(t, tt.expectedPath, actualPath, "For include %v and options %v", tt.include, tt.terragruntOptions)
	}
}

func TestTerraformBuiltInFunctions(t *testing.T) {
	t.Parallel()

	tc := []struct {
		expected any
		input    string
	}{
		{
			input:    "abs(-1)",
			expected: 1.,
		},
		{
			input:    `element(["one", "two", "three"], 1)`,
			expected: "two",
		},
		{
			input:    `chomp(file("other-file.txt"))`,
			expected: "This is a test file",
		},
		{
			input:    `sha1("input")`,
			expected: "140f86aae51ab9e1cda9b4254fe98a74eb54c1a1",
		},
		{
			input:    `split("|", "one|two|three")`,
			expected: []any{"one", "two", "three"},
		},
		{
			input:    `!tobool("false")`,
			expected: true,
		},
		{
			input:    `trimspace("     content     ")`,
			expected: "content",
		},
		{
			input:    `zipmap(["one", "two", "three"], [1, 2, 3])`,
			expected: map[string]any{"one": 1., "two": 2., "three": 3.},
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			terragruntOptions := terragruntOptionsForTest(t, "../test/fixtures/config-terraform-functions/"+config.DefaultTerragruntConfigPath)
			configString := fmt.Sprintf("inputs = { test = %s }", tt.input)
			ctx := config.NewParsingContext(context.Background(), terragruntOptions)
			actual, err := config.ParseConfigString(ctx, terragruntOptions.TerragruntConfigPath, configString, nil)
			require.NoError(t, err, "For hcl '%s' include %v and options %v, unexpected error: %v", tt.input, nil, terragruntOptions, err)

			assert.NotNil(t, actual)

			inputs := actual.Inputs
			assert.NotNil(t, inputs)

			test, containsTest := inputs["test"]
			assert.True(t, containsTest)

			assert.EqualValues(t, tt.expected, test, "For hcl '%s' include %v and options %v", tt.input, nil, terragruntOptions)
		})
	}
}

func TestTerraformOutputJsonToCtyValueMap(t *testing.T) {
	t.Parallel()

	tc := []struct {
		expected map[string]cty.Value
		input    string
	}{
		{
			input:    `{"bool": {"sensitive": false, "type": "bool", "value": true}}`,
			expected: map[string]cty.Value{"bool": cty.True},
		},
		{
			input:    `{"number": {"sensitive": false, "type": "number", "value": 42}}`,
			expected: map[string]cty.Value{"number": cty.NumberIntVal(42)},
		},
		{
			input:    `{"list_string": {"sensitive": false, "type": ["list", "string"], "value": ["4", "2"]}}`,
			expected: map[string]cty.Value{"list_string": cty.ListVal([]cty.Value{cty.StringVal("4"), cty.StringVal("2")})},
		},
		{
			input:    `{"map_string": {"sensitive": false, "type": ["map", "string"], "value": {"x": "foo", "y": "bar"}}}`,
			expected: map[string]cty.Value{"map_string": cty.MapVal(map[string]cty.Value{"x": cty.StringVal("foo"), "y": cty.StringVal("bar")})},
		},
		{
			input: `{"map_list_number": {"sensitive": false, "type": ["map", ["list", "number"]], "value": {"x": [4, 2]}}}`,
			expected: map[string]cty.Value{
				"map_list_number": cty.MapVal(
					map[string]cty.Value{
						"x": cty.ListVal([]cty.Value{cty.NumberIntVal(4), cty.NumberIntVal(2)}),
					},
				),
			},
		},
		{
			input: `{"object": {"sensitive": false, "type": ["object", {"x": "number", "y": "string", "lst": ["list", "string"]}], "value": {"x": 42, "y": "the truth", "lst": ["foo", "bar"]}}}`,
			expected: map[string]cty.Value{
				"object": cty.ObjectVal(
					map[string]cty.Value{
						"x":   cty.NumberIntVal(42),
						"y":   cty.StringVal("the truth"),
						"lst": cty.ListVal([]cty.Value{cty.StringVal("foo"), cty.StringVal("bar")}),
					},
				),
			},
		},
		{
			input: `{"out1": {"sensitive": false, "type": "number", "value": 42}, "out2": {"sensitive": false, "type": "string", "value": "foo bar"}}`,
			expected: map[string]cty.Value{
				"out1": cty.NumberIntVal(42),
				"out2": cty.StringVal("foo bar"),
			},
		},
	}

	mockTargetConfig := config.DefaultTerragruntConfigPath
	for _, tt := range tc {
		converted, err := config.TerraformOutputJSONToCtyValueMap(mockTargetConfig, []byte(tt.input))
		require.NoError(t, err)
		assert.Equal(t, getKeys(converted), getKeys(tt.expected))
		for k, v := range converted {
			assert.True(t, v.Equals(tt.expected[k]).True())
		}
	}
}

func TestReadTerragruntConfigInputs(t *testing.T) {
	t.Parallel()

	options := terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath)

	ctx := config.NewParsingContext(context.Background(), options)
	tgConfigCty, err := config.ParseTerragruntConfig(ctx, "../test/fixtures/inputs/terragrunt.hcl", nil)
	require.NoError(t, err)

	tgConfigMap, err := config.ParseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)

	inputsMap := tgConfigMap["inputs"].(map[string]any)

	assert.Equal(t, "string", inputsMap["string"].(string))
	assert.InEpsilon(t, float64(42), inputsMap["number"].(float64), 0.0000000001)
	assert.True(t, inputsMap["bool"].(bool))
	assert.Equal(t, []any{"a", "b", "c"}, inputsMap["list_string"].([]any))
	assert.Equal(t, []any{float64(1), float64(2), float64(3)}, inputsMap["list_number"].([]any))
	assert.Equal(t, []any{true, false}, inputsMap["list_bool"].([]any))
	assert.Equal(t, map[string]any{"foo": "bar"}, inputsMap["map_string"].(map[string]any))
	assert.Equal(t, map[string]any{"foo": float64(42), "bar": float64(12345)}, inputsMap["map_number"].(map[string]any))
	assert.Equal(t, map[string]any{"foo": true, "bar": false, "baz": true}, inputsMap["map_bool"].(map[string]any))

	assert.Equal(
		t,
		map[string]any{
			"str":  "string",
			"num":  float64(42),
			"list": []any{float64(1), float64(2), float64(3)},
			"map":  map[string]any{"foo": "bar"},
		},
		inputsMap["object"].(map[string]any),
	)

	assert.Equal(t, "default", inputsMap["from_env"].(string))
}

func TestReadTerragruntConfigRemoteState(t *testing.T) {
	t.Parallel()

	options := terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath)
	ctx := config.NewParsingContext(context.Background(), options)
	tgConfigCty, err := config.ParseTerragruntConfig(ctx, "../test/fixtures/terragrunt/terragrunt.hcl", nil)
	require.NoError(t, err)

	tgConfigMap, err := config.ParseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)

	remoteStateMap := tgConfigMap["remote_state"].(map[string]any)
	assert.Equal(t, "s3", remoteStateMap["backend"].(string))
	configMap := remoteStateMap["config"].(map[string]any)
	assert.True(t, configMap["encrypt"].(bool))
	assert.Equal(t, "terraform.tfstate", configMap["key"].(string))
	assert.Equal(
		t,
		map[string]any{"owner": "terragrunt integration test", "name": "Terraform state storage"},
		configMap["s3_bucket_tags"].(map[string]any),
	)
	assert.Equal(
		t,
		map[string]any{"owner": "terragrunt integration test", "name": "Terraform lock table"},
		configMap["dynamodb_table_tags"].(map[string]any),
	)
	assert.Equal(
		t,
		map[string]any{"owner": "terragrunt integration test", "name": "Terraform access log storage"},
		configMap["accesslogging_bucket_tags"].(map[string]any),
	)
}

func TestReadTerragruntConfigHooks(t *testing.T) {
	t.Parallel()

	options := terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath)
	ctx := config.NewParsingContext(context.Background(), options)
	tgConfigCty, err := config.ParseTerragruntConfig(ctx, "../test/fixtures/hooks/before-after-and-on-error/terragrunt.hcl", nil)
	require.NoError(t, err)

	tgConfigMap, err := config.ParseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)

	terraformMap := tgConfigMap["terraform"].(map[string]any)
	beforeHooksMap := terraformMap["before_hook"].(map[string]any)
	assert.Equal(
		t,
		[]any{"touch", "before.out"},
		beforeHooksMap["before_hook_1"].(map[string]any)["execute"].([]any),
	)
	assert.Equal(
		t,
		[]any{"echo", "BEFORE_TERRAGRUNT_READ_CONFIG"},
		beforeHooksMap["before_hook_2"].(map[string]any)["execute"].([]any),
	)

	afterHooksMap := terraformMap["after_hook"].(map[string]any)
	assert.Equal(
		t,
		[]any{"touch", "after.out"},
		afterHooksMap["after_hook_1"].(map[string]any)["execute"].([]any),
	)
	assert.Equal(
		t,
		[]any{"echo", "AFTER_TERRAGRUNT_READ_CONFIG"},
		afterHooksMap["after_hook_2"].(map[string]any)["execute"].([]any),
	)
	errorHooksMap := terraformMap["error_hook"].(map[string]any)
	assert.Equal(
		t,
		[]any{"echo", "ON_APPLY_ERROR"},
		errorHooksMap["error_hook_1"].(map[string]any)["execute"].([]any),
	)
}

func TestReadTerragruntConfigLocals(t *testing.T) {
	t.Parallel()

	options := terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath)
	ctx := config.NewParsingContext(context.Background(), options)
	tgConfigCty, err := config.ParseTerragruntConfig(ctx, "../test/fixtures/locals/canonical/terragrunt.hcl", nil)
	require.NoError(t, err)

	tgConfigMap, err := config.ParseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)

	localsMap := tgConfigMap["locals"].(map[string]any)
	assert.InEpsilon(t, float64(2), localsMap["x"].(float64), 0.0000000001)
	assert.Equal(t, "Hello world\n", localsMap["file_contents"].(string))
	assert.InEpsilon(t, float64(42), localsMap["number_expression"].(float64), 0.0000000001)
}

func TestGetTerragruntSourceForModuleHappyPath(t *testing.T) {
	t.Parallel()

	tc := []struct {
		config   *config.TerragruntConfig
		source   string
		expected string
	}{
		{mockConfigWithSource(""), "", ""},
		{mockConfigWithSource(""), "/source/modules", ""},
		{mockConfigWithSource("git::git@github.com:acme/modules.git//foo/bar"), "/source/modules", "/source/modules//foo/bar"},
		{mockConfigWithSource("git::git@github.com:acme/modules.git//foo/bar?ref=v0.0.1"), "/source/modules", "/source/modules//foo/bar"},
		{mockConfigWithSource("git::git@github.com:acme/emr_cluster.git?ref=feature/fix_bugs"), "/source/modules", "/source/modules//emr_cluster"},
		{mockConfigWithSource("git::ssh://git@ghe.ourcorp.com/OurOrg/some-module.git"), "/source/modules", "/source/modules//some-module"},
		{mockConfigWithSource("github.com/hashicorp/example"), "/source/modules", "/source/modules//example"},
		{mockConfigWithSource("github.com/hashicorp/example//subdir"), "/source/modules", "/source/modules//subdir"},
		{mockConfigWithSource("git@github.com:hashicorp/example.git//subdir"), "/source/modules", "/source/modules//subdir"},
		{mockConfigWithSource("./some/path//to/modulename"), "/source/modules", "/source/modules//to/modulename"},
	}

	for _, tt := range tc {
		// The following is necessary to make sure tt's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		tt := tt
		t.Run(fmt.Sprintf("%v-%s", *tt.config.Terraform.Source, tt.source), func(t *testing.T) {
			t.Parallel()

			actual, err := config.GetTerragruntSourceForModule(tt.source, "mock-for-test", tt.config)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestStartsWith(t *testing.T) {
	t.Parallel()

	tc := []struct {
		config *options.TerragruntOptions
		args   []string
		value  bool
	}{
		{terragruntOptionsForTest(t, ""), []string{"hello world", "hello"}, true},
		{terragruntOptionsForTest(t, ""), []string{"hello world", "world"}, false},
		{terragruntOptionsForTest(t, ""), []string{"hello world", ""}, true},
		{terragruntOptionsForTest(t, ""), []string{"hello world", " "}, false},
		{terragruntOptionsForTest(t, ""), []string{"", ""}, true},
		{terragruntOptionsForTest(t, ""), []string{"", " "}, false},
		{terragruntOptionsForTest(t, ""), []string{" ", ""}, true},
		{terragruntOptionsForTest(t, ""), []string{"", "hello"}, false},
		{terragruntOptionsForTest(t, ""), []string{" ", "hello"}, false},
	}

	for id, tt := range tc {
		tt := tt
		t.Run(fmt.Sprintf("%v %v", id, tt.args), func(t *testing.T) {
			t.Parallel()

			ctx := config.NewParsingContext(context.Background(), tt.config)
			actual, err := config.StartsWith(ctx, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.value, actual)
		})
	}
}

func TestEndsWith(t *testing.T) {
	t.Parallel()

	tc := []struct {
		config *options.TerragruntOptions
		args   []string
		value  bool
	}{
		{terragruntOptionsForTest(t, ""), []string{"hello world", "world"}, true},
		{terragruntOptionsForTest(t, ""), []string{"hello world", "hello"}, false},
		{terragruntOptionsForTest(t, ""), []string{"hello world", ""}, true},
		{terragruntOptionsForTest(t, ""), []string{"hello world", " "}, false},
		{terragruntOptionsForTest(t, ""), []string{"", ""}, true},
		{terragruntOptionsForTest(t, ""), []string{"", " "}, false},
		{terragruntOptionsForTest(t, ""), []string{" ", ""}, true},
		{terragruntOptionsForTest(t, ""), []string{"", "hello"}, false},
		{terragruntOptionsForTest(t, ""), []string{" ", "hello"}, false},
	}

	for id, tt := range tc {
		tt := tt

		t.Run(fmt.Sprintf("%v %v", id, tt.args), func(t *testing.T) {
			t.Parallel()

			ctx := config.NewParsingContext(context.Background(), tt.config)
			actual, err := config.EndsWith(ctx, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.value, actual)
		})
	}
}

func TestTimeCmp(t *testing.T) {
	t.Parallel()

	tc := []struct {
		config *options.TerragruntOptions
		err    string
		args   []string
		value  int64
	}{
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"2017-11-22T00:00:00Z", "2017-11-22T00:00:00Z"},
			value:  0,
			err:    "",
		},
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"2017-11-22T00:00:00Z", "2017-11-22T01:00:00+01:00"},
			value:  0,
			err:    "",
		},
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"2017-11-22T00:00:01Z", "2017-11-22T01:00:00+01:00"},
			value:  1,
			err:    "",
		},
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"2017-11-22T01:00:00Z", "2017-11-22T00:59:00-01:00"},
			value:  -1,
			err:    "",
		},
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"2017-11-22T01:00:00+01:00", "2017-11-22T01:00:00-01:00"},
			value:  -1,
			err:    "",
		},
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"2017-11-22T01:00:00-01:00", "2017-11-22T01:00:00+01:00"},
			value:  1,
			err:    "",
		},
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"2017-11-22T00:00:00Z", "bloop"},
			value:  0,
			err:    `could not parse second parameter "bloop": not a valid RFC3339 timestamp: cannot use "bloop" as year`,
		},
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"2017-11-22 00:00:00Z", "2017-11-22T00:00:00Z"},
			value:  0,
			err:    `could not parse first parameter "2017-11-22 00:00:00Z": not a valid RFC3339 timestamp: missing required time introducer 'T'`,
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(fmt.Sprintf("TimeCmp(%#v, %#v)", tt.args[0], tt.args[1]), func(t *testing.T) {
			t.Parallel()

			ctx := config.NewParsingContext(context.Background(), tt.config)
			actual, err := config.TimeCmp(ctx, tt.args)
			if tt.err != "" {
				require.EqualError(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.value, actual)
		})
	}
}

func TestStrContains(t *testing.T) {
	t.Parallel()

	tc := []struct {
		config *options.TerragruntOptions
		err    string
		args   []string
		value  bool
	}{
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"hello world", "hello"},
			value:  true,
			err:    "",
		},
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"hello world", "world"},
			value:  true,
			err:    "",
		},
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"hello world0", "0"},
			value:  true,
			err:    "",
		},
		{
			config: terragruntOptionsForTest(t, ""),
			args:   []string{"hello world", "test"},
			value:  false,
			err:    "",
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(fmt.Sprintf("StrContains %v", tt.args), func(t *testing.T) {
			t.Parallel()

			ctx := config.NewParsingContext(context.Background(), tt.config)
			actual, err := config.StrContains(ctx, tt.args)
			if tt.err != "" {
				require.EqualError(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.value, actual)
		})
	}
}

func TestReadTFVarsFiles(t *testing.T) {
	t.Parallel()

	options := terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath)
	ctx := config.NewParsingContext(context.Background(), options)
	tgConfigCty, err := config.ParseTerragruntConfig(ctx, "../test/fixtures/read-tf-vars/terragrunt.hcl", nil)
	require.NoError(t, err)

	tgConfigMap, err := config.ParseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)

	locals := tgConfigMap["locals"].(map[string]any)

	assert.Equal(t, "string", locals["string_var"].(string))
	assert.InEpsilon(t, float64(42), locals["number_var"].(float64), 0.0000000001)
	assert.True(t, locals["bool_var"].(bool))
	assert.Equal(t, []any{"hello", "world"}, locals["list_var"].([]any))

	assert.InEpsilon(t, float64(24), locals["json_number_var"].(float64), 0.0000000001)
	assert.Equal(t, "another string", locals["json_string_var"].(string))
	assert.False(t, locals["json_bool_var"].(bool))
}

func mockConfigWithSource(sourceURL string) *config.TerragruntConfig {
	cfg := config.TerragruntConfig{IsPartial: true}
	cfg.Terraform = &config.TerraformConfig{Source: &sourceURL}
	return &cfg
}

// Return keys as a map so it is treated like a set, and order doesn't matter when comparing equivalence
func getKeys(valueMap map[string]cty.Value) map[string]bool {
	keys := map[string]bool{}
	for k := range valueMap {
		keys[k] = true
	}
	return keys
}

func getTrackIncludeFromTestData(includeMap map[string]config.IncludeConfig, params []string) *config.TrackInclude {
	if len(includeMap) == 0 {
		return nil
	}
	currentList := make([]config.IncludeConfig, len(includeMap))
	i := 0
	for _, val := range includeMap {
		currentList[i] = val
		i++
	}
	trackInclude := &config.TrackInclude{
		CurrentList: currentList,
		CurrentMap:  includeMap,
	}
	if len(params) == 0 {
		trackInclude.Original = &currentList[0]
	}
	return trackInclude
}
