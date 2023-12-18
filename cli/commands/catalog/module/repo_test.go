package module

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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

			repo, err := NewRepo(ctx, testCase.repoPath, "")
			assert.NoError(t, err)

			modules, err := repo.FindModules(ctx)
			assert.Equal(t, testCase.expectedErr, err)

			var realData []moduleData

			for _, module := range modules {

				realData = append(realData, moduleData{
					title:       module.Title(),
					description: module.Description(),
					url:         module.URL(),
					moduleDir:   module.moduleDir,
				})
			}

			assert.Equal(t, testCase.expectedData, realData)
		})
	}

}
