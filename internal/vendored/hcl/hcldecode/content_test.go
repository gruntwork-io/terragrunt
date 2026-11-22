package hcldecode_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/vendored/hcl/hcldecode"
)

func contentSchemas() map[string]*hcl.BodySchema {
	return map[string]*hcl.BodySchema{
		"empty": {},
		"attributes": {Attributes: []hcl.AttributeSchema{
			{Name: "a", Required: true},
			{Name: "b"},
			{Name: "a"},
			{Name: "missing", Required: true},
		}},
		"labeled blocks": {Blocks: []hcl.BlockHeaderSchema{
			{Type: "two", LabelNames: []string{"x", "y"}},
			{Type: "none"},
		}},
		"block type named twice": {Blocks: []hcl.BlockHeaderSchema{
			{Type: "two"},
			{Type: "two", LabelNames: []string{"only"}},
		}},
		"attributes and blocks": {
			Attributes: []hcl.AttributeSchema{{Name: "two"}, {Name: "b", Required: true}},
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "none"},
				{Type: "a", LabelNames: []string{"name"}},
			},
		},
	}
}

func contentSources() []string {
	return []string{
		"",
		"a = 1\nb = \"x\"\n",
		"two \"p\" \"q\" {}\ntwo \"p\" {}\ntwo \"p\" \"q\" \"r\" {}\nnone {}\nnone \"extra\" {}\n",
		"a = 1\ntwo = 2\nnone {\n  inner = 1\n}\nother {}\n",
		"a \"n\" {}\na \"m\" {\n  nested \"x\" {}\n}\nb = var.x\n",
	}
}

func TestPartialContentMatchesTheBodys(t *testing.T) {
	t.Parallel()

	for i, src := range contentSources() {
		for name, schema := range contentSchemas() {
			t.Run(fmt.Sprintf("source %d, %s", i, name), func(t *testing.T) {
				t.Parallel()

				assertContentLikePartialContent(t, parseBody(t, src, false), schema)
			})
		}
	}
}

func TestPartialContentMatchesTheBodysForJSON(t *testing.T) {
	t.Parallel()

	body := parseBody(t, `{"a": 1, "two": {"p": {"q": {}}}, "none": {}}`, true)

	for name, schema := range contentSchemas() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assertContentLikePartialContent(t, body, schema)
		})
	}
}

func FuzzPartialContentMatchesTheBodys(f *testing.F) {
	for _, src := range contentSources() {
		f.Add(src)
	}

	f.Fuzz(func(t *testing.T, src string) {
		file, diags := hclsyntax.ParseConfig([]byte(src), "fuzz.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			return
		}

		for _, schema := range contentSchemas() {
			assertContentLikePartialContent(t, file.Body, schema)
		}
	})
}

// assertContentLikePartialContent asserts that hcldecode.PartialContent returns for body and schema the content and
// diagnostics, in order, that body.PartialContent does. An empty attribute map counts as none.
func assertContentLikePartialContent(t *testing.T, body hcl.Body, schema *hcl.BodySchema) {
	t.Helper()

	want, _, wantDiags := body.PartialContent(schema)
	got, gotDiags := hcldecode.PartialContent(body, schema)

	if len(want.Attributes) == 0 {
		want.Attributes = nil
	}

	if len(got.Attributes) == 0 {
		got.Attributes = nil
	}

	assert.Equal(t, want, got)
	assert.Equal(t, diagnosticText(wantDiags), diagnosticText(gotDiags))
}

func diagnosticText(diags hcl.Diagnostics) []string {
	out := make([]string, 0, len(diags))
	for _, diag := range diags {
		out = append(out, fmt.Sprintf("%d %s %v", diag.Severity, diag.Error(), diag.Context))
	}

	return out
}
