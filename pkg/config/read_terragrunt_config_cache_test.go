package config_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// countingFS tallies how often each path is opened beneath it, so a test can
// tell a shared read_terragrunt_config result from one parsed again.
type countingFS struct {
	vfs.FS
	opens map[string]int
	mu    sync.Mutex
}

func (fsys *countingFS) Open(name string) (vfs.File, error) {
	fsys.count(name)

	return fsys.FS.Open(name)
}

func (fsys *countingFS) OpenFile(name string, flag int, perm os.FileMode) (vfs.File, error) {
	fsys.count(name)

	return fsys.FS.OpenFile(name, flag, perm)
}

func (fsys *countingFS) count(name string) {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()

	fsys.opens[filepath.Clean(name)]++
}

func (fsys *countingFS) opensOf(name string) int {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()

	return fsys.opens[name]
}

const readConfigUnitHCL = `
locals {
  common = read_terragrunt_config("../common.hcl")
}

inputs = {
  value = local.common.locals.value
}
`

// writeReadConfigFixture writes two units that both read common.hcl, which
// holds commonHCL and sits beside accounts.yml, and returns the unit config
// paths.
func writeReadConfigFixture(t *testing.T, v *venv.Venv, rootDir, commonHCL string, extra map[string]string) []string {
	t.Helper()

	files := map[string]string{
		"common.hcl":   commonHCL,
		"accounts.yml": "dev: \"111111111111\"\n",
		filepath.Join("unit-a", "terragrunt.hcl"): readConfigUnitHCL,
		filepath.Join("unit-b", "terragrunt.hcl"): readConfigUnitHCL,
	}

	maps.Copy(files, extra)

	for name, contents := range files {
		path := filepath.Join(rootDir, name)
		require.NoError(t, v.FS.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, vfs.WriteFile(v.FS, path, []byte(contents), 0o644))
	}

	return []string{
		filepath.Join(rootDir, "unit-a", config.DefaultTerragruntConfigPath),
		filepath.Join(rootDir, "unit-b", config.DefaultTerragruntConfigPath),
	}
}

// parseUnitInput parses the unit at cfgPath under ctx and returns its value
// input and the files the parse recorded. Each of opts adjusts the parsing
// context before the parse.
func parseUnitInput(
	ctx context.Context,
	t *testing.T,
	v *venv.Venv,
	cfgPath string,
	opts ...func(*config.ParsingContext),
) (any, []string) {
	t.Helper()

	_, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath
	pctx = pctx.WithFileReadTracking()

	for _, opt := range opts {
		opt(pctx)
	}

	cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
	require.NoError(t, err)

	return cfg.Inputs["value"], pctx.FilesRead.Paths()
}

// accountsOpens parses the first count units of a fixture whose common.hcl
// holds commonHCL, all in one command, and returns how often their parses
// opened accounts.yml.
func accountsOpens(t *testing.T, commonHCL string, extra map[string]string, count int) int {
	t.Helper()

	v, rootDir := newMemTestDir(t)
	units := writeReadConfigFixture(t, v, rootDir, commonHCL, extra)

	fsys := &countingFS{FS: v.FS, opens: map[string]int{}}
	v = v.WithFS(fsys)
	ctx := config.WithConfigValues(t.Context())

	for _, unit := range units[:count] {
		value, filesRead := parseUnitInput(ctx, t, v, unit)

		assert.Equal(t, "111111111111", value)
		assert.Contains(t, filesRead, filepath.Join(rootDir, "accounts.yml"))
		assert.Contains(t, filesRead, filepath.Join(rootDir, "common.hcl"))
	}

	return fsys.opensOf(filepath.Join(rootDir, "accounts.yml"))
}

func TestReadTerragruntConfigSharesResultAcrossReadingConfigs(t *testing.T) {
	t.Parallel()

	const commonHCL = `
locals {
  value = yamldecode(file("accounts.yml"))["dev"]
}
`

	assert.Equal(t, accountsOpens(t, commonHCL, nil, 1), accountsOpens(t, commonHCL, nil, 2))
}

