package view_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/view"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
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
	assert.Contains(t, missing.Error(), "terragrunt.hcl", "error should name the file it has no snippet for")
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

// TestHumanRenderShowConfigPath pins one configuration path per line.
func TestHumanRenderShowConfigPath(t *testing.T) {
	t.Parallel()

	render := view.NewHumanRender(venvtest.New(), true)

	actual, err := render.ShowConfigPath([]string{"app/terragrunt.hcl", "db/terragrunt.hcl"})
	require.NoError(t, err)
	assert.Equal(t, "app/terragrunt.hcl\ndb/terragrunt.hcl\n", actual)
}

// TestHumanRenderDiagnosticsColor pins that color needs a terminal on stderr and only adds escape codes.
func TestHumanRenderDiagnosticsColor(t *testing.T) {
	t.Parallel()

	const src = "dependency \"db\" {\n" +
		"  config_path = \"../db\"\n" +
		"}\n"

	file, parseDiags := hclsyntax.ParseConfig([]byte(src), "terragrunt.hcl", hcl.InitialPos)
	require.False(t, parseDiags.HasErrors(), parseDiags.Error())

	body, ok := file.Body.(*hclsyntax.Body)
	require.True(t, ok)

	configPath := body.Blocks[0].Body.Attributes["config_path"].Expr.Range()

	testCases := []struct {
		name        string
		plainPrefix string
		severity    hcl.DiagnosticSeverity
	}{
		{name: "error", severity: hcl.DiagError, plainPrefix: "error: Missing dependency\n"},
		{name: "warning", severity: hcl.DiagWarning, plainPrefix: "warning: Missing dependency\n"},
		{name: "no known severity", severity: hcl.DiagInvalid, plainPrefix: "\nMissing dependency\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diag, err := diagnostic.NewDiagnostic(file, &hcl.Diagnostic{
				Severity: tc.severity,
				Summary:  "Missing dependency",
				Detail:   "This path does not exist.",
				Subject:  &configPath,
			})
			require.NoError(t, err)

			diags := diagnostic.Diagnostics{diag}

			plain, err := view.NewHumanRender(terminalVenv(80), true).Diagnostics(diags)
			require.NoError(t, err)

			noTerminal, err := view.NewHumanRender(venvtest.New(), false).Diagnostics(diags)
			require.NoError(t, err)

			colored, err := view.NewHumanRender(terminalVenv(80), false).Diagnostics(diags)
			require.NoError(t, err)

			assert.True(t, strings.HasPrefix(plain, tc.plainPrefix), "plain output should open with the severity and summary")
			assert.Equal(t, plain, noTerminal, "color should stay off when stderr is not a terminal")
			assert.NotEqual(t, plain, colored, "color should be on when allowed and stderr is a terminal")
			assert.Equal(t, plain, ansi.Strip(colored), "color should only add escape codes")
		})
	}
}

// TestHumanRenderDiagnosticWithoutSeverityResetsColor pins that color left from earlier output is cleared.
func TestHumanRenderDiagnosticWithoutSeverityResetsColor(t *testing.T) {
	t.Parallel()

	actual, err := view.NewHumanRender(terminalVenv(80), false).Diagnostics(diagnostic.Diagnostics{{
		Severity: diagnostic.DiagnosticSeverity(hcl.DiagInvalid),
		Summary:  "Failed to read configuration",
	}})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(actual, "\n\x1b[0m"), "output should reset color before the summary")
	assert.Equal(t, "\nFailed to read configuration\n\n", ansi.Strip(actual))
}

