package module_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.repoPath, func(t *testing.T) {
			t.Parallel()
			// Unfortunately, we are unable to commit the `.git` directory. We have to temporarily rename it while running the tests.
			os.Rename(filepath.Join(tc.repoPath, "gitdir"), filepath.Join(tc.repoPath, ".git"))
			defer os.Rename(filepath.Join(tc.repoPath, ".git"), filepath.Join(tc.repoPath, "gitdir"))

			ctx := context.Background()

			repo, err := module.NewRepo(ctx, log.New(), tc.repoPath, "", false, false)
			require.NoError(t, err)

			modules, err := repo.FindModules(ctx)
			assert.Equal(t, tc.expectedErr, err)

			var realData []moduleData

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
			expectedErr: nil,
		},
		{
			name:        "github enterprise",
			repo:        newRepo(t, "https://github.acme.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://github.acme.com/acme/terraform-aws-modules/tree/main/.",
			expectedErr: nil,
		},
		{
			name:        "gitlab",
			repo:        newRepo(t, "https://gitlab.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://gitlab.com/acme/terraform-aws-modules/-/tree/main/.",
			expectedErr: nil,
		},
		{
			name:        "gitlab self-hosted",
			repo:        newRepo(t, "https://gitlab.acme.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://gitlab.acme.com/acme/terraform-aws-modules/-/tree/main/.",
			expectedErr: nil,
		},
		{
			name:        "bitbucket",
			repo:        newRepo(t, "https://bitbucket.org/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://bitbucket.org/acme/terraform-aws-modules/browse/.?at=main",
			expectedErr: nil,
		},
		{
			name:        "azuredev",
			repo:        newRepo(t, "https://dev.azure.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "https://dev.azure.com/_git/acme/terraform-aws-modules?path=.&version=GBmain",
			expectedErr: nil,
		},
		{
			name:        "unsupported",
			repo:        newRepo(t, "https://fake.com/acme/terraform-aws-modules"),
			moduleDir:   ".",
			expectedURL: "",
			expectedErr: errors.Errorf("hosting: %q is not supported yet", "fake.com"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			url, err := tc.repo.ModuleURL(tc.moduleDir)
			assert.Equal(t, tc.expectedURL, url)
			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
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
