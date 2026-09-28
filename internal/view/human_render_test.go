package view_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"
)

func TestHumanRenderDiagnosticsWithoutSourceCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		diag     *diagnostic.Diagnostic
		name     string
		expected string
	}{
		{
			name: "range and snippet missing",
			diag: &diagnostic.Diagnostic{
				Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
				Summary:  "Failed to read configuration",
				Detail:   "The file could not be opened.",
			},
			expected: "╷\n" +
				"│ Error: Failed to read configuration\n" +
				"│\n" +
				"│ The file could not be opened.\n" +
				"╵\n",
		},
		{
			name: "snippet missing",
			diag: &diagnostic.Diagnostic{
				Range: &diagnostic.Range{
					Filename: "terragrunt.hcl",
					Start:    diagnostic.Pos{Line: 7, Column: 1, Byte: 42},
					End:      diagnostic.Pos{Line: 7, Column: 9, Byte: 50},
				},
				Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
				Summary:  "Unsupported block type",
				Detail:   "Blocks of type \"foo\" are not expected here.",
			},
			expected: "╷\n" +
				"│ Error: Unsupported block type\n" +
				"│\n" +
				"│   on terragrunt.hcl line 7:\n" +
				"│   (source code not available)\n" +
				"│ Blocks of type \"foo\" are not expected here.\n" +
				"╵\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			render := view.NewHumanRender(venvtest.New(), true)

			actual, err := render.Diagnostics(diagnostic.Diagnostics{tc.diag})
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}
