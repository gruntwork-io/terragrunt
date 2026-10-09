package config_test

import (
	"path/filepath"
	"testing"

	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/function"
)

var generationParityWrapSource = filepath.ToSlash(venvtest.Root("/virtual/catalog/stacks/wrap"))

// TestGenerateStackTreeWiresAutoIncludeDependencies pins, for each way a component can reach a
// generated stack tree, which units generation writes and which directories each unit's autoinclude
// depends on. Discovery must report the same units.
//
// The cases combine the component kind, whether a stack-level autoinclude injects or overrides it,
// whether it is expanded, whether it declares its own autoinclude, and how deep it nests.
func TestGenerateStackTreeWiresAutoIncludeDependencies(t *testing.T) {
	t.Parallel()

	liveStackFile := filepath.Join(generationParityLiveDir, config.DefaultStackFile)
	liveAutoIncludeFile := filepath.Join(
		generationParityLiveDir,
		config.DefaultAutoIncludeStackFile,
	)
	teamStackFile := filepath.Join(generationParityStackSource, config.DefaultStackFile)
	wrapStackFile := filepath.Join(generationParityWrapSource, config.DefaultStackFile)

	unitSource := `source = "` + generationParityUnitSource + `"`
	teamSource := `source = "` + generationParityStackSource + `"`
	wrapSource := `source = "` + generationParityWrapSource + `"`

	const gen = config.StackDir

	// teamWithWiredMembers is a catalog stack whose member unit depends on its lead unit.
	teamWithWiredMembers := `
unit "lead" {
  ` + unitSource + `
  path = "lead"
}

unit "member" {
  ` + unitSource + `
  path = "member"

  autoinclude {
    dependency "lead" {
      config_path = unit.lead.path
    }
  }
}
`

	testCases := []struct {
		files map[string]string
		// units maps every generated unit, relative to the live directory, to the directories its
		// autoinclude depends on, also relative to the live directory.
		units map[string][]string
		name  string
	}{
		{
			name: "unit depends on a sibling unit",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}

unit "app" {
  ` + unitSource + `
  path = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }
  }
}
`,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc"): nil,
				filepath.Join(gen, "app"): {filepath.Join(gen, "vpc")},
			},
		},
		{
			name: "unit depends on a sibling unit outside .terragrunt-stack",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"

  no_dot_terragrunt_stack = true
}

unit "app" {
  ` + unitSource + `
  path = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }
  }
}
`,
			},
			units: map[string][]string{
				"vpc":                     nil,
				filepath.Join(gen, "app"): {"vpc"},
			},
		},
		{
			name: "unit depends on a sibling stack",
			files: map[string]string{
				liveStackFile: `
stack "team" {
  ` + teamSource + `
  path = "team"
}

unit "app" {
  ` + unitSource + `
  path = "app"

  autoinclude {
    dependency "team" {
      config_path = stack.team.path
    }
  }
}
`,
				teamStackFile: teamWithWiredMembers,
			},
			units: map[string][]string{
				filepath.Join(gen, "app"):                 {filepath.Join(gen, "team")},
				filepath.Join(gen, "team", gen, "lead"):   nil,
				filepath.Join(gen, "team", gen, "member"): {filepath.Join(gen, "team", gen, "lead")},
			},
		},
		{
			name: "expanded unit depends on a sibling unit per element",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}

unit "app" {
  expansion {
    for_each = toset(["a", "b"])
  }

  ` + unitSource + `
  path = "app/${each.key}"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }
  }
}
`,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc"):      nil,
				filepath.Join(gen, "app", "a"): {filepath.Join(gen, "vpc")},
				filepath.Join(gen, "app", "b"): {filepath.Join(gen, "vpc")},
			},
		},
		{
			name: "unit depends on one element of an expanded sibling unit",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  expansion {
    for_each = toset(["a", "b"])
  }

  ` + unitSource + `
  path = "vpc/${each.key}"
}

unit "app" {
  ` + unitSource + `
  path = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc["b"].path
    }
  }
}
`,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc", "a"): nil,
				filepath.Join(gen, "vpc", "b"): nil,
				filepath.Join(gen, "app"):      {filepath.Join(gen, "vpc", "b")},
			},
		},
		{
			name: "unit in an included stack file depends on a unit of the including file",
			files: map[string]string{
				liveStackFile: `
include "shared" {
  path = "./shared.hcl"
}

unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}
`,
				filepath.Join(generationParityLiveDir, "shared.hcl"): `
unit "app" {
  ` + unitSource + `
  path = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }
  }
}
`,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc"): nil,
				filepath.Join(gen, "app"): {filepath.Join(gen, "vpc")},
			},
		},
		{
			name: "injected unit depends on a unit of the injecting stack file",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}

