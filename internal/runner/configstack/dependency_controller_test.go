package configstack_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockOptions, _ = options.NewTerragruntOptionsForTest("running_module_test")

func cloneOptions(t *testing.T, l log.Logger, opts *options.TerragruntOptions, terragruntConfigPath string) (log.Logger, *options.TerragruntOptions) {
	t.Helper()

	l, newOpts, err := opts.CloneWithConfigPath(l, canonical(t, terragruntConfigPath))
	require.NoError(t, err)

	return l, newOpts
}

func TestToRunningModulesNoModules(t *testing.T) {
	t.Parallel()

	testToRunningModules(t, common.Units{}, configstack.NormalOrder, configstack.RunningModules{})
}

func TestToRunningModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	modules := common.Units{moduleA}
	expected := configstack.RunningModules{"a": runningModuleA}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesTwoModulesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{},
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

	modules := common.Units{moduleA, moduleB}
	expected := configstack.RunningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
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

	modules := common.Units{moduleA, moduleB}
	expected := configstack.RunningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
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

	modules := common.Units{moduleA, moduleB}
	expected := configstack.RunningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, configstack.ReverseOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
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

	modules := common.Units{moduleA, moduleB}
	expected := configstack.RunningModules{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, configstack.IgnoreOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
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

	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleA},
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

	moduleD := &common.Unit{
		Path:              "d",
		Dependencies:      common.Units{moduleC},
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

	moduleE := &common.Unit{
		Path:              "e",
		Dependencies:      common.Units{moduleA, moduleB, moduleC, moduleD},
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

	modules := common.Units{moduleA, moduleB, moduleC, moduleD, moduleE}
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

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
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

	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleA},
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

	moduleD := &common.Unit{
		Path:              "d",
		Dependencies:      common.Units{moduleC},
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

	moduleE := &common.Unit{
		Path:              "e",
		Dependencies:      common.Units{moduleA, moduleB, moduleC, moduleD},
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

	modules := common.Units{moduleA, moduleB, moduleC, moduleD, moduleE}
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

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
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

	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleA},
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

	moduleD := &common.Unit{
		Path:              "d",
		Dependencies:      common.Units{moduleC},
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

	moduleE := &common.Unit{
		Path:              "e",
		Dependencies:      common.Units{moduleA, moduleB, moduleC, moduleD},
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

	modules := common.Units{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := configstack.RunningModules{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, configstack.IgnoreOrder, expected)
}

func testToRunningModules(t *testing.T, modules common.Units, order configstack.DependencyOrder, expected configstack.RunningModules) {
	t.Helper()

	actual, err := configstack.ToRunningModules(modules, order, report.NewReport(), mockOptions)
	if assert.NoError(t, err, "For modules %v and order %v", modules, order) {
		assertRunningModuleMapsEqual(t, expected, actual, true, "For modules %v and order %v", modules, order)
	}
}

func TestRemoveFlagExcludedNoExclude(t *testing.T) {
	t.Parallel()

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
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

	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleA},
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

	moduleD := &common.Unit{
		Path:              "d",
		Dependencies:      common.Units{moduleC},
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

	moduleE := &common.Unit{
		Path:              "e",
		Dependencies:      common.Units{moduleB, moduleD},
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

	actual, err := runningModules.RemoveFlagExcluded(report.NewReport(), false)
	require.NoError(t, err)

	assertRunningModuleMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
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

	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleA},
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

	actual, err := runningModules.RemoveFlagExcluded(report.NewReport(), mockOptions.Experiments.Evaluate(experiment.Report))
	require.NoError(t, err)

	assertRunningModuleMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
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

	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
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

	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleA},
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

	moduleD := &common.Unit{
		Path:              "d",
		Dependencies:      common.Units{moduleB, moduleC},
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

	moduleE := &common.Unit{
		Path:              "e",
		Dependencies:      common.Units{moduleB, moduleD},
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
	actual, err := runningModules.RemoveFlagExcluded(report.NewReport(), false)
	require.NoError(t, err)

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
