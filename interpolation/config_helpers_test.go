package interpolation

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/errors"
	. "github.com/gruntwork-io/terragrunt/models"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
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
		{
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
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
		{
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
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
		params            string
		terragruntOptions *options.TerragruntOptions
		expectedOutput    string
		expectedErr       error
	}{
		{
			`"/bin/bash", "-c", ""echo -n foo""`,
			terragruntOptionsForTest(t, homeDir),
			"foo",
			nil,
		},
		{
			"",
			terragruntOptionsForTest(t, homeDir),
			"",
			EmptyStringNotAllowed("{run_cmd()}"),
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.terragruntOptions.TerragruntConfigPath, func(t *testing.T) {
			actualOutput, actualErr := runCommand(testCase.params, testCase.terragruntOptions)
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
		params            string
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
		expectedErr       error
	}{
		{
			"",
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/"+DefaultTerragruntConfigPath),
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"",
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"../../../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"",
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath, 3),
			"",
			ParentFileNotFound{},
		},
		{
			"",
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/"+DefaultTerragruntConfigPath),
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"",
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+DefaultTerragruntConfigPath),
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"",
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			`"foo.txt"`,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/other-file-names/child/"+DefaultTerragruntConfigPath),
			"../foo.txt",
			nil,
		},
		{
			"",
			terragruntOptionsForTest(t, "/"),
			"",
			ParentFileNotFound{},
		},
		{
			"",
			terragruntOptionsForTest(t, "/fake/path"),
			"",
			ParentFileNotFound{},
		},
		{
			`"foo.txt", "fallback.txt"`,
			terragruntOptionsForTest(t, "/fake/path"),
			"fallback.txt",
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.terragruntOptions.TerragruntConfigPath, func(t *testing.T) {
			actualPath, actualErr := findInParentFolders(testCase.params, testCase.terragruntOptions)
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

func TestParseOptionalQuotedParamsHappyPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		params              string
		expectedNumParams   int
		expectedFirstParam  string
		expectedSecondParam string
	}{
		{``, 0, "", ""},
		{`   `, 0, "", ""},
		{`""`, 1, "", ""},
		{`"foo.txt"`, 1, "foo.txt", ""},
		{`"foo bar baz"`, 1, "foo bar baz", ""},
		{`"",""`, 2, "", ""},
		{`"" ,     ""`, 2, "", ""},
		{`"foo","bar"`, 2, "foo", "bar"},
		{`"foo",     "bar"`, 2, "foo", "bar"},
		{`"","bar"`, 2, "", "bar"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.params, func(t *testing.T) {
			actualFirstParam, actualSecondParam, actualNumParams, err := parseOptionalQuotedParam(testCase.params)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedNumParams, actualNumParams)
			assert.Equal(t, testCase.expectedFirstParam, actualFirstParam)
			assert.Equal(t, testCase.expectedSecondParam, actualSecondParam)
		})
	}
}

func TestParseParamList(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		params       string
		expectedList []string
	}{
		{`"foo"`, []string{"foo"}},
		{`"foo", "bar"`, []string{"foo", "bar"}},
		{`"foo", "bar", "biz"`, []string{"foo", "bar", "biz"}},
		{`"foo, bar"`, []string{"foo, bar"}},
		{``, []string{}},
		{`"foo`, []string{}},
		{`"foo, bar`, []string{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.params, func(t *testing.T) {
			actualList := parseParamList(testCase.params)
			assert.Equal(t, testCase.expectedList, actualList)
		})
	}
}

func TestParseOptionalQuotedParamsErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		params   string
		expected error
	}{
		{`abc`, InvalidStringParams(`abc`)},
		{`"`, InvalidStringParams(`"`)},
		{`"foo", "`, InvalidStringParams(`"foo", "`)},
		{`"foo" "bar"`, InvalidStringParams(`"foo" "bar"`)},
		{`"foo", "bar", "baz"`, InvalidStringParams(`"foo", "bar", "baz"`)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.params, func(t *testing.T) {
			_, _, _, err := parseOptionalQuotedParam(testCase.params)
			if assert.Error(t, err) {
				assert.IsType(t, testCase.expected, errors.Unwrap(err))
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
		expectedErr       error
	}{
		{
			"${path_relative_to_include()}",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			".",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"child",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
			"child/sub-child",
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
			"../../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath, 1),
			"",
			ParentFileNotFound{},
		},
		{
			"${find_in_parent_folders()}",
			nil,
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath, 3),
			"",
			ParentFileNotFound{},
		},
		{
			"${find_in_parent_folders}",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			InvalidInterpolationSyntax("${find_in_parent_folders}"),
		},
		{
			"{find_in_parent_folders()}",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			InvalidInterpolationSyntax("{find_in_parent_folders()}"),
		},
		{
			"find_in_parent_folders()",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			InvalidInterpolationSyntax("find_in_parent_folders()"),
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("%s--%s", testCase.str, testCase.terragruntOptions.TerragruntConfigPath), func(t *testing.T) {
			actualOut, actualErr := resolveTerragruntInterpolation(testCase.str, testCase.include, testCase.terragruntOptions)
			if testCase.expectedErr != nil {
				if assert.Error(t, actualErr) {
					assert.IsType(t, testCase.expectedErr, errors.Unwrap(actualErr))
				}
			} else {
				assert.Nil(t, actualErr)
				assert.Equal(t, testCase.expectedOut, actualOut)
			}
		})
	}
}

func TestResolveTerragruntConfigString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		str               string
		include           *IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedOut       string
		expectedErr       error
	}{
		{
			"",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			nil,
		},
		{
			"foo bar",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"foo bar",
			nil,
		},
		{
			"$foo {bar}",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"$foo {bar}",
			nil,
		},
		{
			"${path_relative_to_include()}",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			".",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"child",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
			"child/sub-child",
			nil,
		},
		{
			"foo/${path_relative_to_include()}/bar",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"foo/./bar",
			nil,
		},
		{
			"foo/${path_relative_to_include()}/bar",
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"foo/child/bar",
			nil,
		},
		{
			"foo/${path_relative_to_include()}/bar/${path_relative_to_include()}",
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"foo/child/bar/child",
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
			"../../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"${    find_in_parent_folders()    }",
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
			"../../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"${find_in_parent_folders ()}",
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
			"",
			InvalidInterpolationSyntax("${find_in_parent_folders ()}"),
		},
		{
			"foo/${find_in_parent_folders()}/bar",
			nil,
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
			fmt.Sprintf("foo/../../%s/bar", DefaultTerragruntConfigPath),
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath, 3),
			"",
			ParentFileNotFound{},
		},
		{
			"foo/${unknown}/bar",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			InvalidInterpolationSyntax("${unknown}"),
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("%s--%s", testCase.str, testCase.terragruntOptions.TerragruntConfigPath), func(t *testing.T) {
			actualOut, actualErr := ResolveTerragruntConfigString(testCase.str, testCase.include, testCase.terragruntOptions)
			if testCase.expectedErr != nil {
				if assert.Error(t, actualErr) {
					assert.IsType(t, testCase.expectedErr, errors.Unwrap(actualErr))
				}
			} else {
				assert.Nil(t, actualErr)
				assert.Equal(t, testCase.expectedOut, actualOut)
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
		expectedErr       error
	}{
		{
			"foo/${get_env()}/bar",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			InvalidGetEnvParams(""),
		},
		{
			"foo/${get_env(Invalid Parameters)}/bar",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			InvalidInterpolationSyntax("${get_env(Invalid Parameters)}"),
		},
		{
			"foo/${get_env('env','')}/bar",
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			InvalidInterpolationSyntax("${get_env('env','')}"),
		},
		{
			`foo/${get_env("","")}/bar`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			InvalidGetEnvParams(`"",""`),
		},
		{
			`foo/${get_env(   ""    ,   ""    )}/bar`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			InvalidGetEnvParams(`   ""    ,   ""    `),
		},
		{
			`${get_env("SOME_VAR", "SOME{VALUE}")}`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"SOME{VALUE}",
			nil,
		},
		{
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT","")}/bar`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"}),
			"foo//bar",
			nil,
		},
		{
			`foo/${get_env(    "TEST_ENV_TERRAGRUNT_HIT"   ,   ""   )}/bar`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"}),
			"foo//bar",
			nil,
		},
		{
			`foo/${get_env("TEST_ENV_
TERRAGRUNT_HIT","")}/bar`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"}),
			"",
			InvalidInterpolationSyntax(`${get_env("TEST_ENV_
TERRAGRUNT_HIT","")}`),
		},
		{
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT","DEFAULT")}/bar`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"}),
			"foo/DEFAULT/bar",
			nil,
		},
		{
			`foo/${get_env(    "TEST_ENV_TERRAGRUNT_HIT      "   ,   "default"   )}/bar`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_HIT": "environment hit  "}),
			"foo/environment hit  /bar",
			nil,
		},
		{
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT","default")}/bar`,
			nil,
			terragruntOptionsForTestWithEnv(t, "/root/child/"+DefaultTerragruntConfigPath, map[string]string{"TEST_ENV_TERRAGRUNT_HIT": "HIT"}),
			"foo/HIT/bar",
			nil,
		},
		{
			// Unclosed quote
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT}/bar`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			"",
			InvalidInterpolationSyntax(`${get_env("TEST_ENV_TERRAGRUNT_HIT}`),
		},
		{
			// Unclosed quote and interpolation pattern
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT/bar`,
			nil,
			terragruntOptionsForTest(t, "/root/child/"+DefaultTerragruntConfigPath),
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT/bar`,
			nil,
		},
	}

	for _, testCase := range testCases {
		actualOut, actualErr := ResolveTerragruntConfigString(testCase.str, testCase.include, testCase.terragruntOptions)
		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "For string '%s' include %v and options %v, expected error %v but got error %v and output %v", testCase.str, testCase.include, testCase.terragruntOptions, testCase.expectedErr, actualErr, actualOut)
		} else {
			assert.Nil(t, actualErr, "For string '%s' include %v and options %v, unexpected error: %v", testCase.str, testCase.include, testCase.terragruntOptions, actualErr)
			assert.Equal(t, testCase.expectedOut, actualOut, "For string '%s' include %v and options %v", testCase.str, testCase.include, testCase.terragruntOptions)
		}
	}
}

