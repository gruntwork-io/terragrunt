package hclparse

import (
	"fmt"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Attributes is a collection of attributes.
type Attributes []*Attribute

// NewAttributes creates a new collection of attributes from a map of HCL attributes.
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

// ValidateIdentifier validates the identifier of each attribute in the collection.
func (attrs Attributes) ValidateIdentifier() error {
	for _, attr := range attrs {
		if err := attr.ValidateIdentifier(); err != nil {
			// TODO: Remove lint suppression
			return nil //nolint:nilerr
		}
	}

	return nil
}

// Range returns the range of the collection of attributes.
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

// Attribute represents an attribute in an HCL file.
type Attribute struct {
	*File
	*hcl.Attribute
}

// ValidateIdentifier validates the identifier of the attribute.
func (attr *Attribute) ValidateIdentifier() error {
	if !hclsyntax.ValidIdentifier(attr.Name) {
		diags := hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid value name",
			Detail:   badIdentifierDetail,
			Subject:  &attr.NameRange,
		}}

		if err := attr.HandleDiagnostics(diags); err != nil {
			return fmt.Errorf("identifier %s is invalid: %w", attr.Name, errors.WithStackTrace(err))
		}
	}

	return nil
}

// Value returns the value of the attribute.
func (attr *Attribute) Value(evalCtx *hcl.EvalContext) (cty.Value, error) {
	evaluatedVal, diags := attr.Expr.Value(evalCtx)

	if err := attr.HandleDiagnostics(diags); err != nil {
		return evaluatedVal, fmt.Errorf("failed to evaluate attribute %s: %w", attr.Name, errors.WithStackTrace(err))
	}

	return evaluatedVal, nil
}
