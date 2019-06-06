package configstack

import (
	"fmt"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

var mockHowThesePathsWereFound = "mock-values-for-test"

func TestResolveTerraformModulesNoPaths(t *testing.T) {
	t.Parallel()

	configPaths := []string{}
	expected := []*TerraformModule{}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              canonical(t, "../test/fixture-modules/module-a"),
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("test")}},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleA}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			RemoteState: state(t, "bucket", "module-b-child/terraform.tfstate"),
			Terraform:   &config.TerraformConfig{Source: ptr("...")},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleB}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("test")},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleA, moduleC}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../test/fixture-modules/module-a")}

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("test")},
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}

	actualModules, actualErr := ResolveTerraformModules(configPaths, opts, mockHowThesePathsWereFound)

	// construct the expected list
	moduleA.FlagExcluded = true
	expected := []*TerraformModule{moduleA, moduleC}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../test/fixture-modules/module-c")}

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("test")},
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}

	actualModules, actualErr := ResolveTerraformModules(configPaths, opts, mockHowThesePathsWereFound)

	// construct the expected list
	moduleC.FlagExcluded = true
	expected := []*TerraformModule{moduleA, moduleC}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../test/fixture-modules/module-c")}

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("test")},
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}

	actualModules, actualErr := ResolveTerraformModules(configPaths, opts, mockHowThesePathsWereFound)

	// construct the expected list
	moduleA.FlagExcluded = false
	expected := []*TerraformModule{moduleA, moduleC}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../test/fixture-modules/module-a")}

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("test")},
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}

	actualModules, actualErr := ResolveTerraformModules(configPaths, opts, mockHowThesePathsWereFound)

	// construct the expected list
	moduleC.FlagExcluded = true
	expected := []*TerraformModule{moduleA, moduleC}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithDependencyExcludeModuleWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../test/fixture-modules/module-c"), canonical(t, "../test/fixture-modules/module-f")}
	opts.ExcludeDirs = []string{canonical(t, "../test/fixture-modules/module-f")}

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("test")},
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleF := &TerraformModule{
		Path:                 canonical(t, "../test/fixture-modules/module-f"),
		Dependencies:         []*TerraformModule{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-f/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: false,
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-f/" + config.DefaultTerragruntConfigPath}

	actualModules, actualErr := ResolveTerraformModules(configPaths, opts, mockHowThesePathsWereFound)

	// construct the expected list
	moduleF.FlagExcluded = true
	expected := []*TerraformModule{moduleA, moduleC, moduleF}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              canonical(t, "../test/fixture-modules/module-a"),
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("test")}},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			RemoteState: state(t, "bucket", "module-b-child/terraform.tfstate"),
			Terraform:   &config.TerraformConfig{Source: ptr("...")},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleD := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-d"),
		Dependencies: []*TerraformModule{moduleA, moduleB, moduleC},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a", "../module-b/module-b-child", "../module-c"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-d/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-d/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleA, moduleB, moduleC, moduleD}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithDependenciesWithIncludes(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              canonical(t, "../test/fixture-modules/module-a"),
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("test")}},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			RemoteState: state(t, "bucket", "module-b-child/terraform.tfstate"),
			Terraform:   &config.TerraformConfig{Source: ptr("...")},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	moduleE := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-e/module-e-child"),
		Dependencies: []*TerraformModule{moduleA, moduleB},
		Config: config.TerragruntConfig{
			RemoteState:  state(t, "bucket", "module-e-child/terraform.tfstate"),
			Dependencies: &config.ModuleDependencies{Paths: []string{"../../module-a", "../../module-b/module-b-child"}},
			Terraform:    &config.TerraformConfig{Source: ptr("test")},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-e/module-e-child/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-e/module-e-child/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleA, moduleB, moduleE}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithExternalDependencies(t *testing.T) {
	t.Parallel()

	moduleF := &TerraformModule{
		Path:                 canonical(t, "../test/fixture-modules/module-f"),
		Dependencies:         []*TerraformModule{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-f/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: false,
	}

	moduleG := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-g"),
		Dependencies: []*TerraformModule{moduleF},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-f"}},
			Terraform:    &config.TerraformConfig{Source: ptr("test")},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-g/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-g/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleF, moduleG}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithNestedExternalDependencies(t *testing.T) {
	t.Parallel()

	moduleH := &TerraformModule{
		Path:                 canonical(t, "../test/fixture-modules/module-h"),
		Dependencies:         []*TerraformModule{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-h/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: false,
	}

	moduleI := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-i"),
		Dependencies: []*TerraformModule{moduleH},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-h"}},
		},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-i/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: false,
	}

	moduleJ := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-j"),
		Dependencies: []*TerraformModule{moduleI},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-i"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-j/"+config.DefaultTerragruntConfigPath)),
	}

	moduleK := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-k"),
		Dependencies: []*TerraformModule{moduleH},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-h"}},
			Terraform:    &config.TerraformConfig{Source: ptr("fire")},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-k/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-j/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-k/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleH, moduleI, moduleJ, moduleK}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesInvalidPaths(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../test/fixture-modules/module-missing-dependency/" + config.DefaultTerragruntConfigPath}

	_, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	if assert.NotNil(t, actualErr, "Unexpected error: %v", actualErr) {
		underlying, ok := errors.Unwrap(actualErr).(ErrorProcessingModule)
		if assert.True(t, ok) {
			unwrapped := errors.Unwrap(underlying.UnderlyingError)
			assert.True(t, os.IsNotExist(unwrapped), "Expected a file not exists error but got %v", underlying.UnderlyingError)
		}
	}
}

