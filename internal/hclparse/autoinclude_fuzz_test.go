package hclparse_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

const (
	aiFuzzStackDir = "/fuzz/stack"
	aiFuzzGenDir   = "/fuzz/stack/.terragrunt-stack/app"
)

// aiFuzzFuncs is a small generate-time function set so function calls in autoinclude bodies can resolve.
func aiFuzzFuncs() map[string]function.Function {
	return map[string]function.Function{
		"upper": stdlib.UpperFunc,
		"lower": stdlib.LowerFunc,
		"merge": stdlib.MergeFunc,
	}
}

// aiFuzzValues exposes a values.* namespace (as if from a sibling terragrunt.values.hcl) for values.* references.
func aiFuzzValues() *cty.Value {
	v := cty.ObjectVal(map[string]cty.Value{
		"region": cty.StringVal("us-east-1"),
		"key":    cty.StringVal("k"),
	})

	return &v
}

// genUnitAutoIncludeBody wraps body as the autoinclude block of a unit "app" (alongside a sibling unit "vpc" it can
// reference) and runs the full parse -> resolve -> generate pipeline. It returns the generated file bytes (nil when
// nothing is generated) and fails the test on any panic.
func genUnitAutoIncludeBody(t *testing.T, body string) []byte {
	t.Helper()

	src := `
locals {
  env   = "prod"
  count = 2
  flag  = true
  list  = ["a", "b"]
  map   = { a = "1" }
}

unit "vpc" {
  source = "."
  path   = "vpc"
}

unit "app" {
  source = "."
  path   = "app"

  autoinclude {
` + body + `
  }
}
`

	var out []byte

	require.NotPanicsf(t, func() {
		out = generateAutoIncludeForKind(t, src, hclparse.KindUnit, "app", hclparse.AutoIncludeFile)
	}, "panic generating unit autoinclude for body:\n%s", body)

	return out
}

// genStackAutoIncludeBody is the stack-kind analogue: it wraps body as the autoinclude block of a stack "child".
func genStackAutoIncludeBody(t *testing.T, body string) []byte {
	t.Helper()

	src := `
unit "vpc" {
  source = "."
  path   = "vpc"
}

stack "child" {
  source = "."
  path   = "child"

  autoinclude {
` + body + `
  }
}
`

	var out []byte

	require.NotPanicsf(t, func() {
		out = generateAutoIncludeForKind(t, src, hclparse.KindStack, "child", hclparse.AutoIncludeStackFile)
	}, "panic generating stack autoinclude for body:\n%s", body)

	return out
}

// generateAutoIncludeForKind parses src, resolves the autoinclude for (kind, name), generates it, and returns the
// generated file bytes (nil when the construct is rejected or produces no file).
func generateAutoIncludeForKind(t *testing.T, src string, kind hclparse.AutoIncludeKind, name, fileName string) []byte {
	t.Helper()

	// The read-back fileName must match the kind the generator derives from, else a stale or missing file masks bugs.
	require.Equalf(t, hclparse.AutoIncludeFileNameForKind(kind), fileName, "harness fileName %q does not match kind %q", fileName, kind)

	srcBytes := []byte(src)
	fs := vfs.NewMemMapFS()

	result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
		Src:       srcBytes,
		Filename:  "terragrunt.stack.hcl",
		StackDir:  aiFuzzStackDir,
		Functions: aiFuzzFuncs(),
		Values:    aiFuzzValues(),
	})
	if err != nil || result == nil {
		return nil
	}

	key := hclparse.AutoIncludeKey(kind, name)

	resolved, ok := result.AutoIncludes[key]
	if !ok {
		return nil
	}

	// A stored entry must be populated and consistent: generation needs EvalCtx and RawBody, and Kind drives the filename.
	require.NotNilf(t, resolved, "autoinclude %s present but nil for body:\n%s", key, src)
	require.NotNilf(t, resolved.EvalCtx, "autoinclude %s resolved with nil EvalCtx for body:\n%s", key, src)
	require.NotNilf(t, resolved.RawBody, "autoinclude %s resolved with nil RawBody for body:\n%s", key, src)
	require.Equalf(t, kind, resolved.Kind, "autoinclude %s resolved with kind %q for body:\n%s", key, resolved.Kind, src)

	// A stack autoinclude injects only unit/stack blocks, so a cleanly resolved stack entry carries no dependencies.
	if kind == hclparse.KindStack {
		require.Emptyf(t, resolved.Dependencies, "stack autoinclude %s retained %d resolved dependencies for body:\n%s", key, len(resolved.Dependencies), src)
	}

	if genErr := hclparse.GenerateAutoIncludeFile(fs, resolved, aiFuzzGenDir, srcBytes, resolved.EvalCtx); genErr != nil {
		return nil
	}

	out, readErr := vfs.ReadFile(fs, filepath.Join(aiFuzzGenDir, fileName))
	if readErr != nil {
		return nil
	}

	return out
}