func TestResolveCommandsInterpolationConfigString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		str               string
		include           *IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedOut       string
		expectedErr       error
	}{
		{
			`"${get_terraform_commands_that_need_locking()}"`,
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			util.CommaSeparatedStrings(TERRAFORM_COMMANDS_NEED_LOCKING),
			nil,
		},
		{
			`commands = ["${get_terraform_commands_that_need_vars()}"]`,
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			fmt.Sprintf("commands = [%s]", util.CommaSeparatedStrings(TERRAFORM_COMMANDS_NEED_VARS)),
			nil,
		},
		{
			`commands = "test-${get_terraform_commands_that_need_vars()}"`,
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			fmt.Sprintf(`commands = "test-%v"`, TERRAFORM_COMMANDS_NEED_VARS),
			nil,
		},
	}

	for _, testCase := range testCases {
		actualOut, actualErr := ResolveTerragruntConfigString(testCase.str, testCase.include, testCase.terragruntOptions)
		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "For string '%s' include %v and options %v, expected error %v but got error %v and output %v", testCase.str, testCase.include, testCase.terragruntOptions, testCase.expectedErr, actualErr, actualOut)
		} else {
			assert.Nil(t, actualErr, "For string '%s' include %v and options %v, unexpected error: %v", testCase.str, testCase.include, testCase.terragruntOptions, actualErr)
			assert.Equal(t, testCase.expectedOut, actualOut, "For string '%s' include %v and options %v", testCase.str, testCase.include, testCase.terragruntOptions)
		}
	}
}

