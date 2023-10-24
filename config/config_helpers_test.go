package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestPathRelativeToInclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		include           map[string]IncludeConfig
		params            []string
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			".",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			"child",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			"child",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../../../" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"child/sub-child/sub-sub-child",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"child/sub-child/sub-sub-child",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+DefaultTerragruntConfigPath),
			"../child/sub-child",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, "../child/sub-child/"+DefaultTerragruntConfigPath),
			"child/sub-child",
		},
		{
			map[string]IncludeConfig{
				"root":  IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath},
				"child": IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath},
			},
			[]string{"child"},
			terragruntOptionsForTest(t, "../child/sub-child/"+DefaultTerragruntConfigPath),
			"../child/sub-child",
		},
	}

	for _, testCase := range testCases {
		trackInclude := getTrackIncludeFromTestData(testCase.include, testCase.params)
		actualPath, actualErr := pathRelativeToInclude(testCase.params, trackInclude, testCase.terragruntOptions)
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}

func TestPathRelativeFromInclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		include           map[string]IncludeConfig
		params            []string
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			".",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			"..",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			"..",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../../../" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"../../..",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"../../..",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+DefaultTerragruntConfigPath),
			"../../other-child",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, "../child/sub-child/"+DefaultTerragruntConfigPath),
			"../..",
		},
		{
			map[string]IncludeConfig{
				"root":  IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath},
				"child": IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath},
			},
			[]string{"child"},
			terragruntOptionsForTest(t, "../child/sub-child/"+DefaultTerragruntConfigPath),
			"../../other-child",
		},
	}

	for _, testCase := range testCases {
		trackInclude := getTrackIncludeFromTestData(testCase.include, testCase.params)
		actualPath, actualErr := pathRelativeFromInclude(testCase.params, trackInclude, testCase.terragruntOptions)
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}

func TestRunCommand(t *testing.T) {
	t.Parallel()

	homeDir := os.Getenv("HOME")
	testCases := []struct {
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
			EmptyStringNotAllowed("{run_cmd()}"),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.terragruntOptions.TerragruntConfigPath, func(t *testing.T) {
			actualOutput, actualErr := runCommand(testCase.params, nil, testCase.terragruntOptions)
			if testCase.expectedErr != nil {
				if assert.Error(t, actualErr) {
					assert.IsType(t, testCase.expectedErr, errors.Unwrap(actualErr))
				}
			} else {
				assert.Nil(t, actualErr)
				assert.Equal(t, testCase.expectedOutput, actualOutput)
			}
		})
	}
}

func absPath(t *testing.T, path string) string {
	out, err := filepath.Abs(path)
	require.NoError(t, err)
	return out
}

func TestFindInParentFolders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		params            []string
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
		expectedErr       error
	}{
		{
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/"+DefaultTerragruntConfigPath),
			absPath(t, "../test/fixture-parent-folders/terragrunt-in-root/"+DefaultTerragruntConfigPath),
			nil,
		},
		{
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			absPath(t, "../test/fixture-parent-folders/terragrunt-in-root/"+DefaultTerragruntConfigPath),
			nil,
		},
		{
			nil,
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath, 3),
			"",
			ParentFileNotFound{},
		},
		{
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/"+DefaultTerragruntConfigPath),
			absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/"+DefaultTerragruntConfigPath),
			nil,
		},
		{
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+DefaultTerragruntConfigPath),
			absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/"+DefaultTerragruntConfigPath),
			nil,
		},
		{
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+DefaultTerragruntConfigPath),
			nil,
		},
		{
			[]string{"foo.txt"},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/other-file-names/child/"+DefaultTerragruntConfigPath),
			absPath(t, "../test/fixture-parent-folders/other-file-names/foo.txt"),
			nil,
		},
		{
			nil,
			terragruntOptionsForTest(t, "/"),
			"",
			ParentFileNotFound{},
		},
		{
			nil,
			terragruntOptionsForTest(t, "/fake/path"),
			"",
			ParentFileNotFound{},
		},
		{
			[]string{"foo.txt", "fallback.txt"},
			terragruntOptionsForTest(t, "/fake/path"),
			"fallback.txt",
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.terragruntOptions.TerragruntConfigPath, func(t *testing.T) {
			actualPath, actualErr := findInParentFolders(testCase.params, nil, testCase.terragruntOptions)
			if testCase.expectedErr != nil {
				if assert.Error(t, actualErr) {
					assert.IsType(t, testCase.expectedErr, errors.Unwrap(actualErr))
				}
			} else {
				assert.Nil(t, actualErr)
				assert.Equal(t, testCase.expectedPath, actualPath)
			}
		})
	}
}