func TestResolveTerraformModuleNoTerraformConfig(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../test/fixture-modules/module-l/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestGetTerragruntSourceForModuleHappyPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		config   *config.TerragruntConfig
		opts     *options.TerragruntOptions
		expected string
	}{
		{mockConfigWithSource(""), mockOptionsWithSource(t, ""), ""},
		{mockConfigWithSource(""), mockOptionsWithSource(t, "/source/modules"), ""},
		{mockConfigWithSource("git::git@github.com:acme/modules.git//foo/bar"), mockOptionsWithSource(t, "/source/modules"), "/source/modules//foo/bar"},
		{mockConfigWithSource("git::git@github.com:acme/modules.git//foo/bar?ref=v0.0.1"), mockOptionsWithSource(t, "/source/modules"), "/source/modules//foo/bar"},
		{mockConfigWithSource("git::git@github.com:acme/emr_cluster.git?ref=feature/fix_bugs"), mockOptionsWithSource(t, "/source/modules"), "/source/modules//emr_cluster"},
		{mockConfigWithSource("git::ssh://git@ghe.ourcorp.com/OurOrg/some-module.git"), mockOptionsWithSource(t, "/source/modules"), "/source/modules//some-module"},
		{mockConfigWithSource("github.com/hashicorp/example"), mockOptionsWithSource(t, "/source/modules"), "/source/modules//example"},
		{mockConfigWithSource("github.com/hashicorp/example//subdir"), mockOptionsWithSource(t, "/source/modules"), "/source/modules//subdir"},
		{mockConfigWithSource("git@github.com:hashicorp/example.git//subdir"), mockOptionsWithSource(t, "/source/modules"), "/source/modules//subdir"},
		{mockConfigWithSource("./some/path//to/modulename"), mockOptionsWithSource(t, "/source/modules"), "/source/modules//to/modulename"},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("%v-%s", testCase.config.Terraform.Source, testCase.opts.Source), func(t *testing.T) {
			actual, err := getTerragruntSourceForModule("mock-for-test", testCase.config, testCase.opts)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func mockOptionsWithSource(t *testing.T, sourceUrl string) *options.TerragruntOptions {
	opts, err := options.NewTerragruntOptionsForTest("mock-for-test.hcl")
	if err != nil {
		t.Fatalf("Error creating terragrunt options for test %v", err)
	}
	opts.Source = sourceUrl
	return opts
}

func mockConfigWithSource(sourceUrl string) *config.TerragruntConfig {
	cfg := config.TerragruntConfig{}
	cfg.Terraform = &config.TerraformConfig{Source: &sourceUrl}
	return &cfg
}

func ptr(str string) *string {
	return &str
}