// reparsesAsValidHCL fails t when generated is non-empty and does not re-parse as valid HCL.
func reparsesAsValidHCL(t *testing.T, body string, generated []byte) {
	t.Helper()

	if generated == nil {
		return
	}

	_, diags := hclsyntax.ParseConfig(generated, "generated.hcl", hcl.Pos{Line: 1, Column: 1})
	if !diags.HasErrors() {
		return
	}

	require.Failf(t, "generated invalid HCL", "autoinclude body:\n%s\noutput:\n%s\ndiags: %s", body, generated, diags.Error())
}

// aiBlockBodySeeds is the shared seed corpus of HCL constructs placed inside an autoinclude block: scalars,
// collections, references, templates, functions, conditionals, for/splat/index/ops, computed keys, and block types.
func aiBlockBodySeeds() []string {
	return []string{
		`inputs = { v = "s" }`,
		`inputs = { v = 42 }`,
		`inputs = { v = true }`,
		`inputs = { v = null }`,
		`inputs = { v = [1, 2, 3] }`,
		`inputs = { v = { a = 1, b = "x" } }`,
		`inputs = { v = local.env }`,
		`inputs = { v = values.region }`,
		`inputs = { v = unit.vpc.path }`,
		`inputs = { v = dependency.vpc.outputs.id }`,
		`inputs = { v = "${local.env}-${dependency.vpc.outputs.id}" }`,
		`inputs = { v = "${0}-${dependency.vpc.outputs.id}" }`,
		`inputs = { v = "${0}${A}" }`,
		`inputs = { v = upper(local.env) }`,
		`inputs = { v = lower(dependency.vpc.outputs.id) }`,
		`inputs = { v = local.flag ? "a" : dependency.vpc.outputs.id }`,
		`inputs = { v = [for x in local.list : upper(x)] }`,
		`inputs = { v = [for x in dependency.vpc.outputs.list : x] }`,
		`inputs = { v = dependency.vpc.outputs.subnets[*].id }`,
		`inputs = { v = dependency.vpc.outputs.list[local.env] }`,
		`inputs = { v = local.count + 1 }`,
		`inputs = { v = 1 / 0 }`,
		`inputs = { v = { "${local.env}_k" = dependency.vpc.outputs.id } }`,
		"dependency \"db\" {\n  config_path = unit.vpc.path\n}",
		"dependency \"db\" {\n  config_path  = unit.vpc.path\n  mock_outputs = { id = local.env }\n}",
		"generate \"g\" {\n  path      = \"g.tf\"\n  if_exists = \"overwrite\"\n  contents  = \"# ${local.env}\"\n}",
		"remote_state {\n  backend = \"local\"\n  config  = { path = local.env }\n}",
		"retry \"e\" {\n  retryable_errors = [\".*\"]\n  max_attempts     = 3\n}",
		`locals { x = 1 }`,
		`autoinclude { inputs = { a = 1 } }`,
		``,
		`{}`,
		`not hcl @#$`,
	}
}

// FuzzAutoIncludeBlockBody mutates the content of a unit autoinclude block and runs the full parse -> resolve ->
// generate pipeline. Invariants: it never panics, and any generated terragrunt.autoinclude.hcl re-parses as valid HCL.
func FuzzAutoIncludeBlockBody(f *testing.F) {
	for _, s := range aiBlockBodySeeds() {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, body string) {
		reparsesAsValidHCL(t, body, genUnitAutoIncludeBody(t, body))
	})
}

// FuzzAutoIncludeBlockBodyStackKind mutates the content of a stack-block autoinclude (which injects unit/stack
// blocks) and runs parse -> resolve -> generate for the stack kind. Same invariants as the unit-kind fuzz.
func FuzzAutoIncludeBlockBodyStackKind(f *testing.F) {
	seeds := []string{
		"unit \"extra\" {\n  source = \".\"\n  path   = \"extra\"\n}",
		"stack \"sub\" {\n  source = \".\"\n  path   = \"sub\"\n}",
		"unit \"extra\" {\n  source = \".\"\n  path   = \"extra\"\n  values = { v = unit.vpc.path }\n}",
		"unit \"extra\" {\n  source = \".\"\n  path   = \"extra\"\n  values = { v = dependency.foo.outputs.bar }\n}",
		`inputs = { a = 1 }`,
		`dependency "x" { config_path = "../x" }`,
		``,
		`{}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, body string) {
		reparsesAsValidHCL(t, body, genStackAutoIncludeBody(t, body))
	})
}
