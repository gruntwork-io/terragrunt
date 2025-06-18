package configstack_test

import (
	"bytes"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
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

	a := &common.Unit{Path: "a", Logger: l}
	b := &common.Unit{Path: "b", Logger: l}
	c := &common.Unit{Path: "c", Logger: l}
	d := &common.Unit{Path: "d", Logger: l}
	e := &common.Unit{Path: "e", Dependencies: []*common.Unit{a}, Logger: l}
	f := &common.Unit{Path: "f", Dependencies: []*common.Unit{a, b}, Logger: l}
	g := &common.Unit{Path: "g", Dependencies: []*common.Unit{e}, Logger: l}
	h := &common.Unit{Path: "h", Dependencies: []*common.Unit{g, f, c}, Logger: l}

	modules := common.Units{a, b, c, d, e, f, g, h}

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

	a := &common.Unit{Path: "/config/a", Logger: l}
	b := &common.Unit{Path: "/config/b", Logger: l}
	c := &common.Unit{Path: "/config/c", Logger: l}
	d := &common.Unit{Path: "/config/d", Logger: l}
	e := &common.Unit{Path: "/config/alpha/beta/gamma/e", Dependencies: []*common.Unit{a}, Logger: l}
	f := &common.Unit{Path: "/config/alpha/beta/gamma/f", Dependencies: []*common.Unit{a, b}, Logger: l}
	g := &common.Unit{Path: "/config/alpha/g", Dependencies: []*common.Unit{e}, Logger: l}
	h := &common.Unit{Path: "/config/alpha/beta/h", Dependencies: []*common.Unit{g, f, c}, Logger: l}

	modules := common.Units{a, b, c, d, e, f, g, h}

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

	a := &common.Unit{Path: "a", FlagExcluded: true, Logger: l}
	b := &common.Unit{Path: "b", Logger: l}
	c := &common.Unit{Path: "c", Logger: l}
	d := &common.Unit{Path: "d", Logger: l}
	e := &common.Unit{Path: "e", Dependencies: []*common.Unit{a}, Logger: l}
	f := &common.Unit{Path: "f", FlagExcluded: true, Dependencies: []*common.Unit{a, b}, Logger: l}
	g := &common.Unit{Path: "g", Dependencies: []*common.Unit{e}, Logger: l}
	h := &common.Unit{Path: "h", Dependencies: []*common.Unit{g, f, c}, Logger: l}

	modules := common.Units{a, b, c, d, e, f, g, h}

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
	a := &common.Unit{Path: "a", Logger: l}
	b := &common.Unit{Path: "b", Logger: l}
	c := &common.Unit{Path: "c", Logger: l}
	d := &common.Unit{Path: "d", Logger: l}

	////////////////////////////////////
	// These modules have dependencies, but no cycles
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
	// These modules have dependencies and cycles
	////////////////////////////////////

	// i -> i
	i := &common.Unit{Path: "i", Dependencies: []*common.Unit{}, Logger: l}
	i.Dependencies = append(i.Dependencies, i)

	// j -> k -> j
	j := &common.Unit{Path: "j", Dependencies: []*common.Unit{}, Logger: l}
	k := &common.Unit{Path: "k", Dependencies: []*common.Unit{j}, Logger: l}
	j.Dependencies = append(j.Dependencies, k)

	// l -> m -> n -> o -> l
	moduleL := &common.Unit{Path: "l", Dependencies: []*common.Unit{}, Logger: l}
	o := &common.Unit{Path: "o", Dependencies: []*common.Unit{moduleL}, Logger: l}
	n := &common.Unit{Path: "n", Dependencies: []*common.Unit{o}, Logger: l}
	m := &common.Unit{Path: "m", Dependencies: []*common.Unit{n}, Logger: l}
	moduleL.Dependencies = append(moduleL.Dependencies, m)

	testCases := []struct {
		modules  common.Units
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
		{[]*common.Unit{moduleL, o, n, m}, common.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
		{[]*common.Unit{a, moduleL, b, o, n, f, m, h}, common.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
	}

	for _, tc := range testCases {
		actual := tc.modules.CheckForCycles()
		if tc.expected == nil {
			require.NoError(t, actual)
		} else if assert.Error(t, actual, "For modules %v", tc.modules) {
			var actualErr common.DependencyCycleError
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
		Stack: &common.Stack{
			Units:  common.Units{},
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
	moduleA := &common.Unit{
		Path:              "a",
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
			Units:  common.Units{moduleA},
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
	moduleA := &common.Unit{
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
			Units:  common.Units{moduleA},
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
	moduleA := &common.Unit{
		Path:              "a",
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
			Units:  common.Units{moduleA},
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
	moduleA := &common.Unit{
		Path:              "a",
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
			Units:  common.Units{moduleA},
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
	moduleA := &common.Unit{
		Path:              "a",
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
			Units:  common.Units{moduleA},
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
	moduleA := &common.Unit{
		Path:              "a",
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
			Units:  common.Units{moduleA},
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
	moduleA := &common.Unit{
		Path:              "a",
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
			Units:  common.Units{moduleA},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
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
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
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
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
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
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
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
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
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
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module c")
	moduleC := &common.Unit{
		Path:              "c",
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
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:                 "c",
		Dependencies:         common.Units{moduleB},
		Config:               config.TerragruntConfig{},
		Logger:               l,
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
		AssumeAlreadyApplied: true,
	}

	dRan := false
	moduleD := &common.Unit{
		Path:              "d",
		Dependencies:      common.Units{moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC, moduleD},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrC := common.ProcessingModuleDependencyError{Module: moduleC, Dependency: moduleB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsA,
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	terragruntOptionsB := optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan)
	terragruntOptionsB.IgnoreDependencyErrors = true
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsB,
	}

	cRan := false
	terragruntOptionsC := optionsWithMockTerragruntCommand(t, "c", nil, &cRan)
	terragruntOptionsC.IgnoreDependencyErrors = true
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsC,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrA := common.ProcessingModuleDependencyError{Module: moduleA, Dependency: moduleB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrB := common.ProcessingModuleDependencyError{moduleB, moduleA, expectedErrA}
	expectedErrC := common.ProcessingModuleDependencyError{moduleC, moduleB, expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	dRan := false
	moduleD := &common.Unit{
		Path:              "d",
		Dependencies:      common.Units{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &common.Unit{
		Path:              "e",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &common.Unit{
		Path:              "f",
		Dependencies:      common.Units{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF},
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
	moduleA := &common.Unit{
		Path:              "large-graph-a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "large-graph-b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module large-graph-c")
	moduleC := &common.Unit{
		Path:              "large-graph-c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &common.Unit{
		Path:              "large-graph-d",
		Dependencies:      common.Units{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-d", nil, &dRan),
	}

	eRan := false
	moduleE := &common.Unit{
		Path:                 "large-graph-e",
		Dependencies:         common.Units{},
		Config:               config.TerragruntConfig{},
		Logger:               l,
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "large-graph-e", nil, &eRan),
		AssumeAlreadyApplied: true,
	}

	fRan := false
	moduleF := &common.Unit{
		Path:              "large-graph-f",
		Dependencies:      common.Units{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-f", nil, &fRan),
	}

	gRan := false
	moduleG := &common.Unit{
		Path:              "large-graph-g",
		Dependencies:      common.Units{moduleE},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-g", nil, &gRan),
	}

	expectedErrD := common.ProcessingModuleDependencyError{Module: moduleD, Dependency: moduleC, Err: expectedErrC}
	expectedErrF := common.ProcessingModuleDependencyError{Module: moduleF, Dependency: moduleD, Err: expectedErrD}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF, moduleG},
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
	moduleA := &common.Unit{
		Path:              "a",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &common.Unit{
		Path:              "b",
		Dependencies:      common.Units{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module c")
	moduleC := &common.Unit{
		Path:              "c",
		Dependencies:      common.Units{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &common.Unit{
		Path:              "d",
		Dependencies:      common.Units{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &common.Unit{
		Path:              "e",
		Dependencies:      common.Units{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &common.Unit{
		Path:              "f",
		Dependencies:      common.Units{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	expectedErrB := common.ProcessingModuleDependencyError{Module: moduleB, Dependency: moduleC, Err: expectedErrC}
	expectedErrA := common.ProcessingModuleDependencyError{Module: moduleA, Dependency: moduleB, Err: expectedErrB}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	require.NoError(t, optsErr)

	opts.Parallelism = options.DefaultParallelism
	runner := configstack.Runner{
		Stack: &common.Stack{
			Units:  common.Units{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF},
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
