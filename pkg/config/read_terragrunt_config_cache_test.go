package config_test

import (
	"bytes"
	"context"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
// input and the files the parse recorded.
func parseUnitInput(ctx context.Context, t *testing.T, v *venv.Venv, cfgPath string) (any, []string) {
	t.Helper()

	_, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath
	pctx = pctx.WithFileReadTracking()

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
