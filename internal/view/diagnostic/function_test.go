package diagnostic_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

var errUndecidedReturnType = errors.New("return type depends on the argument values")

// TestDescribeFunction pins the JSON signature of a function, with dynamic for types that have no JSON form.
func TestDescribeFunction(t *testing.T) {
	t.Parallel()

	handleType := cty.Capsule("handle", reflect.TypeFor[string]())

	testCases := []struct {
		expected *diagnostic.Function
		fn       function.Function
		name     string
	}{
		{
			name: "upper",
			fn:   stdlib.UpperFunc,
			expected: &diagnostic.Function{
				Name:       "upper",
				Params:     []diagnostic.FunctionParam{{Name: "str", Type: json.RawMessage(`"string"`)}},
				ReturnType: json.RawMessage(`"string"`),
			},
		},
		{
			name: "format",
			fn:   stdlib.FormatFunc,
			expected: &diagnostic.Function{
				Name:          "format",
				Params:        []diagnostic.FunctionParam{{Name: "format", Type: json.RawMessage(`"string"`)}},
				VariadicParam: &diagnostic.FunctionParam{Name: "args", Type: json.RawMessage(`"dynamic"`)},
				ReturnType:    json.RawMessage(`"string"`),
			},
		},
		{
			name: "open",
			fn: function.New(&function.Spec{
				Params: []function.Parameter{{Name: "handle", Type: handleType}},
				Type:   function.StaticReturnType(handleType),
			}),
			expected: &diagnostic.Function{
				Name:       "open",
				Params:     []diagnostic.FunctionParam{{Name: "handle", Type: json.RawMessage(`"dynamic"`)}},
				ReturnType: json.RawMessage(`"dynamic"`),
			},
		},
		{
			name: "decode",
			fn: function.New(&function.Spec{
				Params: []function.Parameter{{Name: "src", Type: cty.String}},
				Type: func([]cty.Value) (cty.Type, error) {
					return cty.NilType, errUndecidedReturnType
				},
			}),
			expected: &diagnostic.Function{
				Name:       "decode",
				Params:     []diagnostic.FunctionParam{{Name: "src", Type: json.RawMessage(`"string"`)}},
				ReturnType: json.RawMessage(`"dynamic"`),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, diagnostic.DescribeFunction(tc.name, tc.fn))
		})
	}
}

// TestDescribeFunctionCall pins that only a failed call to a known function names the call and its signature.
func TestDescribeFunctionCall(t *testing.T) {
	t.Parallel()

	upper := diagnostic.DescribeFunction("upper", stdlib.UpperFunc)

	testCases := []struct {
		expected *diagnostic.FunctionCall
		name     string
		src      string
		wrap     bool
	}{
		{
			name:     "call to a known function",
			src:      "value = upper(local.items)\n",
			expected: &diagnostic.FunctionCall{CalledAs: "upper", Signature: upper},
		},
		{
			name:     "call to a namespaced function",
			src:      "value = str::upper(local.items)\n",
			expected: &diagnostic.FunctionCall{CalledAs: "str::upper", Signature: upper},
		},
		{
			name:     "call information wrapped by another extra value",
			src:      "value = upper(local.items)\n",
			wrap:     true,
			expected: &diagnostic.FunctionCall{CalledAs: "upper", Signature: upper},
		},
		{
			name: "call to an unknown function",
			src:  "value = missing(local.items)\n",
		},
		{
			name: "error outside a function call",
			src:  "value = local.name + 1\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, diag := evaluateValue(t, tc.src)
			if tc.wrap {
				diag.Extra = wrapExtra{inner: diag.Extra}
			}

			assert.Equal(t, tc.expected, diagnostic.DescribeFunctionCall(diag))
		})
	}
}

// evaluateValue parses src and returns the file with the one diagnostic from evaluating its value attribute.
func evaluateValue(t *testing.T, src string) (*hcl.File, *hcl.Diagnostic) {
	t.Helper()

	file, parseDiags := hclsyntax.ParseConfig([]byte(src), "terragrunt.hcl", hcl.InitialPos)
	require.False(t, parseDiags.HasErrors(), parseDiags.Error())

	body, ok := file.Body.(*hclsyntax.Body)
	require.True(t, ok)

	attr, ok := body.Attributes["value"]
	require.True(t, ok, "source should set value")

	_, diags := attr.Expr.Value(functionEvalContext())
	require.Len(t, diags, 1, "evaluating value should report exactly one diagnostic")

	return file, diags[0]
}

// functionEvalContext returns an evaluation context with a few locals and string functions.
func functionEvalContext() *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"local": cty.ObjectVal(map[string]cty.Value{
				"name":  cty.StringVal("app"),
				"items": cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			}),
		},
		Functions: map[string]function.Function{
			"upper":      stdlib.UpperFunc,
			"str::upper": stdlib.UpperFunc,
			"format":     stdlib.FormatFunc,
		},
	}
}
