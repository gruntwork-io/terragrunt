package config_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	config "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// includingRepo lays out root.hcl, common.yaml, and n units that include the root and read the YAML.
func includingRepo(tb testing.TB, root string, n int) (vfs.FS, []string) {
	tb.Helper()

	files := map[string]string{
		"root.hcl": `
locals {
  common = yamldecode(file("${get_parent_terragrunt_dir()}/common.yaml"))
  unit   = basename(get_terragrunt_dir())
}

inputs = {
  region = local.common.region
  unit   = local.unit
}
`,
		"common.yaml": "region: us-east-1\n",
	}

	paths := make([]string, 0, n)

	for i := range n {
		rel := filepath.Join(fmt.Sprintf("unit-%d", i), pkgconfig.DefaultTerragruntConfigPath)
		files[rel] = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

locals {
  name   = "unit"
  region = yamldecode(file(find_in_parent_folders("common.yaml"))).region
}

inputs = {
  name = local.name
}
`
		paths = append(paths, filepath.Join(root, rel))
	}

	fsys := vfs.NewMemMapFS()

	for rel, content := range files {
		require.NoError(tb, vfs.WriteFile(fsys, filepath.Join(root, rel), []byte(content), 0o644))
	}

	return fsys, paths
}

// parseForBenchmark parses path through store with a fresh context and fails b on any error.
func parseForBenchmark(b *testing.B, l log.Logger, v *venv.Venv, store *hclparse.Store, path string) {
	b.Helper()

	ctx, run := newTestParsingContext(b, path)

	_, _, err := config.ParseConfigFile(ctx, l, v, store, config.NewParseContext(run), path)
	require.NoError(b, err)
}

func TestParseConfigFileSharesTheParse(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 1)
	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &hclparse.Store{}

	ctx, run := newTestParsingContext(t, paths[0])
	run = run.WithFileReadTracking()

	first, firstCfg, err := config.ParseConfigFile(ctx, l, v, store, config.NewParseContext(run), paths[0])
	require.NoError(t, err)
	assert.Equal(t, hclparse.Stats{Hits: 0, Misses: 4}, store.Stats(), "two files, each at two stages")

	secondCtx, secondRun := newTestParsingContext(t, paths[0])
	secondRun = secondRun.WithFileReadTracking()

	second, secondCfg, err := config.ParseConfigFile(
		secondCtx,
		l,
		v,
		store,
		config.NewParseContext(secondRun),
		paths[0],
	)
	require.NoError(t, err)
	assert.Equal(t, hclparse.Stats{Hits: 1, Misses: 4}, store.Stats(), "the unit's parse is served whole")

	assert.Equal(t, firstCfg, secondCfg)
	assert.NotSame(t, firstCfg, secondCfg, "each caller gets a config of its own")
	assert.Equal(t, "us-east-1", secondCfg.Locals["region"])
	assert.Equal(t, run.FilesRead.Paths(), secondRun.FilesRead.Paths())
	assert.Contains(t, secondRun.FilesRead.Paths(), filepath.Join(root, "common.yaml"))

	firstV1, err := first.ToV1(ctx, l, v)
	require.NoError(t, err)

	secondV1, err := second.ToV1(secondCtx, l, v)
	require.NoError(t, err)

	assert.Equal(t, firstV1, secondV1)
	assert.Equal(t, map[string]any{"name": "unit", "region": "us-east-1", "unit": "unit-0"}, secondV1.Inputs)
}

func TestParseConfigFileSharesIncludedFilesAcrossUnits(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 3)
	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &hclparse.Store{}

	for _, path := range paths {
		ctx, run := newTestParsingContext(t, path)

		parsed, _, err := config.ParseConfigFile(ctx, l, v, store, config.NewParseContext(run), path)
		require.NoError(t, err)

		cfg, err := parsed.ToV1(ctx, l, v)
		require.NoError(t, err)
		assert.Equal(t, filepath.Base(filepath.Dir(path)), cfg.Inputs["unit"])
	}

	assert.Equal(
		t,
		hclparse.Stats{Hits: 2, Misses: 10},
		store.Stats(),
		"the root's syntax tree is parsed once; each unit evaluates it in its own context",
	)
}

func TestParseConfigFileParsesAgainAfterInvalidate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 1)
	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &hclparse.Store{}

	region := func() any {
		ctx, run := newTestParsingContext(t, paths[0])
		run = run.WithFileReadTracking()

		_, cfg, err := config.ParseConfigFile(ctx, l, v, store, config.NewParseContext(run), paths[0])
		require.NoError(t, err)

		return cfg.Locals["region"]
	}

	assert.Equal(t, "us-east-1", region())

	common := filepath.Join(root, "common.yaml")
	require.NoError(t, vfs.WriteFile(fsys, common, []byte("region: eu-west-1\n"), 0o644))

	assert.Equal(t, "us-east-1", region(), "the unit's parse is served until its reads are invalidated")

	store.Invalidate(common)

	assert.Equal(t, "eu-west-1", region())
}

func TestParseConfigFileRunsUnmemoizedForSuppliedValues(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 1)
	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &hclparse.Store{}

	ctx, run := newTestParsingContext(t, paths[0])
	locals := cty.ObjectVal(map[string]cty.Value{"from_run": cty.StringVal("run")})
	run.Locals = &locals

	for range 2 {
		_, _, err := config.ParseConfigFile(ctx, l, v, store, config.NewParseContext(run), paths[0])
		require.NoError(t, err)
	}

	assert.Equal(t, hclparse.Stats{Hits: 2, Misses: 2}, store.Stats(), "only the syntax trees are shared")
}

func TestParseContextStoreContextCoversEachInput(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), pkgconfig.DefaultTerragruntConfigPath)

	_, base := newTestParsingContext(t, cfgPath)
	baseKey, memoized := config.NewParseContext(base).StoreContext()
	require.True(t, memoized)
	require.NotEmpty(t, baseKey)

	keyed := map[string]func(*pkgconfig.ParsingContext){
		"config path":          func(p *pkgconfig.ParsingContext) { p.TerragruntConfigPath += ".other" },
		"original config path": func(p *pkgconfig.ParsingContext) { p.OriginalTerragruntConfigPath += ".other" },
		"working dir":          func(p *pkgconfig.ParsingContext) { p.WorkingDir += "-other" },
		"root working dir":     func(p *pkgconfig.ParsingContext) { p.RootWorkingDir += "-other" },
		"download dir":         func(p *pkgconfig.ParsingContext) { p.DownloadDir += "-other" },
		"source":               func(p *pkgconfig.ParsingContext) { p.Source = "git::other" },
		"source map":           func(p *pkgconfig.ParsingContext) { p.SourceMap = map[string]string{"a": "b"} },
		"command":              func(p *pkgconfig.ParsingContext) { p.TerraformCommand = "apply" },
		"original command":     func(p *pkgconfig.ParsingContext) { p.OriginalTerraformCommand = "apply" },
		"cli args":             func(p *pkgconfig.ParsingContext) { p.TerraformCliArgs = iacargs.New("-lock=false") },
		"iam role": func(p *pkgconfig.ParsingContext) {
			p.IAMRoleOptions = iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/a"}
		},
		"original iam role": func(p *pkgconfig.ParsingContext) {
			p.OriginalIAMRoleOptions = iam.RoleOptions{AssumeRoleDuration: 1}
		},
		"feature flags":     func(p *pkgconfig.ParsingContext) { p.FeatureFlags = map[string]string{"f": "1"} },
		"auth provider cmd": func(p *pkgconfig.ParsingContext) { p.AuthProviderCmd = "creds" },
		"diagnostics":       func(p *pkgconfig.ParsingContext) { p.Parser.Diagnostics = pkgconfig.DiagnosticsSuppressed },
		"ignore diagnostics": func(p *pkgconfig.ParsingContext) {
			p.Parser.IgnoreDiagnostics = true
		},
		"bare include rewrite": func(p *pkgconfig.ParsingContext) {
			p.Parser.RewriteBareInclude = !p.Parser.RewriteBareInclude
		},
		"halt only in blocks": func(p *pkgconfig.ParsingContext) {
			p.Parser.HaltOnErrorOnlyInBlocks = []string{"catalog"}
		},
		"skip output":             func(p *pkgconfig.ParsingContext) { p.SkipOutput = true },
		"skip outputs resolution": func(p *pkgconfig.ParsingContext) { p.SkipOutputsResolution = true },
		"parse depth":             func(p *pkgconfig.ParsingContext) { p.ParseDepth = 3 },
		"skip autoinclude merge":  func(p *pkgconfig.ParsingContext) { p.SkipAutoIncludeMerge = true },
		"read tracking":           func(p *pkgconfig.ParsingContext) { p.FilesRead = pkgconfig.NewFilesRead() },
	}

	for name, mutate := range keyed {
		t.Run("keyed/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestParsingContext(t, cfgPath)
			mutate(run)

			key, memoized := config.NewParseContext(run).StoreContext()
			require.True(t, memoized)
			assert.NotEqual(t, baseKey, key)
		})
	}

	fixed := map[string]func(*pkgconfig.ParsingContext){
		"tofu binary":       func(p *pkgconfig.ParsingContext) { p.TFPath = "terraform" },
		"log abs paths":     func(p *pkgconfig.ParsingContext) { p.LogShowAbsPaths = true },
		"debug":             func(p *pkgconfig.ParsingContext) { p.Debug = true },
		"max parent probes": func(p *pkgconfig.ParsingContext) { p.MaxFoldersToCheck = 5 },
		"decode list": func(p *pkgconfig.ParsingContext) {
			p.PartialParseDecodeList = []pkgconfig.PartialDecodeSectionType{pkgconfig.ExcludeBlock}
		},
	}

	for name, mutate := range fixed {
		t.Run("fixed/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestParsingContext(t, cfgPath)
			mutate(run)

			key, memoized := config.NewParseContext(run).StoreContext()
			require.True(t, memoized)
			assert.Equal(t, baseKey, key)
		})
	}

	value := cty.StringVal("v")
	unmemoized := map[string]func(*pkgconfig.ParsingContext){
		"locals":       func(p *pkgconfig.ParsingContext) { p.Locals = &value },
		"values":       func(p *pkgconfig.ParsingContext) { p.Values = &value },
		"features":     func(p *pkgconfig.ParsingContext) { p.Features = &value },
		"dependencies": func(p *pkgconfig.ParsingContext) { p.DecodedDependencies = &value },
		"includes":     func(p *pkgconfig.ParsingContext) { p.TrackInclude = &pkgconfig.TrackInclude{} },
		"diagnostics handler": func(p *pkgconfig.ParsingContext) {
			p.Parser.DiagnosticsHandler = func(*hcl.File, hcl.Diagnostics) (hcl.Diagnostics, error) { return nil, nil }
		},
	}

	for name, mutate := range unmemoized {
		t.Run("unmemoized/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestParsingContext(t, cfgPath)
			mutate(run)

			_, memoized := config.NewParseContext(run).StoreContext()
			assert.False(t, memoized)
		})
	}
}

func TestParityFixturesSharedStoreWithRacing(t *testing.T) {
	t.Parallel()

	fixtures := loadFixtures(t, parityFixtureRoots, parityExcludedDirs)
	store := &hclparse.Store{}

	t.Cleanup(func() {
		assert.Positive(t, store.Stats().Hits)
	})

	for _, pass := range []string{"first", "second"} {
		t.Run(pass, func(t *testing.T) {
			t.Parallel()

			for _, fixture := range fixtures.configs {
				if _, excluded := withOutputExcludedDirs[fixture.name]; excluded {
					continue
				}

				t.Run(fixture.name, func(t *testing.T) {
					t.Parallel()

					assertParity(t, parityCase{
						newFS:     fixtures.newFS,
						cfgPath:   fixture.path,
						configure: func(*pkgconfig.ParsingContext) {},
						store:     store,
					})
				})
			}
		})
	}
}

func BenchmarkParseConfigFile(b *testing.B) {
	for _, n := range []int{16, 128} {
		b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
			root := b.TempDir()
			fsys, paths := includingRepo(b, root, n)
			v := venvtest.New().WithFS(fsys)
			l := logger.CreateLogger()

			b.Run("store_per_unit", func(b *testing.B) {
				for b.Loop() {
					for _, path := range paths {
						parseForBenchmark(b, l, v, &hclparse.Store{}, path)
					}
				}
			})

			b.Run("store_shared", func(b *testing.B) {
				for b.Loop() {
					store := &hclparse.Store{}

					for _, path := range paths {
						parseForBenchmark(b, l, v, store, path)
					}
				}
			})

			b.Run("store_warm", func(b *testing.B) {
				store := &hclparse.Store{}

				for _, path := range paths {
					parseForBenchmark(b, l, v, store, path)
				}

				b.ResetTimer()

				for b.Loop() {
					for _, path := range paths {
						parseForBenchmark(b, l, v, store, path)
					}
				}
			})
		})
	}
}
