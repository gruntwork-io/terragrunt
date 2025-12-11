package runnerpool_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/options"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestDiscoveryResolverMatchesLegacyPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a trivial tf file so the resolver doesn't skip the unit
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(""), 0o600))
	tgPath := filepath.Join(tmpDir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(tgPath, []byte(""), 0o600))

	// Discovery produces a component with or without config; using empty config is fine here
	discUnit := component.NewUnit(tmpDir).WithConfig(&config.TerragruntConfig{})
	discovered := component.Components{discUnit}

	// Build runner stack from discovery and verify units
	opts, err := options.NewTerragruntOptionsForTest(tgPath)
	require.NoError(t, err)

	l := thlogger.CreateLogger()

	runner, err := runnerpool.NewRunnerPoolStack(context.Background(), l, opts, discovered, nil)
	require.NoError(t, err)

	units := runner.GetStack().Units
	require.Len(t, units, 1)
	require.Equal(t, tmpDir, units[0].Path())
}
