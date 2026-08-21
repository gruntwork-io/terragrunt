package diagnostic_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/hashicorp/hcl/v2"
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

	withoutSubject := diagnostic.NewDiagnostic(nil, &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Multiple terraform blocks",
	})
	require.Nil(t, withoutSubject.Range)

	diags := diagnostic.Diagnostics{withoutSubject}

	assert.False(t, diags.Contains(withoutSubject))
	assert.False(t, diags.Contains(&diagnostic.Diagnostic{Range: rangeOnLine(1)}))
}
