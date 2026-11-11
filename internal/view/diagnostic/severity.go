package diagnostic

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
)

const (
	// DiagnosticSeverityUnknown is the JSON name of a severity that is neither an error nor a warning.
	DiagnosticSeverityUnknown = "unknown"
	// DiagnosticSeverityError is the JSON name of [hcl.DiagError].
	DiagnosticSeverityError = "error"
	// DiagnosticSeverityWarning is the JSON name of [hcl.DiagWarning].
	DiagnosticSeverityWarning = "warning"
)

// DiagnosticSeverity is an [hcl.DiagnosticSeverity] that encodes to JSON by name.
type DiagnosticSeverity hcl.DiagnosticSeverity

func (severity DiagnosticSeverity) String() string {
	switch hcl.DiagnosticSeverity(severity) {
	case hcl.DiagError:
		return DiagnosticSeverityError
	case hcl.DiagWarning:
		return DiagnosticSeverityWarning
	case hcl.DiagInvalid:
		return DiagnosticSeverityUnknown
	default:
		return DiagnosticSeverityUnknown
	}
}

// MarshalJSON encodes the severity as a JSON string holding its name.
func (severity DiagnosticSeverity) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, `"%s"`, severity.String()), nil
}

// UnmarshalJSON decodes a severity name. A name other than "error" or "warning" leaves the severity unchanged.
func (severity *DiagnosticSeverity) UnmarshalJSON(val []byte) error {
	switch strings.Trim(string(val), `"`) {
	case DiagnosticSeverityError:
		*severity = DiagnosticSeverity(hcl.DiagError)
	case DiagnosticSeverityWarning:
		*severity = DiagnosticSeverity(hcl.DiagWarning)
	default:
		*severity = DiagnosticSeverity(hcl.DiagInvalid)
	}

	return nil
}
