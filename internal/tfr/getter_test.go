package tfr

import (
	"context"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terratest/modules/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModuleRegistryURLBasePath(t *testing.T) {
	t.Parallel()

	basePath, err := getModuleRegistryURLBasePath(context.Background(), "registry.terraform.io")
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
	terraformGetHeader, err := getTerraformGetHeader(context.Background(), testModuleURL)
	require.NoError(t, err)
	assert.Contains(t, terraformGetHeader, "github.com/terraform-aws-modules/terraform-aws-vpc")
}

func TestGetDownloadURLFromHeader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
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

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			downloadURL, err := getDownloadURLFromHeader(testCase.moduleURL, testCase.terraformGet)
			require.NoError(t, err)
			assert.Equal(t, testCase.expectedResult, downloadURL)
		})
	}
}

func TestTFRGetterRootDir(t *testing.T) {
	t.Parallel()

	testModuleURL, err := url.Parse("tfr://registry.terraform.io/terraform-aws-modules/vpc/aws?version=3.3.0")
	require.NoError(t, err)

	dstPath, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(dstPath)

	// The dest path must not exist for go getter to work
	moduleDestPath := filepath.Join(dstPath, "terraform-aws-vpc")
	assert.False(t, files.FileExists(filepath.Join(moduleDestPath, "main.tf")))

	tfrGetter := new(TerraformRegistryGetter)
	require.NoError(t, tfrGetter.Get(moduleDestPath, testModuleURL))
	assert.True(t, files.FileExists(filepath.Join(moduleDestPath, "main.tf")))
}

func TestTFRGetterSubModule(t *testing.T) {
	t.Parallel()

	testModuleURL, err := url.Parse("tfr://registry.terraform.io/terraform-aws-modules/vpc/aws//modules/vpc-endpoints?version=3.3.0")
	require.NoError(t, err)

	dstPath, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(dstPath)

	// The dest path must not exist for go getter to work
	moduleDestPath := filepath.Join(dstPath, "terraform-aws-vpc")
	assert.False(t, files.FileExists(filepath.Join(moduleDestPath, "main.tf")))

	tfrGetter := new(TerraformRegistryGetter)
	require.NoError(t, tfrGetter.Get(moduleDestPath, testModuleURL))
	assert.True(t, files.FileExists(filepath.Join(moduleDestPath, "main.tf")))
}

func TestBuildRequestUrlFullPath(t *testing.T) {
	t.Parallel()
	requestUrl, err := buildRequestUrl("gruntwork.io", "https://gruntwork.io/registry/modules/v1/", "/tfr-project/terraform-aws-tfr", "6.6.6")
	assert.Nil(t, err)
	assert.Equal(t, "https://gruntwork.io/registry/modules/v1/tfr-project/terraform-aws-tfr/6.6.6/download", requestUrl.String())
}

func TestBuildRequestUrlRelativePath(t *testing.T) {
	t.Parallel()
	requestUrl, err := buildRequestUrl("gruntwork.io", "/registry/modules/v1", "/tfr-project/terraform-aws-tfr", "6.6.6")
	assert.Nil(t, err)
	assert.Equal(t, "https://gruntwork.io/registry/modules/v1/tfr-project/terraform-aws-tfr/6.6.6/download", requestUrl.String())

}
