package config_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/template"

	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expandedConsumersStackHCL expands an environment unit over dev and prod, each depending on the
// unexpanded repo unit through its autoinclude.
const expandedConsumersStackHCL = `
unit "repo" {
  source = "` + enabledUnitSource + `"
  path   = "repo"
}

unit "environment" {
  expansion {
    for_each = toset(["dev", "prod"])
  }

  source = "` + enabledUnitSource + `"
  path   = "environment/${each.key}"

  values = {
    environment = each.key
  }

  autoinclude {
    dependency "repo" {
      config_path  = unit.repo.path
      mock_outputs = { name = "example" }
    }

    inputs = {
      repository = dependency.repo.outputs.name
    }
  }
}
`

// TestGenerateStackWritesAutoIncludePerExpandedUnit pins that every element of an expanded unit gets its
// own values and an autoinclude depending on the unexpanded unit it references.
func TestGenerateStackWritesAutoIncludePerExpandedUnit(t *testing.T) {
	t.Parallel()

	gen := generateStack(t, map[string]string{
		config.DefaultStackFile: expandedConsumersStackHCL,
	})

	for _, environment := range []string{"dev", "prod"} {
		unitDir := filepath.Join(gen.dir, "environment", environment)

		assert.Equal(t, environment, generatedValueString(t, gen.v.FS, unitDir, "environment"))

		depPaths, err := inthclparse.AutoIncludeDependencyPaths(gen.v.FS, unitDir)
		require.NoError(t, err)
		assert.Equal(t, []string{filepath.Join(gen.dir, "repo")}, depPaths)
	}
}

// TestGenerateStackDropsAutoIncludeOfOverriddenExpandedUnit pins that a sibling
// terragrunt.autoinclude.stack.hcl overriding an expanded unit replaces every element's autoinclude,
// including an element the override declares under the same key.
//
// The audit unit keeps an autoinclude in the merged config, so generation still resolves autoincludes
// from the base stack file.
func TestGenerateStackDropsAutoIncludeOfOverriddenExpandedUnit(t *testing.T) {
	t.Parallel()

	gen := generateStack(t, map[string]string{
		config.DefaultStackFile: expandedConsumersStackHCL + `
unit "audit" {
  source = "` + enabledUnitSource + `"
  path   = "audit"

  autoinclude {
    inputs = {
      audited = true
    }
  }
}
`,
		config.DefaultAutoIncludeStackFile: `
unit "environment" {
  expansion {
    for_each = toset(["dev"])
  }

  source = "` + enabledUnitSource + `"
  path   = "environment/${each.key}"
}
`,
	})

	require.True(t, gen.generated("audit", inthclparse.AutoIncludeFile))
	assert.True(t, gen.generated("environment", "dev", config.DefaultTerragruntConfigPath))
	assert.False(t, gen.generated("environment", "dev", inthclparse.AutoIncludeFile))
	assert.False(t, gen.generated("environment", "prod"))
}

// expandedDependencyStack declares dev and prod units, an env unit expanded over both, and a consumer
// unit whose autoinclude dependency expands by Expansion, resolving ConfigPath and MockName per element.
var expandedDependencyStack = template.Must(template.New("expanded-dependency").Parse(`
unit "dev" {
  source = "` + enabledUnitSource + `"
  path   = "dev"
}

unit "prod" {
  source = "` + enabledUnitSource + `"
  path   = "prod"
}

unit "env" {
  expansion {
    for_each = toset(["dev", "prod"])
  }

  source = "` + enabledUnitSource + `"
  path   = "env/${each.key}"
}

unit "consumer" {
  source = "` + enabledUnitSource + `"
  path   = "consumer"

  autoinclude {
    dependency "env" {
      expansion {
        {{.Expansion}}
      }

      config_path  = {{.ConfigPath}}
      skip_outputs = true
      mock_outputs = {
        name = {{.MockName}}
      }
      mock_outputs_allowed_terraform_commands = ["init"]
    }

    inputs = {
      names = { for key, env in dependency.env : key => env.outputs.name }
    }
  }
}
`))

// dependencyStackFields fills [expandedDependencyStack].
type dependencyStackFields struct {
	Expansion  string
	ConfigPath string
	MockName   string
}

// renderStackTemplate executes a stack file template with fields and returns the HCL it renders.
//
// A field the template names but fields lacks fails the test.
func renderStackTemplate(t *testing.T, stack *template.Template, fields any) string {
	t.Helper()

	var rendered strings.Builder

	require.NoError(t, stack.Execute(&rendered, fields))

	return rendered.String()
}

