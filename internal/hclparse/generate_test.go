package hclparse_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindBlock_NotFound(t *testing.T) {
	t.Parallel()

	body := &hclsyntax.Body{
		Blocks: []*hclsyntax.Block{
			{Type: "dependency", Labels: []string{"vpc"}},
		},
	}

	result := hclparse.FindBlock(body, "dependency", "rds")
	assert.Nil(t, result)
}

func TestFindBlock_Found(t *testing.T) {
	t.Parallel()

	block := &hclsyntax.Block{Type: "dependency", Labels: []string{"vpc"}}
	body := &hclsyntax.Body{
		Blocks: []*hclsyntax.Block{block},
	}

	result := hclparse.FindBlock(body, "dependency", "vpc")
	assert.Equal(t, block, result)
}

func TestFindBlock_NoLabels(t *testing.T) {
	t.Parallel()

	body := &hclsyntax.Body{
		Blocks: []*hclsyntax.Block{
			{Type: "dependency", Labels: nil},
		},
	}

	result := hclparse.FindBlock(body, "dependency", "vpc")
	assert.Nil(t, result)
}

func TestRangeBytes_Valid(t *testing.T) {
	t.Parallel()

	src := []byte("hello world")
	r := hcl.Range{Start: hcl.Pos{Byte: 0}, End: hcl.Pos{Byte: 5}}

	result := hclparse.RangeBytes(src, r)
	assert.Equal(t, []byte("hello"), result)
}

func TestRangeBytes_OutOfBounds(t *testing.T) {
	t.Parallel()

	src := []byte("short")

	tests := []struct {
		name  string
		start int
		end   int
	}{
		{"start past end of src", 10, 15},
		{"end past end of src", 0, 100},
		{"start equals end", 2, 2},
		{"start after end", 5, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := hcl.Range{Start: hcl.Pos{Byte: tt.start}, End: hcl.Pos{Byte: tt.end}}
			result := hclparse.RangeBytes(src, r)
			assert.Nil(t, result)
		})
	}
}

func TestRawTokens_Empty(t *testing.T) {
	t.Parallel()

	result := hclparse.RawTokens(nil)
	assert.Nil(t, result)

	result = hclparse.RawTokens([]byte{})
	assert.Nil(t, result)
}

func TestRawTokens_NonEmpty(t *testing.T) {
	t.Parallel()

	result := hclparse.RawTokens([]byte("test"))
	require.Len(t, result, 1)
	assert.Equal(t, []byte("test"), result[0].Bytes)
}

func TestCommentTokens(t *testing.T) {
	t.Parallel()

	result := hclparse.CommentTokens("# hello")
	require.Len(t, result, 1)
	assert.Equal(t, []byte("# hello"), result[0].Bytes)
}

func TestSortedAttributes_Empty(t *testing.T) {
	t.Parallel()

	result := hclparse.SortedAttributes(nil)
	assert.Empty(t, result)
}

func TestSortedAttributes_Ordering(t *testing.T) {
	t.Parallel()

	attrs := hclsyntax.Attributes{
		"b": {Name: "b", SrcRange: hcl.Range{Start: hcl.Pos{Byte: 20}}},
		"a": {Name: "a", SrcRange: hcl.Range{Start: hcl.Pos{Byte: 10}}},
		"c": {Name: "c", SrcRange: hcl.Range{Start: hcl.Pos{Byte: 30}}},
	}

	sorted := hclparse.SortedAttributes(attrs)
	require.Len(t, sorted, 3)
	assert.Equal(t, "a", sorted[0].Name)
	assert.Equal(t, "b", sorted[1].Name)
	assert.Equal(t, "c", sorted[2].Name)
}
