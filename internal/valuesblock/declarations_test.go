package valuesblock_test

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/valuesblock"
)

// wantDiag is a diagnostic a test expects. An empty detail matches any detail.
type wantDiag struct {
	summary string
	detail  string
	line    int
}

func TestDecode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check func(t *testing.T, decls *valuesblock.Declarations)
		name  string
		src   string
		want  []wantDiag
	}{
		{
			name: "no values block",
			src:  `locals { a = 1 }`,
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()
				assert.Nil(t, decls)
			},
		},
		{
			name: "values attribute is not a block",
			src:  `values = { a = 1 }`,
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()
				assert.Nil(t, decls)
			},
		},
		{
			name: "empty values block",
			src:  `values {}`,
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()
				require.NotNil(t, decls)
				assert.Empty(t, decls.Values)
				assert.Equal(t, 1, decls.DefRange.Start.Line)
			},
		},
		{
			name: "second values block",
			src: `values {
  value "a" {}
}
values {
  value "b" {}
}`,
			want: []wantDiag{{
				summary: "Duplicate values block",
				detail:  "Only one values block is allowed. Another was defined at test.hcl:1,1-7.",
				line:    4,
			}},
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()
				assert.Equal(t, []string{"a"}, decls.Names())
			},
		},
		{
			name: "duplicate value name",
			src: `values {
  value "a" {}
  value "a" {}
}`,
			want: []wantDiag{{
				summary: "Duplicate value declaration",
				detail:  `The value "a" was already declared at test.hcl:2,3-12.`,
				line:    3,
			}},
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()
				assert.Len(t, decls.Values, 1)
			},
		},
		{
			name: "invalid value name",
			src: `values {
  value "1x" {}
}`,
			want: []wantDiag{{summary: "Invalid value name", line: 2}},
		},
		{
			name: "unexpected attribute in value",
			src: `values {
  value "a" {
    sensitive = true
  }
}`,
			want: []wantDiag{{summary: "Unsupported argument", line: 3}},
		},
		{
			name: "nullable",
			src: `values {
  value "set" {
    nullable = true
  }
  value "unset" {}
}`,
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()
				assert.True(t, lookup(t, decls, "set").Nullable)
				assert.False(t, lookup(t, decls, "unset").Nullable)
			},
		},
		{
			name: "invalid nullable",
			src: `values {
  value "a" {
    nullable = "maybe"
  }
  value "b" {
    nullable = null
  }
}`,
			want: []wantDiag{
				{summary: "Invalid nullable", detail: `The nullable attribute of value "a" must be true or false.`, line: 3},
				{summary: "Invalid nullable", detail: `The nullable attribute of value "b" must be true or false.`, line: 6},
			},
		},
		{
			name: "unexpected block in value",
			src: `values {
  value "a" {
    validation {}
  }
}`,
			want: []wantDiag{{summary: "Unsupported block type", line: 3}},
		},
		{
			name: "attribute directly in values",
			src: `values {
  a = 1
}`,
			want: []wantDiag{{summary: "Unsupported argument", line: 2}},
		},
		{
			name: "label on values block",
			src:  `values "x" {}`,
			want: []wantDiag{{summary: "Extraneous label for values", line: 1}},
		},
		{
			name: "types",
			src: `values {
  value "untyped" {}
  value "s" {
    type = string
  }
  value "m" {
    type = map(string)
  }
  value "o" {
    type = object({ a = string, b = optional(number, 3) })
  }
}`,
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()

				untyped := lookup(t, decls, "untyped")
				assert.Equal(t, cty.DynamicPseudoType, untyped.Type)
				assert.Nil(t, untyped.Default)

				s := lookup(t, decls, "s")
				assert.Equal(t, cty.String, s.Type)
				assert.Nil(t, s.TypeDefaults)

				assert.Equal(t, cty.Map(cty.String), lookup(t, decls, "m").Type)

				o := lookup(t, decls, "o")
				assert.Equal(t, cty.ObjectWithOptionalAttrs(
					map[string]cty.Type{"a": cty.String, "b": cty.Number},
					[]string{"b"},
				), o.Type)
				assert.NotNil(t, o.TypeDefaults)
			},
		},
		{
			name: "bad optional attribute default",
			src: `values {
  value "o" {
    type = object({ b = optional(number, "x") })
  }
}`,
			want: []wantDiag{{summary: "Invalid default value for optional attribute", line: 3}},
		},
		{
			name: "quoted type keyword",
			src: `values {
  value "s" {
    type = "string"
  }
}`,
			want: []wantDiag{{summary: "Invalid type specification", line: 3}},
		},
		{
			name: "bare collection type",
			src: `values {
  value "l" {
    type = list
  }
}`,
			want: []wantDiag{{summary: "Invalid type specification", line: 3}},
		},
		{
			name: "defaults that are not constants",
			src: `values {
  value "a" {
    default = local.x
  }
  value "b" {
    default = values.x
  }
  value "c" {
    default = { d = dependency.a.outputs.b }
  }
  value "e" {
    default = upper("a")
  }
}`,
			want: []wantDiag{
				{summary: "Variables not allowed", line: 3},
				{summary: "Variables not allowed", line: 6},
				{summary: "Variables not allowed", line: 9},
				{summary: "Function calls not allowed", line: 12},
			},
		},
		{
			name: "constant defaults",
			src: `values {
  value "template" {
    default = "${1+1}"
  }
  value "for" {
    default = [for x in [1] : x]
  }
  value "null" {
    default = null
  }
}`,
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()
				assertValue(t, cty.NumberIntVal(2), lookup(t, decls, "template").Default)
				assertValue(t, cty.TupleVal([]cty.Value{cty.NumberIntVal(1)}), lookup(t, decls, "for").Default)
				assertValue(t, cty.NullVal(cty.DynamicPseudoType), lookup(t, decls, "null").Default)
			},
		},
		{
			name: "default converted to type",
			src: `values {
  value "n" {
    type    = number
    default = "5"
  }
  value "o" {
    type    = object({ a = string, b = optional(number, 3) })
    default = { a = "x" }
  }
}`,
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()
				assertValue(t, cty.NumberIntVal(5), lookup(t, decls, "n").Default)
				assertValue(t, cty.ObjectVal(map[string]cty.Value{
					"a": cty.StringVal("x"),
					"b": cty.NumberIntVal(3),
				}), lookup(t, decls, "o").Default)
			},
		},
		{
			name: "default that does not conform to a list type",
			src: `values {
  value "l" {
    type    = list(number)
    default = ["x"]
  }
}`,
			want: []wantDiag{{
				summary: `Invalid default for value "l"`,
				detail: "The default does not match the type constraint of the value: " +
					"[0]: a number is required.",
				line: 4,
			}},
		},
		{
			name: "default that does not conform to an object type",
			src: `values {
  value "o" {
    type    = object({ a = number })
    default = { a = "x" }
  }
}`,
			want: []wantDiag{{
				summary: `Invalid default for value "o"`,
				detail: "The default does not match the type constraint of the value: " +
					`attribute "a": a number is required.`,
				line: 4,
			}},
		},
		{
			name: "description",
			src: `values {
  value "a" {
    description = "The a value."
  }
}`,
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()
				assert.Equal(t, "The a value.", lookup(t, decls, "a").Description)
			},
		},
		{
			name: "description that is not a string",
			src: `values {
  value "a" {
    description = ["x"]
  }
}`,
			want: []wantDiag{{
				summary: "Invalid description",
				detail:  `The description of value "a" must be a string.`,
				line:    3,
			}},
		},
		{
			name: "failed declaration stays declared and accepts anything",
			src: `values {
  value "a" {
    type    = list(number)
    default = local.x
  }
}`,
			want: []wantDiag{{summary: "Variables not allowed", line: 4}},
			check: func(t *testing.T, decls *valuesblock.Declarations) {
				t.Helper()

				a := lookup(t, decls, "a")
				assert.Equal(t, cty.DynamicPseudoType, a.Type)
				require.NotNil(t, a.Default)
				assert.False(t, a.Default.IsKnown())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			decls, diags := valuesblock.Decode(parseHCL(t, tc.src))
			assertDiags(t, tc.want, diags)

			if tc.check != nil {
				tc.check(t, decls)
			}
		})
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	body := parseJSON(t, `{
  "values": {
    "value": {
      "tags": {
        "type": "map(string)",
        "default": {}
      },
      "template": {
        "default": "${local.x}"
      },
      "o": {
        "type": "object({a=optional(number,3)})"
      }
    }
  }
}`)

	decls, diags := valuesblock.Decode(body)
	require.Empty(t, diags)

	assert.Equal(t, []string{"o", "tags", "template"}, decls.Names())

	tags := lookup(t, decls, "tags")
	assert.Equal(t, cty.Map(cty.String), tags.Type)
	assertValue(t, cty.MapValEmpty(cty.String), tags.Default)

	assertValue(t, cty.StringVal("${local.x}"), lookup(t, decls, "template").Default)

	o := lookup(t, decls, "o")
	assert.Equal(t, cty.ObjectWithOptionalAttrs(map[string]cty.Type{"a": cty.Number}, []string{"a"}), o.Type)
	assert.NotNil(t, o.TypeDefaults)
}

