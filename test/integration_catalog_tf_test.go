//go:build tf

package test_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestTFScaffoldGitModuleHttps(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tempDir := helpers.TmpDirWOSymlinks(t)

	repo, err := module.NewRepo(
		ctx,
		logger.CreateLogger(),
		venv.OSVenv(),
		&module.RepoOpts{
			CloneURL: "https://github.com/gruntwork-io/terraform-fake-modules",
			Path:     tempDir,
		},
	)
	require.NoError(t, err)

	modules, err := repo.FindModules(ctx, logger.CreateLogger(), vfs.NewOSFS())
	require.NoError(t, err)

	var auroraModule *module.Module

	for _, m := range modules {
		if m.Title() == "Terraform Fake AWS Aurora Module" {
			auroraModule = m
		}
	}

	assert.NotNil(t, auroraModule)

	testPath := helpers.TmpDirWOSymlinks(t)
	opts, err := options.NewTerragruntOptionsForTest(testPath)
	require.NoError(t, err)

	opts.ScaffoldVars = []string{"EnableRootInclude=false"}

	err = scaffold.Run(
		ctx,
		createLogger(),
		venv.OSVenv(),
		opts,
		auroraModule.TerraformSourcePath(),
		"",
	)
	require.NoError(t, err)

	cfg := readConfig(t, opts)
	assert.NotEmpty(t, cfg.Inputs)
	assert.Len(t, cfg.Inputs, 1)
	_, found := cfg.Inputs["vpc_id"]
	assert.True(t, found)
	assert.Contains(
		t,
		*cfg.Terraform.Source,
		"git::https://github.com/gruntwork-io/terraform-fake-modules.git//modules/aws/aurora?ref=v0.0.5",
	)

	helpers.RunTerragrunt(t, "terragrunt init --non-interactive --working-dir "+opts.WorkingDir)
}
