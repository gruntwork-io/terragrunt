package config_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// resolveRunConfigString parses content as the config at cfgPath with [config.Store.ResolveRunConfig], reading it
// from a memory layer over the filesystem of v.
func resolveRunConfigString(
	tb testing.TB,
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	pctx *config.ParsingContext,
	cfgPath, content string,
) (*config.TerragruntConfig, error) {
	tb.Helper()

	v = withConfigFile(tb, v, cfgPath, content)

	return s.ResolveRunConfig(ctx, l, v, config.NewParseContext(v, pctx), cfgPath)
}

// resolvePartialConfigString parses content as the config at cfgPath with [config.Store.ResolvePartialConfig],
// reading it from a memory layer over the filesystem of v.
func resolvePartialConfigString(
	tb testing.TB,
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	pctx *config.ParsingContext,
	cfgPath, content string,
) (*config.TerragruntConfig, error) {
	tb.Helper()

	v = withConfigFile(tb, v, cfgPath, content)

	return s.ResolvePartialConfig(ctx, l, v, config.NewParseContext(v, pctx), cfgPath)
}

// withConfigFile returns a copy of v whose filesystem holds content at cfgPath in a memory layer over that of v.
func withConfigFile(tb testing.TB, v *venv.Venv, cfgPath, content string) *venv.Venv {
	tb.Helper()

	fsys := vfs.NewOverlayFS(v.FS, vfs.NewMemMapFS())
	require.NoError(tb, vfs.WriteFile(fsys, cfgPath, []byte(content), 0o644))

	return v.WithFS(fsys)
}
