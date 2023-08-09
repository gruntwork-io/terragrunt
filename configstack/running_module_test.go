package configstack

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

var mockOptions, _ = options.NewTerragruntOptionsForTest("running_module_test")

func TestToRunningModulesNoModules(t *testing.T) {
	t.Parallel()

	testToRunningModules(t, []*TerraformModule{}, NormalOrder, map[string]*runningModule{})
}

func TestToRunningModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	modules := []*TerraformModule{moduleA}
	expected := map[string]*runningModule{"a": runningModuleA}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesTwoModulesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	modules := []*TerraformModule{moduleA, moduleB}
	expected := map[string]*runningModule{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
	}

	runningModuleA.NotifyWhenDone = []*runningModule{runningModuleB}

	modules := []*TerraformModule{moduleA, moduleB}
	expected := map[string]*runningModule{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{runningModuleA},
	}

	runningModuleA.Dependencies = map[string]*runningModule{"b": runningModuleB}

	modules := []*TerraformModule{moduleA, moduleB}
	expected := map[string]*runningModule{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, ReverseOrder, expected)
}

func TestToRunningModulesTwoModulesWithDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	modules := []*TerraformModule{moduleA, moduleB}
	expected := map[string]*runningModule{"a": runningModuleA, "b": runningModuleB}

	testToRunningModules(t, modules, IgnoreOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
	}

	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      []*TerraformModule{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module:         moduleD,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"c": runningModuleC},
		NotifyWhenDone: []*runningModule{},
	}

	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      []*TerraformModule{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module: moduleE,
		Status: Waiting,
		Err:    nil,
		Dependencies: map[string]*runningModule{
			"a": runningModuleA,
			"b": runningModuleB,
			"c": runningModuleC,
			"d": runningModuleD,
		},
		NotifyWhenDone: []*runningModule{},
	}

	runningModuleA.NotifyWhenDone = []*runningModule{runningModuleB, runningModuleC, runningModuleE}
	runningModuleB.NotifyWhenDone = []*runningModule{runningModuleE}
	runningModuleC.NotifyWhenDone = []*runningModule{runningModuleD, runningModuleE}
	runningModuleD.NotifyWhenDone = []*runningModule{runningModuleE}

	modules := []*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, NormalOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependenciesReverseOrder(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{runningModuleA},
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{runningModuleA},
	}

	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      []*TerraformModule{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module:         moduleD,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{runningModuleC},
	}

	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      []*TerraformModule{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module:         moduleE,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{runningModuleA, runningModuleB, runningModuleC, runningModuleD},
	}

	runningModuleA.Dependencies = map[string]*runningModule{"b": runningModuleB, "c": runningModuleC, "e": runningModuleE}
	runningModuleB.Dependencies = map[string]*runningModule{"e": runningModuleE}
	runningModuleC.Dependencies = map[string]*runningModule{"d": runningModuleD, "e": runningModuleE}
	runningModuleD.Dependencies = map[string]*runningModule{"e": runningModuleE}

	modules := []*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, ReverseOrder, expected)
}

func TestToRunningModulesMultipleModulesWithAndWithoutDependenciesIgnoreOrder(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      []*TerraformModule{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module:         moduleD,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      []*TerraformModule{moduleA, moduleB, moduleC, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module:         moduleE,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
	}

	modules := []*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE}
	expected := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	testToRunningModules(t, modules, IgnoreOrder, expected)
}

func testToRunningModules(t *testing.T, modules []*TerraformModule, order DependencyOrder, expected map[string]*runningModule) {
	actual, err := toRunningModules(modules, order)
	if assert.Nil(t, err, "For modules %v and order %v", modules, order) {
		assertRunningModuleMapsEqual(t, expected, actual, true, "For modules %v and order %v", modules, order)
	}
}

func TestRunModulesNoModules(t *testing.T) {
	t.Parallel()

	err := RunModules([]*TerraformModule{}, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)
}

func TestRunModulesOneModuleSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	err := RunModules([]*TerraformModule{moduleA}, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesOneModuleAssumeAlreadyRan(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:                 "a",
		Dependencies:         []*TerraformModule{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
		AssumeAlreadyApplied: true,
	}

	err := RunModules([]*TerraformModule{moduleA}, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.False(t, aRan)
}

func TestRunModulesReverseOrderOneModuleSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	err := RunModulesReverseOrder([]*TerraformModule{moduleA}, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesIgnoreOrderOneModuleSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	err := RunModulesIgnoreOrder([]*TerraformModule{moduleA}, options.DefaultParallelism)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.True(t, aRan)
}

func TestRunModulesOneModuleError(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := fmt.Errorf("Expected error for module a")
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	err := RunModules([]*TerraformModule{moduleA}, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesReverseOrderOneModuleError(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := fmt.Errorf("Expected error for module a")
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	err := RunModulesReverseOrder([]*TerraformModule{moduleA}, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesIgnoreOrderOneModuleError(t *testing.T) {
	t.Parallel()

	aRan := false
	expectedErrA := fmt.Errorf("Expected error for module a")
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	err := RunModulesIgnoreOrder([]*TerraformModule{moduleA}, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrA)
	assert.True(t, aRan)
}

func TestRunModulesMultipleModulesNoDependenciesSuccess(t *testing.T) {
	t.Parallel()

	aRan := false
	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC}, 1)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	err := RunModulesReverseOrder([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	err := RunModulesIgnoreOrder([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	expectedErrC := fmt.Errorf("Expected error for module c")
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:                 "c",
		Dependencies:         []*TerraformModule{moduleB},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
		AssumeAlreadyApplied: true,
	}

	dRan := false
	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      []*TerraformModule{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC, moduleD}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	err := RunModulesReverseOrder([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	err := RunModulesIgnoreOrder([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrC := DependencyFinishedWithError{moduleC, moduleB, expectedErrB}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: terragruntOptionsA,
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	terragruntOptionsB := optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan)
	terragruntOptionsB.IgnoreDependencyErrors = true
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: terragruntOptionsB,
	}

	cRan := false
	terragruntOptionsC := optionsWithMockTerragruntCommand(t, "c", nil, &cRan)
	terragruntOptionsC.IgnoreDependencyErrors = true
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: terragruntOptionsC,
	}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrA := DependencyFinishedWithError{moduleA, moduleB, expectedErrB}

	err := RunModulesReverseOrder([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	expectedErrB := fmt.Errorf("Expected error for module b")
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", expectedErrB, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	err := RunModulesIgnoreOrder([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	expectedErrB := DependencyFinishedWithError{moduleB, moduleA, expectedErrA}
	expectedErrC := DependencyFinishedWithError{moduleC, moduleB, expectedErrB}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", expectedErrA, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	err := RunModulesIgnoreOrder([]*TerraformModule{moduleA, moduleB, moduleC}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", nil, &cRan),
	}

	dRan := false
	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      []*TerraformModule{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &TerraformModule{
		Path:              "f",
		Dependencies:      []*TerraformModule{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF}, options.DefaultParallelism)
	assert.Nil(t, err)

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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "large-graph-b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-b", nil, &bRan),
	}

	cRan := false
	expectedErrC := fmt.Errorf("Expected error for module large-graph-c")
	moduleC := &TerraformModule{
		Path:              "large-graph-c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &TerraformModule{
		Path:              "large-graph-d",
		Dependencies:      []*TerraformModule{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-d", nil, &dRan),
	}

	eRan := false
	moduleE := &TerraformModule{
		Path:                 "large-graph-e",
		Dependencies:         []*TerraformModule{},
		Config:               config.TerragruntConfig{},
		TerragruntOptions:    optionsWithMockTerragruntCommand(t, "large-graph-e", nil, &eRan),
		AssumeAlreadyApplied: true,
	}

	fRan := false
	moduleF := &TerraformModule{
		Path:              "large-graph-f",
		Dependencies:      []*TerraformModule{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-f", nil, &fRan),
	}

	gRan := false
	moduleG := &TerraformModule{
		Path:              "large-graph-g",
		Dependencies:      []*TerraformModule{moduleE},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "large-graph-g", nil, &gRan),
	}

	expectedErrD := DependencyFinishedWithError{moduleD, moduleC, expectedErrC}
	expectedErrF := DependencyFinishedWithError{moduleF, moduleD, expectedErrD}

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF, moduleG}, options.DefaultParallelism)
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
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "a", nil, &aRan),
	}

	bRan := false
	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "b", nil, &bRan),
	}

	cRan := false
	expectedErrC := fmt.Errorf("Expected error for module c")
	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleB},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "c", expectedErrC, &cRan),
	}

	dRan := false
	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      []*TerraformModule{moduleA, moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "d", nil, &dRan),
	}

	eRan := false
	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "e", nil, &eRan),
	}

	fRan := false
	moduleF := &TerraformModule{
		Path:              "f",
		Dependencies:      []*TerraformModule{moduleE, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: optionsWithMockTerragruntCommand(t, "f", nil, &fRan),
	}

	expectedErrB := DependencyFinishedWithError{moduleB, moduleC, expectedErrC}
	expectedErrA := DependencyFinishedWithError{moduleA, moduleB, expectedErrB}

	err := RunModulesReverseOrder([]*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF}, options.DefaultParallelism)
	assertMultiErrorContains(t, err, expectedErrC, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.False(t, bRan)
	assert.True(t, cRan)
	assert.True(t, dRan)
	assert.True(t, eRan)
	assert.True(t, fRan)
}

