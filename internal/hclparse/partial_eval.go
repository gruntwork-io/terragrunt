package hclparse

import (
	"bytes"
	"errors"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/gruntwork-io/terragrunt/internal/hclhelper"
)

// deferredRoots lists variable root names that cannot be evaluated at generation time.
// dependency.* is the sole deferred root; it resolves inside the generated unit at run time.
// Every other namespace, including values.* (the stack file's values), local.*, unit.*, and
// stack.*, resolves at generate time in the terragrunt.stack.hcl context.
// This map must not be modified after package initialization.
var deferredRoots = map[string]struct{}{
	varDependency: {},
}

// expandedDependencyDeferredRoots extends deferredRoots with the iteration roots of an expanded
// dependency block, which the generated unit binds when it expands the block again.
// This map must not be modified after package initialization.
var expandedDependencyDeferredRoots = map[string]struct{}{
	varDependency: {},
	varEach:       {},
	varCount:      {},
}

// defaultMaxPartialEvalDepth bounds recursion for pathological deeply-nested expressions; past this, fall back to source bytes.
const defaultMaxPartialEvalDepth = 10000

// EvalArgs bundles the shared arguments for partial evaluation functions.
type EvalArgs struct {
	EvalCtx        *hcl.EvalContext
	Deferred       map[string]struct{}
	iterators      map[string]struct{}
	includeIndexes map[*hclsyntax.IndexExpr]bool
	SrcBytes       []byte
	MaxDepth       int
	depth          int
}

// maxDepth returns the recursion bound to enforce, falling back to defaultMaxPartialEvalDepth when the caller leaves MaxDepth unset.
func (args *EvalArgs) maxDepth() int {
	if args.MaxDepth > 0 {
		return args.MaxDepth
	}

	return defaultMaxPartialEvalDepth
}

// PartialEval walks an hclsyntax.Expression tree and returns HCL source text; pure parts evaluate to literals, deferred parts stay verbatim, error signals pathological inputs.
func PartialEval(expr hclsyntax.Expression, args *EvalArgs) ([]byte, error) {
	if args.EvalCtx == nil {
		return RangeBytes(args.SrcBytes, expr.Range()), nil
	}

	maxDepth := args.maxDepth()
	if args.depth > maxDepth {
		return RangeBytes(
			args.SrcBytes,
			expr.Range(),
		), PartialEvalDepthExceededError{
			MaxDepth: maxDepth,
		}
	}

	args.depth++

	defer func() { args.depth-- }()

	// Fast path: an expression with no deferred root (dependency.*) is resolved in the stack file context.
	if args.isPure(expr) {
		val, diags := expr.Value(args.EvalCtx)
		// hclwrite.TokensForValue panics on unknown values; resolve only wholly-known values here.
		if !diags.HasErrors() && val.IsWhollyKnown() && valueRendersAsHCLLiteral(val, 0) {
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
	// Reached only for an expression key (see objectKeyIsExpression) that defers or failed whole-key evaluation.
	case *hclsyntax.ObjectConsKeyExpr:
		return partialEvalObjectKey(e, args)
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
		return partialEvalIndex(e, args)
	case *hclsyntax.RelativeTraversalExpr:
		return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.Source})
	case *hclsyntax.ForExpr:
		return partialEvalFor(e, args)
	// Splat renders only the source; its body runs against the anonymous iterator which cannot be deferred by name, so it stays verbatim.
	case *hclsyntax.SplatExpr:
		return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.Source})
	}

	// Any remaining type emits verbatim source bytes; the generated HCL keeps valid original text evaluated at runtime.
	return RangeBytes(args.SrcBytes, expr.Range()), nil
}

// partialEvalFor renders the collection in the outer scope and the rest with the iterators kept verbatim.
func partialEvalFor(e *hclsyntax.ForExpr, args *EvalArgs) ([]byte, error) {
	collEnd := e.CollExpr.Range().End

	prefix, prefixErr := partialEvalChildren(
		args,
		hcl.Range{Start: e.Range().Start, End: collEnd},
		[]hclsyntax.Expression{e.CollExpr},
	)

	deferred, iterators := args.Deferred, args.iterators
	args.Deferred = deferredWith(deferred, []string{e.KeyVar, e.ValVar})
	args.iterators = deferredWith(iterators, []string{e.KeyVar, e.ValVar})

	defer func() { args.Deferred, args.iterators = deferred, iterators }()

	suffix, suffixErr := partialEvalChildren(
		args,
		hcl.Range{Start: collEnd, End: e.Range().End},
		[]hclsyntax.Expression{e.KeyExpr, e.ValExpr, e.CondExpr},
	)

	if prefixErr != nil {
		return append(prefix, suffix...), prefixErr
	}

	return append(prefix, suffix...), suffixErr
}

