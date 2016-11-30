package spin

import (
	"testing"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

func TestToRunningModules(t *testing.T) {
	t.Parallel()

	mockOptions := options.NewTerragruntOptionsForTest("TestToRunningModules")

	moduleA := &TerraformModule{
		Path: "a",
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	testCases := []struct {
		modules  []*TerraformModule
		order    DependencyOrder
		expected map[string]*runningModule
	}{
		{
			[]*TerraformModule{},
			NormalOrder,
			map[string]*runningModule{},
		},
		{
			[]*TerraformModule{moduleA},
			NormalOrder,
			map[string]*runningModule{
				"a": {
					Module: moduleA,
					Status: Waiting,
					Err: nil,
					Dependencies: map[string]*runningModule{},
				},
			},
		},
	}

	for _, testCase := range testCases {
		actual := toRunningModules(testCase.modules, testCase.order)
		assertRunningModuleMapsEqual(t, testCase.expected, actual, "For modules %v and order %v", testCase.modules, testCase.order)
	}
}

func TestRunModules(t *testing.T) {
	t.Parallel()

	// TODO
}
