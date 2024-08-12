package diagnostic

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
)

const (
	DiagnosticSeverityUnknown = "unknown"
	DiagnosticSeverityError   = "error"
	DiagnosticSeverityWarning = "warning"
)

type DiagnosticSeverity hcl.DiagnosticSeverity

func (severity DiagnosticSeverity) String() string {
	// TODO: Remove lint suppression
	switch hcl.DiagnosticSeverity(severity) { //nolint:exhaustive
	case hcl.DiagError:
		return DiagnosticSeverityError
	case hcl.DiagWarning:
		return DiagnosticSeverityWarning
	default:
		return DiagnosticSeverityUnknown
	}
}

func (severity DiagnosticSeverity) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, severity.String())), nil
}

func (severity *DiagnosticSeverity) UnmarshalJSON(val []byte) error {
	switch strings.Trim(string(val), `"`) {
	case DiagnosticSeverityError:
		*severity = DiagnosticSeverity(hcl.DiagError)
	case DiagnosticSeverityWarning:
		*severity = DiagnosticSeverity(hcl.DiagWarning)
	}
	return nil
}
