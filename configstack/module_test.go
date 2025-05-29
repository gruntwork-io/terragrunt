package configstack_test

import (
	"bytes"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraph(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	a := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "a", Logger: l}
	b := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "b", Logger: l}
	c := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "c", Logger: l}
	d := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "d", Logger: l}
	e := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "e", Dependencies: []*configstack.TerraformModule{a}, Logger: l}
	f := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "f", Dependencies: []*configstack.TerraformModule{a, b}, Logger: l}
	g := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "g", Dependencies: []*configstack.TerraformModule{e}, Logger: l}
	h := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "h", Dependencies: []*configstack.TerraformModule{g, f, c}, Logger: l}

	modules := configstack.TerraformModules{a, b, c, d, e, f, g, h}

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

	a := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "/config/a", Logger: l}
	b := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "/config/b", Logger: l}
	c := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "/config/c", Logger: l}
	d := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "/config/d", Logger: l}
	e := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "/config/alpha/beta/gamma/e", Dependencies: []*configstack.TerraformModule{a}, Logger: l}
	f := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "/config/alpha/beta/gamma/f", Dependencies: []*configstack.TerraformModule{a, b}, Logger: l}
	g := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "/config/alpha/g", Dependencies: []*configstack.TerraformModule{e}, Logger: l}
	h := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "/config/alpha/beta/h", Dependencies: []*configstack.TerraformModule{g, f, c}, Logger: l}

	modules := configstack.TerraformModules{a, b, c, d, e, f, g, h}

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

	a := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "a", FlagExcluded: true, Logger: l}
	b := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "b", Logger: l}
	c := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "c", Logger: l}
	d := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "d", Logger: l}
	e := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "e", Dependencies: []*configstack.TerraformModule{a}, Logger: l}
	f := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "f", FlagExcluded: true, Dependencies: []*configstack.TerraformModule{a, b}, Logger: l}
	g := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "g", Dependencies: []*configstack.TerraformModule{e}, Logger: l}
	h := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "h", Dependencies: []*configstack.TerraformModule{g, f, c}, Logger: l}

	modules := configstack.TerraformModules{a, b, c, d, e, f, g, h}

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
	a := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "a", Logger: l}
	b := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "b", Logger: l}
	c := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "c", Logger: l}
	d := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "d", Logger: l}

	////////////////////////////////////
	// These modules have dependencies, but no cycles
	////////////////////////////////////

	// e -> a
	e := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "e", Dependencies: []*configstack.TerraformModule{a}, Logger: l}

	// f -> a, b
	f := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "f", Dependencies: []*configstack.TerraformModule{a, b}, Logger: l}

	// g -> e -> a
	g := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "g", Dependencies: []*configstack.TerraformModule{e}, Logger: l}

	// h -> g -> e -> a
	// |            /
	//  --> f -> b
	// |
	//  --> c
	h := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "h", Dependencies: []*configstack.TerraformModule{g, f, c}, Logger: l}

	////////////////////////////////////
	// These modules have dependencies and cycles
	////////////////////////////////////

	// i -> i
	i := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "i", Dependencies: []*configstack.TerraformModule{}, Logger: l}
	i.Dependencies = append(i.Dependencies, i)

	// j -> k -> j
	j := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "j", Dependencies: []*configstack.TerraformModule{}, Logger: l}
	k := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "k", Dependencies: []*configstack.TerraformModule{j}, Logger: l}
	j.Dependencies = append(j.Dependencies, k)

	// l -> m -> n -> o -> l
	moduleL := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "l", Dependencies: []*configstack.TerraformModule{}, Logger: l}
	o := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "o", Dependencies: []*configstack.TerraformModule{moduleL}, Logger: l}
	n := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "n", Dependencies: []*configstack.TerraformModule{o}, Logger: l}
	m := &configstack.TerraformModule{Stack: &configstack.Stack{}, Path: "m", Dependencies: []*configstack.TerraformModule{n}, Logger: l}
	moduleL.Dependencies = append(moduleL.Dependencies, m)

	testCases := []struct {
		modules  configstack.TerraformModules
		expected configstack.DependencyCycleError
	}{
		{[]*configstack.TerraformModule{}, nil},
		{[]*configstack.TerraformModule{a}, nil},
		{[]*configstack.TerraformModule{a, b, c, d}, nil},
		{[]*configstack.TerraformModule{a, e}, nil},
		{[]*configstack.TerraformModule{a, b, f}, nil},
		{[]*configstack.TerraformModule{a, e, g}, nil},
		{[]*configstack.TerraformModule{a, b, c, e, f, g, h}, nil},
		{[]*configstack.TerraformModule{i}, configstack.DependencyCycleError([]string{"i", "i"})},
		{[]*configstack.TerraformModule{j, k}, configstack.DependencyCycleError([]string{"j", "k", "j"})},
		{[]*configstack.TerraformModule{moduleL, o, n, m}, configstack.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
		{[]*configstack.TerraformModule{a, moduleL, b, o, n, f, m, h}, configstack.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
	}

	for _, tc := range testCases {
		actual := tc.modules.CheckForCycles()
		if tc.expected == nil {
			require.NoError(t, actual)
		} else if assert.Error(t, actual, "For modules %v", tc.modules) {
			var actualErr configstack.DependencyCycleError
			errors.As(actual, &actualErr)
			assert.Equal(t, []string(tc.expected), []string(actualErr), "For modules %v", tc.modules)
		}
	}
}

func TestRunModulesNoModules(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)
}

func TestRunModulesOneModuleSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesOneModuleAssumeAlreadyRan(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:                &configstack.Stack{},
		Path:                 "a",
		Dependencies:         configstack.TerraformModules{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		AssumeAlreadyApplied: true,
		Logger:               l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.False(t, aRan)
}

func TestRunModulesReverseOrderOneModuleSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModulesReverseOrder(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesIgnoreOrderOneModuleSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModulesIgnoreOrder(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesOneModuleError(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesReverseOrderOneModuleError(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
		Logger:            l,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModulesReverseOrder(t.Context(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesIgnoreOrderOneModuleError(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModulesIgnoreOrder(t.Context(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesSuccessNoParallelism(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(t.Context(), opts, 1)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesReverseOrderMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesReverseOrder(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	require.NoError(t, optsErr)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err := modules.RunModules(t.Context(), opts, options.DefaultParallelism)
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
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module c")
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesWithAssumeAlreadyRanSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:                &configstack.Stack{},
		Path:                 "c",
		Dependencies:         configstack.TerraformModules{moduleB},
		Config:               config.TerragruntConfig{},
		Logger:               l,
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
		AssumeAlreadyApplied: true,
	}

	dRan := false
	moduleD := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
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
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesReverseOrder(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(t.Context(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrC := configstack.ProcessingModuleDependencyError{Module: moduleC, Dependency: moduleB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
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
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsA,
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	terragruntOptionsB := optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan)
	terragruntOptionsB.IgnoreDependencyErrors = true
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsB,
	}

	cRan := false
	terragruntOptionsC := optionsWithMockTerragruntCommand(t, "c", nil, &cRan)
	terragruntOptionsC.IgnoreDependencyErrors = true
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: terragruntOptionsC,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesReverseOrderMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrA := configstack.ProcessingModuleDependencyError{Module: moduleA, Dependency: moduleB, Err: expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesReverseOrder(t.Context(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(t.Context(), opts, options.DefaultParallelism)
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
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrB := configstack.ProcessingModuleDependencyError{moduleB, moduleA, expectedErrA}
	expectedErrC := configstack.ProcessingModuleDependencyError{moduleC, moduleB, expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
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
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(t.Context(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesLargeGraphAllSuccess(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	dRan := false
	moduleD := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "e",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "f",
		Dependencies:      configstack.TerraformModules{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
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
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "large-graph-a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "large-graph-b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module large-graph-c")
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "large-graph-c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "large-graph-d",
		Dependencies:      configstack.TerraformModules{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-d", nil, &dRan),
	}

	eRan := false
	moduleE := &configstack.TerraformModule{
		Stack:                &configstack.Stack{},
		Path:                 "large-graph-e",
		Dependencies:         configstack.TerraformModules{},
		Config:               config.TerragruntConfig{},
		Logger:               l,
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "large-graph-e", nil, &eRan),
		AssumeAlreadyApplied: true,
	}

	fRan := false
	moduleF := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "large-graph-f",
		Dependencies:      configstack.TerraformModules{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-f", nil, &fRan),
	}

	gRan := false
	moduleG := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "large-graph-g",
		Dependencies:      configstack.TerraformModules{moduleE},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-g", nil, &gRan),
	}

	expectedErrD := configstack.ProcessingModuleDependencyError{Module: moduleD, Dependency: moduleC, Err: expectedErrC}
	expectedErrF := configstack.ProcessingModuleDependencyError{Module: moduleF, Dependency: moduleD, Err: expectedErrD}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF, moduleG}
	err = modules.RunModules(t.Context(), opts, options.DefaultParallelism)
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
	moduleA := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module c")
	moduleC := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "e",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &configstack.TerraformModule{
		Stack:             &configstack.Stack{},
		Path:              "f",
		Dependencies:      configstack.TerraformModules{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		Logger:            l,
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	expectedErrB := configstack.ProcessingModuleDependencyError{Module: moduleB, Dependency: moduleC, Err: expectedErrC}
	expectedErrA := configstack.ProcessingModuleDependencyError{Module: moduleA, Dependency: moduleB, Err: expectedErrB}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	require.NoError(t, optsErr)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF}
	err := modules.RunModulesReverseOrder(t.Context(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrC, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.False(t, bRan)
	assert.True(t, cRan)
	assert.True(t, dRan)
	assert.True(t, eRan)
	assert.True(t, fRan)
}