func TestReadTerragruntConfigDoesNotShareCallerDependentResult(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		extra     map[string]string
		name      string
		commonHCL string
	}{
		{
			name: "original terragrunt dir",
			commonHCL: `
locals {
  value = get_original_terragrunt_dir()
}
`,
		},
		{
			name: "original terragrunt dir in a nested read",
			commonHCL: `
locals {
  value = read_terragrunt_config("inner.hcl").locals.value
}
`,
			extra: map[string]string{
				"inner.hcl": `
locals {
  value = get_original_terragrunt_dir()
}
`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, rootDir := newMemTestDir(t)
			units := writeReadConfigFixture(t, v, rootDir, tc.commonHCL, tc.extra)

			ctx := config.WithConfigValues(t.Context())

			for _, unit := range units {
				value, _ := parseUnitInput(ctx, t, v, unit)

				assert.Equal(t, filepath.Dir(unit), value)
			}
		})
	}
}

func TestReadTerragruntConfigDoesNotShareResultAcrossEnvironments(t *testing.T) {
	t.Parallel()

	v, rootDir := newMemTestDir(t)
	units := writeReadConfigFixture(t, v, rootDir, `
locals {
  value = get_env("TG_TEST_READ_CONFIG_VALUE")
}
`, nil)

	ctx := config.WithConfigValues(t.Context())

	for i, unit := range units {
		want := []string{"first", "second"}[i]
		unitV := v.WithEnv(map[string]string{"TG_TEST_READ_CONFIG_VALUE": want})

		value, _ := parseUnitInput(ctx, t, unitV, unit)

		assert.Equal(t, want, value)
	}
}

func TestReadTerragruntConfigDoesNotShareResultWithDependencies(t *testing.T) {
	t.Parallel()

	const commonHCL = `
dependency "dep" {
  config_path  = "./dep"
  skip_outputs = true
}

locals {
  value = yamldecode(file("accounts.yml"))["dev"]
}
`

	extra := map[string]string{filepath.Join("dep", "terragrunt.hcl"): ""}

	assert.Greater(t, accountsOpens(t, commonHCL, extra, 2), accountsOpens(t, commonHCL, extra, 1))
}

func TestReadTerragruntConfigDoesNotSharePerCallResult(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		commonHCL string
	}{
		{
			name:      "uuid",
			commonHCL: `locals { value = uuid() }`,
		},
		{
			name:      "bcrypt",
			commonHCL: `locals { value = bcrypt("secret", 4) }`,
		},
		{
			name:      "uuid in a template",
			commonHCL: `locals { value = templatefile("id.tftpl", {}) }`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, rootDir := newMemTestDir(t)
			units := writeReadConfigFixture(t, v, rootDir, tc.commonHCL, map[string]string{"id.tftpl": "${uuid()}"})

			ctx := config.WithConfigValues(t.Context())

			first, _ := parseUnitInput(ctx, t, v, units[0])
			second, _ := parseUnitInput(ctx, t, v, units[1])

			assert.NotEqual(t, first, second)
		})
	}
}

func TestReadTerragruntConfigDoesNotShareUncachedRunCmd(t *testing.T) {
	t.Parallel()

	// runCmdCalls parses the first count units in one command and returns how
	// often their parses ran the command.
	runCmdCalls := func(count int) int32 {
		var calls atomic.Int32

		exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			calls.Add(1)

			return vexec.Result{Stdout: []byte("fresh\n")}
		})

		v, rootDir := newMemTestDir(t)
		v = v.WithExec(exec)
		units := writeReadConfigFixture(t, v, rootDir, `locals { value = run_cmd("--terragrunt-no-cache", "cmd") }`, nil)

		ctx := config.WithConfigValues(t.Context())

		for _, unit := range units[:count] {
			value, _ := parseUnitInput(ctx, t, v, unit)

			assert.Equal(t, "fresh", value)
		}

		return calls.Load()
	}

	assert.Equal(t, 2*runCmdCalls(1), runCmdCalls(2))
}

