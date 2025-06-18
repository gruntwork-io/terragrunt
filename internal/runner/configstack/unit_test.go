package configstack_test

import (
	"bytes"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/runbase"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/runner/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraph(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	a := &runbase.Unit{Path: "a", Logger: l}
	b := &runbase.Unit{Path: "b", Logger: l}
	c := &runbase.Unit{Path: "c", Logger: l}
	d := &runbase.Unit{Path: "d", Logger: l}
	e := &runbase.Unit{Path: "e", Dependencies: []*runbase.Unit{a}, Logger: l}
	f := &runbase.Unit{Path: "f", Dependencies: []*runbase.Unit{a, b}, Logger: l}
	g := &runbase.Unit{Path: "g", Dependencies: []*runbase.Unit{e}, Logger: l}
	h := &runbase.Unit{Path: "h", Dependencies: []*runbase.Unit{g, f, c}, Logger: l}

	units := runbase.Units{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsForTest("/terragrunt.hcl")
	units.WriteDot(logger.CreateLogger(), &stdout, terragruntOptions)
	expected := strings.TrimSpace(`
digraph {
	"a" ;
	"b" ;
	"c" ;
	"d" ;
	"e" ;
	"e" -> "a";
	"f" ;
	"f" -> "a";
	"f" -> "b";
	"g" ;
	"g" -> "e";
	"h" ;
	"h" -> "g";
	"h" -> "f";
	"h" -> "c";
}
`)
	// clean string to work in cross-platform way
	actual := util.CleanString(stdout.String())
	expected = util.CleanString(expected)

	assert.Contains(t, actual, expected)
}

func TestGraphTrimPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows due to path issues")
	}
	t.Parallel()

	l := logger.CreateLogger()

	a := &runbase.Unit{Path: "/config/a", Logger: l}
	b := &runbase.Unit{Path: "/config/b", Logger: l}
	c := &runbase.Unit{Path: "/config/c", Logger: l}
	d := &runbase.Unit{Path: "/config/d", Logger: l}
	e := &runbase.Unit{Path: "/config/alpha/beta/gamma/e", Dependencies: []*runbase.Unit{a}, Logger: l}
	f := &runbase.Unit{Path: "/config/alpha/beta/gamma/f", Dependencies: []*runbase.Unit{a, b}, Logger: l}
	g := &runbase.Unit{Path: "/config/alpha/g", Dependencies: []*runbase.Unit{e}, Logger: l}
	h := &runbase.Unit{Path: "/config/alpha/beta/h", Dependencies: []*runbase.Unit{g, f, c}, Logger: l}

	units := runbase.Units{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsWithConfigPath("/config/terragrunt.hcl")
	units.WriteDot(logger.CreateLogger(), &stdout, terragruntOptions)
	expected := strings.TrimSpace(`
digraph {
	"a" ;
	"b" ;
	"c" ;
	"d" ;
	"alpha/beta/gamma/e" ;
	"alpha/beta/gamma/e" -> "a";
	"alpha/beta/gamma/f" ;
	"alpha/beta/gamma/f" -> "a";
	"alpha/beta/gamma/f" -> "b";
	"alpha/g" ;
	"alpha/g" -> "alpha/beta/gamma/e";
	"alpha/beta/h" ;
	"alpha/beta/h" -> "alpha/g";
	"alpha/beta/h" -> "alpha/beta/gamma/f";
	"alpha/beta/h" -> "c";
}
`)
	// clean string to work in cross-platform way
	actual := util.CleanString(stdout.String())
	expected = util.CleanString(expected)

	assert.Contains(t, actual, expected)
}

