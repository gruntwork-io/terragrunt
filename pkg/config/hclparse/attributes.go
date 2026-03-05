package hclparse

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/customdecode"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
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
	case *hclsyntax.ObjectConsExpr:
		return evalObjectConsLazily(e, evalCtx)
	case *hclsyntax.FunctionCallExpr:
		return evalFunctionCallLazily(e, evalCtx)
	case *hclsyntax.TemplateExpr:
		return evalTemplateLazily(e, evalCtx)
	case *hclsyntax.TemplateWrapExpr:
		// TemplateWrapExpr is generated for a string that is a single interpolation,
		// e.g. "${cond ? run_cmd_a : run_cmd_b}". Delegate to the inner expression.
		return evalExpressionLazily(e.Wrapped, evalCtx)
	case *hclsyntax.ParenthesesExpr:
		return evalExpressionLazily(e.Expression, evalCtx)
	default:
		// The following hclsyntax expression types are NOT handled and fall through
		// to standard (eager) HCL evaluation. Nested conditionals inside them will
		// NOT benefit from lazy branch selection:
		//   - ForExpr: requires child EvalContext with iteration variables; complex to replicate
		//   - IndexExpr: collection[key] — key sub-expression evaluated eagerly
		//   - BinaryOpExpr / UnaryOpExpr: operand sub-expressions evaluated eagerly
		//   - RelativeTraversalExpr: source sub-expression evaluated eagerly
		//   - SplatExpr: source sub-expression evaluated eagerly
		// These are unlikely to wrap run_cmd ternaries in practice. If user demand arises,
		// IndexExpr and BinaryOpExpr are the simplest to add (same LiteralValueExpr pattern).
		return expr.Value(evalCtx)
	}
}

