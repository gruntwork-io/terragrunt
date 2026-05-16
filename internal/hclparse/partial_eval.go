package hclparse

import (
	"bytes"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// deferredRoots lists variable root names that cannot be evaluated at generation time.
// This map must not be modified after package initialization.
var deferredRoots = map[string]bool{
	varDependency: true,
}

// EvalArgs bundles the shared arguments for partial evaluation functions.
type EvalArgs struct {
	EvalCtx  *hcl.EvalContext
	Deferred map[string]bool
	SrcBytes []byte
}

// PartialEval walks an hclsyntax.Expression tree and returns HCL source text.
// Pure expressions (no deferred refs, no function calls) are evaluated to literals; mixed expressions get per-child treatment: evaluable parts become literals, deferred parts keep their original source text.
func PartialEval(expr hclsyntax.Expression, args *EvalArgs) []byte {
	if args.EvalCtx == nil {
		return RangeBytes(args.SrcBytes, expr.Range())
	}

	// Fast path: pure expression with no function calls, evaluate the whole thing (function calls are preserved because Terragrunt functions can have generation-time side effects).
	if IsPure(expr, args.Deferred) && !containsFunctionCall(expr) {
		val, diags := expr.Value(args.EvalCtx)
		// hclwrite.TokensForValue panics on unknown values; fall back to source bytes so the runtime parser sees the original ref.
		if !diags.HasErrors() && val.IsWhollyKnown() {
			return ValueToHCLBytes(val)
		}

		return RangeBytes(args.SrcBytes, expr.Range())
	}

	return partialEvalByType(expr, args)
}

// partialEvalByType dispatches to type-specific handlers for mixed expressions.
// Unhandled types fall through to verbatim source bytes.
func partialEvalByType(expr hclsyntax.Expression, args *EvalArgs) []byte {
	switch e := expr.(type) {
	case *hclsyntax.LiteralValueExpr:
		return ValueToHCLBytes(e.Val)
	case *hclsyntax.ScopeTraversalExpr:
		return partialEvalTraversal(e, args)
	case *hclsyntax.TemplateExpr:
		return partialEvalTemplate(e, args)
	case *hclsyntax.TemplateWrapExpr:
		return PartialEval(e.Wrapped, args)
	case *hclsyntax.FunctionCallExpr:
		return partialEvalFunctionCall(e, args)
	case *hclsyntax.ObjectConsExpr:
		return partialEvalObject(e, args)
	case *hclsyntax.TupleConsExpr:
		return partialEvalTuple(e, args)
	case *hclsyntax.ConditionalExpr:
		return partialEvalConditional(e, args)
	case *hclsyntax.ParenthesesExpr:
		return partialEvalParens(e, args)
	default:
		// Deliberate fallback: unhandled expression types (FunctionCallExpr, ForExpr,
		// SplatExpr, BinaryOpExpr, UnaryOpExpr, etc.) are emitted as verbatim source
		// bytes. This is safe: the generated HCL will contain the original expression
		// text, which is valid HCL that will be evaluated at runtime.
		return RangeBytes(args.SrcBytes, e.Range())
	}
}

func partialEvalTraversal(e *hclsyntax.ScopeTraversalExpr, args *EvalArgs) []byte {
	if args.Deferred[e.Traversal.RootName()] {
		return RangeBytes(args.SrcBytes, e.Range())
	}

	val, diags := e.Value(args.EvalCtx)
	if !diags.HasErrors() && val.IsWhollyKnown() {
		return ValueToHCLBytes(val)
	}

	return RangeBytes(args.SrcBytes, e.Range())
}

// stitchExpression rebuilds the source bytes of a parent expression with each child replaced by its PartialEval output. Gaps between/around children (operators, brackets, commas, whitespace) come from the source verbatim, so user formatting is preserved and there are no hard-coded separator strings.
func stitchExpression(args *EvalArgs, parentRange hcl.Range, children []hclsyntax.Expression) []byte {
	if len(children) == 0 {
		return RangeBytes(args.SrcBytes, parentRange)
	}

	src := args.SrcBytes
	out := make([]byte, 0, parentRange.End.Byte-parentRange.Start.Byte)
	cursor := parentRange.Start.Byte

	for _, child := range children {
		cr := child.Range()

		out = append(out, src[cursor:cr.Start.Byte]...)
		out = append(out, PartialEval(child, args)...)
		cursor = cr.End.Byte
	}

	out = append(out, src[cursor:parentRange.End.Byte]...)

	return out
}

func partialEvalConditional(e *hclsyntax.ConditionalExpr, args *EvalArgs) []byte {
	if !IsPure(e.Condition, args.Deferred) || containsFunctionCall(e.Condition) {
		return stitchExpression(args, e.Range(), []hclsyntax.Expression{e.Condition, e.TrueResult, e.FalseResult})
	}

	condVal, diags := e.Condition.Value(args.EvalCtx)
	if diags.HasErrors() {
		return RangeBytes(args.SrcBytes, e.Range())
	}

	boolVal, err := convert.Convert(condVal, cty.Bool)
	// Null condition would let True() silently return false (wrong branch); unknown would panic. Fall back to source bytes so runtime evaluation produces a faithful error.
	if err != nil || boolVal.IsNull() || !boolVal.IsKnown() {
		return RangeBytes(args.SrcBytes, e.Range())
	}

	if boolVal.True() {
		return PartialEval(e.TrueResult, args)
	}

	return PartialEval(e.FalseResult, args)
}

func partialEvalFunctionCall(e *hclsyntax.FunctionCallExpr, args *EvalArgs) []byte {
	children := make([]hclsyntax.Expression, len(e.Args))
	copy(children, e.Args)

	return stitchExpression(args, e.Range(), children)
}

func partialEvalParens(e *hclsyntax.ParenthesesExpr, args *EvalArgs) []byte {
	// Pure inner expression — parens are redundant around a single expression; emit the inner directly.
	if IsPure(e.Expression, args.Deferred) {
		return PartialEval(e.Expression, args)
	}

	return stitchExpression(args, e.Range(), []hclsyntax.Expression{e.Expression})
}

// IsPure returns true if the expression has no references to deferred root names.
func IsPure(expr hclsyntax.Expression, deferred map[string]bool) bool {
	for _, traversal := range expr.Variables() {
		if deferred[traversal.RootName()] {
			return false
		}
	}

	return true
}

// containsFunctionCall reports whether expr contains any FunctionCallExpr anywhere in its AST. Uses hclsyntax.Walk so any node type (including ones we don't enumerate) is covered.
func containsFunctionCall(expr hclsyntax.Expression) bool {
	w := &functionCallWalker{}

	_ = hclsyntax.Walk(expr, w)

	return w.found
}

// functionCallWalker is an hclsyntax.Walker that flips found=true on the first FunctionCallExpr it sees.
type functionCallWalker struct {
	found bool
}

func (w *functionCallWalker) Enter(node hclsyntax.Node) hcl.Diagnostics {
	if _, ok := node.(*hclsyntax.FunctionCallExpr); ok {
		w.found = true
	}

	return nil
}

func (w *functionCallWalker) Exit(_ hclsyntax.Node) hcl.Diagnostics {
	return nil
}

func partialEvalTemplate(e *hclsyntax.TemplateExpr, args *EvalArgs) []byte {
	var buf bytes.Buffer

	buf.WriteByte('"')

	for _, part := range e.Parts {
		if lit, ok := part.(*hclsyntax.LiteralValueExpr); ok {
			buf.Write(HCLStringContent(lit.Val.AsString()))

			continue
		}

		if IsPure(part, args.Deferred) && !containsFunctionCall(part) {
			val, diags := part.Value(args.EvalCtx)
			if !diags.HasErrors() && val.IsWhollyKnown() {
				strVal, err := convert.Convert(val, cty.String)
				// Null can't be stringified; fall through to emit as interpolation so runtime produces a faithful error.
				if err == nil && !strVal.IsNull() {
					buf.Write(HCLStringContent(strVal.AsString()))

					continue
				}
			}
		}

		// Deferred, function call, or eval failed: emit as interpolation.
		buf.WriteString("${")
		buf.Write(PartialEval(part, args))
		buf.WriteByte('}')
	}

	buf.WriteByte('"')

	return buf.Bytes()
}

// HCLStringContent returns the inner content of an HCL-escaped string
// (without surrounding quotes). Uses hclwrite.TokensForValue for correct
// escaping of all HCL special characters.
func HCLStringContent(s string) []byte {
	raw := hclwrite.TokensForValue(cty.StringVal(s)).Bytes()

	// TokensForValue produces `"escaped content"`: strip surrounding quotes.
	return bytes.TrimPrefix(bytes.TrimSuffix(raw, []byte{'"'}), []byte{'"'})
}

func partialEvalObject(e *hclsyntax.ObjectConsExpr, args *EvalArgs) []byte {
	// Stitch only the value expressions; keys + `=` + `,` + braces stay verbatim from the source.
	children := make([]hclsyntax.Expression, len(e.Items))
	for i, item := range e.Items {
		children[i] = item.ValueExpr
	}

	return stitchExpression(args, e.Range(), children)
}

func partialEvalTuple(e *hclsyntax.TupleConsExpr, args *EvalArgs) []byte {
	children := make([]hclsyntax.Expression, len(e.Exprs))
	copy(children, e.Exprs)

	return stitchExpression(args, e.Range(), children)
}

// ValueToHCLBytes converts a cty.Value to HCL source text bytes.
func ValueToHCLBytes(val cty.Value) []byte {
	tokens := hclwrite.TokensForValue(val)

	return tokens.Bytes()
}
