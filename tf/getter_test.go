package tf_test

import (
	"context"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terratest/modules/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModuleRegistryURLBasePath(t *testing.T) {
	t.Parallel()

	basePath, err := tf.GetModuleRegistryURLBasePath(context.Background(), log.New(), "registry.terraform.io")
	require.NoError(t, err)
	assert.Equal(t, "/v1/modules/", basePath)
}

func TestGetTerraformHeader(t *testing.T) {
	t.Parallel()

	testModuleURL := url.URL{
		Scheme: "https",
		Host:   "registry.terraform.io",
		Path:   "/v1/modules/terraform-aws-modules/vpc/aws/3.3.0/download",
	}
	terraformGetHeader, err := tf.GetTerraformGetHeader(context.Background(), log.New(), testModuleURL)
	require.NoError(t, err)
	assert.Contains(t, terraformGetHeader, "github.com/terraform-aws-modules/terraform-aws-vpc")
}

func TestGetDownloadURLFromHeader(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name           string
		moduleURL      url.URL
		terraformGet   string
		expectedResult string
	}{
		{
			name: "BaseWithRoot",
			moduleURL: url.URL{
				Scheme: "https",
				Host:   "registry.terraform.io",
			},
			terraformGet:   "/terraform-aws-modules/terraform-aws-vpc",
			expectedResult: "https://registry.terraform.io/terraform-aws-modules/terraform-aws-vpc",
		},
		{
			name:           "PrefixedURL",
			moduleURL:      url.URL{},
			terraformGet:   "github.com/terraform-aws-modules/terraform-aws-vpc",
			expectedResult: "github.com/terraform-aws-modules/terraform-aws-vpc",
		},
		{
			name: "PathWithRoot",
			moduleURL: url.URL{
				Scheme: "https",
				Host:   "registry.terraform.io",
				Path:   "modules/foo/bar",
			},
			terraformGet:   "/terraform-aws-modules/terraform-aws-vpc",
			expectedResult: "https://registry.terraform.io/terraform-aws-modules/terraform-aws-vpc",
		},
		{
			name: "PathWithRelativeRoot",
			moduleURL: url.URL{
				Scheme: "https",
				Host:   "registry.terraform.io",
				Path:   "modules/foo/bar",
			},
			terraformGet:   "./terraform-aws-modules/terraform-aws-vpc",
			expectedResult: "https://registry.terraform.io/modules/foo/terraform-aws-modules/terraform-aws-vpc",
		},
		{
			name: "PathWithRelativeParent",
			moduleURL: url.URL{
				Scheme: "https",
				Host:   "registry.terraform.io",
				Path:   "modules/foo/bar",
			},
			terraformGet:   "../terraform-aws-modules/terraform-aws-vpc",
			expectedResult: "https://registry.terraform.io/modules/terraform-aws-modules/terraform-aws-vpc",
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			downloadURL, err := tf.GetDownloadURLFromHeader(tt.moduleURL, tt.terraformGet)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedResult, downloadURL)
		})
	}
}

func TestTFRGetterRootDir(t *testing.T) {
	t.Parallel()

	testModuleURL, err := url.Parse("tfr://registry.terraform.io/terraform-aws-modules/vpc/aws?version=3.3.0")
	require.NoError(t, err)

	dstPath := t.TempDir()

	// The dest path must not exist for go getter to work
	moduleDestPath := filepath.Join(dstPath, "terraform-aws-vpc")
	assert.False(t, files.FileExists(filepath.Join(moduleDestPath, "main.tf")))

	tfrGetter := new(tf.RegistryGetter)
	tfrGetter.TerragruntOptions, err = options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	require.NoError(t, tfrGetter.Get(moduleDestPath, testModuleURL))
	assert.True(t, files.FileExists(filepath.Join(moduleDestPath, "main.tf")))
}

