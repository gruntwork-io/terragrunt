package hclparse

import (
	"bytes"
	"errors"
	"maps"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// deferredRoots lists variable root names that cannot be evaluated at generation time.
// dependency.* is the sole deferred root; it resolves inside the generated unit at run time.
// Every other namespace, including values.* (the stack file's values), local.*, unit.*, and
// stack.*, resolves at generate time in the terragrunt.stack.hcl context.
// This map must not be modified after package initialization.
var deferredRoots = map[string]bool{
	varDependency: true,
}

// maxPartialEvalDepth bounds recursion for pathological deeply-nested expressions; past this, fall back to source bytes.
const maxPartialEvalDepth = 10000

// EvalArgs bundles the shared arguments for partial evaluation functions.
type EvalArgs struct {
	EvalCtx  *hcl.EvalContext
	Deferred map[string]bool
	SrcBytes []byte
	depth    int
}

// PartialEval walks an hclsyntax.Expression tree and returns HCL source text; pure parts evaluate to literals, deferred parts stay verbatim, error signals pathological inputs.
func PartialEval(expr hclsyntax.Expression, args *EvalArgs) ([]byte, error) {
	if args.EvalCtx == nil {
		return RangeBytes(args.SrcBytes, expr.Range()), nil
	}

	if args.depth > maxPartialEvalDepth {
		return RangeBytes(args.SrcBytes, expr.Range()), PartialEvalDepthExceededError{MaxDepth: maxPartialEvalDepth}
	}

	args.depth++

	defer func() { args.depth-- }()

	// Fast path: an expression with no deferred root (dependency.*) is resolved in the stack file context.
	if IsPure(expr, args.Deferred) {
		val, diags := expr.Value(args.EvalCtx)
		// hclwrite.TokensForValue panics on unknown values; resolve only wholly-known values here.
		if !diags.HasErrors() && val.IsWhollyKnown() {
			return valueToHCLBytes(val), nil
		}

		// Whole-expression evaluation failed (e.g. a function that errors at generate time). Fall back to structural
		// partial evaluation so resolvable sub-parts (stack local.*/values.*) still render to literals and only the
		// unresolvable leaf stays verbatim, rather than emitting the whole expression verbatim and leaking a
		// stack-scoped reference into the generated unit.
	}

	return partialEvalByType(expr, args)
}

// partialEvalByType dispatches to type-specific handlers for mixed expressions; unhandled types fall through to verbatim source bytes.
func partialEvalByType(expr hclsyntax.Expression, args *EvalArgs) ([]byte, error) {
	switch e := expr.(type) {
	case *hclsyntax.LiteralValueExpr:
		return valueToHCLBytes(e.Val), nil
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
	case *hclsyntax.BinaryOpExpr:
		return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.LHS, e.RHS})
	case *hclsyntax.UnaryOpExpr:
		return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.Val})
	case *hclsyntax.IndexExpr:
		return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.Collection, e.Key})
	case *hclsyntax.RelativeTraversalExpr:
		return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.Source})
	// For defers the loop variables so a stack local in the body resolves while the loop var stays verbatim; the collection has no loop vars so the shared deferred set is safe.
	case *hclsyntax.ForExpr:
		saved := args.Deferred
		args.Deferred = maps.Clone(saved)
		args.Deferred[e.KeyVar] = true
		args.Deferred[e.ValVar] = true

		defer func() { args.Deferred = saved }()

		return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.CollExpr, e.KeyExpr, e.ValExpr, e.CondExpr})
	// Splat renders only the source; its body runs against the anonymous iterator which cannot be deferred by name, so it stays verbatim.
	case *hclsyntax.SplatExpr:
		return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.Source})
	}

	// Any remaining type emits verbatim source bytes; the generated HCL keeps valid original text evaluated at runtime.
	return RangeBytes(args.SrcBytes, expr.Range()), nil
}

func partialEvalTraversal(e *hclsyntax.ScopeTraversalExpr, args *EvalArgs) ([]byte, error) {
	if args.Deferred[e.Traversal.RootName()] {
		return RangeBytes(args.SrcBytes, e.Range()), nil
	}

	val, diags := e.Value(args.EvalCtx)
	if !diags.HasErrors() && val.IsWhollyKnown() {
		return valueToHCLBytes(val), nil
	}

	return RangeBytes(args.SrcBytes, e.Range()), PartialEvalUnresolvedError{Reason: "traversal value is null or unknown at generation time", Err: diags}
}

