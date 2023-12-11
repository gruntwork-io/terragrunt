package test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/command"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestScaffoldGitRepo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	repo, err := module.NewRepo(ctx, "github.com/gruntwork-io/terraform-fake-modules.git")
	assert.NoError(t, err)

	modules, err := repo.FindModules(ctx)
	assert.Equal(t, 4, len(modules))
}

func TestScaffoldGitModule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	repo, err := module.NewRepo(ctx, "github.com/gruntwork-io/terraform-fake-modules.git")
	assert.NoError(t, err)

	modules, err := repo.FindModules(ctx)
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

	assert.FileExists(t, fmt.Sprintf("%s/terragrunt.hcl", opts.WorkingDir))

	opts, err = options.NewTerragruntOptionsForTest(filepath.Join(opts.WorkingDir, "terragrunt.hcl"))
	assert.NoError(t, err)

	cfg, err := config.ReadTerragruntConfig(opts)
	assert.NoError(t, err)
	assert.NotEmpty(t, cfg.Inputs)
	assert.Equal(t, 1, len(cfg.Inputs))
	_, found := cfg.Inputs["vpc_id"]
	assert.True(t, found)
	assert.Contains(t, *cfg.Terraform.Source, "git::https://github.com/gruntwork-io/terraform-fake-modules.git//modules/aws/aurora")
}
