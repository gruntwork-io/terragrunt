//go:build mocks

package getproviders_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/tf/getproviders"
	"github.com/gruntwork-io/terragrunt/tf/getproviders/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func mockProviderUpdateLock(t *testing.T, ctrl *gomock.Controller, address, version string) getproviders.Provider {
	t.Helper()

	packageDir := t.TempDir()
	file, err := os.Create(filepath.Join(packageDir, "terraform-provider-v"+version))
	require.NoError(t, err)
	_, err = fmt.Fprintf(file, "mock-provider-content-%s-%s", address, version)
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	var document string

	for i := 0; i < 2; i++ {
		packageName := fmt.Sprintf("%s-%s-%d", address, version, i)
		hasher := sha256.New()
		_, err := hasher.Write([]byte(packageName))
		require.NoError(t, err)
		sha := hex.EncodeToString(hasher.Sum(nil))
		document += fmt.Sprintf("%s %s\n", sha, packageName)
	}

	provider := mocks.NewMockProvider(ctrl)
	provider.EXPECT().Address().Return(address).AnyTimes()
	provider.EXPECT().Version().Return(version).AnyTimes()
	provider.EXPECT().Constraints().Return("").AnyTimes()
	provider.EXPECT().PackageDir().Return(packageDir).AnyTimes()
	provider.EXPECT().Logger().Return(logger.CreateLogger()).AnyTimes()
	provider.EXPECT().DocumentSHA256Sums(gomock.Any()).Return([]byte(document), nil).AnyTimes()

	return provider
}