// partialEvalChildren rebuilds parent source bytes with each child replaced by its PartialEval output; gaps stay verbatim.
func partialEvalChildren(args *EvalArgs, parentRange hcl.Range, children []hclsyntax.Expression) ([]byte, error) {
	if len(children) == 0 {
		return RangeBytes(args.SrcBytes, parentRange), nil
	}

	src := args.SrcBytes
	out := make([]byte, 0, parentRange.End.Byte-parentRange.Start.Byte)
	cursor := parentRange.Start.Byte

	var firstErr error

	for _, child := range children {
		if child == nil {
			continue
		}

		cr := child.Range()

		out = append(out, src[cursor:cr.Start.Byte]...)

		childBytes, err := PartialEval(child, args)
		out = append(out, childBytes...)
		cursor = cr.End.Byte

		if firstErr == nil && err != nil {
			firstErr = err
		}
	}

	out = append(out, src[cursor:parentRange.End.Byte]...)

	return out, firstErr
}

func partialEvalConditional(e *hclsyntax.ConditionalExpr, args *EvalArgs) ([]byte, error) {
	if !IsPure(e.Condition, args.Deferred) {
		return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.Condition, e.TrueResult, e.FalseResult})
	}

	condVal, diags := e.Condition.Value(args.EvalCtx)
	if diags.HasErrors() {
		return RangeBytes(args.SrcBytes, e.Range()), nil
	}

	boolVal, err := convert.Convert(condVal, cty.Bool)
	// Null/unknown condition: emit source bytes and a typed error for strict callers.
	if err != nil || boolVal.IsNull() || !boolVal.IsKnown() {
		return RangeBytes(args.SrcBytes, e.Range()), PartialEvalUnresolvedError{Reason: "conditional condition is null or unknown"}
	}

	if boolVal.True() {
		return PartialEval(e.TrueResult, args)
	}

	return PartialEval(e.FalseResult, args)
}

// partialEvalFunctionCall substitutes resolvable arg literals; unresolved-arg errors are absorbed so try/coalesce/defaults semantics still work (matches legacy pkg/config behavior).
func partialEvalFunctionCall(e *hclsyntax.FunctionCallExpr, args *EvalArgs) ([]byte, error) {
	children := make([]hclsyntax.Expression, len(e.Args))
	copy(children, e.Args)

	result, err := partialEvalChildren(args, e.Range(), children)
	if err == nil || isPartialEvalUnresolved(err) {
		return result, nil
	}

	return result, err
}

func isPartialEvalUnresolved(err error) bool {
	var unresolvedErr PartialEvalUnresolvedError

	return errors.As(err, &unresolvedErr)
}

func partialEvalParens(e *hclsyntax.ParenthesesExpr, args *EvalArgs) ([]byte, error) {
	// Pure inner expression - parens are redundant around a single expression; emit the inner directly.
	if IsPure(e.Expression, args.Deferred) {
		return PartialEval(e.Expression, args)
	}

	return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.Expression})
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

func partialEvalTemplate(e *hclsyntax.TemplateExpr, args *EvalArgs) ([]byte, error) {
	var (
		buf      bytes.Buffer
		firstErr error
	)

	buf.WriteByte('"')

	for _, part := range e.Parts {
		if lit, ok := part.(*hclsyntax.LiteralValueExpr); ok {
			buf.Write(HCLStringContent(lit.Val.AsString()))

			continue
		}

		if IsPure(part, args.Deferred) {
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

		// Deferred or eval failed: emit as interpolation.
		buf.WriteString("${")

		partBytes, err := PartialEval(part, args)
		buf.Write(partBytes)
		buf.WriteByte('}')

		if firstErr == nil && err != nil {
			firstErr = err
		}
	}

	buf.WriteByte('"')

	return buf.Bytes(), firstErr
}

// HCLStringContent returns the inner content of an HCL-escaped string
// (without surrounding quotes). Uses hclwrite.TokensForValue for correct
// escaping of all HCL special characters.
func HCLStringContent(s string) []byte {
	raw := hclwrite.TokensForValue(cty.StringVal(s)).Bytes()

	// TokensForValue produces `"escaped content"`: strip surrounding quotes.
	return bytes.TrimPrefix(bytes.TrimSuffix(raw, []byte{'"'}), []byte{'"'})
}

func partialEvalObject(e *hclsyntax.ObjectConsExpr, args *EvalArgs) ([]byte, error) {
	// Stitch only the value expressions; keys + `=` + `,` + braces stay verbatim from the source.
	children := make([]hclsyntax.Expression, len(e.Items))
	for i, item := range e.Items {
		children[i] = item.ValueExpr
	}

	return partialEvalChildren(args, e.Range(), children)
}

func partialEvalTuple(e *hclsyntax.TupleConsExpr, args *EvalArgs) ([]byte, error) {
	children := make([]hclsyntax.Expression, len(e.Exprs))
	copy(children, e.Exprs)

	return partialEvalChildren(args, e.Range(), children)
}

// valueToHCLBytes converts a cty.Value to HCL source text bytes.
func valueToHCLBytes(val cty.Value) []byte {
	tokens := hclwrite.TokensForValue(val)

	return tokens.Bytes()
}
