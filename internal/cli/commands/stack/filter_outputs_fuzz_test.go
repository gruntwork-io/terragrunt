package stack_test

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/stack"
)

// FuzzFilterOutputs pins that no address, however malformed, panics the filter, and that
// one it cannot read is reported as an [stack.InvalidOutputAddressError] naming it. The
// address is a command-line argument.
//
// It also pins that an address without whitespace or comments, which HCL reads as a
// traversal of names and string keys, resolves to the segments HCL reads.
func FuzzFilterOutputs(f *testing.F) {
	for _, seed := range []string{
		"",
		"vpc.id",
		"1-vpc.id",
		"platform.007.id",
		`shard["web"].id`,
		"shard[0].id",
		`shard["a.b"].id`,
		`shard["a\"b"].id`,
		`shard["a]b"].id`,
		`shard["$${a}"].id`,
		`shard["${a}"].id`,
		`shard[/*"*/0].id`,
		`shard[0/*"*/]#"].id`,
		"shard[0 /*].x[*/].id",
		"shard[0 #].x[\n].id",
		"shard[<<KEY\nweb\nKEY\n].id",
		"shard[1e99999999].id",
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
		if err != nil {
			var addressErr stack.InvalidOutputAddressError
			require.ErrorAs(t, err, &addressErr)
			assert.Equal(t, address, addressErr.Address)
		}

		// HCL reads some invalid UTF-8, quotes included, as part of an identifier.
		segments, ok := traversalSegments(address)
		if !ok || !utf8.ValidString(address) || strings.ContainsAny(address, " \t\r\n#/") {
			return
		}

		leaf := cty.StringVal("leaf")
		want := leaf

		for _, segment := range slices.Backward(segments) {
			want = cty.ObjectVal(map[string]cty.Value{segment: want})
		}

		filtered, err := stack.FilterOutputs(want, address)
		require.NoError(t, err)
		assert.True(t, want.RawEquals(filtered), "address %q resolved to %#v", address, filtered)
	})
}

// traversalSegments returns the names and string keys of address read as an HCL
// traversal. It reports false for an address HCL rejects or one with a number key.
func traversalSegments(address string) ([]string, bool) {
	traversal, diags := hclsyntax.ParseTraversalAbs([]byte(address), "", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, false
	}

	segments := make([]string, 0, len(traversal))

	for _, step := range traversal {
		switch step := step.(type) {
		case hcl.TraverseRoot:
			segments = append(segments, step.Name)
		case hcl.TraverseAttr:
			segments = append(segments, step.Name)
		case hcl.TraverseIndex:
			if step.Key.Type() != cty.String {
				return nil, false
			}

			segments = append(segments, step.Key.AsString())
		default:
			return nil, false
		}
	}

	return segments, true
}

// FuzzFilterOutputsResolvesQuotedKey pins that any name followed by any quoted key
// addresses exactly that element, whatever comments sit inside the brackets, so that
// neither a name, a key, nor a comment can end a segment early or start another.
func FuzzFilterOutputsResolvesQuotedKey(f *testing.F) {
	for _, seed := range []struct{ name, key, before, after string }{
		{name: "shard", key: "web"},
		{name: "1-vpc", key: "0"},
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
		{name: "x", key: "web", before: `"`},
		{name: "x", key: "web", after: `"`},
		{name: "x", key: "web", before: "].y[", after: `]#"`},
		{name: "x", key: "web", before: "\n#]\n", after: "//]"},
	} {
		f.Add(seed.name, seed.key, seed.before, seed.after)
	}

	f.Fuzz(func(t *testing.T, name, key, before, after string) {
		if name == "" || strings.ContainsAny(name, `.[]"`) {
			t.Skip("not a name an address can hold outside brackets")
		}

		if !utf8.ValidString(key) {
			t.Skip("hclwrite quotes invalid UTF-8 as U+FFFD")
		}

		if strings.Contains(before, "*/") || strings.Contains(after, "*/") {
			t.Skip("the text would end the block comment it is put in")
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

		quoted := string(hclwrite.TokensForValue(value).Bytes())
		address := name + "[/*" + before + "*/" + quoted + "/*" + after + "*/].id"

		filtered, err := stack.FilterOutputs(outputs, address)
		require.NoError(t, err, "address %q", address)

		want := cty.ObjectVal(map[string]cty.Value{
			name: cty.ObjectVal(map[string]cty.Value{
				value.AsString(): cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("element-id")}),
			}),
		})
		assert.True(t, want.RawEquals(filtered), "address %q resolved to %#v", address, filtered)
	})
}
