package hcldecode_test

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"

	"github.com/gruntwork-io/terragrunt/internal/vendored/hcl/hcldecode"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

type everyTag struct {
	Remain         hcl.Body       `hcl:",remain"`
	Expr           hcl.Expression `hcl:"expr,attr"`
	Value          cty.Value      `hcl:"value,optional"`
	Count          *int           `hcl:"count,attr"`
	Attr           *hcl.Attribute `hcl:"attr,attr"`
	Single         *child         `hcl:"single,block"`
	Name           string         `hcl:"name,attr"`
	Tags           []string       `hcl:"tags,optional"`
	Many           []child        `hcl:"many,block"`
	ManyPtr        []*child       `hcl:"many_ptr,block"`
	NameRange      hcl.Range      `hcl:"name,attr_range"`
	NameNameRange  hcl.Range      `hcl:"name,attr_name_range"`
	NameValueRange hcl.Range      `hcl:"name,attr_value_range"`
}

type child struct {
	Value      *string     `hcl:"value,attr"`
	Nested     *grandchild `hcl:"nested,block"`
	Label      string      `hcl:"label,label"`
	LabelRange hcl.Range   `hcl:"label,label_range"`
	DefRange   hcl.Range   `hcl:",def_range"`
	TypeRange  hcl.Range   `hcl:",type_range"`
}

type grandchild struct {
	Rest hcl.Attributes `hcl:",remain"`
	Flag bool           `hcl:"flag,attr"`
}

type requiredParts struct {
	Name string    `hcl:"name"`
	Only onlyBlock `hcl:"only,block"`
}

type onlyBlock struct {
	Size int `hcl:"size,attr"`
}

type nestedRemain struct {
	A    *string     `hcl:"a,attr"`
	Rest remainInner `hcl:",remain"`
}

type remainInner struct {
	Body hcl.Body `hcl:",body"`
	B    *string  `hcl:"b,attr"`
}

type mapRemain struct {
	Rest map[string]string `hcl:",remain"`
}

type decodeCase struct {
	newTarget func() any
	name      string
	src       string
	json      bool
}

func everyTagWithValues() any {
	return &everyTag{
		Single:  &child{Label: "old"},
		Many:    []child{{Label: "a"}, {Label: "b"}, {Label: "c"}},
		ManyPtr: []*child{{Label: "kept"}, nil},
	}
}

func decodeCases() []decodeCase {
	full := `
name  = "n-${var.x}"
count = 3
tags  = ["a", upper("b")]
expr  = var.x
attr  = 1 + 2
value = { k = "v" }
extra = "left over"

single "one" {
  value = "s"
  nested {
    flag  = true
    other = "kept"
  }
}

many "a" {}
many "b" {
  value = "mb"
}

many_ptr "p" {
  value = "mp"
}
`

	return []decodeCase{
		{name: "every tag", src: full, newTarget: func() any { return new(everyTag) }},
		{name: "every tag into a target with values", src: full, newTarget: everyTagWithValues},
		{
			name:      "absent optional parts",
			src:       `name = "n"`,
			newTarget: func() any { return new(everyTag) },
		},
		{
			name:      "absent parts into a target with values",
			src:       `name = "n"`,
			newTarget: everyTagWithValues,
		},
		{
			name:      "missing required attribute",
			src:       `count = 1`,
			newTarget: func() any { return new(everyTag) },
		},
		{
			name:      "unsuitable value",
			src:       `name = "n"` + "\n" + `count = "x"`,
			newTarget: func() any { return new(everyTag) },
		},
		{
			name:      "unknown variable",
			src:       `name = var.missing`,
			newTarget: func() any { return new(everyTag) },
		},
		{
			name:      "duplicate single block",
			src:       "name = \"n\"\nsingle \"a\" {}\nsingle \"b\" {}\n",
			newTarget: func() any { return new(everyTag) },
		},
		{
			name:      "unsupported argument in a block",
			src:       "name = \"n\"\nmany \"a\" {\n  nope = 1\n}\n",
			newTarget: func() any { return new(everyTag) },
		},
		{
			name:      "missing block label",
			src:       "name = \"n\"\nmany {}\n",
			newTarget: func() any { return new(everyTag) },
		},
		{
			name:      "missing required block",
			src:       `name = "n"`,
			newTarget: func() any { return new(requiredParts) },
		},
		{
			name:      "required block",
			src:       "name = \"n\"\nonly {\n  size = 2\n}\n",
			newTarget: func() any { return new(requiredParts) },
		},
		{
			name:      "unsupported argument without a remain field",
			src:       "name = \"n\"\nextra = 1\nonly {\n  size = 2\n}\n",
			newTarget: func() any { return new(requiredParts) },
		},
		{
			name:      "nested remain struct",
			src:       "a = \"x\"\nb = \"y\"\nc = \"z\"\n",
			newTarget: func() any { return new(nestedRemain) },
		},
		{
			name:      "map remain",
			src:       "a = \"x\"\nb = \"y\"\n",
			newTarget: func() any { return new(mapRemain) },
		},
		{
			name:      "map remain with a block",
			src:       "a = \"x\"\nb {}\n",
			newTarget: func() any { return new(mapRemain) },
		},
		{
			name:      "map target",
			src:       "a = \"x\"\nb = \"y\"\n",
			newTarget: func() any { return new(map[string]string) },
		},
		{
			name:      "json body",
			src:       `{"name": "n", "count": 2, "many": {"a": {"value": "j"}}, "extra": true}`,
			json:      true,
			newTarget: func() any { return new(everyTag) },
		},
	}
}

