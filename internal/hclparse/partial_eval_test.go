package hclparse_test

import (
	"strings"
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
			name:     "try referencing dependency preserved",
			hcl:      `val = try(dependency.vpc.outputs.id, "default")`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"try(dependency.vpc.outputs.id", `"default"`},
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

			resultBytes, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: tc.evalCtx, Deferred: testDeferred})
			require.NoError(t, err)

			result := string(resultBytes)

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

func TestPartialEval_EvaluatesFunctionCallsUnlessTheyReferenceDependency(t *testing.T) {
	t.Parallel()

	newCtx := func(called *bool) *hcl.EvalContext {
		evalCtx := buildEvalCtx()
		evalCtx.Functions = map[string]function.Function{
			"danger": function.New(&function.Spec{
				VarParam: &function.Parameter{Type: cty.DynamicPseudoType},
				Type:     function.StaticReturnType(cty.String),
				Impl: func([]cty.Value, cty.Type) (cty.Value, error) {
					*called = true
					return cty.StringVal("executed"), nil
				},
			}),
		}

		return evalCtx
	}

	t.Run("pure function call is evaluated", func(t *testing.T) {
		t.Parallel()

		var called bool

		expr, srcBytes := parseFirstAttrExpr(t, `val = danger()`)

		resultBytes, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: newCtx(&called), Deferred: testDeferred})
		require.NoError(t, err)

		assert.True(t, called, "a function call with no deferred reference is evaluated at generate time")
		assert.Contains(t, string(resultBytes), "executed")
	})

	t.Run("function call referencing dependency is preserved", func(t *testing.T) {
		t.Parallel()

		var called bool

		expr, srcBytes := parseFirstAttrExpr(t, `val = danger(dependency.vpc.outputs.id)`)

		resultBytes, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: newCtx(&called), Deferred: testDeferred})
		require.NoError(t, err)

		assert.False(t, called, "a function call referencing dependency.* is preserved for unit-time evaluation")
		assert.Contains(t, string(resultBytes), "dependency.vpc.outputs.id")
	})
}

func TestPartialEval_PreservesConditionalReferencingDependency(t *testing.T) {
	t.Parallel()

	expr, srcBytes := parseFirstAttrExpr(t, `val = dependency.vpc.outputs.enabled ? "yes" : "no"`)

	resultBytes, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: buildEvalCtx(), Deferred: testDeferred})
	require.NoError(t, err)

	assert.Contains(t, string(resultBytes), `dependency.vpc.outputs.enabled ? "yes" : "no"`,
		"a conditional whose condition references dependency.* is preserved for unit-time evaluation")
}

// TestPartialEval_DeeplyNestedExpressionReturnsTypedError verifies depth-exhausted input returns source bytes plus PartialEvalDepthExceededError.
func TestPartialEval_DeeplyNestedExpressionReturnsTypedError(t *testing.T) {
	t.Parallel()

	const depth = 20000

	hcl := "val = " + strings.Repeat("(", depth) + "dependency.vpc.outputs.id" + strings.Repeat(")", depth)
	expr, srcBytes := parseFirstAttrExpr(t, hcl)

	resultBytes, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: buildEvalCtx(), Deferred: testDeferred})

	var depthErr hclparse.PartialEvalDepthExceededError

	require.ErrorAs(t, err, &depthErr)
	assert.NotEmpty(t, resultBytes, "deeply nested input must still return source-byte fallback so callers have valid HCL")
}

func TestPartialEval_ConditionalNullOrUnknownConditionReturnsTypedError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		flagVal cty.Value
		name    string
	}{
		{name: "null condition", flagVal: cty.NullVal(cty.Bool)},
		{name: "unknown condition", flagVal: cty.UnknownVal(cty.Bool)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			evalCtx := &hcl.EvalContext{
				Variables: map[string]cty.Value{
					"local": cty.ObjectVal(map[string]cty.Value{"flag": tc.flagVal}),
				},
			}

			expr, srcBytes := parseFirstAttrExpr(t, `val = local.flag ? "yes" : "no"`)

			resultBytes, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: evalCtx, Deferred: testDeferred})

			var condErr hclparse.PartialEvalUnresolvedError

			require.ErrorAs(t, err, &condErr)
			assert.Contains(t, string(resultBytes), `local.flag ? "yes" : "no"`, "null/unknown condition must still return source fallback so callers have valid HCL")
		})
	}
}

