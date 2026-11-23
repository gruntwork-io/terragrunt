package valuesblock_test

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"

	"github.com/gruntwork-io/terragrunt/internal/valuesblock"
)

const resolveDecls = `values {
  value "tags" {
    type    = map(string)
    default = {}
  }
  value "count" {
    type = number
  }
  value "server" {
    type    = object({ name = string, port = optional(number, 80) })
    default = { name = "web" }
  }
}`

var (
	generating = valuesblock.Component{Kind: valuesblock.KindUnit, Address: "app", Location: "catalog/units/app"}
	reading    = valuesblock.Component{Kind: valuesblock.KindStack, Location: "live/terragrunt.stack.hcl"}
)

func TestResolve(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		want      cty.Value
		component valuesblock.Component
		name      string
		decls     string
		supply    string
		diags     []wantDiag
	}{
		{
			name:      "undeclared key with a suggestion",
			decls:     resolveDecls,
			component: generating,
			supply: `{
  count = 1
  tag   = { env = "dev" }
}`,
			diags: []wantDiag{{
				summary: `Undeclared value "tag" for unit "app"`,
				detail:  `The unit at catalog/units/app does not declare a value named "tag".` + "\n\n" + `Did you mean "tags"?`,
				line:    3,
			}},
			want: cty.ObjectVal(map[string]cty.Value{
				"tags":   cty.MapValEmpty(cty.String),
				"count":  cty.NumberIntVal(1),
				"server": cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("web"), "port": cty.NumberIntVal(80)}),
			}),
		},
		{
			name: "undeclared key without a suggestion",
			decls: `values {
  value "alpha" {}
  value "beta" {}
}`,
			component: reading,
			supply: `{
  alpha = 1
  beta  = 2
  zzz   = 3
}`,
			diags: []wantDiag{{
				summary: `Undeclared value "zzz"`,
				detail: `The stack at live/terragrunt.stack.hcl does not declare a value named "zzz".` + "\n\n" +
					`Declared values: alpha, beta.`,
				line: 4,
			}},
			want: cty.ObjectVal(map[string]cty.Value{"alpha": cty.NumberIntVal(1), "beta": cty.NumberIntVal(2)}),
		},
		{
			name:      "undeclared key in a block that declares nothing",
			decls:     `values {}`,
			component: reading,
			supply:    `{ a = 1 }`,
			diags: []wantDiag{{
				summary: `Undeclared value "a"`,
				detail: `The stack at live/terragrunt.stack.hcl does not declare a value named "a".` + "\n\n" +
					`The values block declares no values.`,
				line: 1,
			}},
			want: cty.EmptyObjectVal,
		},
		{
			name:      "computed keys point at the whole expression",
			decls:     resolveDecls,
			component: generating,
			supply: `merge(
  { count = 1 },
  { tag = {} },
)`,
			diags: []wantDiag{{summary: `Undeclared value "tag" for unit "app"`, line: 1}},
			want: cty.ObjectVal(map[string]cty.Value{
				"tags":   cty.MapValEmpty(cty.String),
				"count":  cty.NumberIntVal(1),
				"server": cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("web"), "port": cty.NumberIntVal(80)}),
			}),
		},
		{
			name:      "several errors in a stable order",
			decls:     resolveDecls,
			component: reading,
			supply: `{
  zeta   = 1
  alpha  = 2
  server = { port = 1 }
}`,
			diags: []wantDiag{
				{summary: `Undeclared value "alpha"`, line: 3},
				{summary: `Undeclared value "zeta"`, line: 2},
				{
					summary: `Invalid value "server"`,
					detail: `The value is not suitable for the type declared at test.hcl:9,3-17: ` +
						`attribute "name" is required.`,
					line: 4,
				},
				{summary: `Missing required value "count"`, line: 1},
			},
			want: cty.ObjectVal(map[string]cty.Value{
				"tags":  cty.MapValEmpty(cty.String),
				"count": cty.UnknownVal(cty.Number),
				"server": cty.UnknownVal(cty.Object(map[string]cty.Type{
					"name": cty.String,
					"port": cty.Number,
				})),
			}),
		},
		{
			name:      "missing required value",
			decls:     resolveDecls,
			component: generating,
			supply:    `{}`,
			diags: []wantDiag{{
				summary: `Missing required value "count" for unit "app"`,
				detail: `The unit at catalog/units/app requires the value "count", declared at test.hcl:6,3-16, ` +
					`and nothing sets it.`,
				line: 1,
			}},
			want: cty.ObjectVal(map[string]cty.Value{
				"tags":   cty.MapValEmpty(cty.String),
				"count":  cty.UnknownVal(cty.Number),
				"server": cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("web"), "port": cty.NumberIntVal(80)}),
			}),
		},
		{
			name: "several missing required values",
			decls: `values {
  value "dep" {}
  value "skip" {
    default = false
  }
  value "inc" {}
  value "env" {}
}`,
			component: generating,
			supply:    `{ skip = true }`,
			diags: []wantDiag{{
				summary: `Missing required values "dep", "inc", "env" for unit "app"`,
				detail: `The unit at catalog/units/app requires the values "dep" (declared at test.hcl:2,3-14), ` +
					`"inc" (declared at test.hcl:6,3-14) and "env" (declared at test.hcl:7,3-14), ` +
					`and nothing sets them.`,
				line: 1,
			}},
			want: cty.ObjectVal(map[string]cty.Value{
				"dep":  cty.DynamicVal,
				"skip": cty.True,
				"inc":  cty.DynamicVal,
				"env":  cty.DynamicVal,
			}),
		},
		{
			name:      "conversion and optional attribute defaults",
			decls:     resolveDecls,
			component: reading,
			supply: `{
  count  = "3"
  tags   = { env = "dev" }
  server = { name = "api" }
}`,
			want: cty.ObjectVal(map[string]cty.Value{
				"tags":   cty.MapVal(map[string]cty.Value{"env": cty.StringVal("dev")}),
				"count":  cty.NumberIntVal(3),
				"server": cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("api"), "port": cty.NumberIntVal(80)}),
			}),
		},
		{
			name: "nested attribute error",
			decls: `values {
  value "o" {
    type = object({ a = object({ b = number }) })
  }
}`,
			component: reading,
			supply:    `{ o = { a = { b = "x" } } }`,
			diags: []wantDiag{{
				summary: `Invalid value "o"`,
				detail: `The value is not suitable for the type declared at test.hcl:2,3-12: ` +
					`attribute "a.b": a number is required.`,
				line: 1,
			}},
			want: cty.ObjectVal(map[string]cty.Value{
				"o": cty.UnknownVal(cty.Object(map[string]cty.Type{
					"a": cty.Object(map[string]cty.Type{"b": cty.Number}),
				})),
			}),
		},
		{
			name: "element errors",
			decls: `values {
  value "l" {
    type = list(object({ a = number }))
  }
  value "m" {
    type = map(number)
  }
}`,
			component: reading,
			supply: `{
  l = [{ a = "x" }]
  m = { k = "y" }
}`,
			diags: []wantDiag{
				{
					summary: `Invalid value "l"`,
					detail: `The value is not suitable for the type declared at test.hcl:2,3-12: ` +
						`[0].a: a number is required.`,
					line: 2,
				},
				{
					summary: `Invalid value "m"`,
					detail: `The value is not suitable for the type declared at test.hcl:5,3-12: ` +
						`["k"]: a number is required.`,
					line: 3,
				},
			},
			want: cty.ObjectVal(map[string]cty.Value{
				"l": cty.UnknownVal(cty.List(cty.Object(map[string]cty.Type{"a": cty.Number}))),
				"m": cty.UnknownVal(cty.Map(cty.Number)),
			}),
		},
		{
			name: "supplied null for a nullable value stays a typed null",
			decls: `values {
  value "s" {
    type     = string
    default  = "d"
    nullable = true
  }
  value "any" {
    default  = "d"
    nullable = true
  }
}`,
			component: reading,
			supply:    `{ s = null, any = null }`,
			want: cty.ObjectVal(map[string]cty.Value{
				"s":   cty.NullVal(cty.String),
				"any": cty.NullVal(cty.DynamicPseudoType),
			}),
		},
		{
			name: "supplied null takes the default",
			decls: `values {
  value "s" {
    type    = string
    default = "d"
  }
  value "none" {
    default = null
  }
}`,
			component: reading,
			supply:    `{ s = null, none = null }`,
			want: cty.ObjectVal(map[string]cty.Value{
				"s":    cty.StringVal("d"),
				"none": cty.NullVal(cty.DynamicPseudoType),
			}),
		},
		{
			name: "supplied null for a required value",
			decls: `values {
  value "n" {
    type = number
  }
}`,
			component: generating,
			supply: `{
  n = null
}`,
			diags: []wantDiag{{
				summary: `Null value "n" for unit "app"`,
				detail: `The unit at catalog/units/app requires the value "n", declared at test.hcl:2,3-12, and does ` +
					`not accept null for it. Set a value, or set nullable = true in its declaration.`,
				line: 2,
			}},
			want: cty.ObjectVal(map[string]cty.Value{"n": cty.UnknownVal(cty.Number)}),
		},
		{
			name:      "supply that is not an object",
			decls:     resolveDecls,
			component: reading,
			supply:    `"x"`,
			diags: []wantDiag{{
				summary: "Invalid values",
				detail: "The values of the stack at live/terragrunt.stack.hcl must be an object, " +
					"but the given value has type string.",
				line: 1,
			}},
			want: cty.UnknownVal(cty.Object(map[string]cty.Type{
				"tags":   cty.Map(cty.String),
				"count":  cty.Number,
				"server": cty.Object(map[string]cty.Type{"name": cty.String, "port": cty.Number}),
			})),
		},
		{
			name: "null supply is empty",
			decls: `values {
  value "a" {
    default = 1
  }
}`,
			component: reading,
			supply:    `null`,
			want:      cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(1)}),
		},
		{
			name: "unknown element converts to a typed unknown",
			decls: `values {
  value "n" {
    type = number
  }
}`,
			component: reading,
			supply:    `{ n = unknown }`,
			want:      cty.ObjectVal(map[string]cty.Value{"n": cty.UnknownVal(cty.Number)}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			decls := decode(t, tc.decls)

			got, diags := decls.Resolve(supplyExpr(t, tc.supply), tc.component)
			assertDiags(t, tc.diags, diags)
			assertValue(t, tc.want, &got)
		})
	}
}

