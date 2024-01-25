package graph

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/configstack"
)

func TestNoDependencies(t *testing.T) {
	t.Parallel()
	stack := &configstack.Stack{
		Modules: []*configstack.TerraformModule{
			{Path: "moduleA"},
			{Path: "moduleB"},
		},
	}

	filterDependencies(stack, "moduleA")

	// Verify that moduleA has FlagExcluded set to false and others set to true
	for _, module := range stack.Modules {
		if module.Path == "moduleA" {
			if module.FlagExcluded {
				t.Errorf("Expected FlagExcluded to be false for workDir module")
			}
		} else {
			if !module.FlagExcluded {
				t.Errorf("Expected FlagExcluded to be true for non-workDir modules")
			}
		}
	}
}

func TestTransientAndUnrelatedDependencies(t *testing.T) {
	t.Parallel()
	moduleA := &configstack.TerraformModule{Path: "moduleA"}
	moduleB := &configstack.TerraformModule{Path: "moduleB", Dependencies: []*configstack.TerraformModule{moduleA}} // B depends on A
	moduleC := &configstack.TerraformModule{Path: "moduleC", Dependencies: []*configstack.TerraformModule{moduleB}} // C depends on B (transiently on A)
	moduleD := &configstack.TerraformModule{Path: "moduleD"}                                                        // D is unrelated

	stack := &configstack.Stack{
		Modules: []*configstack.TerraformModule{moduleA, moduleB, moduleC, moduleD},
	}

	filterDependencies(stack, "moduleA")

	// Check if the FlagExcluded is set correctly
	for _, module := range stack.Modules {
		switch module.Path {
		case "moduleA", "moduleB", "moduleC":
			if module.FlagExcluded {
				t.Errorf("Expected FlagExcluded to be false for module %s, got true", module.Path)
			}
		case "moduleD":
			if !module.FlagExcluded {
				t.Errorf("Expected FlagExcluded to be true for unrelated module %s, got false", module.Path)
			}
		}
	}
}
