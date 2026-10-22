package discovery_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// sharedConfigUnitCounts sizes the shared configuration fixtures. Each account in a fixture holds
// sharedConfigUnitsPerAccount units, so every count is a multiple of it.
var sharedConfigUnitCounts = []int{100, 500, 1000, 5000}

const (
	sharedConfigUnitsPerAccount = 50
	sharedConfigRegistrySize    = 324
)

// BenchmarkDiscoverySharedConfig benchmarks discovery with dependencies, as `find --dependencies` runs it, over a
// repository where every unit includes a root.hcl that reads shared files with read_terragrunt_config. One of those
// files decodes an account registry of about 48 KB.
func BenchmarkDiscoverySharedConfig(b *testing.B) {
	for _, n := range sharedConfigUnitCounts {
		b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
			tmpDir := b.TempDir()
			createSharedConfigFixture(b, tmpDir, n/sharedConfigUnitsPerAccount)

			l := newDiscardLogger()
			opts := &options.TerragruntOptions{
				WorkingDir:        tmpDir,
				RootWorkingDir:    tmpDir,
				MaxFoldersToCheck: options.DefaultMaxFoldersToCheck,
			}
			v := venvtest.NewOSWithEmptyEnv()

			for b.Loop() {
				d := discovery.NewDiscovery(tmpDir).
					WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
					WithRequiresParse().
					WithRelationships()

				components, err := d.Discover(config.WithConfigValues(b.Context()), l, v, opts)
				require.NoError(b, err)
				require.Len(b, components, n)

				withDependencies := 0

				for _, c := range components {
					if len(c.Dependencies()) > 0 {
						withDependencies++
					}
				}

				require.Equal(b, n-n/(sharedConfigUnitsPerAccount/2), withDependencies)
			}
		})
	}
}

// createSharedConfigFixture writes a repository with the given number of accounts under tmpDir. Each account has two
// regions, and each region has a vpc unit and 24 app units that depend on it.
func createSharedConfigFixture(tb testing.TB, tmpDir string, accounts int) {
	tb.Helper()

	files := map[string]string{
		"accounts.yml":       sharedConfigRegistry(),
		"common.hcl":         sharedConfigCommonHCL,
		"root.hcl":           sharedConfigRootHCL,
		"_envcommon/app.hcl": sharedConfigEnvcommonHCL,
	}

	for a := range accounts {
		account := fmt.Sprintf("acct-%03d", a)
		files[filepath.Join(account, "account.hcl")] = fmt.Sprintf("locals {\n  account_name = %q\n}\n", account)

		for _, region := range []string{"us-east-1", "eu-west-1"} {
			regionDir := filepath.Join(account, region)
			files[filepath.Join(regionDir, "region.hcl")] = fmt.Sprintf("locals {\n  aws_region = %q\n}\n", region)
			files[filepath.Join(regionDir, "vpc", "terragrunt.hcl")] = sharedConfigVpcHCL

			for app := range sharedConfigUnitsPerAccount/2 - 1 {
				files[filepath.Join(regionDir, fmt.Sprintf("app-%02d", app), "terragrunt.hcl")] = sharedConfigAppHCL
			}
		}
	}

	for name, contents := range files {
		path := filepath.Join(tmpDir, name)
		require.NoError(tb, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(tb, os.WriteFile(path, []byte(contents), 0644))
	}
}

// sharedConfigRegistry returns an account registry in YAML with sharedConfigRegistrySize accounts.
func sharedConfigRegistry() string {
	var b strings.Builder

	for i := range sharedConfigRegistrySize {
		fmt.Fprintf(&b, "acct-%03d:\n", i)
		fmt.Fprintf(&b, "  id: \"%d\"\n", 100000000000+i)
		fmt.Fprintf(&b, "  email: \"aws+acct-%03d@example.com\"\n", i)
		fmt.Fprintf(&b, "  owner: \"team-%d\"\n", i%17)
		fmt.Fprintf(&b, "  tags:\n    cost_center: \"cc-%d\"\n", i%23)
		fmt.Fprintf(&b, "    environment: %q\n", []string{"dev", "stage", "prod"}[i%3])
	}

	return b.String()
}

const sharedConfigCommonHCL = `
locals {
  account_info = yamldecode(file("accounts.yml"))
  account_ids  = { for name, acct in local.account_info : name => acct.id }
}
`

const sharedConfigRootHCL = `
locals {
  common_vars  = read_terragrunt_config(find_in_parent_folders("common.hcl"))
  account_vars = read_terragrunt_config(find_in_parent_folders("account.hcl"))
  region_vars  = read_terragrunt_config(find_in_parent_folders("region.hcl"))

  account_name = local.account_vars.locals.account_name
  aws_region   = local.region_vars.locals.aws_region
  account_id   = local.common_vars.locals.account_ids[local.account_name]
}

feature "skip_ci" {
  default = false
}

remote_state {
  backend = "s3"
  config = {
    bucket = "tfstate-${local.account_name}"
    key    = "${path_relative_to_include()}/tofu.tfstate"
    region = local.aws_region
  }
}

inputs = {
  account_id = local.account_id
  aws_region = local.aws_region
}
`

const sharedConfigEnvcommonHCL = `
locals {
  account_vars = read_terragrunt_config(find_in_parent_folders("account.hcl"))
}

terraform {
  source = "git::https://example.com/modules.git//app?ref=v1.0.0"
}

inputs = {
  name = "app-${local.account_vars.locals.account_name}"
}
`

const sharedConfigVpcHCL = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "git::https://example.com/modules.git//vpc?ref=v1.0.0"
}
`

const sharedConfigAppHCL = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "envcommon" {
  path   = "${dirname(find_in_parent_folders("root.hcl"))}/_envcommon/app.hcl"
  expose = true
}

dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    vpc_id = "vpc-mock"
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`
