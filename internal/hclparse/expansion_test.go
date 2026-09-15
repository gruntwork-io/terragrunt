package hclparse_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"text/template"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expandedEnvironmentStack declares an unexpanded repo unit and an environment unit expanded by
// Expansion, whose autoinclude depends on repo and passes Element through its inputs.
var expandedEnvironmentStack = template.Must(template.New("expanded-environment").Parse(`
unit "repo" {
  source = "../catalog/units/repo"
  path   = "repo"
}

unit "environment" {
  expansion {
    {{.Expansion}}
  }

  source = "../catalog/units/environment"
  path   = "environment/{{.Element}}"

  autoinclude {
    dependency "repo" {
      config_path = unit.repo.path
    }

    inputs = {
      environment = "{{.Element}}"
    }
  }
}
`))

// environmentStackFields fills [expandedEnvironmentStack].
type environmentStackFields struct {
	Expansion string
	Element   string
}

// expandedDependencyStack declares dev and prod units, an env unit expanded over both, and a consumer
// unit whose autoinclude dependency expands by Expansion and resolves ConfigPath per element.
var expandedDependencyStack = template.Must(template.New("expanded-dependency").Parse(`
unit "dev" {
  source = "../catalog/units/env"
  path   = "dev"
}

unit "prod" {
  source = "../catalog/units/env"
  path   = "prod"
}

unit "env" {
  expansion {
    for_each = { dev = "dev", prod = "prod" }
  }

  source = "../catalog/units/env"
  path   = "env/${each.key}"
}

unit "consumer" {
  source = "../catalog/units/consumer"
  path   = "consumer"

  autoinclude {
    dependency "env" {
      expansion {
        {{.Expansion}}
      }

      config_path = {{.ConfigPath}}
    }
  }
}
`))

// dependencyStackFields fills [expandedDependencyStack].
type dependencyStackFields struct {
	Expansion  string
	ConfigPath string
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

// TestParseStackFileResolvesAutoIncludePerExpandedInstance pins that an expanded unit resolves its
// autoinclude once per element, keyed by the element's address and evaluated with that element bound.
func TestParseStackFileResolvesAutoIncludePerExpandedInstance(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		elements  map[string]string
		name      string
		expansion string
		element   string
	}{
		{
			name:      "for_each",
			expansion: `for_each = { dev = "dev", prod = "prod" }`,
			element:   "env-${each.key}",
			elements: map[string]string{
				"environment[dev]":  "env-dev",
				"environment[prod]": "env-prod",
			},
		},
		{
			name:      "count",
			expansion: `count = 2`,
			element:   "env-${count.index}",
			elements:  map[string]string{"environment[0]": "env-0", "environment[1]": "env-1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := renderStackTemplate(t, expandedEnvironmentStack, environmentStackFields{
				Expansion: tc.expansion,
				Element:   tc.element,
			})

			fs := vfs.NewMemMapFS()

			result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
				Src:      []byte(src),
				Filename: "terragrunt.stack.hcl",
				StackDir: testStackDir,
			})
			require.NoError(t, err)
			require.Len(t, result.Units, 1+len(tc.elements))
			require.Len(t, result.AutoIncludes, len(tc.elements))

			for address, element := range tc.elements {
				resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey(hclparse.KindUnit, address)]
				require.True(t, ok, address)
				require.Len(t, resolved.Dependencies, 1)
				assert.Equal(
					t,
					hclparse.SingleConfigPath(
						filepath.Join(testStackDir, hclparse.StackDir, "repo"),
					),
					resolved.Dependencies[0].ConfigPath,
				)

				unitDir := filepath.Join(testStackDir, hclparse.StackDir, "environment", element)
				require.NoError(t, hclparse.GenerateAutoIncludeFile(
					fs,
					resolved,
					unitDir,
					resolved.SourceBytes,
					resolved.EvalCtx,
				))

				assert.Equal(t, element, generatedInputString(t, fs, unitDir, "environment"))
			}
		})
	}
}

