package config_test

import (
	"path/filepath"
	"testing"

	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/function"
)

// TestStackFileParsersAgreeOnComponents pins that discovery and the autoinclude parser read the
// same components out of a stack directory as the full stack parse.
//
// Discovery must report the generated path of every enabled component, since a dependency on the
// stack directory waits on exactly those paths. The autoinclude parser must report the address of
// every component, since generation looks up each component's autoinclude by that address.
func TestStackFileParsersAgreeOnComponents(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files map[string]string
		name  string
	}{
		{
			name: "unexpanded unit and stack",
			files: map[string]string{
				config.DefaultStackFile: `
unit "vpc" {
  source = "./units/app"
  path   = "vpc"
}

stack "team" {
  source = "./stacks/team"
  path   = "team"
}
`,
			},
		},
		{
			name: "for_each unit",
			files: map[string]string{
				config.DefaultStackFile: `
unit "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  source = "./units/app"
  path   = "aurora/${each.key}"
}
`,
			},
		},
		{
			name: "count unit",
			files: map[string]string{
				config.DefaultStackFile: `
unit "shard" {
  expansion {
    count = 2
  }

  source = "./units/app"
  path   = "shard/${count.index}"
}
`,
			},
		},
		{
			name: "count stack",
			files: map[string]string{
				config.DefaultStackFile: `
stack "team" {
  expansion {
    count = 2
  }

  source = "./stacks/team"
  path   = "team/${count.index}"
}
`,
			},
		},
		{
			name: "empty for_each",
			files: map[string]string{
				config.DefaultStackFile: `
unit "aurora" {
  expansion {
    for_each = toset([])
  }

  source = "./units/app"
  path   = "aurora/${each.key}"
}
`,
			},
		},
		{
			name: "for_each over locals",
			files: map[string]string{
				config.DefaultStackFile: `
locals {
  regions = toset(["east", "west"])
}

unit "vpc" {
  expansion {
    for_each = local.regions
  }

  source = "./units/app"
  path   = "vpc/${each.key}"
}
`,
			},
		},
		{
			name: "for_each over values",
			files: map[string]string{
				"terragrunt.values.hcl": `regions = ["east", "west"]`,
				config.DefaultStackFile: `
unit "vpc" {
  expansion {
    for_each = toset(values.regions)
  }

  source = "./units/app"
  path   = "vpc/${each.key}"
}
`,
			},
		},
		{
			name: "expanded unit without .terragrunt-stack",
			files: map[string]string{
				config.DefaultStackFile: `
unit "vpc" {
  expansion {
    for_each = toset(["east", "west"])
  }

  no_dot_terragrunt_stack = true

  source = "./units/app"
  path   = "vpc-${each.key}"
}
`,
			},
		},
		{
			name: "expanded unit with autoinclude",
			files: map[string]string{
				config.DefaultStackFile: `
unit "repo" {
  source = "./units/app"
  path   = "repo"
}

unit "environment" {
  expansion {
    for_each = toset(["dev", "prod"])
  }

  source = "./units/app"
  path   = "environment/${each.key}"

  autoinclude {
    dependency "repo" {
      config_path = unit.repo.path
    }

    inputs = {
      environment = each.key
    }
  }
}
`,
			},
		},
		{
			name: "keyed unit reference",
			files: map[string]string{
				config.DefaultStackFile: `
unit "vpc" {
  expansion {
    for_each = toset(["east", "west"])
  }

  source = "./units/app"
  path   = "vpc/${each.key}"
}

unit "app" {
  source = "./units/app"
  path   = "app"

  values = {
    vpc_path = unit.vpc["east"].path
  }
}
`,
			},
		},
		{
			name: "expanded unit in included stack file",
			files: map[string]string{
				config.DefaultStackFile: `
include "shared" {
  path = "./shared.hcl"
}
`,
				"shared.hcl": `
unit "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  source = "./units/app"
  path   = "aurora/${each.key}"
}
`,
			},
		},
		{
			name: "disabled unit",
			files: map[string]string{
				config.DefaultStackFile: `
unit "vpc" {
  source = "./units/app"
  path   = "vpc"
}

unit "legacy" {
  enabled = false

  source = "./units/app"
  path   = "legacy"
}
`,
			},
		},
		{
			name: "enabled per element",
			files: map[string]string{
				config.DefaultStackFile: `
unit "shard" {
  expansion {
    for_each = toset(["web", "api"])
  }

  enabled = each.key != "api"

  source = "./units/app"
  path   = "shard/${each.key}"
}
`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, dir := newMemTestDir(t)

			for path, body := range tc.files {
				require.NoError(
					t,
					vfs.WriteFile(v.FS, filepath.Join(dir, path), []byte(body), 0o644),
				)
			}

			stackPath := filepath.Join(dir, config.DefaultStackFile)
			l := logger.CreateLogger()
			ctx, pctx := newTestParsingContext(t, v, stackPath)

			values, err := config.ReadValues(ctx, pctx, l, dir)
			require.NoError(t, err)

			stack, err := config.ReadStackConfigFile(ctx, l, pctx, stackPath, values)
			require.NoError(t, err)

			unitPaths, stackPaths, err := inthclparse.DirectComponentPaths(
				v.FS,
				dir,
				func(stackDir string) (map[string]function.Function, error) {
					return config.EarlyStackParseFunctions(ctx, l, stackDir, pctx)
				},
			)
			require.NoError(t, err)

			wantUnitPaths, wantStackPaths := enabledComponentPaths(stack, dir)
			assert.ElementsMatch(t, wantUnitPaths, unitPaths, "discovery unit paths")
			assert.ElementsMatch(t, wantStackPaths, stackPaths, "discovery stack paths")

			src, err := vfs.ReadFile(v.FS, stackPath)
			require.NoError(t, err)

			funcs, err := config.EarlyStackParseFunctions(ctx, l, dir, pctx)
			require.NoError(t, err)

			parsed, err := inthclparse.ParseStackFile(v.FS, &inthclparse.ParseStackFileInput{
				Values:    values,
				Functions: funcs,
				Filename:  config.DefaultStackFile,
				StackDir:  dir,
				Src:       src,
			})
			require.NoError(t, err)

			wantUnitAddresses, wantStackAddresses := componentAddresses(stack)
			gotUnitAddresses, gotStackAddresses := parsedComponentAddresses(parsed)
			assert.ElementsMatch(
				t,
				wantUnitAddresses,
				gotUnitAddresses,
				"autoinclude parser unit addresses",
			)
			assert.ElementsMatch(
				t,
				wantStackAddresses,
				gotStackAddresses,
				"autoinclude parser stack addresses",
			)
		})
	}
}

