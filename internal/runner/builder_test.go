package runner_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	goversion "github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestBuild pins which units Build discovers for a given set of options.
func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("empty working dir yields no units", func(t *testing.T) {
		t.Parallel()

		v := memVenv(tfVersionOutput)
		require.NoError(t, v.FS.MkdirAll(memRoot, 0o755))

		rnr, err := runner.New(
			t.Context(),
			thlogger.CreateLogger(),
			v,
			newStackOpts(t, memRoot, tf.CommandNamePlan),
		)
		require.NoError(t, err)
		assert.Empty(t, rnr.GetStack().Units, "a working dir without configs discovers no units")
	})

	t.Run("units and their dependencies are discovered", func(t *testing.T) {
		t.Parallel()

		v := memVenv(tfVersionOutput)
		writeUnit(t, v, memRoot, "vpc", "")
		writeUnit(t, v, memRoot, "app", dependencyBlock("../vpc"))

		rnr, err := runner.New(
			t.Context(),
			thlogger.CreateLogger(),
			v,
			newStackOpts(t, memRoot, tf.CommandNamePlan),
		)
		require.NoError(t, err)

		byPath := unitsByPath(rnr)
		require.Len(t, byPath, 2, "both the unit and its dependency are discovered")
		require.Contains(t, byPath, filepath.Join(memRoot, "app"))
		assert.Equal(
			t,
			[]string{filepath.Join(memRoot, "vpc")},
			dependencyPaths(byPath[filepath.Join(memRoot, "app")]),
			"the dependency block is resolved into a graph edge",
		)
	})

	t.Run("root working dir wins over working dir", func(t *testing.T) {
		t.Parallel()

		v := memVenv(tfVersionOutput)
		writeUnit(t, v, memRoot, "vpc", "")
		writeUnit(t, v, memRoot, "app", "")

		opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
		opts.WorkingDir = filepath.Join(memRoot, "app")

		rnr, err := runner.New(t.Context(), thlogger.CreateLogger(), v, opts)
		require.NoError(t, err)
		assert.Len(
			t,
			rnr.GetStack().Units,
			2,
			"discovery starts from the root working dir, so the sibling unit is discovered too",
		)
	})

	t.Run("working dir is used when no root working dir is set", func(t *testing.T) {
		t.Parallel()

		v := memVenv(tfVersionOutput)
		writeUnit(t, v, memRoot, "vpc", "")
		writeUnit(t, v, memRoot, "app", "")

		opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
		opts.RootWorkingDir = ""
		opts.WorkingDir = filepath.Join(memRoot, "app")

		rnr, err := runner.New(t.Context(), thlogger.CreateLogger(), v, opts)
		require.NoError(t, err)

		units := rnr.GetStack().Units
		require.Len(t, units, 1, "discovery falls back to the working dir")
		assert.Equal(t, filepath.Join(memRoot, "app"), units[0].Path())
	})

	t.Run("custom config filename is discovered", func(t *testing.T) {
		t.Parallel()

		v := memVenv(tfVersionOutput)
		unitDir := filepath.Join(memRoot, "unit")
		require.NoError(t, v.FS.MkdirAll(unitDir, 0o755))
		require.NoError(
			t,
			vfs.WriteFile(v.FS, filepath.Join(unitDir, "custom.hcl"), nil, 0o644),
		)

		opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
		opts.TerragruntConfigPath = filepath.Join(memRoot, "custom.hcl")

		rnr, err := runner.New(t.Context(), thlogger.CreateLogger(), v, opts)
		require.NoError(t, err)

		units := rnr.GetStack().Units
		require.Len(t, units, 1, "the custom config filename is added to the discovered filenames")
		assert.Equal(t, unitDir, units[0].Path())
	})

	t.Run("filters and discovery boundary narrow the stack", func(t *testing.T) {
		t.Parallel()

		v := memVenv(tfVersionOutput)
		writeUnit(t, v, memRoot, "keep", "")
		writeUnit(t, v, memRoot, "drop", "")

		l := thlogger.CreateLogger()

		filters, err := filter.ParseFilterQueries(l, []string{filepath.Join(memRoot, "keep")})
		require.NoError(t, err)

		opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
		opts.Filters = filters
		opts.DiscoveryBoundary = memRoot

		rnr, err := runner.New(
			t.Context(),
			l,
			v,
			opts,
			runner.WithWorktrees(&worktrees.Worktrees{OriginalWorkingDir: memRoot}),
		)
		require.NoError(t, err)

		units := rnr.GetStack().Units
		require.Len(t, units, 1, "only the filtered unit survives discovery")
		assert.Equal(t, filepath.Join(memRoot, "keep"), units[0].Path())
	})

	t.Run("unparsable unit config fails the build", func(t *testing.T) {
		t.Parallel()

		v := memVenv(tfVersionOutput)
		writeUnit(t, v, memRoot, "vpc", invalidHCL)

		_, err := runner.New(
			t.Context(),
			thlogger.CreateLogger(),
			v,
			newStackOpts(t, memRoot, tf.CommandNamePlan),
		)
		require.Error(t, err)
	})

	t.Run("missing working dir fails discovery", func(t *testing.T) {
		t.Parallel()

		v := memVenv(tfVersionOutput)
		writeUnit(t, v, memRoot, "vpc", "")

		opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
		opts.WorkingDir = filepath.Join(memRoot, "does-not-exist")
		opts.RootWorkingDir = opts.WorkingDir

		_, err := runner.New(t.Context(), thlogger.CreateLogger(), v, opts)
		require.Error(t, err)
	})
}

