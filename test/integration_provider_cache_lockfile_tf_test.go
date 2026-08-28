//go:build tf

package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

// TestTFTerragruntFullLockfile asserts that a single `terragrunt init` (no
// `providers lock -platform=...` and no preexisting lock file) populates
// `.terraform.lock.hcl` with `h1:` hashes for multiple platforms, both with
// and without the Terragrunt provider cache server enabled.
//
// The OpenTofu provider registry returns pre-computed `h1:` hashes for every
// supported platform via the `packages` field on its per-platform download
// endpoint. With the cache server enabled, the Terragrunt-side lockfile writer
// (internal/tf/getproviders/lock.go) consumes that field via
// ProviderCache.RegistryHashes(), so this subtest passes on any OpenTofu
// version. Without the cache server, OpenTofu itself must read the field, and
// only the 1.12 binary onward does so, hence the version gate below.
func TestTFTerragruntFullLockfile(t *testing.T) {
	t.Parallel()

	if !helpers.IsOpenTofu112OrHigher(t) {
		t.Skip("requires OpenTofu 1.12 or higher")
		return
	}

	testCases := []struct {
		name           string
		providerSource string
		minPlatforms   int
		runWithCache   bool
	}{
		{
			name:           "without provider cache",
			providerSource: "registry.opentofu.org/hashicorp/null",
			minPlatforms:   2,
			runWithCache:   false,
		},
		{
			name:           "with provider cache",
			providerSource: "registry.opentofu.org/hashicorp/null",
			minPlatforms:   2,
			runWithCache:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureProviderCacheFullLockfile)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureProviderCacheFullLockfile)
			rootPath := filepath.Join(tmpEnvPath, testFixtureProviderCacheFullLockfile)

			cmd := "terragrunt init --non-interactive --working-dir " + rootPath

			if tc.runWithCache {
				providerCacheDir := helpers.TmpDirWOSymlinks(t)
				cmd = fmt.Sprintf(
					"terragrunt init --provider-cache --provider-cache-dir %s --non-interactive --working-dir %s",
					providerCacheDir,
					rootPath,
				)
			} else {
				// Without the provider cache server, OpenTofu drives the install. If
				// `~/.terraform.d/plugins` exists, OpenTofu treats it as an implicit
				// filesystem mirror, skips the registry trust chain, and writes only
				// a single-platform `h1:` hash, which defeats what this test asserts.
				homeDir, err := os.UserHomeDir()
				require.NoError(t, err)

				userPluginDir := filepath.Join(homeDir, ".terraform.d", "plugins")
				require.NoFileExists(
					t,
					userPluginDir,
					"this subtest requires %s to not exist so OpenTofu uses the direct registry path; remove or rename it before running",
					userPluginDir,
				)
			}

			helpers.RunTerragrunt(t, cmd)

			lockfilePath := filepath.Join(rootPath, ".terraform.lock.hcl")
			require.FileExists(
				t,
				lockfilePath,
				"expected lock file to exist at %s",
				lockfilePath,
			)

			lockfileContent, err := os.ReadFile(lockfilePath)
			require.NoError(t, err)

			lockfile, diags := hclwrite.ParseConfig(
				lockfileContent,
				lockfilePath,
				hcl.Pos{Line: 1, Column: 1},
			)
			require.False(t, diags.HasErrors(), "diagnostics: %s", diags.Error())
			require.NotNil(t, lockfile)

			providerBlock := lockfile.Body().
				FirstMatchingBlock("provider", []string{tc.providerSource})
			require.NotNil(
				t,
				providerBlock,
				"lock file is missing block for %s; contents:\n%s",
				tc.providerSource,
				string(lockfileContent),
			)

			hashesAttr := providerBlock.Body().GetAttribute("hashes")
			require.NotNil(t, hashesAttr, "provider block has no hashes attribute")

			hashesText := string(hashesAttr.Expr().BuildTokens(nil).Bytes())
			h1Count := strings.Count(hashesText, `"h1:`)

			assert.GreaterOrEqualf(t, h1Count, tc.minPlatforms,
				"expected at least %d h1 hashes (one per platform) but found %d in:\n%s",
				tc.minPlatforms, h1Count, hashesText,
			)
		})
	}
}
