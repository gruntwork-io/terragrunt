package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui/command"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureCatalogLocalTemplate = "fixtures/catalog/local-template"
)

func TestCatalogGitRepoUpdate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tempDir := t.TempDir()

	_, err := module.NewRepo(ctx, logger.CreateLogger(), "github.com/gruntwork-io/terraform-fake-modules.git", tempDir, false, false)
	require.NoError(t, err)

	_, err = module.NewRepo(ctx, logger.CreateLogger(), "github.com/gruntwork-io/terraform-fake-modules.git", tempDir, false, false)
	require.NoError(t, err)
}

func TestScaffoldGitRepo(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tempDir := t.TempDir()

	repo, err := module.NewRepo(ctx, logger.CreateLogger(), "github.com/gruntwork-io/terraform-fake-modules.git", tempDir, false, false)
	require.NoError(t, err)

	modules, err := repo.FindModules(ctx)
	require.NoError(t, err)
	assert.Len(t, modules, 4)
}

func TestScaffoldGitModule(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tempDir := t.TempDir()

	repo, err := module.NewRepo(ctx, logger.CreateLogger(), "https://github.com/gruntwork-io/terraform-fake-modules.git", tempDir, false, false)
	require.NoError(t, err)

	modules, err := repo.FindModules(ctx)
	require.NoError(t, err)

	var auroraModule *module.Module

	for _, m := range modules {
		if m.Title() == "Terraform Fake AWS Aurora Module" {
			auroraModule = m
		}
	}

	assert.NotNil(t, auroraModule)

	testPath := t.TempDir()
	opts, err := options.NewTerragruntOptionsForTest(testPath)
	require.NoError(t, err)

	opts.ScaffoldVars = []string{"EnableRootInclude=false"}

	svc := catalog.NewCatalogService(opts).WithRepoURL("https://github.com/gruntwork-io/terraform-fake-modules.git")
	cmd := command.NewScaffold(createLogger(), opts, svc, auroraModule)
	err = cmd.Run()
	require.NoError(t, err)

	cfg := readConfig(t, opts)
	assert.NotEmpty(t, cfg.Inputs)
	assert.Len(t, cfg.Inputs, 1)
	_, found := cfg.Inputs["vpc_id"]
	assert.True(t, found)
	assert.Contains(t, *cfg.Terraform.Source, "git::https://github.com/gruntwork-io/terraform-fake-modules.git//modules/aws/aurora")
}

func TestScaffoldGitModuleHttps(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tempDir := t.TempDir()

	repo, err := module.NewRepo(ctx, logger.CreateLogger(), "https://github.com/gruntwork-io/terraform-fake-modules", tempDir, false, false)
	require.NoError(t, err)

	modules, err := repo.FindModules(ctx)
	require.NoError(t, err)

	var auroraModule *module.Module

	for _, m := range modules {
		if m.Title() == "Terraform Fake AWS Aurora Module" {
			auroraModule = m
		}
	}

	assert.NotNil(t, auroraModule)

	testPath := t.TempDir()
	opts, err := options.NewTerragruntOptionsForTest(testPath)
	require.NoError(t, err)

	opts.ScaffoldVars = []string{"EnableRootInclude=false"}

	svc := catalog.NewCatalogService(opts).WithRepoURL("https://github.com/gruntwork-io/terraform-fake-modules")
	cmd := command.NewScaffold(createLogger(), opts, svc, auroraModule)
	err = cmd.Run()
	require.NoError(t, err)

	cfg := readConfig(t, opts)
	assert.NotEmpty(t, cfg.Inputs)
	assert.Len(t, cfg.Inputs, 1)
	_, found := cfg.Inputs["vpc_id"]
	assert.True(t, found)
	assert.Contains(t, *cfg.Terraform.Source, "git::https://github.com/gruntwork-io/terraform-fake-modules.git//modules/aws/aurora?ref=v0.0.5")

	helpers.RunTerragrunt(t, "terragrunt init --non-interactive --working-dir "+opts.WorkingDir)
}

func TestCatalogWithLocalDefaultTemplate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCatalogLocalTemplate, ".boilerplate")
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureCatalogLocalTemplate)

	targetPath := filepath.Join(rootPath, "app")
	moduleURL := "github.com/gruntwork-io/terragrunt//test/fixtures/inputs"

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt scaffold --non-interactive --working-dir "+targetPath+" "+moduleURL,
	)

	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	assert.FileExists(t, filepath.Join(targetPath, "terragrunt.hcl"))
	assert.FileExists(t, filepath.Join(targetPath, "custom-template.txt"))

	content, err := util.ReadFileAsString(filepath.Join(targetPath, "terragrunt.hcl"))
	require.NoError(t, err)
	assert.Contains(t, content, "# Custom local template")
}

func readConfig(t *testing.T, opts *options.TerragruntOptions) *config.TerragruntConfig {
	t.Helper()

	assert.FileExists(t, opts.WorkingDir+"/terragrunt.hcl")

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(opts.WorkingDir, "terragrunt.hcl"))
	require.NoError(t, err)

	cfg, err := config.ReadTerragruntConfig(t.Context(), logger.CreateLogger(), opts, config.DefaultParserOptions(logger.CreateLogger(), opts))
	require.NoError(t, err)

	return cfg
}
