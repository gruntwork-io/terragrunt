package runnerpool_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/options"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestDiscoveryResolverMatchesLegacyPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a trivial tf file so legacy resolver doesn't skip the unit
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(""), 0o600))
	tgPath := filepath.Join(tmpDir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(tgPath, []byte(""), 0o600))

	// Discovery produces a component with or without config; using empty config is fine here
	discUnit := component.NewUnit(tmpDir).WithConfig(&config.TerragruntConfig{})
	discovered := component.Components{discUnit}

	// Stack and resolver
	opts, err := options.NewTerragruntOptionsForTest(tgPath)
	require.NoError(t, err)

	stack := &common.Stack{TerragruntOptions: opts}
	resolver, err := common.NewUnitResolver(context.Background(), stack)
	require.NoError(t, err)

	l := thlogger.CreateLogger()

	// New path
	fromDiscovery, err := resolver.ResolveFromDiscovery(context.Background(), l, discovered)
	require.NoError(t, err)
	require.Len(t, fromDiscovery, 1)

	// Legacy path
	unitPaths := []string{filepath.Join(tmpDir, "terragrunt.hcl")}
	legacy, err := resolver.ResolveTerraformModules(context.Background(), l, unitPaths)
	require.NoError(t, err)
	require.Len(t, legacy, 1)

	require.Equal(t, fromDiscovery[0].Path, legacy[0].Path)
}