func TestResolveMapSupply(t *testing.T) {
	t.Parallel()

	decls := decode(t, `values {
  value "n" {
    type = number
  }
  value "o" {
    type = object({ a = string })
  }
}`)

	supplied := cty.MapVal(map[string]cty.Value{
		"n": cty.MapVal(map[string]cty.Value{"a": cty.StringVal("x")}),
		"o": cty.MapVal(map[string]cty.Value{"a": cty.StringVal("y")}),
	})

	got, diags := decls.Resolve(valuesblock.SuppliedByAttributes(supplied, nil, hcl.Range{}), reading)
	assertDiags(t, []wantDiag{{summary: `Invalid value "n"`, line: 0}}, diags)
	assertValue(t, cty.ObjectVal(map[string]cty.Value{
		"n": cty.UnknownVal(cty.Number),
		"o": cty.ObjectVal(map[string]cty.Value{"a": cty.StringVal("y")}),
	}), &got)
}

func TestResolveNothingSupplied(t *testing.T) {
	t.Parallel()

	at := hcl.Range{Filename: "terragrunt.hcl", Start: hcl.Pos{Line: 7}, End: hcl.Pos{Line: 7}}

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()

		decls := decode(t, `values {
  value "a" {
    default = "x"
  }
}`)

		got, diags := decls.Resolve(valuesblock.NothingSupplied(at), reading)
		require.Empty(t, diags)
		assertValue(t, cty.ObjectVal(map[string]cty.Value{"a": cty.StringVal("x")}), &got)
	})

	t.Run("required", func(t *testing.T) {
		t.Parallel()

		decls := decode(t, `values {
  value "a" {}
}`)

		got, diags := decls.Resolve(valuesblock.NothingSupplied(at), reading)
		assertDiags(t, []wantDiag{{summary: `Missing required value "a"`, line: 7}}, diags)
		assert.Equal(t, "terragrunt.hcl", diags[0].Subject.Filename)
		assertValue(t, cty.ObjectVal(map[string]cty.Value{"a": cty.DynamicVal}), &got)
	})

	t.Run("no declarations", func(t *testing.T) {
		t.Parallel()

		got, diags := decode(t, `values {}`).Resolve(valuesblock.NothingSupplied(at), reading)
		require.Empty(t, diags)
		assertValue(t, cty.EmptyObjectVal, &got)
	})
}

