package diagnostic_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
)

// TestDiagnosticCausedByUnknownAndSensitive pins that the cause is read from the extra value through any wrapping.
func TestDiagnosticCausedByUnknownAndSensitive(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		extra     any
		name      string
		unknown   bool
		sensitive bool
	}{
		{name: "no extra"},
		{name: "extra that reports no cause", extra: "unrelated"},
		{name: "caused by unknown", extra: causeExtra{unknown: true}, unknown: true},
		{name: "caused by sensitive", extra: causeExtra{sensitive: true}, sensitive: true},
		{
			name:      "cause wrapped twice",
			extra:     wrapExtra{inner: wrapExtra{inner: causeExtra{unknown: true, sensitive: true}}},
			unknown:   true,
			sensitive: true,
		},
		{name: "wrapper around an unrelated extra", extra: wrapExtra{inner: "unrelated"}},
		{name: "wrapper around nothing", extra: wrapExtra{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diag := &hcl.Diagnostic{Severity: hcl.DiagError, Summary: "Invalid value", Extra: tc.extra}

			assert.Equal(t, tc.unknown, diagnostic.DiagnosticCausedByUnknown(diag), "unknown cause should match the extra")
			assert.Equal(t, tc.sensitive, diagnostic.DiagnosticCausedBySensitive(diag), "sensitive cause should match the extra")
		})
	}
}

// causeExtra is a diagnostic extra value that reports what caused the diagnostic.
type causeExtra struct {
	unknown   bool
	sensitive bool
}

func (e causeExtra) DiagnosticCausedByUnknown() bool {
	return e.unknown
}

func (e causeExtra) DiagnosticCausedBySensitive() bool {
	return e.sensitive
}

// wrapExtra is a diagnostic extra value that wraps the extra value of a diagnostic made downstream.
type wrapExtra struct {
	inner any
}

func (e wrapExtra) UnwrapDiagnosticExtra() any {
	return e.inner
}
