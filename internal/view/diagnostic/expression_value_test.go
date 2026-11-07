package diagnostic_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestSensitiveGoString(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		"marks.Sensitive",
		fmt.Sprintf("%#v", diagnostic.Sensitive),
		"the sensitive mark should print as a Go expression",
	)
}

// TestDescribeExpressionValues pins the statement made about each value an expression reads.
func TestDescribeExpressionValues(t *testing.T) {
	t.Parallel()

	handleType := cty.Capsule("handle", reflect.TypeFor[string]())
	handle := "socket"

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"local": cty.ObjectVal(map[string]cty.Value{
				"name":     cty.StringVal("app"),
				"count":    cty.NumberIntVal(3),
				"ratio":    cty.NumberFloatVal(1.5),
				"enabled":  cty.True,
				"disabled": cty.False,
				"nothing":  cty.NullVal(cty.String),
				"empty":    cty.ListValEmpty(cty.String),
				"one":      cty.ListVal([]cty.Value{cty.StringVal("a")}),
				"tags": cty.MapVal(map[string]cty.Value{
					"env":  cty.StringVal("prod"),
					"team": cty.StringVal("core"),
				}),
				"pair": cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.NumberIntVal(1)}),
				"ids":  cty.SetVal([]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)}),
				"none": cty.EmptyObjectVal,
				"single": cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("i-1"),
				}),
				"server": cty.ObjectVal(map[string]cty.Value{
					"id":   cty.StringVal("i-1"),
					"name": cty.StringVal("web"),
				}),
				"handle":  cty.CapsuleVal(handleType, &handle),
				"secret":  cty.StringVal("hunter2").Mark(diagnostic.Sensitive),
				"pending": cty.UnknownVal(cty.String),
				"later":   cty.DynamicVal,
			}),
		},
	}

	testCases := []struct {
		extra    any
		name     string
		expr     string
		expected []diagnostic.ExpressionValue
	}{
		{
			name: "known primitive values",
			expr: "[local.name, local.count, local.ratio, local.enabled, local.disabled, local.nothing]",
			expected: []diagnostic.ExpressionValue{
				{Traversal: "local.count", Statement: "is 3"},
				{Traversal: "local.disabled", Statement: "is false"},
				{Traversal: "local.enabled", Statement: "is true"},
				{Traversal: "local.name", Statement: `is "app"`},
				{Traversal: "local.nothing", Statement: "is null"},
				{Traversal: "local.ratio", Statement: "is 1.5"},
			},
		},
		{
			name: "collections",
			expr: "[local.empty, local.one, local.tags, local.pair, local.ids]",
			expected: []diagnostic.ExpressionValue{
				{Traversal: "local.empty", Statement: "is empty list of string"},
				{Traversal: "local.ids", Statement: "is set of number with 2 elements"},
				{Traversal: "local.one", Statement: "is list of string with 1 element"},
				{Traversal: "local.pair", Statement: "is tuple with 2 elements"},
				{Traversal: "local.tags", Statement: "is map of string with 2 elements"},
			},
		},
		{
			name: "objects and other types",
			expr: "[local.none, local.single, local.server, local.handle]",
			expected: []diagnostic.ExpressionValue{
				{Traversal: "local.handle", Statement: "is handle"},
				{Traversal: "local.none", Statement: "is object with no attributes"},
				{Traversal: "local.server", Statement: "is object with 2 attributes"},
				{Traversal: "local.single", Statement: `is object with 1 attribute "id"`},
			},
		},
		{
			name: "index steps",
			expr: `[local.one[0], local.tags["env"]]`,
			expected: []diagnostic.ExpressionValue{
				{Traversal: "local.one[0]", Statement: `is "a"`},
				{Traversal: `local.tags["env"]`, Statement: `is "prod"`},
			},
		},
		{
			name: "missing attributes fall back to the longest known prefix",
			expr: "[local.single.missing, local.missing]",
			expected: []diagnostic.ExpressionValue{
				{Traversal: "local.single", Statement: `is object with 1 attribute "id"`},
			},
		},
		{
			name: "repeated traversal",
			expr: "local.name == local.name",
			expected: []diagnostic.ExpressionValue{
				{Traversal: "local.name", Statement: `is "app"`},
			},
		},
		{
			name: "unknown and sensitive values not behind the diagnostic",
			expr: "[local.secret, local.pending, local.later]",
			expected: []diagnostic.ExpressionValue{
				{Traversal: "local.pending", Statement: "is a string"},
			},
		},
		{
			name:  "unknown and sensitive values behind the diagnostic",
			expr:  "[local.secret, local.pending, local.later]",
			extra: causeExtra{unknown: true, sensitive: true},
			expected: []diagnostic.ExpressionValue{
				{Traversal: "local.later", Statement: "will be known only after apply"},
				{Traversal: "local.pending", Statement: "is a string, known only after apply"},
				{Traversal: "local.secret", Statement: "has a sensitive value"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diag := &hcl.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     "Invalid value",
				Expression:  parseExpression(t, tc.expr),
				EvalContext: evalCtx,
				Extra:       tc.extra,
			}

			assert.Equal(t, tc.expected, diagnostic.DescribeExpressionValues(diag))
		})
	}
}

// parseExpression parses src as a native syntax expression.
func parseExpression(t *testing.T, src string) hcl.Expression {
	t.Helper()

	expr, diags := hclsyntax.ParseExpression([]byte(src), "terragrunt.hcl", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())

	return expr
}
