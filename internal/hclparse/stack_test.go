package hclparse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// staticStringExpr builds a literal-string hcl.Expression suitable for the lazy Path/Source fields on UnitBlockHCL / StackBlockHCL.
func staticStringExpr(s string) hcl.Expression {
	return hcl.StaticExpr(cty.StringVal(s), hcl.Range{Filename: "test"})
}

func TestBuildComponentRefMap_Empty(t *testing.T) {
	t.Parallel()

	result := hclparse.BuildComponentRefMap(nil)
	assert.True(t, result.Type().IsObjectType())
}

func TestBuildComponentRefMap_WithRefs(t *testing.T) {
	t.Parallel()

	refs := []hclparse.ComponentRef{
		{Name: "vpc", Path: "vpc"},
		{Name: "app", Path: "app-service"},
	}

	result := hclparse.BuildComponentRefMap(refs)

	require.True(t, result.Type().IsObjectType())

	vpcVal := result.GetAttr("vpc")
	require.True(t, vpcVal.Type().IsObjectType())
	assert.Equal(t, "vpc", vpcVal.GetAttr("path").AsString())
	assert.Equal(t, "vpc", vpcVal.GetAttr("name").AsString())

	appVal := result.GetAttr("app")
	require.True(t, appVal.Type().IsObjectType())
	assert.Equal(t, "app-service", appVal.GetAttr("path").AsString())
	assert.Equal(t, "app", appVal.GetAttr("name").AsString())
}

func TestBuildComponentRefMap_WithChildRefs(t *testing.T) {
	t.Parallel()

	refs := []hclparse.ComponentRef{
		{
			Name: "networking",
			Path: "/project/.terragrunt-stack/networking",
			ChildRefs: []hclparse.ComponentRef{
				{Name: "vpc", Path: "/project/.terragrunt-stack/networking/.terragrunt-stack/vpc"},
				{Name: "subnets", Path: "/project/.terragrunt-stack/networking/.terragrunt-stack/subnets"},
			},
		},
	}

	result := hclparse.BuildComponentRefMap(refs)

	netVal := result.GetAttr("networking")
	require.True(t, netVal.Type().IsObjectType())
	assert.Equal(t, "/project/.terragrunt-stack/networking", netVal.GetAttr("path").AsString())

	// Child unit refs are accessible as nested attributes
	vpcVal := netVal.GetAttr("vpc")
	require.True(t, vpcVal.Type().IsObjectType())
	assert.Equal(t, "/project/.terragrunt-stack/networking/.terragrunt-stack/vpc", vpcVal.GetAttr("path").AsString())
	assert.Equal(t, "vpc", vpcVal.GetAttr("name").AsString())

	subnetsVal := netVal.GetAttr("subnets")
	assert.Equal(t, "/project/.terragrunt-stack/networking/.terragrunt-stack/subnets", subnetsVal.GetAttr("path").AsString())
}

func TestBuildComponentRefMap_MultiLevelChildRefs(t *testing.T) {
	t.Parallel()

	// 3 levels: infra -> deep -> db (stack.infra.deep.db.path)
	refs := []hclparse.ComponentRef{
		{
			Name: "infra",
			Path: "/gen/infra",
			ChildRefs: []hclparse.ComponentRef{
				{Name: "vpc", Path: "/gen/infra/.terragrunt-stack/vpc"},
				{
					Name: "deep",
					Path: "/gen/infra/.terragrunt-stack/deep",
					ChildRefs: []hclparse.ComponentRef{
						{Name: "db", Path: "/gen/infra/.terragrunt-stack/deep/.terragrunt-stack/db"},
					},
				},
			},
		},
	}

	result := hclparse.BuildComponentRefMap(refs)

	// Level 1: infra
	infraVal := result.GetAttr("infra")
	assert.Equal(t, "/gen/infra", infraVal.GetAttr("path").AsString())

	// Level 2: infra.deep
	deepVal := infraVal.GetAttr("deep")
	assert.Equal(t, "/gen/infra/.terragrunt-stack/deep", deepVal.GetAttr("path").AsString())

	// Level 3: infra.deep.db
	dbVal := deepVal.GetAttr("db")
	assert.Equal(t, "/gen/infra/.terragrunt-stack/deep/.terragrunt-stack/db", dbVal.GetAttr("path").AsString())
	assert.Equal(t, "db", dbVal.GetAttr("name").AsString())
}