func TestMockUpdateLockfile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		initialLockfile  string
		expectedLockfile string
		providers        []getproviders.Provider
	}{
		{
			providers:       []getproviders.Provider{},
			initialLockfile: ``,
			expectedLockfile: `
provider "registry.terraform.io/hashicorp/aws" {
  version     = "5.37.0"
  constraints = "5.37.0"
  hashes = [
    "h1:SHOEBOHEif46z7Bb86YZ5evCtAeK5A4gtHdT8RU5OhA=",
    "zh:7c810fb11d8b3ded0cb554a27c27a9d002cc644a7a57c29cae01eeea890f0398",
    "zh:a3366f6b57b0f4b8bf8a5fecf42a834652709a97dd6db1b499c4ab186e33a41f",
  ]
}
`,
		},
		{
			providers: []getproviders.Provider{},
			initialLockfile: `
provider "registry.terraform.io/hashicorp/aws" {
  version     = "5.37.0"
  constraints = "5.37.0"
  hashes = [
    "h1:SHOEBOHEif46z7Bb86YZ5evCtAeK5A4gtHdT8RU5OhA=",
    "zh:7c810fb11d8b3ded0cb554a27c27a9d002cc644a7a57c29cae01eeea890f0398",
    "zh:a3366f6b57b0f4b8bf8a5fecf42a834652709a97dd6db1b499c4ab186e33a41f",
  ]
}

provider "registry.terraform.io/hashicorp/azurerm" {
  version     = "3.101.0"
  constraints = "3.101.0"
  hashes = [
    "h1:Jrkhx+qKaf63sIV/WvE8sPR53QuC16pvTrBjxFVMPYM=",
    "zh:38b02bce5cbe83f938a71716bbf9e8b07fed8b2c6b83c19b5e708eda7dee0f1d",
    "zh:3ed094366ab35c4fcd632471a7e45a84ca6c72b00477cdf1276e541a0171b369",
  ]
}
`,
			expectedLockfile: `
provider "registry.terraform.io/hashicorp/aws" {
  version     = "5.36.0"
  constraints = "5.36.0"
  hashes = [
    "h1:RpTjHdEAYqidB9hFPs68dIhkeIE1c/ZH9fEBdddf0Ik=",
    "zh:8721239b83a06212fb2f474d2acddfa2659a224ef66c77e28e1efe2290a30fa7",
    "zh:ed83a9620eab99e091b9f786f20f03fddb50cba030839fe0529bd518bfd67f8d",
  ]
}

provider "registry.terraform.io/hashicorp/azurerm" {
  version     = "3.101.0"
  constraints = "3.101.0"
  hashes = [
    "h1:Jrkhx+qKaf63sIV/WvE8sPR53QuC16pvTrBjxFVMPYM=",
    "zh:38b02bce5cbe83f938a71716bbf9e8b07fed8b2c6b83c19b5e708eda7dee0f1d",
    "zh:3ed094366ab35c4fcd632471a7e45a84ca6c72b00477cdf1276e541a0171b369",
  ]
}

provider "registry.terraform.io/hashicorp/template" {
  version     = "2.2.0"
  constraints = "2.2.0"
  hashes = [
    "h1:kvJsWhTmFya0WW8jAfY40fDtYhWQ6mOwPQC2ncDNjZs=",
    "zh:02d170f0a0f453155686baf35c10b5a7a230ef20ca49f6e26de1c2691ac70a59",
    "zh:d88ec10849d5a1d9d1db458847bbc62049f0282a2139e5176d645b75a0346992",
  ]
}
`,
		},
		{
			providers: []getproviders.Provider{},
			initialLockfile: `
provider "registry.terraform.io/hashicorp/aws" {
  version     = "5.36.0"
  constraints = ">= 5.36.0"
  hashes = [
    "h1:SHOEBOHEif46z7Bb86YZ5evCtAeK5A4gtHdT8RU5OhA=",
    "zh:7c810fb11d8b3ded0cb554a27c27a9d002cc644a7a57c29cae01eeea890f0398",
    "zh:a3366f6b57b0f4b8bf8a5fecf42a834652709a97dd6db1b499c4ab186e33a41f",
  ]
}

provider "registry.terraform.io/hashicorp/template" {
  version     = "2.1.0"
  constraints = "<= 2.1.0"
  hashes = [
    "h1:vxE/PD8SWl6Lmh5zRvIW1Y559xfUyuV2T/VeQLXi7f0=",
    "zh:6fc271665ac28c3fee773b9dc2b8066280ba35b7e9a14a6148194a240c43f42a",
    "zh:c19f719c9f7ce6d7449fe9c020100faed0705303c7f95beeef81dfd1e4a2004b",
  ]
}
`,
			expectedLockfile: `
provider "registry.terraform.io/hashicorp/aws" {
  version     = "5.37.0"
  constraints = ">= 5.36.0"
  hashes = [
    "h1:SHOEBOHEif46z7Bb86YZ5evCtAeK5A4gtHdT8RU5OhA=",
    "zh:7c810fb11d8b3ded0cb554a27c27a9d002cc644a7a57c29cae01eeea890f0398",
    "zh:a3366f6b57b0f4b8bf8a5fecf42a834652709a97dd6db1b499c4ab186e33a41f",
  ]
}

provider "registry.terraform.io/hashicorp/template" {
  version     = "2.2.0"
  constraints = "2.2.0"
  hashes = [
    "h1:kvJsWhTmFya0WW8jAfY40fDtYhWQ6mOwPQC2ncDNjZs=",
    "zh:02d170f0a0f453155686baf35c10b5a7a230ef20ca49f6e26de1c2691ac70a59",
    "zh:d88ec10849d5a1d9d1db458847bbc62049f0282a2139e5176d645b75a0346992",
  ]
}
`,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			switch i {
			case 0:
				tc.providers = []getproviders.Provider{
					mockProviderUpdateLock(t, ctrl, "registry.terraform.io/hashicorp/aws", "5.37.0"),
				}
			case 1:
				tc.providers = []getproviders.Provider{
					mockProviderUpdateLock(t, ctrl, "registry.terraform.io/hashicorp/aws", "5.36.0"),
					mockProviderUpdateLock(t, ctrl, "registry.terraform.io/hashicorp/template", "2.2.0"),
				}
			case 2:
				tc.providers = []getproviders.Provider{
					mockProviderUpdateLock(t, ctrl, "registry.terraform.io/hashicorp/aws", "5.37.0"),
					mockProviderUpdateLock(t, ctrl, "registry.terraform.io/hashicorp/template", "2.2.0"),
				}
			}

			workingDir := t.TempDir()
			lockfilePath := filepath.Join(workingDir, ".terraform.lock.hcl")

			if tc.initialLockfile != "" {
				file, err := os.Create(lockfilePath)
				require.NoError(t, err)
				_, err = file.WriteString(tc.initialLockfile)
				require.NoError(t, err)
				err = file.Close()
				require.NoError(t, err)
			}

			err := getproviders.UpdateLockfile(t.Context(), workingDir, tc.providers)
			require.NoError(t, err)

			actualLockfile, err := os.ReadFile(lockfilePath)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedLockfile, string(actualLockfile))
		})
	}
}
