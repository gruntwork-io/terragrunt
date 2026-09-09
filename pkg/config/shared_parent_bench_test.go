package config_test

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/require"
)

// benchSharedParentRoot holds locals a shared root config plausibly carries: a chain that
// needs several fixed point passes, and only functions whose result cannot vary with the
// unit doing the including.
const benchSharedParentRoot = `
locals {
  account_name = "acme"
  environment  = "prod"
  region       = "us-east-1"

  name_prefix = "${local.account_name}-${local.environment}"
  bucket      = lower("${local.name_prefix}-${local.region}-state")
  lock_table  = "${local.name_prefix}-locks"

  tags = {
    Account     = local.account_name
    Environment = local.environment
    Region      = local.region
    Owner       = upper(local.account_name)
  }

  tag_keys = sort(keys(local.tags))
  tag_line = join(",", [for k in local.tag_keys : "${k}=${local.tags[k]}"])
}

inputs = {
  bucket     = local.bucket
  lock_table = local.lock_table
  tags       = local.tags
  tag_line   = local.tag_line
}
`

// benchSharedParentRootChildDependent is [benchSharedParentRoot] with one local answering
// from the including unit's own directory, which is what a shared parent looks like when it
// cannot be decoded once for every unit.
const benchSharedParentRootChildDependent = `
locals {
  account_name = "acme"
  environment  = "prod"
  region       = "us-east-1"

  name_prefix = "${local.account_name}-${local.environment}"
  bucket      = lower("${local.name_prefix}-${local.region}-state")
  lock_table  = "${local.name_prefix}-locks"

  unit_dir = get_terragrunt_dir()

  tags = {
    Account     = local.account_name
    Environment = local.environment
    Region      = local.region
    Owner       = upper(local.account_name)
    Unit        = basename(local.unit_dir)
  }

  tag_keys = sort(keys(local.tags))
  tag_line = join(",", [for k in local.tag_keys : "${k}=${local.tags[k]}"])
}

inputs = {
  bucket     = local.bucket
  lock_table = local.lock_table
  tags       = local.tags
  tag_line   = local.tag_line
}
`

// BenchmarkSharedParentParse measures a `run --all` shaped parse: every unit in the run
// includes the same root config, so the parent is decoded once per unit per decode list.
func BenchmarkSharedParentParse(b *testing.B) {
	benchmarkSharedParent(b, benchSharedParentRoot)
}

// BenchmarkSharedParentParseUncacheable measures the same shape with a root whose locals
// answer from the including unit's directory, the case a parent-level memo has to decline.
func BenchmarkSharedParentParseUncacheable(b *testing.B) {
	benchmarkSharedParent(b, benchSharedParentRootChildDependent)
}

// benchmarkSharedParent parses every unit of a tree that shares one root config, at the
// sizes a small stack, a service, and a large monorepo produce.
func benchmarkSharedParent(b *testing.B, root string) {
	b.Helper()

	for _, units := range []int{3, 25, 100} {
		configPaths := benchSharedParentTree(b, units, root)

		b.Run("units="+strconv.Itoa(units), func(b *testing.B) {
			b.ReportAllocs()

			// Parsing logs a line per decode at debug level, so leaving the output attached
			// would measure writes to the test's stderr alongside the parse.
			l := logger.CreateLogger()
			l.SetOptions(log.WithOutput(io.Discard))

			v := venvtest.NewWithOSFS()
			baseCtx, _ := newTestParsingContext(b, v, configPaths[0])

			for b.Loop() {
				// A fresh cache per iteration: config caches are per run, so a run never
				// starts warm.
				ctx := config.WithConfigValues(baseCtx)

				for _, configPath := range configPaths {
					// Each unit is parsed with a parsing context of its own, the way a run
					// gives every unit one while sharing the caches on ctx.
					_, pctx := newTestParsingContext(b, v, configPath)

					if _, err := config.ParseConfigFile(ctx, pctx, l, configPath, nil); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// benchSharedParentTree writes units units that all include one root config holding root,
// and returns their config paths.
func benchSharedParentTree(b *testing.B, units int, root string) []string {
	b.Helper()

	const (
		dirPerm  = 0755
		filePerm = 0644
	)

	dir := b.TempDir()
	require.NoError(b, os.WriteFile(filepath.Join(dir, benchRootFileName), []byte(root), filePerm))

	configPaths := make([]string, units)

	for i := range units {
		unitDir := filepath.Join(dir, "unit-"+strconv.Itoa(i))
		require.NoError(b, os.MkdirAll(unitDir, dirPerm))

		configPaths[i] = filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
		require.NoError(b, os.WriteFile(configPaths[i], []byte(benchUnit(i)), filePerm))
	}

	return configPaths
}

// benchUnit returns a unit that includes the shared root and labels its own inputs.
func benchUnit(i int) string {
	return `
include "root" {
  path = "../` + benchRootFileName + `"
}

inputs = {
  unit = "unit-` + strconv.Itoa(i) + `"
}
`
}
