package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/command"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScaffoldGitRepo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tempDir, err := os.MkdirTemp("", "catalog-*")
	require.NoError(t, err)

	repo, err := module.NewRepo(ctx, "github.com/gruntwork-io/terraform-fake-modules.git", tempDir)
	require.NoError(t, err)

	modules, err := repo.FindModules(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 4, len(modules))
}

func TestScaffoldGitModule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tempDir, err := os.MkdirTemp("", "catalog-*")
	require.NoError(t, err)

	repo, err := module.NewRepo(ctx, "https://github.com/gruntwork-io/terraform-fake-modules.git", tempDir)
	require.NoError(t, err)

	modules, err := repo.FindModules(ctx)
	assert.NoError(t, err)
	var auroraModule *module.Module
	for _, m := range modules {
		if m.Title() == "Terraform Fake AWS Aurora Module" {
			auroraModule = m
		}
	}
	assert.NotNil(t, auroraModule)

	testPath := t.TempDir()
	opts, err := options.NewTerragruntOptionsForTest(testPath)
	assert.NoError(t, err)

	opts.ScaffoldVars = []string{"EnableRootInclude=false"}

	cmd := command.NewScaffold(opts, auroraModule)
	err = cmd.Run()
	assert.NoError(t, err)

	cfg := readConfig(t, opts)
	assert.NotEmpty(t, cfg.Inputs)
	assert.Equal(t, 1, len(cfg.Inputs))
	_, found := cfg.Inputs["vpc_id"]
	assert.True(t, found)
	assert.Contains(t, *cfg.Terraform.Source, "git::https://github.com/gruntwork-io/terraform-fake-modules.git//modules/aws/aurora")
}

func TestScaffoldGitModuleHttps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tempDir, err := os.MkdirTemp("", "catalog-*")
	require.NoError(t, err)

	repo, err := module.NewRepo(ctx, "https://github.com/gruntwork-io/terraform-fake-modules", tempDir)
	assert.NoError(t, err)

	modules, err := repo.FindModules(ctx)
	assert.NoError(t, err)
	var auroraModule *module.Module
	for _, m := range modules {
		if m.Title() == "Terraform Fake AWS Aurora Module" {
			auroraModule = m
		}
	}
	assert.NotNil(t, auroraModule)

	testPath := t.TempDir()
	opts, err := options.NewTerragruntOptionsForTest(testPath)
	assert.NoError(t, err)

	opts.ScaffoldVars = []string{"EnableRootInclude=false"}

	cmd := command.NewScaffold(opts, auroraModule)
	err = cmd.Run()
	assert.NoError(t, err)

	cfg := readConfig(t, opts)
	assert.NotEmpty(t, cfg.Inputs)
	assert.Equal(t, 1, len(cfg.Inputs))
	_, found := cfg.Inputs["vpc_id"]
	assert.True(t, found)
	assert.Contains(t, *cfg.Terraform.Source, "git::https://github.com/gruntwork-io/terraform-fake-modules.git//modules/aws/aurora?ref=v0.0.5")

	runTerragrunt(t, fmt.Sprintf("terragrunt init --terragrunt-non-interactive --terragrunt-working-dir %s", opts.WorkingDir))
}

func readConfig(t *testing.T, opts *options.TerragruntOptions) *config.TerragruntConfig {
	assert.FileExists(t, fmt.Sprintf("%s/terragrunt.hcl", opts.WorkingDir))

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(opts.WorkingDir, "terragrunt.hcl"))
	assert.NoError(t, err)

	cfg, err := config.ReadTerragruntConfig(opts)
	assert.NoError(t, err)

	return cfg
}