func TestReadTerragruntConfigReplaysRunCmdOutputAfterDiscardedRead(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("hello\n")}
	})

	v, rootDir := newMemTestDir(t)
	v = v.WithExec(exec).WithErrWriter(io.Discard)
	units := writeReadConfigFixture(t, v, rootDir, `locals { value = run_cmd("echoer") }`, nil)

	ctx := config.WithConfigValues(t.Context())

	parseUnitInput(ctx, t, v.WithWriter(io.Discard), units[0])

	var out bytes.Buffer

	value, _ := parseUnitInput(ctx, t, v.WithWriter(&out), units[1])

	assert.Equal(t, "hello", value)
	assert.Equal(t, "hello\n", out.String())
}

func TestReadTerragruntConfigSharedResultMatchesIsolatedParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		extra     map[string]string
		name      string
		commonHCL string
		opts      []func(*config.ParsingContext)
	}{
		{
			name:      "static value",
			commonHCL: `locals { value = yamldecode(file("accounts.yml"))["dev"] }`,
		},
		{
			name:      "original terragrunt dir",
			commonHCL: `locals { value = get_original_terragrunt_dir() }`,
		},
		{
			name:      "original terragrunt dir in a nested read",
			commonHCL: `locals { value = read_terragrunt_config("inner.hcl").locals.value }`,
			extra:     map[string]string{"inner.hcl": `locals { value = get_original_terragrunt_dir() }`},
		},
		{
			name: "original terragrunt dir in an exposed include",
			commonHCL: `
include "inner" {
  path   = "inner.hcl"
  expose = true
}

locals {
  value = include.inner.locals.value
}
`,
			extra: map[string]string{"inner.hcl": `locals { value = get_original_terragrunt_dir() }`},
		},
		{
			name:      "working dir",
			commonHCL: `locals { value = get_working_dir() }`,
		},
		{
			name: "working dir with a relative source",
			commonHCL: `
terraform {
  source = "./module"
}

locals {
  value = get_working_dir()
}
`,
			extra: map[string]string{filepath.Join("module", "main.tf"): ""},
		},
		{
			name: "working dir with a source map",
			commonHCL: `
terraform {
  source = "git::https://example.com/modules.git//app"
}

locals {
  value = get_working_dir()
}
`,
			extra: map[string]string{
				filepath.Join("unit-a", "local-modules", "app", "main.tf"): "",
				filepath.Join("unit-b", "local-modules", "app", "main.tf"): "",
			},
			opts: []func(*config.ParsingContext){func(pctx *config.ParsingContext) {
				pctx.SourceMap = map[string]string{"git::https://example.com/modules.git": "./local-modules"}
			}},
		},
		{
			name:      "terragrunt dir",
			commonHCL: `locals { value = get_terragrunt_dir() }`,
		},
		{
			name:      "parent terragrunt dir",
			commonHCL: `locals { value = get_parent_terragrunt_dir() }`,
		},
		{
			name:      "path relative to include",
			commonHCL: `locals { value = path_relative_to_include() }`,
		},
		{
			name:      "find in parent folders",
			commonHCL: `locals { value = read_terragrunt_config("nested/inner.hcl").locals.value }`,
			extra: map[string]string{
				filepath.Join("nested", "inner.hcl"): `locals { value = find_in_parent_folders("accounts.yml") }`,
			},
		},
		{
			name:      "environment variable with a default",
			commonHCL: `locals { value = get_env("TG_TEST_READ_CONFIG_UNSET", "fallback") }`,
		},
		{
			name: "dependency block",
			commonHCL: `
dependency "dep" {
  config_path  = "./dep"
  skip_outputs = true

  mock_outputs = {
    id = "mocked"
  }
}

locals {
  value = get_original_terragrunt_dir()
}
`,
			extra: map[string]string{filepath.Join("dep", "terragrunt.hcl"): ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, rootDir := newMemTestDir(t)
			units := writeReadConfigFixture(t, v, rootDir, tc.commonHCL, tc.extra)

			shared := config.WithConfigValues(t.Context())

			for _, unit := range units {
				isolated, _ := parseUnitInput(config.WithConfigValues(t.Context()), t, v, unit, tc.opts...)
				value, _ := parseUnitInput(shared, t, v, unit, tc.opts...)

				assert.Equal(t, isolated, value, unit)
			}
		})
	}
}

