package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestPathRelativeToInclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		include           *IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			".",
		},
		{
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			"child",
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			"child",
		},
		{
			&IncludeConfig{Path: "../../../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"child/sub-child/sub-sub-child",
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"child/sub-child/sub-sub-child",
		},
		{
			&IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+DefaultTerragruntConfigPath),
			"../child/sub-child",
		},
		{
			&IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, "../child/sub-child/"+DefaultTerragruntConfigPath),
			"child/sub-child",
		},
	}

	for _, testCase := range testCases {
		actualPath, actualErr := pathRelativeToInclude(testCase.include, testCase.terragruntOptions)
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}

func TestPathRelativeFromInclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		include           *IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			".",
		},
		{
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			"..",
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			"..",
		},
		{
			&IncludeConfig{Path: "../../../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"../../..",
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"../../..",
		},
		{
			&IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+DefaultTerragruntConfigPath),
			"../../other-child",
		},
		{
			&IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, "../child/sub-child/"+DefaultTerragruntConfigPath),
			"../..",
		},
	}

	for _, testCase := range testCases {
		actualPath, actualErr := pathRelativeFromInclude(testCase.include, testCase.terragruntOptions)
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
			[]string{"--terragrunt-quiet", "/bin/bash", "-c", "echo -n foo"},
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
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"../../../" + DefaultTerragruntConfigPath,
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
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+DefaultTerragruntConfigPath),
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			[]string{"foo.txt"},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/other-file-names/child/"+DefaultTerragruntConfigPath),
			"../foo.txt",
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
			"../../" + DefaultTerragruntConfigPath,
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
			actualOut, actualErr := ParseConfigString(testCase.str, testCase.terragruntOptions, testCase.include, "mock-path-for-test.hcl")
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
			"InvalidGetEnvParams",
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
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase
		t.Run(testCase.str, func(t *testing.T) {
			actualOut, actualErr := ParseConfigString(testCase.str, testCase.terragruntOptions, testCase.include, "mock-path-for-test.hcl")
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
			actualOut, actualErr := ParseConfigString(testCase.str, testCase.terragruntOptions, testCase.include, "mock-path-for-test.hcl")
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
		include           *IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			helpers.RootFolder + "child",
		},
		{
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/"+DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			&IncludeConfig{Path: "../../../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			helpers.RootFolder,
		},
		{
			&IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, helpers.RootFolder+"child/sub-child/"+DefaultTerragruntConfigPath),
			fmt.Sprintf("%s/other-child", filepath.VolumeName(parentDir)),
		},
		{
			&IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, "../child/sub-child/"+DefaultTerragruntConfigPath),
			parentDir,
		},
	}

	for _, testCase := range testCases {
		actualPath, actualErr := getParentTerragruntDir(testCase.include, testCase.terragruntOptions)
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
			actual, err := ParseConfigString(fmt.Sprintf("inputs = { test = %s }", testCase.input), terragruntOptions, nil, terragruntOptions.TerragruntConfigPath)
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

// Return keys as a map so it is treated like a set, and order doesn't matter when comparing equivalence
func getKeys(valueMap map[string]cty.Value) map[string]bool {
	keys := map[string]bool{}
	for k, _ := range valueMap {
		keys[k] = true
	}
	return keys
}
