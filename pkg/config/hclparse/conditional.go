package hclparse

import (
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"
)

type conditionalRunCmdExpr struct {
	hclsyntax.Expression
	conditional *hclsyntax.ConditionalExpr
}

func (e *conditionalRunCmdExpr) Value(ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	condition, diags := e.conditional.Condition.Value(ctx)
	expr := *e.conditional
	expr.Condition = &evaluatedConditionExpr{
		Expression: expr.Condition,
		value:      condition,
		diags:      diags,
	}
	expr.TrueResult = &inactiveRunCmdExpr{Expression: expr.TrueResult}
	expr.FalseResult = &inactiveRunCmdExpr{Expression: expr.FalseResult}

	if !diags.HasErrors() {
		unmarked, _ := condition.Unmark()
		if boolean, err := convert.Convert(unmarked, cty.Bool); err == nil && boolean.IsKnown() && !boolean.IsNull() {
			if boolean.True() {
				expr.TrueResult = e.conditional.TrueResult
			} else {
				expr.FalseResult = e.conditional.FalseResult
			}
		}
	}

	return expr.Value(ctx)
}

type evaluatedConditionExpr struct {
	hclsyntax.Expression
	value cty.Value
	diags hcl.Diagnostics
}

func (e *evaluatedConditionExpr) Value(_ *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	return e.value, e.diags
}

type inactiveRunCmdExpr struct {
	hclsyntax.Expression
}

func (e *inactiveRunCmdExpr) Value(ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	for parent := ctx; parent != nil; parent = parent.Parent() {
		if fn, ok := parent.Functions["run_cmd"]; ok {
			child := ctx.NewChild()
			child.Functions = map[string]function.Function{"run_cmd": function.Unpredictable(fn)}

			return e.Expression.Value(child)
		}
	}

	return e.Expression.Value(ctx)
}

func hasConditionalRunCmd(expr hclsyntax.Expression) bool {
	var conditional, runCmd bool

	diags := hclsyntax.VisitAll(expr, func(node hclsyntax.Node) hcl.Diagnostics {
		switch node := node.(type) {
		case *hclsyntax.ConditionalExpr:
			conditional = true
		case *hclsyntax.FunctionCallExpr:
			runCmd = runCmd || node.Name == "run_cmd"
		}

		return nil
	})

	return !diags.HasErrors() && conditional && runCmd
}

// HCL needs both result types; inactive branches use an unknown run_cmd result instead of executing it.
// Copy expression nodes so evaluations never modify the parser's shared AST.
func wrapConditionalRunCmd(expr hclsyntax.Expression) hclsyntax.Expression {
	switch expr := expr.(type) {
	case *hclsyntax.ConditionalExpr:
		out := *expr
		out.Condition = wrapConditionalRunCmd(expr.Condition)
		out.TrueResult = wrapConditionalRunCmd(expr.TrueResult)
		out.FalseResult = wrapConditionalRunCmd(expr.FalseResult)

		return &conditionalRunCmdExpr{Expression: expr, conditional: &out}
	case *hclsyntax.ParenthesesExpr:
		out := *expr
		out.Expression = wrapConditionalRunCmd(expr.Expression)

		return &out
	case *hclsyntax.FunctionCallExpr:
		out := *expr
		out.Args = wrapConditionalRunCmdSlice(expr.Args)

		return &out
	case *hclsyntax.RelativeTraversalExpr:
		out := *expr
		out.Source = wrapConditionalRunCmd(expr.Source)

		return &out
	case *hclsyntax.IndexExpr:
		out := *expr
		out.Collection = wrapConditionalRunCmd(expr.Collection)
		out.Key = wrapConditionalRunCmd(expr.Key)

		return &out
	case *hclsyntax.TupleConsExpr:
		out := *expr
		out.Exprs = wrapConditionalRunCmdSlice(expr.Exprs)

		return &out
	case *hclsyntax.ObjectConsExpr:
		out := *expr
		out.Items = slices.Clone(expr.Items)

		for i := range out.Items {
			out.Items[i].KeyExpr = wrapConditionalRunCmd(out.Items[i].KeyExpr)
			out.Items[i].ValueExpr = wrapConditionalRunCmd(out.Items[i].ValueExpr)
		}

		return &out
	case *hclsyntax.ObjectConsKeyExpr:
		out := *expr
		out.Wrapped = wrapConditionalRunCmd(expr.Wrapped)

		return &out
	case *hclsyntax.ForExpr:
		out := *expr
		out.CollExpr = wrapConditionalRunCmd(expr.CollExpr)
		out.KeyExpr = wrapConditionalRunCmd(expr.KeyExpr)
		out.ValExpr = wrapConditionalRunCmd(expr.ValExpr)
		out.CondExpr = wrapConditionalRunCmd(expr.CondExpr)

		return &out
	case *hclsyntax.SplatExpr:
		out := *expr
		out.Source = wrapConditionalRunCmd(expr.Source)
		out.Each = wrapConditionalRunCmd(expr.Each)

		return &out
	case *hclsyntax.TemplateExpr:
		out := *expr
		out.Parts = wrapConditionalRunCmdSlice(expr.Parts)

		return &out
	case *hclsyntax.TemplateWrapExpr:
		out := *expr
		out.Wrapped = wrapConditionalRunCmd(expr.Wrapped)

		return &out
	case *hclsyntax.TemplateJoinExpr:
		out := *expr
		out.Tuple = wrapConditionalRunCmd(expr.Tuple)

		return &out
	case *hclsyntax.BinaryOpExpr:
		out := *expr
		out.LHS = wrapConditionalRunCmd(expr.LHS)
		out.RHS = wrapConditionalRunCmd(expr.RHS)

		return &out
	case *hclsyntax.UnaryOpExpr:
		out := *expr
		out.Val = wrapConditionalRunCmd(expr.Val)

		return &out
	default:
		return expr
	}
}

func wrapConditionalRunCmdSlice(exprs []hclsyntax.Expression) []hclsyntax.Expression {
	out := slices.Clone(exprs)
	for i := range out {
		out[i] = wrapConditionalRunCmd(out[i])
	}

	return out
}
