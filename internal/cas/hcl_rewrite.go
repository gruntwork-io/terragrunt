package cas

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
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
func RewriteStackBlockSource(
	content []byte,
	blockType, blockName, newSource string,
) ([]byte, error) {
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
// update_source_with_cas = true must use a literal source string; a
// non-literal source returns [ErrSourceNotLiteral] wrapped with the block
// type and name. An update_source_with_cas value that [extractBool] cannot
// evaluate returns [ErrUpdateSourceWithCASNotConstant], wrapped the same way.
func ReadStackBlocks(content []byte) ([]stackBlockInfo, error) {
	f, diags := hclwrite.ParseConfig(content, "terragrunt.stack.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse stack HCL: %s", diags.Error())
	}

	var blocks []stackBlockInfo

	evalCtx := constantLocals(f.Body())

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
			updateWithCAS, err := extractBool(attr, evalCtx)
			if err != nil {
				return nil, &WrappedError{
					Op:      bt,
					Context: info.Name,
					Err:     err,
				}
			}

			info.UpdateSourceWithCAS = updateWithCAS
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
// literal string; a non-literal source returns [ErrSourceNotLiteral]. An
// update_source_with_cas value that [extractBool] cannot evaluate returns
// [ErrUpdateSourceWithCASNotConstant].
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
			updateWithCAS, err = extractBool(attr, constantLocals(f.Body()))
			if err != nil {
				return "", false, &WrappedError{
					Op:  "terraform",
					Err: err,
				}
			}
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
// The expression must be a pure quoted string literal: an opening quote,
// quoted-literal parts, and a closing quote. Escaped template sequences
// ("$${", "%%{") tokenize as quoted-literal parts and are accepted. Any other
// token shape, such as template interpolation, function calls, references
// like local.foo, heredocs, or operators, returns [ErrSourceNotLiteral]
// wrapped with the kind of expression found. This raw-token reader cannot
// evaluate expressions, so anything it cannot read verbatim is rejected
// rather than concatenated into a wrong source.
func extractStringLiteral(attr *hclwrite.Attribute) (string, error) {
	tokens := attr.Expr().BuildTokens(nil)

	if len(tokens) < 2 ||
		tokens[0].Type != hclsyntax.TokenOQuote ||
		tokens[len(tokens)-1].Type != hclsyntax.TokenCQuote {
		return "", fmt.Errorf("%w; the source is a %s", ErrSourceNotLiteral, nonLiteralKind(tokens))
	}

	var b strings.Builder

	for _, tok := range tokens[1 : len(tokens)-1] {
		if tok.Type != hclsyntax.TokenQuotedLit {
			return "", fmt.Errorf(
				"%w; the source is a %s",
				ErrSourceNotLiteral,
				nonLiteralKind(tokens),
			)
		}

		b.Write(tok.Bytes)
	}

	return b.String(), nil
}

// nonLiteralKind names the shape of a non-literal expression for error
// messages. The classification is best-effort: every shape maps to
// [ErrSourceNotLiteral] either way, so an imprecise name only affects the
// message text.
func nonLiteralKind(tokens hclwrite.Tokens) string {
	for _, tok := range tokens {
		if tok.Type == hclsyntax.TokenTemplateInterp || tok.Type == hclsyntax.TokenTemplateControl {
			return "template expression"
		}
	}

	if len(tokens) == 0 {
		return "non-literal expression"
	}

	if tokens[0].Type == hclsyntax.TokenOHeredoc {
		return "heredoc"
	}

	if tokens[0].Type == hclsyntax.TokenIdent {
		if len(tokens) > 1 && tokens[1].Type == hclsyntax.TokenOParen {
			return "function call"
		}

		return "reference"
	}

	return "non-literal expression"
}

// extractBool evaluates a bool attribute with the locals from [constantLocals].
// `!false`, `true && false` and `local.use_cas` get the value the full HCL
// parser decodes. A null value counts as false, like an unset attribute. A
// function call, or a local that calls one, returns
// [ErrUpdateSourceWithCASNotConstant], because this reader runs before the
// full parser and has no functions to call.
func extractBool(attr *hclwrite.Attribute, evalCtx *hcl.EvalContext) (bool, error) {
	expr, diags := parseAttrExpr(attr)
	if diags.HasErrors() {
		return false, ErrUpdateSourceWithCASNotConstant
	}

	val, diags := expr.Value(evalCtx)
	if diags.HasErrors() {
		return false, ErrUpdateSourceWithCASNotConstant
	}

	val, err := convert.Convert(val, cty.Bool)
	if err != nil {
		return false, ErrUpdateSourceWithCASNotConstant
	}

	if val.IsNull() {
		return false, nil
	}

	return val.True(), nil
}

// constantLocals returns an evaluation context that sets local.<name> for
// each local in body that evaluates without functions or variables other
// than local. It skips a local that calls a function or reads one that does,
// so a reference to that local fails to evaluate.
func constantLocals(body *hclwrite.Body) *hcl.EvalContext {
	exprs := map[string]hclsyntax.Expression{}

	for _, block := range body.Blocks() {
		if block.Type() != "locals" {
			continue
		}

		for name, attr := range block.Body().Attributes() {
			expr, diags := parseAttrExpr(attr)
			if diags.HasErrors() {
				continue
			}

			exprs[name] = expr
		}
	}

	locals := map[string]cty.Value{}

	// A pass that resolves nothing ends the loop, so len(exprs) passes cover
	// the longest chain of locals.
	for range len(exprs) {
		evalCtx := &hcl.EvalContext{Variables: map[string]cty.Value{"local": cty.ObjectVal(locals)}}
		resolved := false

		for name, expr := range exprs {
			if _, ok := locals[name]; ok {
				continue
			}

			val, diags := expr.Value(evalCtx)
			if diags.HasErrors() || !val.IsWhollyKnown() {
				continue
			}

			locals[name] = val
			resolved = true
		}

		if !resolved {
			break
		}
	}

	return &hcl.EvalContext{Variables: map[string]cty.Value{"local": cty.ObjectVal(locals)}}
}

// parseAttrExpr parses the expression of an hclwrite attribute so it can be
// evaluated.
func parseAttrExpr(attr *hclwrite.Attribute) (hclsyntax.Expression, hcl.Diagnostics) {
	return hclsyntax.ParseExpression(attr.Expr().BuildTokens(nil).Bytes(), "", hcl.InitialPos)
}
