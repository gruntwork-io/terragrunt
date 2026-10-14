package getproviders_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/tf/getproviders"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// FuzzParseProviderConstraints pins that every constraint tofu accepts is
// recorded the way tofu writes it to a lock file.
func FuzzParseProviderConstraints(f *testing.F) {
	for _, seed := range []string{
		"~> 3.0",
		"~> 3",
		"~>3.2.0",
		">= 3.0, < 4.0",
		"< 4.0, >= 3.0",
		"!= 3.2.0, < 4, ~> 3.0",
		"= 3.2.3, 3.2.3",
		"~> 3.2, ~> 3.2.0",
		"> 3.1, >= 3.1, <= 3.1, != 3.1",
		">= 3.0.0-beta1",
		"~> 3.1-rc.1",
		">= 3.0.0+build.5",
		"<= 3.0.0-alpha.2+10",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, constraint string) {
		want, err := semver.LockFileConstraint(constraint)
		if err != nil {
			return
		}

		testDir := venvtest.Root("/module")
		fsys := vfs.NewMemMapFS()

		terraformContent := `terraform {
  required_providers {
    test = {
      source  = "example/test"
      version = ` + string(hclwrite.TokensForValue(cty.StringVal(constraint)).Bytes()) + `
    }
  }
}
`

		require.NoError(t, fsys.MkdirAll(testDir, 0o755))
		require.NoError(
			t,
			vfs.WriteFile(fsys, filepath.Join(testDir, "main.tf"), []byte(terraformContent), 0o644),
		)

		parsed, err := getproviders.ParseProviderConstraints(
			fsys,
			map[string]string{},
			tfimpl.OpenTofu,
			testDir,
		)
		require.NoError(t, err)
		require.Equal(
			t,
			want,
			parsed["registry.opentofu.org/example/test"],
			"constraint: %q",
			constraint,
		)
	})
}
