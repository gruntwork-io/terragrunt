package stack_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/stack"
)

// stackOutputs is the shape StackOutput builds: one object per unit, with an expanded
// unit nesting one object per iteration key under its name.
func stackOutputs() cty.Value {
	return cty.ObjectVal(map[string]cty.Value{
		"vpc": cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("vpc-id"),
		}),
		"shard": cty.ObjectVal(map[string]cty.Value{
			"web":   cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("web-id")}),
			"api":   cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("api-id")}),
			"0":     cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("zero-id")}),
			"a.b":   cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("dotted-id")}),
			`a"b`:   cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("quoted-id")}),
			"12345": cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("numeric-id")}),
			"a]b":   cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("bracket-id")}),
		}),
		"1-vpc": cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("digit-leading-id"),
		}),
		"platform": cty.ObjectVal(map[string]cty.Value{
			"02-db": cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("db-id")}),
			"007":   cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("leading-zeros-id")}),
		}),
	})
}

func TestFilterOutputsResolvesAddresses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		address string
		want    string
	}{
		{
			name:    "bare unit keeps working",
			address: "vpc.id",
			want:    "vpc-id",
		},
		{
			name:    "quoted iteration key",
			address: `shard["web"].id`,
			want:    "web-id",
		},
		{
			name:    "count index",
			address: "shard[0].id",
			want:    "zero-id",
		},
		{
			name:    "count index quoted as a string",
			address: `shard["0"].id`,
			want:    "zero-id",
		},
		{
			name:    "key containing a dot stays one segment",
			address: `shard["a.b"].id`,
			want:    "dotted-id",
		},
		{
			name:    "key containing a quote",
			address: `shard["a\"b"].id`,
			want:    "quoted-id",
		},
		{
			name:    "numeric-looking key",
			address: `shard["12345"].id`,
			want:    "numeric-id",
		},
		{
			name:    "dotted form addresses the same element",
			address: "shard.api.id",
			want:    "api-id",
		},
		{
			name:    "unit name starting with a digit",
			address: "1-vpc.id",
			want:    "digit-leading-id",
		},
		{
			name:    "unit name starting with a digit inside a stack",
			address: "platform.02-db.id",
			want:    "db-id",
		},
		{
			name:    "all-digit unit name keeps its leading zeros",
			address: "platform.007.id",
			want:    "leading-zeros-id",
		},
		{
			name:    "iteration key after a unit name starting with a digit",
			address: `1-vpc["id"]`,
			want:    "digit-leading-id",
		},
		{
			name:    "key containing a closing bracket",
			address: `shard["a]b"].id`,
			want:    "bracket-id",
		},
		{
			name:    "comment holding a quote inside the brackets",
			address: `shard[/*"*/0].id`,
			want:    "zero-id",
		},
		{
			name:    "block comment holding a closing bracket inside the brackets",
			address: "shard[0 /*].x[*/].id",
			want:    "zero-id",
		},
		{
			name:    "line comment holding a closing bracket inside the brackets",
			address: "shard[0 #].x[\n].id",
			want:    "zero-id",
		},
		{
			name:    "count index written with an exponent",
			address: "shard[1.2345e4].id",
			want:    "numeric-id",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filtered, err := stack.FilterOutputs(stackOutputs(), tc.address)
			require.NoError(t, err)

			assert.Equal(t, tc.want, leafString(t, filtered))
		})
	}
}

// leafString walks to the single leaf of the nested object FilterOutputs rebuilds.
func leafString(tb testing.TB, value cty.Value) string {
	tb.Helper()

	for value.Type().IsObjectType() {
		values := value.AsValueMap()
		require.Len(tb, values, 1)

		for _, nested := range values {
			value = nested
		}
	}

	return value.AsString()
}

func TestFilterOutputsKeepsTheAddressItWasGiven(t *testing.T) {
	t.Parallel()

	filtered, err := stack.FilterOutputs(stackOutputs(), `shard["web"].id`)
	require.NoError(t, err)

	shard, ok := filtered.AsValueMap()["shard"]
	require.True(t, ok, "the filtered result must stay wrapped in the unit name")

	web, ok := shard.AsValueMap()["web"]
	require.True(t, ok, "the filtered result must stay wrapped in the iteration key")

	assert.Equal(t, cty.StringVal("web-id"), web.AsValueMap()["id"])
}

func TestFilterOutputsAddressingNothing(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"nope",
		"vpc.nope",
		`shard["nope"].id`,
		"shard[99].id",
		"shard[0].a&b",
		"shard[0].it's",
		// A leaf has no attributes to walk into.
		"vpc.id.deeper",
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()

			filtered, err := stack.FilterOutputs(stackOutputs(), address)
			require.NoError(t, err, "an address that names nothing is not an error")
			assert.Equal(t, cty.NilVal, filtered)
		})
	}
}

func TestFilterOutputsAddressingIntoNull(t *testing.T) {
	t.Parallel()

	outputs := cty.ObjectVal(map[string]cty.Value{
		"unit": cty.ObjectVal(map[string]cty.Value{
			"object": cty.NullVal(cty.Object(map[string]cty.Type{"id": cty.String})),
			"map":    cty.NullVal(cty.Map(cty.String)),
		}),
	})

	for _, address := range []string{
		"unit.object.id",
		"unit.map.id",
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()

			filtered, err := stack.FilterOutputs(outputs, address)
			require.NoError(t, err)
			assert.Equal(t, cty.NilVal, filtered)
		})
	}
}

func TestFilterOutputsRejectsUnreadableAddresses(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		`shard["web"`,
		"shard[",
		"[0]",
		"shard.",
		"shard[web]",
		"shard[0]id",
		"shard..id",
		"1-vpc[",
		`shard["a]"]]`,
		".vpc",
		"shard]",
		`vpc.id"`,
		`shard[0/*"*/]#"].id`,
		`shard["web"/*"*/]//"].id`,
		"shard[0.5].id",
		"shard[1e99999999].id",
		"shard[9223372036854775808].id",
		"shard" + strings.Repeat("[0]", 2000),
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()

			_, err := stack.FilterOutputs(stackOutputs(), address)

			var addressErr stack.InvalidOutputAddressError
			require.ErrorAs(t, err, &addressErr)
			assert.Equal(t, address, addressErr.Address)
		})
	}
}

func TestFilterOutputsWithoutAnAddress(t *testing.T) {
	t.Parallel()

	outputs := stackOutputs()

	filtered, err := stack.FilterOutputs(outputs, "")
	require.NoError(t, err)
	assert.Equal(t, outputs, filtered)
}
