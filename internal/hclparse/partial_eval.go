package hclparse

import (
	"bytes"
	"slices"

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
		if !diags.HasErrors() {
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
	if !diags.HasErrors() {
		return ValueToHCLBytes(val)
	}

	return RangeBytes(args.SrcBytes, e.Range())
}

func partialEvalConditional(e *hclsyntax.ConditionalExpr, args *EvalArgs) []byte {
	if !IsPure(e.Condition, args.Deferred) || containsFunctionCall(e.Condition) {
		return slices.Concat(
			PartialEval(e.Condition, args),
			[]byte(" ? "),
			PartialEval(e.TrueResult, args),
			[]byte(" : "),
			PartialEval(e.FalseResult, args),
		)
	}

	condVal, diags := e.Condition.Value(args.EvalCtx)
	if diags.HasErrors() {
		return RangeBytes(args.SrcBytes, e.Range())
	}

	boolVal, err := convert.Convert(condVal, cty.Bool)
	if err != nil {
		return RangeBytes(args.SrcBytes, e.Range())
	}

	if boolVal.True() {
		return PartialEval(e.TrueResult, args)
	}

	return PartialEval(e.FalseResult, args)
}

func partialEvalFunctionCall(e *hclsyntax.FunctionCallExpr, args *EvalArgs) []byte {
	callArgs := make([][]byte, 0, len(e.Args))

	for i, arg := range e.Args {
		evaluated := PartialEval(arg, args)
		if e.ExpandFinal && i == len(e.Args)-1 {
			evaluated = slices.Concat(evaluated, []byte("..."))
		}

		callArgs = append(callArgs, evaluated)
	}

	return slices.Concat([]byte(e.Name+"("), bytes.Join(callArgs, []byte(", ")), []byte(")"))
}

func partialEvalParens(e *hclsyntax.ParenthesesExpr, args *EvalArgs) []byte {
	inner := PartialEval(e.Expression, args)

	// Pure expressions evaluate to literals: parens not needed.
	if IsPure(e.Expression, args.Deferred) {
		return inner
	}

	// Wrap deferred expressions in parentheses.
	return slices.Concat([]byte("("), inner, []byte(")"))
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

func containsFunctionCall(expr hclsyntax.Expression) bool {
	switch e := expr.(type) {
	case *hclsyntax.FunctionCallExpr:
		return true
	case *hclsyntax.TemplateExpr:
		for _, part := range e.Parts {
			if containsFunctionCall(part) {
				return true
			}
		}
	case *hclsyntax.TemplateWrapExpr:
		return containsFunctionCall(e.Wrapped)
	case *hclsyntax.ObjectConsExpr:
		for _, item := range e.Items {
			if containsFunctionCall(item.KeyExpr) || containsFunctionCall(item.ValueExpr) {
				return true
			}
		}
	case *hclsyntax.TupleConsExpr:
		for _, elem := range e.Exprs {
			if containsFunctionCall(elem) {
				return true
			}
		}
	case *hclsyntax.ConditionalExpr:
		return containsFunctionCall(e.Condition) || containsFunctionCall(e.TrueResult) || containsFunctionCall(e.FalseResult)
	case *hclsyntax.ParenthesesExpr:
		return containsFunctionCall(e.Expression)
	case *hclsyntax.BinaryOpExpr:
		return containsFunctionCall(e.LHS) || containsFunctionCall(e.RHS)
	case *hclsyntax.UnaryOpExpr:
		return containsFunctionCall(e.Val)
	case *hclsyntax.IndexExpr:
		return containsFunctionCall(e.Collection) || containsFunctionCall(e.Key)
	case *hclsyntax.ForExpr:
		return containsFunctionCall(e.CollExpr) ||
			(e.KeyExpr != nil && containsFunctionCall(e.KeyExpr)) ||
			containsFunctionCall(e.ValExpr) ||
			(e.CondExpr != nil && containsFunctionCall(e.CondExpr))
	case *hclsyntax.SplatExpr:
		return containsFunctionCall(e.Source) || containsFunctionCall(e.Each)
	default:
		return false
	}

	return false
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
			if !diags.HasErrors() {
				strVal, err := convert.Convert(val, cty.String)
				if err == nil {
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
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	for _, item := range e.Items {
		key := objectKeyName(item.KeyExpr, args.SrcBytes)
		body.SetAttributeRaw(key, RawTokens(PartialEval(item.ValueExpr, args)))
	}

	// hclwrite produces file-level attributes; wrap in braces for object syntax.
	inner := bytes.TrimSpace(f.Bytes())

	return slices.Concat([]byte("{\n"), inner, []byte("\n}"))
}

func objectKeyName(expr hclsyntax.Expression, srcBytes []byte) string {
	if keyExpr, ok := expr.(*hclsyntax.ObjectConsKeyExpr); ok {
		kw := hcl.ExprAsKeyword(keyExpr)
		if kw != "" {
			return kw
		}
	}

	return string(RangeBytes(srcBytes, expr.Range()))
}

func partialEvalTuple(e *hclsyntax.TupleConsExpr, args *EvalArgs) []byte {
	parts := make([][]byte, 0, len(e.Exprs))

	for _, elem := range e.Exprs {
		parts = append(parts, PartialEval(elem, args))
	}

	return slices.Concat([]byte("["), bytes.Join(parts, []byte(", ")), []byte("]"))
}

// ValueToHCLBytes converts a cty.Value to HCL source text bytes.
func ValueToHCLBytes(val cty.Value) []byte {
	tokens := hclwrite.TokensForValue(val)

	return tokens.Bytes()
}
