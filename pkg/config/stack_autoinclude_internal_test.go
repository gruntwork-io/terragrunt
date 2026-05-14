package config

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStackConfigHasAutoInclude(t *testing.T) {
	t.Parallel()

	autoBody := parseStackTestBody(t, "autoinclude {\n  dependency \"vpc\" {\n    config_path = \"../vpc\"\n  }\n}\n")
	plainBody := parseStackTestBody(t, `source = "x"`)

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

func TestBodyHasBlock(t *testing.T) {
	t.Parallel()

	cases := []struct {
		body hcl.Body
		name string
		want bool
	}{
		{name: "native body with autoinclude", body: parseStackTestBody(t, "autoinclude {\n}\n"), want: true},
		{name: "native body without autoinclude", body: parseStackTestBody(t, `source = "x"`), want: false},
		{name: "native body with other block", body: parseStackTestBody(t, `dependency "vpc" {}`), want: false},
		{name: "empty non syntax body", body: hcl.EmptyBody(), want: false},
		{name: "json body with autoinclude", body: parseStackTestJSONBody(t, `{"autoinclude":[{}]}`), want: true},
		{name: "json body without autoinclude", body: parseStackTestJSONBody(t, `{"source":"x"}`), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, bodyHasBlock(tc.body, "autoinclude"))
		})
	}
}

func parseStackTestBody(t *testing.T, src string) hcl.Body {
	t.Helper()

	file, diags := hclsyntax.ParseConfig([]byte(src), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "parse: %s", diags)

	return file.Body
}

func parseStackTestJSONBody(t *testing.T, src string) hcl.Body {
	t.Helper()

	file, diags := hcljson.Parse([]byte(src), "test.hcl")
	require.False(t, diags.HasErrors(), "parse json: %s", diags)

	return file.Body
}
