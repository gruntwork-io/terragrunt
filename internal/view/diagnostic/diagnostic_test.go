package diagnostic_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rangeOnLine(line int) *diagnostic.Range {
	return &diagnostic.Range{
		Filename: "terragrunt.hcl",
		Start:    diagnostic.Pos{Line: line, Column: 1, Byte: 0},
		End:      diagnostic.Pos{Line: line, Column: 5, Byte: 4},
	}
}

func TestDiagnosticsContains(t *testing.T) {
	t.Parallel()

	tc := []struct {
		find     *diagnostic.Diagnostic
		name     string
		stored   diagnostic.Diagnostics
		expected bool
	}{
		{
			name:     "same range",
			stored:   diagnostic.Diagnostics{{Range: rangeOnLine(1)}},
			find:     &diagnostic.Diagnostic{Range: rangeOnLine(1)},
			expected: true,
		},
		{
			name:     "different range",
			stored:   diagnostic.Diagnostics{{Range: rangeOnLine(1)}},
			find:     &diagnostic.Diagnostic{Range: rangeOnLine(2)},
			expected: false,
		},
		{
			name:     "stored diagnostic without a range is skipped",
			stored:   diagnostic.Diagnostics{{Summary: "no subject"}, {Range: rangeOnLine(1)}},
			find:     &diagnostic.Diagnostic{Range: rangeOnLine(1)},
			expected: true,
		},
		{
			name:     "searched diagnostic without a range",
			stored:   diagnostic.Diagnostics{{Range: rangeOnLine(1)}},
			find:     &diagnostic.Diagnostic{Summary: "no subject"},
			expected: false,
		},
		{
			name:     "neither has a range",
			stored:   diagnostic.Diagnostics{{Summary: "no subject"}},
			find:     &diagnostic.Diagnostic{Summary: "no subject"},
			expected: false,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.stored.Contains(tt.find))
		})
	}
}

func TestDiagnosticsContainsDiagnosticWithoutSubject(t *testing.T) {
	t.Parallel()

	withoutSubject, err := diagnostic.NewDiagnostic(nil, &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Multiple terraform blocks",
	})
	require.NoError(t, err)
	require.Nil(t, withoutSubject.Range)

	diags := diagnostic.Diagnostics{withoutSubject}

	assert.False(t, diags.Contains(withoutSubject))
	assert.False(t, diags.Contains(&diagnostic.Diagnostic{Range: rangeOnLine(1)}))
}

// TestNewDiagnosticPairsRangeWithSnippet pins that a diagnostic built from an HCL diagnostic has a range
// exactly when it has a snippet, which the human rendering relies on.
func TestNewDiagnosticPairsRangeWithSnippet(t *testing.T) {
	t.Parallel()

	file, parseDiags := hclsyntax.ParseConfig([]byte("locals {}\n"), "terragrunt.hcl", hcl.InitialPos)
	require.False(t, parseDiags.HasErrors(), parseDiags.Error())

	subject := hcl.Range{
		Filename: "terragrunt.hcl",
		Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
		End:      hcl.Pos{Line: 1, Column: 7, Byte: 6},
	}

	testCases := []struct {
		file    *hcl.File
		subject *hcl.Range
		name    string
	}{
		{name: "with a subject", file: file, subject: &subject},
		{name: "without a subject", file: file},
		{name: "without a subject or a file"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diag, err := diagnostic.NewDiagnostic(tc.file, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unsupported block type",
				Subject:  tc.subject,
			})
			require.NoError(t, err)
			assert.Equal(t, diag.Range != nil, diag.Snippet != nil)
			assert.Equal(t, tc.subject != nil, diag.Range != nil)
		})
	}
}

// TestNewDiagnosticRejectsSubjectWithoutFile pins that a diagnostic pointing at a position cannot be
// built without the file holding that position, since it would have no source to show.
func TestNewDiagnosticRejectsSubjectWithoutFile(t *testing.T) {
	t.Parallel()

	_, err := diagnostic.NewDiagnostic(nil, &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Unsupported block type",
		Subject:  &hcl.Range{Filename: "terragrunt.hcl"},
	})

	var missing diagnostic.MissingSourceError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, "terragrunt.hcl", missing.Filename)
}

// TestNewSnippetPanicsWithoutFile pins that building a snippet without a file panics, since
// [diagnostic.NewDiagnostic] refuses that case first.
func TestNewSnippetPanicsWithoutFile(t *testing.T) {
	t.Parallel()

	subject := hcl.Range{Filename: "terragrunt.hcl"}

	assert.Panics(t, func() {
		diagnostic.NewSnippet(nil, &hcl.Diagnostic{Subject: &subject}, subject)
	})
}