func TestResolveTerragruntInterpolation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		str               string
		include           *IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedOut       string
		expectedErr       string
	}{
		{
			"terraform { source = path_relative_to_include() }",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			".",
			"",
		},
		{
			"terraform { source = path_relative_to_include() }",
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"child",
			"",
		},
		{
			"terraform { source = find_in_parent_folders() }",
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
			absPath(t, "../test/fixture-parent-folders/terragrunt-in-root/"+DefaultTerragruntConfigPath),
			"",
		},
		{
			"terraform { source = find_in_parent_folders() }",
			nil,
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath, 1),
			"",
			"ParentFileNotFound",
		},
		{
			"terraform { source = find_in_parent_folders() }",
			nil,
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath, 3),
			"",
			"ParentFileNotFound",
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase
		t.Run(fmt.Sprintf("%s--%s", testCase.str, testCase.terragruntOptions.TerragruntConfigPath), func(t *testing.T) {
			actualOut, actualErr := ParseConfigString(testCase.str, testCase.terragruntOptions, testCase.include, "mock-path-for-test.hcl", nil)
			if testCase.expectedErr != "" {
				require.Error(t, actualErr)
				assert.Contains(t, actualErr.Error(), testCase.expectedErr)
			} else {
				require.NoError(t, actualErr)
				require.NotNil(t, actualOut)
				require.NotNil(t, actualOut.Terraform)
				require.NotNil(t, actualOut.Terraform.Source)
				assert.Equal(t, testCase.expectedOut, *actualOut.Terraform.Source)
			}
		})
	}
}

func TestResolveEnvInterpolationConfigString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		str               string
		include           *IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedOut       string
		expectedErr       string
	}{
		{
			`iam_role = "foo/${get_env()}/bar"`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			"InvalidGetEnvParams",
		},
		{
			`iam_role = "foo/${get_env("","")}/bar"`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			"InvalidEnvParamName",
		},
		{
			`iam_role = get_env()`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			"InvalidGetEnvParams",
		},
		{
			`iam_role = get_env("TEST_VAR_1", "TEST_VAR_2", "TEST_VAR_3")`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			"InvalidGetEnvParams",
		},
		{
			`iam_role = get_env("TEST_ENV_TERRAGRUNT_VAR")`,
			nil,
			terragruntOptionsForTestWithEnv(t, fmt.Sprintf("/root/child/%s", DefaultTerragruntConfigPath), map[string]string{"TEST_ENV_TERRAGRUNT_VAR": "SOMETHING"}),
			"SOMETHING",
			"",
		},
		{
			`iam_role = get_env("SOME_VAR", "SOME_VALUE")`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"SOME_VALUE",
			"",
		},
		{
			`iam_role = "foo/${get_env("TEST_ENV_TERRAGRUNT_HIT","")}/bar"`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"}),
			"foo//bar",
			"",
		},
		{
			`iam_role = "foo/${get_env("TEST_ENV_TERRAGRUNT_HIT","DEFAULT")}/bar"`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"}),
			"foo/DEFAULT/bar",
			"",
		},
		{
			`iam_role = "foo/${get_env("TEST_ENV_TERRAGRUNT_VAR")}/bar"`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_VAR": "SOMETHING"}),
			"foo/SOMETHING/bar",
			"",
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase
		t.Run(testCase.str, func(t *testing.T) {
			actualOut, actualErr := ParseConfigString(testCase.str, testCase.terragruntOptions, testCase.include, "mock-path-for-test.hcl", nil)
			if testCase.expectedErr != "" {
				require.Error(t, actualErr)
				assert.Contains(t, actualErr.Error(), testCase.expectedErr)
			} else {
				require.NoError(t, actualErr)
				assert.Equal(t, testCase.expectedOut, actualOut.IamRole)
			}
		})
	}
}