func TestExtractUnitRefs(t *testing.T) {
	t.Parallel()

	units := []*hclparse.UnitBlockHCL{
		{Name: "vpc", Path: staticStringExpr("vpc"), Source: staticStringExpr("../modules/vpc")},
		{Name: "app", Path: staticStringExpr("app-service"), Source: staticStringExpr("../modules/app")},
	}

	refs := hclparse.ExtractUnitRefs(units, nil)

	require.Len(t, refs, 2)
	assert.Equal(t, "vpc", refs[0].Name)
	assert.Equal(t, "vpc", refs[0].Path)
	assert.Equal(t, "app", refs[1].Name)
	assert.Equal(t, "app-service", refs[1].Path)
}

func TestExtractStackRefs(t *testing.T) {
	t.Parallel()

	stacks := []*hclparse.StackBlockHCL{
		{Name: "networking", Path: staticStringExpr("networking"), Source: staticStringExpr("../stacks/networking")},
	}

	refs := hclparse.ExtractStackRefs(stacks, nil)

	require.Len(t, refs, 1)
	assert.Equal(t, "networking", refs[0].Name)
	assert.Equal(t, "networking", refs[0].Path)
}

func TestBuildAutoIncludeEvalContext(t *testing.T) {
	t.Parallel()

	unitRefs := []hclparse.ComponentRef{
		{Name: "vpc", Path: "vpc"},
		{Name: "app", Path: "app"},
	}
	stackRefs := []hclparse.ComponentRef{
		{Name: "infra", Path: "infra-stack"},
	}

	evalCtx := hclparse.BuildAutoIncludeEvalContext(unitRefs, stackRefs)

	require.NotNil(t, evalCtx)
	require.Contains(t, evalCtx.Variables, "unit")
	require.Contains(t, evalCtx.Variables, "stack")

	unitVar := evalCtx.Variables["unit"]
	assert.Equal(t, cty.String, unitVar.GetAttr("vpc").GetAttr("path").Type())
	assert.Equal(t, "vpc", unitVar.GetAttr("vpc").GetAttr("path").AsString())
	assert.Equal(t, "app", unitVar.GetAttr("app").GetAttr("path").AsString())

	stackVar := evalCtx.Variables["stack"]
	assert.Equal(t, "infra-stack", stackVar.GetAttr("infra").GetAttr("path").AsString())
}

func TestDiscoverStackChildUnits(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test/stack-src", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/stack-src/terragrunt.stack.hcl", []byte(`
unit "vpc" {
  source = "../../units/vpc"
  path   = "vpc"
}

unit "db" {
  source = "../../units/db"
  path   = "db"
}
`), 0644))

	refs := hclparse.DiscoverStackChildUnits(fs, "/test/stack-src", "/gen/networking")
	require.Len(t, refs, 2)
	assert.Equal(t, "vpc", refs[0].Name)
	assert.Equal(t, filepath.Join("/gen/networking", ".terragrunt-stack", "vpc"), refs[0].Path)
	assert.Equal(t, "db", refs[1].Name)
	assert.Equal(t, filepath.Join("/gen/networking", ".terragrunt-stack", "db"), refs[1].Path)
}

func TestDiscoverStackChildUnits_NoDotTerragruntStack(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test/stack-src", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/stack-src/terragrunt.stack.hcl", []byte(`
unit "vpc" {
  source = "../../units/vpc"
  path   = "vpc"
  no_dot_terragrunt_stack = true
}

unit "db" {
  source = "../../units/db"
  path   = "db"
}
`), 0644))

	refs := hclparse.DiscoverStackChildUnits(fs, "/test/stack-src", "/gen/networking")
	require.Len(t, refs, 2)
	assert.Equal(t, "vpc", refs[0].Name)
	assert.Equal(t, filepath.Join("/gen/networking", "vpc"), refs[0].Path)
	assert.Equal(t, "db", refs[1].Name)
	assert.Equal(t, filepath.Join("/gen/networking", ".terragrunt-stack", "db"), refs[1].Path)
}

