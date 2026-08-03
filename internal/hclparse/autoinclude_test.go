package hclparse_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestAutoIncludeHCL_Resolve_Nil(t *testing.T) {
	t.Parallel()

	var a *hclparse.AutoIncludeHCL

	result, diags := a.Resolve(nil)
	assert.Nil(t, result)
	assert.False(t, diags.HasErrors())
}

func TestAutoIncludeHCL_Resolve_DependencyConfigPath(t *testing.T) {
	t.Parallel()

	src := `
dependency "vpc" {
  config_path = unit.vpc.path
}
`
	body := parseHCLBody(t, src)

	autoInclude := &hclparse.AutoIncludeHCL{
		Remain: body,
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"vpc": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("../vpc"),
				}),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	result, diags := autoInclude.Resolve(evalCtx)
	require.False(t, diags.HasErrors(), "resolve error: %s", diags.Error())
	require.NotNil(t, result)
	require.Len(t, result.Dependencies, 1)
	assert.Equal(t, "vpc", result.Dependencies[0].Name)
	assert.Equal(t, "../vpc", result.Dependencies[0].ConfigPath)
	assert.NotNil(t, result.Dependencies[0].Block)
	assert.NotNil(t, result.RawBody)
}

// Null or unknown config_path must produce a diagnostic (not a silent zero-value dep) so users see an error and editors get a source position.
func TestAutoIncludeHCL_Resolve_RejectsNullOrUnknownConfigPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		val     cty.Value
		summary string
	}{
		{val: cty.NullVal(cty.String), summary: "Null config_path"},
		{val: cty.UnknownVal(cty.String), summary: "Unknown config_path"},
	}

	for _, tc := range cases {
		t.Run(tc.summary, func(t *testing.T) {
			t.Parallel()

			body := parseHCLBody(t, `
dependency "vpc" {
  config_path = local.target
}
`)

			autoInclude := &hclparse.AutoIncludeHCL{Remain: body}

			evalCtx := &hcl.EvalContext{
				Variables: map[string]cty.Value{
					"local": cty.ObjectVal(map[string]cty.Value{"target": tc.val}),
				},
			}

			result, diags := autoInclude.Resolve(evalCtx)
			require.True(t, diags.HasErrors(), "%s must surface as a diagnostic", tc.summary)
			assert.Equal(t, tc.summary, diags[0].Summary)
			require.NotNil(
				t,
				diags[0].Subject,
				"diagnostic must carry the offending expression's source range",
			)
			require.NotNil(t, result, "best-effort: result is non-nil even when some deps fail")
			assert.Empty(t, result.Dependencies, "failed dep must not be appended")
		})
	}
}

// dependency block with zero labels must be reported as a diagnostic (not silently skipped) so users learn the labelling convention.
func TestAutoIncludeHCL_Resolve_RejectsZeroLabels(t *testing.T) {
	t.Parallel()

	body := parseHCLBody(t, `
dependency {
  config_path = "../vpc"
}
`)

	autoInclude := &hclparse.AutoIncludeHCL{Remain: body}

	result, diags := autoInclude.Resolve(&hcl.EvalContext{Variables: map[string]cty.Value{}})
	require.True(t, diags.HasErrors())
	assert.Contains(t, diags.Error(), "exactly one label, got 0")
	require.NotNil(t, result, "best-effort: result is non-nil even when some deps fail")
	assert.Empty(t, result.Dependencies)
}

// dependency block with two or more labels likewise produces a diagnostic instead of silently using only the first label.
func TestAutoIncludeHCL_Resolve_RejectsTwoLabels(t *testing.T) {
	t.Parallel()

	body := parseHCLBody(t, `
dependency "vpc" "extra" {
  config_path = "../vpc"
}
`)

	autoInclude := &hclparse.AutoIncludeHCL{Remain: body}

	result, diags := autoInclude.Resolve(&hcl.EvalContext{Variables: map[string]cty.Value{}})
	require.True(t, diags.HasErrors())
	assert.Contains(t, diags.Error(), "exactly one label, got 2")
	require.NotNil(t, result, "best-effort: result is non-nil even when some deps fail")
	assert.Empty(t, result.Dependencies)
}

