package discovery_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// rewriteCorpus is a repository whose units use what discovery reads from a parse: includes with locals, a local
// module source, dependency and dependencies blocks, exclude and errors blocks, feature flags, version
// constraints, a stack file, and a unit that does not parse.
var rewriteCorpus = map[string]string{
	"root.hcl": `
locals {
  env    = "dev"
  common = yamldecode(file("common.yaml"))
}

feature "canary" {
  default = false
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/app"
}

inputs = {
  env    = local.env
  region = local.common.region
}
`,
	"common.yaml":         "region: us-east-1\n",
	"modules/app/main.tf": `resource "null_resource" "app" {}` + "\n",
	"apps/web/terragrunt.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "db" {
  config_path = "../db"
}

exclude {
  if      = true
  actions = ["plan"]
}

errors {
  retry "transient" {
    retryable_errors   = [".*timeout.*"]
    max_attempts       = 3
    sleep_interval_sec = 1
  }

  ignore "known" {
    ignorable_errors = [".*known.*"]
    message          = "ignored"
  }
}

inputs = {
  db_id = dependency.db.outputs.id
}
`,
	"apps/db/terragrunt.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependencies {
  paths = ["../../infra/vpc"]
}

terragrunt_version_constraint = ">= 0.1.0"
`,
	"infra/vpc/terragrunt.hcl": `
terraform {
  source = "git::https://example.com/vpc.git//vpc?ref=v1"
}

feature "wide" {
  default = "no"
}
`,
	"apps/broken/terragrunt.hcl": "locals {\n",
	"stacks/live/terragrunt.stack.hcl": `
unit "web" {
  source = "../../apps/web"
  path   = "web"
}
`,
}

// componentView is what discovery and the runner read from a discovered component.
type componentView struct {
	Exclude      *config.ExcludeConfig
	Errors       *config.ErrorsConfig
	Source       *string
	Constraint   string
	Path         string
	Kind         component.Kind
	FeatureFlags config.FeatureFlags
	Dependencies []string
	Reading      []string
	External     bool
	Excluded     bool
}

// discoverCorpus runs discovery over rewriteCorpus, with the rewrite selected when useRewrite is set and parsing
// through store, and returns what it found in path order.
func discoverCorpus(t *testing.T, useRewrite bool, store *hclparse.Store) []componentView {
	t.Helper()

	v := memRepoRootVenv(t, corpusRepoRoot)
	writeFixture(t, v, prefixPaths(rewriteCorpus))

	if useRewrite {
		v.Env[rewrite.EnvRewrite] = "true"
	}

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = corpusRepoRoot
	opts.RootWorkingDir = corpusRepoRoot

	l := logger.CreateLogger()

	d := discovery.NewDiscovery(corpusRepoRoot).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: corpusRepoRoot}).
		WithGitRoot(corpusRepoRoot).
		WithRequiresParse().
		WithRelationships().
		WithParseExclude().
		WithTrackReads().
		WithSuppressParseErrors().
		WithParseStore(store)

	components, err := d.Discover(t.Context(), l, v, opts)
	require.NoError(t, err)

	views := make([]componentView, 0, len(components))

	for _, c := range components {
		view := componentView{
			Path:     c.Path(),
			Kind:     c.Kind(),
			External: c.External(),
			Reading:  slices.Sorted(slices.Values(c.Reading())),
		}

		unit, ok := c.(*component.Unit)
		if !ok {
			views = append(views, view)

			continue
		}

		view.Excluded = unit.Excluded()
		view.Dependencies = slices.Sorted(slices.Values(unit.Dependencies().Paths()))

		if cfg := unit.Config(); cfg != nil {
			view.Exclude = cfg.Exclude
			view.Errors = cfg.Errors
			view.Constraint = cfg.TerragruntVersionConstraint
			view.FeatureFlags = cfg.FeatureFlags

			if cfg.Terraform != nil {
				view.Source = cfg.Terraform.Source
			}
		}

		views = append(views, view)
	}

	slices.SortFunc(views, func(a, b componentView) int {
		return strings.Compare(a.Path, b.Path)
	})

	return views
}

func TestDiscoveryConfigRewriteMatchesDefaultParser(t *testing.T) {
	t.Parallel()

	want := discoverCorpus(t, false, new(hclparse.Store))
	require.NotEmpty(t, want)

	store := new(hclparse.Store)

	got := discoverCorpus(t, true, store)
	assert.Equal(t, want, got)

	misses := store.Stats().Misses
	require.Positive(t, misses, "the rewrite parsed through the store")

	again := discoverCorpus(t, true, store)
	assert.Equal(t, want, again)
	assert.Equal(t, misses, store.Stats().Misses, "the second discovery parsed nothing")
}

func TestDiscoveryConfigRewriteOffByDefault(t *testing.T) {
	t.Parallel()

	store := new(hclparse.Store)

	discoverCorpus(t, false, store)

	assert.Equal(t, hclparse.Stats{}, store.Stats())
}

// prefixPaths returns files with each path joined onto the corpus repo root.
func prefixPaths(files map[string]string) map[string]string {
	prefixed := make(map[string]string, len(files))

	for path, contents := range files {
		prefixed[repoPath(path)] = contents
	}

	return prefixed
}
