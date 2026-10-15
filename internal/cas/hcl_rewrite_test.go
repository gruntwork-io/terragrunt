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

// updateSourceWithCASLocals starts every updateSourceWithCASCases input. It
// defines a plain local, a local read from another, a local that calls a
// function, and two locals that read each other.
const updateSourceWithCASLocals = `locals {
  use_cas = true
  no_cas  = !local.use_cas
  env_cas = get_env("USE_CAS", "true")
  cycle_a = local.cycle_b
  cycle_b = local.cycle_a
}
`

// updateSourceWithCASCases lists update_source_with_cas expressions with the
// value the full HCL parser decodes for each. Expressions the CAS reader
// cannot evaluate expect ErrUpdateSourceWithCASNotConstant instead.
var updateSourceWithCASCases = []struct {
	expectedErr error
	name        string
	expr        string
	expected    bool
}{
	{name: "true", expr: `true`, expected: true},
	{name: "false", expr: `false`, expected: false},
	{name: "negated false", expr: `!false`, expected: true},
	{name: "negated true", expr: `!true`, expected: false},
	{name: "double negation", expr: `!!true`, expected: true},
	{name: "parenthesized", expr: `(true)`, expected: true},
	{name: "negated parenthesized", expr: `!(false)`, expected: true},
	{name: "string true", expr: `"true"`, expected: true},
	{name: "string false", expr: `"false"`, expected: false},
	{name: "logical and", expr: `true && false`, expected: false},
	{name: "logical or", expr: `false || true`, expected: true},
	{name: "conditional", expr: `true ? false : true`, expected: false},
	{name: "comparison", expr: `1 == 1`, expected: true},
	{name: "null", expr: `null`, expected: false},
	{name: "local", expr: `local.use_cas`, expected: true},
	{name: "chained local", expr: `local.no_cas`, expected: false},
	{name: "literal and local", expr: `true && local.use_cas`, expected: true},
	{name: "literal and false local", expr: `true && local.no_cas`, expected: false},
	{name: "function call", expr: `tobool("true")`, expectedErr: cas.ErrUpdateSourceWithCASNotConstant},
	{name: "literal and function", expr: `true && tobool("true")`, expectedErr: cas.ErrUpdateSourceWithCASNotConstant},
	{name: "local calls function", expr: `local.env_cas == "true"`, expectedErr: cas.ErrUpdateSourceWithCASNotConstant},
	{name: "cyclic local", expr: `local.cycle_a`, expectedErr: cas.ErrUpdateSourceWithCASNotConstant},
	{name: "undefined local", expr: `local.missing`, expectedErr: cas.ErrUpdateSourceWithCASNotConstant},
	{name: "include reference", expr: `include.root.locals.use_cas`, expectedErr: cas.ErrUpdateSourceWithCASNotConstant},
	{name: "not a bool", expr: `"yes"`, expectedErr: cas.ErrUpdateSourceWithCASNotConstant},
}

func TestReadTerraformSourceInfo_UpdateSourceWithCASExpressions(t *testing.T) {
	t.Parallel()

	for _, tc := range updateSourceWithCASCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := []byte(updateSourceWithCASLocals + `
terraform {
  source = "../modules/service"
  update_source_with_cas = ` + tc.expr + `
}
`)

			source, updateWithCAS, err := cas.ReadTerraformSourceInfo(input)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)

				var wrapped *cas.WrappedError

				require.ErrorAs(t, err, &wrapped)
				assert.Equal(t, "terraform", wrapped.Op)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "../modules/service", source)
			assert.Equal(t, tc.expected, updateWithCAS)
		})
	}
}