// enabledComponentPaths returns the generated path under dir of every enabled unit and stack in
// stack.
func enabledComponentPaths(stack *config.StackConfig, dir string) (units, stacks []string) {
	units = make([]string, 0, len(stack.Units))
	stacks = make([]string, 0, len(stack.Stacks))

	for _, u := range stack.Units {
		if u.IsEnabled() {
			units = append(units, u.GeneratedPath(dir))
		}
	}

	for _, s := range stack.Stacks {
		if s.IsEnabled() {
			stacks = append(stacks, s.GeneratedPath(dir))
		}
	}

	return units, stacks
}

// componentAddresses returns the address of every unit and stack in stack, enabled or not.
func componentAddresses(stack *config.StackConfig) (units, stacks []string) {
	units = make([]string, 0, len(stack.Units))
	stacks = make([]string, 0, len(stack.Stacks))

	for _, u := range stack.Units {
		units = append(units, componentAddress(u.Name, u.Expansion))
	}

	for _, s := range stack.Stacks {
		stacks = append(stacks, componentAddress(s.Name, s.Expansion))
	}

	return units, stacks
}

// componentAddress returns the address of the component labeled name, given the expansion block
// it decoded with, which is nil for a block that declares none.
func componentAddress(name string, expansion *hclparse.ExpansionBlock) string {
	if expansion == nil {
		return name
	}

	return expansion.Address(name)
}

// parsedComponentAddresses returns the address of every unit and stack the autoinclude parser read.
func parsedComponentAddresses(parsed *inthclparse.ParseResult) (units, stacks []string) {
	units = make([]string, 0, len(parsed.Units))
	stacks = make([]string, 0, len(parsed.Stacks))

	for _, u := range parsed.Units {
		units = append(units, u.Address())
	}

	for _, s := range parsed.Stacks {
		stacks = append(stacks, s.Address())
	}

	return units, stacks
}