func TestResolveMultipleInterpolationsConfigString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		str               string
		include           *IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedOut       string
		expectedErr       error
	}{
		{
			`${get_env("NON_EXISTING_VAR1", "default1")}-${get_env("NON_EXISTING_VAR2", "default2")}`,
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			fmt.Sprintf("default1-default2"),
			nil,
		},
		{
			// Included within quotes
			`"${get_env("NON_EXISTING_VAR1", "default1")}-${get_env("NON_EXISTING_VAR2", "default2")}"`,
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			`"default1-default2"`,
			nil,
		},
		{
			// Malformed parameters
			`${get_env("NON_EXISTING_VAR1", "default"-${get_terraform_commands_that_need_vars()}`,
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			fmt.Sprintf(`${get_env("NON_EXISTING_VAR1", "default"-%v`, TERRAFORM_COMMANDS_NEED_VARS),
			nil,
		},
		{
			`test1 = "${get_env("NON_EXISTING_VAR1", "default")}" test2 = ["${get_terraform_commands_that_need_vars()}"]`,
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			fmt.Sprintf(`test1 = "default" test2 = [%v]`, util.CommaSeparatedStrings(TERRAFORM_COMMANDS_NEED_VARS)),
			nil,
		},
		{
			`${get_env("NON_EXISTING_VAR1", "default")}-${get_terraform_commands_that_need_vars()}`,
			nil,
			terragruntOptionsForTest(t, DefaultTerragruntConfigPath),
			fmt.Sprintf("default-%v", TERRAFORM_COMMANDS_NEED_VARS),
			nil,
		},
	}

	for _, testCase := range testCases {
		actualOut, actualErr := ResolveTerragruntConfigString(testCase.str, testCase.include, testCase.terragruntOptions)
		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "For string '%s' include %v and options %v, expected error %v but got error %v and output %v", testCase.str, testCase.include, testCase.terragruntOptions, testCase.expectedErr, actualErr, actualOut)
		} else {
			assert.Nil(t, actualErr, "For string '%s' include %v and options %v, unexpected error: %v", testCase.str, testCase.include, testCase.terragruntOptions, actualErr)
			assert.Equal(t, testCase.expectedOut, actualOut, "For string '%s' include %v and options %v", testCase.str, testCase.include, testCase.terragruntOptions)
		}
	}
}

