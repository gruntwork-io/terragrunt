package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const stackUnitOutputsRoot = "/live"

// expandedUnit builds a unit as the expanding decoder hands it over: the block's label with
// the element's key alongside it.
func expandedUnit(name, key, path string) *config.Unit {
	return &config.Unit{
		Name: name,
		Path: path,
		Expansion: &hclparse.ExpansionBlock{
			EachKey: new(key),
		},
	}
}

// TestCollectStackUnitOutputsNestsExpandedElements pins that every element of an expanded unit
// survives collection. The elements share their block's label, so keying by name alone kept
// only the last one, handing another element's outputs to whatever read the label.
func TestCollectStackUnitOutputsNestsExpandedElements(t *testing.T) {
	t.Parallel()

	outputs := map[string]string{
		"aurora/web": `{"role":{"value":"web","type":"string"}}`,
		"aurora/api": `{"role":{"value":"api","type":"string"}}`,
		"vpc":        `{"role":{"value":"vpc","type":"string"}}`,
	}

	files := map[string]string{}
	for unitPath := range outputs {
		files[filepath.Join(config.StackDir, unitPath, config.DefaultTerragruntConfigPath)] = ""
	}

	v := venvtest.New().WithFS(venvtest.NewFS(t, stackUnitOutputsRoot, files))

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, v, filepath.Join(stackUnitOutputsRoot, "terragrunt.hcl"))

	jsonCache := cache.ContextCache[[]byte](ctx, config.JSONOutputCacheContextKey)

	for unitPath, out := range outputs {
		jsonCache.Put(ctx, filepath.Join(
			stackUnitOutputsRoot, config.StackDir, unitPath, config.DefaultTerragruntConfigPath,
		), []byte(out))
	}

	collected, err := config.CollectStackUnitOutputs(
		ctx,
		pctx,
		l,
		stackUnitOutputsRoot,
		[]*config.Unit{
			expandedUnit("aurora", "web", "aurora/web"),
			expandedUnit("aurora", "api", "aurora/api"),
			{Name: "vpc", Path: "vpc"},
		},
		&config.Dependency{},
	)
	require.NoError(t, err)

	role := func(v cty.Value) string { return v.GetAttr("role").AsString() }

	require.Contains(t, collected, "aurora")

	aurora := collected["aurora"]

	for _, key := range []string{"web", "api"} {
		require.Truef(
			t,
			aurora.Type().HasAttribute(key),
			"aurora holds no %q element, collected as %s",
			key,
			aurora.Type().FriendlyName(),
		)
	}

	assert.Equal(t, "web", role(aurora.GetAttr("web")))
	assert.Equal(t, "api", role(aurora.GetAttr("api")))

	require.Contains(t, collected, "vpc")
	assert.Equal(t, "vpc", role(collected["vpc"]))
}
