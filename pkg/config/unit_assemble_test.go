package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestResolveRunConfigChecksTheDependenciesDirsOfEachFileItParses(t *testing.T) {
	t.Parallel()

	for _, tc := range missingDependenciesDirCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := vfs.MemRoot
			path := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
			v := venv.MemVenv().WithFS(memFS(root, tc.files)(t))
			l := logger.CreateLogger()
			ctx, run := newTestUnitParsingContext(t, path)

			_, err := (&config.Store{}).ResolveRunConfig(ctx, l, v, config.NewParseContext(v, run), path)
			require.ErrorAs(t, err, new(config.DependencyDirNotFoundError))
		})
	}
}

func TestResolveUnitQueueConfigChecksNoDependenciesDir(t *testing.T) {
	t.Parallel()

	for _, tc := range missingDependenciesDirCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := vfs.MemRoot
			path := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
			v := venv.MemVenv().WithFS(memFS(root, tc.files)(t))
			l := logger.CreateLogger()
			ctx, run := newTestUnitParsingContext(t, path)
			run.PartialParseDecodeList = []config.PartialDecodeSectionType{
				config.DependenciesBlock,
				config.DependencyBlock,
			}

			cfg, err := (&config.Store{}).ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(v, run), path)
			require.NoError(t, err)
			require.NotNil(t, cfg)
		})
	}
}

// missingDependenciesDirCase is a unit whose own file or included file lists a dependencies dir that does not exist.
type missingDependenciesDirCase struct {
	files map[string]string
	name  string
}

// missingDependenciesDirCases returns a case for a missing dependencies dir in the unit's own file, in a shallow
// merged include, and in an exposed include that does not merge.
func missingDependenciesDirCases() []missingDependenciesDirCase {
	const missingRoot = `
dependencies {
  paths = ["${get_terragrunt_dir()}/../not-generated"]
}
`

	return []missingDependenciesDirCase{
		{
			name: "own",
			files: map[string]string{
				"unit/terragrunt.hcl": `
dependencies {
  paths = ["../not-generated"]
}
`,
			},
		},
		{
			name: "shallow merged",
			files: map[string]string{
				"root.hcl": missingRoot,
				"unit/terragrunt.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
			},
		},
		{
			name: "exposed without merging",
			files: map[string]string{
				"root.hcl": missingRoot,
				"unit/terragrunt.hcl": `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  expose         = true
  merge_strategy = "no_merge"
}
`,
			},
		},
	}
}
