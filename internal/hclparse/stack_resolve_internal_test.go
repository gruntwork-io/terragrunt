package hclparse

import (
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseSourceExpr parses `source = <expr>` and returns the expression node, so each test case only needs to write the expression on the right-hand side.
func parseSourceExpr(t *testing.T, exprSrc string) hcl.Expression {
	t.Helper()

	file, diags := hclsyntax.ParseConfig([]byte("source = "+exprSrc), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

	body, ok := file.Body.(*hclsyntax.Body)
	require.True(t, ok)

	attr, ok := body.Attributes["source"]
	require.True(t, ok)

	return attr.Expr
}

// TestResolveStackSource pins recursion eligibility for nested stack source expressions: plain literals (fast path), stdlib-evaluable expressions like `get_terragrunt_dir()`-derived strings, and expressions that need parser-owned namespaces (which must return false so discovery skips the unresolvable nested stack instead of crashing).
func TestResolveStackSource(t *testing.T) {
	t.Parallel()

	baseDir := "/abs/catalog/stacks/foo"

	cases := []struct {
		name         string
		expr         string
		want         string
		wantContains string
		ok           bool
	}{
		{name: "plain literal", expr: `"../../units/foo"`, ok: true, want: "../../units/foo"},
		{name: "absolute literal", expr: `"/abs/path/units/foo"`, ok: true, want: "/abs/path/units/foo"},
		{name: "format function (tf stdlib)", expr: `format("%s/units/%s", "../catalog", "foo")`, ok: true, want: "../catalog/units/foo"},
		{name: "replace function (tf stdlib)", expr: `replace("path/with/slashes", "/", "-")`, ok: true, want: "path-with-slashes"},
		{name: "terragrunt function not in stdlib", expr: `"${get_terragrunt_dir()}/../foo"`, ok: false},
		{name: "references parser namespace local", expr: `"${local.x}/foo"`, ok: false},
		{name: "references parser namespace unit", expr: `unit.foo.path`, ok: false},
		{name: "references parser namespace values", expr: `"${values.cloud}-foo"`, ok: false},
		{name: "null literal", expr: `null`, ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := resolveStackSource(parseSourceExpr(t, tc.expr), baseDir)
			assert.Equal(t, tc.ok, ok)

			if !tc.ok {
				return
			}

			if tc.wantContains != "" {
				assert.Equal(t, filepath.Clean(tc.wantContains), filepath.Clean(got))

				return
			}

			assert.Equal(t, tc.want, got)
		})
	}
}

// TestResolveStackSource_NilExpression pins that a nil expression returns ("", false) without panicking.
func TestResolveStackSource_NilExpression(t *testing.T) {
	t.Parallel()

	got, ok := resolveStackSource(nil, "/x")
	assert.False(t, ok)
	assert.Empty(t, got)
}