func TestResolveCommandsInterpolationConfigString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		str               string
		include           *IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedFooInput  []string
	}{
		{
			"inputs = { foo = get_terraform_commands_that_need_locking() }",
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			TERRAFORM_COMMANDS_NEED_LOCKING,
		},
		{
			`inputs = { foo = get_terraform_commands_that_need_vars() }`,
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			TERRAFORM_COMMANDS_NEED_VARS,
		},
		{
			"inputs = { foo = get_terraform_commands_that_need_parallelism() }",
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			TERRAFORM_COMMANDS_NEED_PARALLELISM,
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		t.Run(testCase.str, func(t *testing.T) {
			actualOut, actualErr := ParseConfigString(testCase.str, testCase.terragruntOptions, testCase.include, "mock-path-for-test.hcl", nil)
			require.NoError(t, actualErr, "For string '%s' include %v and options %v, unexpected error: %v", testCase.str, testCase.include, testCase.terragruntOptions, actualErr)

			require.NotNil(t, actualOut)

			inputs := actualOut.Inputs
			require.NotNil(t, inputs)

			foo, containsFoo := inputs["foo"]
			assert.True(t, containsFoo)

			fooSlice := toStringSlice(t, foo)

			assert.EqualValues(t, testCase.expectedFooInput, fooSlice, "For string '%s' include %v and options %v", testCase.str, testCase.include, testCase.terragruntOptions)
		})
	}
}

func TestResolveCliArgsInterpolationConfigString(t *testing.T) {
	t.Parallel()

	for _, cliArgs := range [][]string{nil, {}, {"apply"}, {"plan", "-out=planfile"}} {
		opts := terragruntOptionsForTest(t, DefaultTerragruntConfigPath)
		opts.TerraformCliArgs = cliArgs
		expectedFooInput := cliArgs
		// Expecting nil to be returned for get_terraform_cli_args() call for
		// either nil or empty array of input args
		if len(cliArgs) == 0 {
			expectedFooInput = nil
		}
		testCase := struct {
			str               string
			include           *IncludeConfig
			terragruntOptions *options.TerragruntOptions
			expectedFooInput  []string
		}{
			"inputs = { foo = get_terraform_cli_args() }",
			nil,
			opts,
			expectedFooInput,
		}
		t.Run(testCase.str, func(t *testing.T) {
			actualOut, actualErr := ParseConfigString(testCase.str, testCase.terragruntOptions, testCase.include, "mock-path-for-test.hcl", nil)
			require.NoError(t, actualErr, "For string '%s' include %v and options %v, unexpected error: %v", testCase.str, testCase.include, testCase.terragruntOptions, actualErr)

			require.NotNil(t, actualOut)

			inputs := actualOut.Inputs
			require.NotNil(t, inputs)

			foo, containsFoo := inputs["foo"]
			assert.True(t, containsFoo)

			fooSlice := toStringSlice(t, foo)

			assert.EqualValues(t, testCase.expectedFooInput, fooSlice, "For string '%s' include %v and options %v", testCase.str, testCase.include, testCase.terragruntOptions)
		})
	}
}

func toStringSlice(t *testing.T, value interface{}) []string {
	asInterfaceSlice, isInterfaceSlice := value.([]interface{})
	require.True(t, isInterfaceSlice)

	var out []string
	for _, item := range asInterfaceSlice {
		asStr, isStr := item.(string)
		require.True(t, isStr)
		out = append(out, asStr)
	}

	return out
}

func TestGetTerragruntDirAbsPath(t *testing.T) {
	t.Parallel()
	workingDir, err := os.Getwd()
	assert.Nil(t, err, "Could not get current working dir: %v", err)
	testGetTerragruntDir(t, "/foo/bar/terragrunt.hcl", fmt.Sprintf("%s/foo/bar", filepath.VolumeName(workingDir)))
}

func TestGetTerragruntDirRelPath(t *testing.T) {
	t.Parallel()
	workingDir, err := os.Getwd()
	assert.Nil(t, err, "Could not get current working dir: %v", err)
	workingDir = filepath.ToSlash(workingDir)

	testGetTerragruntDir(t, "foo/bar/terragrunt.hcl", fmt.Sprintf("%s/foo/bar", workingDir))
}

func testGetTerragruntDir(t *testing.T, configPath string, expectedPath string) {
	terragruntOptions, err := options.NewTerragruntOptionsForTest(configPath)
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	actualPath, err := getTerragruntDir(nil, terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expectedPath, actualPath)
}