func TestDecodeJSONInvalidType(t *testing.T) {
	t.Parallel()

	body := parseJSON(t, `{"values": {"value": {"s": {"type": "\"string\""}}}}`)

	_, diags := valuesblock.Decode(body)
	assertDiags(t, []wantDiag{{summary: "Invalid type specification", line: 1}}, diags)
}

func TestDeclarationsLookup(t *testing.T) {
	t.Parallel()

	decls := decode(t, `values {
  value "b" {}
  value "a" {}
}`)

	assert.Equal(t, []string{"a", "b"}, decls.Names())
	assert.Equal(t, "b", decls.Values[0].Name)

	got, ok := decls.Lookup("a")
	require.True(t, ok)
	assert.Equal(t, "a", got.Name)
	assert.Equal(t, 3, got.DefRange.Start.Line)

	_, ok = decls.Lookup("c")
	assert.False(t, ok)
}

// decode returns the declarations in src, which must decode without diagnostics.
func decode(t *testing.T, src string) *valuesblock.Declarations {
	t.Helper()

	decls, diags := valuesblock.Decode(parseHCL(t, src))
	require.Empty(t, diags)
	require.NotNil(t, decls)

	return decls
}

// lookup returns the declaration of name in decls, which must declare it.
func lookup(t *testing.T, decls *valuesblock.Declarations, name string) *valuesblock.Declaration {
	t.Helper()

	require.NotNil(t, decls)

	decl, ok := decls.Lookup(name)
	require.True(t, ok, "value %q is not declared", name)

	return decl
}

