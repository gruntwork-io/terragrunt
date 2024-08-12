package hclparse

import (
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

type Attributes []*Attribute

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

func (attrs Attributes) ValidateIdentifier() error {
	for _, attr := range attrs {
		if err := attr.ValidateIdentifier(); err != nil {
			// TODO: Remove lint suppression
			return nil //nolint:nilerr
		}
	}

	return nil
}

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

type Attribute struct {
	*File
	*hcl.Attribute
}

func (attr *Attribute) ValidateIdentifier() error {
	if !hclsyntax.ValidIdentifier(attr.Name) {
		diags := hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid value name",
			Detail:   badIdentifierDetail,
			Subject:  &attr.NameRange,
		}}

		if err := attr.HandleDiagnostics(diags); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return nil
}

func (attr *Attribute) Value(evalCtx *hcl.EvalContext) (cty.Value, error) {
	evaluatedVal, diags := attr.Expr.Value(evalCtx)

	if err := attr.HandleDiagnostics(diags); err != nil {
		return evaluatedVal, errors.WithStackTrace(err)
	}

	return evaluatedVal, nil
}
