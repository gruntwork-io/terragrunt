package hclparser

import (
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

type Attributes []*Attribute

// GetAttrs loads the block into name expression pairs to assist with evaluation of the attrs prior to
// evaluating the whole config. Note that this is exactly the same as
// terraform/configs/named_values.go:decodeLocalsBlock
func NewAttributes(file *File, hclAttrs hcl.Attributes) Attributes {
	var attrs Attributes

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
			return nil
		}
	}

	return nil
}

// Attribute represents a single local name binding. This holds the unevaluated expression, extracted from the parsed file
// (but before decoding) so that we can look for references to other locals before evaluating.
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

		if err := attr.diagnosticsError(diags); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return nil
}

func (attr *Attribute) Value(evalCtx *hcl.EvalContext) (cty.Value, error) {
	evaluatedVal, diags := attr.Expr.Value(evalCtx)

	if err := attr.diagnosticsError(diags); err != nil {
		return evaluatedVal, errors.WithStackTrace(err)
	}

	return evaluatedVal, nil
}
