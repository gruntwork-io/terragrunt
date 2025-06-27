package common_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	runbase "github.com/gruntwork-io/terragrunt/internal/runner/runbase"
)

func TestUnit_String(t *testing.T) {
	t.Parallel()
	unit := &runbase.Unit{
		Path:                 "test/path",
		FlagExcluded:         true,
		AssumeAlreadyApplied: false,
		Dependencies: runbase.Units{
			&runbase.Unit{Path: "dep1"},
			&runbase.Unit{Path: "dep2"},
		},
	}
	str := unit.String()
	assert.Contains(t, str, "test/path")
	assert.Contains(t, str, "excluded: true")
	assert.Contains(t, str, "dependencies: [dep1, dep2]")
}

func TestUnit_FlushOutput(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	writer := runbase.NewUnitWriter(&buf)
	unit := &runbase.Unit{
		TerragruntOptions: &options.TerragruntOptions{Writer: writer},
	}
	_, _ = writer.Write([]byte("test output"))
	err := unit.FlushOutput()
	require.NoError(t, err)
	assert.Equal(t, "test output", buf.String())

	unit.TerragruntOptions.Writer = &bytes.Buffer{}
	assert.NoError(t, unit.FlushOutput())
}

func TestUnit_PlanFile_OutputFile_JSONOutputFolder(t *testing.T) {
	t.Parallel()
	unit := &runbase.Unit{
		Path: "module/path",
		TerragruntOptions: &options.TerragruntOptions{
			TerraformCommand: "plan",
			JSONOutputFolder: "json-folder",
		},
	}
	opts := &options.TerragruntOptions{OutputFolder: "out-folder", JSONOutputFolder: "json-folder", WorkingDir: "/work"}
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
	unit := &runbase.Unit{Path: "foo/bar"}
	assert.True(t, unit.FindUnitInPath([]string{"foo/bar", "baz"}))
	assert.False(t, unit.FindUnitInPath([]string{"baz"}))
}

func TestUnitsMap_MergeMaps(t *testing.T) {
	t.Parallel()
	m1 := runbase.UnitsMap{"a": &runbase.Unit{Path: "a"}}
	m2 := runbase.UnitsMap{"b": &runbase.Unit{Path: "b"}}
	merged := m1.MergeMaps(m2)
	assert.Contains(t, merged, "a")
	assert.Contains(t, merged, "b")
}

func TestUnitsMap_FindByPath(t *testing.T) {
	t.Parallel()
	m := runbase.UnitsMap{"foo": &runbase.Unit{Path: "foo"}}
	assert.Equal(t, "foo", m.FindByPath("foo").Path)
	assert.Nil(t, m.FindByPath("bar"))
}

func TestUnitsMap_SortedKeys(t *testing.T) {
	t.Parallel()
	m := runbase.UnitsMap{"b": nil, "a": nil, "c": nil}
	keys := m.SortedKeys()
	assert.Equal(t, []string{"a", "b", "c"}, keys)
}

func TestUnitsMap_CrossLinkDependencies(t *testing.T) {
	t.Parallel()
	// Use absolute paths for both keys and Path fields
	aPath := "/abs/a"
	bPath := "/abs/b"
	m := runbase.UnitsMap{
		aPath: &runbase.Unit{Path: aPath, Config: config.TerragruntConfig{}},
		bPath: &runbase.Unit{Path: bPath, Config: config.TerragruntConfig{Dependencies: &config.ModuleDependencies{Paths: []string{aPath}}}},
	}
	units, err := m.CrossLinkDependencies([]string{aPath, bPath})
	require.NoError(t, err)
	assert.Len(t, units, 2)
	assert.Equal(t, aPath, units[0].Path)
	assert.Equal(t, bPath, units[1].Path)
	assert.Equal(t, aPath, units[1].Dependencies[0].Path)
}

func TestUnits_WriteDot(t *testing.T) {
	t.Parallel()
	units := runbase.Units{
		&runbase.Unit{Path: "a"},
		&runbase.Unit{Path: "b", Dependencies: runbase.Units{&runbase.Unit{Path: "a"}}, FlagExcluded: true},
	}
	var buf bytes.Buffer
	opts := &options.TerragruntOptions{TerragruntConfigPath: "/foo/terragrunt.hcl"}
	l := logger.CreateLogger()
	err := units.WriteDot(l, &buf, opts)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "digraph {")
	assert.Contains(t, out, "a")
	assert.Contains(t, out, "b")
	assert.Contains(t, out, "[color=red]")
}

func TestUnits_CheckForCycles(t *testing.T) {
	t.Parallel()
	unitA := &runbase.Unit{Path: "a"}
	unitB := &runbase.Unit{Path: "b", Dependencies: runbase.Units{unitA}}
	unitA.Dependencies = runbase.Units{unitB} // cycle
	units := runbase.Units{unitA, unitB}
	err := units.CheckForCycles()
	assert.Error(t, err)
}
