package hclparse_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func TestPartialEval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hcl      string
		evalCtx  *hcl.EvalContext
		contains []string
		excludes []string
	}{
		{
			name:     "literal string",
			hcl:      `val = "hello"`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"hello"`},
		},
		{
			name:     "literal number",
			hcl:      `val = 42`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"42"},
		},
		{
			name:     "literal bool",
			hcl:      `val = true`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"true"},
		},
		{
			name:     "pure local ref",
			hcl:      `val = local.env`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"production"`},
			excludes: []string{"local.env"},
		},
		{
			name:     "deferred dependency ref",
			hcl:      `val = dependency.vpc.outputs.vpc_id`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"dependency.vpc.outputs.vpc_id"},
		},
		{
			name:     "mixed template",
			hcl:      `val = "${local.env}-${dependency.vpc.outputs.vpc_id}-suffix"`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"production", "${dependency.vpc.outputs.vpc_id}", "suffix"},
			excludes: []string{"local.env"},
		},
		{
			name:     "pure template",
			hcl:      `val = "${local.env}-${local.region}"`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"production-us-east-1"`},
			excludes: []string{"local.env", "local.region"},
		},
		{
			name: "object with mixed values",
			hcl: `val = {
  env    = local.env
  vpc_id = dependency.vpc.outputs.vpc_id
}`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"production"`, "dependency.vpc.outputs.vpc_id"},
			excludes: []string{"local.env"},
		},
		{
			name: "pure object",
			hcl: `val = {
  env    = local.env
  region = local.region
}`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"production"`, `"us-east-1"`},
			excludes: []string{"local.env", "local.region"},
		},
		{
			name:     "tuple with mixed elements",
			hcl:      `val = [local.env, dependency.vpc.outputs.vpc_id, "literal"]`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"production"`, "dependency.vpc.outputs.vpc_id", `"literal"`},
			excludes: []string{"local.env"},
		},
		{
			name:     "try with deferred arg verbatim",
			hcl:      `val = try(dependency.vpc.outputs.vpc_id, "default")`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"try(dependency.vpc.outputs.vpc_id", `"default"`},
		},
		{
			name:     "conditional pure condition true",
			hcl:      `val = local.flag ? "yes" : dependency.vpc.outputs.vpc_id`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"yes"`},
		},
		{
			name:     "conditional pure condition false",
			hcl:      `val = !local.flag ? "yes" : dependency.vpc.outputs.vpc_id`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"dependency.vpc.outputs.vpc_id"},
		},
		{
			name:     "conditional deferred condition",
			hcl:      `val = dependency.vpc.outputs.ready ? "yes" : "no"`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"dependency.vpc.outputs.ready", `"yes"`, `"no"`},
		},
		{
			name:     "for expression with deferred",
			hcl:      `val = [for k, v in dependency.vpc.outputs.tags : v]`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"for", "dependency.vpc.outputs.tags"},
		},
		{
			name:     "binary op with deferred",
			hcl:      `val = local.count + dependency.vpc.outputs.count`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"local.count", "dependency.vpc.outputs.count"},
		},
		{
			name:     "parentheses pure inner",
			hcl:      `val = (local.env)`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"production"`},
			excludes: []string{"local.env"},
		},
		{
			name:     "parentheses deferred inner",
			hcl:      `val = (dependency.vpc.outputs.vpc_id)`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"(dependency.vpc.outputs.vpc_id)"},
		},
		{
			name:     "nil eval ctx",
			hcl:      `val = local.env`,
			evalCtx:  nil,
			contains: []string{"local.env"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr, srcBytes := parseFirstAttrExpr(t, tc.hcl)
			result := string(hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: tc.evalCtx, Deferred: testDeferred}))

			for _, want := range tc.contains {
				assert.Contains(t, result, want, "expected result to contain %q, got %q", want, result)
			}

			for _, notWant := range tc.excludes {
				assert.NotContains(t, result, notWant, "expected result NOT to contain %q, got %q", notWant, result)
			}
		})
	}
}

func TestIsPure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hcl  string
		want bool
	}{
		{
			name: "literal",
			hcl:  `val = "hello"`,
			want: true,
		},
		{
			name: "local ref",
			hcl:  `val = local.env`,
			want: true,
		},
		{
			name: "dependency ref",
			hcl:  `val = dependency.vpc.outputs.id`,
			want: false,
		},
		{
			name: "mixed template",
			hcl:  `val = "${local.env}-${dependency.vpc.outputs.id}"`,
			want: false,
		},
		{
			name: "pure template",
			hcl:  `val = "${local.env}-${local.region}"`,
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr, _ := parseFirstAttrExpr(t, tc.hcl)
			got := hclparse.IsPure(expr, testDeferred)
			assert.Equal(t, tc.want, got)
		})
	}
}

// testDeferred is the standard deferred roots map for tests.
var testDeferred = map[string]bool{
	"dependency": true,
}

// parseFirstAttrExpr parses an HCL snippet with a single attribute and returns
// the attribute's expression, the source bytes, and the eval context.
func parseFirstAttrExpr(t *testing.T, src string) (hclsyntax.Expression, []byte) {
	t.Helper()

	srcBytes := []byte(src)

	file, diags := hclsyntax.ParseConfig(srcBytes, "test.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

	body, ok := file.Body.(*hclsyntax.Body)
	require.True(t, ok)
	require.NotEmpty(t, body.Attributes, "expected at least one attribute")

	// Return the first attribute (map iteration order, but single attr).
	for _, attr := range body.Attributes {
		return attr.Expr, srcBytes
	}

	t.Fatal("unreachable")

	return nil, nil
}

func TestPartialEval_PreservesFunctionCalls(t *testing.T) {
	t.Parallel()

	var called bool

	evalCtx := buildEvalCtx()
	evalCtx.Functions = map[string]function.Function{
		"danger": function.New(&function.Spec{
			Type: function.StaticReturnType(cty.String),
			Impl: func([]cty.Value, cty.Type) (cty.Value, error) {
				called = true
				return cty.StringVal("executed"), nil
			},
		}),
	}

	expr, srcBytes := parseFirstAttrExpr(t, `val = danger()`)
	result := string(hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: evalCtx, Deferred: testDeferred}))

	assert.False(t, called, "partial evaluation must preserve function calls instead of executing them during generation")
	assert.Contains(t, result, "danger()")
	assert.NotContains(t, result, "executed")
}

// buildEvalCtx creates an eval context with local.env = "production" and
// local.region = "us-east-1" for testing.
func buildEvalCtx() *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"local": cty.ObjectVal(map[string]cty.Value{
				"env":    cty.StringVal("production"),
				"region": cty.StringVal("us-east-1"),
				"count":  cty.NumberIntVal(3),
				"flag":   cty.BoolVal(true),
			}),
		},
	}
}
