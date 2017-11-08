package config

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

func TestPathRelativeToInclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		include           *IncludeConfig
		terragruntOptions options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			".",
		},
		{
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"child",
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"child",
		},
		{
			&IncludeConfig{Path: "../../../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"child/sub-child/sub-sub-child",
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"child/sub-child/sub-sub-child",
		},
		{
			&IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../child/sub-child",
		},
		{
			&IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: "../child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"child/sub-child",
		},
		{
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"child/sub-child",
		},
	}

	for _, testCase := range testCases {
		actualPath, actualErr := pathRelativeToInclude(testCase.include, &testCase.terragruntOptions)
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}

func TestPathRelativeFromInclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		include           *IncludeConfig
		terragruntOptions options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			".",
		},
		{
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"..",
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"..",
		},
		{
			&IncludeConfig{Path: "../../../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../../..",
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../../..",
		},
		{
			&IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../../other-child",
		},
		{
			&IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: "../child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../..",
		},
		{
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../..",
		},
	}

	for _, testCase := range testCases {
		actualPath, actualErr := pathRelativeFromInclude(testCase.include, &testCase.terragruntOptions)
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}

func TestFindInParentFolders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		params            string
		terragruntOptions options.TerragruntOptions
		expectedPath      string
		expectedErr       error
	}{
		{
			"",
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"",
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../../../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"",
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			ParentFileNotFound{},
		},
		{
			"",
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"",
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"",
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			`"foo.txt"`,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/other-file-names/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../foo.txt",
			nil,
		},
		{
			"",
			options.TerragruntOptions{TerragruntConfigPath: "/", NonInteractive: true},
			"",
			ParentFileNotFound{},
		},
		{
			"",
			options.TerragruntOptions{TerragruntConfigPath: "/fake/path", NonInteractive: true},
			"",
			ParentFileNotFound{},
		},
		{
			`"foo.txt", "fallback.txt"`,
			options.TerragruntOptions{TerragruntConfigPath: "/fake/path", NonInteractive: true},
			"fallback.txt",
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.terragruntOptions.TerragruntConfigPath, func(t *testing.T) {
			actualPath, actualErr := findInParentFolders(testCase.params, &testCase.terragruntOptions)
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
		terragruntOptions options.TerragruntOptions
		expectedOut       interface{}
		expectedErr       error
	}{
		{
			"${path_relative_to_include()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			".",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"child",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"child/sub-child",
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			ParentFileNotFound{},
		},
		{
			"${find_in_parent_folders}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidInterpolationSyntax("${find_in_parent_folders}"),
		},
		{
			"{find_in_parent_folders()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidInterpolationSyntax("{find_in_parent_folders()}"),
		},
		{
			"find_in_parent_folders()",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidInterpolationSyntax("find_in_parent_folders()"),
		},
		{
			`${import_parent_tree("*.tfvars")}`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/tfvar-tree/child/sub-child", NonInteractive: true},
			[]string{
				"-var-file=/Users/simas/go/src/github.com/gruntwork-io/terragrunt/test/fixture-parent-folders/tfvar-tree/terraform.tfvars",
				"-var-file=/Users/simas/go/src/github.com/gruntwork-io/terragrunt/test/fixture-parent-folders/tfvar-tree/variables.tfvars",
			},
			nil,
		},
		{
			`${import_parent_tree()}`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/tfvar-tree/child/sub-child", NonInteractive: true},
			[]string{},
			nil,
		},
		{
			`${import_parent_tree("versions.tfvars")}`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/tfvar-tree/child/sub-child", NonInteractive: true},
			[]string{
				"-var-file=/Users/simas/go/src/github.com/gruntwork-io/terragrunt/test/fixture-parent-folders/tfvar-tree/terraform.tfvars",
				"-var-file=/Users/simas/go/src/github.com/gruntwork-io/terragrunt/test/fixture-parent-folders/tfvar-tree/variables.tfvars",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.str, func(t *testing.T) {
			actualOut, actualErr := resolveTerragruntInterpolation(testCase.str, testCase.include, &testCase.terragruntOptions)
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
		terragruntOptions options.TerragruntOptions
		expectedOut       string
		expectedErr       error
	}{
		{
			"",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			nil,
		},
		{
			"foo bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"foo bar",
			nil,
		},
		{
			"$foo {bar}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"$foo {bar}",
			nil,
		},
		{
			"${path_relative_to_include()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			".",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"child",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"child/sub-child",
			nil,
		},
		{
			"foo/${path_relative_to_include()}/bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"foo/./bar",
			nil,
		},
		{
			"foo/${path_relative_to_include()}/bar",
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"foo/child/bar",
			nil,
		},
		{
			"foo/${path_relative_to_include()}/bar/${path_relative_to_include()}",
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"foo/child/bar/child",
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"${    find_in_parent_folders()    }",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"../../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			"${find_in_parent_folders ()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidInterpolationSyntax("${find_in_parent_folders ()}"),
		},
		{
			"foo/${find_in_parent_folders()}/bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			fmt.Sprintf("foo/../../%s/bar", DefaultTerragruntConfigPath),
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			ParentFileNotFound{},
		},
		{
			"foo/${unknown}/bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidInterpolationSyntax("${unknown}"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.str, func(t *testing.T) {
			actualOut, actualErr := ResolveTerragruntConfigString(testCase.str, testCase.include, &testCase.terragruntOptions)
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
		terragruntOptions options.TerragruntOptions
		expectedOut       string
		expectedErr       error
	}{
		{
			"foo/${get_env()}/bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidGetEnvParams(""),
		},
		{
			"foo/${get_env(Invalid Parameters)}/bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidInterpolationSyntax("${get_env(Invalid Parameters)}"),
		},
		{
			"foo/${get_env('env','')}/bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidInterpolationSyntax("${get_env('env','')}"),
		},
		{
			`foo/${get_env("","")}/bar`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidGetEnvParams(`"",""`),
		},
		{
			`foo/${get_env(   ""    ,   ""    )}/bar`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidGetEnvParams(`   ""    ,   ""    `),
		},
		{
			`${get_env("SOME_VAR", "SOME{VALUE}")}`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"SOME{VALUE}",
			nil,
		},
		{
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT","")}/bar`,
			nil,
			options.TerragruntOptions{
				TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath,
				NonInteractive:       true,
				Env:                  map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"},
			},
			"foo//bar",
			nil,
		},
		{
			`foo/${get_env(    "TEST_ENV_TERRAGRUNT_HIT"   ,   ""   )}/bar`,
			nil,
			options.TerragruntOptions{
				TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath,
				NonInteractive:       true,
				Env:                  map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"},
			},
			"foo//bar",
			nil,
		},
		{
			`foo/${get_env("TEST_ENV_
TERRAGRUNT_HIT","")}/bar`,
			nil,
			options.TerragruntOptions{
				TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath,
				NonInteractive:       true,
				Env:                  map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"},
			},
			"",
			InvalidInterpolationSyntax(`${get_env("TEST_ENV_
TERRAGRUNT_HIT","")}`),
		},
		{
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT","DEFAULT")}/bar`,
			nil,
			options.TerragruntOptions{
				TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath,
				NonInteractive:       true,
				Env:                  map[string]string{"TEST_ENV_TERRAGRUNT_OTHER": "SOMETHING"},
			},
			"foo/DEFAULT/bar",
			nil,
		},
		{
			`foo/${get_env(    "TEST_ENV_TERRAGRUNT_HIT      "   ,   "default"   )}/bar`,
			nil,
			options.TerragruntOptions{
				TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath,
				NonInteractive:       true,
				Env:                  map[string]string{"TEST_ENV_TERRAGRUNT_HIT": "environment hit  "},
			},
			"foo/environment hit  /bar",
			nil,
		},
		{
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT","default")}/bar`,
			nil,
			options.TerragruntOptions{
				TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath,
				NonInteractive:       true,
				Env:                  map[string]string{"TEST_ENV_TERRAGRUNT_HIT": "HIT"},
			},
			"foo/HIT/bar",
			nil,
		},
		{
			// Unclosed quote
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT}/bar`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			"",
			InvalidInterpolationSyntax(`${get_env("TEST_ENV_TERRAGRUNT_HIT}`),
		},
		{
			// Unclosed quote and interpolation pattern
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT/bar`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			`foo/${get_env("TEST_ENV_TERRAGRUNT_HIT/bar`,
			nil,
		},
	}

	for _, testCase := range testCases {
		actualOut, actualErr := ResolveTerragruntConfigString(testCase.str, testCase.include, &testCase.terragruntOptions)
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
		terragruntOptions options.TerragruntOptions
		expectedOut       string
		expectedErr       error
	}{
		{
			`"${get_terraform_commands_that_need_locking()}"`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: DefaultTerragruntConfigPath, NonInteractive: true},
			util.CommaSeparatedStrings(TERRAFORM_COMMANDS_NEED_LOCKING),
			nil,
		},
		{
			`commands = ["${get_terraform_commands_that_need_vars()}"]`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: DefaultTerragruntConfigPath, NonInteractive: true},
			fmt.Sprintf("commands = [%s]", util.CommaSeparatedStrings(TERRAFORM_COMMANDS_NEED_VARS)),
			nil,
		},
		{
			`commands = "test-${get_terraform_commands_that_need_vars()}"`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: DefaultTerragruntConfigPath, NonInteractive: true},
			fmt.Sprintf(`commands = "test-%v"`, TERRAFORM_COMMANDS_NEED_VARS),
			nil,
		},
	}

	for _, testCase := range testCases {
		actualOut, actualErr := ResolveTerragruntConfigString(testCase.str, testCase.include, &testCase.terragruntOptions)
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
		terragruntOptions options.TerragruntOptions
		expectedOut       string
		expectedErr       error
	}{
		{
			`${get_env("NON_EXISTING_VAR1", "default1")}-${get_env("NON_EXISTING_VAR2", "default2")}`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: DefaultTerragruntConfigPath, NonInteractive: true},
			fmt.Sprintf("default1-default2"),
			nil,
		},
		{
			// Included within quotes
			`"${get_env("NON_EXISTING_VAR1", "default1")}-${get_env("NON_EXISTING_VAR2", "default2")}"`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: DefaultTerragruntConfigPath, NonInteractive: true},
			`"default1-default2"`,
			nil,
		},
		{
			// Malformed parameters
			`${get_env("NON_EXISTING_VAR1", "default"-${get_terraform_commands_that_need_vars()}`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: DefaultTerragruntConfigPath, NonInteractive: true},
			fmt.Sprintf(`${get_env("NON_EXISTING_VAR1", "default"-%v`, TERRAFORM_COMMANDS_NEED_VARS),
			nil,
		},
		{
			`test1 = "${get_env("NON_EXISTING_VAR1", "default")}" test2 = ["${get_terraform_commands_that_need_vars()}"]`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: DefaultTerragruntConfigPath, NonInteractive: true},
			fmt.Sprintf(`test1 = "default" test2 = [%v]`, util.CommaSeparatedStrings(TERRAFORM_COMMANDS_NEED_VARS)),
			nil,
		},
		{
			`${get_env("NON_EXISTING_VAR1", "default")}-${get_terraform_commands_that_need_vars()}`,
			nil,
			options.TerragruntOptions{TerragruntConfigPath: DefaultTerragruntConfigPath, NonInteractive: true},
			fmt.Sprintf("default-%v", TERRAFORM_COMMANDS_NEED_VARS),
			nil,
		},
	}

	for _, testCase := range testCases {
		actualOut, actualErr := ResolveTerragruntConfigString(testCase.str, testCase.include, &testCase.terragruntOptions)
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

func TestGetParentTfVarsDir(t *testing.T) {
	t.Parallel()

	currentDir, err := os.Getwd()
	assert.Nil(t, err, "Could not get current working dir: %v", err)
	parentDir := filepath.ToSlash(filepath.Dir(currentDir))

	testCases := []struct {
		include           *IncludeConfig
		terragruntOptions options.TerragruntOptions
		expectedPath      string
	}{
		{
			nil,
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			helpers.RootFolder + "child",
		},
		{
			&IncludeConfig{Path: "../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			helpers.RootFolder,
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			helpers.RootFolder,
		},
		{
			&IncludeConfig{Path: "../../../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			helpers.RootFolder,
		},
		{
			&IncludeConfig{Path: helpers.RootFolder + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			helpers.RootFolder,
		},
		{
			&IncludeConfig{Path: "../../other-child/" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: helpers.RootFolder + "child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			fmt.Sprintf("%s/other-child", filepath.VolumeName(parentDir)),
		},
		{
			&IncludeConfig{Path: "../../" + DefaultTerragruntConfigPath},
			options.TerragruntOptions{TerragruntConfigPath: "../child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			parentDir,
		},
		{
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/" + DefaultTerragruntConfigPath, NonInteractive: true},
			fmt.Sprintf("%s/test/fixture-parent-folders/terragrunt-in-root", parentDir),
		},
	}

	for _, testCase := range testCases {
		actualPath, actualErr := getParentTfVarsDir(testCase.include, &testCase.terragruntOptions)
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}
