package view_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHumanRenderDiagnosticsWithoutSourceCode(t *testing.T) {
	t.Parallel()

	render := view.NewHumanRender(venvtest.New(), true)

	actual, err := render.Diagnostics(diagnostic.Diagnostics{{
		Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
		Summary:  "Failed to read configuration",
		Detail:   "The file could not be opened.",
	}})
	require.NoError(t, err)
	require.Equal(
		t,
		"error: Failed to read configuration\n"+
			"  = The file could not be opened.\n"+
			"\n",
		actual,
	)
}

// TestHumanRenderDiagnosticsPointAtSource pins the pointer drawn under the source a diagnostic reports.
func TestHumanRenderDiagnosticsPointAtSource(t *testing.T) {
	t.Parallel()

	const src = "locals {\n" +
		"  name = \"app\"\n" +
		"}\n" +
		"\n" +
		"dependency \"db\" {\n" +
		"\tconfig_path = \"../db\"\n" +
		"}\n" +
		"\n" +
		"inputs = {\n" +
		"  one = 1\n" +
		"  two = 2\n" +
		"}\n"

	file, parseDiags := hclsyntax.ParseConfig([]byte(src), "terragrunt.hcl", hcl.InitialPos)
	require.False(t, parseDiags.HasErrors(), parseDiags.Error())

	body, ok := file.Body.(*hclsyntax.Body)
	require.True(t, ok)

	configPath := body.Blocks[1].Body.Attributes["config_path"].Expr.Range()
	inputs := body.Attributes["inputs"].Expr.Range()

	testCases := []struct {
		diag     hcl.Diagnostic
		name     string
		expected string
	}{
		{
			name: "detail beside the pointer",
			diag: hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Missing dependency",
				Detail:   "This path does not exist.\nCreate it or fix the path.",
				Subject:  &configPath,
			},
			expected: "error: Missing dependency\n" +
				" --> terragrunt.hcl:6:16\n" +
				"  |\n" +
				"6 | \tconfig_path = \"../db\"\n" +
				"  | \t              ^^^^^^^ This path does not exist.\n" +
				"  |\n" +
				"  = Create it or fix the path.\n" +
				"\n",
		},
		{
			name: "detail too wide for the pointer line",
			diag: hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Missing dependency",
				Detail: "The unit configuration that this path names is not there, " +
					"so nothing can read outputs from it.",
				Subject: &configPath,
			},
			expected: "warning: Missing dependency\n" +
				" --> terragrunt.hcl:6:16\n" +
				"  |\n" +
				"6 | \tconfig_path = \"../db\"\n" +
				"  | \t              ^^^^^^^\n" +
				"  |\n" +
				"  = The unit configuration that this path names is not there, so nothing can\n" +
				"    read outputs from it.\n" +
				"\n",
		},
		{
			name: "source on several lines",
			diag: hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid inputs",
				Detail:   "not allowed",
				Subject:  &inputs,
			},
			expected: "error: Invalid inputs\n" +
				"  --> terragrunt.hcl:9:10\n" +
				"   |\n" +
				" 9 | inputs = {\n" +
				"   |          ^\n" +
				"10 |   one = 1\n" +
				"   |   ^^^^^^^\n" +
				"11 |   two = 2\n" +
				"   |   ^^^^^^^\n" +
				"12 | }\n" +
				"   | ^ not allowed\n" +
				"\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			render := view.NewHumanRender(venvtest.New(), true)

			diag, err := diagnostic.NewDiagnostic(file, &tc.diag)
			require.NoError(t, err)

			actual, err := render.Diagnostics(diagnostic.Diagnostics{diag})
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}

// TestHumanRenderDiagnosticsRejectsRangeWithoutSnippet pins that a hand-built diagnostic with a source
// position and no snippet is refused with an error, before anything is rendered.
func TestHumanRenderDiagnosticsRejectsRangeWithoutSnippet(t *testing.T) {
	t.Parallel()

	render := view.NewHumanRender(venvtest.New(), true)

	actual, err := render.Diagnostics(diagnostic.Diagnostics{
		{
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Rendered",
		},
		{
			Range: &diagnostic.Range{
				Filename: "terragrunt.hcl",
				Start:    diagnostic.Pos{Line: 7, Column: 1, Byte: 42},
				End:      diagnostic.Pos{Line: 7, Column: 9, Byte: 50},
			},
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Unsupported block type",
		},
	})

	var missing view.MissingSnippetError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, "terragrunt.hcl", missing.Filename)
	assert.Empty(t, actual)
}

// TestHumanRenderDiagnosticPanicsOnRangeWithoutSnippet pins that rendering one diagnostic with a source
// position and no snippet panics, since [view.HumanRender.Diagnostics] refuses it first.
func TestHumanRenderDiagnosticPanicsOnRangeWithoutSnippet(t *testing.T) {
	t.Parallel()

	render, ok := view.NewHumanRender(venvtest.New(), true).(*view.HumanRender)
	require.True(t, ok)

	assert.Panics(t, func() {
		render.Diagnostic(&diagnostic.Diagnostic{
			Range:    &diagnostic.Range{Filename: "terragrunt.hcl"},
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Unsupported block type",
		})
	})
}

// TestHumanRenderDiagnosticsHighlightEndingBeforeStart pins that a snippet whose highlight ends before it
// starts is drawn with one caret at the start instead of failing.
func TestHumanRenderDiagnosticsHighlightEndingBeforeStart(t *testing.T) {
	t.Parallel()

	render := view.NewHumanRender(venvtest.New(), true)

	actual, err := render.Diagnostics(diagnostic.Diagnostics{
		{
			Range: &diagnostic.Range{
				Filename: "terragrunt.hcl",
				Start:    diagnostic.Pos{Line: 1, Column: 15, Byte: 14},
				End:      diagnostic.Pos{Line: 1, Column: 10, Byte: 9},
			},
			Snippet: &diagnostic.Snippet{
				Code:                 `config_path = "../db"`,
				StartLine:            1,
				HighlightStartOffset: 14,
				HighlightEndOffset:   9,
			},
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Missing dependency",
		},
	})
	require.NoError(t, err)
	assert.Equal(
		t,
		"error: Missing dependency\n"+
			" --> terragrunt.hcl:1:15\n"+
			"  |\n"+
			`1 | config_path = "../db"`+"\n"+
			"  |               ^\n"+
			"\n",
		actual,
	)
}
