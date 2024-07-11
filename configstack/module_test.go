package configstack

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestGraph(t *testing.T) {
	a := &TerraformModule{Path: "a"}
	b := &TerraformModule{Path: "b"}
	c := &TerraformModule{Path: "c"}
	d := &TerraformModule{Path: "d"}
	e := &TerraformModule{Path: "e", Dependencies: []*TerraformModule{a}}
	f := &TerraformModule{Path: "f", Dependencies: []*TerraformModule{a, b}}
	g := &TerraformModule{Path: "g", Dependencies: []*TerraformModule{e}}
	h := &TerraformModule{Path: "h", Dependencies: []*TerraformModule{g, f, c}}

	modules := TerraformModules{a, b, c, d, e, f, g, h}

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
	a := &TerraformModule{Path: "/config/a"}
	b := &TerraformModule{Path: "/config/b"}
	c := &TerraformModule{Path: "/config/c"}
	d := &TerraformModule{Path: "/config/d"}
	e := &TerraformModule{Path: "/config/alpha/beta/gamma/e", Dependencies: []*TerraformModule{a}}
	f := &TerraformModule{Path: "/config/alpha/beta/gamma/f", Dependencies: []*TerraformModule{a, b}}
	g := &TerraformModule{Path: "/config/alpha/g", Dependencies: []*TerraformModule{e}}
	h := &TerraformModule{Path: "/config/alpha/beta/h", Dependencies: []*TerraformModule{g, f, c}}

	modules := TerraformModules{a, b, c, d, e, f, g, h}

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
	a := &TerraformModule{Path: "a", FlagExcluded: true}
	b := &TerraformModule{Path: "b"}
	c := &TerraformModule{Path: "c"}
	d := &TerraformModule{Path: "d"}
	e := &TerraformModule{Path: "e", Dependencies: []*TerraformModule{a}}
	f := &TerraformModule{Path: "f", FlagExcluded: true, Dependencies: []*TerraformModule{a, b}}
	g := &TerraformModule{Path: "g", Dependencies: []*TerraformModule{e}}
	h := &TerraformModule{Path: "h", Dependencies: []*TerraformModule{g, f, c}}

	modules := TerraformModules{a, b, c, d, e, f, g, h}

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
	a := &TerraformModule{Path: "a"}
	b := &TerraformModule{Path: "b"}
	c := &TerraformModule{Path: "c"}
	d := &TerraformModule{Path: "d"}

	////////////////////////////////////
	// These modules have dependencies, but no cycles
	////////////////////////////////////

	// e -> a
	e := &TerraformModule{Path: "e", Dependencies: []*TerraformModule{a}}

	// f -> a, b
	f := &TerraformModule{Path: "f", Dependencies: []*TerraformModule{a, b}}

	// g -> e -> a
	g := &TerraformModule{Path: "g", Dependencies: []*TerraformModule{e}}

	// h -> g -> e -> a
	// |            /
	//  --> f -> b
	// |
	//  --> c
	h := &TerraformModule{Path: "h", Dependencies: []*TerraformModule{g, f, c}}

	////////////////////////////////////
	// These modules have dependencies and cycles
	////////////////////////////////////

	// i -> i
	i := &TerraformModule{Path: "i", Dependencies: []*TerraformModule{}}
	i.Dependencies = append(i.Dependencies, i)

	// j -> k -> j
	j := &TerraformModule{Path: "j", Dependencies: []*TerraformModule{}}
	k := &TerraformModule{Path: "k", Dependencies: []*TerraformModule{j}}
	j.Dependencies = append(j.Dependencies, k)

	// l -> m -> n -> o -> l
	l := &TerraformModule{Path: "l", Dependencies: []*TerraformModule{}}
	o := &TerraformModule{Path: "o", Dependencies: []*TerraformModule{l}}
	n := &TerraformModule{Path: "n", Dependencies: []*TerraformModule{o}}
	m := &TerraformModule{Path: "m", Dependencies: []*TerraformModule{n}}
	l.Dependencies = append(l.Dependencies, m)

	testCases := []struct {
		modules  TerraformModules
		expected DependencyCycleError
	}{
		{[]*TerraformModule{}, nil},
		{[]*TerraformModule{a}, nil},
		{[]*TerraformModule{a, b, c, d}, nil},
		{[]*TerraformModule{a, e}, nil},
		{[]*TerraformModule{a, b, f}, nil},
		{[]*TerraformModule{a, e, g}, nil},
		{[]*TerraformModule{a, b, c, e, f, g, h}, nil},
		{[]*TerraformModule{i}, DependencyCycleError([]string{"i", "i"})},
		{[]*TerraformModule{j, k}, DependencyCycleError([]string{"j", "k", "j"})},
		{[]*TerraformModule{l, o, n, m}, DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
		{[]*TerraformModule{a, l, b, o, n, f, m, h}, DependencyCycleError([]string{"l", "m", "n", "o", "l"})},
	}

	for _, testCase := range testCases {
		actual := testCase.modules.CheckForCycles()
		if testCase.expected == nil {
			assert.Nil(t, actual)
		} else if assert.NotNil(t, actual, "For modules %v", testCase.modules) {
			actualErr := errors.Unwrap(actual).(DependencyCycleError)
			assert.Equal(t, []string(testCase.expected), []string(actualErr), "For modules %v", testCase.modules)
		}
	}
}

func TestRunModulesNoModules(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)
}

func TestRunModulesOneModuleSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesOneModuleAssumeAlreadyRan(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:                 "a",
		Dependencies:         TerraformModules{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		AssumeAlreadyApplied: true,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.False(t, aRan)
}

func TestRunModulesReverseOrderOneModuleSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA}
	err = modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesIgnoreOrderOneModuleSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesOneModuleError(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := fmt.Errorf("Expected error for module a")
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesReverseOrderOneModuleError(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := fmt.Errorf("Expected error for module a")
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA}
	err = modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesIgnoreOrderOneModuleError(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := fmt.Errorf("Expected error for module a")
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesSuccessNoParallelism(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, 1)
	assert.Nil(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesReverseOrderMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, optsErr)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err := modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesNoDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := fmt.Errorf("Expected error for module a")
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	expectedErrC := fmt.Errorf("Expected error for module c")
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesWithAssumeAlreadyRanSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:                 "c",
		Dependencies:         TerraformModules{moduleB},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
		AssumeAlreadyApplied: true,
	}

	dRan := false
	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      TerraformModules{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC, moduleD}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.False(t, cRan)
	assert.True(t, dRan)
}

func TestRunModulesReverseOrderMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrC := ProcessingModuleDependencyError{moduleC, moduleB, expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
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
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: terragruntOptionsA,
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	terragruntOptionsB := optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan)
	terragruntOptionsB.IgnoreDependencyErrors = true
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: terragruntOptionsB,
	}

	cRan := false
	terragruntOptionsC := optionsWithMockTerragruntCommand(t, "c", nil, &cRan)
	terragruntOptionsC.IgnoreDependencyErrors = true
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: terragruntOptionsC,
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesReverseOrderMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrA := ProcessingModuleDependencyError{moduleA, moduleB, expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesOneFailure(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrB)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := fmt.Errorf("Expected error for module a")
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrB := ProcessingModuleDependencyError{moduleB, moduleA, expectedErrA}
	expectedErrC := ProcessingModuleDependencyError{moduleC, moduleB, expectedErrB}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.False(t, bRan)
	assert.False(t, cRan)
}

func TestRunModulesIgnoreOrderMultipleModulesWithDependenciesMultipleFailures(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := fmt.Errorf("Expected error for module a")
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC}
	err = modules.RunModulesIgnoreOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)

	assert.True(t, aRan)
	assert.True(t, bRan)
	assert.True(t, cRan)
}

func TestRunModulesMultipleModulesWithDependenciesLargeGraphAllSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	dRan := false
	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      TerraformModules{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &TerraformModule{
		Path:              "f",
		Dependencies:      TerraformModules{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF}
	err = modules.RunModules(context.Background(), opts, options.DefaultParallelism)
	assert.NoError(t, err)

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
	moduleA := &TerraformModule{
		Path:              "large-graph-a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "large-graph-b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-b", nil, &bRan),
	}

	cRan := false
	expectedErrC := fmt.Errorf("Expected error for module large-graph-c")
	moduleC := &TerraformModule{
		Path:              "large-graph-c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &TerraformModule{
		Path:              "large-graph-d",
		Dependencies:      TerraformModules{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-d", nil, &dRan),
	}

	eRan := false
	moduleE := &TerraformModule{
		Path:                 "large-graph-e",
		Dependencies:         TerraformModules{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "large-graph-e", nil, &eRan),
		AssumeAlreadyApplied: true,
	}

	fRan := false
	moduleF := &TerraformModule{
		Path:              "large-graph-f",
		Dependencies:      TerraformModules{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-f", nil, &fRan),
	}

	gRan := false
	moduleG := &TerraformModule{
		Path:              "large-graph-g",
		Dependencies:      TerraformModules{moduleE},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-g", nil, &gRan),
	}

	expectedErrD := ProcessingModuleDependencyError{moduleD, moduleC, expectedErrC}
	expectedErrF := ProcessingModuleDependencyError{moduleF, moduleD, expectedErrD}

	opts, err := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, err)

	modules := TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF, moduleG}
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
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      TerraformModules{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	expectedErrC := fmt.Errorf("Expected error for module c")
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      TerraformModules{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      TerraformModules{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      TerraformModules{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &TerraformModule{
		Path:              "f",
		Dependencies:      TerraformModules{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	expectedErrB := ProcessingModuleDependencyError{moduleB, moduleC, expectedErrC}
	expectedErrA := ProcessingModuleDependencyError{moduleA, moduleB, expectedErrB}

	opts, optsErr := options.NewTerragruntOptionsForTest("")
	assert.NoError(t, optsErr)

	modules := TerraformModules{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF}
	err := modules.RunModulesReverseOrder(context.Background(), opts, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrC, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.False(t, bRan)
	assert.True(t, cRan)
	assert.True(t, dRan)
	assert.True(t, eRan)
	assert.True(t, fRan)
}
