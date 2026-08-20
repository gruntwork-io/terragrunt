package stack_test

import (
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
			"12345": cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("numeric-id")}),
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
			name:    "numeric-looking key",
			address: `shard["12345"].id`,
			want:    "numeric-id",
		},
		{
			name:    "dotted form addresses the same element",
			address: "shard.api.id",
			want:    "api-id",
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

func TestFilterOutputsRejectsUnreadableAddresses(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		`shard["web"`,
		"shard[",
		"[0]",
		"shard.",
		"shard[web]",
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
