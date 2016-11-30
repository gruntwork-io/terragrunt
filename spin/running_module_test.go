package spin

import (
	"testing"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

var mockOptions = options.NewTerragruntOptionsForTest("running_module_test")

func TestToRunningModulesNoModules(t *testing.T) {
	t.Parallel()

	testToRunningModules(t, []*TerraformModule{}, NormalOrder, map[string]*runningModule{})
}

func TestToRunningModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path: "a",
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module: moduleA,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	modules := []*TerraformModule{moduleA}
	expected := map[string]*runningModule{"a": runningModuleA}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesTwoModulesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path: "a",
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module: moduleA,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path: "b",
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module: moduleB,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	modules := []*TerraformModule{moduleA, moduleB}
	expected := map[string]*runningModule{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path: "a",
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module: moduleA,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path: "b",
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module: moduleB,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
	}

	runningModuleA.NotifyWhenDone = []*runningModule{runningModuleB}

	modules := []*TerraformModule{moduleA, moduleB}
	expected := map[string]*runningModule{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path: "a",
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module: moduleA,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path: "b",
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module: moduleB,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{runningModuleA},
	}

	runningModuleA.Dependencies = map[string]*runningModule{"b": runningModuleB}

	modules := []*TerraformModule{moduleA, moduleB}
	expected := map[string]*runningModule{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, ReverseOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path: "a",
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module: moduleA,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path: "b",
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module: moduleB,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
	}

	moduleC := &TerraformModule{
		Path: "c",
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module: moduleC,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
	}

	moduleD := &TerraformModule{
		Path: "d",
		Dependencies: []*TerraformModule{moduleC},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module: moduleD,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{"c": runningModuleC},
		NotifyWhenDone: []*runningModule{},
	}

	moduleE := &TerraformModule{
		Path: "e",
		Dependencies: []*TerraformModule{moduleA, moduleB, moduleC, moduleD},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module: moduleE,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{
			"a": runningModuleA,
			"b": runningModuleB,
			"c": runningModuleC,
			"d": runningModuleD,
		},
		NotifyWhenDone: []*runningModule{},
	}

	runningModuleA.NotifyWhenDone = []*runningModule{runningModuleB, runningModuleC, runningModuleE}
	runningModuleB.NotifyWhenDone = []*runningModule{runningModuleE}
	runningModuleC.NotifyWhenDone = []*runningModule{runningModuleD, runningModuleE}
	runningModuleD.NotifyWhenDone = []*runningModule{runningModuleE}

	modules := []*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path: "a",
		Dependencies: []*TerraformModule{},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module: moduleA,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path: "b",
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module: moduleB,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{runningModuleA},
	}

	moduleC := &TerraformModule{
		Path: "c",
		Dependencies: []*TerraformModule{moduleA},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module: moduleC,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{runningModuleA},
	}

	moduleD := &TerraformModule{
		Path: "d",
		Dependencies: []*TerraformModule{moduleC},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module: moduleD,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{runningModuleC},
	}

	moduleE := &TerraformModule{
		Path: "e",
		Dependencies: []*TerraformModule{moduleA, moduleB, moduleC, moduleD},
		Config: config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module: moduleE,
		Status: Waiting,
		Err: nil,
		Dependencies: map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{runningModuleA, runningModuleB, runningModuleC, runningModuleD},
	}

	runningModuleA.Dependencies = map[string]*runningModule{"b": runningModuleB, "c": runningModuleC, "e": runningModuleE}
	runningModuleB.Dependencies = map[string]*runningModule{"e": runningModuleE}
	runningModuleC.Dependencies = map[string]*runningModule{"d": runningModuleD, "e": runningModuleE}
	runningModuleD.Dependencies = map[string]*runningModule{"e": runningModuleE}

	modules := []*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, ReverseOrder, expected)
}

func testToRunningModules(t *testing.T, modules []*TerraformModule, order DependencyOrder, expected map[string]*runningModule) {
	actual, err := toRunningModules(modules, order)
	if assert.Nil(t, err, "For modules %v and order %v", modules, order) {
		assertRunningModuleMapsEqual(t, expected, actual, true, "For modules %v and order %v", modules, order)
	}
}

func TestRunModules(t *testing.T) {
	t.Parallel()

	// TODO
}
