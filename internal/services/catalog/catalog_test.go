package catalog_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModules_HappyPath(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.ScaffoldRootFileName = config.RecommendedParentConfigName

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		// Use t.TempDir() for the dummyRepoDir to ensure cleanup and parallelism safety.
		dummyRepoDir := filepath.Join(t.TempDir(), strings.ReplaceAll(repoURL, "github.com/gruntwork-io/", ""))
		os.MkdirAll(filepath.Join(dummyRepoDir, ".git"), 0755)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "config"), []byte("[remote \"origin\"]\nurl = "+repoURL), 0644)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)

		if repoURL == "github.com/gruntwork-io/repo1" {
			readme1Path := filepath.Join(dummyRepoDir, "README.md")
			os.WriteFile(readme1Path, []byte("# module1-title\nThis is module1."), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "module1.tf"), []byte{}, 0644)

			return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
		}

		if repoURL == "github.com/gruntwork-io/repo2" {
			readme2Path := filepath.Join(dummyRepoDir, "README.md")
			os.WriteFile(readme2Path, []byte("# module2-title\nThis is module2."), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "module2.tf"), []byte{}, 0644)

			return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
		}

		return nil, fmt.Errorf("unexpected repoURL in mock newRepoFunc: %s", repoURL)
	}

	tmpDir := t.TempDir()
	rootFile := filepath.Join(tmpDir, "root.hcl")
	err := os.WriteFile(rootFile, []byte(`catalog {
	urls = [
		"github.com/gruntwork-io/repo1",
		"github.com/gruntwork-io/repo2",
	]
}`), 0600)
	require.NoError(t, err)

	unitDir := filepath.Join(tmpDir, "unit")
	os.MkdirAll(unitDir, 0755)
	opts.TerragruntConfigPath = filepath.Join(unitDir, "terragrunt.hcl")

	svc := catalog.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo)

	l := logger.CreateLogger()

	err = svc.Load(t.Context(), l)
	require.NoError(t, err)

	modules := svc.Modules()

	require.NotNil(t, modules)
	assert.Len(t, modules, 2)
	assert.Equal(t, "module1-title", modules[0].Title())
	assert.Equal(t, "module2-title", modules[1].Title())
}

func TestListModules_NoRepositoriesConfigured(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.ScaffoldRootFileName = config.RecommendedParentConfigName

	tmpDir := t.TempDir()
	opts.TerragruntConfigPath = filepath.Join(tmpDir, "nonexistent-terragrunt.hcl")

	// No customNewRepoFunc needed as it should error before trying to create a repo.
	svc := catalog.NewCatalogService(opts)
	l := logger.CreateLogger()
	err := svc.Load(t.Context(), l)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no catalog URLs provided")
}

func TestListModules_SingleRepoFromFlag(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.ScaffoldRootFileName = config.RecommendedParentConfigName

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		if repoURL == "github.com/gruntwork-io/only-repo" {
			dummyRepoDir := filepath.Join(t.TempDir(), "only-repo")
			os.MkdirAll(filepath.Join(dummyRepoDir, ".git"), 0755)
			os.WriteFile(filepath.Join(dummyRepoDir, ".git", "config"), []byte("[remote \"origin\"]\nurl = "+repoURL), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "README.md"), []byte("# moduleA-title"), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "moduleA.tf"), []byte{}, 0644)

			return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
		}

		return nil, fmt.Errorf("unexpected repoURL: %s", repoURL)
	}

	svc := catalog.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo).WithRepoURL("github.com/gruntwork-io/only-repo")
	l := logger.CreateLogger()
	err := svc.Load(t.Context(), l)

	modules := svc.Modules()

	require.NoError(t, err)
	require.NotNil(t, modules)
	assert.Len(t, modules, 1)
	assert.Equal(t, "moduleA-title", modules[0].Title())
}

func TestListModules_ErrorFromNewRepo(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.ScaffoldRootFileName = config.RecommendedParentConfigName

	expectedErr := errors.Errorf("failed to clone repo")
	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		return nil, expectedErr
	}

	svc := catalog.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo).WithRepoURL("github.com/gruntwork-io/error-repo")
	l := logger.CreateLogger()
	err := svc.Load(t.Context(), l)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find modules in some repositories", "Error message mismatch: %v", err)
	assert.True(t, errors.Is(err, expectedErr) || strings.Contains(err.Error(), expectedErr.Error()), "Original error not wrapped correctly: %v", err)
}

func TestListModules_ErrorFromFindModules(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.ScaffoldRootFileName = config.RecommendedParentConfigName

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		if repoURL == "github.com/gruntwork-io/find-error-repo" {
			dummyRepoDir := filepath.Join(t.TempDir(), "find-error-repo-dir")
			os.MkdirAll(filepath.Join(dummyRepoDir, ".git"), 0755)
			os.WriteFile(filepath.Join(dummyRepoDir, ".git", "config"), []byte("[remote \"origin\"]\nurl = "+repoURL), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)

			moduleDirWithBadReadme := filepath.Join(dummyRepoDir, "problem_module")
			os.MkdirAll(moduleDirWithBadReadme, 0755)
			os.WriteFile(filepath.Join(moduleDirWithBadReadme, "main.tf"), []byte("{}"), 0644)
			os.Mkdir(filepath.Join(moduleDirWithBadReadme, "README.md"), 0755)

			return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
		}

		return nil, fmt.Errorf("unexpected repoURL: %s", repoURL)
	}

	svc := catalog.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo).WithRepoURL("github.com/gruntwork-io/find-error-repo")
	l := logger.CreateLogger()
	err := svc.Load(t.Context(), l)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no modules found in any of the configured repositories")
}

func TestListModules_NoModulesFound(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.ScaffoldRootFileName = config.RecommendedParentConfigName

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		dummyRepoDir := filepath.Join(t.TempDir(), "empty-repo-dir")
		os.MkdirAll(filepath.Join(dummyRepoDir, ".git"), 0755)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "config"), []byte("[remote \"origin\"]\nurl = "+repoURL), 0644)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)

		return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
	}

	svc := catalog.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo).WithRepoURL("github.com/gruntwork-io/empty-repo")
	l := logger.CreateLogger()
	err := svc.Load(t.Context(), l)
	require.Error(t, err)

	modules := svc.Modules()

	assert.Contains(t, err.Error(), "no modules found in any of the configured repositories")
	assert.Empty(t, modules, "Should return empty modules slice on 'no modules found' error")
}