func TestResolveValuesFileAttributes(t *testing.T) {
	t.Parallel()

	file, diags := hclsyntax.ParseConfig([]byte(`count = 1
tag   = {}
`), "terragrunt.values.hcl", hcl.InitialPos)
	require.Empty(t, diags)

	attrs, diags := file.Body.JustAttributes()
	require.Empty(t, diags)

	supplied := cty.ObjectVal(map[string]cty.Value{"count": cty.NumberIntVal(1), "tag": cty.EmptyObjectVal})

	_, diags = decode(t, resolveDecls).Resolve(
		valuesblock.SuppliedByAttributes(supplied, attrs, file.Body.MissingItemRange()),
		reading,
	)
	assertDiags(t, []wantDiag{{summary: `Undeclared value "tag"`, line: 2}}, diags)
	assert.Equal(t, "terragrunt.values.hcl", diags[0].Subject.Filename)
	assert.Equal(t, 1, diags[0].Subject.Start.Column)
	assert.Equal(t, 11, diags[0].Subject.End.Column)
}

func TestResolveKeepsMarks(t *testing.T) {
	t.Parallel()

	decls := decode(t, `values {
  value "secret" {
    type = string
  }
  value "server" {
    type = object({ name = string, port = optional(number, 80) })
  }
}`)

	supplied := cty.ObjectVal(map[string]cty.Value{
		"secret": cty.NumberIntVal(1).Mark("sensitive"),
		"server": cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("api").Mark("sensitive")}),
	}).Mark("whole")

	got, diags := decls.Resolve(valuesblock.SuppliedByAttributes(supplied, nil, hcl.Range{}), reading)
	require.Empty(t, diags)
	assertValue(t, cty.ObjectVal(map[string]cty.Value{
		"secret": cty.StringVal("1").Mark("sensitive"),
		"server": cty.ObjectVal(map[string]cty.Value{
			"name": cty.StringVal("api").Mark("sensitive"),
			"port": cty.NumberIntVal(80),
		}),
	}).Mark("whole"), &got)
}