func TestAutoIncludeHCL_Resolve_MultipleDependencies(t *testing.T) {
	t.Parallel()

	src := `
dependency "vpc" {
  config_path = unit.vpc.path
}

dependency "db" {
  config_path = unit.database.path
}
`
	body := parseHCLBody(t, src)

	autoInclude := &hclparse.AutoIncludeHCL{
		Remain: body,
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"vpc": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("../vpc"),
				}),
				"database": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("../database"),
				}),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	result, diags := autoInclude.Resolve(evalCtx)
	require.False(t, diags.HasErrors(), "resolve error: %s", diags.Error())
	require.NotNil(t, result)
	require.Len(t, result.Dependencies, 2)
	assert.Equal(t, "vpc", result.Dependencies[0].Name)
	assert.Equal(t, "../vpc", result.Dependencies[0].ConfigPath)
	assert.Equal(t, "db", result.Dependencies[1].Name)
	assert.Equal(t, "../database", result.Dependencies[1].ConfigPath)
}

func TestAutoIncludeHCL_Resolve_StackRef(t *testing.T) {
	t.Parallel()

	src := `
dependency "networking" {
  config_path = stack.networking.path
}
`
	body := parseHCLBody(t, src)

	autoInclude := &hclparse.AutoIncludeHCL{
		Remain: body,
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.EmptyObjectVal,
			"stack": cty.ObjectVal(map[string]cty.Value{
				"networking": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("../networking"),
				}),
			}),
		},
	}

	result, diags := autoInclude.Resolve(evalCtx)
	require.False(t, diags.HasErrors(), "resolve error: %s", diags.Error())
	require.NotNil(t, result)
	require.Len(t, result.Dependencies, 1)
	assert.Equal(t, "networking", result.Dependencies[0].Name)
	assert.Equal(t, "../networking", result.Dependencies[0].ConfigPath)
}

func TestAutoIncludeHCL_Resolve_DependencyWithMockOutputs(t *testing.T) {
	t.Parallel()

	// dependency block with config_path + mock_outputs + inputs
	// Only config_path should be evaluated; inputs are left in RawBody
	src := `
dependency "vpc" {
  config_path = unit.vpc.path

  mock_outputs_allowed_terraform_commands = ["plan"]
  mock_outputs = {
    val = "fake-val"
  }
}

inputs = {
  val = dependency.vpc.outputs.val
}
`
	body := parseHCLBody(t, src)

	autoInclude := &hclparse.AutoIncludeHCL{
		Remain: body,
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"vpc": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("/abs/path/to/.terragrunt-stack/vpc"),
				}),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	result, diags := autoInclude.Resolve(evalCtx)
	require.False(t, diags.HasErrors(), "resolve error: %s", diags.Error())
	require.NotNil(t, result)

	// Dependency config_path resolved
	require.Len(t, result.Dependencies, 1)
	assert.Equal(t, "vpc", result.Dependencies[0].Name)
	assert.Equal(t, "/abs/path/to/.terragrunt-stack/vpc", result.Dependencies[0].ConfigPath)

	// RawBody preserved (contains inputs with dependency.vpc.outputs.val)
	assert.NotNil(t, result.RawBody)
}

func TestAutoIncludeDependencyPaths_NoFile(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.NoError(t, err)
	assert.Nil(t, paths)
}

func TestAutoIncludeDependencyPaths_WithDependency(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	require.NoError(t, vfs.WriteFile(fs, filepath.Join("/test", hclparse.AutoIncludeFile), []byte(`
dependency "vpc" {
  config_path = "../unit-w-outputs"
}
`), 0644))

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Equal(t, filepath.Clean(filepath.Join("/test", "..", "unit-w-outputs")), paths[0])
}

func TestAutoIncludeDependencyPaths_MultipleDeps(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	require.NoError(t, vfs.WriteFile(fs, filepath.Join("/test", hclparse.AutoIncludeFile), []byte(`
dependency "vpc" {
  config_path = "../vpc"
}
dependency "db" {
  config_path = "../database"
}
`), 0644))

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.NoError(t, err)
	require.Len(t, paths, 2)
}

