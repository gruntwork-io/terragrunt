package configstack_test

import (
	"bytes"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraph(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	a := &common.Unit{Path: "a", Logger: l}
	b := &common.Unit{Path: "b", Logger: l}
	c := &common.Unit{Path: "c", Logger: l}
	d := &common.Unit{Path: "d", Logger: l}
	e := &common.Unit{Path: "e", Dependencies: []*common.Unit{a}, Logger: l}
	f := &common.Unit{Path: "f", Dependencies: []*common.Unit{a, b}, Logger: l}
	g := &common.Unit{Path: "g", Dependencies: []*common.Unit{e}, Logger: l}
	h := &common.Unit{Path: "h", Dependencies: []*common.Unit{g, f, c}, Logger: l}

	units := common.Units{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer

	terragruntOptions, err := options.NewTerragruntOptionsForTest("/terragrunt.hcl")
	require.NoError(t, err)
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

	a := &common.Unit{Path: "/config/a", Logger: l}
	b := &common.Unit{Path: "/config/b", Logger: l}
	c := &common.Unit{Path: "/config/c", Logger: l}
	d := &common.Unit{Path: "/config/d", Logger: l}
	e := &common.Unit{Path: "/config/alpha/beta/gamma/e", Dependencies: []*common.Unit{a}, Logger: l}
	f := &common.Unit{Path: "/config/alpha/beta/gamma/f", Dependencies: []*common.Unit{a, b}, Logger: l}
	g := &common.Unit{Path: "/config/alpha/g", Dependencies: []*common.Unit{e}, Logger: l}
	h := &common.Unit{Path: "/config/alpha/beta/h", Dependencies: []*common.Unit{g, f, c}, Logger: l}

	units := common.Units{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer

	terragruntOptions, err := options.NewTerragruntOptionsWithConfigPath("/config/terragrunt.hcl")
	require.NoError(t, err)
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

	a := &common.Unit{Path: "a", FlagExcluded: true, Logger: l}
	b := &common.Unit{Path: "b", Logger: l}
	c := &common.Unit{Path: "c", Logger: l}
	d := &common.Unit{Path: "d", Logger: l}
	e := &common.Unit{Path: "e", Dependencies: []*common.Unit{a}, Logger: l}
	f := &common.Unit{Path: "f", FlagExcluded: true, Dependencies: []*common.Unit{a, b}, Logger: l}
	g := &common.Unit{Path: "g", Dependencies: []*common.Unit{e}, Logger: l}
	h := &common.Unit{Path: "h", Dependencies: []*common.Unit{g, f, c}, Logger: l}

	units := common.Units{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer

	terragruntOptions, err := options.NewTerragruntOptionsForTest("/terragrunt.hcl")
	require.NoError(t, err)
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
	a := &common.Unit{Path: "a", Logger: l}
	b := &common.Unit{Path: "b", Logger: l}
	c := &common.Unit{Path: "c", Logger: l}
	d := &common.Unit{Path: "d", Logger: l}

	////////////////////////////////////
	// These units have dependencies, but no cycles
	////////////////////////////////////

	// e -> a
	e := &common.Unit{Path: "e", Dependencies: []*common.Unit{a}, Logger: l}

	// f -> a, b
	f := &common.Unit{Path: "f", Dependencies: []*common.Unit{a, b}, Logger: l}

	// g -> e -> a
	g := &common.Unit{Path: "g", Dependencies: []*common.Unit{e}, Logger: l}

	// h -> g -> e -> a
	// |            /
	//  --> f -> b
	// |
	//  --> c
	h := &common.Unit{Path: "h", Dependencies: []*common.Unit{g, f, c}, Logger: l}

	////////////////////////////////////
	// These units have dependencies and cycles
	////////////////////////////////////

	// i -> i
	i := &common.Unit{Path: "i", Dependencies: []*common.Unit{}, Logger: l}
	i.Dependencies = append(i.Dependencies, i)

	// j -> k -> j
	j := &common.Unit{Path: "j", Dependencies: []*common.Unit{}, Logger: l}
	k := &common.Unit{Path: "k", Dependencies: []*common.Unit{j}, Logger: l}
	j.Dependencies = append(j.Dependencies, k)

	// l -> m -> n -> o -> l
	unitNameL := &common.Unit{Path: "l", Dependencies: []*common.Unit{}, Logger: l}
	o := &common.Unit{Path: "o", Dependencies: []*common.Unit{unitNameL}, Logger: l}
	n := &common.Unit{Path: "n", Dependencies: []*common.Unit{o}, Logger: l}
	m := &common.Unit{Path: "m", Dependencies: []*common.Unit{n}, Logger: l}
	unitNameL.Dependencies = append(unitNameL.Dependencies, m)

	testCases := []struct {
		units    common.Units
		expected common.DependencyCycleError
	}{
		{[]*common.Unit{}, nil},
		{[]*common.Unit{a}, nil},
		{[]*common.Unit{a, b, c, d}, nil},
		{[]*common.Unit{a, e}, nil},
		{[]*common.Unit{a, b, f}, nil},
		{[]*common.Unit{a, e, g}, nil},
		{common.Units{a, b, c, e, f, g, h}, nil},
		{[]*common.Unit{i}, common.DependencyCycleError([]string{"i", "i"})},
		{[]*common.Unit{j, k}, common.DependencyCycleError([]string{"j", "k", "j"})},
		{[]*common.Unit{unitNameL, o, n, m}, common.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
		{[]*common.Unit{a, unitNameL, b, o, n, f, m, h}, common.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
	}

	for _, tc := range testCases {
		actual := tc.units.CheckForCycles()
		if tc.expected == nil {
			require.NoError(t, actual)
		} else if assert.Error(t, actual, "For units %v", tc.units) {
			var actualErr common.DependencyCycleError
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
		Stack: &common.Stack{
			Units:  common.Units{},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
		},
	}

	err = runner.RunUnits(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
}

func TestRunUnitsOneUnitSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:                 "a",
		Dependencies:         common.Units{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		AssumeAlreadyApplied: true,
		Logger:               l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = 1
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	require.NoError(t, optsErr)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for unit c")
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:                 "/c",
		Dependencies:         common.Units{unitB},
		Config:               config.TerragruntConfig{},
		Logger:               l,
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
		AssumeAlreadyApplied: true,
	}

	dRan := false
	unitD := &common.Unit{
		Path:              "/d",
		Dependencies:      common.Units{unitC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC, unitD},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrC := common.ProcessingUnitDependencyError{Unit: unitC, Dependency: unitB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsA,
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	terragruntOptionsB := optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan)
	terragruntOptionsB.IgnoreDependencyErrors = true
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsB,
	}

	cRan := false
	terragruntOptionsC := optionsWithMockTerragruntCommand(t, "c", nil, &cRan)
	terragruntOptionsC.IgnoreDependencyErrors = true
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsC,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrA := common.ProcessingUnitDependencyError{Unit: unitA, Dependency: unitB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for unit b")
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrB := common.ProcessingUnitDependencyError{Unit: unitB, Dependency: unitA, Err: expectedErrA}
	expectedErrC := common.ProcessingUnitDependencyError{Unit: unitC, Dependency: unitB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	dRan := false
	unitD := &common.Unit{
		Path:              "/d",
		Dependencies:      common.Units{unitA, unitB, unitC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	unitE := &common.Unit{
		Path:              "/e",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	unitF := &common.Unit{
		Path:              "/f",
		Dependencies:      common.Units{unitE, unitD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC, unitD, unitE, unitF},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/large-graph-a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/large-graph-b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for unit large-graph-c")
	unitC := &common.Unit{
		Path:              "/large-graph-c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-c", expectedErrC, &cRan),
	}

	dRan := false
	unitD := &common.Unit{
		Path:              "/large-graph-d",
		Dependencies:      common.Units{unitA, unitB, unitC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-d", nil, &dRan),
	}

	eRan := false
	unitE := &common.Unit{
		Path:                 "/large-graph-e",
		Dependencies:         common.Units{},
		Config:               config.TerragruntConfig{},
		Logger:               l,
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "large-graph-e", nil, &eRan),
		AssumeAlreadyApplied: true,
	}

	fRan := false
	unitF := &common.Unit{
		Path:              "/large-graph-f",
		Dependencies:      common.Units{unitE, unitD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-f", nil, &fRan),
	}

	gRan := false
	unitG := &common.Unit{
		Path:              "/large-graph-g",
		Dependencies:      common.Units{unitE},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-g", nil, &gRan),
	}

	expectedErrD := common.ProcessingUnitDependencyError{Unit: unitD, Dependency: unitC, Err: expectedErrC}
	expectedErrF := common.ProcessingUnitDependencyError{Unit: unitF, Dependency: unitD, Err: expectedErrD}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC, unitD, unitE, unitF, unitG},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
	unitA := &common.Unit{
		Path:              "/a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	unitB := &common.Unit{
		Path:              "/b",
		Dependencies:      common.Units{unitA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for unit c")
	unitC := &common.Unit{
		Path:              "/c",
		Dependencies:      common.Units{unitB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	dRan := false
	unitD := &common.Unit{
		Path:              "/d",
		Dependencies:      common.Units{unitA, unitB, unitC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	unitE := &common.Unit{
		Path:              "/e",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	unitF := &common.Unit{
		Path:              "/f",
		Dependencies:      common.Units{unitE, unitD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	expectedErrB := common.ProcessingUnitDependencyError{Unit: unitB, Dependency: unitC, Err: expectedErrC}
	expectedErrA := common.ProcessingUnitDependencyError{Unit: unitA, Dependency: unitB, Err: expectedErrB}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	require.NoError(t, optsErr)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{unitA, unitB, unitC, unitD, unitE, unitF},
			Report: report.NewReport().WithWorkingDir(t.TempDir()),
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
