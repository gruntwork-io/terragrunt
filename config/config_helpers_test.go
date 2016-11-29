package config

import (
	"testing"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/gruntwork-io/terragrunt/errors"
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
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			".",
		},
		{
			&IncludeConfig{Path: "../.terragrunt"},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"child",
		},
		{
			&IncludeConfig{Path: "/root/.terragrunt"},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"child",
		},
		{
			&IncludeConfig{Path: "../../../.terragrunt"},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/sub-child/sub-sub-child/.terragrunt", NonInteractive: true},
			"child/sub-child/sub-sub-child",
		},
		{
			&IncludeConfig{Path: "/root/.terragrunt"},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/sub-child/sub-sub-child/.terragrunt", NonInteractive: true},
			"child/sub-child/sub-sub-child",
		},
		{
			&IncludeConfig{Path: "../../other-child/.terragrunt"},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/sub-child/.terragrunt", NonInteractive: true},
			"../child/sub-child",
		},
		{
			&IncludeConfig{Path: "../../.terragrunt"},
			options.TerragruntOptions{TerragruntConfigPath: "../child/sub-child/.terragrunt", NonInteractive: true},
			"child/sub-child",
		},
		{
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/.terragrunt", NonInteractive: true},
			"child/sub-child",
		},
	}

	for _, testCase := range testCases {
		actualPath, actualErr := pathRelativeToInclude(testCase.include, &testCase.terragruntOptions)
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}

func TestFindInParentFolders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		terragruntOptions options.TerragruntOptions
		expectedPath      string
		expectedErr       error
	}{
		{
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/.terragrunt", NonInteractive: true},
			"../.terragrunt",
			nil,
		},
		{
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/.terragrunt", NonInteractive: true},
			"../../../.terragrunt",
			nil,
		},
		{
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/.terragrunt", NonInteractive: true},
			"",
			ParentTerragruntConfigNotFound("../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/.terragrunt"),
		},
		{
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/.terragrunt", NonInteractive: true},
			"../.terragrunt",
			nil,
		},
		{
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/.terragrunt", NonInteractive: true},
			"../.terragrunt",
			nil,
		},
		{
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/sub-sub-child/.terragrunt", NonInteractive: true},
			"../.terragrunt",
			nil,
		},
		{
			options.TerragruntOptions{TerragruntConfigPath: "/", NonInteractive: true},
			"",
			ParentTerragruntConfigNotFound("/"),
		},
		{
			options.TerragruntOptions{TerragruntConfigPath: "/fake/path", NonInteractive: true},
			"",
			ParentTerragruntConfigNotFound("/fake/path"),
		},
	}

	for _, testCase := range testCases {
		actualPath, actualErr := findInParentFolders(&testCase.terragruntOptions)
		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "For options %v, expected error %v but got error %v", testCase.terragruntOptions, testCase.expectedErr, actualErr)
		} else {
			assert.Nil(t, actualErr, "For options %v, unexpected error: %v", testCase.terragruntOptions, actualErr)
			assert.Equal(t, testCase.expectedPath, actualPath, "For options %v", testCase.terragruntOptions)
		}
	}
}

func TestResolveTerragruntInterpolation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		str               string
		include           *IncludeConfig
		terragruntOptions options.TerragruntOptions
		expectedOut       string
		expectedErr       error
	}{
		{
			"${path_relative_to_include()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			".",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "../.terragrunt"},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"child",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/.terragrunt", NonInteractive: true},
			"child/sub-child",
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/.terragrunt", NonInteractive: true},
			"../../.terragrunt",
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/.terragrunt", NonInteractive: true},
			"",
			ParentTerragruntConfigNotFound("../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/.terragrunt"),
		},
		{
			"${find_in_parent_folders}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"",
			InvalidInterpolationSyntax("${find_in_parent_folders}"),
		},
		{
			"{find_in_parent_folders()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"",
			InvalidInterpolationSyntax("{find_in_parent_folders()}"),
		},
		{
			"find_in_parent_folders()",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"",
			InvalidInterpolationSyntax("find_in_parent_folders()"),
		},
	}

	for _, testCase := range testCases {
		actualOut, actualErr := resolveTerragruntInterpolation(testCase.str, testCase.include, &testCase.terragruntOptions)
		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "For string '%s' include %v and options %v, expected error %v but got error %v", testCase.str, testCase.include, testCase.terragruntOptions, testCase.expectedErr, actualErr)
		} else {
			assert.Nil(t, actualErr, "For string '%s' include %v and options %v, unexpected error: %v", testCase.str, testCase.include, testCase.terragruntOptions, actualErr)
			assert.Equal(t, testCase.expectedOut, actualOut, "For string '%s' include %v and options %v", testCase.str, testCase.include, testCase.terragruntOptions)
		}
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
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"",
			nil,
		},
		{
			"foo bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"foo bar",
			nil,
		},
		{
			"$foo {bar}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"$foo {bar}",
			nil,
		},
		{
			"${path_relative_to_include()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			".",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "../.terragrunt"},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"child",
			nil,
		},
		{
			"${path_relative_to_include()}",
			&IncludeConfig{Path: "${find_in_parent_folders()}"},
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/.terragrunt", NonInteractive: true},
			"child/sub-child",
			nil,
		},
		{
			"foo/${path_relative_to_include()}/bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"foo/./bar",
			nil,
		},
		{
			"foo/${path_relative_to_include()}/bar",
			&IncludeConfig{Path: "../.terragrunt"},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"foo/child/bar",
			nil,
		},
		{
			"foo/${path_relative_to_include()}/bar/${path_relative_to_include()}",
			&IncludeConfig{Path: "../.terragrunt"},
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"foo/child/bar/child",
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/.terragrunt", NonInteractive: true},
			"../../.terragrunt",
			nil,
		},
		{
			"foo/${find_in_parent_folders()}/bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/.terragrunt", NonInteractive: true},
			"foo/../../.terragrunt/bar",
			nil,
		},
		{
			"${find_in_parent_folders()}",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/.terragrunt", NonInteractive: true},
			"",
			ParentTerragruntConfigNotFound("../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/.terragrunt"),
		},
		{
			"foo/${unknown}/bar",
			nil,
			options.TerragruntOptions{TerragruntConfigPath: "/root/child/.terragrunt", NonInteractive: true},
			"",
			InvalidInterpolationSyntax("${unknown}"),
		},
	}

	for _, testCase := range testCases {
		actualOut, actualErr := ResolveTerragruntConfigString(testCase.str, testCase.include, &testCase.terragruntOptions)
		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "For string '%s' include %v and options %v, expected error %v but got error %v", testCase.str, testCase.include, testCase.terragruntOptions, testCase.expectedErr, actualErr)
		} else {
			assert.Nil(t, actualErr, "For string '%s' include %v and options %v, unexpected error: %v", testCase.str, testCase.include, testCase.terragruntOptions, actualErr)
			assert.Equal(t, testCase.expectedOut, actualOut, "For string '%s' include %v and options %v", testCase.str, testCase.include, testCase.terragruntOptions)
		}
	}
}