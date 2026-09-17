package hclparse

import (
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// BodyAsCty renders the block's body the way a JSON config writes it. An expression that cannot
// be evaluated on its own, which is every reference to the iteration a block expands over,
// becomes the template a JSON config spells it with.
func (src *SourceBlock) BodyAsCty() (cty.Value, error) {
	source, err := src.Body()
	if err != nil {
		return cty.NilVal, err
	}

	text := []byte(source)

	file, diags := hclsyntax.ParseConfig(text, src.Range.Filename, hcl.InitialPos)
	if diags.HasErrors() {
		return cty.NilVal, diags
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok || len(body.Blocks) != 1 {
		return cty.NilVal, UnquotableBlockError{Subject: &src.Range}
	}

	return bodyAsCty(body.Blocks[0].Body, text)
}

func bodyAsCty(body *hclsyntax.Body, text []byte) (cty.Value, error) {
	out := map[string]cty.Value{}

	for name, attr := range body.Attributes {
		value, err := exprAsCty(attr.Expr, text)
		if err != nil {
			return cty.NilVal, err
		}

		out[name] = value
	}

	for _, block := range body.Blocks {
		if len(block.Labels) > 0 {
			blockRange := block.DefRange()

			return cty.NilVal, UnquotableBlockError{Subject: &blockRange}
		}

		value, err := bodyAsCty(block.Body, text)
		if err != nil {
			return cty.NilVal, err
		}

		out[block.Type] = value
	}

	return cty.ObjectVal(out), nil
}

func exprAsCty(expr hclsyntax.Expression, text []byte) (cty.Value, error) {
	switch expr := expr.(type) {
	case *hclsyntax.TemplateExpr:
		rendered, err := templateAsString(expr, text)
		if err != nil {
			return cty.NilVal, err
		}

		return cty.StringVal(rendered), nil
	case *hclsyntax.TemplateWrapExpr:
		// Wrapping this again would nest a string in a string and turn what it evaluates to
		// into text.
		return cty.StringVal("${" + exprText(expr.Wrapped, text) + "}"), nil
	case *hclsyntax.ObjectConsExpr:
		return objectConsAsCty(expr, text)
	case *hclsyntax.TupleConsExpr:
		return tupleConsAsCty(expr, text)
	}

	if value, diags := expr.Value(nil); !diags.HasErrors() && value.IsWhollyKnown() {
		return value, nil
	}

	// JSON reads a string as a template, and a template that is a single interpolation keeps the
	// type of whatever that interpolation evaluates to.
	return cty.StringVal("${" + exprText(expr, text) + "}"), nil
}

// objectConsAsCty renders an object one key at a time, so a reference in one value does not turn
// the whole object into a string.
func objectConsAsCty(expr *hclsyntax.ObjectConsExpr, text []byte) (cty.Value, error) {
	out := map[string]cty.Value{}

	for _, item := range expr.Items {
		key, diags := item.KeyExpr.Value(nil)
		if diags.HasErrors() || key.Type() != cty.String || !key.IsKnown() {
			// A key JSON cannot spell, such as one computed from the iteration. The object goes
			// back whole, as the one interpolation that rebuilds it.
			return cty.StringVal("${" + exprText(expr, text) + "}"), nil
		}

		value, err := exprAsCty(item.ValueExpr, text)
		if err != nil {
			return cty.NilVal, err
		}

		out[key.AsString()] = value
	}

	return cty.ObjectVal(out), nil
}

func tupleConsAsCty(expr *hclsyntax.TupleConsExpr, text []byte) (cty.Value, error) {
	out := make([]cty.Value, 0, len(expr.Exprs))

	for _, element := range expr.Exprs {
		value, err := exprAsCty(element, text)
		if err != nil {
			return cty.NilVal, err
		}

		out = append(out, value)
	}

	return cty.TupleVal(out), nil
}

// templateAsString rebuilds a template from its own source, unevaluated.
func templateAsString(template *hclsyntax.TemplateExpr, text []byte) (string, error) {
	var sb strings.Builder

	for _, part := range template.Parts {
		literal, ok := part.(*hclsyntax.LiteralValueExpr)
		if !ok {
			source := exprText(part, text)

			// A directive is already written in template syntax and spans its own markers, so
			// an interpolation around it would be read as part of the expression it controls.
			if strings.HasPrefix(source, "%{") {
				sb.WriteString(source)

				continue
			}

			sb.WriteString("${" + source + "}")

			continue
		}

		if literal.Val.Type() != cty.String {
			return "", UnquotableBlockError{Subject: &template.SrcRange}
		}

		sb.WriteString(escapeTemplateMarkers(literal.Val.AsString()))
	}

	return sb.String(), nil
}

func exprText(expr hclsyntax.Expression, text []byte) string {
	rng := expr.Range()

	return string(text[rng.Start.Byte:rng.End.Byte])
}
