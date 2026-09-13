package config_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const stackUnitOutputsRoot = "/live"

const nestedSubnetStackHCL = `
unit "subnet" {
  source = "./units/subnet"
  path   = "subnet"
}
`

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

// generatedUnitConfig returns the path, relative to stackUnitOutputsRoot, of the terragrunt.hcl
// a unit at unitPath generates to beneath the chain of nested stack paths.
func generatedUnitConfig(unitPath string, stackPaths ...string) string {
	parts := []string{config.StackDir}
	for _, stackPath := range stackPaths {
		parts = append(parts, stackPath, config.StackDir)
	}

	return filepath.Join(append(parts, unitPath, config.DefaultTerragruntConfigPath)...)
}

// roleOutput is the output JSON of a unit whose only output, role, holds role.
func roleOutput(role string) string {
	return `{"role":{"value":"` + role + `","type":"string"}}`
}

// withConfigFiles adds an empty terragrunt.hcl to files at each of configPaths.
func withConfigFiles(files map[string]string, configPaths ...string) map[string]string {
	for _, configPath := range configPaths {
		files[configPath] = ""
	}

	return files
}

// newStackOutputsContext builds an in-memory stack tree from files and primes the output cache
// with outputs, both keyed by path relative to stackUnitOutputsRoot. A unit left out of outputs
// fails its fetch.
func newStackOutputsContext(
	t *testing.T,
	files map[string]string,
	outputs map[string]string,
) (context.Context, *config.ParsingContext) {
	t.Helper()

	v := venvtest.New().WithFS(venvtest.NewFS(t, stackUnitOutputsRoot, files))

	ctx, pctx := newTestParsingContext(t, v, filepath.Join(stackUnitOutputsRoot, "terragrunt.hcl"))

	// The caches the fetch path reads live on the context, and only WithConfigValues puts
	// them there. Without it every lookup below builds a throwaway cache and misses.
	ctx = config.WithConfigValues(ctx)

	// Priming the fetch cache is what keeps this off a real tofu binary: the venv's exec is
	// fail-closed, so a cache miss would surface as an error rather than a shell-out.
	jsonCache := cache.ContextCache[[]byte](ctx, config.JSONOutputCacheContextKey)

	// ContextCache hands back a detached instance when the key is absent, so dropping the call
	// above would leave every Put below invisible to the fetch path and quietly send this test
	// looking for a real tofu binary. Two lookups agreeing on one pointer prove it is installed.
	require.Same(
		t,
		jsonCache,
		cache.ContextCache[[]byte](ctx, config.JSONOutputCacheContextKey),
		"the output cache is not on the context, so priming it below would do nothing",
	)

	for configPath, out := range outputs {
		jsonCache.Put(ctx, filepath.Join(stackUnitOutputsRoot, configPath), []byte(out))
	}

	return ctx, pctx
}

// requireAttr fails with the shape it found when value has no attribute name, so a missing
// level reports what was collected rather than panicking on GetAttr.
func requireAttr(t *testing.T, value cty.Value, name string) cty.Value {
	t.Helper()

	require.Truef(
		t,
		value.Type().IsObjectType() && value.Type().HasAttribute(name),
		"no %q attribute, collected as %s",
		name,
		value.Type().FriendlyName(),
	)

	return value.GetAttr(name)
}

// TestCollectStackOutputsNestsExpandedElements pins that every element of an expanded unit
// survives collection. The elements share their block's label, so keying by name alone kept
// only the last one, handing another element's outputs to whatever read the label.
func TestCollectStackOutputsNestsExpandedElements(t *testing.T) {
	t.Parallel()

	outputs := map[string]string{
		generatedUnitConfig("aurora/web"): roleOutput("web"),
		generatedUnitConfig("aurora/api"): roleOutput("api"),
		generatedUnitConfig("vpc"):        roleOutput("vpc"),
	}

	files := map[string]string{}
	for configPath := range outputs {
		files[configPath] = ""
	}

	ctx, pctx := newStackOutputsContext(t, files, outputs)

	collected, err := config.CollectStackOutputs(
		ctx,
		pctx,
		logger.CreateLogger(),
		stackUnitOutputsRoot,
		&config.StackConfig{
			Units: []*config.Unit{
				expandedUnit("aurora", "web", "aurora/web"),
				expandedUnit("aurora", "api", "aurora/api"),
				{Name: "vpc", Path: "vpc"},
			},
		},
		&config.Dependency{},
		inthclparse.DefaultMaxStackRecursionDepth,
	)
	require.NoError(t, err)

	require.Contains(t, collected, "aurora")

	aurora := collected["aurora"]
	assert.Equal(t, "web", requireAttr(t, requireAttr(t, aurora, "web"), "role").AsString())
	assert.Equal(t, "api", requireAttr(t, requireAttr(t, aurora, "api"), "role").AsString())

	// A unit with no expansion keeps the flat address it always had.
	require.Contains(t, collected, "vpc")
	assert.Equal(t, "vpc", requireAttr(t, collected["vpc"], "role").AsString())
}

