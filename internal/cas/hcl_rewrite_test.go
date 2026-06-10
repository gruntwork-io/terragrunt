package cas_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteTerraformSource(t *testing.T) {
	t.Parallel()

	input := []byte(`terraform {
  source = "../..//modules/ec2-asg-service"

  update_source_with_cas = true
}
`)

	result, err := cas.RewriteTerraformSource(input, "cas::sha1:abc123//modules/ec2-asg-service")
	require.NoError(t, err)

	assert.Contains(t, string(result), `"cas::sha1:abc123//modules/ec2-asg-service"`)
	assert.Contains(t, string(result), "update_source_with_cas")
}

func TestRewriteTerraformSource_NoBlock(t *testing.T) {
	t.Parallel()

	input := []byte(`locals {
  region = "us-east-1"
}
`)

	_, err := cas.RewriteTerraformSource(input, "cas::sha1:abc123")
	require.ErrorIs(t, err, cas.ErrNoTerraformBlock)
}

func TestRewriteStackBlockSource(t *testing.T) {
	t.Parallel()

	input := []byte(`unit "service" {
  source = "../..//units/ec2-asg-stateful-service"

  update_source_with_cas = true

  path = "service"
}

unit "other" {
  source = "../other"
  path   = "other"
}
`)

	result, err := cas.RewriteStackBlockSource(input, "unit", "service", "cas::sha1:def456")
	require.NoError(t, err)

	resultStr := string(result)
	assert.Contains(t, resultStr, `"cas::sha1:def456"`)
	// "other" unit should remain unchanged
	assert.Contains(t, resultStr, `"../other"`)
}

func TestRewriteStackBlockSource_NotFound(t *testing.T) {
	t.Parallel()

	input := []byte(`unit "service" {
  source = "../../units/service"
  path   = "service"
}
`)

	_, err := cas.RewriteStackBlockSource(input, "unit", "nonexistent", "cas::sha1:abc")
	require.ErrorIs(t, err, cas.ErrBlockNotFound)
}

func TestReadStackBlocks(t *testing.T) {
	t.Parallel()

	input := []byte(`unit "service" {
  source = "../..//units/ec2-asg-stateful-service"
  update_source_with_cas = true
  path = "service"
}

stack "nested" {
  source = "../stacks/nested"
  path   = "nested"
}

unit "plain" {
  source = "../units/plain"
  path   = "plain"
}
`)

	blocks, err := cas.ReadStackBlocks(input)
	require.NoError(t, err)
	require.Len(t, blocks, 3)

	assert.Equal(t, "service", blocks[0].Name)
	assert.Equal(t, "unit", blocks[0].BlockType)
	assert.Equal(t, "../..//units/ec2-asg-stateful-service", blocks[0].Source)
	assert.True(t, blocks[0].UpdateSourceWithCAS)

	assert.Equal(t, "nested", blocks[1].Name)
	assert.Equal(t, "stack", blocks[1].BlockType)
	assert.False(t, blocks[1].UpdateSourceWithCAS)

	assert.Equal(t, "plain", blocks[2].Name)
	assert.False(t, blocks[2].UpdateSourceWithCAS)
}

func TestReadTerraformSourceInfo(t *testing.T) {
	t.Parallel()

	input := []byte(`terraform {
  source = "../..//modules/ec2-asg-service"
  update_source_with_cas = true
}
`)

	source, updateWithCAS, err := cas.ReadTerraformSourceInfo(input)
	require.NoError(t, err)
	assert.Equal(t, "../..//modules/ec2-asg-service", source)
	assert.True(t, updateWithCAS)
}

func TestReadTerraformSourceInfo_NoUpdateFlag(t *testing.T) {
	t.Parallel()

	input := []byte(`terraform {
  source = "github.com/foo/bar//modules/vpc?ref=v1.0.0"
}
`)

	source, updateWithCAS, err := cas.ReadTerraformSourceInfo(input)
	require.NoError(t, err)
	assert.Equal(t, "github.com/foo/bar//modules/vpc?ref=v1.0.0", source)
	assert.False(t, updateWithCAS)
}

func TestReadStackBlocks_InterpolatedSourceWithCAS(t *testing.T) {
	t.Parallel()

	input := []byte(`unit "service" {
  source = "../units/${local.name}"
  update_source_with_cas = true
  path = "service"
}
`)

	_, err := cas.ReadStackBlocks(input)
	require.ErrorIs(t, err, cas.ErrSourceNotLiteral)
}

func TestReadStackBlocks_InterpolatedSourceWithoutCAS(t *testing.T) {
	t.Parallel()

	input := []byte(`unit "service" {
  source = "../units/${local.name}"
  path   = "service"
}
`)

	blocks, err := cas.ReadStackBlocks(input)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.False(t, blocks[0].UpdateSourceWithCAS)
}

