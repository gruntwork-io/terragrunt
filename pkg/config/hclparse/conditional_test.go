package hclparse_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func TestConditionalRunCmdTypesAndDiagnostics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expr       string
		condition  cty.Value
		want       cty.Value
		diagnostic string
		calls      []string
	}{
		{
			name:      "convert selected number",
			expr:      `condition ? run_cmd("A") : 42`,
			condition: cty.False, want: cty.StringVal("42"),
		},
		{
			name:      "typed null",
			expr:      `condition ? null : run_cmd("A")`,
			condition: cty.True, want: cty.NullVal(cty.String),
		},
		{
			name:      "incompatible branch types",
			expr:      `condition ? [] : run_cmd("A")`,
			condition: cty.False, calls: []string{"A"}, diagnostic: "Inconsistent conditional result types",
		},
		{
			name:      "unknown condition",
			expr:      `condition ? run_cmd("A") : run_cmd("B")`,
			condition: cty.UnknownVal(cty.Bool), want: cty.UnknownVal(cty.String),
		},
		{
			name:      "null condition",
			expr:      `condition ? run_cmd("A") : run_cmd("B")`,
			condition: cty.NullVal(cty.Bool), diagnostic: "Null condition",
		},
		{
			name:      "invalid condition type",
			expr:      `condition ? run_cmd("A") : run_cmd("B")`,
			condition: cty.NumberIntVal(1), diagnostic: "Incorrect condition type",
		},
		{
			name:      "convert string condition",
			expr:      `condition ? run_cmd("A") : run_cmd("B")`,
			condition: cty.StringVal("false"), want: cty.StringVal("B"), calls: []string{"B"},
		},
		{
			name:      "marked condition",
			expr:      `condition ? run_cmd("A") : run_cmd("B")`,
			condition: cty.True.Mark("sensitive"), want: cty.StringVal("A").Mark("sensitive"), calls: []string{"A"},
		},
		{
			name:      "selected command failure",
			expr:      `condition ? run_cmd("fail") : run_cmd("B")`,
			condition: cty.True, calls: []string{"fail"}, diagnostic: "Error in function call",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, err := hclparse.NewParser().ParseFromString("value = "+tt.expr, "test.hcl")
			require.NoError(t, err)
			attrs, err := file.JustAttributes()
			require.NoError(t, err)
			require.Len(t, attrs, 1)

			var calls []string

			ctx := conditionalRunCmdContext(tt.condition, func(name string) { calls = append(calls, name) })

			value, err := attrs[0].Value(ctx)
			if tt.diagnostic != "" {
				require.Error(t, err)

				var diags hcl.Diagnostics

				require.ErrorAs(t, err, &diags)
				require.NotEmpty(t, diags)
				assert.Equal(t, tt.diagnostic, diags[0].Summary)
			} else {
				require.NoError(t, err)
				assert.True(t, tt.want.RawEquals(value), "want %#v, got %#v", tt.want, value)
			}

			assert.Equal(t, tt.calls, calls)
		})
	}
}

func TestConditionalRunCmdWithRacing(t *testing.T) {
	t.Parallel()

	file, err := hclparse.NewParser().ParseFromString(`value = condition ? run_cmd("A") : run_cmd("B")`, "test.hcl")
	require.NoError(t, err)
	attrs, err := file.JustAttributes()
	require.NoError(t, err)
	require.Len(t, attrs, 1)

	for i := range 32 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			condition := i%2 == 0

			want := "B"
			if condition {
				want = "A"
			}

			var calls []string

			ctx := conditionalRunCmdContext(cty.BoolVal(condition), func(name string) { calls = append(calls, name) })

			value, err := attrs[0].Value(ctx)
			require.NoError(t, err)
			assert.True(t, cty.StringVal(want).RawEquals(value))
			assert.Equal(t, []string{want}, calls)
		})
	}

	t.Cleanup(func() {
		var calls []string

		_, diags := attrs[0].Expr.Value(conditionalRunCmdContext(cty.True, func(name string) { calls = append(calls, name) }))
		assert.False(t, diags.HasErrors())
		assert.Equal(t, []string{"A", "B"}, calls)
	})
}

func conditionalRunCmdContext(condition cty.Value, record func(string)) *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: map[string]cty.Value{"condition": condition},
		Functions: map[string]function.Function{
			"run_cmd": function.New(&function.Spec{
				Params: []function.Parameter{{Name: "command", Type: cty.String}},
				Type:   function.StaticReturnType(cty.String),
				Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
					name := args[0].AsString()
					record(name)

					if name == "fail" {
						return cty.NilVal, errors.New("command failed")
					}

					return cty.StringVal(name), nil
				},
			}),
		},
	}
}
