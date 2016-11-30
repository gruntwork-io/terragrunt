package spin

import (
	"testing"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/gruntwork-io/terragrunt/config"
)

func TestResolveTerraformModules(t *testing.T) {
	t.Parallel()

	mockOptions := options.NewTerragruntOptionsForTest("TestResolveTerraformModules")

	moduleA := TerraformModule{
		Path: abs(t, "../test/fixture-modules/module-a"),
		Dependencies: []TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions.Clone("../test/fixture-modules/module-a/.terragrunt"),
	}

	moduleB := TerraformModule{
		Path: abs(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: []TerraformModule{},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-b-child"),
			RemoteState: state(t, "bucket", "module-b-child/terraform.tfstate"),
		},
		TerragruntOptions: mockOptions.Clone("../test/fixture-modules/module-b/module-b-child/.terragrunt"),
	}

	moduleC := TerraformModule{
		Path: abs(t, "../test/fixture-modules/module-c"),
		Dependencies: []TerraformModule{moduleA},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-c"),
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a"}},
		},
		TerragruntOptions: mockOptions.Clone("../test/fixture-modules/module-c/.terragrunt"),
	}

	moduleD := TerraformModule{
		Path: abs(t, "../test/fixture-modules/module-d"),
		Dependencies: []TerraformModule{moduleA, moduleB, moduleC},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../module-a", "../module-b/module-b-child", "../module-c"}},
		},
		TerragruntOptions: mockOptions.Clone("../test/fixture-modules/module-d/.terragrunt"),
	}

	moduleE := TerraformModule{
		Path: abs(t, "../test/fixture-modules/module-e/module-e-child"),
		Dependencies: []TerraformModule{moduleA, moduleB},
		Config: config.TerragruntConfig{
			Lock: lock(t, "module-e-child"),
			RemoteState: state(t, "bucket", "module-e-child/terraform.tfstate"),
			Dependencies: &config.ModuleDependencies{Paths: []string{"../../module-a", "../../module-b/module-b-child"}},
		},
		TerragruntOptions: mockOptions.Clone("../test/fixture-modules/module-e/module-e-child/.terragrunt"),
	}

	testCases := []struct {
		terragruntConfigPaths []string
		expectedModules       []TerraformModule
		expectedErr           error
	}{
		{
			[]string{},
			[]TerraformModule{},
			nil,
		},
		{
			[]string{"../test/fixture-modules/module-a/.terragrunt"},
			[]TerraformModule{moduleA},
			nil,
		},
		{
			[]string{"../test/fixture-modules/module-b/module-b-child/.terragrunt"},
			[]TerraformModule{moduleB},
			nil,
		},
		{
			[]string{"../test/fixture-modules/module-a/.terragrunt", "../test/fixture-modules/module-c/.terragrunt"},
			[]TerraformModule{moduleA, moduleC},
			nil,
		},
		{
			[]string{"../test/fixture-modules/module-a/.terragrunt", "../test/fixture-modules/module-b/module-b-child/.terragrunt", "../test/fixture-modules/module-c/.terragrunt", "../test/fixture-modules/module-d/.terragrunt"},
			[]TerraformModule{moduleA, moduleB, moduleC, moduleD},
			nil,
		},
		{
			[]string{"../test/fixture-modules/module-a/.terragrunt", "../test/fixture-modules/module-b/module-b-child/.terragrunt", "../test/fixture-modules/module-e/module-e-child/.terragrunt"},
			[]TerraformModule{moduleA, moduleB, moduleE},
			nil,
		},
		{
			[]string{"../test/fixture-modules/module-c/.terragrunt"},
			[]TerraformModule{},
			UnrecognizedDependency{
				ModulePath: abs(t, "../test/fixture-modules/module-c"),
				DependencyPath: "../module-a",
				TerragruntConfigPaths: []string{"../test/fixture-modules/module-c/.terragrunt"},
			},
		},
		{
			[]string{"../test/fixture-modules/module-missing-dependency/.terragrunt"},
			[]TerraformModule{},
			UnrecognizedDependency{
				ModulePath: abs(t, "../test/fixture-modules/module-missing-dependency"),
				DependencyPath: "../not-a-real-dependency",
				TerragruntConfigPaths: []string{"../test/fixture-modules/module-missing-dependency/.terragrunt"},
			},
		},
	}

	for _, testCase := range testCases {
		actualModules, actualErr := ResolveTerraformModules(testCase.terragruntConfigPaths, mockOptions)
		if testCase.expectedErr != nil && assert.NotNil(t, actualErr, "Expected error %v for paths %v but got nil", testCase.expectedErr, testCase.terragruntConfigPaths) {
			assertErrorsEqual(t, testCase.expectedErr, actualErr, "Expected error %v for paths %v but got: %v", testCase.expectedErr, testCase.terragruntConfigPaths, actualErr)
		} else if assert.Nil(t, actualErr, "Unexpected error for paths %v: %v", testCase.terragruntConfigPaths, actualErr) {
			assertModuleListsEqual(t, testCase.expectedModules, actualModules, "For config paths %v", testCase.terragruntConfigPaths)
		}
	}
}