// TestHumanRenderDiagnosticsWrapping pins the notes left unwrapped: on a tiny terminal and on preformatted lines.
func TestHumanRenderDiagnosticsWrapping(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		detail   string
		expected string
		width    int
	}{
		{
			name:   "terminal narrower than the indent",
			width:  3,
			detail: "The file could not be opened because it is locked.",
			expected: "error: Failed to read configuration\n" +
				"  = The file could not be opened because it is locked.\n" +
				"\n",
		},
		{
			name:   "preformatted line",
			width:  30,
			detail: "Run this command:\n    terragrunt run --all -- plan -lock=false",
			expected: "error: Failed to read configuration\n" +
				"  = Run this command:\n" +
				"        terragrunt run --all -- plan -lock=false\n" +
				"\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual, err := view.NewHumanRender(terminalVenv(tc.width), true).Diagnostics(diagnostic.Diagnostics{{
				Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
				Summary:  "Failed to read configuration",
				Detail:   tc.detail,
			}})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// TestHumanRenderDiagnosticsFunctionCallNotes pins the call signature and value notes under a failed function call.
func TestHumanRenderDiagnosticsFunctionCallNotes(t *testing.T) {
	t.Parallel()

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"local": cty.ObjectVal(map[string]cty.Value{
				"name":  cty.StringVal("app"),
				"items": cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			}),
		},
		Functions: map[string]function.Function{
			"upper":  stdlib.UpperFunc,
			"format": stdlib.FormatFunc,
		},
	}

	testCases := []struct {
		name     string
		src      string
		expected string
	}{
		{
			name: "fixed parameters",
			src:  "value = upper([local.name, local.items])\n",
			expected: "error: Invalid function argument\n" +
				" --> terragrunt.hcl:1:15\n" +
				"  |\n" +
				"1 | value = upper([local.name, local.items])\n" +
				"  |               ^ Wrong type.\n" +
				"  |\n" +
				"  = while calling upper(str)\n" +
				"  = local.items is list of string with 2 elements\n" +
				"  = local.name is \"app\"\n" +
				"\n",
		},
		{
			name: "variadic parameter",
			src:  "value = format(local.items, 1)\n",
			expected: "error: Invalid function argument\n" +
				" --> terragrunt.hcl:1:16\n" +
				"  |\n" +
				"1 | value = format(local.items, 1)\n" +
				"  |                ^^^^^^^^^^^ Wrong type.\n" +
				"  |\n" +
				"  = while calling format(format, args...)\n" +
				"  = local.items is list of string with 2 elements\n" +
				"\n",
		},
		{
			name: "call over several lines",
			src:  "value = format(local.items,\n  \"x\")\n",
			expected: "error: Invalid function argument\n" +
				" --> terragrunt.hcl:1:16\n" +
				"  |\n" +
				"1 | value = format(local.items,\n" +
				"  |                ^^^^^^^^^^^ Wrong type.\n" +
				"2 |   \"x\")\n" +
				"  |\n" +
				"  = while calling format(format, args...)\n" +
				"  = local.items is list of string with 2 elements\n" +
				"\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, parseDiags := hclsyntax.ParseConfig([]byte(tc.src), "terragrunt.hcl", hcl.InitialPos)
			require.False(t, parseDiags.HasErrors(), parseDiags.Error())

			body, ok := file.Body.(*hclsyntax.Body)
			require.True(t, ok)

			_, evalDiags := body.Attributes["value"].Expr.Value(evalCtx)
			require.Len(t, evalDiags, 1, "evaluating value should report exactly one diagnostic")

			hclDiag := evalDiags[0]
			hclDiag.Summary = "Invalid function argument"
			hclDiag.Detail = "Wrong type."

			diag, err := diagnostic.NewDiagnostic(file, hclDiag)
			require.NoError(t, err)

			actual, err := view.NewHumanRender(venvtest.New(), true).Diagnostics(diagnostic.Diagnostics{diag})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// terminalVenv returns an in-memory venv whose stderr is a terminal of the given width.
func terminalVenv(width int) *venv.Venv {
	v := venvtest.New()
	v.Terminal = &venv.Terminal{
		StdinIsTTY:  func() bool { return false },
		StdoutIsTTY: func() bool { return false },
		StderrIsTTY: func() bool { return true },
		Width:       func() int { return width },
	}

	return v
}
