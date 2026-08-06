package runnerpool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	goversion "github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestResolveWorkingDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		rootWorkingDir string
		workingDir     string
		expected       string
	}{
		{
			name:           "root working dir wins",
			rootWorkingDir: "/repo",
			workingDir:     "/repo/unit",
			expected:       "/repo",
		},
		{
			name:           "falls back to working dir",
			rootWorkingDir: "",
			workingDir:     "/repo/unit",
			expected:       "/repo/unit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := &options.TerragruntOptions{
				RootWorkingDir: tc.rootWorkingDir,
				WorkingDir:     tc.workingDir,
			}

			assert.Equal(t, tc.expected, resolveWorkingDir(opts))
		})
	}
}

func TestBuildConfigFilenames(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		configPath string
		extra      string
	}{
		{
			name:       "default config name is not duplicated",
			configPath: "/repo/unit/terragrunt.hcl",
		},
		{
			name:       "default stack file is not duplicated",
			configPath: "/repo/unit/" + config.DefaultStackFile,
		},
		{
			name:       "custom config name is appended",
			configPath: "/repo/unit/custom.hcl",
			extra:      "custom.hcl",
		},
		{
			name:       "empty config path adds nothing",
			configPath: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildConfigFilenames(&options.TerragruntOptions{TerragruntConfigPath: tc.configPath})

			assert.Subset(t, got, discovery.DefaultConfigFilenames)

			if tc.extra == "" {
				assert.Len(t, got, len(discovery.DefaultConfigFilenames))

				return
			}

			assert.Contains(t, got, tc.extra)
			assert.Len(t, got, len(discovery.DefaultConfigFilenames)+1)
		})
	}
}

func TestExtractWorktrees(t *testing.T) {
	t.Parallel()

	w := &worktrees.Worktrees{OriginalWorkingDir: "/repo"}

	testCases := []struct {
		expected *worktrees.Worktrees
		name     string
		opts     []common.Option
	}{
		{
			name:     "no options",
			opts:     nil,
			expected: nil,
		},
		{
			name:     "no worktree option",
			opts:     []common.Option{common.WithParseOptions([]hclparse.Option{})},
			expected: nil,
		},
		{
			name:     "worktree option found",
			opts:     []common.Option{common.WithParseOptions(nil), common.WithWorktrees(w)},
			expected: w,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, extractWorktrees(tc.opts))
		})
	}
}

func TestDoWithTelemetry(t *testing.T) {
	t.Parallel()

	l := thlogger.CreateLogger()

	called := false

	require.NoError(t, doWithTelemetry(t.Context(), l, "test_event", nil,
		func(context.Context, log.Logger) error {
			called = true

			return nil
		}))
	assert.True(t, called)

	errBoom := errors.New("boom")
	err := doWithTelemetry(t.Context(), l, "test_event", map[string]any{"k": "v"},
		func(context.Context, log.Logger) error {
			return errBoom
		})
	require.ErrorIs(t, err, errBoom)
}

func TestNewBaseDiscovery(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/repo/terragrunt.hcl")
	require.NoError(t, err)

	opts.TerraformCliArgs = opts.TerraformCliArgs.Clone()

	d := newBaseDiscovery(opts, "/repo", discovery.DefaultConfigFilenames, common.WithParseOptions(nil))
	require.NotNil(t, d)
}

func TestPrepareDiscoveryFindsCustomConfigFilename(t *testing.T) {
	t.Parallel()

	tmpDir := tmpDirWOSymlinks(t)
	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "custom.hcl"), []byte(""), 0o600))

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "custom.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = tmpDir

	d := prepareDiscovery(opts)
	require.NotNil(t, d)

	components, err := d.Discover(t.Context(), thlogger.CreateLogger(), osFSVenv(), opts)
	require.NoError(t, err)
	require.Len(t, components, 1)
	assert.Equal(t, unitDir, components[0].Path())
}

