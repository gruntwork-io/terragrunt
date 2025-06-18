package configstack_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"

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

	testToRunningModules(t, runbase.Units{}, configstack.NormalOrder, configstack.RunningUnits{})
}

func TestToRunningModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	modules := runbase.Units{moduleA}
	expected := configstack.RunningUnits{"a": ctrlA}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesTwoModulesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(moduleB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	modules := runbase.Units{moduleA, moduleB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(moduleB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	ctrlA.NotifyWhenDone = []*configstack.DependencyController{ctrlB}

	modules := runbase.Units{moduleA, moduleB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(moduleB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{ctrlA}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	ctrlA.Dependencies = configstack.RunningUnits{"b": ctrlB}

	modules := runbase.Units{moduleA, moduleB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningModules(t, modules, configstack.ReverseOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(moduleB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	modules := runbase.Units{moduleA, moduleB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningModules(t, modules, configstack.IgnoreOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(moduleB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	ctrlA.NotifyWhenDone = []*configstack.DependencyController{ctrlB}

	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(moduleC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil

	ctrlA.NotifyWhenDone = []*configstack.DependencyController{ctrlC}

	moduleD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlD := configstack.NewDependencyController(moduleD)
	ctrlD.Dependencies = map[string]*configstack.DependencyController{"c": ctrlC}
	ctrlD.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlD.Runner.Status = runbase.Waiting
	ctrlD.Runner.Err = nil

	ctrlC.NotifyWhenDone = []*configstack.DependencyController{ctrlD}

	moduleE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlE := configstack.NewDependencyController(moduleE)
	ctrlE.Dependencies = configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
	}
	ctrlE.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlE.Runner.Status = runbase.Waiting
	ctrlE.Runner.Err = nil

	ctrlA.NotifyWhenDone = []*configstack.DependencyController{ctrlB, ctrlC, ctrlE}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{ctrlE}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{ctrlD, ctrlE}
	ctrlD.NotifyWhenDone = []*configstack.DependencyController{ctrlE}

	modules := runbase.Units{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}

	testToRunningModules(t, modules, configstack.NormalOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(moduleB)
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(moduleC)
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil

	moduleD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlD := configstack.NewDependencyController(moduleD)
	ctrlD.Runner.Status = runbase.Waiting
	ctrlD.Runner.Err = nil

	moduleE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlE := configstack.NewDependencyController(moduleE)
	ctrlE.Runner.Status = runbase.Waiting
	ctrlE.Runner.Err = nil

	// Set up dependencies and notify lists for reverse order
	ctrlA.Dependencies = configstack.RunningUnits{
		"b": ctrlB,
		"c": ctrlC,
		"e": ctrlE,
	}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}

	ctrlB.Dependencies = configstack.RunningUnits{
		"e": ctrlE,
	}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{ctrlA}

	ctrlC.Dependencies = configstack.RunningUnits{
		"d": ctrlD,
		"e": ctrlE,
	}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{ctrlA}

	ctrlD.Dependencies = configstack.RunningUnits{
		"e": ctrlE,
	}
	ctrlD.NotifyWhenDone = []*configstack.DependencyController{ctrlC}

	ctrlE.Dependencies = configstack.RunningUnits{}
	ctrlE.NotifyWhenDone = []*configstack.DependencyController{ctrlA, ctrlB, ctrlC, ctrlD}

	modules := runbase.Units{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}

	testToRunningModules(t, modules, configstack.ReverseOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(moduleB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(moduleC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil

	moduleD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlD := configstack.NewDependencyController(moduleD)
	ctrlD.Dependencies = map[string]*configstack.DependencyController{}
	ctrlD.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlD.Runner.Status = runbase.Waiting
	ctrlD.Runner.Err = nil

	moduleE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlE := configstack.NewDependencyController(moduleE)
	ctrlE.Dependencies = configstack.RunningUnits{}
	ctrlE.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlE.Runner.Status = runbase.Waiting
	ctrlE.Runner.Err = nil

	modules := runbase.Units{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}

	testToRunningModules(t, modules, configstack.IgnoreOrder, expected)
}

func testToRunningModules(t *testing.T, modules runbase.Units, order configstack.DependencyOrder, expected configstack.RunningUnits) {
	t.Helper()

	actual, err := configstack.ToRunningModules(modules, order, report.NewReport(), mockOptions)
	if assert.NoError(t, err, "For modules %v and order %v", modules, order) {
		assertDependencyControllerMapsEqual(t, expected, actual, true, "For modules %v and order %v", modules, order)
	}
}

func TestRemoveFlagExcludedNoExclude(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(moduleB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(moduleC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil

	moduleD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlD := configstack.NewDependencyController(moduleD)
	ctrlD.Dependencies = map[string]*configstack.DependencyController{"c": ctrlC}
	ctrlD.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlD.Runner.Status = runbase.Waiting
	ctrlD.Runner.Err = nil

	moduleE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{moduleB, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlE := configstack.NewDependencyController(moduleE)
	ctrlE.Dependencies = configstack.RunningUnits{
		"b": ctrlB,
		"d": ctrlD,
	}
	ctrlE.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlE.Runner.Status = runbase.Waiting
	ctrlE.Runner.Err = nil

	runningModules := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}

	expected := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}

	actual, err := runningModules.RemoveFlagExcluded(report.NewReport(), false)
	require.NoError(t, err)

	assertDependencyControllerMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(moduleB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(moduleC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil
	ctrlC.Runner.Unit.FlagExcluded = true

	runningModules := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
	}

	expected := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
	}

	actual, err := runningModules.RemoveFlagExcluded(report.NewReport(), mockOptions.Experiments.Evaluate(experiment.Report))
	require.NoError(t, err)

	assertDependencyControllerMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(moduleA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(moduleB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(moduleC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil
	ctrlC.Runner.Unit.FlagExcluded = true

	moduleD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlD := configstack.NewDependencyController(moduleD)
	ctrlD.Dependencies = map[string]*configstack.DependencyController{}
	ctrlD.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlD.Runner.Status = runbase.Waiting
	ctrlD.Runner.Err = nil

	moduleE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{moduleB, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlE := configstack.NewDependencyController(moduleE)
	ctrlE.Dependencies = configstack.RunningUnits{
		"b": ctrlB,
		"d": ctrlD,
	}
	ctrlE.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlE.Runner.Status = runbase.Waiting
	ctrlE.Runner.Err = nil

	runningModules := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}
	actual, err := runningModules.RemoveFlagExcluded(report.NewReport(), false)
	require.NoError(t, err)

	_ctrlD := configstack.NewDependencyController(moduleD)
	_ctrlD.Dependencies = map[string]*configstack.DependencyController{}
	_ctrlD.NotifyWhenDone = []*configstack.DependencyController{}
	_ctrlD.Runner.Status = runbase.Waiting
	_ctrlD.Runner.Err = nil

	expected := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"d": _ctrlD,
		"e": ctrlE,
	}

	assertDependencyControllerMapsEqual(t, expected, actual, true)
}
