package configstack_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockOptions, _ = options.NewTerragruntOptionsForTest("running_module_test")

func cloneOptions(t *testing.T, opts *options.TerragruntOptions, terragruntConfigPath string) *options.TerragruntOptions {
	t.Helper()

	newOpts, err := opts.Clone(canonical(t, terragruntConfigPath))
	require.NoError(t, err)
	return newOpts
}

func TestToRunningModulesNoModules(t *testing.T) {
	t.Parallel()

	testToRunningModules(t, configstack.TerraformModules{}, configstack.NormalOrder, configstack.RunningModules{})
}

func TestToRunningModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	modules := configstack.TerraformModules{moduleA}
	expected := configstack.RunningModules{"a": runningModuleA}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesTwoModulesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &configstack.RunningModule{
		Module:         moduleB,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	modules := configstack.TerraformModules{moduleA, moduleB}
	expected := configstack.RunningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &configstack.RunningModule{
		Module:         moduleB,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"a": runningModuleA},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	runningModuleA.NotifyWhenDone = []*configstack.RunningModule{runningModuleB}

	modules := configstack.TerraformModules{moduleA, moduleB}
	expected := configstack.RunningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &configstack.RunningModule{
		Module:         moduleB,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{runningModuleA},
	}

	runningModuleA.Dependencies = configstack.RunningModules{"b": runningModuleB}

	modules := configstack.TerraformModules{moduleA, moduleB}
	expected := configstack.RunningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, configstack.ReverseOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &configstack.RunningModule{
		Module:         moduleB,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	modules := configstack.TerraformModules{moduleA, moduleB}
	expected := configstack.RunningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, configstack.IgnoreOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &configstack.RunningModule{
		Module:         moduleB,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"a": runningModuleA},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &configstack.RunningModule{
		Module:         moduleC,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"a": runningModuleA},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleD := &configstack.TerraformModule{
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &configstack.RunningModule{
		Module:         moduleD,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"c": runningModuleC},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleE := &configstack.TerraformModule{
		Path:              "e",
		Dependencies:      configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &configstack.RunningModule{
		Module: moduleE,
		Status: configstack.Waiting,
		Err:    nil,
		Dependencies: configstack.RunningModules{
			"a": runningModuleA,
			"b": runningModuleB,
			"c": runningModuleC,
			"d": runningModuleD,
		},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	runningModuleA.NotifyWhenDone = []*configstack.RunningModule{runningModuleB, runningModuleC, runningModuleE}
	runningModuleB.NotifyWhenDone = []*configstack.RunningModule{runningModuleE}
	runningModuleC.NotifyWhenDone = []*configstack.RunningModule{runningModuleD, runningModuleE}
	runningModuleD.NotifyWhenDone = []*configstack.RunningModule{runningModuleE}

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := configstack.RunningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &configstack.RunningModule{
		Module:         moduleB,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{runningModuleA},
	}

	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &configstack.RunningModule{
		Module:         moduleC,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{runningModuleA},
	}

	moduleD := &configstack.TerraformModule{
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &configstack.RunningModule{
		Module:         moduleD,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{runningModuleC},
	}

	moduleE := &configstack.TerraformModule{
		Path:              "e",
		Dependencies:      configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &configstack.RunningModule{
		Module:         moduleE,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{runningModuleA, runningModuleB, runningModuleC, runningModuleD},
	}

	runningModuleA.Dependencies = configstack.RunningModules{"b": runningModuleB, "c": runningModuleC, "e": runningModuleE}
	runningModuleB.Dependencies = configstack.RunningModules{"e": runningModuleE}
	runningModuleC.Dependencies = configstack.RunningModules{"d": runningModuleD, "e": runningModuleE}
	runningModuleD.Dependencies = configstack.RunningModules{"e": runningModuleE}

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := configstack.RunningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, configstack.ReverseOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &configstack.RunningModule{
		Module:         moduleB,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &configstack.RunningModule{
		Module:         moduleC,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleD := &configstack.TerraformModule{
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &configstack.RunningModule{
		Module:         moduleD,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	moduleE := &configstack.TerraformModule{
		Path:              "e",
		Dependencies:      configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &configstack.RunningModule{
		Module:         moduleE,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
	}

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := configstack.RunningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, configstack.IgnoreOrder, expected)
}

func testToRunningModules(t *testing.T, modules configstack.TerraformModules, order configstack.DependencyOrder, expected configstack.RunningModules) {
	t.Helper()

	actual, err := modules.ToRunningModules(order)
	if assert.NoError(t, err, "For modules %v and order %v", modules, order) {
		assertRunningModuleMapsEqual(t, expected, actual, true, "For modules %v and order %v", modules, order)
	}
}

func TestRemoveFlagExcludedNoExclude(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &configstack.RunningModule{
		Module:         moduleB,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"a": runningModuleA},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &configstack.RunningModule{
		Module:         moduleC,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"a": runningModuleA},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	moduleD := &configstack.TerraformModule{
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &configstack.RunningModule{
		Module:         moduleD,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"c": runningModuleC},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	moduleE := &configstack.TerraformModule{
		Path:              "e",
		Dependencies:      configstack.TerraformModules{moduleB, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &configstack.RunningModule{
		Module: moduleE,
		Status: configstack.Waiting,
		Err:    nil,
		Dependencies: configstack.RunningModules{
			"b": runningModuleB,
			"d": runningModuleD,
		},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	runningModules := configstack.RunningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	expected := configstack.RunningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	actual := runningModules.RemoveFlagExcluded()
	assertRunningModuleMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &configstack.RunningModule{
		Module:         moduleB,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"a": runningModuleA},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &configstack.RunningModule{
		Module:         moduleC,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"a": runningModuleA},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   true,
	}

	runningModules := configstack.RunningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
	}

	expected := configstack.RunningModules{
		"a": runningModuleA,
		"b": runningModuleB,
	}

	actual := runningModules.RemoveFlagExcluded()
	assertRunningModuleMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &configstack.RunningModule{
		Module:         moduleA,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &configstack.RunningModule{
		Module:         moduleB,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"a": runningModuleA},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &configstack.RunningModule{
		Module:         moduleC,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"a": runningModuleA},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   true,
	}

	moduleD := &configstack.TerraformModule{
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &configstack.RunningModule{
		Module: moduleD,
		Status: configstack.Waiting,
		Err:    nil,
		Dependencies: configstack.RunningModules{
			"b": runningModuleB,
			"c": runningModuleC,
		},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	moduleE := &configstack.TerraformModule{
		Path:              "e",
		Dependencies:      configstack.TerraformModules{moduleB, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &configstack.RunningModule{
		Module: moduleE,
		Status: configstack.Waiting,
		Err:    nil,
		Dependencies: configstack.RunningModules{
			"b": runningModuleB,
			"d": runningModuleD,
		},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	runningModules := configstack.RunningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}
	actual := runningModules.RemoveFlagExcluded()

	_runningModuleD := &configstack.RunningModule{
		Module:         moduleD,
		Status:         configstack.Waiting,
		Err:            nil,
		Dependencies:   configstack.RunningModules{"b": runningModuleB},
		NotifyWhenDone: []*configstack.RunningModule{},
		FlagExcluded:   false,
	}

	expected := configstack.RunningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"d": _runningModuleD,
		"e": runningModuleE,
	}

	assertRunningModuleMapsEqual(t, expected, actual, true)
}
