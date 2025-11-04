package module_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
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

			ctx := t.Context()

			repo, err := module.NewRepo(ctx, logger.CreateLogger(), tc.repoPath, "", false, false)
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

//nolint:paralleltest,tparallel // intentional:  // subtests share repoPath, running in parallel causes race
func TestFindModulesWithCustomPaths(t *testing.T) {
	t.Parallel()

	type moduleData struct {
		title       string
		description string
		url         string
		moduleDir   string
	}

	repoPath := "testdata/find_modules_custom_paths"

	testCases := []struct {
		name         string
		modulePaths  []string
		expectedData []moduleData
	}{
		{
			name:        "single custom path - infra-modules",
			modulePaths: []string{"infra-modules"},
			expectedData: []moduleData{
				{
					title:       "Security Group Module",
					description: "This Terraform Module creates and manages AWS Security Groups with configurable ingress and egress rules for controlling network access to resources.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/infra-modules/terraform-aws-security-group",
					moduleDir:   "infra-modules/terraform-aws-security-group",
				},
				{
					title:       "VPC Module",
					description: "This Terraform Module creates a production-ready AWS Virtual Private Cloud (VPC) with public and private subnets, NAT gateways, and Internet gateway for secure network isolation.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/infra-modules/terraform-aws-vpc",
					moduleDir:   "infra-modules/terraform-aws-vpc",
				},
			},
		},
		{
			name:        "single custom path - platform-modules",
			modulePaths: []string{"platform-modules"},
			expectedData: []moduleData{
				{
					title:       "EKS Platform Module",
					description: "This Terraform Module provisions a complete Amazon EKS (Elastic Kubernetes Service) platform with managed node groups, cluster add-ons, and IRSA configuration.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/platform-modules/terraform-aws-eks",
					moduleDir:   "platform-modules/terraform-aws-eks",
				},
				{
					title:       "Monitoring Platform Module",
					description: "This Terraform Module sets up a comprehensive monitoring and observability stack for AWS infrastructure using CloudWatch, SNS, and optional third-party integrations.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/platform-modules/terraform-aws-monitoring",
					moduleDir:   "platform-modules/terraform-aws-monitoring",
				},
			},
		},
		{
			name:        "multiple custom paths",
			modulePaths: []string{"infra-modules", "platform-modules"},
			expectedData: []moduleData{
				{
					title:       "Security Group Module",
					description: "This Terraform Module creates and manages AWS Security Groups with configurable ingress and egress rules for controlling network access to resources.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/infra-modules/terraform-aws-security-group",
					moduleDir:   "infra-modules/terraform-aws-security-group",
				},
				{
					title:       "VPC Module",
					description: "This Terraform Module creates a production-ready AWS Virtual Private Cloud (VPC) with public and private subnets, NAT gateways, and Internet gateway for secure network isolation.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/infra-modules/terraform-aws-vpc",
					moduleDir:   "infra-modules/terraform-aws-vpc",
				},
				{
					title:       "EKS Platform Module",
					description: "This Terraform Module provisions a complete Amazon EKS (Elastic Kubernetes Service) platform with managed node groups, cluster add-ons, and IRSA configuration.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/platform-modules/terraform-aws-eks",
					moduleDir:   "platform-modules/terraform-aws-eks",
				},
				{
					title:       "Monitoring Platform Module",
					description: "This Terraform Module sets up a comprehensive monitoring and observability stack for AWS infrastructure using CloudWatch, SNS, and optional third-party integrations.",
					url:         "https://github.com/gruntwork-io/terraform-aws-eks/tree/master/platform-modules/terraform-aws-monitoring",
					moduleDir:   "platform-modules/terraform-aws-monitoring",
				},
			},
		},
		{
			name:         "nonexistent path returns empty",
			modulePaths:  []string{"does-not-exist"},
			expectedData: []moduleData{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// NOTE: Not using t.Parallel() to avoid race conditions on gitdir rename

			// Unfortunately, we are unable to commit the `.git` directory.
			// We have to temporarily rename it while running the tests.
			os.Rename(filepath.Join(repoPath, "gitdir"), filepath.Join(repoPath, ".git"))
			defer os.Rename(filepath.Join(repoPath, ".git"), filepath.Join(repoPath, "gitdir"))

			ctx := t.Context()

			repo, err := module.NewRepo(ctx, logger.CreateLogger(), repoPath, "", false, false, module.WithModulePaths(tc.modulePaths))
			require.NoError(t, err)

			modules, err := repo.FindModules(ctx)
			require.NoError(t, err)

			var realData []moduleData
			for _, module := range modules {
				realData = append(realData, moduleData{
					title:       module.Title(),
					description: module.Description(),
					url:         module.URL(),
					moduleDir:   module.ModuleDir(),
				})
			}

			assert.ElementsMatch(t, tc.expectedData, realData)
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
