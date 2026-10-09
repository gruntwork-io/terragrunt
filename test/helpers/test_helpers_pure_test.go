package helpers_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsWindows(t *testing.T) {
	t.Parallel()

	assert.Equal(t, runtime.GOOS == "windows", helpers.IsWindows())
}

func TestRootFolder(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		assert.Regexp(t, `^[A-Za-z]:/$`, helpers.RootFolder)

		return
	}

	assert.Equal(t, "/", helpers.RootFolder)
}

func TestFileURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "rooted slash path",
			path:     "/tmp/module",
			expected: "file:///tmp/module",
		},
		{
			name:     "drive letter path gets a leading slash",
			path:     "C:/tmp/module",
			expected: "file:///C:/tmp/module",
		},
		{
			name:     "native separators become slashes",
			path:     filepath.Join("C:"+string(filepath.Separator), "tmp", "module"),
			expected: "file:///C:/tmp/module",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, helpers.FileURL(tc.path))
		})
	}
}

func TestToSlashAll(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		[]string{"live/app", "live/db/unit", "root"},
		helpers.ToSlashAll([]string{
			filepath.Join("live", "app"),
			filepath.Join("live", "db", "unit"),
			"root",
		}),
	)

	empty := helpers.ToSlashAll(nil)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

func TestValidateHookTraceParent(t *testing.T) {
	t.Parallel()

	output := strings.Join([]string{
		"before_hook ran",
		`after_hook {"traceparent": "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"}`,
		"done",
	}, "\n")

	helpers.ValidateHookTraceParent(t, "after_hook", output)
}

func TestCreateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	helpers.CreateFile(t, dir, "nested", "deeper", "terragrunt.hcl")

	path := filepath.Join(dir, "nested", "deeper", "terragrunt.hcl")
	require.FileExists(t, path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Zero(t, info.Size())
}

func TestMakeDiscoveryContext(t *testing.T) {
	t.Parallel()

	base := &component.DiscoveryContext{
		WorkingDir: "/base",
		Cmd:        "plan",
		Args:       []string{"-out", "plan.out"},
	}

	got := helpers.MakeDiscoveryContext(base, "/unit")

	assert.Equal(t, "/unit", got.WorkingDir)
	assert.Equal(t, "plan", got.Cmd)
	assert.Equal(t, []string{"-out", "plan.out"}, got.Args)

	// The copy owns its arguments, so changing them leaves the base alone.
	got.Args[0] = "-destroy"

	assert.Equal(t, []string{"-out", "plan.out"}, base.Args)
	assert.Equal(t, "/base", base.WorkingDir)
}

func TestMakeOpts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	opts := helpers.MakeOpts(dir)

	require.NotNil(t, opts)
	assert.Equal(t, dir, opts.WorkingDir)
	assert.Equal(t, dir, opts.RootWorkingDir)
}

func TestTmpDirWOSymlinks(t *testing.T) {
	t.Parallel()

	dir := helpers.TmpDirWOSymlinks(t)

	require.DirExists(t, dir)

	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, resolved, dir)
}

func TestFindCacheWorkingDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	moduleDir := filepath.Join(root, ".terragrunt-cache", "hash", "module")

	// Files beside the hash and module directories are not part of the layout.
	thTWriteFile(t, filepath.Join(root, ".terragrunt-cache", "stray.txt"), "")
	thTWriteFile(t, filepath.Join(root, ".terragrunt-cache", "hash", ".lock"), "")
	thTWriteFile(t, filepath.Join(moduleDir, "main.tf"), "")

	assert.Equal(t, moduleDir, helpers.FindCacheWorkingDir(t, root))
}

func TestFileExistsInCache(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	thTWriteFile(t, filepath.Join(root, ".terragrunt-cache", "hash", "module", "main.tf"), "")

	assert.True(t, helpers.FileExistsInCache(t, root, "main.tf"))
	assert.False(t, helpers.FileExistsInCache(t, root, "outputs.tf"))
}

func TestFindCachedFile(t *testing.T) {
	t.Parallel()

	unitDir := t.TempDir()
	want := filepath.Join(unitDir, ".terragrunt-cache", "a", "b", "outputs.json")

	thTWriteFile(t, want, "{}")
	thTWriteFile(t, filepath.Join(unitDir, "terragrunt.hcl"), "")
	thTWriteFile(t, filepath.Join(unitDir, ".terragrunt-cache", "a", "main.tf"), "")

	assert.Equal(t, want, helpers.FindCachedFile(t, unitDir, "outputs.json"))
}

func TestValidateAuthProviderScript(t *testing.T) {
	t.Parallel()

	var got vexec.Invocation

	v := &venv.Venv{
		Exec: vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			got = inv

			return vexec.Result{Stdout: []byte(`{"envs": {"TG_TEST_TOKEN": "secret"}}`)}
		}),
		Env: map[string]string{"TG_TEST_ENV": "value"},
	}

	helpers.ValidateAuthProviderScript(t, v, "/work", "./auth-provider.sh")

	// The script runs through the venv's executor, in dir, with the venv's environment.
	assert.Equal(t, "./auth-provider.sh", got.Name)
	assert.Equal(t, "/work", got.Dir)
	assert.Equal(t, []string{"TG_TEST_ENV=value"}, got.Env)
}

func TestIsExperimentMode(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected bool
	}{
		{name: "true", value: "true", expected: true},
		{name: "case and surrounding space are ignored", value: " TRUE ", expected: true},
		{name: "false", value: "false"},
		{name: "empty", value: ""},
		{name: "other truthy spelling", value: "1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TG_EXPERIMENT_MODE", tc.value)

			assert.Equal(t, tc.expected, helpers.IsExperimentMode(t))
		})
	}
}

func TestSkipInExperimentMode(t *testing.T) {
	t.Run("runs when experiment mode is off", func(t *testing.T) {
		t.Setenv("TG_EXPERIMENT_MODE", "false")

		helpers.SkipInExperimentMode(t, "some-experiment")

		assert.False(t, t.Skipped())
	})

	var (
		skippedTest *testing.T
		reached     bool
	)

	t.Run("skips when experiment mode forces experiments on", func(t *testing.T) {
		skippedTest = t

		t.Setenv("TG_EXPERIMENT_MODE", "true")

		helpers.SkipInExperimentMode(t, "some-experiment")

		reached = true
	})

	require.NotNil(t, skippedTest)
	assert.True(t, skippedTest.Skipped())
	assert.False(t, reached, "SkipInExperimentMode must stop the test")
}

// thTWriteFile writes content to path, creating its parent directories.
func thTWriteFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
