package cas

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// RewriteTerraformSource rewrites the `source` attribute inside a `terraform {}` block.
// Returns the rewritten HCL content.
func RewriteTerraformSource(content []byte, newSource string) ([]byte, error) {
	f, diags := hclwrite.ParseConfig(content, "terragrunt.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", diags.Error())
	}

	for _, block := range f.Body().Blocks() {
		if block.Type() != "terraform" {
			continue
		}

		block.Body().SetAttributeValue("source", cty.StringVal(newSource))

		return f.Bytes(), nil
	}

	return nil, ErrNoTerraformBlock
}

// RewriteStackBlockSource rewrites the `source` attribute in a named `unit` or `stack` block.
// blockType is "unit" or "stack", blockName is the label.
func RewriteStackBlockSource(content []byte, blockType, blockName, newSource string) ([]byte, error) {
	f, diags := hclwrite.ParseConfig(content, "terragrunt.stack.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", diags.Error())
	}

	for _, block := range f.Body().Blocks() {
		if block.Type() != blockType {
			continue
		}

		labels := block.Labels()
		if len(labels) == 0 || labels[0] != blockName {
			continue
		}

		block.Body().SetAttributeValue("source", cty.StringVal(newSource))

		return f.Bytes(), nil
	}

	return nil, &WrappedError{
		Op:      blockType,
		Context: blockName,
		Err:     ErrBlockNotFound,
	}
}

// stackBlockInfo holds parsed information about a unit or stack block in a stack file.
type stackBlockInfo struct {
	Name                string
	Source              string
	BlockType           string
	UpdateSourceWithCAS bool
}

// ReadStackBlocks reads all unit and stack blocks from a stack HCL file,
// extracting the source and update_source_with_cas attributes. Blocks that set
// update_source_with_cas = true must use a literal source string; an
// interpolated source returns [ErrSourceNotLiteral] wrapped with the block
// type and name.
func ReadStackBlocks(content []byte) ([]stackBlockInfo, error) {
	f, diags := hclwrite.ParseConfig(content, "terragrunt.stack.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse stack HCL: %s", diags.Error())
	}

	var blocks []stackBlockInfo

	for _, block := range f.Body().Blocks() {
		bt := block.Type()
		if bt != "unit" && bt != "stack" {
			continue
		}

		labels := block.Labels()
		if len(labels) == 0 {
			continue
		}

		info := stackBlockInfo{
			Name:      labels[0],
			BlockType: bt,
		}

		var sourceErr error

		if attr := block.Body().GetAttribute("source"); attr != nil {
			info.Source, sourceErr = extractStringLiteral(attr)
		}

		if attr := block.Body().GetAttribute("update_source_with_cas"); attr != nil {
			info.UpdateSourceWithCAS = extractBoolLiteral(attr)
		}

		// A non-literal source only matters for blocks CAS will rewrite;
		// other blocks are skipped by the caller, so their sources stay
		// untouched and are evaluated later by the full HCL parser.
		if info.UpdateSourceWithCAS && sourceErr != nil {
			return nil, &WrappedError{
				Op:      bt,
				Context: info.Name,
				Err:     sourceErr,
			}
		}

		blocks = append(blocks, info)
	}

	return blocks, nil
}

// ReadTerraformSourceInfo reads the source and update_source_with_cas from a
// terraform block. When update_source_with_cas = true, the source must be a
// literal string; an interpolated source returns [ErrSourceNotLiteral].
func ReadTerraformSourceInfo(content []byte) (source string, updateWithCAS bool, err error) {
	f, diags := hclwrite.ParseConfig(content, "terragrunt.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		return "", false, fmt.Errorf("failed to parse HCL: %s", diags.Error())
	}

	for _, block := range f.Body().Blocks() {
		if block.Type() != "terraform" {
			continue
		}

		var sourceErr error

		if attr := block.Body().GetAttribute("source"); attr != nil {
			source, sourceErr = extractStringLiteral(attr)
		}

		if attr := block.Body().GetAttribute("update_source_with_cas"); attr != nil {
			updateWithCAS = extractBoolLiteral(attr)
		}

		// Without the rewrite opt-in the raw source is never consumed, so a
		// non-literal source is left for the full HCL parser to evaluate.
		if updateWithCAS && sourceErr != nil {
			return "", false, &WrappedError{
				Op:  "terraform",
				Err: sourceErr,
			}
		}

		return source, updateWithCAS, nil
	}

	return "", false, nil
}

// extractStringLiteral extracts a string value from an hclwrite attribute.
// Returns [ErrSourceNotLiteral] when the expression contains template
// interpolation ("${...}") or control ("%{...}") sequences, which cannot be
// evaluated by this raw-token reader. Escaped sequences ("$${", "%%{") are
// plain literal text and do not trigger the error. Returns empty string for
// other non-literal expressions.
func extractStringLiteral(attr *hclwrite.Attribute) (string, error) {
	tokens := attr.Expr().BuildTokens(nil)

	var b strings.Builder

	for _, tok := range tokens {
		if tok.Type == hclsyntax.TokenTemplateInterp || tok.Type == hclsyntax.TokenTemplateControl {
			return "", ErrSourceNotLiteral
		}

		if tok.Type == hclsyntax.TokenQuotedLit {
			b.Write(tok.Bytes)
		}
	}

	return b.String(), nil
}

// extractBoolLiteral extracts a boolean value from an hclwrite attribute.
// Returns false if the attribute is not a simple boolean literal.
func extractBoolLiteral(attr *hclwrite.Attribute) bool {
	tokens := attr.Expr().BuildTokens(nil)
	for _, tok := range tokens {
		if tok.Type == hclsyntax.TokenIdent {
			return string(tok.Bytes) == "true"
		}
	}

	return false
}
