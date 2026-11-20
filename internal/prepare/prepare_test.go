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

func TestPrepareConfigReadsThroughTheStore(t *testing.T) {
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

	v := venv.MemVenv().WithFS(fsys)
	store := new(rewrite.Store)

	opts, err := options.NewTerragruntOptionsWithConfigPath(path)
	require.NoError(t, err)

	prepared, err := prepare.PrepareConfig(config.WithCaches(t.Context()), logger.CreateLogger(), v, store, opts)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"region": "us-east-1"}, prepared.Cfg.Inputs)
	assert.NotZero(t, store.Stats().Misses)
}
