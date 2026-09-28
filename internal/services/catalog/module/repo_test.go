package module_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestFindModules(t *testing.T) {
	t.Parallel()

	type moduleData struct {
		title       string
		description string
		url         string
		moduleDir   string
	}

	testCases := []struct {
		expectedErr  error
		repoPath     string
		expectedData []moduleData
	}{
		{
			repoPath: "testdata/find_modules",
			expectedData: []moduleData{
				{
					title:       "ALB Ingress Controller Module",
					description: "This Terraform Module installs and configures the AWS ALB Ingress Controller on an EKS cluster, so that you can configure an ALB using Ingress resources.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/modules/eks-alb-ingress-controller",
					moduleDir:   "modules/eks-alb-ingress-controller",
				},
				{
					title:       "ALB Ingress Controller IAM Policy Module",
					description: "This Terraform Module defines an IAM policy that defines the minimal set of permissions necessary for the AWS ALB Ingress Controller.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/modules/eks-alb-ingress-controller-iam-policy",
					moduleDir:   "modules/eks-alb-ingress-controller-iam-policy",
				},
				{
					title:       "EKS AWS Auth Merger",
					description: "This module contains a go CLI, docker container, and terraform module for deploying a Kubernetes controller for managing mappings between AWS IAM roles and users to RBAC groups in Kubernetes.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/modules/eks-aws-auth-merger",
					moduleDir:   "modules/eks-aws-auth-merger",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.repoPath, func(t *testing.T) {
			t.Parallel()

			// The fixture carries its git metadata as `gitdir`, because a real
			// `.git` directory cannot be committed. Copying first keeps that
			// rename out of the checked-out tree.
			repoDir := filepath.Join(t.TempDir(), "repo")
			require.NoError(t, os.CopyFS(repoDir, os.DirFS(tc.repoPath)))
			require.NoError(t, os.Rename(
				filepath.Join(repoDir, "gitdir"),
				filepath.Join(repoDir, ".git"),
			))

			ctx := t.Context()

			v := venvtest.NewOSWithEmptyEnv()
			v.HTTP = vhttp.NewNoNetworkClient()

			repo, err := module.NewRepo(
				ctx,
				logger.CreateLogger(),
				v,
				&module.RepoOpts{CloneURL: repoDir},
			)
			require.NoError(t, err)

			modules, err := repo.FindModules(ctx, logger.CreateLogger(), vfs.NewOSFS())
			assert.Equal(t, tc.expectedErr, err)

			realData := make([]moduleData, 0, len(modules))

			for _, module := range modules {
				realData = append(realData, moduleData{
					title:       module.Title(),
					description: module.Description(),
					url:         module.URL(),
					moduleDir:   module.ModuleDir(),
				})
			}

			assert.Equal(t, tc.expectedData, realData)
		})
	}
}

// TestNewRepoRemoteCloneRejectsNonOSFS pins the performClone contract: a
// remote clone writes through go-getter to the real OS, so an in-memory
// bundle must be rejected before any clone work starts.
func TestNewRepoRemoteCloneRejectsNonOSFS(t *testing.T) {
	t.Parallel()

	_, err := module.NewRepo(
		t.Context(),
		logger.CreateLogger(),
		venvtest.New(),
		&module.RepoOpts{
			CloneURL: "https://example.com/org/target.git",
			Path:     t.TempDir(),
		},
	)
	require.ErrorIs(t, err, module.ErrRemoteCloneFSNotOS)
}

// TestNewRepoCloneDirectoryName pins the directory name the catalog derives
// from a clone URL. The prepared directory carries the clone-complete marker,
// so [module.NewRepo] reads the repo in place instead of cloning. Any other
// derived name sends it down the clone path, which the in-memory filesystem
// refuses.
func TestNewRepoCloneDirectoryName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		cloneURL string
	}{
		{
			name:     "https",
			cloneURL: "https://github.com/acme/terraform-aws-modules.git",
		},
		{
			name:     "https without .git",
			cloneURL: "https://github.com/acme/terraform-aws-modules",
		},
		{
			name:     "ssh scp-style",
			cloneURL: "git@github.com:acme/terraform-aws-modules.git",
		},
		{
			name:     "forced git getter",
			cloneURL: "git::https://github.com/acme/terraform-aws-modules.git",
		},
		{
			name:     "ref query parameter",
			cloneURL: "https://github.com/acme/terraform-aws-modules.git?ref=v1.2.3",
		},
		{
			name:     "fragment",
			cloneURL: "https://github.com/acme/terraform-aws-modules.git#v1.2.3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New()
			cloneRoot := venvtest.Root("/catalog")
			clonePath := filepath.Join(cloneRoot, "terraform-aws-modules")

			writeFakeRepo(t, v.FS, clonePath)
			require.NoError(t, vfs.WriteFile(
				v.FS,
				filepath.Join(clonePath, module.CloneCompleteSentinel),
				nil,
				0o644,
			))

			repo, err := module.NewRepo(
				t.Context(),
				logger.CreateLogger(),
				v,
				&module.RepoOpts{
					CloneURL: tc.cloneURL,
					Path:     cloneRoot,
				},
			)
			require.NoError(t, err)
			assert.Equal(t, clonePath, repo.Path())
		})
	}
}

