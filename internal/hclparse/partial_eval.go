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
// Pure expressions (no deferred refs) are fully evaluated to literals.
// Mixed expressions get per-child treatment: evaluable parts become literals,
// deferred parts keep their original source text.
func PartialEval(expr hclsyntax.Expression, args *EvalArgs) []byte {
	if args.EvalCtx == nil {
		return RangeBytes(args.SrcBytes, expr.Range())
	}

	// Fast path: no deferred refs anywhere — evaluate the whole thing.
	if IsPure(expr, args.Deferred) {
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
		return RangeBytes(args.SrcBytes, e.Range())
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
		// bytes. This is safe — the generated HCL will contain the original expression
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
	if !IsPure(e.Condition, args.Deferred) {
		return RangeBytes(args.SrcBytes, e.Range())
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

func partialEvalParens(e *hclsyntax.ParenthesesExpr, args *EvalArgs) []byte {
	inner := PartialEval(e.Expression, args)

	// Pure expressions evaluate to literals — parens not needed.
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

func partialEvalTemplate(e *hclsyntax.TemplateExpr, args *EvalArgs) []byte {
	var buf bytes.Buffer

	buf.WriteByte('"')

	for _, part := range e.Parts {
		if lit, ok := part.(*hclsyntax.LiteralValueExpr); ok {
			buf.Write(HCLStringContent(lit.Val.AsString()))

			continue
		}

		if IsPure(part, args.Deferred) {
			val, diags := part.Value(args.EvalCtx)
			if !diags.HasErrors() && val.Type() == cty.String {
				buf.Write(HCLStringContent(val.AsString()))

				continue
			}
		}

		// Deferred or eval failed — emit as interpolation.
		buf.WriteString("${")
		buf.Write(RangeBytes(args.SrcBytes, part.Range()))
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

	// TokensForValue produces `"escaped content"` — strip surrounding quotes.
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

func ValueToHCLBytes(val cty.Value) []byte {
	tokens := hclwrite.TokensForValue(val)

	return tokens.Bytes()
}
