package hclhelper_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclhelper"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

func TestTokensForValueRoundTrips(t *testing.T) {
	t.Parallel()

	forObject := cty.ObjectVal(map[string]cty.Value{"for": cty.StringVal("x"), "zone": cty.StringVal("y")})

	testCases := []struct {
		value cty.Value
		name  string
	}{
		{name: "for as the only key", value: cty.ObjectVal(map[string]cty.Value{"for": cty.StringVal("x")})},
		{name: "for as the first key", value: forObject},
		{name: "for after another key", value: cty.ObjectVal(map[string]cty.Value{
			"a":   cty.StringVal("x"),
			"for": cty.StringVal("y"),
		})},
		{name: "for in a map", value: cty.MapVal(map[string]cty.Value{"for": cty.StringVal("x")})},
		{name: "for in nested objects", value: cty.ObjectVal(map[string]cty.Value{
			"for": cty.ObjectVal(map[string]cty.Value{"for": forObject}),
		})},
		{name: "object in a tuple", value: cty.TupleVal([]cty.Value{cty.StringVal("for"), forObject})},
		{name: "object in a tuple in a tuple", value: cty.TupleVal([]cty.Value{cty.TupleVal([]cty.Value{forObject})})},
		{name: "object in a list", value: cty.ListVal([]cty.Value{forObject, forObject})},
		{name: "object in a set", value: cty.SetVal([]cty.Value{forObject})},
		{name: "map in a map", value: cty.MapVal(map[string]cty.Value{
			"for": cty.MapVal(map[string]cty.Value{"for": cty.True}),
		})},
		{name: "keys that are not identifiers", value: cty.ObjectVal(map[string]cty.Value{
			"":         cty.NumberIntVal(1),
			"1a":       cty.NumberIntVal(2),
			"a b":      cty.NumberIntVal(3),
			"a-b":      cty.NumberIntVal(4),
			"a.b":      cty.NumberIntVal(5),
			"${a}":     cty.NumberIntVal(6),
			"%{if b}":  cty.NumberIntVal(7),
			`"quoted"`: cty.NumberIntVal(8),
		})},
		{name: "for as a string", value: cty.StringVal("for")},
		{name: "null in an object", value: cty.ObjectVal(map[string]cty.Value{"for": cty.NullVal(cty.String)})},
		{name: "null object", value: cty.NullVal(cty.Object(map[string]cty.Type{"for": cty.String}))},
		{name: "empty object", value: cty.EmptyObjectVal},
		{name: "empty tuple", value: cty.EmptyTupleVal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file := hclwrite.NewEmptyFile()
			file.Body().SetAttributeRaw("value", hclhelper.TokensForValue(tc.value))

			assert.JSONEq(t, valueJSON(t, tc.value), valueJSON(t, parseValueAttribute(t, file.Bytes())))
		})
	}
}

func TestTokensForValueRoundTripsKeywordAsFirstKey(t *testing.T) {
	t.Parallel()

	keywords := []string{"each", "else", "endfor", "endif", "false", "for", "if", "in", "null", "true"}

	for _, keyword := range keywords {
		t.Run(keyword, func(t *testing.T) {
			t.Parallel()

			value := cty.ObjectVal(map[string]cty.Value{keyword: cty.StringVal("x")})

			file := hclwrite.NewEmptyFile()
			file.Body().SetAttributeRaw("value", hclhelper.TokensForValue(value))

			assert.JSONEq(t, valueJSON(t, value), valueJSON(t, parseValueAttribute(t, file.Bytes())))
		})
	}
}

func TestTokensForValueQuotesOnlyForKeys(t *testing.T) {
	t.Parallel()

	value := cty.ObjectVal(map[string]cty.Value{
		"for":  cty.StringVal("for"),
		"if":   cty.True,
		"list": cty.TupleVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{"for": cty.NullVal(cty.String)})}),
	})

	file := hclwrite.NewEmptyFile()
	file.Body().SetAttributeRaw("value", hclhelper.TokensForValue(value))

	assert.Equal(t, `value = {
  "for" = "for"
  if    = true
  list = [{
    "for" = null
  }]
}
`, string(file.Bytes()))
}

func TestTokensForValueMatchesHCLWriteWithoutForKeys(t *testing.T) {
	t.Parallel()

	value := cty.ObjectVal(map[string]cty.Value{
		"a b":  cty.StringVal("x"),
		"if":   cty.True,
		"list": cty.ListVal([]cty.Value{cty.MapVal(map[string]cty.Value{"zone": cty.NumberIntVal(1)})}),
		"null": cty.NullVal(cty.String),
	})

	assert.Equal(t, hclwrite.TokensForValue(value).Bytes(), hclhelper.TokensForValue(value).Bytes())
}

// parseValueAttribute parses src and returns what its value attribute evaluates to.
func parseValueAttribute(t *testing.T, src []byte) cty.Value {
	t.Helper()

	file, diags := hclsyntax.ParseConfig(src, "value.hcl", hcl.InitialPos)
	require.False(t, diags.HasErrors(), "%s\n%s", diags, src)

	attrs, diags := file.Body.JustAttributes()
	require.False(t, diags.HasErrors(), "%s", diags)

	value, diags := attrs["value"].Expr.Value(nil)
	require.False(t, diags.HasErrors(), "%s", diags)

	return value
}

// valueJSON returns value as JSON, which reads the same for a collection and the tuple or object HCL parses it as.
func valueJSON(t *testing.T, value cty.Value) string {
	t.Helper()

	encoded, err := ctyjson.Marshal(value, value.Type())
	require.NoError(t, err)

	return string(encoded)
}
