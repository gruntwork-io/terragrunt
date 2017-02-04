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
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/terraform.tfvars")),
	}

	configPaths := []string{"../test/fixture-modules/module-a/terraform.tfvars"}
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
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/terraform.tfvars")),
	}

	configPaths := []string{"../test/fixture-modules/module-b/module-b-child/terraform.tfvars"}
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
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/terraform.tfvars")),
	}

	moduleC := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-c"),
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/terraform.tfvars")),
	}

	configPaths := []string{"../test/fixture-modules/module-a/terraform.tfvars", "../test/fixture-modules/module-c/terraform.tfvars"}
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
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/terraform.tfvars")),
	}

	moduleB := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-b-child"),
			RemoteState: state(t, "bucket", "module-b-child/terraform.tfstate"),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/terraform.tfvars")),
	}

	moduleC := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-c"),
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/terraform.tfvars")),
	}

	moduleD := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-d"),
		Dependencies: []*TerraformModule{moduleA, moduleB, moduleC},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a", "../module-b/module-b-child", "../module-c"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-d/terraform.tfvars")),
	}

	configPaths := []string{"../test/fixture-modules/module-a/terraform.tfvars", "../test/fixture-modules/module-b/module-b-child/terraform.tfvars", "../test/fixture-modules/module-c/terraform.tfvars", "../test/fixture-modules/module-d/terraform.tfvars"}
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
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/terraform.tfvars")),
	}

	moduleB := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-b-child"),
			RemoteState: state(t, "bucket", "module-b-child/terraform.tfstate"),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/terraform.tfvars")),
	}

	moduleE := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-e/module-e-child"),
		Dependencies: []*TerraformModule{moduleA, moduleB},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-e-child"),
			RemoteState: state(t, "bucket", "module-e-child/terraform.tfstate"),
			Dependencies: &config.ModuleDependencies{Paths: []string{"../../module-a", "../../module-b/module-b-child"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-e/module-e-child/terraform.tfvars")),
	}


	configPaths := []string{"../test/fixture-modules/module-a/terraform.tfvars", "../test/fixture-modules/module-b/module-b-child/terraform.tfvars", "../test/fixture-modules/module-e/module-e-child/terraform.tfvars"}
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
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-f/terraform.tfvars")),
		AssumeAlreadyApplied: true,
	}

	moduleG := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-g"),
		Dependencies: []*TerraformModule{moduleF},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-f"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-g/terraform.tfvars")),
	}

	configPaths := []string{"../test/fixture-modules/module-g/terraform.tfvars"}
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
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-h/terraform.tfvars")),
		AssumeAlreadyApplied: true,
	}

	moduleI := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-i"),
		Dependencies: []*TerraformModule{moduleH},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-h"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-i/terraform.tfvars")),
		AssumeAlreadyApplied: true,
	}

	moduleJ := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-j"),
		Dependencies: []*TerraformModule{moduleI},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-i"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-j/terraform.tfvars")),
	}

	moduleK := &TerraformModule{
		Path: canonical(t, "../test/fixture-modules/module-k"),
		Dependencies: []*TerraformModule{moduleH},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-h"}},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-k/terraform.tfvars")),
	}

	configPaths := []string{"../test/fixture-modules/module-j/terraform.tfvars", "../test/fixture-modules/module-k/terraform.tfvars"}
	expected := []*TerraformModule{moduleH, moduleI, moduleJ, moduleK}

	actualModules, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesInvalidPaths(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../test/fixture-modules/module-missing-dependency/terraform.tfvars"}

	_, actualErr := ResolveTerraformModules(configPaths, mockOptions)
	if assert.NotNil(t, actualErr, "Unexpected error: %v", actualErr) {
		unwrapped := errors.Unwrap(actualErr)
		assert.True(t, os.IsNotExist(unwrapped), "Expected a file not exists error but got %v", unwrapped)
	}
}