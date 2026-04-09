package hclparse

import (
	"bytes"
	"slices"
	"strings"

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
		return rangeBytes(args.SrcBytes, expr.Range())
	}

	// Fast path: no deferred refs anywhere — evaluate the whole thing.
	if IsPure(expr, args.Deferred) {
		val, diags := expr.Value(args.EvalCtx)
		if !diags.HasErrors() {
			return valueToHCLBytes(val)
		}

		return rangeBytes(args.SrcBytes, expr.Range())
	}

	return partialEvalByType(expr, args)
}

// partialEvalByType dispatches to type-specific handlers for mixed expressions.
// Unhandled types fall through to verbatim source bytes.
func partialEvalByType(expr hclsyntax.Expression, args *EvalArgs) []byte {
	switch e := expr.(type) {
	case *hclsyntax.LiteralValueExpr:
		return valueToHCLBytes(e.Val)
	case *hclsyntax.ScopeTraversalExpr:
		return partialEvalTraversal(e, args)
	case *hclsyntax.TemplateExpr:
		return partialEvalTemplate(e, args)
	case *hclsyntax.TemplateWrapExpr:
		return rangeBytes(args.SrcBytes, e.Range())
	case *hclsyntax.ObjectConsExpr:
		return partialEvalObject(e, args)
	case *hclsyntax.TupleConsExpr:
		return partialEvalTuple(e, args)
	case *hclsyntax.ConditionalExpr:
		return partialEvalConditional(e, args)
	case *hclsyntax.ParenthesesExpr:
		return partialEvalParens(e, args)
	default:
		return rangeBytes(args.SrcBytes, e.Range())
	}
}

func partialEvalTraversal(e *hclsyntax.ScopeTraversalExpr, args *EvalArgs) []byte {
	if args.Deferred[e.Traversal.RootName()] {
		return rangeBytes(args.SrcBytes, e.Range())
	}

	val, diags := e.Value(args.EvalCtx)
	if !diags.HasErrors() {
		return valueToHCLBytes(val)
	}

	return rangeBytes(args.SrcBytes, e.Range())
}

func partialEvalConditional(e *hclsyntax.ConditionalExpr, args *EvalArgs) []byte {
	if !IsPure(e.Condition, args.Deferred) {
		return rangeBytes(args.SrcBytes, e.Range())
	}

	condVal, diags := e.Condition.Value(args.EvalCtx)
	if diags.HasErrors() {
		return rangeBytes(args.SrcBytes, e.Range())
	}

	boolVal, err := convert.Convert(condVal, cty.Bool)
	if err != nil {
		return rangeBytes(args.SrcBytes, e.Range())
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
			buf.WriteString(escapeHCLStringLit(lit.Val.AsString()))

			continue
		}

		if IsPure(part, args.Deferred) {
			val, diags := part.Value(args.EvalCtx)
			if !diags.HasErrors() && val.Type() == cty.String {
				buf.WriteString(escapeHCLStringLit(val.AsString()))

				continue
			}
		}

		// Deferred or eval failed — emit as interpolation.
		buf.WriteString("${")
		buf.Write(rangeBytes(args.SrcBytes, part.Range()))
		buf.WriteByte('}')
	}

	buf.WriteByte('"')

	return buf.Bytes()
}

func partialEvalObject(e *hclsyntax.ObjectConsExpr, args *EvalArgs) []byte {
	var buf bytes.Buffer

	buf.WriteString("{\n")

	for _, item := range e.Items {
		keyBytes := partialEvalObjectKey(item.KeyExpr, args)
		valBytes := PartialEval(item.ValueExpr, args)

		buf.WriteString("  ")
		buf.Write(keyBytes)
		buf.WriteString(" = ")
		buf.Write(valBytes)
		buf.WriteByte('\n')
	}

	buf.WriteByte('}')

	return buf.Bytes()
}

func partialEvalObjectKey(expr hclsyntax.Expression, args *EvalArgs) []byte {
	if keyExpr, ok := expr.(*hclsyntax.ObjectConsKeyExpr); ok {
		kw := hcl.ExprAsKeyword(keyExpr)
		if kw != "" {
			return []byte(kw)
		}

		return PartialEval(keyExpr.Wrapped, args)
	}

	return PartialEval(expr, args)
}

func partialEvalTuple(e *hclsyntax.TupleConsExpr, args *EvalArgs) []byte {
	var buf bytes.Buffer

	buf.WriteByte('[')

	for i, elem := range e.Exprs {
		if i > 0 {
			buf.WriteString(", ")
		}

		buf.Write(PartialEval(elem, args))
	}

	buf.WriteByte(']')

	return buf.Bytes()
}

func escapeHCLStringLit(s string) string {
	var buf strings.Builder

	buf.Grow(len(s))

	for _, r := range s {
		switch r {
		case '\\':
			buf.WriteString(`\\`)
		case '"':
			buf.WriteString(`\"`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			buf.WriteRune(r)
		}
	}

	result := buf.String()
	result = strings.ReplaceAll(result, "${", "$${")
	result = strings.ReplaceAll(result, "%{", "%%{")

	return result
}

func valueToHCLBytes(val cty.Value) []byte {
	tokens := hclwrite.TokensForValue(val)

	return tokens.Bytes()
}