stack "team" {
  ` + teamSource + `
  path = "team"

  autoinclude {
    unit "extra" {
      ` + unitSource + `
      path = "extra"

      autoinclude {
        dependency "vpc" {
          config_path = unit.vpc.path
        }
      }
    }
  }
}
`,
				teamStackFile: teamWithWiredMembers,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc"):                 nil,
				filepath.Join(gen, "team", gen, "lead"):   nil,
				filepath.Join(gen, "team", gen, "member"): {filepath.Join(gen, "team", gen, "lead")},
				filepath.Join(gen, "team", gen, "extra"):  {filepath.Join(gen, "vpc")},
			},
		},
		{
			name: "injected unit outside .terragrunt-stack depends on a unit of the injecting stack file",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}

stack "team" {
  ` + teamSource + `
  path = "team"

  autoinclude {
    unit "extra" {
      ` + unitSource + `
      path = "extra"

      no_dot_terragrunt_stack = true

      autoinclude {
        dependency "vpc" {
          config_path = unit.vpc.path
        }
      }
    }
  }
}
`,
				teamStackFile: teamWithWiredMembers,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc"):                 nil,
				filepath.Join(gen, "team", gen, "lead"):   nil,
				filepath.Join(gen, "team", gen, "member"): {filepath.Join(gen, "team", gen, "lead")},
				filepath.Join(gen, "team", "extra"):       {filepath.Join(gen, "vpc")},
			},
		},
		{
			name: "overriding unit moves and the nested stack's own dependency follows it",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}

stack "team" {
  ` + teamSource + `
  path = "team"

  autoinclude {
    unit "lead" {
      ` + unitSource + `
      path = "captain"

      autoinclude {
        dependency "vpc" {
          config_path = unit.vpc.path
        }
      }
    }
  }
}
`,
				teamStackFile: teamWithWiredMembers,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc"):                  nil,
				filepath.Join(gen, "team", gen, "captain"): {filepath.Join(gen, "vpc")},
				filepath.Join(gen, "team", gen, "member"):  {filepath.Join(gen, "team", gen, "captain")},
			},
		},
		{
			name: "overriding unit without autoinclude drops the overridden unit's dependency",
			files: map[string]string{
				liveStackFile: `
stack "team" {
  ` + teamSource + `
  path = "team"

  autoinclude {
    unit "member" {
      ` + unitSource + `
      path = "member"
    }
  }
}
`,
				teamStackFile: teamWithWiredMembers,
			},
			units: map[string][]string{
				filepath.Join(gen, "team", gen, "lead"):   nil,
				filepath.Join(gen, "team", gen, "member"): nil,
			},
		},
		{
			name: "overriding unit replaces a unit declared in an included stack file",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}

stack "team" {
  ` + teamSource + `
  path = "team"

  autoinclude {
    unit "member" {
      ` + unitSource + `
      path = "member"

      autoinclude {
        dependency "vpc" {
          config_path = unit.vpc.path
        }
      }
    }
  }
}
`,
				teamStackFile: `
include "shared" {
  path = "./shared.hcl"
}
`,
				filepath.Join(generationParityStackSource, "shared.hcl"): teamWithWiredMembers,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc"):                 nil,
				filepath.Join(gen, "team", gen, "lead"):   nil,
				filepath.Join(gen, "team", gen, "member"): {filepath.Join(gen, "vpc")},
			},
		},
		{
			name: "expanded stack injects a unit that depends on the injecting stack file per element",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}

stack "team" {
  expansion {
    for_each = toset(["a", "b"])
  }

  ` + teamSource + `
  path = "team/${each.key}"

  autoinclude {
    unit "extra" {
      ` + unitSource + `
      path = "extra-${each.key}"

      autoinclude {
        dependency "vpc" {
          config_path = unit.vpc.path
        }
      }
    }
  }
}
`,
				teamStackFile: `
