// Package diagnostic provides a way to represent diagnostics in a way
// that can be easily marshalled to JSON.
package diagnostic

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
)

// MissingSourceError reports an HCL diagnostic that points at a position in Filename, built without
// the file it points into.
type MissingSourceError struct {
	Filename string
}

func (err MissingSourceError) Error() string {
	return fmt.Sprintf("diagnostic points into %s but has no parsed file to read its source from", err.Filename)
}

type Diagnostics []*Diagnostic

// Contains reports whether diags holds a diagnostic with the same source range as find.
// A diagnostic without a range matches nothing, not even another one without a range.
func (diags *Diagnostics) Contains(find *Diagnostic) bool {
	for _, diag := range *diags {
		if find.Range != nil && diag.Range != nil && find.Range.String() == diag.Range.String() {
			return true
		}
	}

	return false
}

type Diagnostic struct {
	Range    *Range             `json:"range,omitempty"`
	Snippet  *Snippet           `json:"snippet,omitempty"`
	Summary  string             `json:"summary"`
	Detail   string             `json:"detail"`
	Severity DiagnosticSeverity `json:"severity"`
}

// NewDiagnostic converts hclDiag, read from file, into a Diagnostic. A Diagnostic it returns has a
// Range exactly when it has a Snippet, and file is needed only for an hclDiag with a Subject.
//
// Returns [MissingSourceError] when hclDiag has a Subject and file is nil.
func NewDiagnostic(file *hcl.File, hclDiag *hcl.Diagnostic) (*Diagnostic, error) {
	if hclDiag.Subject != nil && file == nil {
		return nil, MissingSourceError{Filename: hclDiag.Subject.Filename}
	}

	diag := &Diagnostic{
		Severity: DiagnosticSeverity(hclDiag.Severity),
		Summary:  hclDiag.Summary,
		Detail:   hclDiag.Detail,
	}

	if hclDiag.Subject == nil {
		return diag, nil
	}

	highlightRange := *hclDiag.Subject
	if highlightRange.Empty() {
		highlightRange.End.Byte++
		highlightRange.End.Column++
	}

	diag.Snippet = NewSnippet(file, hclDiag, highlightRange)

	diag.Range = &Range{
		Filename: highlightRange.Filename,
		Start: Pos{
			Line:   highlightRange.Start.Line,
			Column: highlightRange.Start.Column,
			Byte:   highlightRange.Start.Byte,
		},
		End: Pos{
			Line:   highlightRange.End.Line,
			Column: highlightRange.End.Column,
			Byte:   highlightRange.End.Byte,
		},
	}

	return diag, nil
}
