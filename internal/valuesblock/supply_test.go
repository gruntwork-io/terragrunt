package valuesblock_test

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/valuesblock"
)

func TestKeyRange(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
		key  string
		want hcl.Range
	}{
		{
			name: "literal key",
			src:  `{ a = 1, bb = 2 }`,
			key:  "bb",
			want: hcl.Range{
				Filename: "test.hcl",
				Start:    hcl.Pos{Line: 1, Column: 10, Byte: 9},
				End:      hcl.Pos{Line: 1, Column: 12, Byte: 11},
			},
		},
		{
			name: "quoted key",
			src:  `{ "a" = 1 }`,
			key:  "a",
			want: hcl.Range{
				Filename: "test.hcl",
				Start:    hcl.Pos{Line: 1, Column: 3, Byte: 2},
				End:      hcl.Pos{Line: 1, Column: 6, Byte: 5},
			},
		},
		{
			name: "absent key",
			src:  `{ a = 1 }`,
			key:  "b",
			want: hcl.Range{
				Filename: "test.hcl",
				Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
				End:      hcl.Pos{Line: 1, Column: 10, Byte: 9},
			},
		},
		{
			name: "computed expression",
			src:  `merge(local.a, { b = 1 })`,
			key:  "b",
			want: hcl.Range{
				Filename: "test.hcl",
				Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
				End:      hcl.Pos{Line: 1, Column: 26, Byte: 25},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr, diags := hclsyntax.ParseExpression([]byte(tc.src), "test.hcl", hcl.InitialPos)
			require.Empty(t, diags)

			assert.Equal(t, tc.want, valuesblock.KeyRange(expr, tc.key))
		})
	}
}

func TestKeyRangeJSON(t *testing.T) {
	t.Parallel()

	file, diags := json.Parse([]byte(`{"values": {"a": 1, "bb": 2}}`), "test.hcl.json")
	require.Empty(t, diags)

	attrs, diags := file.Body.JustAttributes()
	require.Empty(t, diags)

	assert.Equal(t, hcl.Range{
		Filename: "test.hcl.json",
		Start:    hcl.Pos{Line: 1, Column: 21, Byte: 20},
		End:      hcl.Pos{Line: 1, Column: 25, Byte: 24},
	}, valuesblock.KeyRange(attrs["values"].Expr, "bb"))
}

func TestNameSuggestion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		given string
		want  string
		names []string
	}{
		{name: "one edit away", given: "tag", names: []string{"tags", "team"}, want: "tags"},
		{name: "case differs", given: "Tags", names: []string{"tags"}, want: "tags"},
		{name: "too far", given: "region", names: []string{"tags", "count"}, want: ""},
		{name: "closest wins", given: "abcd", names: []string{"abxx", "abce"}, want: "abce"},
		{name: "first of equally close", given: "ab", names: []string{"abc", "abe"}, want: "abc"},
		{name: "no names", given: "tag", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, valuesblock.NameSuggestion(tc.given, tc.names))
		})
	}
}