// parseHCL parses src as the native HCL file test.hcl.
func parseHCL(t *testing.T, src string) hcl.Body {
	t.Helper()

	file, diags := hclsyntax.ParseConfig([]byte(src), "test.hcl", hcl.InitialPos)
	require.Empty(t, diags)

	return file.Body
}

// parseJSON parses src as the HCL JSON file test.hcl.json.
func parseJSON(t *testing.T, src string) hcl.Body {
	t.Helper()

	file, diags := json.Parse([]byte(src), "test.hcl.json")
	require.Empty(t, diags)

	return file.Body
}

// assertDiags asserts that got holds the diagnostics in want, in order.
func assertDiags(t *testing.T, want []wantDiag, got hcl.Diagnostics) {
	t.Helper()

	require.Len(t, got, len(want), "diagnostics: %v", got)

	for i, w := range want {
		assert.Equal(t, w.summary, got[i].Summary)

		if w.detail != "" {
			assert.Equal(t, w.detail, got[i].Detail)
		}

		require.NotNil(t, got[i].Subject, "diagnostic %q has no subject", got[i].Summary)
		assert.Equal(t, w.line, got[i].Subject.Start.Line, "line of %q", got[i].Summary)
	}
}

// assertValue asserts that got is want, marks included.
func assertValue(t *testing.T, want cty.Value, got *cty.Value) {
	t.Helper()

	require.NotNil(t, got)
	assert.True(t, want.RawEquals(*got), "want %#v, got %#v", want, *got)
}
