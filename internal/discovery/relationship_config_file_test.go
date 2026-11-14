package discovery_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestDiscoveryRelatesADependencyOnAConfigFile(t *testing.T) {
	t.Parallel()

	const dependsOnFile = `
dependency "shared" {
  config_path = "../../shared/another-name.hcl"
}
`

	v := memRepoRootVenv(t, corpusRepoRoot)
	writeFixture(t, v, map[string]string{
		repoPath("apps", "api", "terragrunt.hcl"): dependsOnFile,
		repoPath("apps", "web", "terragrunt.hcl"): dependsOnFile,
		repoPath("shared", "another-name.hcl"):    "dependencies {\n  paths = [\"../base\"]\n}\n",
		repoPath("base", "terragrunt.hcl"):        "",
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = corpusRepoRoot
	opts.RootWorkingDir = corpusRepoRoot

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{"./apps/**"})
	require.NoError(t, err)

	components, err := discovery.NewDiscovery(corpusRepoRoot).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: corpusRepoRoot}).
		WithGitRoot(corpusRepoRoot).
		WithFilters(filters).
		WithRelationships().
		Discover(config.WithCaches(t.Context()), l, v, new(rewrite.Store), opts)
	require.NoError(t, err)
	require.Len(t, components, 2)

	for _, c := range components {
		require.Len(t, c.Dependencies(), 1, c.Path())

		shared := c.Dependencies()[0]

		assert.Equal(t, repoPath("shared", "another-name.hcl"), shared.Path())
		assert.Equal(t, []string{repoPath("base")}, shared.Dependencies().Paths())
	}
}