func TestGraphFlagExcluded(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	a := &runbase.Unit{Path: "a", FlagExcluded: true, Logger: l}
	b := &runbase.Unit{Path: "b", Logger: l}
	c := &runbase.Unit{Path: "c", Logger: l}
	d := &runbase.Unit{Path: "d", Logger: l}
	e := &runbase.Unit{Path: "e", Dependencies: []*runbase.Unit{a}, Logger: l}
	f := &runbase.Unit{Path: "f", FlagExcluded: true, Dependencies: []*runbase.Unit{a, b}, Logger: l}
	g := &runbase.Unit{Path: "g", Dependencies: []*runbase.Unit{e}, Logger: l}
	h := &runbase.Unit{Path: "h", Dependencies: []*runbase.Unit{g, f, c}, Logger: l}

	units := runbase.Units{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsForTest("/terragrunt.hcl")
	units.WriteDot(logger.CreateLogger(), &stdout, terragruntOptions)
	expected := strings.TrimSpace(`
digraph {
	"a" [color=red];
	"b" ;
	"c" ;
	"d" ;
	"e" ;
	"e" -> "a";
	"f" [color=red];
	"f" -> "a";
	"f" -> "b";
	"g" ;
	"g" -> "e";
	"h" ;
	"h" -> "g";
	"h" -> "f";
	"h" -> "c";
}
`)

	// clean string to work in cross-platform way
	actual := util.CleanString(stdout.String())
	expected = util.CleanString(expected)

	assert.Contains(t, actual, expected)
}

func TestCheckForCycles(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	////////////////////////////////////
	// These units have no dependencies
	////////////////////////////////////
	a := &runbase.Unit{Path: "a", Logger: l}
	b := &runbase.Unit{Path: "b", Logger: l}
	c := &runbase.Unit{Path: "c", Logger: l}
	d := &runbase.Unit{Path: "d", Logger: l}

	////////////////////////////////////
	// These units have dependencies, but no cycles
	////////////////////////////////////

	// e -> a
	e := &runbase.Unit{Path: "e", Dependencies: []*runbase.Unit{a}, Logger: l}

	// f -> a, b
	f := &runbase.Unit{Path: "f", Dependencies: []*runbase.Unit{a, b}, Logger: l}

	// g -> e -> a
	g := &runbase.Unit{Path: "g", Dependencies: []*runbase.Unit{e}, Logger: l}

	// h -> g -> e -> a
	// |            /
	//  --> f -> b
	// |
	//  --> c
	h := &runbase.Unit{Path: "h", Dependencies: []*runbase.Unit{g, f, c}, Logger: l}

	////////////////////////////////////
	// These units have dependencies and cycles
	////////////////////////////////////

	// i -> i
	i := &runbase.Unit{Path: "i", Dependencies: []*runbase.Unit{}, Logger: l}
	i.Dependencies = append(i.Dependencies, i)

	// j -> k -> j
	j := &runbase.Unit{Path: "j", Dependencies: []*runbase.Unit{}, Logger: l}
	k := &runbase.Unit{Path: "k", Dependencies: []*runbase.Unit{j}, Logger: l}
	j.Dependencies = append(j.Dependencies, k)

	// l -> m -> n -> o -> l
	unitL := &runbase.Unit{Path: "l", Dependencies: []*runbase.Unit{}, Logger: l}
	o := &runbase.Unit{Path: "o", Dependencies: []*runbase.Unit{unitL}, Logger: l}
	n := &runbase.Unit{Path: "n", Dependencies: []*runbase.Unit{o}, Logger: l}
	m := &runbase.Unit{Path: "m", Dependencies: []*runbase.Unit{n}, Logger: l}
	unitL.Dependencies = append(unitL.Dependencies, m)

	testCases := []struct {
		units    runbase.Units
		expected runbase.DependencyCycleError
	}{
		{[]*runbase.Unit{}, nil},
		{[]*runbase.Unit{a}, nil},
		{[]*runbase.Unit{a, b, c, d}, nil},
		{[]*runbase.Unit{a, e}, nil},
		{[]*runbase.Unit{a, b, f}, nil},
		{[]*runbase.Unit{a, e, g}, nil},
		{runbase.Units{a, b, c, e, f, g, h}, nil},
		{[]*runbase.Unit{i}, runbase.DependencyCycleError([]string{"i", "i"})},
		{[]*runbase.Unit{j, k}, runbase.DependencyCycleError([]string{"j", "k", "j"})},
		{[]*runbase.Unit{unitL, o, n, m}, runbase.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
		{[]*runbase.Unit{a, unitL, b, o, n, f, m, h}, runbase.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
	}

	for _, tc := range testCases {
		actual := tc.units.CheckForCycles()
		if tc.expected == nil {
			require.NoError(t, actual)
		} else if assert.Error(t, actual, "For units %v", tc.units) {
			var actualErr runbase.DependencyCycleError
			errors.As(actual, &actualErr)
			assert.Equal(t, []string(tc.expected), []string(actualErr), "For units %v", tc.units)
		}
	}
}

func TestRunUnitsNoUnits(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{},
			Report: report.NewReport(),
		},
	}

	err = runner.RunUnits(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
}

func TestRunUnitsOneUnitSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA},
			Report: report.NewReport(),
		},
	}

	err = runner.RunUnits(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunUnitsOneUnitAssumeAlreadyRan(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	unitA := &runbase.Unit{
		Path:                 "a",
		Dependencies:         runbase.Units{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		AssumeAlreadyApplied: true,
		Logger:               l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.False(t, aRan)
}

func TestRunUnitsReverseOrderOneUnitSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsReverseOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunUnitsIgnoreOrderOneUnitSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsIgnoreOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunUnitsOneUnitError(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	expectedErrA := errors.New("Expected error for unit a")
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunUnitsReverseOrderOneUnitError(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	expectedErrA := errors.New("Expected error for unit a")
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsReverseOrder(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunUnitsIgnoreOrderOneUnitError(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	expectedErrA := errors.New("Expected error for unit a")
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsIgnoreOrder(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunUnitsMultipleUnitsNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsMultipleUnitsNoDependenciesSuccessNoParallelism(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = 1
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)

	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsReverseOrderMultipleUnitsNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsReverseOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsIgnoreOrderMultipleUnitsNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsIgnoreOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsMultipleUnitsNoDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	require.NoError(t, optsErr)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err := runner.RunUnits(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsMultipleUnitsNoDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	expectedErrA := errors.New("Expected error for unit a")
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for unit c")
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrA, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsMultipleUnitsWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsMultipleUnitsWithDependenciesWithAssumeAlreadyRanSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:                 "c",
		Dependencies:         runbase.Units{unitB},
		Config:               config.TerragruntConfig{},
		Logger:               l,
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
		AssumeAlreadyApplied: true,
	}

	dRan := false
	unitD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{unitC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC, unitD},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.False(t, cRan)
	assert.True(t, dRan)
}

func TestRunUnitsReverseOrderMultipleUnitsWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsReverseOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsIgnoreOrderMultipleUnitsWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsIgnoreOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsMultipleUnitsWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrC := runbase.ProcessingUnitDependencyError{Unit: unitC, Dependency: unitB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.False(t, cRan)
}

func TestRunUnitsMultipleUnitsWithDependenciesOneFailureIgnoreDependencyErrors(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	terragruntOptionsA := optionsWithMockTerragruntCommand(t, "a", nil, &aRan)
	terragruntOptionsA.IgnoreDependencyErrors = true
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsA,
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	terragruntOptionsB := optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan)
	terragruntOptionsB.IgnoreDependencyErrors = true
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsB,
	}

	cRan := false
	terragruntOptionsC := optionsWithMockTerragruntCommand(t, "c", nil, &cRan)
	terragruntOptionsC.IgnoreDependencyErrors = true
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsC,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsReverseOrderMultipleUnitsWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrA := runbase.ProcessingUnitDependencyError{Unit: unitA, Dependency: unitB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsReverseOrder(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsIgnoreOrderMultipleUnitsWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsIgnoreOrder(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsMultipleUnitsWithDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	expectedErrA := errors.New("Expected error for unit a")
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrB := runbase.ProcessingUnitDependencyError{Unit: unitB, Dependency: unitA, Err: expectedErrA}
	expectedErrC := runbase.ProcessingUnitDependencyError{Unit: unitC, Dependency: unitB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrA, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.False(t, bRan)
	assert.False(t, cRan)
}

func TestRunUnitsIgnoreOrderMultipleUnitsWithDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	expectedErrA := errors.New("Expected error for unit a")
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnitsIgnoreOrder(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrA)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunUnitsMultipleUnitsWithDependenciesLargeGraphAllSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	dRan := false
	unitD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{unitA, unitB, unitC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	unitE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	unitF := &runbase.Unit{
		Path:              "f",
		Dependencies:      runbase.Units{unitE, unitD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC, unitD, unitE, unitF},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)

	require.NoError(t, err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
	assert.True(t, dRan)
	assert.True(t, eRan)
	assert.True(t, fRan)
}

func TestRunUnitsMultipleUnitsWithDependenciesLargeGraphPartialFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "large-graph-a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "large-graph-b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for unit large-graph-c")
	unitC := &runbase.Unit{
		Path:              "large-graph-c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-c", expectedErrC, &cRan),
	}

	dRan := false
	unitD := &runbase.Unit{
		Path:              "large-graph-d",
		Dependencies:      runbase.Units{unitA, unitB, unitC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-d", nil, &dRan),
	}

	eRan := false
	unitE := &runbase.Unit{
		Path:                 "large-graph-e",
		Dependencies:         runbase.Units{},
		Config:               config.TerragruntConfig{},
		Logger:               l,
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "large-graph-e", nil, &eRan),
		AssumeAlreadyApplied: true,
	}

	fRan := false
	unitF := &runbase.Unit{
		Path:              "large-graph-f",
		Dependencies:      runbase.Units{unitE, unitD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-f", nil, &fRan),
	}

	gRan := false
	unitG := &runbase.Unit{
		Path:              "large-graph-g",
		Dependencies:      runbase.Units{unitE},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-g", nil, &gRan),
	}

	expectedErrD := runbase.ProcessingUnitDependencyError{Unit: unitD, Dependency: unitC, Err: expectedErrC}
	expectedErrF := runbase.ProcessingUnitDependencyError{Unit: unitF, Dependency: unitD, Err: expectedErrD}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC, unitD, unitE, unitF, unitG},
			Report: report.NewReport(),
		},
	}
	err = runner.RunUnits(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrC, expectedErrD, expectedErrF)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
	assert.False(t, dRan)
	assert.False(t, eRan)
	assert.False(t, fRan)
	assert.True(t, gRan)
}

func TestRunUnitsReverseOrderMultipleUnitsWithDependenciesLargeGraphPartialFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	unitA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for unit c")
	unitC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	dRan := false
	unitD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{unitA, unitB, unitC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	unitE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	unitF := &runbase.Unit{
		Path:              "f",
		Dependencies:      runbase.Units{unitE, unitD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	expectedErrB := runbase.ProcessingUnitDependencyError{Unit: unitB, Dependency: unitC, Err: expectedErrC}
	expectedErrA := runbase.ProcessingUnitDependencyError{Unit: unitA, Dependency: unitB, Err: expectedErrB}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	require.NoError(t, optsErr)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{unitA, unitB, unitC, unitD, unitE, unitF},
			Report: report.NewReport(),
		},
	}
	err := runner.RunUnitsReverseOrder(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrC, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.False(t, bRan)
	assert.True(t, cRan)
	assert.True(t, dRan)
	assert.True(t, eRan)
	assert.True(t, fRan)
}