// TestBuild_VersionConstraints pins the typed errors Build returns for unsatisfied version constraints.
func TestNew_VersionConstraints(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		assertErr func(*testing.T, error)
		name      string
		unitBody  string
		tfVersion string
	}{
		{
			name:      "default constraints are satisfied",
			tfVersion: tfVersionOutput,
		},
		{
			name:      "terraform version constraint is violated",
			unitBody:  `terraform_version_constraint = ">= v2.0.0"`,
			tfVersion: tfVersionOutput,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var target run.InvalidTerraformVersion

				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:      "terragrunt version constraint is violated",
			unitBody:  `terragrunt_version_constraint = ">= v99.0.0"`,
			tfVersion: tfVersionOutput,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var target run.InvalidTerragruntVersion

				require.ErrorAs(t, err, &target)
			},
		},
		{
			name:      "version probe output is unparsable",
			tfVersion: "not a version",
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var target run.InvalidTerraformVersionSyntax

				require.ErrorAs(t, err, &target)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := memVenv(tc.tfVersion)
			writeUnit(t, v, memRoot, "vpc", tc.unitBody)

			opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
			opts.TerragruntVersion = goversion.Must(goversion.NewVersion("0.1.0"))

			rnr, err := runner.New(t.Context(), thlogger.CreateLogger(), v, opts)

			if tc.assertErr != nil {
				tc.assertErr(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, rnr.GetStack().Units, 1, "a unit meeting its constraints is kept")
		})
	}
}

// TestCheckUnitVersionConstraints_UnitLeftUnparsed pins that a unit discovery left unparsed
// still gets the constraints its config declares. The check parses that config by its full
// path, not by the bare filename discovery recorded.
func TestCheckUnitVersionConstraints_UnitLeftUnparsed(t *testing.T) {
	t.Parallel()

	v := memVenv(tfVersionOutput)
	unitDir := writeUnit(t, v, memRoot, "vpc", `terragrunt_version_constraint = ">= v99.0.0"`)

	unit := component.NewUnit(unitDir)
	require.Nil(t, unit.Config(), "the unit has no parsed config for the check to reuse")

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	opts.TerragruntVersion = goversion.Must(goversion.NewVersion("0.1.0"))

	l := thlogger.CreateLogger()

	unitOpts, unitLogger, err := runner.BuildUnitOpts(l, opts, unit)
	require.NoError(t, err)

	err = runner.CheckUnitVersionConstraints(t.Context(), l, v, unitOpts, unitLogger, unit)

	var target run.InvalidTerragruntVersion

	require.ErrorAs(t, err, &target)
}

// TestBuild_TerraformBinaryOverridesVersionProbe pins that the version probe runs a unit's terraform_binary.
func TestNew_TerraformBinaryOverridesVersionProbe(t *testing.T) {
	t.Parallel()

	var (
		mu      sync.Mutex
		invoked []string
	)

	v := venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		mu.Lock()
		defer mu.Unlock()

		invoked = append(invoked, inv.Name)

		return vexec.Result{Stdout: []byte(tfVersionOutput + "\n")}
	})

	writeUnit(t, v, memRoot, "vpc", `terraform_binary = "custom-tofu"`)

	_, err := runner.New(
		t.Context(),
		thlogger.CreateLogger(),
		v,
		newStackOpts(t, memRoot, tf.CommandNamePlan),
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	assert.Contains(t, invoked, "custom-tofu", "the version probe runs the unit's terraform_binary")
}

// dependencyBlock returns a dependency block pointing at configPath.
func dependencyBlock(configPath string) string {
	return `dependency "dep" {
  config_path = "` + configPath + `"
}`
}