func TestRemoveFlagExcludedNoExclude(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      []*TerraformModule{moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module:         moduleD,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"c": runningModuleC},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      []*TerraformModule{moduleB, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module: moduleE,
		Status: Waiting,
		Err:    nil,
		Dependencies: map[string]*runningModule{
			"b": runningModuleB,
			"d": runningModuleD,
		},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	running_modules := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	expected := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}

	actual := removeFlagExcluded(running_modules)
	assertRunningModuleMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   true,
	}

	running_modules := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
	}

	expected := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
	}

	actual := removeFlagExcluded(running_modules)
	assertRunningModuleMapsEqual(t, expected, actual, true)
}

func TestRemoveFlagExcludedOneExcludeWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:              "a",
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleA := &runningModule{
		Module:         moduleA,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleB := &TerraformModule{
		Path:              "b",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleB := &runningModule{
		Module:         moduleB,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleC := &TerraformModule{
		Path:              "c",
		Dependencies:      []*TerraformModule{moduleA},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleC := &runningModule{
		Module:         moduleC,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"a": runningModuleA},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   true,
	}

	moduleD := &TerraformModule{
		Path:              "d",
		Dependencies:      []*TerraformModule{moduleB, moduleC},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleD := &runningModule{
		Module: moduleD,
		Status: Waiting,
		Err:    nil,
		Dependencies: map[string]*runningModule{
			"b": runningModuleB,
			"c": runningModuleC,
		},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	moduleE := &TerraformModule{
		Path:              "e",
		Dependencies:      []*TerraformModule{moduleB, moduleD},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: mockOptions,
	}

	runningModuleE := &runningModule{
		Module: moduleE,
		Status: Waiting,
		Err:    nil,
		Dependencies: map[string]*runningModule{
			"b": runningModuleB,
			"d": runningModuleD,
		},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	running_modules := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
		"c": runningModuleC,
		"d": runningModuleD,
		"e": runningModuleE,
	}
	actual := removeFlagExcluded(running_modules)

	_runningModuleD := &runningModule{
		Module:         moduleD,
		Status:         Waiting,
		Err:            nil,
		Dependencies:   map[string]*runningModule{"b": runningModuleB},
		NotifyWhenDone: []*runningModule{},
		FlagExcluded:   false,
	}

	expected := map[string]*runningModule{
		"a": runningModuleA,
		"b": runningModuleB,
		"d": _runningModuleD,
		"e": runningModuleE,
	}

	assertRunningModuleMapsEqual(t, expected, actual, true)
}
