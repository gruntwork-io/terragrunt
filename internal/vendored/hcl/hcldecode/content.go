// Copyright IBM Corp. 2014, 2025
// SPDX-License-Identifier: MPL-2.0

package hcldecode

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// PartialContent returns the content of body that schema names, with the diagnostics, as body.PartialContent does,
// leaving out the body that call returns for what schema does not name. Attributes is nil when body has none of the
// attributes schema names.
//
// A native HCL body is read directly, which leaves out the maps and the body PartialContent builds for that
// remainder.
//
// body must be one a parser produced: a file's body, or the body of a block in it. A native body that PartialContent
// returned as a remainder hides what that call matched, which a direct read cannot see.
func PartialContent(body hcl.Body, schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Diagnostics) {
	native, ok := body.(*hclsyntax.Body)
	if !ok {
		content, _, diags := body.PartialContent(schema)

		return content, diags
	}

	content := &hcl.BodyContent{MissingItemRange: native.MissingItemRange()}

	diags := nativeAttributes(native, schema, content)

	for _, block := range native.Blocks {
		blockSchema, wanted := wantedBlock(schema, block.Type)
		if !wanted {
			continue
		}

		if diag := labelDiagnostic(block, blockSchema); diag != nil {
			diags = append(diags, diag)

			continue
		}

		content.Blocks = append(content.Blocks, block.AsHCLBlock())
	}

	return content, diags
}

// nativeAttributes sets on content the attributes of body that schema names, and returns a diagnostic for each
// required one that body lacks. An attribute schema names twice counts as lacking the second time.
func nativeAttributes(
	body *hclsyntax.Body,
	schema *hcl.BodySchema,
	content *hcl.BodyContent,
) hcl.Diagnostics {
	var diags hcl.Diagnostics

	for _, attrSchema := range schema.Attributes {
		attr, exists := body.Attributes[attrSchema.Name]
		_, taken := content.Attributes[attrSchema.Name]

		if exists && !taken {
			if content.Attributes == nil {
				content.Attributes = make(hcl.Attributes, len(schema.Attributes))
			}

			content.Attributes[attrSchema.Name] = attr.AsHCLAttribute()

			continue
		}

		if attrSchema.Required {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Missing required argument",
				Detail: fmt.Sprintf(
					"The argument %q is required, but no definition was found.",
					attrSchema.Name,
				),
				Subject: body.MissingItemRange().Ptr(),
			})
		}
	}

	return diags
}

// wantedBlock returns the header schema names for blocks of blockType, the last one when it names that type more
// than once, and whether it names the type.
func wantedBlock(schema *hcl.BodySchema, blockType string) (hcl.BlockHeaderSchema, bool) {
	for _, blockSchema := range slices.Backward(schema.Blocks) {
		if blockSchema.Type == blockType {
			return blockSchema, true
		}
	}

	return hcl.BlockHeaderSchema{}, false
}

// labelDiagnostic returns the diagnostic for block having more or fewer labels than blockSchema names, nil when the
// count matches.
func labelDiagnostic(block *hclsyntax.Block, blockSchema hcl.BlockHeaderSchema) *hcl.Diagnostic {
	names := blockSchema.LabelNames
	if len(block.Labels) == len(names) {
		return nil
	}

	context := hcl.RangeBetween(block.TypeRange, block.OpenBraceRange).Ptr()

	if len(block.Labels) > len(names) && len(names) == 0 {
		return &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Extraneous label for " + block.Type,
			Detail:   fmt.Sprintf("No labels are expected for %s blocks.", block.Type),
			Subject:  block.LabelRanges[0].Ptr(),
			Context:  context,
		}
	}

	if len(block.Labels) > len(names) {
		return &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Extraneous label for " + block.Type,
			Detail: fmt.Sprintf(
				"Only %d labels (%s) are expected for %s blocks.",
				len(names),
				strings.Join(names, ", "),
				block.Type,
			),
			Subject: block.LabelRanges[len(names)].Ptr(),
			Context: context,
		}
	}

	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  fmt.Sprintf("Missing %s for %s", names[len(block.Labels)], block.Type),
		Detail: fmt.Sprintf(
			"All %s blocks must have %d labels (%s).",
			block.Type,
			len(names),
			strings.Join(names, ", "),
		),
		Subject: &block.OpenBraceRange,
		Context: context,
	}
}