func TestDecodeBodyMatchesGohcl(t *testing.T) {
	t.Parallel()

	for _, tc := range decodeCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := parseBody(t, tc.src, tc.json)

			assertDecodesLikeGohcl(t, body, tc.newTarget)
		})
	}
}

func TestImpliedBodySchemaMatchesGohcl(t *testing.T) {
	t.Parallel()

	targets := []any{
		new(everyTag),
		new(child),
		new(grandchild),
		new(requiredParts),
		new(nestedRemain),
		new(remainInner),
		new(config.UnitConfig),
		new(config.TerraformHCL),
		new(config.RemoteStateHCL),
		new(config.RemoteStateBody),
		new(config.IncludeConfig),
	}

	for _, target := range targets {
		t.Run(fmt.Sprintf("%T", target), func(t *testing.T) {
			t.Parallel()

			wantSchema, wantPartial := gohcl.ImpliedBodySchema(target)
			gotSchema, gotPartial := hcldecode.ImpliedBodySchema(target)

			assert.Equal(t, wantSchema, gotSchema)
			assert.Equal(t, wantPartial, gotPartial)
		})
	}
}

func TestDecodeBodyConcurrentWithRacing(t *testing.T) {
	t.Parallel()

	type racingTarget struct {
		Name string  `hcl:"name,attr"`
		Many []child `hcl:"many,block"`
	}

	body := parseBody(t, "name = \"n\"\nmany \"a\" {}\n", false)

	var wg sync.WaitGroup

	results := make([]racingTarget, 16)
	for i := range results {
		wg.Go(func() {
			assert.Empty(t, hcldecode.DecodeBody(body, nil, &results[i]))
		})
	}

	wg.Wait()

	for _, result := range results {
		assert.Equal(t, "n", result.Name)
		assert.Equal(t, []string{"a"}, labels(result.Many))
	}
}

func FuzzDecodeBodyMatchesGohcl(f *testing.F) {
	for _, tc := range decodeCases() {
		if !tc.json {
			f.Add(tc.src)
		}
	}

	f.Fuzz(func(t *testing.T, src string) {
		file, diags := hclsyntax.ParseConfig([]byte(src), "fuzz.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			return
		}

		assertDecodesLikeGohcl(t, file.Body, func() any { return new(everyTag) })
	})
}

// assertDecodesLikeGohcl decodes body into a target from newTarget with gohcl and into another with hcldecode, and
// asserts that both decode the same values, report the same diagnostics, and panic alike.
func assertDecodesLikeGohcl(t *testing.T, body hcl.Body, newTarget func() any) {
	t.Helper()

	want, wantDiags, wantPanic := decodeWith(gohcl.DecodeBody, body, newTarget())
	got, gotDiags, gotPanic := decodeWith(hcldecode.DecodeBody, body, newTarget())

	require.Equal(t, wantPanic, gotPanic)
	assert.Equal(t, want, got)
	assert.Equal(t, diagnosticStrings(wantDiags), diagnosticStrings(gotDiags))
}

func decodeWith(
	decode func(hcl.Body, *hcl.EvalContext, any) hcl.Diagnostics,
	body hcl.Body,
	target any,
) (_ any, diags hcl.Diagnostics, panicked any) {
	defer func() {
		panicked = recover()
	}()

	diags = decode(body, evalContext(), target)

	return target, diags, nil
}

func evalContext() *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"var": cty.ObjectVal(map[string]cty.Value{"x": cty.StringVal("x")}),
		},
		Functions: map[string]function.Function{"upper": stdlib.UpperFunc},
	}
}

func parseBody(t *testing.T, src string, isJSON bool) hcl.Body {
	t.Helper()

	if isJSON {
		file, diags := hcljson.Parse([]byte(src), "test.json")
		require.False(t, diags.HasErrors(), diags.Error())

		return file.Body
	}

	file, diags := hclsyntax.ParseConfig([]byte(src), "test.hcl", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())

	return file.Body
}

// diagnosticStrings returns the text of each of diags, sorted, since gohcl reports the diagnostics of attributes in
// map order.
func diagnosticStrings(diags hcl.Diagnostics) []string {
	out := make([]string, 0, len(diags))
	for _, diag := range diags {
		out = append(out, fmt.Sprintf("%d %s", diag.Severity, diag.Error()))
	}

	slices.Sort(out)

	return out
}

func labels(children []child) []string {
	out := make([]string, 0, len(children))
	for i := range children {
		out = append(out, children[i].Label)
	}

	return out
}