func partialEvalTraversal(e *hclsyntax.ScopeTraversalExpr, args *EvalArgs) ([]byte, error) {
	if args.defers(e.Traversal) {
		return RangeBytes(args.SrcBytes, e.Range()), nil
	}

	val, diags := e.Value(args.EvalCtx)
	// A non-finite number (e.g. a local resolving to 1/0 -> Inf) renders as an invalid bare "Inf", so it is treated as
	// unresolved and kept verbatim, mirroring the whole-expression guard in PartialEval.
	if !diags.HasErrors() && val.IsWhollyKnown() && valueRendersAsHCLLiteral(val, 0) {
		return valueToHCLBytes(val), nil
	}

	return RangeBytes(
		args.SrcBytes,
		e.Range(),
	), PartialEvalUnresolvedError{
		Reason: "traversal value is null, unknown, or non-finite at generation time",
		Err:    diags,
	}
}

// partialEvalChildren rebuilds parent source bytes with each child replaced by its PartialEval output; gaps stay verbatim.
func partialEvalChildren(
	args *EvalArgs,
	parentRange hcl.Range,
	children []hclsyntax.Expression,
) ([]byte, error) {
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
	if !args.isPure(e.Condition) {
		return partialEvalChildren(
			args,
			e.Range(),
			[]hclsyntax.Expression{e.Condition, e.TrueResult, e.FalseResult},
		)
	}

	condVal, diags := e.Condition.Value(args.EvalCtx)
	if diags.HasErrors() {
		return RangeBytes(args.SrcBytes, e.Range()), nil
	}

	boolVal, err := convert.Convert(condVal, cty.Bool)
	// Null/unknown condition: emit source bytes and a typed error for strict callers.
	if err != nil || boolVal.IsNull() || !boolVal.IsKnown() {
		return RangeBytes(
			args.SrcBytes,
			e.Range(),
		), PartialEvalUnresolvedError{
			Reason: "conditional condition is null or unknown",
		}
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
	if args.isPure(e.Expression) {
		return PartialEval(e.Expression, args)
	}

	return partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.Expression})
}

// IsPure reports whether expr reads no deferred root, where `path.include` is not a deferred read.
func IsPure(expr hclsyntax.Expression, deferred map[string]struct{}) bool {
	return (&EvalArgs{Deferred: deferred}).isPure(expr)
}

// isPure reports whether expr reads no traversal that args defers.
func (args *EvalArgs) isPure(expr hclsyntax.Expression) bool {
	wholePath := false

	for _, traversal := range expr.Variables() {
		if !args.defers(traversal) {
			continue
		}

		if !isWholePath(traversal) {
			return false
		}

		wholePath = true
	}

	return !wholePath || !args.readsWholePath(expr)
}

// isWholePath reports whether traversal names the `path` object with no attribute.
func isWholePath(traversal hcl.Traversal) bool {
	return len(traversal) == 1 && traversal.RootName() == varPath
}

// readsWholePath reports whether expr reads the `path` object other than through an index with the key "include".
func (args *EvalArgs) readsWholePath(expr hclsyntax.Expression) bool {
	if _, iterator := args.iterators[varPath]; iterator {
		return true
	}

	walker := &pathReadWalker{
		args:     args,
		bound:    map[string]int{},
		includes: map[hclsyntax.Expression]struct{}{},
	}
	diags := hclsyntax.Walk(expr, walker)

	return walker.whole || diags.HasErrors()
}

// pathReadWalker finds a read of the `path` object that is not an index with the key "include".
type pathReadWalker struct {
	args     *EvalArgs
	bound    map[string]int
	includes map[hclsyntax.Expression]struct{}
	whole    bool
}

// Enter records the names that node binds and the read of `path` that node makes.
func (w *pathReadWalker) Enter(node hclsyntax.Node) hcl.Diagnostics {
	switch n := node.(type) {
	case hclsyntax.ChildScope:
		for name := range n.LocalNames {
			w.bound[name]++
		}
	case *hclsyntax.IndexExpr:
		if w.args.indexesPathInclude(n, w.bound) {
			w.includes[n.Collection] = struct{}{}
		}
	case *hclsyntax.ScopeTraversalExpr:
		if _, include := w.includes[n]; !include && w.bound[varPath] == 0 && isWholePath(n.Traversal) {
			w.whole = true
		}
	}

	return nil
}

// Exit releases the names that node binds.
func (w *pathReadWalker) Exit(node hclsyntax.Node) hcl.Diagnostics {
	if scope, ok := node.(hclsyntax.ChildScope); ok {
		for name := range scope.LocalNames {
			w.bound[name]--
		}
	}

	return nil
}