func TestDiscoverStackChildUnits_NoStackFile(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	refs := hclparse.DiscoverStackChildUnits(fs, "/nonexistent", "/gen")
	assert.Nil(t, refs)
}

func TestBuildAutoIncludeEvalContext_WithChildRefs(t *testing.T) {
	t.Parallel()

	stackRefs := []hclparse.ComponentRef{
		{
			Name: "stack_w_outputs",
			Path: "/project/.terragrunt-stack/stack-w-outputs",
			ChildRefs: []hclparse.ComponentRef{
				{Name: "unit_w_outputs", Path: "/project/.terragrunt-stack/stack-w-outputs/.terragrunt-stack/unit-w-outputs"},
			},
		},
	}

	evalCtx := hclparse.BuildAutoIncludeEvalContext(nil, stackRefs)

	stackVar := evalCtx.Variables["stack"]
	stackRef := stackVar.GetAttr("stack_w_outputs")

	// stack.stack_w_outputs.path works
	assert.Equal(t, "/project/.terragrunt-stack/stack-w-outputs", stackRef.GetAttr("path").AsString())

	// stack.stack_w_outputs.unit_w_outputs.path works
	unitRef := stackRef.GetAttr("unit_w_outputs")
	assert.Equal(t, "/project/.terragrunt-stack/stack-w-outputs/.terragrunt-stack/unit-w-outputs", unitRef.GetAttr("path").AsString())
}

func TestUnitPathsFromStackDir(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}

unit "db" {
  source = "../units/db"
  path   = "db"
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test")
	require.NoError(t, err)
	require.Len(t, paths, 2)
	assert.Contains(t, paths[0], ".terragrunt-stack")
	assert.Contains(t, paths[1], ".terragrunt-stack")
}

func TestUnitPathsFromStackDir_PathWithUnsupportedFunctionReturnsError(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = get_repo_root()
}
`), 0644))

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get_repo_root")
}

func TestUnitPathsFromStackDir_NotAStack(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test")
	require.NoError(t, err)
	assert.Nil(t, paths)
}

func TestUnitPathsFromStackDir_Nonexistent(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/nonexistent")
	require.NoError(t, err)
	assert.Nil(t, paths)
}

func TestUnitPathsFromStackDir_MalformedReturnsError(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`unit "x" { source = "." `), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(fs, "/test")
	require.Error(t, err)
	assert.Nil(t, paths)

	var fpe hclparse.FileParseError
	require.ErrorAs(t, err, &fpe)
}

func TestParseStackFileFromPath(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`
unit "app" {
  source = "../units/app"
  path   = "app"
}
`), 0644))

	result, err := hclparse.ParseStackFileFromPath(fs, "/test")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Units, 1)
	assert.Equal(t, "app", result.Units[0].Name)
}

func TestParseStackFileFromPath_NoFile(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test", 0755))

	result, err := hclparse.ParseStackFileFromPath(fs, "/test")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// ParseStackFileFromPath is strict: passing a regular file produces an error. Callers that may receive non-directory paths (e.g. discovery) must filter them upstream.
func TestParseStackFileFromPath_StackDirIsFileReturnsError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "another-name.hcl")
	require.NoError(t, os.WriteFile(filePath, []byte(`# regular file, not a directory`), 0644))

	result, err := hclparse.ParseStackFileFromPath(vfs.NewOSFS(), filePath)
	require.Error(t, err)
	assert.Nil(t, result)

	var readErr hclparse.FileReadError
	require.ErrorAs(t, err, &readErr)
	// On macOS, t.TempDir() returns paths under /var/folders/... where /var is a symlink to /private/var; util.ResolvePath follows it, so resolve our side too before comparing.
	resolvedFilePath, evalErr := filepath.EvalSymlinks(filePath)
	require.NoError(t, evalErr)
	assert.Equal(t, filepath.Join(resolvedFilePath, "terragrunt.stack.hcl"), readErr.FilePath)
}

