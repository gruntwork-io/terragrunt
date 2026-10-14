package runner_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/runner"
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

			units := runner.FindDependentUnits(t.Context(), thlogger.CreateLogger(), v, opts, tc.cfg)

			paths := make([]string, 0, len(units))
			for _, unit := range units {
				paths = append(paths, unit.Path())
			}

			slices.Sort(paths)

			assert.Equal(t, tc.want, paths)
		})
	}
}
