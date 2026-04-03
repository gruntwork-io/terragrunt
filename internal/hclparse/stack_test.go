package hclparse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

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

func TestExtractUnitRefs(t *testing.T) {
	t.Parallel()

	units := []*hclparse.UnitBlockHCL{
		{Name: "vpc", Path: "vpc", Source: "../modules/vpc"},
		{Name: "app", Path: "app-service", Source: "../modules/app"},
	}

	refs := hclparse.ExtractUnitRefs(units)

	require.Len(t, refs, 2)
	assert.Equal(t, "vpc", refs[0].Name)
	assert.Equal(t, "vpc", refs[0].Path)
	assert.Equal(t, "app", refs[1].Name)
	assert.Equal(t, "app-service", refs[1].Path)
}

func TestExtractStackRefs(t *testing.T) {
	t.Parallel()

	stacks := []*hclparse.StackBlockHCL{
		{Name: "networking", Path: "networking", Source: "../stacks/networking"},
	}

	refs := hclparse.ExtractStackRefs(stacks)

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

	tmpDir := t.TempDir()
	stackSrcDir := filepath.Join(tmpDir, "stack-src")
	require.NoError(t, os.MkdirAll(stackSrcDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stackSrcDir, "terragrunt.stack.hcl"), []byte(`
unit "vpc" {
  source = "../../units/vpc"
  path   = "vpc"
}

unit "db" {
  source = "../../units/db"
  path   = "db"
}
`), 0644))

	refs := hclparse.DiscoverStackChildUnits(stackSrcDir, "/gen/networking")

	require.Len(t, refs, 2)
	assert.Equal(t, "vpc", refs[0].Name)
	assert.Equal(t, filepath.Join("/gen/networking", ".terragrunt-stack", "vpc"), refs[0].Path)
	assert.Equal(t, "db", refs[1].Name)
	assert.Equal(t, filepath.Join("/gen/networking", ".terragrunt-stack", "db"), refs[1].Path)
}

func TestDiscoverStackChildUnits_NoDotTerragruntStack(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	stackSrcDir := filepath.Join(tmpDir, "stack-src")
	require.NoError(t, os.MkdirAll(stackSrcDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stackSrcDir, "terragrunt.stack.hcl"), []byte(`
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

	refs := hclparse.DiscoverStackChildUnits(stackSrcDir, "/gen/networking")

	require.Len(t, refs, 2)
	// vpc has no_dot_terragrunt_stack=true, goes directly under stackGenDir
	assert.Equal(t, "vpc", refs[0].Name)
	assert.Equal(t, filepath.Join("/gen/networking", "vpc"), refs[0].Path)
	// db is normal, goes under .terragrunt-stack/
	assert.Equal(t, "db", refs[1].Name)
	assert.Equal(t, filepath.Join("/gen/networking", ".terragrunt-stack", "db"), refs[1].Path)
}

func TestDiscoverStackChildUnits_NoStackFile(t *testing.T) {
	t.Parallel()

	refs := hclparse.DiscoverStackChildUnits("/nonexistent", "/gen")
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
