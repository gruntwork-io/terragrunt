package stack_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/stack"
)

// FuzzFilterOutputs pins that no address, however malformed, panics the filter, and that
// one it cannot read is reported as an [stack.InvalidOutputAddressError] naming it. The
// address is a command-line argument.
func FuzzFilterOutputs(f *testing.F) {
	for _, seed := range []string{
		"",
		"vpc.id",
		"01-network.id",
		"platform.007.id",
		`shard["web"].id`,
		"shard[0].id",
		`shard["a.b"].id`,
		`shard["a\"b"].id`,
		`shard["a]b"].id`,
		`shard["$${a}"].id`,
		`shard["${a}"].id`,
		`shard[/*"*/0].id`,
		"shard[0 /*].x[*/].id",
		"shard[0 #].x[\n].id",
		"shard[<<KEY\nweb\nKEY\n].id",
		"shard[*].id",
		"shard[",
		"shard]",
		"shard..id",
		" vpc . id ",
		"vpc.\xff",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, address string) {
		_, err := stack.FilterOutputs(stackOutputs(), address)
		if err == nil {
			return
		}

		var addressErr stack.InvalidOutputAddressError
		require.ErrorAs(t, err, &addressErr)
		assert.Equal(t, address, addressErr.Address)
	})
}

// FuzzFilterOutputsResolvesQuotedKey pins that any name followed by any quoted key
// addresses exactly that element, so neither can end its segment early or start another.
func FuzzFilterOutputsResolvesQuotedKey(f *testing.F) {
	for _, seed := range []struct{ name, key string }{
		{name: "shard", key: "web"},
		{name: "01-network", key: "0"},
		{name: "007", key: "a.b"},
		{name: "with space", key: `a"b`},
		{name: "a#b", key: "a]b"},
		{name: "a//b", key: "/*"},
		{name: "/*", key: "${a}"},
		{name: "ünïcödé", key: "%{if a}"},
		{name: "café", key: "café"},
		{name: "x", key: `back\slash`},
		{name: "x", key: "new\nline"},
		{name: "x", key: ""},
	} {
		f.Add(seed.name, seed.key)
	}

	f.Fuzz(func(t *testing.T, name, key string) {
		if name == "" || strings.ContainsAny(name, `.[]"`) {
			t.Skip("not a name an address can hold outside brackets")
		}

		if !utf8.ValidString(key) {
			t.Skip("cty reads invalid UTF-8 as U+FFFD")
		}

		// cty normalizes strings to NFC, so the key is compared in that form.
		value := cty.StringVal(key)
		element := cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("element-id")})
		outputs := cty.ObjectVal(map[string]cty.Value{
			name: cty.ObjectVal(map[string]cty.Value{
				value.AsString(): element,
				"other":          cty.EmptyObjectVal,
			}),
			"other": cty.EmptyObjectVal,
		})

		address := name + "[" + string(hclwrite.TokensForValue(value).Bytes()) + "]"

		filtered, err := stack.FilterOutputs(outputs, address)
		require.NoError(t, err, "address %q", address)

		want := cty.ObjectVal(map[string]cty.Value{
			name: cty.ObjectVal(map[string]cty.Value{value.AsString(): element}),
		})
		assert.True(t, want.RawEquals(filtered), "address %q resolved to %#v", address, filtered)
	})
}
