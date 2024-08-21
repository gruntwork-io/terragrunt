package diagnostic

import (
	"bufio"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcled"
)

// Snippet represents source code information about the diagnostic.
type Snippet struct {
	// Context is derived from HCL's hcled.ContextString output. This gives a high-level summary of the root context of the diagnostic.
	Context string `json:"context"`

	// Code is a possibly-multi-line string of Terraform configuration, which includes both the diagnostic source and any relevant context as defined by the diagnostic.
	Code string `json:"code"`

	// StartLine is the line number in the source file for the first line of the snippet code block.
	StartLine int `json:"start_line"`

	// HighlightStartOffset is the character offset into Code at which the diagnostic source range starts, which ought to be highlighted as such by the consumer of this data.
	HighlightStartOffset int `json:"highlight_start_offset"`

	// HighlightEndOffset is the character offset into Code at which the diagnostic source range ends.
	HighlightEndOffset int `json:"highlight_end_offset"`

	// Values is a sorted slice of expression values which may be useful in understanding the source of an error in a complex expression.
	Values []ExpressionValue `json:"values"`

	// FunctionCall is information about a function call whose failure is being reported by this diagnostic, if any.
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
}

func NewSnippet(file *hcl.File, hclDiag *hcl.Diagnostic, highlightRange hcl.Range) *Snippet {
	snipRange := *hclDiag.Subject
	if hclDiag.Context != nil {
		// Show enough of the source code to include both the subject and context ranges, which overlap in all reasonable situations.
		snipRange = hcl.RangeOver(snipRange, *hclDiag.Context)
	}

	if snipRange.Empty() {
		snipRange.End.Byte++
		snipRange.End.Column++
	}

	snippet := &Snippet{
		StartLine: hclDiag.Subject.Start.Line,
	}

	if file != nil && file.Bytes != nil {
		snippet.Context = hcled.ContextString(file, hclDiag.Subject.Start.Byte-1)

		var (
			codeStartByte int
			code          strings.Builder
		)

		sc := hcl.NewRangeScanner(file.Bytes, hclDiag.Subject.Filename, bufio.ScanLines)

		for sc.Scan() {
			lineRange := sc.Range()
			if lineRange.Overlaps(snipRange) {
				if codeStartByte == 0 && code.Len() == 0 {
					codeStartByte = lineRange.Start.Byte
				}

				code.Write(lineRange.SliceBytes(file.Bytes))
				code.WriteRune('\n')
			}
		}

		codeStr := strings.TrimSuffix(code.String(), "\n")
		snippet.Code = codeStr

		start := highlightRange.Start.Byte - codeStartByte
		end := start + (highlightRange.End.Byte - highlightRange.Start.Byte)

		if start < 0 {
			start = 0
		} else if start > len(codeStr) {
			start = len(codeStr)
		}

		if end < 0 {
			end = 0
		} else if end > len(codeStr) {
			end = len(codeStr)
		}

		snippet.HighlightStartOffset = start
		snippet.HighlightEndOffset = end
	}

	if hclDiag.Expression == nil || hclDiag.EvalContext == nil {
		return snippet
	}

	snippet.Values = DescribeExpressionValues(hclDiag)
	snippet.FunctionCall = DescribeFunctionCall(hclDiag)

	return snippet
}
