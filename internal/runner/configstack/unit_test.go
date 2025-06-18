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

	modules := runbase.Units{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsForTest("/terragrunt.hcl")
	modules.WriteDot(logger.CreateLogger(), &stdout, terragruntOptions)
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

	modules := runbase.Units{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsWithConfigPath("/config/terragrunt.hcl")
	modules.WriteDot(logger.CreateLogger(), &stdout, terragruntOptions)
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

	modules := runbase.Units{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsForTest("/terragrunt.hcl")
	modules.WriteDot(logger.CreateLogger(), &stdout, terragruntOptions)
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
	// These modules have no dependencies
	////////////////////////////////////
	a := &runbase.Unit{Path: "a", Logger: l}
	b := &runbase.Unit{Path: "b", Logger: l}
	c := &runbase.Unit{Path: "c", Logger: l}
	d := &runbase.Unit{Path: "d", Logger: l}

	////////////////////////////////////
	// These modules have dependencies, but no cycles
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
	// These modules have dependencies and cycles
	////////////////////////////////////

	// i -> i
	i := &runbase.Unit{Path: "i", Dependencies: []*runbase.Unit{}, Logger: l}
	i.Dependencies = append(i.Dependencies, i)

	// j -> k -> j
	j := &runbase.Unit{Path: "j", Dependencies: []*runbase.Unit{}, Logger: l}
	k := &runbase.Unit{Path: "k", Dependencies: []*runbase.Unit{j}, Logger: l}
	j.Dependencies = append(j.Dependencies, k)

	// l -> m -> n -> o -> l
	moduleL := &runbase.Unit{Path: "l", Dependencies: []*runbase.Unit{}, Logger: l}
	o := &runbase.Unit{Path: "o", Dependencies: []*runbase.Unit{moduleL}, Logger: l}
	n := &runbase.Unit{Path: "n", Dependencies: []*runbase.Unit{o}, Logger: l}
	m := &runbase.Unit{Path: "m", Dependencies: []*runbase.Unit{n}, Logger: l}
	moduleL.Dependencies = append(moduleL.Dependencies, m)

	testCases := []struct {
		modules  runbase.Units
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
		{[]*runbase.Unit{moduleL, o, n, m}, runbase.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
		{[]*runbase.Unit{a, moduleL, b, o, n, f, m, h}, runbase.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
	}

	for _, tc := range testCases {
		actual := tc.modules.CheckForCycles()
		if tc.expected == nil {
			require.NoError(t, actual)
		} else if assert.Error(t, actual, "For modules %v", tc.modules) {
			var actualErr runbase.DependencyCycleError
			errors.As(actual, &actualErr)
			assert.Equal(t, []string(tc.expected), []string(actualErr), "For modules %v", tc.modules)
		}
	}
}

func TestRunModulesNoModules(t *testing.T) {
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

	err = runner.RunModules(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
}

func TestRunModulesOneModuleSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	moduleA := &runbase.Unit{
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
			Units:  runbase.Units{moduleA},
			Report: report.NewReport(),
		},
	}

	err = runner.RunModules(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesOneModuleAssumeAlreadyRan(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	moduleA := &runbase.Unit{
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
			Units:  runbase.Units{moduleA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.False(t, aRan)
}

func TestRunModulesReverseOrderOneModuleSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	moduleA := &runbase.Unit{
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
			Units:  runbase.Units{moduleA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesReverseOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesIgnoreOrderOneModuleSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	moduleA := &runbase.Unit{
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
			Units:  runbase.Units{moduleA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesIgnoreOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesOneModuleError(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &runbase.Unit{
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
			Units:  runbase.Units{moduleA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesReverseOrderOneModuleError(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &runbase.Unit{
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
			Units:  runbase.Units{moduleA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesReverseOrder(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesIgnoreOrderOneModuleError(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &runbase.Unit{
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
			Units:  runbase.Units{moduleA},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesIgnoreOrder(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
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
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesSuccessNoParallelism(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
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
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)

	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesReverseOrderMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
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
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesReverseOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
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
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesIgnoreOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
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
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err := runner.RunModules(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module c")
	moduleC := &runbase.Unit{
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
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrA, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesWithAssumeAlreadyRanSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
		Path:                 "c",
		Dependencies:         runbase.Units{moduleB},
		Config:               config.TerragruntConfig{},
		Logger:               l,
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
		AssumeAlreadyApplied: true,
	}

	dRan := false
	moduleD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC, moduleD},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.False(t, cRan)
	assert.True(t, dRan)
}

func TestRunModulesReverseOrderMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesReverseOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesIgnoreOrder(t.Context(), opts)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrC := runbase.ProcessingModuleDependencyError{Module: moduleC, Dependency: moduleB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.False(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesOneFailureIgnoreDependencyErrors(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	terragruntOptionsA := optionsWithMockTerragruntCommand(t, "a", nil, &aRan)
	terragruntOptionsA.IgnoreDependencyErrors = true
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsA,
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	terragruntOptionsB := optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan)
	terragruntOptionsB.IgnoreDependencyErrors = true
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsB,
	}

	cRan := false
	terragruntOptionsC := optionsWithMockTerragruntCommand(t, "c", nil, &cRan)
	terragruntOptionsC.IgnoreDependencyErrors = true
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsC,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesReverseOrderMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrA := runbase.ProcessingModuleDependencyError{Module: moduleA, Dependency: moduleB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesReverseOrder(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesIgnoreOrder(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrB := runbase.ProcessingModuleDependencyError{moduleB, moduleA, expectedErrA}
	expectedErrC := runbase.ProcessingModuleDependencyError{moduleC, moduleB, expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrA, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.False(t, bRan)
	assert.False(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModulesIgnoreOrder(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrA)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesLargeGraphAllSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	dRan := false
	moduleD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &runbase.Unit{
		Path:              "f",
		Dependencies:      runbase.Units{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)

	require.NoError(t, err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
	assert.True(t, dRan)
	assert.True(t, eRan)
	assert.True(t, fRan)
}

func TestRunModulesMultipleModulesWithDependenciesLargeGraphPartialFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "large-graph-a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "large-graph-b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module large-graph-c")
	moduleC := &runbase.Unit{
		Path:              "large-graph-c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &runbase.Unit{
		Path:              "large-graph-d",
		Dependencies:      runbase.Units{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-d", nil, &dRan),
	}

	eRan := false
	moduleE := &runbase.Unit{
		Path:                 "large-graph-e",
		Dependencies:         runbase.Units{},
		Config:               config.TerragruntConfig{},
		Logger:               l,
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "large-graph-e", nil, &eRan),
		AssumeAlreadyApplied: true,
	}

	fRan := false
	moduleF := &runbase.Unit{
		Path:              "large-graph-f",
		Dependencies:      runbase.Units{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-f", nil, &fRan),
	}

	gRan := false
	moduleG := &runbase.Unit{
		Path:              "large-graph-g",
		Dependencies:      runbase.Units{moduleE},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-g", nil, &gRan),
	}

	expectedErrD := runbase.ProcessingModuleDependencyError{Module: moduleD, Dependency: moduleC, Err: expectedErrC}
	expectedErrF := runbase.ProcessingModuleDependencyError{Module: moduleF, Dependency: moduleD, Err: expectedErrD}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF, moduleG},
			Report: report.NewReport(),
		},
	}
	err = runner.RunModules(t.Context(), opts)

	assertMultiErrorContains(t, err, expectedErrC, expectedErrD, expectedErrF)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
	assert.False(t, dRan)
	assert.False(t, eRan)
	assert.False(t, fRan)
	assert.True(t, gRan)
}

func TestRunModulesReverseOrderMultipleModulesWithDependenciesLargeGraphPartialFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &runbase.Unit{
		Path:              "a",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &runbase.Unit{
		Path:              "b",
		Dependencies:      runbase.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module c")
	moduleC := &runbase.Unit{
		Path:              "c",
		Dependencies:      runbase.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &runbase.Unit{
		Path:              "d",
		Dependencies:      runbase.Units{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &runbase.Unit{
		Path:              "e",
		Dependencies:      runbase.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &runbase.Unit{
		Path:              "f",
		Dependencies:      runbase.Units{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	expectedErrB := runbase.ProcessingModuleDependencyError{Module: moduleB, Dependency: moduleC, Err: expectedErrC}
	expectedErrA := runbase.ProcessingModuleDependencyError{Module: moduleA, Dependency: moduleB, Err: expectedErrB}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	require.NoError(t, optsErr)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &runbase.Stack{
			Units:  runbase.Units{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF},
			Report: report.NewReport(),
		},
	}
	err := runner.RunModulesReverseOrder(t.Context(), opts)
	assertMultiErrorContains(t, err, expectedErrC, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.False(t, bRan)
	assert.True(t, cRan)
	assert.True(t, dRan)
	assert.True(t, eRan)
	assert.True(t, fRan)
}