func TestResolveUnknownSupply(t *testing.T) {
	t.Parallel()

	got, diags := decode(t, resolveDecls).Resolve(
		valuesblock.SuppliedByAttributes(cty.DynamicVal, nil, hcl.Range{}),
		reading,
	)
	require.Empty(t, diags)
	assertValue(t, cty.UnknownVal(cty.Object(map[string]cty.Type{
		"tags":   cty.Map(cty.String),
		"count":  cty.Number,
		"server": cty.Object(map[string]cty.Type{"name": cty.String, "port": cty.Number}),
	})), &got)
}

func TestResolveFailedDeclarationAcceptsAnything(t *testing.T) {
	t.Parallel()

	decls, diags := valuesblock.Decode(parseHCL(t, `values {
  value "a" {
    type = list
  }
  value "b" {
    type    = number
    default = local.x
  }
}`))
	require.Len(t, diags, 2)

	supplied := cty.ObjectVal(map[string]cty.Value{"a": cty.StringVal("x")})

	got, diags := decls.Resolve(valuesblock.SuppliedByAttributes(supplied, nil, hcl.Range{}), reading)
	require.Empty(t, diags)
	assertValue(t, cty.ObjectVal(map[string]cty.Value{"a": cty.StringVal("x"), "b": cty.DynamicVal}), &got)
}

func TestResolveIsIdempotent(t *testing.T) {
	t.Parallel()

	decls := decode(t, resolveDecls)

	first, diags := decls.Resolve(supplyExpr(t, `{ count = "2", server = { name = "api" } }`), reading)
	require.Empty(t, diags)

	second, diags := decls.Resolve(valuesblock.SuppliedByAttributes(first, nil, hcl.Range{}), reading)
	require.Empty(t, diags)
	assertValue(t, first, &second)
}

// supplyExpr parses src as the values attribute of a block in stack.hcl and evaluates it with the merge function and
// an unknown variable.
func supplyExpr(t *testing.T, src string) *valuesblock.Supplied {
	t.Helper()

	expr, diags := hclsyntax.ParseExpression([]byte(src), "stack.hcl", hcl.InitialPos)
	require.Empty(t, diags)

	val, diags := expr.Value(&hcl.EvalContext{
		Variables: map[string]cty.Value{"unknown": cty.DynamicVal},
		Functions: map[string]function.Function{"merge": stdlib.MergeFunc},
	})
	require.Empty(t, diags)

	return valuesblock.SuppliedByExpr(val, expr)
}
