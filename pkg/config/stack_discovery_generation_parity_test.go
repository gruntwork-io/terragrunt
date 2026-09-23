package config_test

import (
	"io/fs"
	"path/filepath"
	"testing"

	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/function"
)

const generationParityMaxStackDepth = 5

var (
	generationParityLiveDir = venvtest.Root("/virtual/live")

	// The sources are interpolated into HCL strings, where a backslash starts an escape, so they
	// keep forward slashes. filepath.Join converts them wherever they name a file.
	generationParityUnitSource  = filepath.ToSlash(venvtest.Root("/virtual/catalog/units/app"))
	generationParityStackSource = filepath.ToSlash(venvtest.Root("/virtual/catalog/stacks/team"))
)

// TestUnitPathsFromStackDirAgreesWithGeneration pins that discovery reports exactly the units stack
// generation writes, through generated nested stacks and sibling terragrunt.autoinclude.stack.hcl
// files.
func TestUnitPathsFromStackDirAgreesWithGeneration(t *testing.T) {
	t.Parallel()

	liveStackFile := filepath.Join(generationParityLiveDir, config.DefaultStackFile)
	liveAutoIncludeFile := filepath.Join(
		generationParityLiveDir,
		config.DefaultAutoIncludeStackFile,
	)
	teamStackFile := filepath.Join(generationParityStackSource, config.DefaultStackFile)

	testCases := []struct {
		files map[string]string
		name  string
	}{
		{
			name: "nested stack expanding over the parent's each.value",
			files: map[string]string{
				liveStackFile: `
stack "team" {
  expansion {
    for_each = { a = ["x", "y"], b = ["z"] }
  }

  source = "` + generationParityStackSource + `"
  path   = "team/${each.key}"

  values = {
    members = each.value
  }
}
`,
				teamStackFile: `
unit "member" {
  expansion {
    for_each = toset(values.members)
  }

  source = "` + generationParityUnitSource + `"
  path   = "member/${each.key}"
}
`,
			},
		},
		{
			name: "disabled nested stack",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  source = "` + generationParityUnitSource + `"
  path   = "vpc"
}

stack "team" {
  enabled = false

  source = "` + generationParityStackSource + `"
  path   = "team"
}
`,
				teamStackFile: `
unit "member" {
  source = "` + generationParityUnitSource + `"
  path   = "member"
}
`,
			},
		},
		{
			name: "nested unit without .terragrunt-stack",
			files: map[string]string{
				liveStackFile: `
stack "team" {
  expansion {
    count = 2
  }

  source = "` + generationParityStackSource + `"
  path   = "team/${count.index}"
}
`,
				teamStackFile: `
unit "member" {
  no_dot_terragrunt_stack = true

  source = "` + generationParityUnitSource + `"
  path   = "member"
}
`,
			},
		},
		{
			name: "sibling stack autoinclude disables a unit",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  source = "` + generationParityUnitSource + `"
  path   = "vpc"
}

unit "legacy" {
  source = "` + generationParityUnitSource + `"
  path   = "legacy"
}
`,
				liveAutoIncludeFile: `
unit "legacy" {
  enabled = false

  source = "` + generationParityUnitSource + `"
  path   = "legacy"
}
`,
			},
		},
		{
			name: "sibling stack autoinclude replaces an expanded unit",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  expansion {
    for_each = toset(["east", "west"])
  }

  source = "` + generationParityUnitSource + `"
  path   = "vpc/${each.key}"
}
`,
				liveAutoIncludeFile: `
