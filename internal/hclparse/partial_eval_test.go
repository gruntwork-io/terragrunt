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
			// A generate-time-known condition collapses outright to the chosen branch: the unchosen
			// branch and the conditional operator are dropped, so the deferred dependency arm disappears.
			name:     "conditional pure condition true collapses to the chosen branch",
			hcl:      `val = local.flag ? "yes" : dependency.vpc.outputs.vpc_id`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"yes"`},
			excludes: []string{"dependency.vpc.outputs.vpc_id", "?"},
		},
		{
			name:     "conditional pure condition false collapses to the chosen branch",
			hcl:      `val = !local.flag ? "yes" : dependency.vpc.outputs.vpc_id`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"dependency.vpc.outputs.vpc_id"},
			excludes: []string{`"yes"`, "?"},
		},
		{
			name:     "conditional deferred condition",
			hcl:      `val = dependency.vpc.outputs.ready ? "yes" : "no"`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"dependency.vpc.outputs.ready", `"yes"`, `"no"`},
		},
		{
			// A deferred condition keeps the conditional structure while each arm is partially evaluated:
			// the local.* arm renders to a literal at generate time, the dependency.* arm stays verbatim.
			name:     "conditional deferred condition renders local arm, defers dependency arm",
			hcl:      `val = dependency.vpc.outputs.ready ? local.region : dependency.vpc.outputs.vpc_id`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`dependency.vpc.outputs.ready ?`, `"us-east-1"`, "dependency.vpc.outputs.vpc_id"},
			excludes: []string{"local.region"},
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
			contains: []string{"3", "dependency.vpc.outputs.count"},
			excludes: []string{"local.count"},
		},
		{
			// Regression: a number-literal interpolation (${0}) in a template that takes the structural path
			// (it references dependency.*) must not panic; the literal renders and the dependency stays verbatim.
			name:     "number literal interpolation with deferred",
			hcl:      `val = "${0}-${dependency.vpc.outputs.id}"`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"0-${dependency.vpc.outputs.id}"`},
		},
		{
			name:     "bool literal interpolation with deferred",
			hcl:      `val = "${true}-${dependency.vpc.outputs.id}"`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"true-${dependency.vpc.outputs.id}"`},
		},
		{
			name:     "float literal interpolation with deferred",
			hcl:      `val = "${1.5}-${dependency.vpc.outputs.id}"`,
			evalCtx:  buildEvalCtx(),
			contains: []string{`"1.5-${dependency.vpc.outputs.id}"`},
		},
		{
			// A null literal can't be stringified: it stays an interpolation so the runtime
			// produces a faithful error instead of generate baking in a wrong value.
			name:     "null literal interpolation with deferred stays deferred",
			hcl:      `val = "${null}-${dependency.vpc.outputs.id}"`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"${null}", "${dependency.vpc.outputs.id}"},
		},
		{
			// A pure part whose value can't convert to string (a tuple) is emitted back as an
			// interpolation rather than rendered inline.
			name:     "non-string-convertible interpolation with deferred stays deferred",
			hcl:      `val = "${[1, 2]}-${dependency.vpc.outputs.id}"`,
			evalCtx:  buildEvalCtx(),
			contains: []string{"${[1, 2]}", "${dependency.vpc.outputs.id}"},
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

// TestPartialEval_NonFiniteNumberFallsBackToSource checks a non-finite result (1 / 0 -> +Inf) falls back to verbatim source, not an invalid bare "Inf".
func TestPartialEval_NonFiniteNumberFallsBackToSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hcl      string
		contains string
	}{
		{name: "positive infinity from division", hcl: `val = 1 / 0`, contains: "1 / 0"},
		{name: "negative infinity from division", hcl: `val = -1 / 0`, contains: "-1 / 0"},
		{name: "infinity nested in tuple", hcl: `val = [1 / 0]`, contains: "1 / 0"},
		{name: "infinity nested in object", hcl: `val = { v = 1 / 0 }`, contains: "1 / 0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr, srcBytes := parseFirstAttrExpr(t, tc.hcl)

			resultBytes, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: buildEvalCtx(), Deferred: testDeferred})
			require.NoError(t, err)

			result := string(resultBytes)

			assert.NotContains(t, result, "Inf", "non-finite number must not render as an Inf identifier, got %q", result)
			assert.Contains(t, result, tc.contains, "expected verbatim source fallback, got %q", result)

			_, diags := hclsyntax.ParseExpression(resultBytes, "result.hcl", hcl.Pos{Line: 1, Column: 1})
			assert.False(t, diags.HasErrors(), "partial-eval result must be valid HCL, got %q: %s", result, diags.Error())
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

// TestPartialEval_FunctionResolution pins that a function call resolves at generate time unless it references the
// deferred dependency.* root, in which case the call (and the dependency reference) stays verbatim for the generated
// unit. Stack-level local.* always renders, including inside a preserved call.
func TestPartialEval_FunctionResolution(t *testing.T) {
	t.Parallel()

	newCtx := func(called *bool) *hcl.EvalContext {
		evalCtx := buildEvalCtx()
		evalCtx.Variables["dependency"] = cty.ObjectVal(map[string]cty.Value{
			"vpc": cty.ObjectVal(map[string]cty.Value{
				"outputs": cty.ObjectVal(map[string]cty.Value{
					"region": cty.StringVal("eu-west-1"),
				}),
			}),
		})
		evalCtx.Functions = map[string]function.Function{
			"upper": function.New(&function.Spec{
				Params: []function.Parameter{{Type: cty.String}},
				Type:   function.StaticReturnType(cty.String),
				Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
					*called = true

					return cty.StringVal(strings.ToUpper(args[0].AsString())), nil
				},
			}),
			"tag": function.New(&function.Spec{
				VarParam: &function.Parameter{Type: cty.String},
				Type:     function.StaticReturnType(cty.String),
				Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
					*called = true

					parts := make([]string, len(args))
					for i, a := range args {
						parts[i] = a.AsString()
					}

					return cty.StringVal(strings.Join(parts, "-")), nil
				},
			}),
		}

		return evalCtx
	}

	eval := func(t *testing.T, called *bool, src string) string {
		t.Helper()

		expr, srcBytes := parseFirstAttrExpr(t, src)
		out, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: newCtx(called), Deferred: testDeferred})
		require.NoError(t, err)

		return string(out)
	}

	t.Run("pure function call resolves at generate time", func(t *testing.T) {
		t.Parallel()

		var called bool

		out := eval(t, &called, `val = upper(local.region)`)
		assert.True(t, called, "a function with no dependency.* reference runs at generate time")
		assert.Contains(t, out, `"US-EAST-1"`)
		assert.NotContains(t, out, "local.region")
	})

	t.Run("function referencing dependency stays verbatim and does not run", func(t *testing.T) {
		t.Parallel()

		var called bool

		out := eval(t, &called, `val = upper(dependency.vpc.outputs.region)`)
		assert.False(t, called, "a function whose argument references dependency.* must not run at generate time")
		assert.Contains(t, out, "upper(dependency.vpc.outputs.region)", "the call stays verbatim for the generated unit")
	})

	t.Run("deferred call renders its sibling local arg", func(t *testing.T) {
		t.Parallel()

		var called bool

		out := eval(t, &called, `val = tag(local.region, dependency.vpc.outputs.region)`)
		assert.False(t, called, "a call referencing dependency.* must not run at generate time")
		assert.Contains(t, out, `tag("us-east-1", dependency.vpc.outputs.region)`, "the stack local renders while the dependency arg defers")
		assert.NotContains(t, out, "local.region")
	})

	t.Run("function call in a conditional condition resolves", func(t *testing.T) {
		t.Parallel()

		var called bool

		out := eval(t, &called, `val = upper(local.region) == "US-EAST-1" ? "a" : "b"`)
		assert.True(t, called)
		assert.Contains(t, out, `"a"`, "the condition resolves at generate time and selects the true branch")
	})
}

// TestPartialEval_CompositeExpressions pins that for/splat/binary-op/unary-op/index expressions render their
// non-deferred stack references (local.*) while keeping the deferred parts (dependency.* and loop bodies operating
// on loop variables) verbatim, so the generated unit never sees an unresolved stack-level reference.
func TestPartialEval_CompositeExpressions(t *testing.T) {
	t.Parallel()

	eval := func(t *testing.T, src string) string {
		t.Helper()

		expr, srcBytes := parseFirstAttrExpr(t, src)
		out, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: buildEvalCtx(), Deferred: testDeferred})
		require.NoError(t, err)

		return string(out)
	}

	t.Run("binary op renders local and defers dependency", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = local.region == dependency.vpc.outputs.region`)
		assert.Contains(t, out, `"us-east-1" ==`, "the stack local renders")
		assert.Contains(t, out, "dependency.vpc.outputs.region", "the dependency reference stays verbatim")
		assert.NotContains(t, out, "local.region")
	})

	t.Run("unary op defers dependency", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = !dependency.vpc.outputs.enabled`)
		assert.Contains(t, out, "!dependency.vpc.outputs.enabled")
	})

	t.Run("index renders local key and defers dependency collection", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = dependency.vpc.outputs.subnets[local.count]`)
		assert.Contains(t, out, "dependency.vpc.outputs.subnets[3]", "the local index renders while the dependency collection defers")
		assert.NotContains(t, out, "local.count")
	})

	t.Run("for renders collection local and defers the loop body", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = [for x in local.names : upper(x)]`)
		assert.Contains(t, out, `["a", "b"]`, "the collection local renders")
		assert.Contains(t, out, "upper(x)", "the loop body runs in the unit and stays verbatim")
		assert.NotContains(t, out, "local.names")
	})

	t.Run("for over a dependency collection stays verbatim", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = [for x in dependency.vpc.outputs.names : x]`)
		assert.Contains(t, out, "[for x in dependency.vpc.outputs.names : x]")
	})

	t.Run("for renders a stack local in the body and defers the loop var", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = [for s in dependency.vpc.outputs.subnets : local.region]`)
		assert.Contains(t, out, `"us-east-1"`, "the stack local in the loop body renders")
		assert.Contains(t, out, "dependency.vpc.outputs.subnets", "the dependency collection defers")
		assert.NotContains(t, out, "local.region")
	})

	t.Run("loop var shadowing a namespace does not leak to a sibling after the for", func(t *testing.T) {
		t.Parallel()

		// The loop var `local` shadows the local namespace inside the for; after the for the deferred set must be
		// restored so the sibling local.region renders instead of being kept verbatim as a still-deferred root.
		out := eval(t, `val = [[for local in dependency.vpc.outputs.a : local], local.region]`)
		assert.Contains(t, out, `"us-east-1"`, "the sibling local.region renders after the for, proving the deferred set was restored")
		assert.Contains(t, out, "[for local in dependency.vpc.outputs.a : local]", "the loop var stays verbatim inside the for")
	})

	t.Run("nested for keeps both loop vars verbatim and renders the body local", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = [for x in dependency.vpc.outputs.a : [for x in dependency.vpc.outputs.b : "${x}-${local.region}"]]`)
		assert.Contains(t, out, "${x}-us-east-1", "the body local renders while the loop var stays verbatim")
		assert.NotContains(t, out, "${local.region}", "local.region must not stay verbatim")
	})

	t.Run("splat over a dependency source stays verbatim", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = dependency.vpc.outputs.subnets[*].id`)
		assert.Contains(t, out, "dependency.vpc.outputs.subnets[*].id")
	})
}

// TestPartialEval_ObjectKeys pins that an expression key resolves at generate time unless it references the deferred dependency.* root, while a literal-name key keeps its source form, so the generated unit never sees an unresolved stack-level reference in a key.
func TestPartialEval_ObjectKeys(t *testing.T) {
	t.Parallel()

	evalCtx := buildEvalCtx()
	evalCtx.Functions = map[string]function.Function{
		"upper": function.New(&function.Spec{
			Params: []function.Parameter{{Type: cty.String}},
			Type:   function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
				return cty.StringVal(strings.ToUpper(args[0].AsString())), nil
			},
		}),
	}

	tests := []struct {
		name     string
		hcl      string
		contains []string
		excludes []string
	}{
		{
			// The reported bug: a deferred value forces the structural path; the interpolated key must still resolve.
			name:     "interpolated key with deferred value resolves",
			hcl:      `val = { "${local.env}_key" = dependency.vpc.outputs.vpc_id }`,
			contains: []string{`"production_key"`, "dependency.vpc.outputs.vpc_id"},
			excludes: []string{"local.env"},
		},
		{
			name:     "interpolated key resolves in a nested object",
			hcl:      `val = { outer = { "${local.env}_key" = dependency.vpc.outputs.vpc_id } }`,
			contains: []string{`"production_key"`, "dependency.vpc.outputs.vpc_id"},
			excludes: []string{"local.env"},
		},
		{
			name:     "naked identifier key stays verbatim",
			hcl:      `val = { name = dependency.vpc.outputs.vpc_id }`,
			contains: []string{"name"},
			excludes: []string{`"name"`},
		},
		{
			name:     "quoted literal key keeps its content",
			hcl:      `val = { "my-key" = dependency.vpc.outputs.vpc_id }`,
			contains: []string{`"my-key"`},
		},
		{
			name:     "key deferring to dependency stays verbatim while the pure value renders",
			hcl:      `val = { "${dependency.vpc.outputs.vpc_id}_key" = local.env }`,
			contains: []string{"${dependency.vpc.outputs.vpc_id}", `"production"`},
			excludes: []string{"local.env"},
		},
		{
			name:     "mixed template key renders the local part and defers the dependency part",
			hcl:      `val = { "${local.env}-${dependency.vpc.outputs.vpc_id}" = "x" }`,
			contains: []string{"production-${dependency.vpc.outputs.vpc_id}"},
			excludes: []string{"local.env"},
		},
		{
			name:     "function call key resolves at generate time",
			hcl:      `val = { "${upper(local.env)}_key" = dependency.vpc.outputs.vpc_id }`,
			contains: []string{`"PRODUCTION_key"`},
			excludes: []string{"upper(", "local.env"},
		},
		{
			name:     "parenthesized key resolves as an expression",
			hcl:      `val = { (local.env) = dependency.vpc.outputs.vpc_id }`,
			contains: []string{`"production"`},
			excludes: []string{"local.env", "("},
		},
		{
			name:     "parenthesized key deferring to dependency stays verbatim",
			hcl:      `val = { (dependency.vpc.outputs.vpc_id) = "x" }`,
			contains: []string{"(dependency.vpc.outputs.vpc_id)"},
		},
		{
			name:     "keyword key stays verbatim",
			hcl:      `val = { true = dependency.vpc.outputs.vpc_id }`,
			contains: []string{"true"},
			excludes: []string{`"true"`},
		},
		{
			// HCL rejects a naked multi-step traversal key as ambiguous, so it keeps its source form instead of silently resolving as a reference.
			name:     "naked multi-step traversal key stays verbatim",
			hcl:      `val = { local.env = dependency.vpc.outputs.vpc_id }`,
			contains: []string{"local.env"},
			excludes: []string{`"production"`},
		},
		{
			// A single-interpolation key (TemplateWrapExpr) must re-emit its "${...}" wrapper: a naked traversal key is ambiguous HCL.
			name:     "deferred single-interpolation key keeps its quotes",
			hcl:      `val = { "${dependency.vpc.outputs.vpc_id}" = "x" }`,
			contains: []string{`"${dependency.vpc.outputs.vpc_id}"`},
		},
		{
			name:     "pure single-interpolation key resolves",
			hcl:      `val = { "${local.env}" = dependency.vpc.outputs.vpc_id }`,
			contains: []string{`"production"`},
			excludes: []string{"local.env"},
		},
		{
			// A naked loop-var identifier would be a literal-string key, silently collapsing every element to the same key.
			name:     "loop-var single-interpolation key keeps its quotes",
			hcl:      `val = [for k in ["a", "b"] : { "${k}" = dependency.vpc.outputs.vpc_id }]`,
			contains: []string{`"${k}"`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr, srcBytes := parseFirstAttrExpr(t, tc.hcl)

			resultBytes, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: evalCtx, Deferred: testDeferred})
			require.NoError(t, err)

			result := string(resultBytes)

			// Substring assertions cannot catch a malformed key rendering, so the emitted HCL must also parse.
			_, diags := hclsyntax.ParseConfig([]byte("attr = "+result), "out.hcl", hcl.Pos{Line: 1, Column: 1})
			require.False(t, diags.HasErrors(), "emitted HCL must parse, got %q: %s", result, diags.Error())

			for _, want := range tc.contains {
				assert.Contains(t, result, want, "expected result to contain %q, got %q", want, result)
			}

			for _, notWant := range tc.excludes {
				assert.NotContains(t, result, notWant, "expected result NOT to contain %q, got %q", notWant, result)
			}
		})
	}
}

// TestPartialEval_ObjectKeyUndefinedReferenceFails pins that an object key referencing a name missing from the stack context surfaces a typed error (failing generation) instead of leaking the unresolved reference into the generated unit.
func TestPartialEval_ObjectKeyUndefinedReferenceFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hcl  string
	}{
		{
			name: "template key with undefined local",
			hcl:  `val = { "${local.missing}_k" = dependency.vpc.outputs.vpc_id }`,
		},
		{
			name: "single-interpolation key with undefined local",
			hcl:  `val = { "${local.missing}" = dependency.vpc.outputs.vpc_id }`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr, srcBytes := parseFirstAttrExpr(t, tc.hcl)

			_, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: buildEvalCtx(), Deferred: testDeferred})
			require.Error(t, err, "a key referencing an undefined stack name must fail generation")

			var unresolvedErr hclparse.PartialEvalUnresolvedError
			require.ErrorAs(t, err, &unresolvedErr)
		})
	}
}

// TestPartialEval_TemplateDirectives pins that a %{ for }/%{ if } directive on the structural path is emitted verbatim as a unit (re-emitting it inside ${...} would corrupt the output) while the template's other parts still partially evaluate, and that a generate-time-knowable directive template resolves on the fast path.
func TestPartialEval_TemplateDirectives(t *testing.T) {
	t.Parallel()

	eval := func(t *testing.T, src string) string {
		t.Helper()

		expr, srcBytes := parseFirstAttrExpr(t, src)
		out, err := hclparse.PartialEval(expr, &hclparse.EvalArgs{SrcBytes: srcBytes, EvalCtx: buildEvalCtx(), Deferred: testDeferred})
		require.NoError(t, err)

		return string(out)
	}

	requireValidHCL := func(t *testing.T, out string) {
		t.Helper()

		_, diags := hclsyntax.ParseConfig([]byte("attr = "+out), "out.hcl", hcl.Pos{Line: 1, Column: 1})
		require.False(t, diags.HasErrors(), "the emitted template must be valid HCL, got %q: %s", out, diags.Error())
	}

	t.Run("for directive with a deferred body stays verbatim and valid", func(t *testing.T) {
		t.Parallel()

		tmpl := `"%{ for x in local.names }${x}-${dependency.vpc.outputs.id}%{ endfor }"`
		out := eval(t, `val = `+tmpl)
		assert.Equal(t, tmpl, out, "a directive template on the structural path is emitted verbatim")
		requireValidHCL(t, out)
	})

	t.Run("if directive with a deferred condition stays verbatim and valid", func(t *testing.T) {
		t.Parallel()

		tmpl := `"prefix-%{ if dependency.vpc.outputs.ready }yes%{ endif }"`
		out := eval(t, `val = `+tmpl)
		assert.Equal(t, tmpl, out)
		requireValidHCL(t, out)
	})

	t.Run("if directive with a deferred body stays verbatim and valid", func(t *testing.T) {
		t.Parallel()

		tmpl := `"%{ if local.flag }${dependency.vpc.outputs.id}%{ endif }"`
		out := eval(t, `val = `+tmpl)
		assert.Equal(t, tmpl, out)
		requireValidHCL(t, out)
	})

	t.Run("genuine conditional interpolation with a deferred condition is not mistaken for a directive", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = "v-${dependency.vpc.outputs.ready ? local.env : "b"}"`)
		assert.Equal(t, `"v-${dependency.vpc.outputs.ready ? "production" : "b"}"`, out,
			"a real ${cond ? a : b} part keeps its interpolation wrapping while its pure arms render")
		requireValidHCL(t, out)
	})

	t.Run("genuine conditional interpolation with a deferred arm is not mistaken for a directive", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = "v-${!local.flag ? local.env : dependency.vpc.outputs.id}"`)
		assert.Equal(t, `"v-${dependency.vpc.outputs.id}"`, out,
			"a real conditional part collapses to its chosen branch instead of being emitted as a directive")
		requireValidHCL(t, out)
	})

	t.Run("pure interpolation resolves next to a deferred directive", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = "${local.env}-%{ if dependency.vpc.outputs.ready }yes%{ endif }"`)
		assert.Equal(t, `"production-%{ if dependency.vpc.outputs.ready }yes%{ endif }"`, out,
			"the stack local renders while the directive stays verbatim as a unit")
		requireValidHCL(t, out)
	})

	t.Run("pure directive template resolves on the fast path", func(t *testing.T) {
		t.Parallel()

		out := eval(t, `val = "%{ for x in local.names }${x},%{ endfor }"`)
		assert.Equal(t, `"a,b,"`, out, "a generate-time-knowable directive template resolves to its literal")
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
				"names":  cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			}),
		},
	}
}
