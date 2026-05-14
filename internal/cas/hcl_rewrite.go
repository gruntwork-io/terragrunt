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
// extracting the source and update_source_with_cas attributes.
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

		if attr := block.Body().GetAttribute("source"); attr != nil {
			info.Source = extractStringLiteral(attr)
		}

		if attr := block.Body().GetAttribute("update_source_with_cas"); attr != nil {
			info.UpdateSourceWithCAS = extractBoolLiteral(attr)
		}

		blocks = append(blocks, info)
	}

	return blocks, nil
}

// ReadTerraformSourceInfo reads the source and update_source_with_cas from a terraform block.
func ReadTerraformSourceInfo(content []byte) (source string, updateWithCAS bool, err error) {
	f, diags := hclwrite.ParseConfig(content, "terragrunt.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		return "", false, fmt.Errorf("failed to parse HCL: %s", diags.Error())
	}

	for _, block := range f.Body().Blocks() {
		if block.Type() != "terraform" {
			continue
		}

		if attr := block.Body().GetAttribute("source"); attr != nil {
			source = extractStringLiteral(attr)
		}

		if attr := block.Body().GetAttribute("update_source_with_cas"); attr != nil {
			updateWithCAS = extractBoolLiteral(attr)
		}

		return source, updateWithCAS, nil
	}

	return "", false, nil
}

// extractStringLiteral extracts a string value from an hclwrite attribute.
// Returns empty string if the attribute is not a simple string literal.
func extractStringLiteral(attr *hclwrite.Attribute) string {
	tokens := attr.Expr().BuildTokens(nil)

	var b strings.Builder

	for _, tok := range tokens {
		if tok.Type == hclsyntax.TokenQuotedLit {
			b.Write(tok.Bytes)
		}
	}

	return b.String()
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
