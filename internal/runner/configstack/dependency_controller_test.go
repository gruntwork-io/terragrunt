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

// Helper to create a runbase.Unit with default fields
func newTestUnit(path string, dependencies runbase.Units, config config.TerragruntConfig, opts *options.TerragruntOptions) *runbase.Unit {
	return &runbase.Unit{
		Path:              path,
		Dependencies:      dependencies,
		Config:            config,
		TerragruntOptions: opts,
	}
}

// Helper to create and initialize a DependencyController for a unit
func newTestDependencyController(unit *runbase.Unit) *configstack.DependencyController {
	ctrl := configstack.NewDependencyController(unit)
	ctrl.Dependencies = map[string]*configstack.DependencyController{}
	ctrl.NotifyWhenDone = []*configstack.DependencyController{}
	ctrl.Runner.Status = runbase.Waiting
	ctrl.Runner.Err = nil
	return ctrl
}

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

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	units := runbase.Units{unitA}
	expected := configstack.RunningUnits{"a": ctrlA}

	testToRunningUnits(t, units, configstack.NormalOrder, expected)
}

func TestToRunningUnitsTwoUnitsNoDependencies(t *testing.T) {
	t.Parallel()

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	unitB := newTestUnit("b", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlB := newTestDependencyController(unitB)

	units := runbase.Units{unitA, unitB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningUnits(t, units, configstack.NormalOrder, expected)
}

func TestToRunningUnitsTwoUnitsWithDependencies(t *testing.T) {
	t.Parallel()

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	unitB := newTestUnit("b", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlB := newTestDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}

	ctrlA.NotifyWhenDone = []*configstack.DependencyController{ctrlB}

	units := runbase.Units{unitA, unitB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningUnits(t, units, configstack.NormalOrder, expected)
}

func TestToRunningUnitsTwoUnitsWithDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	unitB := newTestUnit("b", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlB := newTestDependencyController(unitB)
	ctrlB.NotifyWhenDone = []*configstack.DependencyController{ctrlA}

	ctrlA.Dependencies = configstack.RunningUnits{"b": ctrlB}

	units := runbase.Units{unitA, unitB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningUnits(t, units, configstack.ReverseOrder, expected)
}

func TestToRunningUnitsTwoUnitsWithDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	unitB := newTestUnit("b", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlB := newTestDependencyController(unitB)

	units := runbase.Units{unitA, unitB}
	expected := configstack.RunningUnits{"a": ctrlA, "b": ctrlB}

	testToRunningUnits(t, units, configstack.IgnoreOrder, expected)
}

func TestToRunningUnitsMultipleUnitsWithAndWithoutDependencies(t *testing.T) {
	t.Parallel()

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	unitB := newTestUnit("b", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlB := newTestDependencyController(unitB)
	ctrlB.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}

	ctrlA.NotifyWhenDone = []*configstack.DependencyController{ctrlB}

	unitC := newTestUnit("c", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlC := newTestDependencyController(unitC)
	ctrlC.Dependencies = map[string]*configstack.DependencyController{"a": ctrlA}

	ctrlA.NotifyWhenDone = []*configstack.DependencyController{ctrlC}

	unitD := newTestUnit("d", runbase.Units{unitC}, config.TerragruntConfig{}, mockOptions)
	ctrlD := newTestDependencyController(unitD)
	ctrlD.Dependencies = map[string]*configstack.DependencyController{"c": ctrlC}

	ctrlC.NotifyWhenDone = []*configstack.DependencyController{ctrlD}

	unitE := newTestUnit("e", runbase.Units{unitA, unitB, unitC, unitD}, config.TerragruntConfig{}, mockOptions)
	ctrlE := newTestDependencyController(unitE)
	ctrlE.Dependencies = configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
	}

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

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	unitB := newTestUnit("b", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlB := newTestDependencyController(unitB)

	unitC := newTestUnit("c", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlC := newTestDependencyController(unitC)

	unitD := newTestUnit("d", runbase.Units{unitC}, config.TerragruntConfig{}, mockOptions)
	ctrlD := newTestDependencyController(unitD)

	unitE := newTestUnit("e", runbase.Units{unitA, unitB, unitC, unitD}, config.TerragruntConfig{}, mockOptions)
	ctrlE := newTestDependencyController(unitE)

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

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	unitB := newTestUnit("b", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlB := newTestDependencyController(unitB)

	unitC := newTestUnit("c", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlC := newTestDependencyController(unitC)

	unitD := newTestUnit("d", runbase.Units{unitC}, config.TerragruntConfig{}, mockOptions)
	ctrlD := newTestDependencyController(unitD)

	unitE := newTestUnit("e", runbase.Units{unitA, unitB, unitC, unitD}, config.TerragruntConfig{}, mockOptions)
	ctrlE := newTestDependencyController(unitE)
	ctrlE.Dependencies = configstack.RunningUnits{}
	ctrlE.NotifyWhenDone = []*configstack.DependencyController{}

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

	actual, err := configstack.ToRunningUnits(units, order, report.NewReport(), mockOptions)
	if assert.NoError(t, err, "For units %v and order %v", units, order) {
		assertDependencyControllerMapsEqual(t, expected, actual, true, "For units %v and order %v", units, order)
	}
}

func TestRemoveFlagExcludedNoExclude(t *testing.T) {
	t.Parallel()

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	unitB := newTestUnit("b", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlB := newTestDependencyController(unitB)

	unitC := newTestUnit("c", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlC := newTestDependencyController(unitC)

	unitD := newTestUnit("d", runbase.Units{unitC}, config.TerragruntConfig{}, mockOptions)
	ctrlD := newTestDependencyController(unitD)

	unitE := newTestUnit("e", runbase.Units{unitB, unitD}, config.TerragruntConfig{}, mockOptions)
	ctrlE := newTestDependencyController(unitE)

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

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	unitB := newTestUnit("b", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlB := newTestDependencyController(unitB)

	unitC := newTestUnit("c", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlC := newTestDependencyController(unitC)
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

	unitA := newTestUnit("a", runbase.Units{}, config.TerragruntConfig{}, mockOptions)
	ctrlA := newTestDependencyController(unitA)

	unitB := newTestUnit("b", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlB := newTestDependencyController(unitB)

	unitC := newTestUnit("c", runbase.Units{unitA}, config.TerragruntConfig{}, mockOptions)
	ctrlC := newTestDependencyController(unitC)
	ctrlC.Runner.Unit.FlagExcluded = true

	unitD := newTestUnit("d", runbase.Units{unitC}, config.TerragruntConfig{}, mockOptions)
	ctrlD := newTestDependencyController(unitD)

	unitE := newTestUnit("e", runbase.Units{unitB, unitD}, config.TerragruntConfig{}, mockOptions)
	ctrlE := newTestDependencyController(unitE)

	runningUnits := configstack.RunningUnits{
		"a": ctrlA,
		"b": ctrlB,
		"c": ctrlC,
		"d": ctrlD,
		"e": ctrlE,
	}
	actual, err := runningUnits.RemoveFlagExcluded(report.NewReport(), false)
	require.NoError(t, err)

	_ctrlD := newTestDependencyController(unitD)
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