func TestAutoIncludeDependencyPaths_Symlink(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	resolvedTmpDir, _ := filepath.EvalSymlinks(tmpDir)

	// Create real unit directory with autoinclude file
	realDir := filepath.Join(resolvedTmpDir, "real-unit")
	require.NoError(t, os.MkdirAll(realDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(realDir, hclparse.AutoIncludeFile), []byte(`
dependency "vpc" {
  config_path = "../vpc"
}
`), 0644))

	// Create symlink to unit directory
	symlinkDir := filepath.Join(tmpDir, "symlinked-unit")
	require.NoError(t, os.Symlink(realDir, symlinkDir))

	// AutoIncludeDependencyPaths via symlink should resolve correctly
	paths, err := hclparse.AutoIncludeDependencyPaths(vfs.NewOSFS(), symlinkDir)
	require.NoError(t, err)
	require.Len(t, paths, 1)

	// The dependency path should be resolved relative to the REAL directory, not the symlink
	assert.NotContains(t, paths[0], "symlinked-unit")
	// ../vpc resolved from real-unit gives <tmpDir>/vpc
	expected := filepath.Clean(filepath.Join(realDir, "..", "vpc"))
	assert.Equal(t, expected, paths[0])
}

func TestAutoIncludeDependencyPaths_AbsolutePath(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	require.NoError(t, vfs.WriteFile(fs, filepath.Join("/test", hclparse.AutoIncludeFile), []byte(`
dependency "vpc" {
  config_path = "/absolute/path/to/vpc"
}
`), 0644))

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Equal(t, "/absolute/path/to/vpc", paths[0])
}

// Each malformed dependency block surfaces as a typed MalformedDependencyError naming the dependency: the contract is loud-fail, not silent skip.
func TestAutoIncludeDependencyPaths_MalformedReturnsTypedError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		content        string
		wantDepName    string
		wantReasonPart string
	}{
		{
			name:           "missing config_path",
			content:        `dependency "x" {}`,
			wantDepName:    "x",
			wantReasonPart: "missing config_path",
		},
		{
			name:           "non-string config_path",
			content:        `dependency "x" { config_path = 42 }`,
			wantDepName:    "x",
			wantReasonPart: "config_path must be a string, got number",
		},
		{
			name:           "unevaluable config_path",
			content:        `dependency "x" { config_path = unit.vpc.path }`,
			wantDepName:    "x",
			wantReasonPart: "config_path",
		},
		{
			name:           "no labels",
			content:        `dependency { config_path = "../vpc" }`,
			wantDepName:    "(unlabeled)",
			wantReasonPart: "exactly one label, got 0",
		},
		{
			name:           "two labels",
			content:        `dependency "vpc" "extra" { config_path = "../vpc" }`,
			wantDepName:    "vpc extra",
			wantReasonPart: "exactly one label, got 2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()
			require.NoError(
				t,
				vfs.WriteFile(
					fs,
					filepath.Join("/test", hclparse.AutoIncludeFile),
					[]byte(tc.content),
					0644,
				),
			)

			paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
			require.Error(t, err)
			assert.Nil(t, paths)

			var malformedErr hclparse.MalformedDependencyError
			require.ErrorAs(t, err, &malformedErr)
			assert.Equal(t, tc.wantDepName, malformedErr.Name)
			assert.Contains(t, malformedErr.Reason, tc.wantReasonPart)
			assert.Contains(t, err.Error(), "malformed dependency "+strconv.Quote(tc.wantDepName))
		})
	}
}

// Mixed-case fixture: one valid dependency + one malformed. Strict contract: producer returns (nil, err) on any malformed block - no partial paths.
func TestAutoIncludeDependencyPaths_MixedValidAndMalformed(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, filepath.Join("/test", hclparse.AutoIncludeFile), []byte(`
dependency "vpc" {
  config_path = "../vpc"
}

dependency "broken" {}
`), 0644))

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.Error(t, err)
	assert.Nil(t, paths, "strict fail-fast: no partial paths when any block is malformed")

	var malformedErr hclparse.MalformedDependencyError
	require.ErrorAs(t, err, &malformedErr)
	assert.Equal(t, "broken", malformedErr.Name)
}

func TestAutoIncludeDependencyPaths_FileParseErrorOnSyntaxError(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(
		t,
		vfs.WriteFile(
			fs,
			filepath.Join("/test", hclparse.AutoIncludeFile),
			[]byte(`dependency "x" { config_path = "`),
			0644,
		),
	)

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.Error(t, err)
	assert.Nil(t, paths)

	var fpe hclparse.FileParseError
	require.ErrorAs(t, err, &fpe)
}

