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
		params            []string
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			".",
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			"child",
		},
		{
			map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			"child",
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../../../" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			"child/sub-child/sub-sub-child",
		},
		{
			map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			"child/sub-child/sub-sub-child",
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+config.DefaultTerragruntConfigPath),
			"../child/sub-child",
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../../" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, "../child/sub-child/"+config.DefaultTerragruntConfigPath),
			"child/sub-child",
		},
		{
			map[string]config.IncludeConfig{
				"root":  {Path: "../../" + config.DefaultTerragruntConfigPath},
				"child": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath},
			},
			[]string{"child"},
			terragruntOptionsForTest(t, "../child/sub-child/"+config.DefaultTerragruntConfigPath),
			"../child/sub-child",
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
		params            []string
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			".",
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			"..",
		},
		{
			map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			"..",
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../../../" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			"../../..",
		},
		{
			map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			"../../..",
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+config.DefaultTerragruntConfigPath),
			"../../other-child",
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../../" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, "../child/sub-child/"+config.DefaultTerragruntConfigPath),
			"../..",
		},
		{
			map[string]config.IncludeConfig{
				"root":  {Path: "../../" + config.DefaultTerragruntConfigPath},
				"child": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath},
			},
			[]string{"child"},
			terragruntOptionsForTest(t, "../child/sub-child/"+config.DefaultTerragruntConfigPath),
			"../../other-child",
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
		params            []string
		terragruntOptions *options.TerragruntOptions
		expectedOutput    string
		expectedErr       error
	}{
		{
			[]string{"/bin/bash", "-c", "echo -n foo"},
			terragruntOptionsForTest(t, homeDir),
			"foo",
			nil,
		},
		{
			[]string{"/bin/bash", "-c", "echo foo"},
			terragruntOptionsForTest(t, homeDir),
			"foo",
			nil,
		},
		{
			[]string{"--terragrunt-quiet", "/bin/bash", "-c", "echo -n foo"},
			terragruntOptionsForTest(t, homeDir),
			"foo",
			nil,
		},
		{
			[]string{"--terragrunt-quiet", "/bin/bash", "-c", "echo foo"},
			terragruntOptionsForTest(t, homeDir),
			"foo",
			nil,
		},
		{
			[]string{"--terragrunt-global-cache", "/bin/bash", "-c", "echo foo"},
			terragruntOptionsForTest(t, homeDir),
			"foo",
			nil,
		},
		{
			[]string{"--terragrunt-global-cache", "--terragrunt-quiet", "/bin/bash", "-c", "echo foo"},
			terragruntOptionsForTest(t, homeDir),
			"foo",
			nil,
		},
		{
			[]string{"--terragrunt-quiet", "--terragrunt-global-cache", "/bin/bash", "-c", "echo foo"},
			terragruntOptionsForTest(t, homeDir),
			"foo",
			nil,
		},
		{
			nil,
			terragruntOptionsForTest(t, homeDir),
			"",
			config.EmptyStringNotAllowedError("{run_cmd()}"),
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
		name              string
		params            []string
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
		expectedErr       error
	}{
		{
			"simple-lookup",
			[]string{"root.hcl"},
			terragruntOptionsForTest(t, "../test/fixtures/parent-folders/terragrunt-in-root/child/"+config.DefaultTerragruntConfigPath),
			absPath(t, "../test/fixtures/parent-folders/terragrunt-in-root/root.hcl"),
			nil,
		},
		{
			"nested-lookup",
			[]string{"root.hcl"},
			terragruntOptionsForTest(t, "../test/fixtures/parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			absPath(t, "../test/fixtures/parent-folders/terragrunt-in-root/root.hcl"),
			nil,
		},
		{
			"lookup-with-max-folders",
			[]string{"root.hcl"},
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixtures/parent-folders/no-terragrunt-in-root/child/sub-child/"+config.DefaultTerragruntConfigPath, 3),
			"",
			config.ParentFileNotFoundError{},
		},
		{
			"multiple-terragrunt-in-parents",
			[]string{"root.hcl"},
			terragruntOptionsForTest(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/"+config.DefaultTerragruntConfigPath),
			absPath(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/root.hcl"),
			nil,
		},
		{
			"multiple-terragrunt-in-parents-under-child",
			[]string{"root.hcl"},
			terragruntOptionsForTest(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+config.DefaultTerragruntConfigPath),
			absPath(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/root.hcl"),
			nil,
		},
		{
			"multiple-terragrunt-in-parents-under-sub-child",

			[]string{"root.hcl"},
			terragruntOptionsForTest(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			absPath(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/sub-child/root.hcl"),
			nil,
		},
		{
			"parent-file-that-isnt-terragrunt",
			[]string{"foo.txt"},
			terragruntOptionsForTest(t, "../test/fixtures/parent-folders/other-file-names/child/"+config.DefaultTerragruntConfigPath),
			absPath(t, "../test/fixtures/parent-folders/other-file-names/foo.txt"),
			nil,
		},
		{
			"parent-file-that-isnt-terragrunt-in-another-subfolder",
			[]string{"common/foo.txt"},
			terragruntOptionsForTest(t, "../test/fixtures/parent-folders/in-another-subfolder/live/"+config.DefaultTerragruntConfigPath),
			absPath(t, "../test/fixtures/parent-folders/in-another-subfolder/common/foo.txt"),
			nil,
		},
		{
			"parent-file-that-isnt-terragrunt-in-another-subfolder-with-params",
			[]string{"tfwork"},
			terragruntOptionsForTest(t, "../test/fixtures/parent-folders/with-params/tfwork/tg/"+config.DefaultTerragruntConfigPath),
			absPath(t, "../test/fixtures/parent-folders/with-params/tfwork"),
			nil,
		},
		{
			"not-found",
			nil,
			terragruntOptionsForTest(t, "/"),
			"",
			config.ParentFileNotFoundError{},
		},
		{
			"not-found-with-path",
			nil,
			terragruntOptionsForTest(t, "/fake/path"),
			"",
			config.ParentFileNotFoundError{},
		},
		{
			"fallback",
			[]string{"foo.txt", "fallback.txt"},
			terragruntOptionsForTest(t, "/fake/path"),
			"fallback.txt",
			nil,
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

func toStringSlice(t *testing.T, value interface{}) []string {
	t.Helper()

	if value == nil {
		return nil
	}

	asInterfaceSlice, isInterfaceSlice := value.([]interface{})
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
		params            []string
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			helpers.RootFolder + "child",
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+config.DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../../../" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			map[string]config.IncludeConfig{"": {Path: helpers.RootFolder + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+config.DefaultTerragruntConfigPath),
			filepath.VolumeName(parentDir) + "/other-child",
		},
		{
			map[string]config.IncludeConfig{"": {Path: "../../" + config.DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, "../child/sub-child/"+config.DefaultTerragruntConfigPath),
			parentDir,
		},
		{
			map[string]config.IncludeConfig{
				"root":  {Path: "../../" + config.DefaultTerragruntConfigPath},
				"child": {Path: "../../other-child/" + config.DefaultTerragruntConfigPath},
			},
			[]string{"child"},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+config.DefaultTerragruntConfigPath),
			filepath.VolumeName(parentDir) + "/other-child",
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
		input    string
		expected interface{}
	}{
		{
			"abs(-1)",
			1.,
		},
		{
			`element(["one", "two", "three"], 1)`,
			"two",
		},
		{
			`chomp(file("other-file.txt"))`,
			"This is a test file",
		},
		{
			`sha1("input")`,
			"140f86aae51ab9e1cda9b4254fe98a74eb54c1a1",
		},
		{
			`split("|", "one|two|three")`,
			[]interface{}{"one", "two", "three"},
		},
		{
			`!tobool("false")`,
			true,
		},
		{
			`trimspace("     content     ")`,
			"content",
		},
		{
			`zipmap(["one", "two", "three"], [1, 2, 3])`,
			map[string]interface{}{"one": 1., "two": 2., "three": 3.},
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
		input    string
		expected map[string]cty.Value
	}{
		{
			`{"bool": {"sensitive": false, "type": "bool", "value": true}}`,
			map[string]cty.Value{"bool": cty.True},
		},
		{
			`{"number": {"sensitive": false, "type": "number", "value": 42}}`,
			map[string]cty.Value{"number": cty.NumberIntVal(42)},
		},
		{
			`{"list_string": {"sensitive": false, "type": ["list", "string"], "value": ["4", "2"]}}`,
			map[string]cty.Value{"list_string": cty.ListVal([]cty.Value{cty.StringVal("4"), cty.StringVal("2")})},
		},
		{
			`{"map_string": {"sensitive": false, "type": ["map", "string"], "value": {"x": "foo", "y": "bar"}}}`,
			map[string]cty.Value{"map_string": cty.MapVal(map[string]cty.Value{"x": cty.StringVal("foo"), "y": cty.StringVal("bar")})},
		},
		{
			`{"map_list_number": {"sensitive": false, "type": ["map", ["list", "number"]], "value": {"x": [4, 2]}}}`,
			map[string]cty.Value{
				"map_list_number": cty.MapVal(
					map[string]cty.Value{
						"x": cty.ListVal([]cty.Value{cty.NumberIntVal(4), cty.NumberIntVal(2)}),
					},
				),
			},
		},
		{
			`{"object": {"sensitive": false, "type": ["object", {"x": "number", "y": "string", "lst": ["list", "string"]}], "value": {"x": 42, "y": "the truth", "lst": ["foo", "bar"]}}}`,
			map[string]cty.Value{
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
			`{"out1": {"sensitive": false, "type": "number", "value": 42}, "out2": {"sensitive": false, "type": "string", "value": "foo bar"}}`,
			map[string]cty.Value{
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

	inputsMap := tgConfigMap["inputs"].(map[string]interface{})

	assert.Equal(t, "string", inputsMap["string"].(string))
	assert.InEpsilon(t, float64(42), inputsMap["number"].(float64), 0.0000000001)
	assert.True(t, inputsMap["bool"].(bool))
	assert.Equal(t, []interface{}{"a", "b", "c"}, inputsMap["list_string"].([]interface{}))
	assert.Equal(t, []interface{}{float64(1), float64(2), float64(3)}, inputsMap["list_number"].([]interface{}))
	assert.Equal(t, []interface{}{true, false}, inputsMap["list_bool"].([]interface{}))
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, inputsMap["map_string"].(map[string]interface{}))
	assert.Equal(t, map[string]interface{}{"foo": float64(42), "bar": float64(12345)}, inputsMap["map_number"].(map[string]interface{}))
	assert.Equal(t, map[string]interface{}{"foo": true, "bar": false, "baz": true}, inputsMap["map_bool"].(map[string]interface{}))

	assert.Equal(
		t,
		map[string]interface{}{
			"str":  "string",
			"num":  float64(42),
			"list": []interface{}{float64(1), float64(2), float64(3)},
			"map":  map[string]interface{}{"foo": "bar"},
		},
		inputsMap["object"].(map[string]interface{}),
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

	remoteStateMap := tgConfigMap["remote_state"].(map[string]interface{})
	assert.Equal(t, "s3", remoteStateMap["backend"].(string))
	configMap := remoteStateMap["config"].(map[string]interface{})
	assert.True(t, configMap["encrypt"].(bool))
	assert.Equal(t, "terraform.tfstate", configMap["key"].(string))
	assert.Equal(
		t,
		map[string]interface{}{"owner": "terragrunt integration test", "name": "Terraform state storage"},
		configMap["s3_bucket_tags"].(map[string]interface{}),
	)
	assert.Equal(
		t,
		map[string]interface{}{"owner": "terragrunt integration test", "name": "Terraform lock table"},
		configMap["dynamodb_table_tags"].(map[string]interface{}),
	)
	assert.Equal(
		t,
		map[string]interface{}{"owner": "terragrunt integration test", "name": "Terraform access log storage"},
		configMap["accesslogging_bucket_tags"].(map[string]interface{}),
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

	terraformMap := tgConfigMap["terraform"].(map[string]interface{})
	beforeHooksMap := terraformMap["before_hook"].(map[string]interface{})
	assert.Equal(
		t,
		[]interface{}{"touch", "before.out"},
		beforeHooksMap["before_hook_1"].(map[string]interface{})["execute"].([]interface{}),
	)
	assert.Equal(
		t,
		[]interface{}{"echo", "BEFORE_TERRAGRUNT_READ_CONFIG"},
		beforeHooksMap["before_hook_2"].(map[string]interface{})["execute"].([]interface{}),
	)

	afterHooksMap := terraformMap["after_hook"].(map[string]interface{})
	assert.Equal(
		t,
		[]interface{}{"touch", "after.out"},
		afterHooksMap["after_hook_1"].(map[string]interface{})["execute"].([]interface{}),
	)
	assert.Equal(
		t,
		[]interface{}{"echo", "AFTER_TERRAGRUNT_READ_CONFIG"},
		afterHooksMap["after_hook_2"].(map[string]interface{})["execute"].([]interface{}),
	)
	errorHooksMap := terraformMap["error_hook"].(map[string]interface{})
	assert.Equal(
		t,
		[]interface{}{"echo", "ON_APPLY_ERROR"},
		errorHooksMap["error_hook_1"].(map[string]interface{})["execute"].([]interface{}),
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

	localsMap := tgConfigMap["locals"].(map[string]interface{})
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
		args   []string
		value  int64
		err    string
	}{
		{terragruntOptionsForTest(t, ""), []string{"2017-11-22T00:00:00Z", "2017-11-22T00:00:00Z"}, 0, ""},
		{terragruntOptionsForTest(t, ""), []string{"2017-11-22T00:00:00Z", "2017-11-22T01:00:00+01:00"}, 0, ""},
		{terragruntOptionsForTest(t, ""), []string{"2017-11-22T00:00:01Z", "2017-11-22T01:00:00+01:00"}, 1, ""},
		{terragruntOptionsForTest(t, ""), []string{"2017-11-22T01:00:00Z", "2017-11-22T00:59:00-01:00"}, -1, ""},
		{terragruntOptionsForTest(t, ""), []string{"2017-11-22T01:00:00+01:00", "2017-11-22T01:00:00-01:00"}, -1, ""},
		{terragruntOptionsForTest(t, ""), []string{"2017-11-22T01:00:00-01:00", "2017-11-22T01:00:00+01:00"}, 1, ""},
		{terragruntOptionsForTest(t, ""), []string{"2017-11-22T00:00:00Z", "bloop"}, 0, `could not parse second parameter "bloop": not a valid RFC3339 timestamp: cannot use "bloop" as year`},
		{terragruntOptionsForTest(t, ""), []string{"2017-11-22 00:00:00Z", "2017-11-22T00:00:00Z"}, 0, `could not parse first parameter "2017-11-22 00:00:00Z": not a valid RFC3339 timestamp: missing required time introducer 'T'`},
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
		args   []string
		value  bool
		err    string
	}{
		{terragruntOptionsForTest(t, ""), []string{"hello world", "hello"}, true, ""},
		{terragruntOptionsForTest(t, ""), []string{"hello world", "world"}, true, ""},
		{terragruntOptionsForTest(t, ""), []string{"hello world0", "0"}, true, ""},
		{terragruntOptionsForTest(t, ""), []string{"9hello world0", "9"}, true, ""},
		{terragruntOptionsForTest(t, ""), []string{"hello world", "test"}, false, ""},
		{terragruntOptionsForTest(t, ""), []string{"hello", "hello"}, true, ""},
		{terragruntOptionsForTest(t, ""), []string{}, false, "Empty string value is not allowed for parameter to the strcontains function"},
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

	locals := tgConfigMap["locals"].(map[string]interface{})

	assert.Equal(t, "string", locals["string_var"].(string))
	assert.InEpsilon(t, float64(42), locals["number_var"].(float64), 0.0000000001)
	assert.True(t, locals["bool_var"].(bool))
	assert.Equal(t, []interface{}{"hello", "world"}, locals["list_var"].([]interface{}))

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
