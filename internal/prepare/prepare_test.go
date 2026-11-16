package prepare_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/prepare"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestPrepareConfigUsesTheStoreOnlyWhenTheRewriteIsSelected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		env        map[string]string
		assertions func(t *testing.T, stats rewrite.Stats)
		name       string
	}{
		{
			name: "selected",
			env:  map[string]string{rewrite.EnvRewrite: "true"},
			assertions: func(t *testing.T, stats rewrite.Stats) {
				t.Helper()
				assert.NotZero(t, stats.Misses)
			},
		},
		{
			name: "not selected",
			env:  map[string]string{},
			assertions: func(t *testing.T, stats rewrite.Stats) {
				t.Helper()
				assert.Zero(t, stats)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "unit", config.DefaultTerragruntConfigPath)

			fsys := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
locals {
  region = "us-east-1"
}

inputs = {
  region = local.region
}
`), 0o644))

			v := venv.MemVenv().WithFS(fsys).WithEnv(tt.env)
			store := new(rewrite.Store)

			opts, err := options.NewTerragruntOptionsWithConfigPath(v.Exec, path)
			require.NoError(t, err)

			prepared, err := prepare.PrepareConfig(config.WithCaches(t.Context()), logger.CreateLogger(), v, store, opts)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"region": "us-east-1"}, prepared.Cfg.Inputs)
			tt.assertions(t, store.Stats())
		})
	}
}
