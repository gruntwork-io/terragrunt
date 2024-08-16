package configstack_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraph(t *testing.T) {
	t.Parallel()

	a := &configstack.TerraformModule{Path: "a"}
	b := &configstack.TerraformModule{Path: "b"}
	c := &configstack.TerraformModule{Path: "c"}
	d := &configstack.TerraformModule{Path: "d"}
	e := &configstack.TerraformModule{Path: "e", Dependencies: []*configstack.TerraformModule{a}}
	f := &configstack.TerraformModule{Path: "f", Dependencies: []*configstack.TerraformModule{a, b}}
	g := &configstack.TerraformModule{Path: "g", Dependencies: []*configstack.TerraformModule{e}}
	h := &configstack.TerraformModule{Path: "h", Dependencies: []*configstack.TerraformModule{g, f, c}}

	modules := configstack.TerraformModules{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsForTest("/terragrunt.hcl")
	modules.WriteDot(&stdout, terragruntOptions)
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
	assert.True(t, strings.Contains(stdout.String(), expected))
}

func TestGraphTrimPrefix(t *testing.T) {
	t.Parallel()

	a := &configstack.TerraformModule{Path: "/config/a"}
	b := &configstack.TerraformModule{Path: "/config/b"}
	c := &configstack.TerraformModule{Path: "/config/c"}
	d := &configstack.TerraformModule{Path: "/config/d"}
	e := &configstack.TerraformModule{Path: "/config/alpha/beta/gamma/e", Dependencies: []*configstack.TerraformModule{a}}
	f := &configstack.TerraformModule{Path: "/config/alpha/beta/gamma/f", Dependencies: []*configstack.TerraformModule{a, b}}
	g := &configstack.TerraformModule{Path: "/config/alpha/g", Dependencies: []*configstack.TerraformModule{e}}
	h := &configstack.TerraformModule{Path: "/config/alpha/beta/h", Dependencies: []*configstack.TerraformModule{g, f, c}}

	modules := configstack.TerraformModules{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsWithConfigPath("/config/terragrunt.hcl")
	modules.WriteDot(&stdout, terragruntOptions)
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
	assert.True(t, strings.Contains(stdout.String(), expected))
}

func TestGraphFlagExcluded(t *testing.T) {
	t.Parallel()

	a := &configstack.TerraformModule{Path: "a", FlagExcluded: true}
	b := &configstack.TerraformModule{Path: "b"}
	c := &configstack.TerraformModule{Path: "c"}
	d := &configstack.TerraformModule{Path: "d"}
	e := &configstack.TerraformModule{Path: "e", Dependencies: []*configstack.TerraformModule{a}}
	f := &configstack.TerraformModule{Path: "f", FlagExcluded: true, Dependencies: []*configstack.TerraformModule{a, b}}
	g := &configstack.TerraformModule{Path: "g", Dependencies: []*configstack.TerraformModule{e}}
	h := &configstack.TerraformModule{Path: "h", Dependencies: []*configstack.TerraformModule{g, f, c}}

	modules := configstack.TerraformModules{a, b, c, d, e, f, g, h}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsForTest("/terragrunt.hcl")
	modules.WriteDot(&stdout, terragruntOptions)
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
	assert.True(t, strings.Contains(stdout.String(), expected))
}

func TestCheckForCycles(t *testing.T) {
	t.Parallel()

	////////////////////////////////////
	// These modules have no dependencies
	////////////////////////////////////
	a := &configstack.TerraformModule{Path: "a"}
	b := &configstack.TerraformModule{Path: "b"}
	c := &configstack.TerraformModule{Path: "c"}
	d := &configstack.TerraformModule{Path: "d"}

	////////////////////////////////////
	// These modules have dependencies, but no cycles
	////////////////////////////////////

	// e -> a
	e := &configstack.TerraformModule{Path: "e", Dependencies: []*configstack.TerraformModule{a}}

	// f -> a, b
	f := &configstack.TerraformModule{Path: "f", Dependencies: []*configstack.TerraformModule{a, b}}

	// g -> e -> a
	g := &configstack.TerraformModule{Path: "g", Dependencies: []*configstack.TerraformModule{e}}

	// h -> g -> e -> a
	// |            /
	//  --> f -> b
	// |
	//  --> c
	h := &configstack.TerraformModule{Path: "h", Dependencies: []*configstack.TerraformModule{g, f, c}}

	////////////////////////////////////
	// These modules have dependencies and cycles
	////////////////////////////////////

	// i -> i
	i := &configstack.TerraformModule{Path: "i", Dependencies: []*configstack.TerraformModule{}}
	i.Dependencies = append(i.Dependencies, i)

	// j -> k -> j
	j := &configstack.TerraformModule{Path: "j", Dependencies: []*configstack.TerraformModule{}}
	k := &configstack.TerraformModule{Path: "k", Dependencies: []*configstack.TerraformModule{j}}
	j.Dependencies = append(j.Dependencies, k)

	// l -> m -> n -> o -> l
	l := &configstack.TerraformModule{Path: "l", Dependencies: []*configstack.TerraformModule{}}
	o := &configstack.TerraformModule{Path: "o", Dependencies: []*configstack.TerraformModule{l}}
	n := &configstack.TerraformModule{Path: "n", Dependencies: []*configstack.TerraformModule{o}}
	m := &configstack.TerraformModule{Path: "m", Dependencies: []*configstack.TerraformModule{n}}
	l.Dependencies = append(l.Dependencies, m)

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
		{[]*configstack.TerraformModule{l, o, n, m}, configstack.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
		{[]*configstack.TerraformModule{a, l, b, o, n, f, m, h}, configstack.DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
	}

	for _, testCase := range testCases {
		actual := testCase.modules.CheckForCycles()
		if testCase.expected == nil {
			require.NoError(t, actual)
		} else if assert.Error(t, actual, "For modules %v", testCase.modules) {
			var actualErr configstack.DependencyCycleError
			errors.As(actual, &actualErr)
			assert.Equal(t, []string(testCase.expected), []string(actualErr), "For modules %v", testCase.modules)
		}
	}
}

func TestRunModulesNoModules(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)
}

func TestRunModulesOneModuleSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesOneModuleAssumeAlreadyRan(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:                 "a",
		Dependencies:         configstack.TerraformModules{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		AssumeAlreadyApplied: true,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.False(t, aRan)
}

func TestRunModulesReverseOrderOneModuleSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesIgnoreOrderOneModuleSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesOneModuleError(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesReverseOrderOneModuleError(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesIgnoreOrderOneModuleError(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesSuccessNoParallelism(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, 1)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesReverseOrderMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	require.NoError(t, optsErr)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err := modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module c")
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesWithAssumeAlreadyRanSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:                 "c",
		Dependencies:         configstack.TerraformModules{moduleB},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
		AssumeAlreadyApplied: true,
	}

	dRan := false
	moduleD := &configstack.TerraformModule{
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.False(t, cRan)
	assert.True(t, dRan)
}

func TestRunModulesReverseOrderMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	require.NoError(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrC := configstack.ProcessingModuleDependencyError{moduleC, moduleB, expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.False(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesOneFailureIgnoreDependencyErrors(t *testing.T) {
	t.Parallel()

	aRan := false
	terragruntOptionsA := optionsWithMockTerragruntCommand(t, "a", nil, &aRan)
	terragruntOptionsA.IgnoreDependencyErrors = true
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: terragruntOptionsA,
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	terragruntOptionsB := optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan)
	terragruntOptionsB.IgnoreDependencyErrors = true
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: terragruntOptionsB,
	}

	cRan := false
	terragruntOptionsC := optionsWithMockTerragruntCommand(t, "c", nil, &cRan)
	terragruntOptionsC.IgnoreDependencyErrors = true
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: terragruntOptionsC,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesReverseOrderMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrA := configstack.ProcessingModuleDependencyError{moduleA, moduleB, expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := errors.New("Expected error for module b")
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrB := configstack.ProcessingModuleDependencyError{moduleB, moduleA, expectedErrA}
	expectedErrC := configstack.ProcessingModuleDependencyError{moduleC, moduleB, expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.False(t, bRan)
	assert.False(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := errors.New("Expected error for module a")
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesLargeGraphAllSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	dRan := false
	moduleD := &configstack.TerraformModule{
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &configstack.TerraformModule{
		Path:              "e",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &configstack.TerraformModule{
		Path:              "f",
		Dependencies:      configstack.TerraformModules{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
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

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "large-graph-a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "large-graph-b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module large-graph-c")
	moduleC := &configstack.TerraformModule{
		Path:              "large-graph-c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &configstack.TerraformModule{
		Path:              "large-graph-d",
		Dependencies:      configstack.TerraformModules{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-d", nil, &dRan),
	}

	eRan := false
	moduleE := &configstack.TerraformModule{
		Path:                 "large-graph-e",
		Dependencies:         configstack.TerraformModules{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "large-graph-e", nil, &eRan),
		AssumeAlreadyApplied: true,
	}

	fRan := false
	moduleF := &configstack.TerraformModule{
		Path:              "large-graph-f",
		Dependencies:      configstack.TerraformModules{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-f", nil, &fRan),
	}

	gRan := false
	moduleG := &configstack.TerraformModule{
		Path:              "large-graph-g",
		Dependencies:      configstack.TerraformModules{moduleE},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-g", nil, &gRan),
	}

	expectedErrD := configstack.ProcessingModuleDependencyError{moduleD, moduleC, expectedErrC}
	expectedErrF := configstack.ProcessingModuleDependencyError{moduleF, moduleD, expectedErrD}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF, moduleG}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
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

	aRan := false
	moduleA := &configstack.TerraformModule{
		Path:              "a",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &configstack.TerraformModule{
		Path:              "b",
		Dependencies:      configstack.TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	expectedErrC := errors.New("Expected error for module c")
	moduleC := &configstack.TerraformModule{
		Path:              "c",
		Dependencies:      configstack.TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &configstack.TerraformModule{
		Path:              "d",
		Dependencies:      configstack.TerraformModules{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &configstack.TerraformModule{
		Path:              "e",
		Dependencies:      configstack.TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &configstack.TerraformModule{
		Path:              "f",
		Dependencies:      configstack.TerraformModules{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	expectedErrB := configstack.ProcessingModuleDependencyError{moduleB, moduleC, expectedErrC}
	expectedErrA := configstack.ProcessingModuleDependencyError{moduleA, moduleB, expectedErrB}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	require.NoError(t, optsErr)

	modules := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF}
	err := modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrC, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.False(t, bRan)
	assert.True(t, cRan)
	assert.True(t, dRan)
	assert.True(t, eRan)
	assert.True(t, fRan)
}