// defers reports whether traversal reads a deferred root, other than `path.include`, or a deferred attribute.
func (args *EvalArgs) defers(traversal hcl.Traversal) bool {
	root := traversal.RootName()
	if _, deferred := args.Deferred[root]; !deferred {
		return args.defersAttribute(traversal)
	}

	_, iterator := args.iterators[root]

	return iterator || !readsPathInclude(traversal)
}

// defersAttribute reports whether traversal is a reference such as `unit.path` that args defers by root and attribute.
func (args *EvalArgs) defersAttribute(traversal hcl.Traversal) bool {
	name, named := FirstAttribute(traversal)
	if !named || len(traversal[1:]) != 1 {
		return false
	}

	_, deferred := args.Deferred[traversal.RootName()+"."+name]

	return deferred
}

// readsPathInclude reports whether traversal reads `path.include`.
func readsPathInclude(traversal hcl.Traversal) bool {
	name, named := FirstAttribute(traversal)

	return traversal.RootName() == varPath && named && name == pathAttrInclude
}

// FirstAttribute returns the attribute that the step after the root of traversal reads, and false when it names none.
func FirstAttribute(traversal hcl.Traversal) (string, bool) {
	if len(traversal) <= 1 {
		return "", false
	}

	switch step := traversal[1].(type) {
	case hcl.TraverseAttr:
		return step.Name, true
	case hcl.TraverseIndex:
		key := step.Key
		if key.Type() == cty.String && key.IsKnown() && !key.IsNull() {
			return key.AsString(), true
		}
	}

	return "", false
}

// partialEvalIndex renders an index by child; `path[key]` with the key "include" that has no value is an error.
func partialEvalIndex(e *hclsyntax.IndexExpr, args *EvalArgs) ([]byte, error) {
	index, err := partialEvalChildren(args, e.Range(), []hclsyntax.Expression{e.Collection, e.Key})
	if err != nil || !args.indexesPathInclude(e, nil) {
		return index, err
	}

	include := hcl.Traversal{hcl.TraverseRoot{Name: varPath}, hcl.TraverseAttr{Name: pathAttrInclude}}

	val, diags := include.TraverseAbs(args.EvalCtx)
	if !diags.HasErrors() && val.IsWhollyKnown() && valueRendersAsHCLLiteral(val, 0) {
		return valueToHCLBytes(val), nil
	}

	return index, PartialEvalUnresolvedError{
		Reason: "path.include is not available at generation time",
		Err:    diags,
	}
}

// indexesPathInclude reports whether e reads `path.include` through an index whose key reads no name in bound.
func (args *EvalArgs) indexesPathInclude(e *hclsyntax.IndexExpr, bound map[string]int) bool {
	collection, ok := e.Collection.(*hclsyntax.ScopeTraversalExpr)
	if !ok || !isWholePath(collection.Traversal) || bound[varPath] > 0 {
		return false
	}

	_, deferred := args.Deferred[varPath]
	_, iterator := args.iterators[varPath]

	if !deferred || iterator {
		return false
	}

	for _, traversal := range e.Key.Variables() {
		if bound[traversal.RootName()] > 0 {
			return false
		}
	}

	if include, known := args.includeIndexes[e]; known {
		return include
	}

	if args.includeIndexes == nil {
		args.includeIndexes = map[*hclsyntax.IndexExpr]bool{}
	}

	args.includeIndexes[e] = args.isPathIncludeKey(e.Key)

	return args.includeIndexes[e]
}

// isPathIncludeKey reports whether key reads no deferred traversal and has the value "include".
func (args *EvalArgs) isPathIncludeKey(key hclsyntax.Expression) bool {
	if !args.isPure(key) {
		return false
	}

	val, diags := key.Value(args.EvalCtx)
	if diags.HasErrors() {
		return false
	}

	val, _ = val.Unmark()

	return val.Type() == cty.String && val.IsKnown() && !val.IsNull() && val.AsString() == pathAttrInclude
}