func TestGetTfVarsDirAbsPath(t *testing.T) {
	t.Parallel()
	workingDir, err := os.Getwd()
	assert.Nil(t, err, "Could not get current working dir: %v", err)
	testGetTfVarsDir(t, "/foo/bar/terraform.tfvars", fmt.Sprintf("%s/foo/bar", filepath.VolumeName(workingDir)))
}

func TestGetTfVarsDirRelPath(t *testing.T) {
	t.Parallel()
	workingDir, err := os.Getwd()
	assert.Nil(t, err, "Could not get current working dir: %v", err)
	workingDir = filepath.ToSlash(workingDir)

	testGetTfVarsDir(t, "foo/bar/terraform.tfvars", fmt.Sprintf("%s/foo/bar", workingDir))
}

func testGetTfVarsDir(t *testing.T, configPath string, expectedPath string) {
	terragruntOptions, err := options.NewTerragruntOptionsForTest(configPath)
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	actualPath, err := getTfVarsDir(terragruntOptions)

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

func TestGetParentTfVarsDir(t *testing.T) {
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
		{
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath),
			fmt.Sprintf("%s/test/fixture-parent-folders/terragrunt-in-root", parentDir),
		},
	}

	for _, testCase := range testCases {
		actualPath, actualErr := getParentTfVarsDir(testCase.include, testCase.terragruntOptions)
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}