// TestParseStackFileResolvesExpandedAutoIncludeDependency pins that an autoinclude dependency declaring
// an expansion resolves config_path once per element, in that element's eval context.
func TestParseStackFileResolvesExpandedAutoIncludeDependency(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		components map[string]string
		name       string
		expansion  string
		configPath string
		metaArg    pkghclparse.MetaArg
	}{
		{
			name:       "for_each",
			expansion:  `for_each = { first = "dev", second = "prod" }`,
			configPath: `unit[each.value].path`,
			metaArg:    pkghclparse.MetaArgForEach,
			components: map[string]string{"first": "dev", "second": "prod"},
		},
		{
			name:       "count",
			expansion:  `count = 2`,
			configPath: `[unit.dev.path, unit.prod.path][count.index]`,
			metaArg:    pkghclparse.MetaArgCount,
			components: map[string]string{"0": "dev", "1": "prod"},
		},
		{
			name:       "keyed ref to an expanded unit",
			expansion:  `for_each = { dev = "dev", prod = "prod" }`,
			configPath: `unit.env[each.key].path`,
			metaArg:    pkghclparse.MetaArgForEach,
			components: map[string]string{
				"dev":  filepath.Join("env", "dev"),
				"prod": filepath.Join("env", "prod"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := renderStackTemplate(t, expandedDependencyStack, dependencyStackFields{
				Expansion:  tc.expansion,
				ConfigPath: tc.configPath,
			})

			result, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
				Src:      []byte(src),
				Filename: "terragrunt.stack.hcl",
				StackDir: testStackDir,
			})
			require.NoError(t, err)

			resolved, ok := result.AutoIncludes[hclparse.AutoIncludeKey(hclparse.KindUnit, "consumer")]
			require.True(t, ok)
			require.Len(t, resolved.Dependencies, 1)

			metaArg, components := expandedConfigPaths(t, resolved.Dependencies[0].ConfigPath)
			assert.Equal(t, tc.metaArg, metaArg)

			want := make(map[string]string, len(tc.components))
			for key, component := range tc.components {
				want[key] = filepath.Join(testStackDir, hclparse.StackDir, component)
			}

			assert.Equal(t, want, components)
		})
	}
}

// TestParseStackFileReportsEveryFailingExpandedDependencyElement pins that an expanded autoinclude
// dependency reports the config_path failure of every element rather than only the first, and reports a
// failure several elements share once.
func TestParseStackFileReportsEveryFailingExpandedDependencyElement(t *testing.T) {
	t.Parallel()

	_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
		Src: []byte(`
unit "consumer" {
  source = "../catalog/units/consumer"
  path   = "consumer"

  autoinclude {
    dependency "env" {
      expansion {
        for_each = { a = 1, b = null, c = 2 }
      }

      config_path = each.value
    }
  }
}
`),
		Filename: "terragrunt.stack.hcl",
		StackDir: testStackDir,
	})

	var diags hcl.Diagnostics
	require.ErrorAs(t, err, &diags)
	assert.Len(t, diags, 2)
}