// TestCollectStackOutputsNestsNestedStackUnits pins that a unit generated by a nested stack is
// collected under the nested stack's name, the address `terragrunt stack output` gives it. Only
// the stack file's own units used to be collected, though the run queue waited for nested ones.
func TestCollectStackOutputsNestsNestedStackUnits(t *testing.T) {
	t.Parallel()

	vpcConfig := generatedUnitConfig("vpc")
	subnetConfig := generatedUnitConfig("subnet", "network")

	files := withConfigFiles(map[string]string{
		filepath.Join(config.StackDir, "network", config.DefaultStackFile): nestedSubnetStackHCL,
	}, vpcConfig, subnetConfig)

	ctx, pctx := newStackOutputsContext(t, files, map[string]string{
		vpcConfig:    roleOutput("vpc"),
		subnetConfig: roleOutput("subnet"),
	})

	collected, err := config.CollectStackOutputs(
		ctx,
		pctx,
		logger.CreateLogger(),
		stackUnitOutputsRoot,
		&config.StackConfig{
			Units:  []*config.Unit{{Name: "vpc", Path: "vpc"}},
			Stacks: []*config.Stack{{Name: "network", Path: "network"}},
		},
		&config.Dependency{},
		inthclparse.DefaultMaxStackRecursionDepth,
	)
	require.NoError(t, err)

	require.Contains(t, collected, "vpc")
	assert.Equal(t, "vpc", requireAttr(t, collected["vpc"], "role").AsString())

	require.Contains(t, collected, "network")
	subnet := requireAttr(t, collected["network"], "subnet")
	assert.Equal(t, "subnet", requireAttr(t, subnet, "role").AsString())
}

// TestCollectStackOutputsMocksNestedStackUnits pins that a nested stack's unit with no outputs
// falls back to the mock_outputs entry under the nested stack's name.
func TestCollectStackOutputsMocksNestedStackUnits(t *testing.T) {
	t.Parallel()

	vpcConfig := generatedUnitConfig("vpc")
	subnetConfig := generatedUnitConfig("subnet", "network")

	files := withConfigFiles(map[string]string{
		filepath.Join(config.StackDir, "network", config.DefaultStackFile): nestedSubnetStackHCL,
	}, vpcConfig, subnetConfig)

	ctx, pctx := newStackOutputsContext(t, files, map[string]string{
		vpcConfig: roleOutput("vpc"),
	})

	// render falls back to mocks whatever the fetch error, which is the only way to reach the
	// fallback here without a remote state backend reporting a missing object.
	pctx.TerraformCliArgs = iacargs.New("render")

	mocks := cty.ObjectVal(map[string]cty.Value{
		"network": cty.ObjectVal(map[string]cty.Value{
			"subnet": cty.ObjectVal(map[string]cty.Value{
				"role": cty.StringVal("mock-subnet"),
			}),
		}),
	})

	collected, err := config.CollectStackOutputs(
		ctx,
		pctx,
		logger.CreateLogger(),
		stackUnitOutputsRoot,
		&config.StackConfig{
			Units:  []*config.Unit{{Name: "vpc", Path: "vpc"}},
			Stacks: []*config.Stack{{Name: "network", Path: "network"}},
		},
		&config.Dependency{Name: "networking", MockOutputs: &mocks},
		inthclparse.DefaultMaxStackRecursionDepth,
	)
	require.NoError(t, err)

	require.Contains(t, collected, "vpc")
	assert.Equal(t, "vpc", requireAttr(t, collected["vpc"], "role").AsString())

	require.Contains(t, collected, "network")
	subnet := requireAttr(t, collected["network"], "subnet")
	assert.Equal(t, "mock-subnet", requireAttr(t, subnet, "role").AsString())
}

// TestCollectStackOutputsRejectsUnitAndStackSharingAName pins that a unit and a nested stack with
// the same name fail collection. Stack file validation checks unit names and stack names
// separately, so the two can coexist and would otherwise overwrite each other's outputs.
func TestCollectStackOutputsRejectsUnitAndStackSharingAName(t *testing.T) {
	t.Parallel()

	unitConfig := generatedUnitConfig("network-unit")
	subnetConfig := generatedUnitConfig("subnet", "network")

	files := withConfigFiles(map[string]string{
		filepath.Join(config.StackDir, "network", config.DefaultStackFile): nestedSubnetStackHCL,
	}, unitConfig, subnetConfig)

	ctx, pctx := newStackOutputsContext(t, files, map[string]string{
		unitConfig:   roleOutput("unit"),
		subnetConfig: roleOutput("subnet"),
	})

	_, err := config.CollectStackOutputs(
		ctx,
		pctx,
		logger.CreateLogger(),
		stackUnitOutputsRoot,
		&config.StackConfig{
			Units:  []*config.Unit{{Name: "network", Path: "network-unit"}},
			Stacks: []*config.Stack{{Name: "network", Path: "network"}},
		},
		&config.Dependency{},
		inthclparse.DefaultMaxStackRecursionDepth,
	)

	var collision config.StackOutputAddressCollisionError

	require.ErrorAs(t, err, &collision)
	assert.Equal(t, "network", collision.Name)
}

// TestCollectStackOutputsBoundsNestingDepth pins that a nested stack generating back into its own
// directory stops at maxDepth instead of recursing without end.
func TestCollectStackOutputsBoundsNestingDepth(t *testing.T) {
	t.Parallel()

	const loopingStackHCL = `
stack "loop" {
  source = "./stacks/loop"
  path   = "."

  no_dot_terragrunt_stack = true
}
`

	files := map[string]string{
		filepath.Join(config.StackDir, "loop", config.DefaultStackFile): loopingStackHCL,
	}

	ctx, pctx := newStackOutputsContext(t, files, map[string]string{})

	const maxDepth = 3

	_, err := config.CollectStackOutputs(
		ctx,
		pctx,
		logger.CreateLogger(),
		stackUnitOutputsRoot,
		&config.StackConfig{
			Stacks: []*config.Stack{{Name: "loop", Path: "loop"}},
		},
		&config.Dependency{},
		maxDepth,
	)

	var depthErr inthclparse.StackRecursionDepthExceededError

	require.ErrorAs(t, err, &depthErr)
	assert.Equal(t, maxDepth, depthErr.MaxDepth)
}