func partialEvalTemplate(e *hclsyntax.TemplateExpr, args *EvalArgs) ([]byte, error) {
	var (
		buf      bytes.Buffer
		firstErr error
	)

	buf.WriteByte('"')

	for _, part := range e.Parts {
		if lit, ok := part.(*hclsyntax.LiteralValueExpr); ok && lit.Val.Type() == cty.String {
			buf.Write(HCLStringContent(lit.Val.AsString()))

			continue
		}

		if args.isPure(part) {
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

		// A %{ for }/%{ if } directive part spans its own markers and cannot be re-emitted inside ${...}, so it is emitted verbatim as a unit and resolves in the generated unit.
		if directiveBytes, isDirective := templateDirectiveSource(
			part,
			args.SrcBytes,
		); isDirective {
			buf.Write(directiveBytes)

			continue
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

// templateDirectiveSource returns the verbatim source of a %{ for }/%{ if } directive part, or nil and false for any other template part. hclsyntax has no directive node, so detection is per synthesized type: only those two types can ever take the verbatim path, every other part keeps its ${...} wrapping.
func templateDirectiveSource(part hclsyntax.Expression, src []byte) ([]byte, bool) {
	srcBytes := RangeBytes(src, part.Range())
	if len(srcBytes) == 0 {
		return nil, false
	}

	switch part.(type) {
	// A TemplateJoinExpr is synthesized only by a %{ for } directive.
	case *hclsyntax.TemplateJoinExpr:
		return srcBytes, true
	// A %{ if } directive desugars to a ConditionalExpr that, unlike a genuine ${cond ? a : b} interpolation, spans its own directive markers and so does not re-parse as a standalone expression.
	case *hclsyntax.ConditionalExpr:
		return srcBytes, !isExpressionSource(srcBytes)
	default:
		return nil, false
	}
}

// isExpressionSource reports whether src parses as a standalone HCL expression; an interpolation part does (its range is exactly the inner expression) while a directive part does not (its range spans the directive markers).
func isExpressionSource(src []byte) bool {
	_, diags := hclsyntax.ParseExpression(src, "", hcl.Pos{Line: 1, Column: 1})

	return !diags.HasErrors()
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
	// Stitch the value expressions plus any expression keys; a literal-name key stays verbatim from the source like the `=` + `,` + braces.
	children := make([]hclsyntax.Expression, 0, len(e.Items))

	for _, item := range e.Items {
		if objectKeyIsExpression(item.KeyExpr) {
			children = append(children, item.KeyExpr)
		}

		children = append(children, item.ValueExpr)
	}

	return partialEvalChildren(args, e.Range(), children)
}

// objectKeyIsExpression reports whether HCL evaluates an object key as an expression (interpolated template, function call, or parenthesized key); a naked identifier or keyword key is a literal string (mirrors ObjectConsKeyExpr's literal-name detection) and a naked multi-step traversal key keeps its source form so HCL's ambiguity error surfaces where the object is evaluated.
func objectKeyIsExpression(keyExpr hclsyntax.Expression) bool {
	key, ok := keyExpr.(*hclsyntax.ObjectConsKeyExpr)
	if !ok {
		return false
	}

	// A parenthesized key is always evaluated as an expression, even when it wraps a bare identifier.
	if key.ForceNonLiteral {
		return true
	}

	if hcl.ExprAsKeyword(key.Wrapped) != "" {
		return false
	}

	if _, naked := key.Wrapped.(*hclsyntax.ScopeTraversalExpr); naked {
		return false
	}

	return true
}

// partialEvalObjectKey renders an expression key that defers or failed whole-key evaluation; a single-interpolation key (TemplateWrapExpr) re-emits its "${...}" wrapper around the partially evaluated inner expression, because the naked inner expression changes meaning in key position (ambiguous or literal-string HCL).
func partialEvalObjectKey(e *hclsyntax.ObjectConsKeyExpr, args *EvalArgs) ([]byte, error) {
	wrap, ok := e.Wrapped.(*hclsyntax.TemplateWrapExpr)
	if !ok {
		return PartialEval(e.Wrapped, args)
	}

	inner, err := PartialEval(wrap.Wrapped, args)

	out := append([]byte(`"${`), inner...)
	out = append(out, '}', '"')

	return out, err
}

func partialEvalTuple(e *hclsyntax.TupleConsExpr, args *EvalArgs) ([]byte, error) {
	children := make([]hclsyntax.Expression, len(e.Exprs))
	copy(children, e.Exprs)

	return partialEvalChildren(args, e.Range(), children)
}

// valueToHCLBytes converts a cty.Value to HCL source text bytes.
func valueToHCLBytes(val cty.Value) []byte {
	tokens := hclhelper.TokensForValue(val)

	return tokens.Bytes()
}

// maxCtyRenderWalkDepth bounds the valueRendersAsHCLLiteral walk to prevent stack overflow on pathological values.
const maxCtyRenderWalkDepth = defaultMaxPartialEvalDepth

// valueRendersAsHCLLiteral reports whether val and its nested elements contain no non-finite number (which hclwrite emits as an invalid bare "Inf").
func valueRendersAsHCLLiteral(val cty.Value, depth int) bool {
	if depth > maxCtyRenderWalkDepth {
		return false
	}

	if val.IsNull() || !val.IsKnown() {
		return true
	}

	valType := val.Type()

	if valType == cty.Number {
		return !val.AsBigFloat().IsInf()
	}

	if valType.IsListType() || valType.IsSetType() || valType.IsTupleType() ||
		valType.IsMapType() ||
		valType.IsObjectType() {
		for it := val.ElementIterator(); it.Next(); {
			_, elem := it.Element()
			if !valueRendersAsHCLLiteral(elem, depth+1) {
				return false
			}
		}
	}

	return true
}
