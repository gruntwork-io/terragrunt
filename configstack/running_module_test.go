package configstack

import (
	"bytes"
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

func testToRunningModules(t *testing.T, modules []*TerraformModule, order DependencyOrder, expected map[string]*runningModule) {
	actual, err := toRunningModules(modules, order)
	if assert.Nil(t, err, "For modules %v and order %v", modules, order) {
		assertRunningModuleMapsEqual(t, expected, actual, true, "For modules %v and order %v", modules, order)
	}
}

func TestRunModulesNoModules(t *testing.T) {
	t.Parallel()

	err := RunModules([]*TerraformModule{})
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

	err := RunModules([]*TerraformModule{moduleA})
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

	err := RunModules([]*TerraformModule{moduleA})
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

	err := RunModulesReverseOrder([]*TerraformModule{moduleA})
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

	err := RunModules([]*TerraformModule{moduleA})
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

	err := RunModulesReverseOrder([]*TerraformModule{moduleA})
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

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC})
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

	err := RunModulesReverseOrder([]*TerraformModule{moduleA, moduleB, moduleC})
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

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC})
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

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC})
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

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC})
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

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC, moduleD})
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

	err := RunModulesReverseOrder([]*TerraformModule{moduleA, moduleB, moduleC})
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

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC})
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

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC})
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

	err := RunModulesReverseOrder([]*TerraformModule{moduleA, moduleB, moduleC})
	assertMultiErrorContains(t, err, expectedErrB, expectedErrA)

	assert.False(t, aRan)
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

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC})
	assertMultiErrorContains(t, err, expectedErrA, expectedErrB, expectedErrC)

	assert.True(t, aRan)
	assert.False(t, bRan)
	assert.False(t, cRan)
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

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF})
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

	err := RunModules([]*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF, moduleG})
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

	err := RunModulesReverseOrder([]*TerraformModule{moduleA, moduleB, moduleC, moduleD, moduleE, moduleF})
	assertMultiErrorContains(t, err, expectedErrC, expectedErrB, expectedErrA)

	assert.False(t, aRan)
	assert.False(t, bRan)
	assert.True(t, cRan)
	assert.True(t, dRan)
	assert.True(t, eRan)
	assert.True(t, fRan)
}

func TestGenerateDetailedErrorMessage(t *testing.T) {

	const (
		initText      = "Terraform has been successfully initialized!"
		argMissingErr = "Error: Missing required argument"
		exitCodeErr   = "Hit multiple errors:\nexit status 1"
		modulePath    = "a"
	)

	var initBuffer bytes.Buffer
	initBuffer.WriteString(initText)

	module := &TerraformModule{
		Path:              modulePath,
		Dependencies:      []*TerraformModule{},
		Config:            config.TerragruntConfig{},
		TerragruntOptions: &options.TerragruntOptions{InitStream: initBuffer, ExcludeDirs: []string{}},
	}

	runningModule, err := toRunningModules([]*TerraformModule{module}, NormalOrder)
	assert.Nil(t, err)

	var errBuffer bytes.Buffer
	errBuffer.WriteString(fmt.Sprintf("%s\n%s", initText, argMissingErr))

	runningModule[modulePath].Err = fmt.Errorf(exitCodeErr)
	runningModule[modulePath].ErrStream = errBuffer

	detailedErrorMessage := generateDetailedErrorMessage(runningModule[modulePath])
	fmt.Printf("DETAILED ERROR:\n%s\n", detailedErrorMessage)

	// ensure terraform init output is removed from detailed error messages
	assert.NotContains(t, detailedErrorMessage.Error(), initText)
	assert.Contains(t, detailedErrorMessage.Error(), fmt.Sprintf("%s (root error)", modulePath))
	assert.Contains(t, detailedErrorMessage.Error(), exitCodeErr)
	assert.Contains(t, detailedErrorMessage.Error(), argMissingErr)
}