func terragruntOptionsForTest(t *testing.T, configPath string) *options.TerragruntOptions {
	opts, err := options.NewTerragruntOptionsForTest(configPath)
	if err != nil {
		t.Fatalf("Failed to create TerragruntOptions: %v", err)
	}
	return opts
}

func terragruntOptionsForTestWithMaxFolders(t *testing.T, configPath string, maxFoldersToCheck int) *options.TerragruntOptions {
	opts := terragruntOptionsForTest(t, configPath)
	opts.MaxFoldersToCheck = maxFoldersToCheck
	return opts
}

func terragruntOptionsForTestWithEnv(t *testing.T, configPath string, env map[string]string) *options.TerragruntOptions {
	opts := terragruntOptionsForTest(t, configPath)
	opts.Env = env
	return opts
}

func TestGetParentTerragruntDir(t *testing.T) {
	t.Parallel()

	currentDir, err := os.Getwd()
	assert.Nil(t, err, "Could not get current working dir: %v", err)
	parentDir := filepath.ToSlash(filepath.Dir(currentDir))

	testCases := []struct {
		include           map[string]IncludeConfig
		params            []string
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			helpers.RootFolder + "child",
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../../../" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+DefaultTerragruntConfigPath),
			fmt.Sprintf("%s/other-child", filepath.VolumeName(parentDir)),
		},
		{
			map[string]IncludeConfig{"": IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath}},
			nil,
			terragruntOptionsForTest(t, "../child/sub-child/"+DefaultTerragruntConfigPath),
			parentDir,
		},
		{
			map[string]IncludeConfig{
				"root":  IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath},
				"child": IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath},
			},
			[]string{"child"},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+DefaultTerragruntConfigPath),
			fmt.Sprintf("%s/other-child", filepath.VolumeName(parentDir)),
		},
	}

	for _, testCase := range testCases {
		trackInclude := getTrackIncludeFromTestData(testCase.include, testCase.params)
		actualPath, actualErr := getParentTerragruntDir(testCase.params, trackInclude, testCase.terragruntOptions)
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}

func TestTerraformBuiltInFunctions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
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

	for _, testCase := range testCases {
		t.Run(testCase.input, func(t *testing.T) {

			terragruntOptions := terragruntOptionsForTest(t, "../test/fixture-config-terraform-functions/"+DefaultTerragruntConfigPath)
			actual, err := ParseConfigString(fmt.Sprintf("inputs = { test = %s }", testCase.input), terragruntOptions, nil, terragruntOptions.TerragruntConfigPath, nil)
			require.NoError(t, err, "For hcl '%s' include %v and options %v, unexpected error: %v", testCase.input, nil, terragruntOptions, err)

			require.NotNil(t, actual)

			inputs := actual.Inputs
			require.NotNil(t, inputs)

			test, containsTest := inputs["test"]
			assert.True(t, containsTest)

			assert.EqualValues(t, testCase.expected, test, "For hcl '%s' include %v and options %v", testCase.input, nil, terragruntOptions)
		})
	}
}

func TestTerraformOutputJsonToCtyValueMap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
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

	mockTargetConfig := DefaultTerragruntConfigPath
	for _, testCase := range testCases {
		converted, err := terraformOutputJsonToCtyValueMap(mockTargetConfig, []byte(testCase.input))
		assert.NoError(t, err)
		assert.Equal(t, getKeys(converted), getKeys(testCase.expected))
		for k, v := range converted {
			assert.True(t, v.Equals(testCase.expected[k]).True())
		}
	}
}

