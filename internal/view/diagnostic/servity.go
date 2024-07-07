package diagnostic

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
)

const (
	DiagnosticSeverityUnknown = "unknown"
	DiagnosticSeverityError   = "error"
	DiagnosticSeverityWarning = "warning"
)

type DiagnosticSeverity hcl.DiagnosticSeverity

func (severity DiagnosticSeverity) String() string {
	switch hcl.DiagnosticSeverity(severity) {
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
