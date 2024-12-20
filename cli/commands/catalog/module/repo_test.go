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
		repoPath     string
		expectedData []moduleData
		expectedErr  error
	}{
		{
			"testdata/find_modules",
			[]moduleData{
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
			nil,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.repoPath, func(t *testing.T) {
			t.Parallel()
			// Unfortunately, we are unable to commit the `.git` directory. We have to temporarily rename it while running the tests.
			os.Rename(filepath.Join(testCase.repoPath, "gitdir"), filepath.Join(testCase.repoPath, ".git"))
			defer os.Rename(filepath.Join(testCase.repoPath, ".git"), filepath.Join(testCase.repoPath, "gitdir"))

			ctx := context.Background()

			repo, err := module.NewRepo(ctx, log.New(), testCase.repoPath, "", false)
			require.NoError(t, err)

			modules, err := repo.FindModules(ctx)
			assert.Equal(t, testCase.expectedErr, err)

			var realData []moduleData

			for _, module := range modules {

				realData = append(realData, moduleData{
					title:       module.Title(),
					description: module.Description(),
					url:         module.URL(),
					moduleDir:   module.ModuleDir(),
				})
			}

			assert.Equal(t, testCase.expectedData, realData)
		})
	}

}

func TestModuleURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		repo        *module.Repo
		moduleDir   string
		expectedURL string
		expectedErr error
	}{
		{
			"github",
			newRepo(t, "https://github.com/acme/terraform-aws-modules"),
			".",
			"https://github.com/acme/terraform-aws-modules/tree/main/.",
			nil,
		},
		{
			"github enterprise",
			newRepo(t, "https://github.acme.com/acme/terraform-aws-modules"),
			".",
			"https://github.acme.com/acme/terraform-aws-modules/tree/main/.",
			nil,
		},
		{
			"gitlab",
			newRepo(t, "https://gitlab.com/acme/terraform-aws-modules"),
			".",
			"https://gitlab.com/acme/terraform-aws-modules/-/tree/main/.",
			nil,
		},
		{
			"bitbucket",
			newRepo(t, "https://bitbucket.org/acme/terraform-aws-modules"),
			".",
			"https://bitbucket.org/acme/terraform-aws-modules/browse/.?at=main",
			nil,
		},
		{
			"azuredev",
			newRepo(t, "https://dev.azure.com/acme/terraform-aws-modules"),
			".",
			"https://dev.azure.com/_git/acme/terraform-aws-modules?path=.&version=GBmain",
			nil,
		},
		{
			"unsupported",
			newRepo(t, "https://fake.com/acme/terraform-aws-modules"),
			".",
			"",
			errors.Errorf("hosting: %q is not supported yet", "fake.com"),
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			url, err := testCase.repo.ModuleURL(testCase.moduleDir)
			assert.Equal(t, testCase.expectedURL, url)
			if testCase.expectedErr != nil {
				assert.EqualError(t, err, testCase.expectedErr.Error())
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
