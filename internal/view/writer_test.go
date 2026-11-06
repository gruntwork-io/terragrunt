package view_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errClosedOutput = errors.New("output closed")

// TestWriterDiagnosticsJSON pins that diagnostics written through the JSON render read back unchanged.
func TestWriterDiagnosticsJSON(t *testing.T) {
	t.Parallel()

	file, parseDiags := hclsyntax.ParseConfig([]byte("locals {\n"), "terragrunt.hcl", hcl.InitialPos)
	require.Len(t, parseDiags, 1, "source should report exactly one parse diagnostic")

	withSource, err := diagnostic.NewDiagnostic(file, parseDiags[0])
	require.NoError(t, err)

	withoutSource, err := diagnostic.NewDiagnostic(nil, &hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Deprecated attribute",
		Detail:   "Use the new attribute instead.",
	})
	require.NoError(t, err)

	diags := diagnostic.Diagnostics{withSource, withoutSource}

	var out bytes.Buffer

	require.NoError(t, view.NewWriter(&out, view.NewJSONRender()).Diagnostics(diags))

	var decoded diagnostic.Diagnostics

	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	assert.Equal(t, diags, decoded, "diagnostics should read back from the JSON written")
	assert.Equal(t, byte('\n'), out.Bytes()[out.Len()-1], "JSON output should end with a newline")
}

// TestWriterShowConfigPath pins that each file a diagnostic points into is written once, in the order first seen.
func TestWriterShowConfigPath(t *testing.T) {
	t.Parallel()

	diags := diagnostic.Diagnostics{
		{Range: &diagnostic.Range{Filename: "app/terragrunt.hcl"}},
		{Summary: "no range"},
		{Range: &diagnostic.Range{}},
		{Range: &diagnostic.Range{Filename: "db/terragrunt.hcl"}},
		{Range: &diagnostic.Range{Filename: "app/terragrunt.hcl"}},
	}

	testCases := []struct {
		render   view.Render
		name     string
		expected string
	}{
		{
			name:     "human",
			render:   view.NewHumanRender(venvtest.New(), true),
			expected: "app/terragrunt.hcl\ndb/terragrunt.hcl\n",
		},
		{
			name:     "json",
			render:   view.NewJSONRender(),
			expected: `["app/terragrunt.hcl","db/terragrunt.hcl"]` + "\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer

			require.NoError(t, view.NewWriter(&out, tc.render).ShowConfigPath(diags))
			assert.Equal(t, tc.expected, out.String())
		})
	}
}

// TestWriterDiagnosticsRenderError pins that nothing is written when the render refuses the diagnostics.
func TestWriterDiagnosticsRenderError(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	err := view.NewWriter(&out, view.NewHumanRender(venvtest.New(), true)).Diagnostics(diagnostic.Diagnostics{{
		Range:    &diagnostic.Range{Filename: "terragrunt.hcl"},
		Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
		Summary:  "Unsupported block type",
	}})

	var missing view.MissingSnippetError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, "terragrunt.hcl", missing.Filename)
	assert.Zero(t, out.Len(), "nothing should be written for refused diagnostics")
}

// TestWriterOutputError pins that a failure to write the rendered output is returned to the caller.
func TestWriterOutputError(t *testing.T) {
	t.Parallel()

	diags := diagnostic.Diagnostics{{
		Range:    &diagnostic.Range{Filename: "terragrunt.hcl"},
		Snippet:  &diagnostic.Snippet{Code: "locals {", StartLine: 1},
		Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
		Summary:  "Unclosed configuration block",
	}}

	writer := view.NewWriter(failingWriter{}, view.NewJSONRender())

	require.ErrorIs(t, writer.Diagnostics(diags), errClosedOutput)
	require.ErrorIs(t, writer.ShowConfigPath(diags), errClosedOutput)
}

// failingWriter is an output whose every write fails.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errClosedOutput
}