func TestPartialEval_FunctionCallArgumentsArePartiallyEvaluated(t *testing.T) {
	t.Parallel()

	evalCtx := buildEvalCtx()
	evalCtx.Variables["unit"] = cty.ObjectVal(map[string]cty.Value{
		"vpc": cty.ObjectVal(map[string]cty.Value{
			"path": cty.StringVal("/abs/vpc"),
		}),
	})
	evalCtx.Variables["stack"] = cty.ObjectVal(map[string]cty.Value{
		"network": cty.ObjectVal(map[string]cty.Value{
			"path": cty.StringVal("/abs/network"),
		}),
	})

	cases := []struct {
		name     string
		hcl      string
		contains []string
		excludes []string
	}{
		{
			name:     "unit ref argument alongside dependency",
			hcl:      `val = format("%s-%s", unit.vpc.path, dependency.vpc.outputs.id)`,
			contains: []string{`"/abs/vpc"`, "dependency.vpc.outputs.id"},
			excludes: []string{"unit.vpc.path"},
		},
		{
			name:     "stack ref argument alongside dependency",
			hcl:      `val = format("%s-%s", stack.network.path, dependency.vpc.outputs.id)`,
			contains: []string{`"/abs/network"`, "dependency.vpc.outputs.id"},
			excludes: []string{"stack.network.path"},
		},
		{
			name:     "template interpolation alongside dependency",
			hcl:      `val = "prefix-${format("%s-%s", unit.vpc.path, dependency.vpc.outputs.id)}"`,
			contains: []string{`"/abs/vpc"`, "dependency.vpc.outputs.id"},
			excludes: []string{"unit.vpc.path"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr, srcBytes := parseFirstAttrExpr(t, tc.hcl)

			resultBytes, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: evalCtx, Deferred: testDeferred})
			require.NoError(t, err)

			result := string(resultBytes)

			for _, want := range tc.contains {
				assert.Contains(t, result, want)
			}

			for _, notWant := range tc.excludes {
				assert.NotContains(t, result, notWant)
			}
		})
	}
}

// TestPartialEval_DeferFunctions pins the deferred-zone contract: a function call stays verbatim (and is not
// executed) while stack-level local.* still resolves, including inside the preserved call's arguments.
func TestPartialEval_DeferFunctions(t *testing.T) {
	t.Parallel()

	newCtx := func(called *bool) *hcl.EvalContext {
		evalCtx := buildEvalCtx()
		evalCtx.Functions = map[string]function.Function{
			"run_cmd": function.New(&function.Spec{
				VarParam: &function.Parameter{Type: cty.DynamicPseudoType},
				Type:     function.StaticReturnType(cty.String),
				Impl:     func([]cty.Value, cty.Type) (cty.Value, error) { *called = true; return cty.StringVal("ran"), nil },
			}),
			"flag_fn": function.New(&function.Spec{
				Type: function.StaticReturnType(cty.Bool),
				Impl: func([]cty.Value, cty.Type) (cty.Value, error) { *called = true; return cty.True, nil },
			}),
		}

		return evalCtx
	}

	eval := func(t *testing.T, called *bool, src string) string {
		t.Helper()

		expr, srcBytes := parseFirstAttrExpr(t, src)
		out, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: newCtx(called), Deferred: testDeferred, DeferFunctions: true})
		require.NoError(t, err)

		return string(out)
	}

	t.Run("function call verbatim, local argument rendered", func(t *testing.T) {
		t.Parallel()

		var called bool

		out := eval(t, &called, `val = run_cmd("echo", local.region)`)
		assert.False(t, called, "a function in a deferred zone must not run at generate time")
		assert.Contains(t, out, `run_cmd("echo", "us-east-1")`, "the call stays verbatim while the stack local renders")
		assert.NotContains(t, out, "local.region")
	})

	t.Run("function call in conditional condition verbatim", func(t *testing.T) {
		t.Parallel()

		var called bool

		out := eval(t, &called, `val = flag_fn() ? "a" : "b"`)
		assert.False(t, called)
		assert.Contains(t, out, "flag_fn()", "a function in a conditional condition stays verbatim in a deferred zone")
	})

	t.Run("function call in template verbatim, local rendered", func(t *testing.T) {
		t.Parallel()

		var called bool

		out := eval(t, &called, `val = "pre-${run_cmd("echo", local.region)}-post"`)
		assert.False(t, called)
		assert.Contains(t, out, `pre-${run_cmd("echo", "us-east-1")}-post`)
	})

	t.Run("a pure local still renders with DeferFunctions", func(t *testing.T) {
		t.Parallel()

		var called bool

		out := eval(t, &called, `val = local.region`)
		assert.False(t, called)
		assert.Contains(t, out, `"us-east-1"`)
	})
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
