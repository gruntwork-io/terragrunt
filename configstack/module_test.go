package configstack

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

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
		Config:            config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("test")}, IsPartial: true},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleA}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneJsonModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              canonical(t, "../test/fixture-modules/json-module-a"),
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("test")}, IsPartial: true},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-a/"+config.DefaultTerragruntJsonConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/json-module-a/" + config.DefaultTerragruntJsonConfigPath}
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
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/module-b/terragrunt.hcl")},
			},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleB}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneJsonModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/json-module-b/terragrunt.hcl")},
			},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-b/module-b-child/"+config.DefaultTerragruntJsonConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/json-module-b/module-b-child/" + config.DefaultTerragruntJsonConfigPath}
	expected := []*TerraformModule{moduleB}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneHclModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/hcl-module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/hcl-module-b/terragrunt.hcl.json")},
			},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/hcl-module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/hcl-module-b/module-b-child/" + config.DefaultTerragruntConfigPath}
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
			IsPartial: true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleA, moduleC}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesJsonModulesWithHclDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("test")},
			IsPartial: true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-c/"+config.DefaultTerragruntJsonConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/json-module-c/" + config.DefaultTerragruntJsonConfigPath}
	expected := []*TerraformModule{moduleA, moduleC}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesHclModulesWithJsonDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("test")},
			IsPartial: true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-a/"+config.DefaultTerragruntJsonConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/hcl-module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../json-module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/hcl-module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/json-module-a/" + config.DefaultTerragruntJsonConfigPath, "../test/fixture-modules/hcl-module-c/" + config.DefaultTerragruntConfigPath}
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
			IsPartial: true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
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

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependencyAndConflictingNaming(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../test/fixture-modules/module-a")}

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("test")},
			IsPartial: true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleAbba := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-abba"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-abba/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-abba/" + config.DefaultTerragruntConfigPath}

	actualModules, actualErr := ResolveTerraformModules(configPaths, opts, mockHowThesePathsWereFound)

	// construct the expected list
	moduleA.FlagExcluded = true
	expected := []*TerraformModule{moduleA, moduleC, moduleAbba}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependencyAndConflictingNamingAndGlob(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../test/fixture-modules/module-a*")}

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("test")},
			IsPartial: true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleAbba := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-abba"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-abba/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-abba/" + config.DefaultTerragruntConfigPath}

	actualModules, actualErr := ResolveTerraformModules(configPaths, opts, mockHowThesePathsWereFound)

	// construct the expected list
	moduleA.FlagExcluded = true
	moduleAbba.FlagExcluded = true
	expected := []*TerraformModule{moduleA, moduleC, moduleAbba}

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
			IsPartial: true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
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
			IsPartial: true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
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
			IsPartial: true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
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
			IsPartial: true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleF := &TerraformModule{
		Path:                 canonical(t, "../test/fixture-modules/module-f"),
		Dependencies:         []*TerraformModule{},
		Config:               config.TerragruntConfig{IsPartial: true},
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
		Config:            config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("test")}, IsPartial: true},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/module-b/terragrunt.hcl")},
			},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleD := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-d"),
		Dependencies: []*TerraformModule{moduleA, moduleB, moduleC},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a", "../module-b/module-b-child", "../module-c"}},
			IsPartial:    true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-d/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-d/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleA, moduleB, moduleC, moduleD}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithMixedDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              canonical(t, "../test/fixture-modules/module-a"),
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("test")}, IsPartial: true},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/json-module-b/terragrunt.hcl")},
			},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-b/module-b-child/"+config.DefaultTerragruntJsonConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleD := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-d"),
		Dependencies: []*TerraformModule{moduleA, moduleB, moduleC},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a", "../json-module-b/module-b-child", "../module-c"}},
			IsPartial:    true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-d/"+config.DefaultTerragruntJsonConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/json-module-b/module-b-child/" + config.DefaultTerragruntJsonConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/json-module-d/" + config.DefaultTerragruntJsonConfigPath}
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
		Config:            config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("test")}, IsPartial: true},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/module-b/terragrunt.hcl")},
			},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	moduleE := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-e/module-e-child"),
		Dependencies: []*TerraformModule{moduleA, moduleB},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../../module-a", "../../module-b/module-b-child"}},
			Terraform:    &config.TerraformConfig{Source: ptr("test")},
			IsPartial:    true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/module-e/terragrunt.hcl")},
			},
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
		Config:               config.TerragruntConfig{IsPartial: true},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-f/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: true,
	}

	moduleG := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-g"),
		Dependencies: []*TerraformModule{moduleF},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-f"}},
			Terraform:    &config.TerraformConfig{Source: ptr("test")},
			IsPartial:    true,
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
		Config:               config.TerragruntConfig{IsPartial: true},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-h/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: true,
	}

	moduleI := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-i"),
		Dependencies: []*TerraformModule{moduleH},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-h"}},
			IsPartial:    true,
		},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-i/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: true,
	}

	moduleJ := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-j"),
		Dependencies: []*TerraformModule{moduleI},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-i"}},
			Terraform:    &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:    true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-j/"+config.DefaultTerragruntConfigPath)),
	}

	moduleK := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-k"),
		Dependencies: []*TerraformModule{moduleH},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-h"}},
			Terraform:    &config.TerraformConfig{Source: ptr("fire")},
			IsPartial:    true,
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-k/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-j/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-k/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{moduleH, moduleI, moduleJ, moduleK}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	require.NoError(t, actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesInvalidPaths(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../test/fixture-modules/module-missing-dependency/" + config.DefaultTerragruntConfigPath}

	_, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	require.Error(t, actualErr)

	underlying, ok := errors.Unwrap(actualErr).(ErrorProcessingModule)
	require.True(t, ok)

	unwrapped := errors.Unwrap(underlying.UnderlyingError)
	assert.True(t, os.IsNotExist(unwrapped), "Expected a file not exists error but got %v", underlying.UnderlyingError)
}

func TestResolveTerraformModuleNoTerraformConfig(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../test/fixture-modules/module-l/" + config.DefaultTerragruntConfigPath}
	expected := []*TerraformModule{}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions, mockHowThesePathsWereFound)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func ptr(str string) *string {
	return &str
}

func TestLogReductionHook(t *testing.T) {
	t.Parallel()
	var hook = NewForceLogLevelHook(logrus.ErrorLevel)

	stdout := bytes.Buffer{}

	var testLogger = logrus.New()
	testLogger.Out = &stdout
	testLogger.AddHook(hook)
	testLogger.Level = logrus.DebugLevel

	logrus.NewEntry(testLogger).Info("Test tomato")
	logrus.NewEntry(testLogger).Error("666 potato 111")

	out := stdout.String()

	var firstLogEntry = ""
	var secondLogEntry = ""

	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "tomato") {
			firstLogEntry = line
			continue
		}
		if strings.Contains(line, "potato") {
			secondLogEntry = line
			continue
		}
	}
	// check that both entries got logged with error level
	assert.Contains(t, firstLogEntry, "level=error")
	assert.Contains(t, secondLogEntry, "level=error")

}