func TestAutoIncludeDependencyPaths_FileParseErrorOnJSON(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	jsonBody := `{"dependency": {"vpc": {"config_path": "../vpc"}}}`
	require.NoError(
		t,
		vfs.WriteFile(fs, filepath.Join("/test", hclparse.AutoIncludeFile), []byte(jsonBody), 0644),
	)

	paths, err := hclparse.AutoIncludeDependencyPaths(fs, "/test")
	require.Error(t, err)
	assert.Nil(t, paths)

	var fpe hclparse.FileParseError
	require.ErrorAs(t, err, &fpe)
}

func TestResolveForKind_StackAutoIncludeDepValues(t *testing.T) {
	t.Parallel()

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"producer": cty.ObjectVal(
					map[string]cty.Value{"path": cty.StringVal("../producer")},
				),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	// Unsupported: a stack autoinclude declares a dependency and an injected unit's values reference its outputs.
	badSrc := `
dependency "producer" {
  config_path = unit.producer.path
}

unit "extra" {
  source = "."
  path   = "extra"

  values = {
    v = dependency.producer.outputs.val
  }
}
`
	bad := &hclparse.AutoIncludeHCL{Remain: parseHCLBody(t, badSrc)}

	_, diags := bad.ResolveForKind(evalCtx, hclparse.KindStack, "net")
	require.True(
		t,
		diags.HasErrors(),
		"stack autoinclude consuming a sibling dependency via injected values must error",
	)

	extra, ok := diags[0].Extra.(error)
	require.True(t, ok, "the diagnostic Extra must carry an error")

	var typed hclparse.StackAutoIncludeDependencyValuesError
	require.ErrorAs(t, extra, &typed, "the diagnostic must carry the typed error")
	assert.Equal(t, "net", typed.StackName)
	assert.Equal(t, "extra", typed.UnitName)
	assert.Contains(t, diags[0].Detail, "supported cross-level pattern")

	// Supported: injected unit values reference only unit.X.path, not a dependency output.
	okSrc := `
unit "extra" {
  source = "."
  path   = "extra"

  values = {
    v = unit.producer.path
  }
}
`
	supported := &hclparse.AutoIncludeHCL{Remain: parseHCLBody(t, okSrc)}

	_, okDiags := supported.ResolveForKind(evalCtx, hclparse.KindStack, "net")
	require.False(
		t,
		okDiags.HasErrors(),
		"literal unit.path values must still resolve: %s",
		okDiags.Error(),
	)

	// A real unit-level autoinclude carries a top-level dependency feeding unit-level inputs; the same
	// dependency-output reference is allowed when the kind is unit (the established cross-level pattern).
	unitSrc := `
dependency "producer" {
  config_path = unit.producer.path
}

inputs = {
  v = dependency.producer.outputs.val
}
`
	unitAutoInclude := &hclparse.AutoIncludeHCL{Remain: parseHCLBody(t, unitSrc)}

	_, unitDiags := unitAutoInclude.ResolveForKind(evalCtx, hclparse.KindUnit, "")
	require.False(
		t,
		unitDiags.HasErrors(),
		"unit-level autoinclude with a dependency-output input must still resolve: %s",
		unitDiags.Error(),
	)
}

// TestResolveForKind_StackAutoIncludeDepValuesIndexForm pins that the index traversal form
// dependency["producer"].outputs.val trips the same typed error as the attribute form, instead of
// slipping past and falling through to a cryptic generic decode failure.
func TestResolveForKind_StackAutoIncludeDepValuesIndexForm(t *testing.T) {
	t.Parallel()

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"producer": cty.ObjectVal(
					map[string]cty.Value{"path": cty.StringVal("../producer")},
				),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	// The unsupported dependency-output reference written in index form.
	badSrc := `
dependency "producer" {
  config_path = unit.producer.path
}

unit "extra" {
  source = "."
  path   = "extra"

  values = {
    v = dependency["producer"].outputs.val
  }
}
`
	bad := &hclparse.AutoIncludeHCL{Remain: parseHCLBody(t, badSrc)}

	_, diags := bad.ResolveForKind(evalCtx, hclparse.KindStack, "net")
	require.True(t, diags.HasErrors(), "the index traversal form must trip the typed error too")

	extra, ok := diags[0].Extra.(error)
	require.True(t, ok, "the diagnostic Extra must carry an error")

	var typed hclparse.StackAutoIncludeDependencyValuesError
	require.ErrorAs(
		t,
		extra,
		&typed,
		"the diagnostic must carry the typed error for the index form",
	)
	assert.Equal(t, "extra", typed.UnitName)
	assert.Contains(t, diags[0].Detail, "supported cross-level pattern")
}

