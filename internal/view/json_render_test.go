package view_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJSONRenderDiagnostics pins the JSON document written for a diagnostic that points into a file.
func TestJSONRenderDiagnostics(t *testing.T) {
	t.Parallel()

	file, parseDiags := hclsyntax.ParseConfig([]byte("locals {\n"), "terragrunt.hcl", hcl.InitialPos)
	require.Len(t, parseDiags, 1, "source should report exactly one parse diagnostic")

	hclDiag := parseDiags[0]
	hclDiag.Summary = "Unclosed configuration block"
	hclDiag.Detail = "Add the closing brace."

	diag, err := diagnostic.NewDiagnostic(file, hclDiag)
	require.NoError(t, err)

	actual, err := view.NewJSONRender().Diagnostics(diagnostic.Diagnostics{diag})
	require.NoError(t, err)
	assert.JSONEq(t, `[{
		"range": {
			"filename": "terragrunt.hcl",
			"start": {"line": 1, "column": 8, "byte": 7},
			"end": {"line": 1, "column": 9, "byte": 8}
		},
		"snippet": {
			"context": "locals",
			"code": "locals {",
			"values": null,
			"start_line": 1,
			"highlight_start_offset": 7,
			"highlight_end_offset": 8
		},
		"summary": "Unclosed configuration block",
		"detail": "Add the closing brace.",
		"severity": "error"
	}]`, actual)
}
