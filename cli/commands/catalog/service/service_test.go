package service_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/service"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModules_HappyPath(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.Logger = log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel))

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		// Use t.TempDir() for the dummyRepoDir to ensure cleanup and parallelism safety.
		dummyRepoDir := filepath.Join(t.TempDir(), strings.ReplaceAll(repoURL, "mock://", ""))
		os.MkdirAll(filepath.Join(dummyRepoDir, ".git"), 0755)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "config"), []byte("[remote \"origin\"]\nurl = "+repoURL), 0644)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)

		if repoURL == "mock://repo1" {
			readme1Path := filepath.Join(dummyRepoDir, "README.md")
			os.WriteFile(readme1Path, []byte("# module1-title\nThis is module1."), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "module1.tf"), []byte{}, 0644)
			return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
		}
		if repoURL == "mock://repo2" {
			readme2Path := filepath.Join(dummyRepoDir, "README.md")
			os.WriteFile(readme2Path, []byte("# module2-title\nThis is module2."), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "module2.tf"), []byte{}, 0644)
			return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
		}
		return nil, fmt.Errorf("unexpected repoURL in mock newRepoFunc: %s", repoURL)
	}

	tmpDir := t.TempDir()
	catalogFile := filepath.Join(tmpDir, "catalog.hcl")
	err := os.WriteFile(catalogFile, []byte(`urls = ["mock://repo1", "mock://repo2"]`), 0600)
	require.NoError(t, err)
	opts.TerragruntConfigPath = filepath.Join(tmpDir, "terragrunt.hcl")

	svc := service.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo)
	modulesResult, err := svc.ListModules(context.Background())

	require.NoError(t, err)
	require.NotNil(t, modulesResult)
	assert.Len(t, modulesResult, 2)
	assert.Equal(t, "module1-title", modulesResult[0].Title())
	assert.Equal(t, "module2-title", modulesResult[1].Title())
}

func TestListModules_NoRepositoriesConfigured(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.Logger = log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel))

	tmpDir := t.TempDir()
	opts.TerragruntConfigPath = filepath.Join(tmpDir, "nonexistent-terragrunt.hcl")

	// No customNewRepoFunc needed as it should error before trying to create a repo.
	svc := service.NewCatalogService(opts)
	_, err := svc.ListModules(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no catalog URLs provided")
}

func TestListModules_SingleRepoFromFlag(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.Logger = log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel))

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		if repoURL == "mock://only-repo" {
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

	svc := service.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo).WithRepoURL("mock://only-repo")
	modulesResult, err := svc.ListModules(context.Background())

	require.NoError(t, err)
	require.NotNil(t, modulesResult)
	assert.Len(t, modulesResult, 1)
	assert.Equal(t, "moduleA-title", modulesResult[0].Title())
}

func TestListModules_ErrorFromNewRepo(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.Logger = log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel))

	expectedErr := errors.Errorf("failed to clone repo")
	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		return nil, expectedErr
	}

	svc := service.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo).WithRepoURL("mock://error-repo")
	_, err := svc.ListModules(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find modules in some repositories", "Error message mismatch: %v", err)
	assert.True(t, errors.Is(err, expectedErr) || strings.Contains(err.Error(), expectedErr.Error()), "Original error not wrapped correctly: %v", err)
}

func TestListModules_ErrorFromFindModules(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.Logger = log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel))

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		if repoURL == "mock://find-error-repo" {
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

	svc := service.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo).WithRepoURL("mock://find-error-repo")
	_, err := svc.ListModules(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find modules in some repositories", "Error message mismatch: %v", err)
	assert.True(t, strings.Contains(err.Error(), "problem_module") && (strings.Contains(err.Error(), "README.md") || strings.Contains(err.Error(), "read")), "Underlying error not indicative of FindDoc failure: %v", err)
}

func TestListModules_NoModulesFound(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.Logger = log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel))

	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		dummyRepoDir := filepath.Join(t.TempDir(), "empty-repo-dir")
		os.MkdirAll(filepath.Join(dummyRepoDir, ".git"), 0755)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "config"), []byte("[remote \"origin\"]\nurl = "+repoURL), 0644)
		os.WriteFile(filepath.Join(dummyRepoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)
		return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
	}

	svc := service.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo).WithRepoURL("mock://empty-repo")
	returnedModules, err := svc.ListModules(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no modules found in any of the configured repositories")
	assert.Empty(t, returnedModules, "Should return empty modules slice on 'no modules found' error")
}

func TestListModules_EmptyRepoURLInListSkipped(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.Logger = log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel))

	var calledRepoURLs []string
	mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
		calledRepoURLs = append(calledRepoURLs, repoURL)
		if repoURL == "mock://valid-repo" {
			dummyRepoDir := filepath.Join(t.TempDir(), "valid-repo-dir")
			os.MkdirAll(filepath.Join(dummyRepoDir, ".git"), 0755)
			os.WriteFile(filepath.Join(dummyRepoDir, ".git", "config"), []byte("[remote \"origin\"]\nurl = "+repoURL), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "README.md"), []byte("# moduleValid-title"), 0644)
			os.WriteFile(filepath.Join(dummyRepoDir, "moduleValid.tf"), []byte{}, 0644)
			return module.NewRepo(ctx, logger, dummyRepoDir, path, walkWithSymlinks, allowCAS)
		}
		// This case should not be hit if empty URLs are correctly skipped by the service before calling newRepo.
		// However, if an empty URL somehow reaches here, this mock will error.
		return nil, fmt.Errorf("newRepoFunc called with unexpected or empty URL: '%s'", repoURL)
	}

	tmpDir := t.TempDir()
	catalogFile := filepath.Join(tmpDir, "catalog.hcl")
	err := os.WriteFile(catalogFile, []byte(`urls = ["mock://valid-repo", ""]`), 0600)
	require.NoError(t, err)
	opts.TerragruntConfigPath = filepath.Join(tmpDir, "terragrunt.hcl")

	svc := service.NewCatalogService(opts).WithNewRepoFunc(mockNewRepo)
	modulesResult, err := svc.ListModules(context.Background())

	require.NoError(t, err)
	assert.Len(t, modulesResult, 1)
	assert.Equal(t, "moduleValid-title", modulesResult[0].Title())
	assert.Equal(t, []string{"mock://valid-repo"}, calledRepoURLs, "newRepoFunc should only be called for valid URLs")
}
