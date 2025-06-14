package catalog_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogCommandInitialization(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Create mock repository function for testing
	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		// Create a temporary directory structure for testing
		dummyRepoDir := filepath.Join(t.TempDir(), strings.ReplaceAll(repoURL, "github.com/gruntwork-io/", ""))
		os.MkdirAll(filepath.Join(dummyRepoDir, ".git"), 0755)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "config"), []byte("[remote \"origin\"]\nurl = "+repoURL), 0644)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)

		// Create test modules based on repoURL
		switch repoURL {
		case "github.com/gruntwork-io/test-repo-1":
			readme1Path := filepath.Join(dummyRepoDir, "README.md")
			os.WriteFile(readme1Path, []byte("# AWS VPC Module\nThis module creates a VPC in AWS with all the necessary components."), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "main.tf"), []byte("# VPC terraform configuration"), 0644)
		default:
			return nil, fmt.Errorf("unexpected repoURL in mock: %s", repoURL)
		}

		return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
	}

	// Create a temporary root config file
	tmpDir := t.TempDir()
	rootFile := filepath.Join(tmpDir, "root.hcl")
	err = os.WriteFile(rootFile, []byte(`catalog {
	urls = [
		"github.com/gruntwork-io/test-repo-1",
	]
}`), 0600)
	require.NoError(t, err)

	unitDir := filepath.Join(tmpDir, "unit")
	os.MkdirAll(unitDir, 0755)
	opts.TerragruntConfigPath = filepath.Join(unitDir, "terragrunt.hcl")
	opts.ScaffoldRootFileName = config.RecommendedParentConfigName

	// Test that the catalog service loads correctly
	svc := catalog.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo)

	ctx := t.Context()
	l := logger.CreateLogger()
	err = svc.Load(ctx, l)
	require.NoError(t, err)

	modules := svc.Modules()
	assert.Len(t, modules, 1, "should have 1 test module")
	assert.Equal(t, "AWS VPC Module", modules[0].Title())

	// Test that the Run function would not return an error for no modules found
	// (since we have modules loaded)
	assert.NotEmpty(t, modules, "catalog should have modules for TUI to display")
}