// TestGenerateStackExpandsAutoIncludeDependencyInGeneratedUnit pins that an autoinclude dependency
// declaring an expansion generates into a unit that expands it again, with every element pointing at
// the component its config_path resolved to in the stack file.
func TestGenerateStackExpandsAutoIncludeDependencyInGeneratedUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		components map[string]string
		names      map[string]any
		name       string
		expansion  string
		configPath string
		mockName   string
	}{
		{
			name:       "for_each over a set",
			expansion:  `for_each = toset(["dev", "prod"])`,
			configPath: `unit[each.key].path`,
			mockName:   `"mock-${each.key}"`,
			components: map[string]string{"dev": "dev", "prod": "prod"},
			names:      map[string]any{"dev": "mock-dev", "prod": "mock-prod"},
		},
		{
			name:       "for_each over a map",
			expansion:  `for_each = { first = "dev", second = "prod" }`,
			configPath: `unit[each.value].path`,
			mockName:   `"mock-${each.value}"`,
			components: map[string]string{"first": "dev", "second": "prod"},
			names:      map[string]any{"first": "mock-dev", "second": "mock-prod"},
		},
		{
			name:       "count",
			expansion:  `count = 2`,
			configPath: `[unit.dev.path, unit.prod.path][count.index]`,
			mockName:   `"mock-${count.index}"`,
			components: map[string]string{"0": "dev", "1": "prod"},
			names:      map[string]any{"0": "mock-0", "1": "mock-1"},
		},
		{
			name:       "keyed ref to an expanded unit",
			expansion:  `for_each = toset(["dev", "prod"])`,
			configPath: `unit.env[each.key].path`,
			mockName:   `"mock-${each.key}"`,
			components: map[string]string{
				"dev":  filepath.Join("env", "dev"),
				"prod": filepath.Join("env", "prod"),
			},
			names: map[string]any{"dev": "mock-dev", "prod": "mock-prod"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gen := generateStack(t, map[string]string{
				config.DefaultStackFile: renderStackTemplate(
					t,
					expandedDependencyStack,
					dependencyStackFields{
						Expansion:  tc.expansion,
						ConfigPath: tc.configPath,
						MockName:   tc.mockName,
					},
				),
			})

			unitDir := filepath.Join(gen.dir, "consumer")
			cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

			ctx, pctx := newTestParsingContext(t, gen.v, cfgPath)
			pctx.OriginalTerraformCommand = tfInitCommand

			parsed, err := config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), cfgPath, nil)
			require.NoError(t, err)

			assert.Equal(t, tc.names, parsed.Inputs["names"])

			components := make(map[string]string, len(parsed.TerragruntDependencies))

			for _, dep := range parsed.TerragruntDependencies {
				require.NotNil(t, dep.Expansion)

				rel, err := filepath.Rel(
					gen.dir,
					filepath.Join(unitDir, dep.ConfigPath.AsString()),
				)
				require.NoError(t, err)

				components[dep.Expansion.Key()] = rel
			}

			assert.Equal(t, tc.components, components)
		})
	}
}

// TestReadStackConfigResolvesKeyedUnitRef pins that a values attribute reaches one element of an
// expanded unit through unit.<name>[key].path.
func TestReadStackConfigResolvesKeyedUnitRef(t *testing.T) {
	t.Parallel()

	stackCfg, err := parseStackString(t, `
unit "env" {
  expansion {
    for_each = toset(["dev", "prod"])
  }

  source = "./units/env"
  path   = "env/${each.key}"
}

unit "app" {
  source = "./units/app"
  path   = "app"

  values = {
    dev_path = unit.env["dev"].path
  }
}
`)
	require.NoError(t, err)

	idx := slices.IndexFunc(stackCfg.Units, func(unit *config.Unit) bool {
		return unit.Name == "app"
	})
	require.NotEqual(t, -1, idx)
	require.NotNil(t, stackCfg.Units[idx].Values)

	assert.Equal(
		t,
		filepath.Join(config.StackDir, "env", "dev"),
		stackCfg.Units[idx].Values.GetAttr("dev_path").AsString(),
	)
}

// TestReadStackConfigRejectsExpandedAndUnexpandedUnitSharingALabel pins that a unit label declared
// both with and without an expansion fails the stack parse, since unit.<name> cannot refer to both.
func TestReadStackConfigRejectsExpandedAndUnexpandedUnitSharingALabel(t *testing.T) {
	t.Parallel()

	err := parseStackErr(t, `
unit "env" {
  source = "./units/env"
  path   = "env"
}

unit "env" {
  expansion {
    for_each = toset(["dev"])
  }

  source = "./units/env"
  path   = "env/${each.key}"
}
`)

	var collision inthclparse.ComponentRefCollisionError
	require.ErrorAs(t, err, &collision)
	assert.Equal(t, inthclparse.VarUnit, collision.Root)
	assert.Equal(t, "env", collision.Name)
}

// generatedValueString returns the string value named key from the terragrunt.values.hcl generated
// in unitDir.
func generatedValueString(t *testing.T, fs vfs.FS, unitDir, key string) string {
	t.Helper()

	content, err := vfs.ReadFile(fs, filepath.Join(unitDir, "terragrunt.values.hcl"))
	require.NoError(t, err)

	file, diags := hclsyntax.ParseConfig(content, "terragrunt.values.hcl", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())

	attrs, diags := file.Body.JustAttributes()
	require.False(t, diags.HasErrors(), diags.Error())

	value, diags := attrs[key].Expr.Value(nil)
	require.False(t, diags.HasErrors(), diags.Error())

	return value.AsString()
}