// evalConditionalLazily evaluates a conditional (ternary) expression lazily:
// it evaluates the condition first and then evaluates only the selected branch.
// This prevents side-effectful functions like run_cmd from executing in the
// unselected branch.
//
// Known divergences from upstream HCL behaviour for known-boolean conditions:
//  1. Type-consistency: standard HCL evaluates both branches to perform type-unification
//     checks (requiring true/false result types to be unifiable) and converts the selected
//     branch to the unified type. This implementation returns the selected branch as-is,
//     so mismatched branch types (e.g. string vs number) are not detected/converted.
//  2. Mark propagation: standard HCL collects cty marks from ALL three sub-expressions
//     (condition + both branches) and applies them to the result. This implementation only
//     propagates marks from the condition and the selected branch; marks on the unselected
//     branch are lost. Terragrunt does not use cty marks for sensitivity, so this is benign.
//
// For non-resolvable conditions (unknown, null, marked, non-bool), the fallback path
// delegates to standard HCL evaluation which preserves both behaviours.
func evalConditionalLazily(e *hclsyntax.ConditionalExpr, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	condVal, condDiags := evalExpressionLazily(e.Condition, evalCtx)
	if condDiags.HasErrors() {
		return cty.DynamicVal, condDiags
	}

	// Fall back to standard HCL evaluation for edge cases where the condition
	// value cannot be used for a short-circuit decision.
	// Substitute the already-evaluated condition as a LiteralValueExpr to
	// prevent double evaluation of side-effectful condition expressions.
	// Note: this fallback still evaluates BOTH branches eagerly (required by
	// HCL for type-unification). This is acceptable because the fallback only
	// triggers for unknown/null/marked/non-bool conditions, which are rare.
	if !condVal.IsKnown() || condVal.IsNull() || condVal.Type() != cty.Bool || condVal.IsMarked() {
		modifiedExpr := &hclsyntax.ConditionalExpr{
			Condition: &hclsyntax.LiteralValueExpr{
				Val:      condVal,
				SrcRange: e.Condition.Range(),
			},
			TrueResult:  e.TrueResult,
			FalseResult: e.FalseResult,
			SrcRange:    e.SrcRange,
		}

		val, fbDiags := modifiedExpr.Value(evalCtx)

		return val, append(condDiags, fbDiags...)
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
// Note: unlike standard HCL TupleConsExpr.Value(), this short-circuits on the
// first element error to prevent executing further side-effectful expressions.
func evalTupleConsLazily(e *hclsyntax.TupleConsExpr, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	if len(e.Exprs) == 0 {
		return cty.EmptyTupleVal, nil
	}

	vals := make([]cty.Value, len(e.Exprs))

	var diags hcl.Diagnostics

	for i, expr := range e.Exprs {
		val, valDiags := evalExpressionLazily(expr, evalCtx)

		diags = append(diags, valDiags...)
		if valDiags.HasErrors() {
			return cty.DynamicVal, diags
		}

		vals[i] = val
	}

	return cty.TupleVal(vals), diags
}

// evalObjectConsLazily evaluates an object construction expression by applying
// evalExpressionLazily to each value expression. Key expressions are kept as-is
// so that the original ObjectConsExpr.Value() can handle unknown keys, mark
// propagation, and object type construction as normal.
// Note: key expressions are evaluated eagerly by the delegated ObjectConsExpr.Value().
// Ternary expressions with side effects in object keys will not benefit from lazy eval.
func evalObjectConsLazily(e *hclsyntax.ObjectConsExpr, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	lazyItems := make([]hclsyntax.ObjectConsItem, len(e.Items))

	var diags hcl.Diagnostics

	for i, item := range e.Items {
		val, valDiags := evalExpressionLazily(item.ValueExpr, evalCtx)

		diags = append(diags, valDiags...)
		if valDiags.HasErrors() {
			// Return DynamicVal on error — matches HCL's own error-path return value
			// and avoids re-evaluating already-executed side-effectful args.
			return cty.DynamicVal, diags
		}

		lazyItems[i] = hclsyntax.ObjectConsItem{
			KeyExpr: item.KeyExpr,
			ValueExpr: &hclsyntax.LiteralValueExpr{
				Val:      val,
				SrcRange: item.ValueExpr.Range(),
			},
		}
	}

	modifiedExpr := &hclsyntax.ObjectConsExpr{
		Items:     lazyItems,
		SrcRange:  e.SrcRange,
		OpenRange: e.OpenRange,
	}

	val, callDiags := modifiedExpr.Value(evalCtx)

	return val, append(diags, callDiags...)
}

// evalFunctionCallLazily evaluates a function call expression by applying
// evalExpressionLazily to each argument before passing control to the original
// FunctionCallExpr.Value(). This ensures the selected branch of any conditional
// argument is the only one that executes, while all function-level logic (lookup,
// type-checking, ExpandFinal handling) is preserved unchanged.
func evalFunctionCallLazily(e *hclsyntax.FunctionCallExpr, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	// Check function existence before evaluating any args — mirrors HCL's own evaluation
	// order: an unknown function returns DynamicVal without touching its arguments.
	// Without this guard, run_cmd (or any side-effectful arg) would execute even when the
	// function itself doesn't exist.
	f, exists := lookupFunctionInCtx(evalCtx, e.Name)
	if !exists {
		return e.Value(evalCtx)
	}

	// Functions that use custom expression decoders (try, can, …) receive the raw
	// expression AST rather than a pre-evaluated value.  Converting their args to
	// LiteralValueExpr would break their error-catching semantics.
	if functionHasCustomArgDecoder(f) {
		return e.Value(evalCtx)
	}

	lazyArgs := make([]hclsyntax.Expression, len(e.Args))

	var diags hcl.Diagnostics

	for i, arg := range e.Args {
		val, argDiags := evalExpressionLazily(arg, evalCtx)

		diags = append(diags, argDiags...)
		if argDiags.HasErrors() {
			// Return DynamicVal on error — matches HCL's own error-path return value
			// and avoids re-evaluating already-executed side-effectful args.
			return cty.DynamicVal, diags
		}

		lazyArgs[i] = &hclsyntax.LiteralValueExpr{
			Val:      val,
			SrcRange: arg.Range(),
		}
	}

	modifiedExpr := &hclsyntax.FunctionCallExpr{
		Name:            e.Name,
		Args:            lazyArgs,
		ExpandFinal:     e.ExpandFinal,
		NameRange:       e.NameRange,
		OpenParenRange:  e.OpenParenRange,
		CloseParenRange: e.CloseParenRange,
	}

	val, callDiags := modifiedExpr.Value(evalCtx)

	return val, append(diags, callDiags...)
}

// evalTemplateLazily evaluates a template expression by applying evalExpressionLazily
// to each part and substituting results as LiteralValueExpr nodes. The modified
// TemplateExpr.Value() then handles string concatenation, unknown propagation, and
// mark handling as normal.
func evalTemplateLazily(e *hclsyntax.TemplateExpr, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	lazyParts := make([]hclsyntax.Expression, len(e.Parts))

	var diags hcl.Diagnostics

	for i, part := range e.Parts {
		val, partDiags := evalExpressionLazily(part, evalCtx)

		diags = append(diags, partDiags...)
		if partDiags.HasErrors() {
			// Return DynamicVal on error — matches HCL's own error-path return value
			// and avoids re-evaluating already-executed side-effectful args.
			return cty.DynamicVal, diags
		}

		lazyParts[i] = &hclsyntax.LiteralValueExpr{
			Val:      val,
			SrcRange: part.Range(),
		}
	}

	modifiedExpr := &hclsyntax.TemplateExpr{
		Parts:    lazyParts,
		SrcRange: e.SrcRange,
	}

	val, callDiags := modifiedExpr.Value(evalCtx)

	return val, append(diags, callDiags...)
}

// lookupFunctionInCtx walks the EvalContext parent chain to find a named function,
// mirroring the lookup order used by hclsyntax.FunctionCallExpr.Value().
func lookupFunctionInCtx(ctx *hcl.EvalContext, name string) (function.Function, bool) {
	for ctx != nil {
		if ctx.Functions != nil {
			if f, ok := ctx.Functions[name]; ok {
				return f, true
			}
		}

		ctx = ctx.Parent()
	}

	return function.Function{}, false
}

// functionHasCustomArgDecoder returns true if any of f's parameters uses a custom
// expression decoder (e.g. try, can). Those functions need the original expression
// AST rather than a pre-evaluated LiteralValueExpr.
func functionHasCustomArgDecoder(f function.Function) bool {
	for _, p := range f.Params() {
		if customdecode.CustomExpressionDecoderForType(p.Type) != nil {
			return true
		}
	}

	if vp := f.VarParam(); vp != nil {
		if customdecode.CustomExpressionDecoderForType(vp.Type) != nil {
			return true
		}
	}

	return false
}
