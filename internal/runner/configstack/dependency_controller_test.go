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

var mockOptions, _ = options.NewTerragruntOptionsForTest("running_unit_test")

func cloneOptions(t *testing.T, l log.Logger, opts *options.TerragruntOptions, terragruntConfigPath string) (log.Logger, *options.TerragruntOptions) {
	t.Helper()

	l, newOpts, err := opts.CloneWithConfigPath(l, canonical(t, terragruntConfigPath))
	require.NoError(t, err)

	return l, newOpts
}

func TestToRunningUnitsNoUnits(t *testing.T) {
	t.Parallel()

	testToRunningUnits(t, runbase.Units{}, configstack.NormalOrder, configstack.RunningUnits{})
}

func TestToRunningUnitsOneUnitNoDependencies(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	units := runbase.Units{unitA}
	expected := configstack.RunningUnits{"a": ctrlA}

	testToRunningUnits(t, units, configstack.NormalOrder, expected)
}

func TestToRunningUnitsTwoUnitsNoDependencies(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	units := runbase.Units{unitA, unitB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningUnits(t, units, configstack.NormalOrder, expected)
}

func TestToRunningUnitsTwoUnitsWithDependencies(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	ctrlA.NotifyWhenDone = []*configstack.DependencyController{ctrlB}

	units := runbase.Units{unitA, unitB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningUnits(t, units, configstack.NormalOrder, expected)
}

func TestToRunningUnitsTwoUnitsWithDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{ctrlA}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	ctrlA.Dependencies = configstack.RunningUnits{"b": ctrlB}

	units := runbase.Units{unitA, unitB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningUnits(t, units, configstack.ReverseOrder, expected)
}

func TestToRunningUnitsTwoUnitsWithDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	units := runbase.Units{unitA, unitB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningUnits(t, units, configstack.IgnoreOrder, expected)
}

func TestToRunningUnitsMultipleUnitsWithAndWithoutDependencies(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	ctrlA.NotifyWhenDone = []*configstack.DependencyController{ctrlB}

	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(unitC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil

	ctrlA.NotifyWhenDone = []*configstack.DependencyController{ctrlC}

	unitD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{unitC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlD := configstack.NewDependencyController(unitD)
	ctrlD.Dependencies = map[string]*configstack.DependencyController{"c": ctrlC}
	ctrlD.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlD.Runner.Status = runbase.Waiting
	ctrlD.Runner.Err = nil

	ctrlC.NotifyWhenDone = []*configstack.DependencyController{ctrlD}

	unitE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{unitA, unitB, unitC, unitD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlE := configstack.NewDependencyController(unitE)
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

	units := runbase.Units{unitA, unitB, unitC, unitD, unitE}
	expected := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}

	testToRunningUnits(t, units, configstack.NormalOrder, expected)
}

func TestToRunningUnitsMultipleUnitsWithAndWithoutDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(unitB)
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(unitC)
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil

	unitD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{unitC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlD := configstack.NewDependencyController(unitD)
	ctrlD.Runner.Status = runbase.Waiting
	ctrlD.Runner.Err = nil

	unitE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{unitA, unitB, unitC, unitD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlE := configstack.NewDependencyController(unitE)
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

	units := runbase.Units{unitA, unitB, unitC, unitD, unitE}
	expected := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}

	testToRunningUnits(t, units, configstack.ReverseOrder, expected)
}

func TestToRunningUnitsMultipleUnitsWithAndWithoutDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(unitC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil

	unitD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{unitC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlD := configstack.NewDependencyController(unitD)
	ctrlD.Dependencies = map[string]*configstack.DependencyController{}
	ctrlD.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlD.Runner.Status = runbase.Waiting
	ctrlD.Runner.Err = nil

	unitE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{unitA, unitB, unitC, unitD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlE := configstack.NewDependencyController(unitE)
	ctrlE.Dependencies = configstack.RunningUnits{}
	ctrlE.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlE.Runner.Status = runbase.Waiting
	ctrlE.Runner.Err = nil

	units := runbase.Units{unitA, unitB, unitC, unitD, unitE}
	expected := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}

	testToRunningUnits(t, units, configstack.IgnoreOrder, expected)
}

func testToRunningUnits(t *testing.T, units runbase.Units, order configstack.DependencyOrder, expected configstack.RunningUnits) {
	t.Helper()

	actual, err := configstack.ToRunningModules(units, order, report.NewReport(), mockOptions)
	if assert.NoError(t, err, "For units %v and order %v", units, order) {
		assertDependencyControllerMapsEqual(t, expected, actual, true, "For units %v and order %v", units, order)
	}
}

func TestRemoveFlagExcludedNoExclude(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(unitC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil

	unitD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{unitC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlD := configstack.NewDependencyController(unitD)
	ctrlD.Dependencies = map[string]*configstack.DependencyController{"c": ctrlC}
	ctrlD.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlD.Runner.Status = runbase.Waiting
	ctrlD.Runner.Err = nil

	unitE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{unitB, unitD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlE := configstack.NewDependencyController(unitE)
	ctrlE.Dependencies = configstack.RunningUnits{
		"b": ctrlB,
		"d": ctrlD,
	}
	ctrlE.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlE.Runner.Status = runbase.Waiting
	ctrlE.Runner.Err = nil

	runningUnits := configstack.RunningUnits{
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

	actual, err := runningUnits.RemoveFlagExcluded(report.NewReport(), false)
	require.NoError(t, err)

	assertDependencyControllerMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeNoDependencies(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(unitC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil
	ctrlC.Runner.Unit.FlagExcluded = true

	runningUnits := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
	}

	expected := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
	}

	actual, err := runningUnits.RemoveFlagExcluded(report.NewReport(), mockOptions.Experiments.Evaluate(experiment.Report))
	require.NoError(t, err)

	assertDependencyControllerMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeWithDependencies(t *testing.T) {
	t.Parallel()

	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlA := configstack.NewDependencyController(unitA)
	ctrlA.Dependencies = map[string]*configstack.DependencyController{}
	ctrlA.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlA.Runner.Status = runbase.Waiting
	ctrlA.Runner.Err = nil

	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlB := configstack.NewDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlB.Runner.Status = runbase.Waiting
	ctrlB.Runner.Err = nil

	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlC := configstack.NewDependencyController(unitC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}
	ctrlC.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlC.Runner.Status = runbase.Waiting
	ctrlC.Runner.Err = nil
	ctrlC.Runner.Unit.FlagExcluded = true

	unitD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{unitC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlD := configstack.NewDependencyController(unitD)
	ctrlD.Dependencies = map[string]*configstack.DependencyController{}
	ctrlD.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlD.Runner.Status = runbase.Waiting
	ctrlD.Runner.Err = nil

	unitE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{unitB, unitD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	ctrlE := configstack.NewDependencyController(unitE)
	ctrlE.Dependencies = configstack.RunningUnits{
		"b": ctrlB,
		"d": ctrlD,
	}
	ctrlE.NotifyWhenDone = []*configstack.DependencyController{}
	ctrlE.Runner.Status = runbase.Waiting
	ctrlE.Runner.Err = nil

	runningUnits := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}
	actual, err := runningUnits.RemoveFlagExcluded(report.NewReport(), false)
	require.NoError(t, err)

	_ctrlD := configstack.NewDependencyController(unitD)
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