func TestReadTerragruntConfigInputs(t *testing.T) {
	t.Parallel()

	options := terragruntOptionsForTest(t, DefaultTerragruntConfigPath)
	tgConfigCty, err := readTerragruntConfig("../test/fixture-inputs/terragrunt.hcl", nil, options)
	require.NoError(t, err)

	tgConfigMap, err := parseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)

	inputsMap := tgConfigMap["inputs"].(map[string]interface{})
	assert.Equal(t, inputsMap["string"].(string), "string")
	assert.Equal(t, inputsMap["number"].(float64), float64(42))
	assert.Equal(t, inputsMap["bool"].(bool), true)
	assert.Equal(t, inputsMap["list_string"].([]interface{}), []interface{}{"a", "b", "c"})
	assert.Equal(t, inputsMap["list_number"].([]interface{}), []interface{}{float64(1), float64(2), float64(3)})
	assert.Equal(t, inputsMap["list_bool"].([]interface{}), []interface{}{true, false})
	assert.Equal(t, inputsMap["map_string"].(map[string]interface{}), map[string]interface{}{"foo": "bar"})
	assert.Equal(
		t,
		inputsMap["map_number"].(map[string]interface{}),
		map[string]interface{}{"foo": float64(42), "bar": float64(12345)},
	)
	assert.Equal(
		t,
		inputsMap["map_bool"].(map[string]interface{}),
		map[string]interface{}{"foo": true, "bar": false, "baz": true},
	)
	assert.Equal(
		t,
		inputsMap["object"].(map[string]interface{}),
		map[string]interface{}{
			"str":  "string",
			"num":  float64(42),
			"list": []interface{}{float64(1), float64(2), float64(3)},
			"map":  map[string]interface{}{"foo": "bar"},
		},
	)
	assert.Equal(t, inputsMap["from_env"].(string), "default")
}

func TestReadTerragruntConfigRemoteState(t *testing.T) {
	t.Parallel()

	options := terragruntOptionsForTest(t, DefaultTerragruntConfigPath)
	tgConfigCty, err := readTerragruntConfig("../test/fixture/terragrunt.hcl", nil, options)
	require.NoError(t, err)

	tgConfigMap, err := parseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)

	remoteStateMap := tgConfigMap["remote_state"].(map[string]interface{})
	assert.Equal(t, remoteStateMap["backend"].(string), "s3")
	configMap := remoteStateMap["config"].(map[string]interface{})
	assert.Equal(t, configMap["encrypt"].(bool), true)
	assert.Equal(t, configMap["key"].(string), "terraform.tfstate")
	assert.Equal(
		t,
		configMap["s3_bucket_tags"].(map[string]interface{}),
		map[string]interface{}{"owner": "terragrunt integration test", "name": "Terraform state storage"},
	)
	assert.Equal(
		t,
		configMap["dynamodb_table_tags"].(map[string]interface{}),
		map[string]interface{}{"owner": "terragrunt integration test", "name": "Terraform lock table"},
	)
	assert.Equal(
		t,
		configMap["accesslogging_bucket_tags"].(map[string]interface{}),
		map[string]interface{}{"owner": "terragrunt integration test", "name": "Terraform access log storage"},
	)
}

func TestReadTerragruntConfigHooks(t *testing.T) {
	t.Parallel()

	options := terragruntOptionsForTest(t, DefaultTerragruntConfigPath)
	tgConfigCty, err := readTerragruntConfig("../test/fixture-hooks/before-after-and-on-error/terragrunt.hcl", nil, options)
	require.NoError(t, err)

	tgConfigMap, err := parseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)

	terraformMap := tgConfigMap["terraform"].(map[string]interface{})
	beforeHooksMap := terraformMap["before_hook"].(map[string]interface{})
	assert.Equal(
		t,
		beforeHooksMap["before_hook_1"].(map[string]interface{})["execute"].([]interface{}),
		[]interface{}{"touch", "before.out"},
	)
	assert.Equal(
		t,
		beforeHooksMap["before_hook_2"].(map[string]interface{})["execute"].([]interface{}),
		[]interface{}{"echo", "BEFORE_TERRAGRUNT_READ_CONFIG"},
	)

	afterHooksMap := terraformMap["after_hook"].(map[string]interface{})
	assert.Equal(
		t,
		afterHooksMap["after_hook_1"].(map[string]interface{})["execute"].([]interface{}),
		[]interface{}{"touch", "after.out"},
	)
	assert.Equal(
		t,
		afterHooksMap["after_hook_2"].(map[string]interface{})["execute"].([]interface{}),
		[]interface{}{"echo", "AFTER_TERRAGRUNT_READ_CONFIG"},
	)
	errorHooksMap := terraformMap["error_hook"].(map[string]interface{})
	assert.Equal(
		t,
		errorHooksMap["error_hook_1"].(map[string]interface{})["execute"].([]interface{}),
		[]interface{}{"echo", "ON_APPLY_ERROR"},
	)
}