// TestNewRepoLocalSourceKeepsPath pins that the catalog reads a local source
// where it sits, whatever the directory is called.
func TestNewRepoLocalSourceKeepsPath(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	localPath := filepath.Join(venvtest.Root("/catalog"), "terraform-aws-modules.git")

	writeFakeRepo(t, v.FS, localPath)

	repo, err := module.NewRepo(
		t.Context(),
		logger.CreateLogger(),
		v,
		&module.RepoOpts{CloneURL: localPath, Path: venvtest.Root("/clone-root")},
	)
	require.NoError(t, err)
	assert.Equal(t, localPath, repo.Path())
}

// writeFakeRepo writes the git metadata [module.NewRepo] parses, so it reads
// the directory as a repo without running a clone.
func writeFakeRepo(t *testing.T, fsys vfs.FS, path string) {
	t.Helper()

	const config = `[remote "origin"]
	url = https://github.com/acme/terraform-aws-modules.git
`

	gitDir := filepath.Join(path, ".git")
	require.NoError(t, fsys.MkdirAll(gitDir, 0o755))
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(gitDir, "config"), []byte(config), 0o644),
	)
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644),
	)
}

func TestModuleURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr error
		repo        *module.Repo
		name        string
		moduleDir   string
		expectedURL string
	}{
		{
			name:        "github",
			repo:        newRepo(t, "https://github.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://github.com/acme/terraform-aws-modules/tree/main/.",
		},
		{
			name:        "github enterprise",
			repo:        newRepo(t, "https://github.acme.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://github.acme.com/acme/terraform-aws-modules/tree/main/.",
		},
		{
			name:        "gitlab",
			repo:        newRepo(t, "https://gitlab.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://gitlab.com/acme/terraform-aws-modules/-/tree/main/.",
		},
		{
			name:        "gitlab self-hosted",
			repo:        newRepo(t, "https://gitlab.acme.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://gitlab.acme.com/acme/terraform-aws-modules/-/tree/main/.",
		},
		{
			name:        "bitbucket",
			repo:        newRepo(t, "https://bitbucket.org/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://bitbucket.org/acme/terraform-aws-modules/browse/.?at=main",
		},
		{
			name:        "azuredev",
			repo:        newRepo(t, "https://dev.azure.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://dev.azure.com/_git/acme/terraform-aws-modules?path=.&version=GBmain",
		},
		{
			name:        "unsupported",
			repo:        newRepo(t, "https://fake.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			url := tc.repo.ModuleURL(tc.moduleDir)
			assert.Equal(t, tc.expectedURL, url)
		})
	}
}

func newRepo(t *testing.T, url string) *module.Repo {
	t.Helper()

	return &module.Repo{
		RemoteURL:  url,
		BranchName: "main",
	}
}