func TestReadTerragruntConfigKeepsReadingContextInputsApart(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		configure func(pctx *config.ParsingContext, setting string)
		want      func(setting string) any
		name      string
		commonHCL string
	}{
		{
			name:      "tofu command",
			commonHCL: `locals { value = get_terraform_command() }`,
			configure: func(pctx *config.ParsingContext, setting string) { pctx.TerraformCommand = setting },
			want:      func(setting string) any { return setting },
		},
		{
			name:      "tofu CLI arguments",
			commonHCL: `locals { value = get_terraform_cli_args() }`,
			configure: func(pctx *config.ParsingContext, setting string) {
				pctx.TerraformCliArgs = iacargs.New().SetCommand(setting)
			},
			want: func(setting string) any { return []any{setting} },
		},
		{
			name: "feature flag override",
			commonHCL: `
feature "name" {
  default = "default"
}

locals {
  value = feature.name.value
}
`,
			configure: func(pctx *config.ParsingContext, setting string) {
				pctx.FeatureFlags = map[string]string{"name": setting}
			},
			want: func(setting string) any { return setting },
		},
		{
			name:      "source flag",
			commonHCL: `locals { value = get_terragrunt_source_cli_flag() }`,
			configure: func(pctx *config.ParsingContext, setting string) { pctx.Source = setting },
			want:      func(setting string) any { return setting },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, rootDir := newMemTestDir(t)
			units := writeReadConfigFixture(t, v, rootDir, tc.commonHCL, nil)

			ctx := config.WithConfigValues(t.Context())

			for i, setting := range []string{"plan", "apply"} {
				value, _ := parseUnitInput(ctx, t, v, units[i], func(pctx *config.ParsingContext) {
					tc.configure(pctx, setting)
				})

				assert.Equal(t, tc.want(setting), value)
			}
		})
	}
}

func TestReadTerragruntConfigRereadsFileChangedDuringCommand(t *testing.T) {
	t.Parallel()

	v, rootDir := newMemTestDir(t)
	units := writeReadConfigFixture(t, v, rootDir, `locals { value = "before" }`, nil)

	ctx := config.WithConfigValues(t.Context())

	before, _ := parseUnitInput(ctx, t, v, units[0])
	assert.Equal(t, "before", before)

	commonPath := filepath.Join(rootDir, "common.hcl")
	require.NoError(t, vfs.WriteFile(v.FS, commonPath, []byte(`locals { value = "after" }`), 0o644))

	later := time.Now().Add(time.Hour)
	require.NoError(t, v.FS.Chtimes(commonPath, later, later))

	after, _ := parseUnitInput(ctx, t, v, units[1])
	assert.Equal(t, "after", after)
}

func TestReadTerragruntConfigConcurrentReadsWithRacing(t *testing.T) {
	t.Parallel()

	const unitCount = 16

	testCases := []struct {
		want      func(unitDir string) any
		name      string
		commonHCL string
	}{
		{
			name:      "shared result",
			commonHCL: `locals { value = yamldecode(file("accounts.yml"))["dev"] }`,
			want:      func(string) any { return "111111111111" },
		},
		{
			name:      "result per unit",
			commonHCL: `locals { value = get_original_terragrunt_dir() }`,
			want:      func(unitDir string) any { return unitDir },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			extra := map[string]string{}
			for i := range unitCount {
				extra[filepath.Join(fmt.Sprintf("unit-%02d", i), "terragrunt.hcl")] = readConfigUnitHCL
			}

			v, rootDir := newMemTestDir(t)
			writeReadConfigFixture(t, v, rootDir, tc.commonHCL, extra)

			ctx := config.WithConfigValues(t.Context())
			values := make([]any, unitCount)

			var g errgroup.Group

			for i := range unitCount {
				cfgPath := filepath.Join(rootDir, fmt.Sprintf("unit-%02d", i), config.DefaultTerragruntConfigPath)

				_, pctx := newTestParsingContext(t, cfgPath)
				pctx.OriginalTerragruntConfigPath = cfgPath

				g.Go(func() error {
					cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
					if err != nil {
						return err
					}

					values[i] = cfg.Inputs["value"]

					return nil
				})
			}

			require.NoError(t, g.Wait())

			for i, value := range values {
				assert.Equal(t, tc.want(filepath.Join(rootDir, fmt.Sprintf("unit-%02d", i))), value)
			}
		})
	}
}

