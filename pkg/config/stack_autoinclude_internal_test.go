package config

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
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

func TestStackConfigHasAutoIncludeHCL(t *testing.T) {
	t.Parallel()

	nativeAutoincludeBody := parseStackTestBody(t, "autoinclude {\n}\n")
	nativePlainBody := parseStackTestBody(t, `source = "x"`)
	jsonAutoincludeBody := parseStackTestJSONBody(t, `{"autoinclude":[{}]}`)

	cases := []struct {
		cfg  *StackConfig
		name string
		want bool
	}{
		{name: "nil config", cfg: nil, want: false},
		{name: "native unit autoinclude", cfg: &StackConfig{Units: []*Unit{{Remain: nativeAutoincludeBody}}}, want: true},
		{name: "native stack autoinclude", cfg: &StackConfig{Stacks: []*Stack{{Remain: nativeAutoincludeBody}}}, want: true},
		{name: "native unit without autoinclude", cfg: &StackConfig{Units: []*Unit{{Remain: nativePlainBody}}}, want: false},
		{name: "json body with autoinclude", cfg: &StackConfig{Units: []*Unit{{Remain: jsonAutoincludeBody}}}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, stackConfigHasAutoIncludeHCL(tc.cfg))
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

			assert.Equal(t, tc.want, bodyHasBlock(tc.body))
		})
	}
}

func TestTopLevelDependencyName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		body      hcl.Body
		wantName  string
		wantFound bool
	}{
		{name: "labeled dependency", body: parseStackTestBody(t, "dependency \"vpc\" {\n  config_path = \"../vpc\"\n}\n"), wantName: "vpc", wantFound: true},
		{name: "unlabeled dependency is not a clean match", body: parseStackTestBody(t, "dependency {\n}\n"), wantName: "", wantFound: false},
		{name: "no dependency", body: parseStackTestBody(t, "unit \"a\" {\n  source = \".\"\n  path = \"a\"\n}\n"), wantName: "", wantFound: false},
		{name: "nil body", body: nil, wantName: "", wantFound: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			name, found := topLevelDependencyName(tc.body)
			assert.Equal(t, tc.wantFound, found)
			assert.Equal(t, tc.wantName, name)
		})
	}
}

// TestLogStackAutoIncludeOverrides asserts the by-design behavior: a nested
// autoinclude inside an injected unit does not propagate and is reported via a
// debug log, and an injected name that overrides an existing one is also logged.
func TestLogStackAutoIncludeOverrides(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	l := log.New(log.WithOutput(buf), log.WithLevel(log.DebugLevel), log.WithFormatter(format.NewFormatter(format.NewKeyValueFormatPlaceholders())))

	existing := &StackConfigFile{Units: []*Unit{{Name: "extra", Remain: parseStackTestBody(t, `source = "."`)}}}

	nestedAutoIncludeBody := parseStackTestBody(t, "autoinclude {\n  unit \"deep\" {\n    source = \".\"\n    path = \"deep\"\n  }\n}\n")
	included := &StackConfigFile{Units: []*Unit{{Name: "extra", Remain: nestedAutoIncludeBody}}}

	logStackAutoIncludeOverrides(l, existing, included)

	out := buf.String()
	assert.Contains(t, out, "overrides existing unit \"extra\"", "an injected unit that replaces an existing one must be logged")
	assert.Contains(t, out, "nested autoinclude is not propagated", "a nested autoinclude block must be reported as not propagated")

	// The contract is debug-only reporting: a regression promoting these to a louder level must fail here.
	assert.Contains(t, out, "level=debug", "the override and no-propagation reporting must be emitted at debug level")
	assert.NotContains(t, out, "level=info", "the reporting must not surface at info level")
	assert.NotContains(t, out, "level=warn", "the reporting must not surface at warn level")
	assert.NotContains(t, out, "level=error", "the reporting must not surface at error level")
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
