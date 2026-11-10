package hclparse

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Attributes lists the attributes of a block or file body.
type Attributes []*Attribute

// NewAttributes wraps hclAttrs, binding each to file so its diagnostics go through the file's parser. The order is
// random, since hclAttrs is a map.
func NewAttributes(file *File, hclAttrs hcl.Attributes) Attributes {
	var attrs = make(Attributes, 0, len(hclAttrs))

	for _, hclAttr := range hclAttrs {
		attrs = append(attrs, &Attribute{
			File:      file,
			Attribute: hclAttr,
		})
	}

	return attrs
}

// ValidateIdentifier returns the error of the first attribute whose name is not a valid identifier.
func (attrs Attributes) ValidateIdentifier() error {
	for _, attr := range attrs {
		if err := attr.ValidateIdentifier(); err != nil {
			return err
		}
	}

	return nil
}

// Range returns a range in the file of attrs that spans every attribute.
func (attrs Attributes) Range() hcl.Range {
	var rng hcl.Range

	for _, attr := range attrs {
		rng.Filename = attr.Range.Filename

		if rng.Start.Line > attr.Range.Start.Line || rng.Start.Column > attr.Range.Start.Column {
			rng.Start = attr.Range.Start
		}

		if rng.End.Line < attr.Range.End.Line || rng.End.Column < attr.Range.End.Column {
			rng.End = attr.Range.End
		}
	}

	return rng
}

// Attribute is an HCL attribute bound to the [File] that declares it.
type Attribute struct {
	*File
	*hcl.Attribute
}

// ValidateIdentifier reports a diagnostic through the file's parser when the attribute name is not a valid
// identifier.
func (attr *Attribute) ValidateIdentifier() error {
	if !hclsyntax.ValidIdentifier(attr.Name) {
		diags := hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid value name",
			Detail:   badIdentifierDetail,
			Subject:  &attr.NameRange,
		}}

		if err := attr.HandleDiagnostics(diags); err != nil {
			return err
		}
	}

	return nil
}

// Value evaluates the attribute's expression in evalCtx. It returns the value even when evaluation fails.
func (attr *Attribute) Value(evalCtx *hcl.EvalContext) (cty.Value, error) {
	evaluatedVal, diags := attr.Expr.Value(evalCtx)

	if err := attr.HandleDiagnostics(diags); err != nil {
		return evaluatedVal, err
	}

	return evaluatedVal, nil
}