func TestReadTerragruntConfigRecordsFilesReadAfterUntrackedRead(t *testing.T) {
	t.Parallel()

	v, rootDir := newMemTestDir(t)
	units := writeReadConfigFixture(t, v, rootDir, `locals { value = yamldecode(file("accounts.yml"))["dev"] }`, nil)

	ctx := config.WithConfigValues(t.Context())

	parseUnitInput(ctx, t, v, units[0], func(pctx *config.ParsingContext) { pctx.FilesRead = nil })

	_, filesRead := parseUnitInput(ctx, t, v, units[1])

	assert.Contains(t, filesRead, filepath.Join(rootDir, "accounts.yml"))
}

func TestReadTerragruntConfigHidesReadingConfigLocalsAndFeatures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		commonHCL string
	}{
		{
			name:      "locals",
			commonHCL: `feature "region" { default = local.region }`,
		},
		{
			name:      "feature flags",
			commonHCL: `feature "region" { default = feature.unit_region.value }`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, rootDir := newMemTestDir(t)
			writeReadConfigFixture(t, v, rootDir, tc.commonHCL, nil)

			unitPath := filepath.Join(rootDir, "reader", config.DefaultTerragruntConfigPath)
			require.NoError(t, v.FS.MkdirAll(filepath.Dir(unitPath), 0o755))
			require.NoError(t, vfs.WriteFile(v.FS, unitPath, []byte(`
feature "unit_region" {
  default = "us-east-1"
}

locals {
  region = "us-east-1"
  path   = "../common.hcl"
  common = read_terragrunt_config(local.path)
}
`), 0o644))

			ctx, pctx := newTestParsingContext(t, unitPath)
			pctx.OriginalTerragruntConfigPath = unitPath

			_, err := config.ParseConfigFile(config.WithConfigValues(ctx), logger.CreateLogger(), v, pctx, unitPath, nil)
			require.Error(t, err)
		})
	}
}

func TestReadTerragruntConfigReportsDiagnosticsOnEveryRead(t *testing.T) {
	t.Parallel()

	// commonDiagnostics parses the first count units in one command with a
	// diagnostics handler that drops every diagnostic, as hcl validate does,
	// and returns how often the handler saw diagnostics for common.hcl.
	commonDiagnostics := func(count int) int32 {
		v, rootDir := newMemTestDir(t)
		units := writeReadConfigFixture(t, v, rootDir, `
unsupported_argument = true

locals {
  value = "value"
}
`, nil)

		commonPath := filepath.Join(rootDir, "common.hcl")
		ctx := config.WithConfigValues(t.Context())

		var calls atomic.Int32

		for _, unit := range units[:count] {
			parseUnitInput(ctx, t, v, unit, func(pctx *config.ParsingContext) {
				pctx.Parser.DiagnosticsHandler = func(file *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
					for _, diag := range diags {
						if diag.Subject != nil && diag.Subject.Filename == commonPath {
							calls.Add(1)
						}
					}

					return nil, nil
				}
			})
		}

		return calls.Load()
	}

	once := commonDiagnostics(1)

	require.Positive(t, once)
	assert.Equal(t, 2*once, commonDiagnostics(2))
}