// TestResolveForKind_StackAutoIncludeDepValuesDynamicIndex pins that a dynamic index
// (dependency[values.dep_name].outputs.val) trips the typed error too. The dynamic index hides the
// dependency name from static analysis, so the validator must reject the dependency root rather than
// let it fall through to a cryptic generic decode failure.
func TestResolveForKind_StackAutoIncludeDepValuesDynamicIndex(t *testing.T) {
	t.Parallel()

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"producer": cty.ObjectVal(
					map[string]cty.Value{"path": cty.StringVal("../producer")},
				),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	// The dependency output reference written with a dynamic index whose name is not statically known.
	badSrc := `
dependency "producer" {
  config_path = unit.producer.path
}

unit "extra" {
  source = "."
  path   = "extra"

  values = {
    dep_name = "producer"
    v        = dependency[values.dep_name].outputs.val
  }
}
`
	bad := &hclparse.AutoIncludeHCL{Remain: parseHCLBody(t, badSrc)}

	_, diags := bad.ResolveForKind(evalCtx, hclparse.KindStack, "net")
	require.True(t, diags.HasErrors(), "a dynamic dependency index must trip the typed error too")

	extra, ok := diags[0].Extra.(error)
	require.True(t, ok, "the diagnostic Extra must carry an error")

	var typed hclparse.StackAutoIncludeDependencyValuesError
	require.ErrorAs(
		t,
		extra,
		&typed,
		"the diagnostic must carry the typed error for the dynamic index form",
	)
	assert.Equal(t, "extra", typed.UnitName)
	assert.Contains(t, diags[0].Detail, "supported cross-level pattern")
}

// TestResolveForKind_StackAutoIncludeUndeclaredDepRefAlsoRejected pins that the detection is generic,
// not scoped to the autoinclude's own declared dependencies: injected values that reference ANY
// dependency (here "other", which the autoinclude does not declare) still trip the typed error, because
// injected values are evaluated at generation time when no dependency outputs exist.
func TestResolveForKind_StackAutoIncludeUndeclaredDepRefAlsoRejected(t *testing.T) {
	t.Parallel()

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"producer": cty.ObjectVal(
					map[string]cty.Value{"path": cty.StringVal("../producer")},
				),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	// The autoinclude declares "producer"; the injected unit's values reference a DIFFERENT, undeclared dep.
	src := `
dependency "producer" {
  config_path = unit.producer.path
}

unit "extra" {
  source = "."
  path   = "extra"

  values = {
    v = dependency.other.outputs.val
  }
}
`
	autoInclude := &hclparse.AutoIncludeHCL{Remain: parseHCLBody(t, src)}

	_, diags := autoInclude.ResolveForKind(evalCtx, hclparse.KindStack, "net")
	require.True(
		t,
		diags.HasErrors(),
		"any dependency reference in injected values must be rejected, declared or not",
	)

	extra, ok := diags[0].Extra.(error)
	require.True(t, ok, "the diagnostic Extra must carry an error")

	var typed hclparse.StackAutoIncludeDependencyValuesError
	require.ErrorAs(
		t,
		extra,
		&typed,
		"the diagnostic must carry the typed error even for an undeclared dependency",
	)
	assert.Equal(t, "extra", typed.UnitName)
	assert.Contains(t, diags[0].Detail, "supported cross-level pattern")
}

// parseHCLBody is a test helper that parses an HCL string and returns the body.
func parseHCLBody(t *testing.T, src string) hcl.Body {
	t.Helper()

	file, diags := hclsyntax.ParseConfig([]byte(src), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

	return file.Body
}