func TestReadTerragruntConfigLocals(t *testing.T) {
	t.Parallel()

	options := terragruntOptionsForTest(t, DefaultTerragruntConfigPath)
	tgConfigCty, err := readTerragruntConfig("../test/fixture-locals/canonical/terragrunt.hcl", nil, options)
	require.NoError(t, err)

	tgConfigMap, err := parseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)

	localsMap := tgConfigMap["locals"].(map[string]interface{})
	assert.Equal(t, localsMap["x"].(float64), float64(2))
	assert.Equal(t, localsMap["file_contents"].(string), "Hello world\n")
	assert.Equal(t, localsMap["number_expression"].(float64), float64(42))
}

func TestGetTerragruntSourceForModuleHappyPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		config   *TerragruntConfig
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

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase
		t.Run(fmt.Sprintf("%v-%s", *testCase.config.Terraform.Source, testCase.source), func(t *testing.T) {
			actual, err := GetTerragruntSourceForModule(testCase.source, "mock-for-test", testCase.config)
			require.NoError(t, err)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestStartsWith(t *testing.T) {
	t.Parallel()

	testCases := []struct {
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

	for id, testCase := range testCases {
		testCase := testCase
		t.Run(fmt.Sprintf("%v %v", id, testCase.args), func(t *testing.T) {
			actual, err := startsWith(testCase.args, nil, testCase.config)
			assert.NoError(t, err)
			assert.Equal(t, testCase.value, actual)
		})
	}
}

func TestEndsWith(t *testing.T) {
	t.Parallel()

	testCases := []struct {
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

	for id, testCase := range testCases {
		testCase := testCase
		t.Run(fmt.Sprintf("%v %v", id, testCase.args), func(t *testing.T) {
			actual, err := endsWith(testCase.args, nil, testCase.config)
			assert.NoError(t, err)
			assert.Equal(t, testCase.value, actual)
		})
	}
}

func TestTimeCmp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
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

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("TimeCmp(%#v, %#v)", testCase.args[0], testCase.args[1]), func(t *testing.T) {
			t.Parallel()

			actual, err := timeCmp(testCase.args, nil, testCase.config)
			if testCase.err != "" {
				assert.EqualError(t, err, testCase.err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, testCase.value, actual)
		})
	}
}

func TestStrContains(t *testing.T) {
	t.Parallel()

	testCases := []struct {
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

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("StrContains %v", testCase.args), func(t *testing.T) {
			t.Parallel()

			actual, err := strContains(testCase.args, nil, testCase.config)
			if testCase.err != "" {
				assert.EqualError(t, err, testCase.err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, testCase.value, actual)
		})
	}
}

func TestReadTFVarsFiles(t *testing.T) {
	t.Parallel()

	options := terragruntOptionsForTest(t, DefaultTerragruntConfigPath)
	tgConfigCty, err := readTerragruntConfig("../test/fixture-read-tf-vars/terragrunt.hcl", nil, options)
	require.NoError(t, err)

	tgConfigMap, err := parseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)

	locals := tgConfigMap["locals"].(map[string]interface{})

	assert.Equal(t, locals["string_var"].(string), "string")
	assert.Equal(t, locals["number_var"].(float64), float64(42))
	assert.Equal(t, locals["bool_var"].(bool), true)
	assert.Equal(t, locals["list_var"].([]interface{}), []interface{}{"hello", "world"})

	assert.Equal(t, locals["json_number_var"].(float64), float64(24))
	assert.Equal(t, locals["json_string_var"].(string), "another string")
	assert.Equal(t, locals["json_bool_var"].(bool), false)
}

func mockConfigWithSource(sourceUrl string) *TerragruntConfig {
	cfg := TerragruntConfig{IsPartial: true}
	cfg.Terraform = &TerraformConfig{Source: &sourceUrl}
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

func getTrackIncludeFromTestData(includeMap map[string]IncludeConfig, params []string) *TrackInclude {
	if len(includeMap) == 0 {
		return nil
	}
	currentList := make([]IncludeConfig, len(includeMap))
	i := 0
	for _, val := range includeMap {
		currentList[i] = val
		i++
	}
	trackInclude := &TrackInclude{
		CurrentList: currentList,
		CurrentMap:  includeMap,
	}
	if len(params) == 0 {
		trackInclude.Original = &currentList[0]
	}
	return trackInclude
}
