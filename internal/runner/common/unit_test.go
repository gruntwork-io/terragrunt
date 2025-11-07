package common_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/gruntwork-io/terragrunt/internal/component"
	common "github.com/gruntwork-io/terragrunt/internal/runner/common"
)

func TestUnit_String(t *testing.T) {
	t.Parallel()

	c := component.NewUnit("test/path")
	c.AddDependency(component.NewUnit("dep1"))
	c.AddDependency(component.NewUnit("dep2"))

	unit := &common.Unit{
		Unit: c,
	}
	str := unit.String()
	assert.Contains(t, str, "test/path")
	assert.Contains(t, str, "excluded: false")
	assert.Contains(t, str, "dependencies: [dep1, dep2]")
}

func TestUnit_FlushOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	writer := common.NewUnitWriter(&buf)

	opts := &options.TerragruntOptions{Writer: writer}
	c := component.NewUnit("test/path")
	c.SetOpts(opts)
	unit := &common.Unit{Unit: c}
	_, _ = writer.Write([]byte("test output"))

	l := logger.CreateLogger()

	err := unit.FlushOutput(l)
	require.NoError(t, err)
	assert.Equal(t, "test output", buf.String())

	opts.Writer = &bytes.Buffer{}
	c.SetOpts(opts)

	assert.NoError(t, unit.FlushOutput(l))
}

func TestUnit_PlanFile_OutputFile_JSONOutputFolder(t *testing.T) {
	t.Parallel()

	c := component.NewUnit("module/path")
	opts := &options.TerragruntOptions{
		TerraformCommand: "plan",
		JSONOutputFolder: "json-folder",
	}
	c.SetOpts(opts)

	unit := &common.Unit{
		Unit: c,
	}

	opts = &options.TerragruntOptions{OutputFolder: "out-folder", JSONOutputFolder: "json-folder", WorkingDir: "/work"}
	l := logger.CreateLogger()

	planFile := unit.PlanFile(l, opts)
	assert.NotEmpty(t, planFile)
	assert.Contains(t, planFile, "/out-folder/module/path/")
	assert.True(t, hasSuffix(planFile, ".tfplan"), "planFile should end with .tfplan: %s", planFile)

	outputFile := unit.OutputFile(l, opts)
	assert.NotEmpty(t, outputFile)
	assert.Contains(t, outputFile, "/out-folder/module/path/")
	assert.True(t, hasSuffix(outputFile, ".tfplan"), "outputFile should end with .tfplan: %s", outputFile)

	jsonFile := unit.OutputJSONFile(l, opts)
	assert.NotEmpty(t, jsonFile)
	assert.Contains(t, jsonFile, "/json-folder/module/path/")
	assert.True(t, hasSuffix(jsonFile, ".json"), "jsonFile should end with .json: %s", jsonFile)
}

// hasSuffix is a helper to handle both Unix and Windows path separators
func hasSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}

func TestUnit_FindUnitInPath(t *testing.T) {
	t.Parallel()

	unit := &common.Unit{Unit: component.NewUnit("foo/bar")}
	assert.True(t, unit.FindUnitInPath([]string{"foo/bar", "baz"}))
	assert.False(t, unit.FindUnitInPath([]string{"baz"}))
}

func TestUnitsMap_MergeMaps(t *testing.T) {
	t.Parallel()

	m1 := common.UnitsMap{"a": &common.Unit{Unit: component.NewUnit("a")}}
	m2 := common.UnitsMap{"b": &common.Unit{Unit: component.NewUnit("b")}}
	merged := m1.MergeMaps(m2)
	assert.Contains(t, merged, "a")
	assert.Contains(t, merged, "b")
}

func TestUnitsMap_FindByPath(t *testing.T) {
	t.Parallel()

	m := common.UnitsMap{"foo": &common.Unit{Unit: component.NewUnit("foo")}}
	assert.Equal(t, "foo", m.FindByPath("foo").Path())
	assert.Nil(t, m.FindByPath("bar"))
}

func TestUnitsMap_SortedKeys(t *testing.T) {
	t.Parallel()

	m := common.UnitsMap{"b": nil, "a": nil, "c": nil}
	keys := m.SortedKeys()
	assert.Equal(t, []string{"a", "b", "c"}, keys)
}

func TestUnitsMap_ConvertDiscoveryToRunner(t *testing.T) {
	t.Parallel()
	// Use absolute paths for both keys and Path fields
	aPath := "/abs/a"
	bPath := "/abs/b"
	m := common.UnitsMap{
		aPath: &common.Unit{Unit: component.NewUnit(aPath)},
		bPath: &common.Unit{
			Unit: component.NewUnit(bPath).WithConfig(&config.TerragruntConfig{
				Dependencies: &config.ModuleDependencies{Paths: []string{aPath}},
			}),
		},
	}
	units, err := m.ConvertDiscoveryToRunner([]string{aPath, bPath})
	require.NoError(t, err)
	assert.Len(t, units, 2)
	assert.Equal(t, aPath, units[0].Path())
	assert.Equal(t, bPath, units[1].Path())
	assert.Equal(t, aPath, units[1].Dependencies()[0].Path())
}
