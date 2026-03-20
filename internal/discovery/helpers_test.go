package discovery

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestExtractDependencyPathsSkipsUnknownDependencyConfigPath(t *testing.T) {
	t.Parallel()

	// This test verifies that if a dependency config path is not a known string
	// the function returns an error and does not attempt to resolve the path.
	cfg := &config.TerragruntConfig{
		TerragruntDependencies: config.Dependencies{
			{
				Name:       "target",
				ConfigPath: cty.UnknownVal(cty.String),
			},
		},
	}

	component := component.NewUnit("/tmp/live/test")

	depPaths, err := extractDependencyPaths(cfg, component)
	require.ErrorContains(t, err, "dependency config path is not a valid known string")
	assert.Empty(t, depPaths)
}
