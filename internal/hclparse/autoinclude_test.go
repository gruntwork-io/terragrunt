package hclparse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestAutoIncludeHCL_Resolve_Nil(t *testing.T) {
	t.Parallel()

	var a *hclparse.AutoIncludeHCL

	result, diags := a.Resolve(nil)
	assert.Nil(t, result)
	assert.False(t, diags.HasErrors())
}

func TestAutoIncludeHCL_Resolve_DependencyConfigPath(t *testing.T) {
	t.Parallel()

	src := `
dependency "vpc" {
  config_path = unit.vpc.path
}
`
	body := parseHCLBody(t, src)

	autoInclude := &hclparse.AutoIncludeHCL{
		Remain: body,
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"vpc": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("../vpc"),
					"name": cty.StringVal("vpc"),
				}),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	result, diags := autoInclude.Resolve(evalCtx)
	require.False(t, diags.HasErrors(), "resolve error: %s", diags.Error())
	require.NotNil(t, result)
	require.Len(t, result.Dependencies, 1)
	assert.Equal(t, "vpc", result.Dependencies[0].Name)
	assert.Equal(t, "../vpc", result.Dependencies[0].ConfigPath)
	assert.NotNil(t, result.Dependencies[0].Block)
	assert.NotNil(t, result.RawBody)
}

func TestAutoIncludeHCL_Resolve_MultipleDependencies(t *testing.T) {
	t.Parallel()

	src := `
dependency "vpc" {
  config_path = unit.vpc.path
}

dependency "db" {
  config_path = unit.database.path
}
`
	body := parseHCLBody(t, src)

	autoInclude := &hclparse.AutoIncludeHCL{
		Remain: body,
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"vpc": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("../vpc"),
					"name": cty.StringVal("vpc"),
				}),
				"database": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("../database"),
					"name": cty.StringVal("database"),
				}),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	result, diags := autoInclude.Resolve(evalCtx)
	require.False(t, diags.HasErrors(), "resolve error: %s", diags.Error())
	require.NotNil(t, result)
	require.Len(t, result.Dependencies, 2)
	assert.Equal(t, "vpc", result.Dependencies[0].Name)
	assert.Equal(t, "../vpc", result.Dependencies[0].ConfigPath)
	assert.Equal(t, "db", result.Dependencies[1].Name)
	assert.Equal(t, "../database", result.Dependencies[1].ConfigPath)
}

func TestAutoIncludeHCL_Resolve_StackRef(t *testing.T) {
	t.Parallel()

	src := `
dependency "networking" {
  config_path = stack.networking.path
}
`
	body := parseHCLBody(t, src)

	autoInclude := &hclparse.AutoIncludeHCL{
		Remain: body,
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.EmptyObjectVal,
			"stack": cty.ObjectVal(map[string]cty.Value{
				"networking": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("../networking"),
					"name": cty.StringVal("networking"),
				}),
			}),
		},
	}

	result, diags := autoInclude.Resolve(evalCtx)
	require.False(t, diags.HasErrors(), "resolve error: %s", diags.Error())
	require.NotNil(t, result)
	require.Len(t, result.Dependencies, 1)
	assert.Equal(t, "networking", result.Dependencies[0].Name)
	assert.Equal(t, "../networking", result.Dependencies[0].ConfigPath)
}

func TestAutoIncludeHCL_Resolve_DependencyWithMockOutputs(t *testing.T) {
	t.Parallel()

	// dependency block with config_path + mock_outputs + inputs
	// Only config_path should be evaluated; inputs are left in RawBody
	src := `
dependency "vpc" {
  config_path = unit.vpc.path

  mock_outputs_allowed_terraform_commands = ["plan"]
  mock_outputs = {
    val = "fake-val"
  }
}

inputs = {
  val = dependency.vpc.outputs.val
}
`
	body := parseHCLBody(t, src)

	autoInclude := &hclparse.AutoIncludeHCL{
		Remain: body,
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"vpc": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("/abs/path/to/.terragrunt-stack/vpc"),
					"name": cty.StringVal("vpc"),
				}),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	result, diags := autoInclude.Resolve(evalCtx)
	require.False(t, diags.HasErrors(), "resolve error: %s", diags.Error())
	require.NotNil(t, result)

	// Dependency config_path resolved
	require.Len(t, result.Dependencies, 1)
	assert.Equal(t, "vpc", result.Dependencies[0].Name)
	assert.Equal(t, "/abs/path/to/.terragrunt-stack/vpc", result.Dependencies[0].ConfigPath)

	// RawBody preserved (contains inputs with dependency.vpc.outputs.val)
	assert.NotNil(t, result.RawBody)
}

func TestAutoIncludeDependencyPaths_NoFile(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.NoError(t, err)
	assert.Nil(t, paths)
}

func TestAutoIncludeDependencyPaths_WithDependency(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	require.NoError(t, vfs.WriteFile(fs, filepath.Join("/test", hclparse.AutoIncludeFile), []byte(`
dependency "vpc" {
  config_path = "../unit-w-outputs"
}
`), 0644))

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Equal(t, filepath.Clean(filepath.Join("/test", "..", "unit-w-outputs")), paths[0])
}

func TestAutoIncludeDependencyPaths_MultipleDeps(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	require.NoError(t, vfs.WriteFile(fs, filepath.Join("/test", hclparse.AutoIncludeFile), []byte(`
dependency "vpc" {
  config_path = "../vpc"
}
dependency "db" {
  config_path = "../database"
}
`), 0644))

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.NoError(t, err)
	require.Len(t, paths, 2)
}

func TestAutoIncludeDependencyPaths_Symlink(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	resolvedTmpDir, _ := filepath.EvalSymlinks(tmpDir)

	// Create real unit directory with autoinclude file
	realDir := filepath.Join(resolvedTmpDir, "real-unit")
	require.NoError(t, os.MkdirAll(realDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(realDir, hclparse.AutoIncludeFile), []byte(`
dependency "vpc" {
  config_path = "../vpc"
}
`), 0644))

	// Create symlink to unit directory
	symlinkDir := filepath.Join(tmpDir, "symlinked-unit")
	require.NoError(t, os.Symlink(realDir, symlinkDir))

	// AutoIncludeDependencyPaths via symlink should resolve correctly
	paths, err := hclparse.AutoIncludeDependencyPaths(vfs.NewOSFS(), symlinkDir)
	require.NoError(t, err)
	require.Len(t, paths, 1)

	// The dependency path should be resolved relative to the REAL directory, not the symlink
	assert.NotContains(t, paths[0], "symlinked-unit")
	// ../vpc resolved from real-unit gives <tmpDir>/vpc
	expected := filepath.Clean(filepath.Join(realDir, "..", "vpc"))
	assert.Equal(t, expected, paths[0])
}

func TestAutoIncludeDependencyPaths_AbsolutePath(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	require.NoError(t, vfs.WriteFile(fs, filepath.Join("/test", hclparse.AutoIncludeFile), []byte(`
dependency "vpc" {
  config_path = "/absolute/path/to/vpc"
}
`), 0644))

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Equal(t, "/absolute/path/to/vpc", paths[0])
}

// parseHCLBody is a test helper that parses an HCL string and returns the body.
func parseHCLBody(t *testing.T, src string) hcl.Body {
	t.Helper()

	file, diags := hclsyntax.ParseConfig([]byte(src), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

	return file.Body
}
