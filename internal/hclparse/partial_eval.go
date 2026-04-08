package hclparse

import (
	"bytes"
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

// PartialEval walks an hclsyntax.Expression tree and returns HCL source text.
// Pure expressions (no deferred refs) are fully evaluated to literals.
// Mixed expressions get per-child treatment: evaluable parts become literals,
// deferred parts keep their original source text.
func PartialEval(expr hclsyntax.Expression, srcBytes []byte, evalCtx *hcl.EvalContext, deferred map[string]bool) []byte {
	if evalCtx == nil {
		return rangeBytes(srcBytes, expr.Range())
	}

	// Fast path: no deferred refs anywhere — evaluate the whole thing.
	if IsPure(expr, deferred) {
		val, diags := expr.Value(evalCtx)
		if !diags.HasErrors() {
			return valueToHCLBytes(val)
		}

		return rangeBytes(srcBytes, expr.Range())
	}

	// Not pure — dispatch by expression type for partial evaluation.
	switch e := expr.(type) {
	case *hclsyntax.LiteralValueExpr:
		return valueToHCLBytes(e.Val)

	case *hclsyntax.ScopeTraversalExpr:
		if deferred[e.Traversal.RootName()] {
			return rangeBytes(srcBytes, e.Range())
		}

		val, diags := e.Value(evalCtx)
		if !diags.HasErrors() {
			return valueToHCLBytes(val)
		}

		return rangeBytes(srcBytes, e.Range())

	case *hclsyntax.TemplateExpr:
		return partialEvalTemplate(e, srcBytes, evalCtx, deferred)

	case *hclsyntax.TemplateWrapExpr:
		return rangeBytes(srcBytes, e.Range())

	case *hclsyntax.ObjectConsExpr:
		return partialEvalObject(e, srcBytes, evalCtx, deferred)

	case *hclsyntax.TupleConsExpr:
		return partialEvalTuple(e, srcBytes, evalCtx, deferred)

	case *hclsyntax.ConditionalExpr:
		if IsPure(e.Condition, deferred) {
			condVal, diags := e.Condition.Value(evalCtx)
			if !diags.HasErrors() {
				boolVal, err := convert.Convert(condVal, cty.Bool)
				if err == nil {
					if boolVal.True() {
						return PartialEval(e.TrueResult, srcBytes, evalCtx, deferred)
					}

					return PartialEval(e.FalseResult, srcBytes, evalCtx, deferred)
				}
			}
		}

		return rangeBytes(srcBytes, e.Range())

	case *hclsyntax.ParenthesesExpr:
		if IsPure(e.Expression, deferred) {
			return PartialEval(e.Expression, srcBytes, evalCtx, deferred)
		}

		inner := PartialEval(e.Expression, srcBytes, evalCtx, deferred)

		var buf bytes.Buffer

		buf.WriteByte('(')
		buf.Write(inner)
		buf.WriteByte(')')

		return buf.Bytes()

	default:
		// FunctionCallExpr, ForExpr, SplatExpr, BinaryOpExpr, UnaryOpExpr,
		// RelativeTraversalExpr, IndexExpr, AnonSymbolExpr — all verbatim.
		return rangeBytes(srcBytes, e.Range())
	}
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

func partialEvalTemplate(e *hclsyntax.TemplateExpr, srcBytes []byte, evalCtx *hcl.EvalContext, deferred map[string]bool) []byte {
	var buf bytes.Buffer

	buf.WriteByte('"')

	for _, part := range e.Parts {
		if lit, ok := part.(*hclsyntax.LiteralValueExpr); ok {
			buf.WriteString(escapeHCLStringLit(lit.Val.AsString()))

			continue
		}

		if IsPure(part, deferred) {
			val, diags := part.Value(evalCtx)
			if !diags.HasErrors() && val.Type() == cty.String {
				buf.WriteString(escapeHCLStringLit(val.AsString()))

				continue
			}
		}

		// Deferred or eval failed — emit as interpolation.
		buf.WriteString("${")
		buf.Write(rangeBytes(srcBytes, part.Range()))
		buf.WriteByte('}')
	}

	buf.WriteByte('"')

	return buf.Bytes()
}

func partialEvalObject(e *hclsyntax.ObjectConsExpr, srcBytes []byte, evalCtx *hcl.EvalContext, deferred map[string]bool) []byte {
	var buf bytes.Buffer

	buf.WriteString("{\n")

	for _, item := range e.Items {
		keyBytes := partialEvalObjectKey(item.KeyExpr, srcBytes, evalCtx, deferred)
		valBytes := PartialEval(item.ValueExpr, srcBytes, evalCtx, deferred)

		buf.WriteString("  ")
		buf.Write(keyBytes)
		buf.WriteString(" = ")
		buf.Write(valBytes)
		buf.WriteByte('\n')
	}

	buf.WriteByte('}')

	return buf.Bytes()
}

func partialEvalObjectKey(expr hclsyntax.Expression, srcBytes []byte, evalCtx *hcl.EvalContext, deferred map[string]bool) []byte {
	if keyExpr, ok := expr.(*hclsyntax.ObjectConsKeyExpr); ok {
		kw := hcl.ExprAsKeyword(keyExpr)
		if kw != "" {
			return []byte(kw)
		}

		return PartialEval(keyExpr.Wrapped, srcBytes, evalCtx, deferred)
	}

	return PartialEval(expr, srcBytes, evalCtx, deferred)
}

func partialEvalTuple(e *hclsyntax.TupleConsExpr, srcBytes []byte, evalCtx *hcl.EvalContext, deferred map[string]bool) []byte {
	var buf bytes.Buffer

	buf.WriteByte('[')

	for i, elem := range e.Exprs {
		if i > 0 {
			buf.WriteString(", ")
		}

		buf.Write(PartialEval(elem, srcBytes, evalCtx, deferred))
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
