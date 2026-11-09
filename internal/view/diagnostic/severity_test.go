package diagnostic_test

import (
	"encoding/json"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiagnosticSeverityJSONRoundTrip pins that a severity is written by name and only error and warning read back.
func TestDiagnosticSeverityJSONRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		expected string
		severity hcl.DiagnosticSeverity
		readBack hcl.DiagnosticSeverity
	}{
		{
			name:     "error",
			severity: hcl.DiagError,
			expected: diagnostic.DiagnosticSeverityError,
			readBack: hcl.DiagError,
		},
		{
			name:     "warning",
			severity: hcl.DiagWarning,
			expected: diagnostic.DiagnosticSeverityWarning,
			readBack: hcl.DiagWarning,
		},
		{
			name:     "invalid",
			severity: hcl.DiagInvalid,
			expected: diagnostic.DiagnosticSeverityUnknown,
			readBack: hcl.DiagInvalid,
		},
		{
			name:     "out of range",
			severity: hcl.DiagnosticSeverity(99),
			expected: diagnostic.DiagnosticSeverityUnknown,
			readBack: hcl.DiagInvalid,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			severity := diagnostic.DiagnosticSeverity(tc.severity)

			assert.Equal(t, tc.expected, severity.String(), "severity should print by name")

			raw, err := json.Marshal(severity)
			require.NoError(t, err)
			assert.JSONEq(t, `"`+tc.expected+`"`, string(raw), "severity should marshal as its name")

			decoded := diagnostic.DiagnosticSeverity(99)

			require.NoError(t, json.Unmarshal(raw, &decoded))
			assert.Equal(t, diagnostic.DiagnosticSeverity(tc.readBack), decoded, "severity should read back from its name")
		})
	}
}

func TestDiagnosticSeverityUnmarshalUnrecognizedName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		raw  string
	}{
		{name: "unrecognized name", raw: `"fatal"`},
		{name: "empty name", raw: `""`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			decoded := diagnostic.DiagnosticSeverity(hcl.DiagError)

			require.NoError(t, json.Unmarshal([]byte(tc.raw), &decoded))
			assert.Equal(t, diagnostic.DiagnosticSeverity(hcl.DiagInvalid), decoded)
		})
	}
}
