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

func TestGetDownloadURLFromRegistry(t *testing.T) {
	t.Parallel()

	testModuleURL := url.URL{
		Scheme: "https",
		Host:   "registry.terraform.io",
		Path:   "/v1/modules/terraform-aws-modules/vpc/aws/3.3.0/download",
	}
	downloadURL, err := getDownloadURLFromRegistry(context.Background(), testModuleURL)
	require.NoError(t, err)
	assert.Contains(t, downloadURL, "github.com/terraform-aws-modules/terraform-aws-vpc")
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
