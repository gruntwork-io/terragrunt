package hclparse_test

import (
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