func TestReadStackBlocks_EscapedTemplateSourceWithCAS(t *testing.T) {
	t.Parallel()

	// "$${" and "%%{" are escapes for literal "${" and "%{", not
	// interpolation, so the source still parses as a literal.
	input := []byte(`unit "service" {
  source = "../units/a$${b}%%{c}"
  update_source_with_cas = true
  path = "service"
}
`)

	blocks, err := cas.ReadStackBlocks(input)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.True(t, blocks[0].UpdateSourceWithCAS)
	assert.Equal(t, "../units/a$${b}%%{c}", blocks[0].Source)
}

func TestReadTerraformSourceInfo_InterpolatedSourceWithCAS(t *testing.T) {
	t.Parallel()

	input := []byte(`terraform {
  source = "../..//modules/${var.env}"
  update_source_with_cas = true
}
`)

	_, _, err := cas.ReadTerraformSourceInfo(input)
	require.ErrorIs(t, err, cas.ErrSourceNotLiteral)
}

func TestReadTerraformSourceInfo_InterpolatedSourceWithoutCAS(t *testing.T) {
	t.Parallel()

	input := []byte(`terraform {
  source = "../..//modules/${var.env}"
}
`)

	_, updateWithCAS, err := cas.ReadTerraformSourceInfo(input)
	require.NoError(t, err)
	assert.False(t, updateWithCAS)
}

func TestReadTerraformSourceInfo_EscapedTemplateSourceWithCAS(t *testing.T) {
	t.Parallel()

	input := []byte(`terraform {
  source = "../..//modules/a$${b}"
  update_source_with_cas = true
}
`)

	source, updateWithCAS, err := cas.ReadTerraformSourceInfo(input)
	require.NoError(t, err)
	assert.Equal(t, "../..//modules/a$${b}", source)
	assert.True(t, updateWithCAS)
}

// nonLiteralSourceCases enumerates source expressions that are not pure
// quoted string literals. Each must be rejected when the block opts in to
// update_source_with_cas and tolerated when it does not.
var nonLiteralSourceCases = []struct {
	name   string
	source string
}{
	{name: "interpolation", source: `"../units/${local.name}"`},
	{name: "reference", source: `local.foo`},
	{name: "function call", source: `join("/", ["..", "units", "service"])`},
	{name: "heredoc", source: "<<-EOT\n  ../units/service\nEOT"},
	{name: "string concatenation", source: `"../units/" + local.name`},
	{name: "number", source: `42`},
}

func TestReadStackBlocks_NonLiteralSourceWithCAS(t *testing.T) {
	t.Parallel()

	for _, tc := range nonLiteralSourceCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := []byte(`unit "service" {
  source = ` + tc.source + `
  update_source_with_cas = true
  path = "service"
}
`)

			_, err := cas.ReadStackBlocks(input)
			require.ErrorIs(t, err, cas.ErrSourceNotLiteral)

			var wrapped *cas.WrappedError

			require.ErrorAs(t, err, &wrapped)
			assert.Equal(t, "unit", wrapped.Op)
			assert.Equal(t, "service", wrapped.Context)
		})
	}
}

func TestReadStackBlocks_NonLiteralSourceWithoutCAS(t *testing.T) {
	t.Parallel()

	for _, tc := range nonLiteralSourceCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := []byte(`unit "service" {
  source = ` + tc.source + `
  path   = "service"
}
`)

			blocks, err := cas.ReadStackBlocks(input)
			require.NoError(t, err)
			require.Len(t, blocks, 1)
			assert.False(t, blocks[0].UpdateSourceWithCAS)
		})
	}
}

func TestReadTerraformSourceInfo_NonLiteralSourceWithCAS(t *testing.T) {
	t.Parallel()

	for _, tc := range nonLiteralSourceCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := []byte(`terraform {
  source = ` + tc.source + `
  update_source_with_cas = true
}
`)

			_, _, err := cas.ReadTerraformSourceInfo(input)
			require.ErrorIs(t, err, cas.ErrSourceNotLiteral)
		})
	}
}

func TestReadTerraformSourceInfo_NonLiteralSourceWithoutCAS(t *testing.T) {
	t.Parallel()

	for _, tc := range nonLiteralSourceCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := []byte(`terraform {
  source = ` + tc.source + `
}
`)

			_, updateWithCAS, err := cas.ReadTerraformSourceInfo(input)
			require.NoError(t, err)
			assert.False(t, updateWithCAS)
		})
	}
}
