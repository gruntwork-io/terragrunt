package runner_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestFindDependentUnits(t *testing.T) {
	t.Parallel()

	withRootInclude := &config.TerragruntConfig{
		ProcessedIncludes: config.IncludeConfigsMap{
			"root": {Name: "root", Path: filepath.Join(memRoot, "root.hcl")},
		},
	}

	testCases := []struct {
		cfg     *config.TerragruntConfig
		name    string
		unit    string
		vpcBody string
		want    []string
		gitRepo bool
	}{
		{
			name:    "searched from the git repo root",
			cfg:     &config.TerragruntConfig{},
			unit:    "vpc",
			gitRepo: true,
			want:    []string{filepath.Join(memRoot, "app")},
		},
		{
			name: "searched from the include directories outside a git repo",
			cfg:  withRootInclude,
			unit: "vpc",
			want: []string{filepath.Join(memRoot, "app")},
		},
		{
			name:    "unit nothing depends on",
			cfg:     &config.TerragruntConfig{},
			unit:    "app",
			gitRepo: true,
			want:    []string{},
		},
		{
			name: "no git repo and no includes",
			cfg:  &config.TerragruntConfig{},
			unit: "vpc",
			want: []string{},
		},
		{
			name:    "stack that cannot be built",
			cfg:     &config.TerragruntConfig{},
			unit:    "vpc",
			vpcBody: dependencyBlock("../app"),
			gitRepo: true,
			want:    []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := memVenv(tfVersionOutput)
			writeUnit(t, v, memRoot, "vpc", tc.vpcBody)
			writeUnit(t, v, memRoot, "app", dependencyBlock("../vpc"))

			if tc.gitRepo {
				require.NoError(t, v.FS.MkdirAll(filepath.Join(memRoot, ".git"), 0o755))
			}

			opts := newStackOpts(t, filepath.Join(memRoot, tc.unit), tf.CommandNameDestroy)

			units := runner.FindDependentUnits(
				config.WithCaches(t.Context()),
				thlogger.CreateLogger(),
				v,
				new(config.Store),
				opts,
				tc.cfg,
			)

			paths := make([]string, 0, len(units))
			for _, unit := range units {
				paths = append(paths, unit.Path())
			}

			slices.Sort(paths)

			assert.Equal(t, tc.want, paths)
		})
	}
}

func TestFindDependentUnits_ExcludedUnits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		units map[string]string
		name  string
		unit  string
		want  []string
	}{
		{
			name: "target excluded from destroy",
			units: map[string]string{
				"vpc": excludeBlock(`"destroy"`),
				"app": dependencyBlock("../vpc"),
			},
			unit: "vpc",
			want: []string{"app"},
		},
		{
			name: "target excluded from all actions",
			units: map[string]string{
				"vpc": excludeBlock(`"all"`),
				"app": dependencyBlock("../vpc"),
			},
			unit: "vpc",
			want: []string{"app"},
		},
		{
			name: "target excluded from all actions except output",
			units: map[string]string{
				"vpc": excludeBlock(`"all_except_output"`),
				"app": dependencyBlock("../vpc"),
			},
			unit: "vpc",
			want: []string{"app"},
		},
		{
			name: "target excluded from plan only",
			units: map[string]string{
				"vpc": excludeBlock(`"plan"`),
				"app": dependencyBlock("../vpc"),
			},
			unit: "vpc",
			want: []string{"app"},
		},
		{
			name: "dependent excluded from destroy",
			units: map[string]string{
				"vpc": "",
				"mid": dependencyBlock("../vpc") + "\n" + excludeBlock(`"destroy"`),
				"app": dependencyBlock("../mid"),
			},
			unit: "vpc",
			want: []string{"app", "mid"},
		},
		{
			name: "dependent protected by prevent_destroy",
			units: map[string]string{
				"vpc": "",
				"app": dependencyBlock("../vpc") + "\nprevent_destroy = true",
			},
			unit: "vpc",
			want: []string{"app"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := memVenv(tfVersionOutput)
			for name, body := range tc.units {
				writeUnit(t, v, memRoot, name, body)
			}

			require.NoError(t, v.FS.MkdirAll(filepath.Join(memRoot, ".git"), 0o755))

			opts := newStackOpts(t, filepath.Join(memRoot, tc.unit), tf.CommandNameDestroy)

			units := runner.FindDependentUnits(
				config.WithCaches(t.Context()),
				thlogger.CreateLogger(),
				v,
				new(config.Store),
				opts,
				&config.TerragruntConfig{},
			)

			paths := make([]string, 0, len(units))
			for _, unit := range units {
				paths = append(paths, unit.Path())
			}

			want := make([]string, 0, len(tc.want))
			for _, name := range tc.want {
				want = append(want, filepath.Join(memRoot, name))
			}

			assert.Equal(t, want, paths)
		})
	}
}

func TestFindDependentUnits_TerragruntVersionConstraint(t *testing.T) {
	t.Parallel()

	const constraint = `terragrunt_version_constraint = ">= 1.0.0"`

	v := memVenv(tfVersionOutput)
	writeUnit(t, v, memRoot, "vpc", constraint)
	writeUnit(t, v, memRoot, "app", dependencyBlock("../vpc")+"\n"+constraint)
	require.NoError(t, v.FS.MkdirAll(filepath.Join(memRoot, ".git"), 0o755))

	opts := newStackOpts(t, filepath.Join(memRoot, "vpc"), tf.CommandNameDestroy)
	opts.TerragruntVersion = semver.MustParse("1.2.3")

	units := runner.FindDependentUnits(
		config.WithCaches(t.Context()),
		thlogger.CreateLogger(),
		v,
		new(config.Store),
		opts,
		&config.TerragruntConfig{},
	)

	paths := make([]string, 0, len(units))
	for _, unit := range units {
		paths = append(paths, unit.Path())
	}

	assert.Equal(t, []string{filepath.Join(memRoot, "app")}, paths)
}

// excludeBlock returns an exclude block that always applies to the given comma-separated quoted actions.
func excludeBlock(actions string) string {
	return `exclude {
  if      = true
  actions = [` + actions + `]
}`
}
