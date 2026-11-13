package generate_test

import (
	"fmt"
	"path/filepath"
	"testing"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStackDiscoveryRespectsParseErrorSuppression(t *testing.T) {
	t.Parallel()

	for _, enabled := range []bool{false, true} {
		for _, suppress := range []bool{false, true} {
			t.Run(fmt.Sprintf("experiment=%t/suppress=%t", enabled, suppress), func(t *testing.T) {
				t.Parallel()

				root := venvtest.Root("/repo")
				fsys := venvtest.NewFS(t, root, map[string]string{
					"broken/terragrunt.hcl": `locals { bad = absent.value }`,
					"valid/terragrunt.hcl":  "",
				})
				v := venvtest.NewOSWithEmptyEnv().WithFS(fsys)
				opts := options.NewTerragruntOptions(v.Exec)

				opts.WorkingDir = root
				if enabled {
					require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))
				}

				store := new(rewrite.Store)
				ctx := generate.WithCoordinator(config.WithCaches(t.Context()), fsys, store, opts)

				d := discovery.NewDiscovery(root).WithRelationships()
				if suppress {
					d = d.WithSuppressParseErrors()
				}

				components, err := d.Discover(ctx, logger.CreateLogger(), v, store, opts)
				if !suppress {
					require.Error(t, err)
					return
				}

				require.NoError(t, err)

				var paths []string
				for _, component := range components {
					paths = append(paths, component.Path())
				}

				assert.Contains(t, paths, filepath.Join(root, "valid"))
			})
		}
	}
}

func TestStackDiscoveryDoesNotSuppressGenerationFailures(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		source   string
		expected string
	}{
		{name: "copy failure", source: `"../missing-source"`, expected: "missing-source"},
		{name: "stack parse failure", source: "absent.value", expected: "absent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			fsys := venvtest.NewFS(t, root, map[string]string{
				"consumer/terragrunt.hcl": `dependencies { paths = ["../producer"] }`,
				"producer/terragrunt.stack.hcl": fmt.Sprintf(`unit "app" {
 source = %s
 path = "app"
}`, tc.source),
			})
			v := venvtest.NewOSWithEmptyEnv().WithFS(fsys)
			opts := options.NewTerragruntOptions(v.Exec)
			opts.WorkingDir = root
			opts.NoCAS = true
			require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))

			store := new(rewrite.Store)
			ctx := generate.WithCoordinator(config.WithCaches(t.Context()), fsys, store, opts)
			_, err := discovery.NewDiscovery(root).WithRelationships().WithSuppressParseErrors().
				Discover(ctx, logger.CreateLogger(), v, store, opts)

			require.ErrorContains(t, err, tc.expected)
		})
	}
}
