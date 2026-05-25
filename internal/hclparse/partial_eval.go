package hclparse

import (
	"bytes"
	"errors"

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

	// Fast path: pure expression with no function calls, evaluate the whole thing (function calls are preserved because Terragrunt functions can have generation-time side effects).
	if IsPure(expr, args.Deferred) && !containsFunctionCall(expr) {
		val, diags := expr.Value(args.EvalCtx)
		// hclwrite.TokensForValue panics on unknown values; fall back to source bytes and surface a typed error.
		if !diags.HasErrors() && val.IsWhollyKnown() {
			return valueToHCLBytes(val), nil
		}

		return RangeBytes(args.SrcBytes, expr.Range()), PartialEvalUnresolvedError{Reason: "value is null or unknown at generation time", Err: diags}
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
	}

	// Unhandled types (ForExpr, SplatExpr, BinaryOpExpr, UnaryOpExpr, etc.) emit verbatim source bytes; the generated HCL contains valid original text evaluated at runtime.
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
	if !IsPure(e.Condition, args.Deferred) || containsFunctionCall(e.Condition) {
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

// containsFunctionCall reports whether expr contains any FunctionCallExpr anywhere in its AST.
//
// It gates PartialEval's fast path: an expression with no deferred refs would normally be
// evaluated eagerly to a literal, but if it contains a function call the call is preserved
// verbatim instead.
//
// This matters because Terragrunt functions (get_terragrunt_dir, find_in_parent_folders,
// path_relative_to_include, read_terragrunt_config, etc.) resolve directory context from where
// the eval runs:
//   - At autoinclude generation time, the context is the stack file's directory.
//   - At unit parse time, the context is the consumer unit's directory.
//
// Executing the function at generation time would bake the stack-file directory into the
// generated terragrunt.autoinclude.hcl; preserving the call leaves resolution to the unit
// parse, where the directory context is correct.
//
// hclsyntax.Walk traverses every node type (ForExpr, SplatExpr, BinaryOpExpr, ...) so nested
// function calls are detected regardless of the enclosing expression.
func containsFunctionCall(expr hclsyntax.Expression) bool {
	w := &functionCallWalker{}

	// Walk returns hcl.Diagnostics by signature; our walker's Enter/Exit return nil, so the result is always empty and intentionally discarded.
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