func TestPrepareDiscoveryWithFiltersAndBoundary(t *testing.T) {
	t.Parallel()

	tmpDir := tmpDirWOSymlinks(t)
	writeUnit(t, tmpDir, "keep")
	writeUnit(t, tmpDir, "drop")

	l := thlogger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{filepath.Join(tmpDir, "keep")})
	require.NoError(t, err)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir
	opts.Filters = filters
	opts.DiscoveryBoundary = tmpDir

	d := prepareDiscovery(opts, common.WithWorktrees(&worktrees.Worktrees{OriginalWorkingDir: tmpDir}))
	require.NotNil(t, d)

	components, err := d.Discover(t.Context(), l, osFSVenv(), opts)
	require.NoError(t, err)
	require.Len(t, components, 1)
	assert.Equal(t, filepath.Join(tmpDir, "keep"), components[0].Path())
}

func TestDiscoverWithRetry(t *testing.T) {
	t.Parallel()

	tmpDir := tmpDirWOSymlinks(t)
	writeUnit(t, tmpDir, "vpc")
	writeUnitWithDependency(t, tmpDir, "app", "../vpc")

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	discovered, err := discoverWithRetry(t.Context(), thlogger.CreateLogger(), osFSVenv(), opts)
	require.NoError(t, err)
	assert.Len(t, discovered, 2)
}

func TestDiscoverWithRetryReturnsDiscoveryError(t *testing.T) {
	t.Parallel()

	tmpDir := tmpDirWOSymlinks(t)
	writeUnit(t, tmpDir, "unit")

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = filepath.Join(tmpDir, "does-not-exist")
	opts.RootWorkingDir = opts.WorkingDir

	_, err = discoverWithRetry(t.Context(), thlogger.CreateLogger(), osFSVenv(), opts)
	require.Error(t, err)
}

func TestCreateRunner(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/repo/terragrunt.hcl")
	require.NoError(t, err)

	comps := component.Components{
		component.NewUnit("/repo/vpc").WithConfig(&config.TerragruntConfig{}),
	}

	rnr, err := createRunner(t.Context(), thlogger.CreateLogger(), opts, comps)
	require.NoError(t, err)
	require.NotNil(t, rnr)
	assert.Len(t, rnr.GetStack().Units, 1)
}

func TestBuild(t *testing.T) {
	t.Parallel()

	t.Run("no units", func(t *testing.T) {
		t.Parallel()

		tmpDir := tmpDirWOSymlinks(t)

		opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
		require.NoError(t, err)

		opts.WorkingDir = tmpDir
		opts.RootWorkingDir = tmpDir

		rnr, err := Build(t.Context(), thlogger.CreateLogger(), osFSVenv(), opts)
		require.NoError(t, err)
		assert.Empty(t, rnr.GetStack().Units)
	})

	t.Run("discovered units", func(t *testing.T) {
		t.Parallel()

		tmpDir := tmpDirWOSymlinks(t)
		writeUnit(t, tmpDir, "vpc")
		writeUnitWithDependency(t, tmpDir, "app", "../vpc")

		opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
		require.NoError(t, err)

		opts.WorkingDir = tmpDir
		opts.RootWorkingDir = tmpDir

		rnr, err := Build(t.Context(), thlogger.CreateLogger(), tfVersionVenv("OpenTofu v1.9.0"), opts)
		require.NoError(t, err)
		assert.Len(t, rnr.GetStack().Units, 2)
	})

	t.Run("discovery failure", func(t *testing.T) {
		t.Parallel()

		tmpDir := tmpDirWOSymlinks(t)

		opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
		require.NoError(t, err)

		opts.WorkingDir = filepath.Join(tmpDir, "does-not-exist")
		opts.RootWorkingDir = opts.WorkingDir

		_, err = Build(t.Context(), thlogger.CreateLogger(), osFSVenv(), opts)
		require.Error(t, err)
	})

	t.Run("version constraint failure", func(t *testing.T) {
		t.Parallel()

		tmpDir := tmpDirWOSymlinks(t)
		writeUnit(t, tmpDir, "vpc")

		opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
		require.NoError(t, err)

		opts.WorkingDir = tmpDir
		opts.RootWorkingDir = tmpDir

		_, err = Build(t.Context(), thlogger.CreateLogger(), tfVersionVenv("not a version"), opts)
		require.ErrorContains(t, err, "failed to populate Terraform version for unit")
	})
}