unit "vpc" {
  source = "` + generationParityUnitSource + `"
  path   = "vpc"
}
`,
			},
		},
		{
			name: "sibling stack autoinclude injects a unit",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  source = "` + generationParityUnitSource + `"
  path   = "vpc"
}
`,
				liveAutoIncludeFile: `
unit "extra" {
  source = "` + generationParityUnitSource + `"
  path   = "extra"
}
`,
			},
		},
		{
			name: "parent stack autoinclude injects a unit",
			files: map[string]string{
				liveStackFile: `
stack "team" {
  source = "` + generationParityStackSource + `"
  path   = "team"

  autoinclude {
    unit "extra" {
      source = "` + generationParityUnitSource + `"
      path   = "extra"
    }
  }
}
`,
				teamStackFile: `
unit "member" {
  source = "` + generationParityUnitSource + `"
  path   = "member"
}
`,
			},
		},
		{
			name: "parent stack autoinclude disables a nested unit",
			files: map[string]string{
				liveStackFile: `
stack "team" {
  source = "` + generationParityStackSource + `"
  path   = "team"

  autoinclude {
    unit "member" {
      enabled = false

      source = "` + generationParityUnitSource + `"
      path   = "member"
    }
  }
}
`,
				teamStackFile: `
unit "lead" {
  source = "` + generationParityUnitSource + `"
  path   = "lead"
}

unit "member" {
  source = "` + generationParityUnitSource + `"
  path   = "member"
}
`,
			},
		},
		{
			name: "expanded parent stack autoinclude injects a unit per element",
			files: map[string]string{
				liveStackFile: `
stack "team" {
  expansion {
    for_each = toset(["a", "b"])
  }

  source = "` + generationParityStackSource + `"
  path   = "team/${each.key}"

  autoinclude {
    unit "extra" {
      source = "` + generationParityUnitSource + `"
      path   = "extra-${each.key}"
    }
  }
}
`,
				teamStackFile: `
unit "member" {
  source = "` + generationParityUnitSource + `"
  path   = "member"
}
`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := generateStackTree(t, tc.files)

			configPaths := filesNamed(
				t,
				v.FS,
				generationParityLiveDir,
				config.DefaultTerragruntConfigPath,
			)
			require.NotEmpty(t, configPaths)

			generatedUnitPaths := make([]string, 0, len(configPaths))
			for _, configPath := range configPaths {
				generatedUnitPaths = append(generatedUnitPaths, filepath.Dir(configPath))
			}

			l := logger.CreateLogger()
			ctx, pctx := newTestParsingContext(t, v, liveStackFile)

			unitPaths, err := inthclparse.UnitPathsFromStackDir(
				ctx,
				v.FS,
				generationParityLiveDir,
				&inthclparse.StackDirArgs{
					FuncsFor: func(stackDir string) (map[string]function.Function, error) {
						return config.EarlyStackParseFunctions(ctx, l, stackDir, pctx)
					},
				},
			)
			require.NoError(t, err)
			assert.ElementsMatch(t, generatedUnitPaths, unitPaths)
		})
	}
}

// generateStackTree writes files into an in-memory filesystem beside a unit source at
// generationParityUnitSource, then generates every stack file under generationParityLiveDir, one
// level at a time until a pass finds no stack file it has not generated.
//
// Fails the test when the tree nests deeper than generationParityMaxStackDepth.
func generateStackTree(t *testing.T, files map[string]string) *venv.Venv {
	t.Helper()

	v := venvtest.New()

	for _, name := range []string{"main.tf", config.DefaultTerragruntConfigPath} {
		require.NoError(t, vfs.WriteFile(
			v.FS,
			filepath.Join(generationParityUnitSource, name),
			nil,
			0o644,
		))
	}

	for path, body := range files {
		require.NoError(t, vfs.WriteFile(v.FS, path, []byte(body), 0o644))
	}

	generated := make(map[string]struct{})

	for range generationParityMaxStackDepth {
		progressed := false

		for _, stackPath := range filesNamed(t, v.FS, generationParityLiveDir, config.DefaultStackFile) {
			if _, done := generated[stackPath]; done {
				continue
			}

			generateStackLevel(t, v, stackPath)

			generated[stackPath] = struct{}{}
			progressed = true
		}

		if !progressed {
			return v
		}
	}

	require.FailNow(
		t,
		"stack tree nests deeper than the generation bound",
		"bound: %d",
		generationParityMaxStackDepth,
	)

	return nil
}

// generateStackLevel generates the units and stacks stackPath declares, without recursing into the
// stacks it generates.
func generateStackLevel(t *testing.T, v *venv.Venv, stackPath string) {
	t.Helper()

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, v, stackPath)

	pctx.TerragruntStackConfigPath = stackPath
	pctx.NoCAS = true

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	require.NoError(t, config.GenerateStackFile(ctx, l, pctx, pool, stackPath))
	require.NoError(t, pool.Wait())
}

// filesNamed returns the path of every file named name under root.
func filesNamed(t *testing.T, fsys vfs.FS, root, name string) []string {
	t.Helper()

	var paths []string

	require.NoError(
		t,
		vfs.WalkDir(fsys, root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !entry.IsDir() && entry.Name() == name {
				paths = append(paths, path)
			}

			return nil
		}),
	)

	return paths
}