unit "lead" {
  ` + unitSource + `
  path = "lead"
}
`,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc"):                       nil,
				filepath.Join(gen, "team", "a", gen, "lead"):    nil,
				filepath.Join(gen, "team", "a", gen, "extra-a"): {filepath.Join(gen, "vpc")},
				filepath.Join(gen, "team", "b", gen, "lead"):    nil,
				filepath.Join(gen, "team", "b", gen, "extra-b"): {filepath.Join(gen, "vpc")},
			},
		},
		{
			name: "injected stack injects a unit two levels down",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}

stack "wrap" {
  ` + wrapSource + `
  path = "wrap"

  autoinclude {
    stack "team" {
      ` + teamSource + `
      path = "team"

      autoinclude {
        unit "lead" {
          ` + unitSource + `
          path = "captain"

          autoinclude {
            dependency "vpc" {
              config_path = unit.vpc.path
            }
          }
        }
      }
    }
  }
}
`,
				wrapStackFile: `
stack "team" {
  ` + teamSource + `
  path = "team"
}
`,
				teamStackFile: teamWithWiredMembers,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc"): nil,
				filepath.Join(gen, "wrap", gen, "team", gen, "captain"): {
					filepath.Join(gen, "vpc"),
				},
				filepath.Join(gen, "wrap", gen, "team", gen, "member"): {
					filepath.Join(gen, "wrap", gen, "team", gen, "captain"),
				},
			},
		},
		{
			name: "sibling stack autoinclude file overrides a unit with an expanded unit",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}

unit "app" {
  ` + unitSource + `
  path = "app"

  autoinclude {
    dependency "a" {
      config_path = unit.vpc["a"].path
    }

    dependency "b" {
      config_path = unit.vpc["b"].path
    }
  }
}
`,
				liveAutoIncludeFile: `
unit "vpc" {
  expansion {
    for_each = toset(["a", "b"])
  }

  ` + unitSource + `
  path = "vpc/${each.key}"
}
`,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc", "a"): nil,
				filepath.Join(gen, "vpc", "b"): nil,
				filepath.Join(gen, "app"): {
					filepath.Join(gen, "vpc", "a"),
					filepath.Join(gen, "vpc", "b"),
				},
			},
		},
		{
			name: "sibling stack autoinclude file overrides an expanded unit and drops every element's dependency",
			files: map[string]string{
				liveStackFile: `
unit "vpc" {
  ` + unitSource + `
  path = "vpc"
}

unit "app" {
  expansion {
    for_each = toset(["a", "b"])
  }

  ` + unitSource + `
  path = "app/${each.key}"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }
  }
}
`,
				liveAutoIncludeFile: `
unit "app" {
  ` + unitSource + `
  path = "app"
}
`,
			},
			units: map[string][]string{
				filepath.Join(gen, "vpc"): nil,
				filepath.Join(gen, "app"): nil,
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

			generated := make([]string, 0, len(configPaths))
			for _, configPath := range configPaths {
				generated = append(generated, filepath.Dir(configPath))
			}

			want := make([]string, 0, len(tc.units))
			for unit := range tc.units {
				want = append(want, filepath.Join(generationParityLiveDir, unit))
			}

			require.ElementsMatch(t, want, generated, "generated units")

			for unit, deps := range tc.units {
				wantDeps := make([]string, 0, len(deps))
				for _, dep := range deps {
					wantDeps = append(wantDeps, filepath.Join(generationParityLiveDir, dep))
				}

				gotDeps, err := inthclparse.AutoIncludeDependencyPaths(
					v.FS,
					filepath.Join(generationParityLiveDir, unit),
				)
				require.NoError(t, err)
				assert.ElementsMatch(t, wantDeps, gotDeps, "dependencies of %s", unit)
			}

			l := logger.CreateLogger()
			ctx, pctx := newTestParsingContext(t, liveStackFile)

			discovered, err := inthclparse.UnitPathsFromStackDir(
				ctx,
				v.FS,
				generationParityLiveDir,
				&inthclparse.StackDirArgs{
					FuncsFor: func(stackDir string) (map[string]function.Function, error) {
						return config.EarlyStackParseFunctions(ctx, l, v, pctx, stackDir)
					},
				},
			)
			require.NoError(t, err)
			assert.ElementsMatch(t, want, discovered, "discovered units")
		})
	}
}
