package configstack

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

var mockOptions, _ = options.NewTerragruntOptionsForTest("running_module_test")

func TestToRunningModulesNoModules(t *testing.T) {
	t.Parallel()

	testToRunningModules(t, TerraformModules{}, NormalOrder, runningModules{})
}

func TestToRunningModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	modules := TerraformModules{moduleA}
	expected := runningModules{"a": runningModuleA}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesTwoModulesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	modules := TerraformModules{moduleA, moduleB}
	expected := runningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
	}

	runningModuleA.NotifyWhenDone = []*runningModule{runningModuleB}

	modules := TerraformModules{moduleA, moduleB}
	expected := runningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{runningModuleA},
	}

	runningModuleA.Dependencies = runningModules{"b": runningModuleB}

	modules := TerraformModules{moduleA, moduleB}
	expected := runningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, ReverseOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	modules := TerraformModules{moduleA, moduleB}
	expected := runningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, IgnoreOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
	}

	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module:         moduleD,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"c": runningModuleC},
		NotifyWhenDone: []*runningModule{},
	}

	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      TerraformModules{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module: moduleE,
		Status: Waiting,
		Err:    nil,
		Dependencies: runningModules{
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

	modules := TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := runningModules{
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
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{runningModuleA},
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{runningModuleA},
	}

	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module:         moduleD,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{runningModuleC},
	}

	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      TerraformModules{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module:         moduleE,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{runningModuleA, runningModuleB, runningModuleC, runningModuleD},
	}

	runningModuleA.Dependencies = runningModules{"b": runningModuleB, "c": runningModuleC, "e": runningModuleE}
	runningModuleB.Dependencies = runningModules{"e": runningModuleE}
	runningModuleC.Dependencies = runningModules{"d": runningModuleD, "e": runningModuleE}
	runningModuleD.Dependencies = runningModules{"e": runningModuleE}

	modules := TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := runningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, ReverseOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module:         moduleD,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      TerraformModules{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module:         moduleE,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
	}

	modules := TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := runningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, IgnoreOrder, expected)
}

func testToRunningModules(t *testing.T, modules TerraformModules, order DependencyOrder, expected runningModules) {
	actual, err := modules.toRunningModules(order)
	if assert.Nil(t, err, "For modules %v and order %v", modules, order) {
		assertRunningModuleMapsEqual(t, expected, actual, true, "For modules %v and order %v", modules, order)
	}
}

func TestRemoveFlagExcludedNoExclude(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module:         moduleD,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"c": runningModuleC},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      TerraformModules{moduleB, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module: moduleE,
		Status: Waiting,
		Err:    nil,
		Dependencies: runningModules{
			"b": runningModuleB,
			"d": runningModuleD,
		},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	running_modules := runningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	expected := runningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	actual := running_modules.removeFlagExcluded()
	assertRunningModuleMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   true,
	}

	running_modules := runningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
	}

	expected := runningModules{
		"a": runningModuleA,
		"b": runningModuleB,
	}

	actual := running_modules.removeFlagExcluded()
	assertRunningModuleMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   true,
	}

	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      TerraformModules{moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module: moduleD,
		Status: Waiting,
		Err:    nil,
		Dependencies: runningModules{
			"b": runningModuleB,
			"c": runningModuleC,
		},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      TerraformModules{moduleB, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module: moduleE,
		Status: Waiting,
		Err:    nil,
		Dependencies: runningModules{
			"b": runningModuleB,
			"d": runningModuleD,
		},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	running_modules := runningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}
	actual := running_modules.removeFlagExcluded()

	_runningModuleD := &runningModule{
		Module:         moduleD,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   runningModules{"b": runningModuleB},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	expected := runningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"d": _runningModuleD,
		"e": runningModuleE,
	}

	assertRunningModuleMapsEqual(t, expected, actual, true)
}
