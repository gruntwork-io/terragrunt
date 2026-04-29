package config

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStackConfigHasAutoInclude(t *testing.T) {
	t.Parallel()

	autoBody := parseSyntaxBody(t, "autoinclude {\n  dependency \"vpc\" {\n    config_path = \"../vpc\"\n  }\n}\n")
	plainBody := parseSyntaxBody(t, `source = "x"`)

	cases := []struct {
		cfg  *StackConfig
		name string
		want bool
	}{
		{name: "nil config", cfg: nil, want: false},
		{name: "empty config", cfg: &StackConfig{}, want: false},
		{name: "unit with autoinclude", cfg: &StackConfig{Units: []*Unit{{Remain: autoBody}}}, want: true},
		{name: "stack with autoinclude", cfg: &StackConfig{Stacks: []*Stack{{Remain: autoBody}}}, want: true},
		{name: "unit without autoinclude", cfg: &StackConfig{Units: []*Unit{{Remain: plainBody}}}, want: false},
		{name: "nil unit entry tolerated", cfg: &StackConfig{Units: []*Unit{nil, {Remain: autoBody}}}, want: true},
		{name: "nil stack entry tolerated", cfg: &StackConfig{Stacks: []*Stack{nil, {Remain: autoBody}}}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, stackConfigHasAutoInclude(tc.cfg))
		})
	}
}

// JSON-format remain bodies aren't *hclsyntax.Body and must return false: autoinclude blocks are only supported in native HCL.
func TestHasAutoIncludeInBody_NonSyntaxBodyReturnsFalse(t *testing.T) {
	t.Parallel()

	assert.False(t, hasAutoIncludeInBody(hcl.EmptyBody()))
}

func TestHasAutoIncludeInBody_NativeBody(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		want bool
	}{
		{name: "with autoinclude", src: "autoinclude {\n}\n", want: true},
		{name: "no autoinclude", src: `source = "x"`, want: false},
		{name: "other block", src: `dependency "vpc" {}`, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, hasAutoIncludeInBody(parseSyntaxBody(t, tc.src)))
		})
	}
}

// parseSyntaxBody is a helper that parses src into an *hclsyntax.Body for hasAutoIncludeInBody tests.
func parseSyntaxBody(t *testing.T, src string) hcl.Body {
	t.Helper()

	file, diags := hclsyntax.ParseConfig([]byte(src), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "parse: %s", diags)

	return file.Body
}