func TestTFRGetterSubModule(t *testing.T) {
	t.Parallel()

	testModuleURL, err := url.Parse("tfr://registry.terraform.io/terraform-aws-modules/vpc/aws//modules/vpc-endpoints?version=3.3.0")
	require.NoError(t, err)

	dstPath := t.TempDir()

	// The dest path must not exist for go getter to work
	moduleDestPath := filepath.Join(dstPath, "terraform-aws-vpc")
	assert.False(t, files.FileExists(filepath.Join(moduleDestPath, "main.tf")))

	tfrGetter := new(tf.RegistryGetter)
	tfrGetter.TerragruntOptions, _ = options.NewTerragruntOptionsForTest("")
	require.NoError(t, tfrGetter.Get(moduleDestPath, testModuleURL))
	assert.True(t, files.FileExists(filepath.Join(moduleDestPath, "main.tf")))
}

func TestBuildRequestUrlFullPath(t *testing.T) {
	t.Parallel()
	requestURL, err := tf.BuildRequestURL("gruntwork.io", "https://gruntwork.io/registry/modules/v1/", "/tfr-project/terraform-aws-tfr", "6.6.6")
	require.NoError(t, err)
	assert.Equal(t, "https://gruntwork.io/registry/modules/v1/tfr-project/terraform-aws-tfr/6.6.6/download", requestURL.String())
}

func TestBuildRequestUrlRelativePath(t *testing.T) {
	t.Parallel()
	requestURL, err := tf.BuildRequestURL("gruntwork.io", "/registry/modules/v1", "/tfr-project/terraform-aws-tfr", "6.6.6")
	require.NoError(t, err)
	assert.Equal(t, "https://gruntwork.io/registry/modules/v1/tfr-project/terraform-aws-tfr/6.6.6/download", requestURL.String())
}

// Combine all the tests cases below into one test function, including the last one where a non existant module is requested
func TestGetTargetVersion(t *testing.T) {
	t.Parallel()
	registryDomain := "registry.terraform.io"
	moduleRegistryBasePath := "/v1/modules/"
	tc := []struct {
		name           string
		modulePath     string
		targetVersion  string
		expectedResult string
	}{
		{
			name:           "FixedVersion",
			modulePath:     "/terraform-aws-modules/iam/aws",
			targetVersion:  "3.3.0",
			expectedResult: "3.3.0",
		},
		{
			name:           "PessimisticPatchVersion",
			modulePath:     "/terraform-aws-modules/iam/aws",
			targetVersion:  "~> 0.0.1",
			expectedResult: "0.0.7",
		},
		{
			name:           "PessimisticMinorVersion",
			modulePath:     "/terraform-aws-modules/iam/aws",
			targetVersion:  "~> 3.3",
			expectedResult: "3.16.0",
		},
		{
			name:           "ComplexConstraint",
			modulePath:     "/terraform-aws-modules/iam/aws",
			targetVersion:  ">= 3.3.0, <5.0.0,!= 4.24.1,!=4.24.0",
			expectedResult: "4.23.0",
		},
		{
			name:           "InvalidConstraint",
			modulePath:     "/terraform-aws-modules/iam/aws",
			targetVersion:  ">= 3.3.0 and >4.24.1",
			expectedResult: "",
		},
		{
			name:           "UnsatisfiableConstraint",
			modulePath:     "/terraform-aws-modules/iam/aws",
			targetVersion:  ">= 3.3.0, <3.2.0",
			expectedResult: "",
		},
		{
			name:           "InvalidVersion",
			modulePath:     "/terraform-aws-modules/iam/aws",
			targetVersion:  "~> a.b.c",
			expectedResult: "",
		},
		{
			name:           "NotExistingModule",
			modulePath:     "/terraform-not-existing-modules/not-existing-module",
			targetVersion:  "3.3.0",
			expectedResult: "",
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			targetVersion, err := tf.GetTargetVersion(context.Background(), log.New(), registryDomain, moduleRegistryBasePath, tt.modulePath, tt.targetVersion)
			if tt.expectedResult == "" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, targetVersion)
			}
		})
	}
}