func TestCheckVersionConstraintsNoUnits(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/repo/terragrunt.hcl")
	require.NoError(t, err)

	require.NoError(t, checkVersionConstraints(t.Context(), thlogger.CreateLogger(), venvtest.New(), opts, nil))
}

func TestCheckVersionConstraintsPropagatesUnitError(t *testing.T) {
	t.Parallel()

	tmpDir := tmpDirWOSymlinks(t)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	// The unit carries no parsed config and no config file on disk, so the
	// partial parse inside the check fails.
	unit := component.NewUnit(filepath.Join(tmpDir, "missing"))

	err = checkVersionConstraints(t.Context(), thlogger.CreateLogger(), tfVersionVenv("OpenTofu v1.9.0"), opts, []*component.Unit{unit})
	require.ErrorContains(t, err, "failed to parse config for unit")
}

func TestCheckUnitVersionConstraints(t *testing.T) {
	t.Parallel()

	tmpDir := tmpDirWOSymlinks(t)

	testCases := []struct {
		name        string
		unitConfig  *config.TerragruntConfig
		tfVersion   string
		expectedErr string
	}{
		{
			name:       "parsed config satisfies defaults",
			unitConfig: &config.TerragruntConfig{},
			tfVersion:  "OpenTofu v1.9.0",
		},
		{
			name: "terraform_binary overrides tf path",
			unitConfig: &config.TerragruntConfig{
				TerraformBinary: "custom-tofu",
			},
			tfVersion: "OpenTofu v1.9.0",
		},
		{
			name: "terraform version constraint violated",
			unitConfig: &config.TerragruntConfig{
				TerraformVersionConstraint: ">= v2.0.0",
			},
			tfVersion:   "OpenTofu v1.9.0",
			expectedErr: "terraform version check failed for unit",
		},
		{
			name: "terragrunt version constraint violated",
			unitConfig: &config.TerragruntConfig{
				TerragruntVersionConstraint: ">= v99.0.0",
			},
			tfVersion:   "OpenTofu v1.9.0",
			expectedErr: "terragrunt version check failed for unit",
		},
		{
			name:        "version probe fails",
			unitConfig:  &config.TerragruntConfig{},
			tfVersion:   "not a version",
			expectedErr: "failed to populate Terraform version for unit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
			require.NoError(t, err)

			opts.WorkingDir = tmpDir
			opts.TerragruntVersion = mustVersion(t, "0.1.0")

			unit := component.NewUnit(tmpDir).WithConfig(tc.unitConfig)
			l := thlogger.CreateLogger()

			unitOpts, unitLogger, err := BuildUnitOpts(l, opts, unit)
			require.NoError(t, err)

			err = checkUnitVersionConstraints(
				t.Context(),
				l,
				tfVersionVenv(tc.tfVersion),
				unitOpts,
				unitLogger,
				unit,
			)

			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "1.9.0", unitOpts.TerraformVersion.String())
		})
	}
}

// tmpDirWOSymlinks returns a symlink-free temp dir, matching the canonical
// paths discovery reports.
func tmpDirWOSymlinks(t *testing.T) string {
	t.Helper()

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	return tmpDir
}

// osFSVenv returns an in-memory venv backed by the real filesystem, for tests
// that discover or parse configs written to a temp dir.
func osFSVenv() *venv.Venv {
	return venvtest.New().WithFS(vfs.NewOSFS())
}

// tfVersionVenv returns a venv whose exec answers any `-version` invocation
// with the given output, so version probes never spawn a real binary.
func tfVersionVenv(output string) *venv.Venv {
	return osFSVenv().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte(output + "\n")}
	})
}

func mustVersion(t *testing.T, v string) *goversion.Version {
	t.Helper()

	parsed, err := goversion.NewVersion(v)
	require.NoError(t, err)

	return parsed
}

func writeUnit(t *testing.T, root, name string) string {
	t.Helper()

	return writeUnitWithContent(t, root, name, "")
}

func writeUnitWithDependency(t *testing.T, root, name, depPath string) string {
	t.Helper()

	return writeUnitWithContent(t, root, name, "dependency \"dep\" {\n  config_path = \""+depPath+"\"\n}\n")
}

func writeUnitWithContent(t *testing.T, root, name, content string) string {
	t.Helper()

	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(content), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(""), 0o600))

	return dir
}