// TestBuildComponentRefMapKeysExpandedElements pins that the elements of an expanded component nest
// under its label by key, beside the refs of unexpanded components.
func TestBuildComponentRefMapKeysExpandedElements(t *testing.T) {
	t.Parallel()

	refs, err := hclparse.BuildComponentRefMap(hclparse.VarUnit, []hclparse.ComponentRef{
		{Name: "repo", Path: "/stack/repo"},
		{
			Name:     "env",
			Path:     "/stack/env/dev",
			Instance: pkghclparse.InstanceKey{EachKey: new("dev")},
		},
		{
			Name:     "env",
			Path:     "/stack/env/prod",
			Instance: pkghclparse.InstanceKey{EachKey: new("prod")},
		},
		{
			Name:     "shard",
			Path:     "/stack/shard/0",
			Instance: pkghclparse.InstanceKey{CountIndex: new(0)},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "/stack/repo", refs.GetAttr("repo").GetAttr("path").AsString())
	assert.Equal(t, "/stack/env/dev", refs.GetAttr("env").GetAttr("dev").GetAttr("path").AsString())
	assert.Equal(
		t,
		"/stack/env/prod",
		refs.GetAttr("env").GetAttr("prod").GetAttr("path").AsString(),
	)
	assert.Equal(t, "/stack/shard/0", refs.GetAttr("shard").GetAttr("0").GetAttr("path").AsString())
}

// TestParseStackFileRejectsExpandedAndUnexpandedUnitSharingALabel pins that a unit label declared both
// with and without an expansion fails the parse, since unit.<name> cannot refer to both.
func TestParseStackFileRejectsExpandedAndUnexpandedUnitSharingALabel(t *testing.T) {
	t.Parallel()

	_, err := hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{
		Src:      []byte(expandedAndUnexpandedEnvStack),
		Filename: "terragrunt.stack.hcl",
		StackDir: testStackDir,
	})

	var collision hclparse.ComponentRefCollisionError
	require.ErrorAs(t, err, &collision)
	assert.Equal(t, hclparse.VarUnit, collision.Root)
	assert.Equal(t, "env", collision.Name)
}

// TestUnitPathsFromStackDirRejectsExpandedAndUnexpandedUnitSharingALabel pins that discovery rejects
// the same label collision the full parse does.
func TestUnitPathsFromStackDirRejectsExpandedAndUnexpandedUnitSharingALabel(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(
		t,
		vfs.WriteFile(
			fs,
			"/test/terragrunt.stack.hcl",
			[]byte(expandedAndUnexpandedEnvStack),
			0644,
		),
	)

	_, err := hclparse.UnitPathsFromStackDir(fs, "/test", &hclparse.StackDirArgs{FuncsFor: noFuncs})

	var collision hclparse.ComponentRefCollisionError
	require.ErrorAs(t, err, &collision)
	assert.Equal(t, "env", collision.Name)
}

// expandedAndUnexpandedEnvStack declares the env label on both an unexpanded and an expanded unit.
const expandedAndUnexpandedEnvStack = `
unit "env" {
  source = "../units/env"
  path   = "env"
}

unit "env" {
  expansion {
    for_each = { dev = "dev" }
  }

  source = "../units/env"
  path   = "env/${each.key}"
}
`

// TestUnitPathsFromStackDirExpandsUnitBlocks pins that discovery returns the generated path of every
// element of an expanded unit.
func TestUnitPathsFromStackDirExpandsUnitBlocks(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/test/terragrunt.stack.hcl", []byte(`
unit "repo" {
  source = "../units/repo"
  path   = "repo"
}

unit "environment" {
  expansion {
    for_each = { dev = "dev", prod = "prod" }
  }

  source = "../units/environment"
  path   = "environment/${each.key}"
}
`), 0644))

	paths, err := hclparse.UnitPathsFromStackDir(
		fs,
		"/test",
		&hclparse.StackDirArgs{FuncsFor: noFuncs},
	)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		filepath.Join("/test", hclparse.StackDir, "repo"),
		filepath.Join("/test", hclparse.StackDir, "environment", "dev"),
		filepath.Join("/test", hclparse.StackDir, "environment", "prod"),
	}, paths)
}

// expandedConfigPaths returns the meta-argument that expanded a dependency and the config_path of each
// of its elements by key.
//
// Fails the test when configPath is not an expanded config_path.
func expandedConfigPaths(
	t *testing.T,
	configPath hclparse.ConfigPath,
) (pkghclparse.MetaArg, map[string]string) {
	t.Helper()

	switch paths := configPath.(type) {
	case hclparse.CountConfigPaths:
		byIndex := make(map[string]string, len(paths))

		for i, path := range paths {
			byIndex[strconv.Itoa(i)] = path
		}

		return pkghclparse.MetaArgCount, byIndex
	case hclparse.ForEachConfigPaths:
		return pkghclparse.MetaArgForEach, paths.Paths
	default:
		require.Failf(t, "config_path is not expanded", "got %T", configPath)

		return "", nil
	}
}

// generatedInputString returns the string input named key from the terragrunt.autoinclude.hcl
// generated in unitDir.
func generatedInputString(t *testing.T, fs vfs.FS, unitDir, key string) string {
	t.Helper()

	content, err := vfs.ReadFile(fs, filepath.Join(unitDir, hclparse.AutoIncludeFile))
	require.NoError(t, err)

	file, diags := hclsyntax.ParseConfig(content, hclparse.AutoIncludeFile, hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())

	body, _, diags := file.Body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{{Name: "inputs", Required: true}},
	})
	require.False(t, diags.HasErrors(), diags.Error())

	inputs, diags := body.Attributes["inputs"].Expr.Value(nil)
	require.False(t, diags.HasErrors(), diags.Error())

	return inputs.GetAttr(key).AsString()
}
