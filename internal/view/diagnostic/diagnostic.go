// Package diagnostic provides a way to represent diagnostics in a way
// that can be easily marshalled to JSON.
package diagnostic

import (
	"github.com/hashicorp/hcl/v2"
)

type Diagnostics []*Diagnostic

func (diags *Diagnostics) Contains(find *Diagnostic) bool {
	for _, diag := range *diags {
		if find.Range != nil && find.Range.String() == diag.Range.String() {
			return true
		}
	}

	return false
}

type Diagnostic struct {
	Severity DiagnosticSeverity `json:"severity"`
	Summary  string             `json:"summary"`
	Detail   string             `json:"detail"`
	Range    *Range             `json:"range,omitempty"`
	Snippet  *Snippet           `json:"snippet,omitempty"`
}

func NewDiagnostic(file *hcl.File, hclDiag *hcl.Diagnostic) *Diagnostic {
	diag := &Diagnostic{
		Severity: DiagnosticSeverity(hclDiag.Severity),
		Summary:  hclDiag.Summary,
		Detail:   hclDiag.Detail,
	}

	if hclDiag.Subject == nil {
		return diag
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

	return diag
}
