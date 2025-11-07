package common_test

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

func TestResolveFromDiscovery_UsesDiscoveryConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a discovery unit with a pre-parsed config including Terraform.source
	src := "./module"
	tgCfg := &config.TerragruntConfig{
		Terraform: &config.TerraformConfig{Source: &src},
	}

	discUnit := component.NewUnit(tmpDir)
	discUnit.WithConfig(tgCfg)

	discovered := component.Components{discUnit}

	// Prepare options and stack (ensure config file exists)
	tgPath := filepath.Join(tmpDir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(tgPath, []byte(""), 0o600))

	opts, err := options.NewTerragruntOptionsForTest(tgPath)
	require.NoError(t, err)

	// Quiet test logging with non-nil formatter
	l := thlogger.CreateLogger()

	stack := &common.Stack{TerragruntOptions: opts}
	resolver, err := common.NewUnitResolver(context.Background(), stack)
	require.NoError(t, err)

	units, err := resolver.ResolveFromDiscovery(context.Background(), l, discovered)
	require.NoError(t, err)

	require.Len(t, units, 1)
	require.Equal(t, tmpDir, units[0].Component.Path())
	require.NotNil(t, units[0].Component.Config().Terraform)
	require.NotNil(t, units[0].Component.Config().Terraform.Source)
	require.Equal(t, src, *units[0].Component.Config().Terraform.Source)
}