func TestReadStackBlocks_UpdateSourceWithCASExpressions(t *testing.T) {
	t.Parallel()

	for _, tc := range updateSourceWithCASCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := []byte(updateSourceWithCASLocals + `
unit "service" {
  source = "../units/service"
  update_source_with_cas = ` + tc.expr + `
  path = "service"
}

stack "nested" {
  source = "../stacks/nested"
  update_source_with_cas = ` + tc.expr + `
  path = "nested"
}
`)

			blocks, err := cas.ReadStackBlocks(input)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)

				var wrapped *cas.WrappedError

				require.ErrorAs(t, err, &wrapped)
				assert.Equal(t, "unit", wrapped.Op)
				assert.Equal(t, "service", wrapped.Context)

				return
			}

			require.NoError(t, err)
			require.Len(t, blocks, 2)
			assert.Equal(t, tc.expected, blocks[0].UpdateSourceWithCAS)
			assert.Equal(t, tc.expected, blocks[1].UpdateSourceWithCAS)
		})
	}
}

// TestReadStackBlocks_StackAutoIncludeBlocks pins that unit and stack blocks injected by a stack-level
// autoinclude, at any depth, are read after the block that injects them, with a location naming it. Blocks
// in a unit's own autoinclude are not stack blocks and are skipped.
func TestReadStackBlocks_StackAutoIncludeBlocks(t *testing.T) {
	t.Parallel()

	input := []byte(`unit "function" {
  source = "../..//units/function"
  path = "function"
}

stack "fn" {
  source = "../..//stacks/fn"
  update_source_with_cas = true
  path = "fn"

  autoinclude {
    unit "function" {
      source = "../..//units/override"
      update_source_with_cas = true
      path = "function"

      autoinclude {
        dependency "queue" {
          config_path = "../queue"
        }
      }
    }

    stack "inner" {
      source = "../..//stacks/inner"
      path = "inner"

      autoinclude {
        unit "deep" {
          source = "../..//units/deep"
          update_source_with_cas = true
          path = "deep"
        }
      }
    }
  }
}
`)

	blocks, err := cas.ReadStackBlocks(input)
	require.NoError(t, err)
	require.Len(t, blocks, 5)

	want := []struct {
		location string
		source   string
		withCAS  bool
	}{
		{`unit "function"`, "../..//units/function", false},
		{`stack "fn"`, "../..//stacks/fn", true},
		{`stack "fn" > unit "function"`, "../..//units/override", true},
		{`stack "fn" > stack "inner"`, "../..//stacks/inner", false},
		{`stack "fn" > stack "inner" > unit "deep"`, "../..//units/deep", true},
	}

	for i, w := range want {
		assert.Equal(t, w.location, blocks[i].Location)
		assert.Equal(t, w.source, blocks[i].Source, w.location)
		assert.Equal(t, w.withCAS, blocks[i].UpdateSourceWithCAS, w.location)
	}
}

// TestReadStackBlocks_StackAutoIncludeNonLiteralSource pins that a block injected by a stack-level
// autoinclude is validated like a top-level one: with update_source_with_cas its source must be a literal,
// and the error names the block by its location. Without the flag the source is left alone.
func TestReadStackBlocks_StackAutoIncludeNonLiteralSource(t *testing.T) {
	t.Parallel()

	stackFile := func(flag string) []byte {
		return []byte(`stack "fn" {
  source = "../..//stacks/fn"
  path = "fn"

  autoinclude {
    unit "function" {
      source = "${get_repo_root()}/units/override"
      ` + flag + `
      path = "function"
    }
  }
}
`)
	}

	_, err := cas.ReadStackBlocks(stackFile("update_source_with_cas = true"))
	require.ErrorIs(t, err, cas.ErrSourceNotLiteral)

	var wrapped *cas.WrappedError

	require.ErrorAs(t, err, &wrapped)
	assert.Equal(t, "unit", wrapped.Op)
	assert.Equal(t, `stack "fn" > unit "function"`, wrapped.Context)

	_, err = cas.ReadStackBlocks(stackFile("update_source_with_cas = upper(true)"))
	require.ErrorIs(t, err, cas.ErrUpdateSourceWithCASNotConstant)

	blocks, err := cas.ReadStackBlocks(stackFile(""))
	require.NoError(t, err)
	require.Len(t, blocks, 2)
	assert.False(t, blocks[1].UpdateSourceWithCAS)
}
