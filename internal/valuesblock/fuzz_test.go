package valuesblock_test

import (
	"testing"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/valuesblock"
)

// FuzzDecodeResolve pins that decoding any values block and resolving any supplied values against it returns an
// object with one attribute per declared value, and that every resolution diagnostic points into the supply.
func FuzzDecodeResolve(f *testing.F) {
	configs := []string{
		`values { value "a" { type = object({ n = optional(number, 1), s = optional(set(string)) }) } }`,
		`values {
  value "t" {
    type    = tuple([string, number])
    default = ["x", 1]
  }
}`,
		`values { value "m" { type = map(list(number)) } }`,
		`values { value "any" {} }`,
		"values {}\nvalues {}",
		`values {
  value "a" {}
  value "a" {}
}`,
		`values { value "a" { default = local.x } }`,
	}

	supplies := []string{
		`null`,
		`{}`,
		`{ a = {} }`,
		`{ a = { n = null } }`,
		`{ t = ["x"] }`,
		`{ m = { k = ["nope"] } }`,
		`{ (local.k) = 1 }`,
		`"str"`,
		`[1, 2]`,
	}

	for _, config := range configs {
		for _, supply := range supplies {
			f.Add(config, supply)
		}
	}

	f.Fuzz(func(t *testing.T, config, supply string) {
		if !utf8.ValidString(config) || !utf8.ValidString(supply) {
			t.Skip()
		}

		file, diags := hclsyntax.ParseConfig([]byte(config), "component.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			t.Skip()
		}

		decls, diags := valuesblock.Decode(file.Body)
		requireSummaries(t, diags)

		if decls == nil {
			return
		}

		expr, diags := hclsyntax.ParseExpression([]byte(supply), "supply.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			t.Skip()
		}

		val, diags := expr.Value(nil)
		if diags.HasErrors() {
			t.Skip()
		}

		got, diags := decls.Resolve(valuesblock.SuppliedByExpr(val, expr), valuesblock.Component{
			Kind:     valuesblock.KindUnit,
			Address:  `app["a"]`,
			Location: "./units/app",
		})
		requireSummaries(t, diags)

		for _, diag := range diags {
			require.NotNil(t, diag.Subject, diag.Summary)
			assert.Equal(t, "supply.hcl", diag.Subject.Filename, diag.Summary)
		}

		require.True(t, got.Type().IsObjectType(), got.Type().FriendlyName())

		names := decls.Names()
		assert.Len(t, got.Type().AttributeTypes(), len(names))

		for _, name := range names {
			assert.True(t, got.Type().HasAttribute(name), name)
		}
	})
}

// requireSummaries requires that every one of diags has a summary.
func requireSummaries(t *testing.T, diags hcl.Diagnostics) {
	t.Helper()

	for _, diag := range diags {
		require.NotEmpty(t, diag.Summary)
	}
}
