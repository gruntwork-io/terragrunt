package diagnostic

import (
	"bufio"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcled"
)

// Snippet represents source code information about the diagnostic.
type Snippet struct {
	FunctionCall         *FunctionCall     `json:"function_call,omitempty"`
	Context              string            `json:"context"`
	Code                 string            `json:"code"`
	Values               []ExpressionValue `json:"values"`
	StartLine            int               `json:"start_line"`
	HighlightStartOffset int               `json:"highlight_start_offset"`
	HighlightEndOffset   int               `json:"highlight_end_offset"`
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
