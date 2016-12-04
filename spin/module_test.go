package spin

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"os"
)

func TestResolveTerraformModulesNoPaths(t *testing.T) {
	t.Parallel()

	configPaths := []string{}
	expected := []*TerraformModule{}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/.terragrunt")),
	}

	configPaths := []string{"../test/fixture-modules/module-a/.terragrunt"}
	expected := []*TerraformModule{moduleA}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleB := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-b-child"),
			RemoteState: state(t, "bucket", "module-b-child/terraform.tfstate"),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/.terragrunt")),
	}

	configPaths := []string{"../test/fixture-modules/module-b/module-b-child/.terragrunt"}
	expected := []*TerraformModule{moduleB}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/.terragrunt")),
	}

	moduleC := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-c"),
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/.terragrunt")),
	}

	configPaths := []string{"../test/fixture-modules/module-a/.terragrunt", "../test/fixture-modules/module-c/.terragrunt"}
	expected := []*TerraformModule{moduleA, moduleC}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/.terragrunt")),
	}

	moduleB := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-b-child"),
			RemoteState: state(t, "bucket", "module-b-child/terraform.tfstate"),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/.terragrunt")),
	}

	moduleC := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-c"),
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/.terragrunt")),
	}

	moduleD := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-d"),
		Dependencies: []*TerraformModule{moduleA, moduleB, moduleC},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a", "../module-b/module-b-child", "../module-c"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-d/.terragrunt")),
	}

	configPaths := []string{"../test/fixture-modules/module-a/.terragrunt", "../test/fixture-modules/module-b/module-b-child/.terragrunt", "../test/fixture-modules/module-c/.terragrunt", "../test/fixture-modules/module-d/.terragrunt"}
	expected := []*TerraformModule{moduleA, moduleB, moduleC, moduleD}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithDependenciesWithIncludes(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/.terragrunt")),
	}

	moduleB := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-b-child"),
			RemoteState: state(t, "bucket", "module-b-child/terraform.tfstate"),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/.terragrunt")),
	}

	moduleE := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-e/module-e-child"),
		Dependencies: []*TerraformModule{moduleA, moduleB},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-e-child"),
			RemoteState: state(t, "bucket", "module-e-child/terraform.tfstate"),
			Dependencies: &config.ModuleDependencies{Paths: []string{"../../module-a", "../../module-b/module-b-child"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-e/module-e-child/.terragrunt")),
	}


	configPaths := []string{"../test/fixture-modules/module-a/.terragrunt", "../test/fixture-modules/module-b/module-b-child/.terragrunt", "../test/fixture-modules/module-e/module-e-child/.terragrunt"}
	expected := []*TerraformModule{moduleA, moduleB, moduleE}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithExternalDependencies(t *testing.T) {
	t.Parallel()

	moduleF := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-f"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-f/.terragrunt")),
		AssumeAlreadyApplied: true,
	}

	moduleG := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-g"),
		Dependencies: []*TerraformModule{moduleF},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-f"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-g/.terragrunt")),
	}

	configPaths := []string{"../test/fixture-modules/module-g/.terragrunt"}
	expected := []*TerraformModule{moduleF, moduleG}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithNestedExternalDependencies(t *testing.T) {
	t.Parallel()

	moduleH := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-h"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-h/.terragrunt")),
		AssumeAlreadyApplied: true,
	}

	moduleI := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-i"),
		Dependencies: []*TerraformModule{moduleH},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-h"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-i/.terragrunt")),
		AssumeAlreadyApplied: true,
	}

	moduleJ := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-j"),
		Dependencies: []*TerraformModule{moduleI},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-i"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-j/.terragrunt")),
	}

	moduleK := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-k"),
		Dependencies: []*TerraformModule{moduleH},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-h"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-k/.terragrunt")),
	}

	configPaths := []string{"../test/fixture-modules/module-j/.terragrunt", "../test/fixture-modules/module-k/.terragrunt"}
	expected := []*TerraformModule{moduleH, moduleI, moduleJ, moduleK}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesInvalidPaths(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../test/fixture-modules/module-missing-dependency/.terragrunt"}

	_, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	if assert.NotNil(t, actualErr, "Unexpected error: %v", actualErr) {
		unwrapped := errors.Unwrap(actualErr)
		assert.True(t, os.IsNotExist(unwrapped), "Expected a file not exists error but got %v", unwrapped)
	}
}