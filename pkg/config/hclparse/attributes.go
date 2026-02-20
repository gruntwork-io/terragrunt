package hclparse

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
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
			return errors.New(err)
		}
	}

	return nil
}

func (attr *Attribute) Value(evalCtx *hcl.EvalContext) (cty.Value, error) {
	evaluatedVal, diags := evalExpressionLazily(attr.Expr, evalCtx)

	if err := attr.HandleDiagnostics(diags); err != nil {
		return evaluatedVal, errors.New(err)
	}

	return evaluatedVal, nil
}

// evalExpressionLazily evaluates an HCL expression with lazy conditional evaluation.
// For ternary/conditional expressions, only the selected branch is evaluated,
// preventing side-effectful functions like run_cmd from executing in the unselected branch.
// Container expression types (tuples, template wrappers, parentheses) are traversed
// recursively so that nested conditionals are also handled lazily.
func evalExpressionLazily(expr hcl.Expression, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	switch e := expr.(type) {
	case *hclsyntax.ConditionalExpr:
		return evalConditionalLazily(e, evalCtx)
	case *hclsyntax.TupleConsExpr:
		return evalTupleConsLazily(e, evalCtx)
	case *hclsyntax.TemplateWrapExpr:
		// TemplateWrapExpr is generated for a string that is a single interpolation,
		// e.g. "${cond ? run_cmd_a : run_cmd_b}". Delegate to the inner expression.
		return evalExpressionLazily(e.Wrapped, evalCtx)
	case *hclsyntax.ParenthesesExpr:
		return evalExpressionLazily(e.Expression, evalCtx)
	default:
		return expr.Value(evalCtx)
	}
}

// evalConditionalLazily evaluates a conditional (ternary) expression lazily:
// it evaluates the condition first and then evaluates only the selected branch.
// This prevents side-effectful functions like run_cmd from executing in the
// unselected branch.
//
// Known divergence from upstream HCL behaviour: standard HCL's ConditionalExpr
// evaluates both branches to perform branch type-consistency checks (requiring the
// true and false result types to be unifiable). This implementation skips that check
// for known-boolean conditions. The selected branch's value is returned as-is.
// Users who rely on HCL rejecting mismatched branch types should be aware that
// Terragrunt will not surface that error when the condition is resolvable at parse time.
func evalConditionalLazily(e *hclsyntax.ConditionalExpr, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	condVal, condDiags := evalExpressionLazily(e.Condition, evalCtx)
	if condDiags.HasErrors() {
		return cty.NilVal, condDiags
	}

	// Fall back to standard HCL evaluation for edge cases where the condition
	// value cannot be used for a short-circuit decision.
	if !condVal.IsKnown() || condVal.IsNull() || condVal.Type() != cty.Bool || condVal.IsMarked() {
		return e.Value(evalCtx)
	}

	if condVal.True() {
		selectedVal, selectedDiags := evalExpressionLazily(e.TrueResult, evalCtx)
		return selectedVal, append(condDiags, selectedDiags...)
	}

	selectedVal, selectedDiags := evalExpressionLazily(e.FalseResult, evalCtx)

	return selectedVal, append(condDiags, selectedDiags...)
}

// evalTupleConsLazily evaluates a tuple construction expression by applying
// evalExpressionLazily to each element. This allows nested conditional expressions
// inside list literals to benefit from lazy branch selection.
func evalTupleConsLazily(e *hclsyntax.TupleConsExpr, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	if len(e.Exprs) == 0 {
		return cty.TupleVal(nil), nil
	}

	vals := make([]cty.Value, len(e.Exprs))

	var diags hcl.Diagnostics

	for i, expr := range e.Exprs {
		val, valDiags := evalExpressionLazily(expr, evalCtx)
		vals[i] = val

		diags = append(diags, valDiags...)
	}

	return cty.TupleVal(vals), diags
}
