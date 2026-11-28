package diagnostic_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// TestNewDiagnosticSnippetFromEvaluation pins that an evaluation diagnostic carries the values read and the failed call.
func TestNewDiagnosticSnippetFromEvaluation(t *testing.T) {
	t.Parallel()

	file, hclDiag := evaluateValue(t, "value = upper(local.items)\n")

	diag, err := diagnostic.NewDiagnostic(file, hclDiag)
	require.NoError(t, err)

	assert.Equal(t, &diagnostic.Range{
		Filename: "terragrunt.hcl",
		Start:    diagnostic.Pos{Line: 1, Column: 15, Byte: 14},
		End:      diagnostic.Pos{Line: 1, Column: 26, Byte: 25},
	}, diag.Range)
	assert.Equal(t, &diagnostic.Snippet{
		FunctionCall: &diagnostic.FunctionCall{
			CalledAs:  "upper",
			Signature: diagnostic.DescribeFunction("upper", stdlib.UpperFunc),
		},
		Code: "value = upper(local.items)",
		Values: []diagnostic.ExpressionValue{
			{Traversal: "local.items", Statement: "is list of string with 2 elements"},
		},
		StartLine:            1,
		HighlightStartOffset: 14,
		HighlightEndOffset:   25,
	}, diag.Snippet)
}

// TestNewDiagnosticSnippetFromParsing pins the snippet of a parse diagnostic, including an empty subject at the end.
func TestNewDiagnosticSnippetFromParsing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedRange   *diagnostic.Range
		expectedSnippet *diagnostic.Snippet
		name            string
		src             string
	}{
		{
			name: "subject inside a block",
			src:  "locals {\n",
			expectedRange: &diagnostic.Range{
				Filename: "terragrunt.hcl",
				Start:    diagnostic.Pos{Line: 1, Column: 8, Byte: 7},
				End:      diagnostic.Pos{Line: 1, Column: 9, Byte: 8},
			},
			expectedSnippet: &diagnostic.Snippet{
				Context:              "locals",
				Code:                 "locals {",
				StartLine:            1,
				HighlightStartOffset: 7,
				HighlightEndOffset:   8,
			},
		},
		{
			name: "empty subject at the end of a line",
			src:  "value = ",
			expectedRange: &diagnostic.Range{
				Filename: "terragrunt.hcl",
				Start:    diagnostic.Pos{Line: 1, Column: 9, Byte: 8},
				End:      diagnostic.Pos{Line: 1, Column: 10, Byte: 9},
			},
			expectedSnippet: &diagnostic.Snippet{
				Code:                 "value = ",
				StartLine:            1,
				HighlightStartOffset: 8,
				HighlightEndOffset:   8,
			},
		},
		{
			name: "empty subject past the last line",
			src:  "value = [\n",
			expectedRange: &diagnostic.Range{
				Filename: "terragrunt.hcl",
				Start:    diagnostic.Pos{Line: 2, Column: 1, Byte: 10},
				End:      diagnostic.Pos{Line: 2, Column: 2, Byte: 11},
			},
			expectedSnippet: &diagnostic.Snippet{
				StartLine: 2,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, parseDiags := hclsyntax.ParseConfig([]byte(tc.src), "terragrunt.hcl", hcl.InitialPos)
			require.Len(t, parseDiags, 1, "source should report exactly one parse diagnostic")

			diag, err := diagnostic.NewDiagnostic(file, parseDiags[0])
			require.NoError(t, err)

			assert.Equal(t, tc.expectedRange, diag.Range)
			assert.Equal(t, tc.expectedSnippet, diag.Snippet)
		})
	}
}