func TestParseStackFileFromPath_Symlink(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create the real stack directory
	realDir := filepath.Join(tmpDir, "real-stack")
	require.NoError(t, os.MkdirAll(realDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(realDir, "terragrunt.stack.hcl"), []byte(`
unit "app" {
  source = "../units/app"
  path   = "app"
}
`), 0644))

	// Create a symlink pointing to the real directory
	symlinkDir := filepath.Join(tmpDir, "symlinked-stack")
	require.NoError(t, os.Symlink(realDir, symlinkDir))

	// Parse via the symlink — should work the same as via real path
	result, err := hclparse.ParseStackFileFromPath(vfs.NewOSFS(), symlinkDir)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Units, 1)
	assert.Equal(t, "app", result.Units[0].Name)
}

// Uses OSFS because MemMapFS does not faithfully reproduce os.Symlink semantics that util.ResolvePath relies on.
func TestUnitPathsFromStackDir_Symlink(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	realDir := filepath.Join(tmpDir, "real-stack")
	require.NoError(t, os.MkdirAll(realDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(realDir, "terragrunt.stack.hcl"), []byte(`
unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}
`), 0644))

	symlinkDir := filepath.Join(tmpDir, "symlinked-stack")
	require.NoError(t, os.Symlink(realDir, symlinkDir))

	paths, err := hclparse.UnitPathsFromStackDir(vfs.NewOSFS(), symlinkDir)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	// Path should resolve to the real directory, not the symlink path.
	assert.Contains(t, paths[0], "real-stack")
	assert.NotContains(t, paths[0], "symlinked-stack")
}

func TestDiscoverStackChildUnits_Symlink(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create real source directory with stack file
	realSrcDir := filepath.Join(tmpDir, "real-source")
	require.NoError(t, os.MkdirAll(realSrcDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(realSrcDir, "terragrunt.stack.hcl"), []byte(`
unit "db" {
  source = "../../units/db"
  path   = "db"
}
`), 0644))

	// Create symlink to source
	symlinkSrcDir := filepath.Join(tmpDir, "symlinked-source")
	require.NoError(t, os.Symlink(realSrcDir, symlinkSrcDir))

	refs := hclparse.DiscoverStackChildUnits(vfs.NewOSFS(), symlinkSrcDir, "/gen/stack")
	require.Len(t, refs, 1)
	assert.Equal(t, "db", refs[0].Name)
}

// Best-effort discovery: a malformed nested stack file yields empty refs without an error so the parent stack still parses.
func TestDiscoverStackChildUnits_MalformedReturnsEmpty(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test/stack-src", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/stack-src/terragrunt.stack.hcl", []byte(`unit "x" { source = "."`), 0644))

	refs := hclparse.DiscoverStackChildUnits(fs, "/test/stack-src", "/gen")
	assert.Nil(t, refs)
}

func TestParseStackFile_WithInclude(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/test/includes", 0755))
	require.NoError(t, vfs.WriteFile(fs, "/test/includes/extra.stack.hcl", []byte(`
unit "monitoring" {
  source = "../catalog/units/monitoring"
  path   = "monitoring"
}
`), 0644))

	// Create the main stack file
	mainSrc := `
include "extra" {
  path = "/test/includes/extra.stack.hcl"
}

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}
`

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{Src: []byte(mainSrc), Filename: "/test/terragrunt.stack.hcl", StackDir: "/test"})
	require.NoError(t, err)

	// Should have both units: vpc from main + monitoring from include
	require.Len(t, result.Units, 2)

	names := []string{result.Units[0].Name, result.Units[1].Name}
	assert.Contains(t, names, "vpc")
	assert.Contains(t, names, "monitoring")
}
