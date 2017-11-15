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

func TestInterpolatePathRelativeToInclude(t *testing.T) {
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
		ti := TerragruntInterpolation{Options:  testCase.terragruntOptions, include: testCase.include}
		fn := ti.interpolatePathRelativeToInclude()
		actualPath, actualErr := fn.Callback([]interface{}{})
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}

func TestInterpolatePathRelativeFromInclude(t *testing.T) {
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
		ti := TerragruntInterpolation{include: testCase.include, Options: testCase.terragruntOptions}
		fn := ti.interpolatePathRelativeFromInclude()
		actualPath, actualErr := fn.Callback([]interface{}{})
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}

func TestInterpolateFindInParentFolders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		params            []string
		terragruntOptions *options.TerragruntOptions
		expectedPath      string
		expectedErr       error
	}{
		{
			[]string{},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/"+DefaultTerragruntConfigPath),
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			[]string{},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath),
			"../../../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			[]string{},
			terragruntOptionsForTestWithMaxFolders(t, "../test/fixture-parent-folders/no-terragrunt-in-root/child/sub-child/"+DefaultTerragruntConfigPath, 3),
			"",
			ParentFileNotFound{},
		},
		{
			[]string{},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/"+DefaultTerragruntConfigPath),
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			[]string{},
			terragruntOptionsForTest(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+DefaultTerragruntConfigPath),
			"../" + DefaultTerragruntConfigPath,
			nil,
		},
		{
			[]string{},
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
			[]string{},
			terragruntOptionsForTest(t, "/"),
			"",
			ParentFileNotFound{},
		},
		{
			[]string{},
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
			ti := TerragruntInterpolation{Options: testCase.terragruntOptions}

			fn := ti.interpolateFindInParentFolders()
			params := make([]interface{}, len(testCase.params))
			for i,val := range testCase.params {
				params[i] = val
			}
			actualPath, actualErr := fn.Callback(params)
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

func TestInterpolateResolveTerragruntInterpolation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		str               string
		include           *IncludeConfig
		terragruntOptions *options.TerragruntOptions
		expectedOut       interface{}
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
		{
			`${import_parent_tree("*.tfvars")}`,
			nil,
			&options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/tfvar-tree/child/sub-child", NonInteractive: true},
			[]string{
				"-var-file=/Users/simas/go/src/github.com/gruntwork-io/terragrunt/test/fixture-parent-folders/tfvar-tree/terraform.tfvars",
				"-var-file=/Users/simas/go/src/github.com/gruntwork-io/terragrunt/test/fixture-parent-folders/tfvar-tree/variables.tfvars",
			},
			nil,
		},
		{
			`${import_parent_tree()}`,
			nil,
			&options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/tfvar-tree/child/sub-child", NonInteractive: true},
			[]string{},
			nil,
		},
		{
			`${import_parent_tree("versions.tfvars")}`,
			nil,
			&options.TerragruntOptions{TerragruntConfigPath: "../test/fixture-parent-folders/tfvar-tree/child/sub-child", NonInteractive: true},
			[]string{
				"-var-file=/Users/simas/go/src/github.com/gruntwork-io/terragrunt/test/fixture-parent-folders/tfvar-tree/terraform.tfvars",
				"-var-file=/Users/simas/go/src/github.com/gruntwork-io/terragrunt/test/fixture-parent-folders/tfvar-tree/variables.tfvars",
			},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("%s--%s", testCase.str, testCase.terragruntOptions.TerragruntConfigPath), func(t *testing.T) {
			ti := TerragruntInterpolation{include: testCase.include, Options: testCase.terragruntOptions}
			actualOut, actualErr := ti.ResolveTerragruntConfig(testCase.str)
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

func TestInterpolateResolveMultipleInterpolationsConfigString(t *testing.T) {
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

func TestInterpolateGetTfVarsDirAbsPath(t *testing.T) {
	t.Parallel()
	workingDir, err := os.Getwd()
	assert.Nil(t, err, "Could not get current working dir: %v", err)
	testInterpolateGetTfVarsDir(t, "/foo/bar/terraform.tfvars", fmt.Sprintf("%s/foo/bar", filepath.VolumeName(workingDir)))
}

func TestInterpolateGetTfVarsDirRelPath(t *testing.T) {
	t.Parallel()
	workingDir, err := os.Getwd()
	assert.Nil(t, err, "Could not get current working dir: %v", err)
	workingDir = filepath.ToSlash(workingDir)

	testInterpolateGetTfVarsDir(t, "foo/bar/terraform.tfvars", fmt.Sprintf("%s/foo/bar", workingDir))
}

func testInterpolateGetTfVarsDir(t *testing.T, configPath string, expectedPath string) {
	terragruntOptions, err := options.NewTerragruntOptionsForTest(configPath)
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	ti := TerragruntInterpolation{Options: terragruntOptions}
	actualPath, err := ti.getTfVarsDir()

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expectedPath, actualPath)
}

func TestInterpolateGetParentTfVarsDir(t *testing.T) {
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
		ti := TerragruntInterpolation{include: testCase.include, Options: testCase.terragruntOptions}
		actualPath, actualErr := ti.getParentTfVarsDir()
		assert.Nil(t, actualErr, "For include %v and options %v, unexpected error: %v", testCase.include, testCase.terragruntOptions, actualErr)
		assert.Equal(t, testCase.expectedPath, actualPath, "For include %v and options %v", testCase.include, testCase.terragruntOptions)
	}
}
