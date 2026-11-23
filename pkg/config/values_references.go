package config

import (
	"cmp"
	"errors"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// ValueReference is a read of values.<Name> in a config file.
type ValueReference struct {
	// Name is the value the reference reads. For values.a.b, it is a.
	Name string
	// Guard is the innermost function, [FuncTry] or [FuncCan], whose call returns a fallback in place of an error from
	// the read. It is the empty string when no such call encloses the read. The last argument of try is not guarded by
	// that try.
	Guard string
	// Range is the source range of the whole traversal.
	Range hcl.Range
}

// The functions that guard a read of values.
const (
	// FuncTry is the try function, which returns its first argument that evaluates without an error.
	FuncTry = "try"
	// FuncCan is the can function, which returns whether its argument evaluates without an error.
	FuncCan = "can"
)

// ValueReferences returns the reads of values.<name> in file outside its values block, sorted by position. It returns
// none for a file of HCL JSON.
//
// A read whose first step is not an attribute or a literal string index, such as a bare values or
// values[local.key], names no value and is left out.
//
// Returns [BlockNestingExceededError] when the blocks of file nest too deep.
func ValueReferences(file *hclparse.File) ([]ValueReference, error) {
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, nil
	}

	refs, err := bodyValueReferences(file.ConfigPath, body, 0)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(refs, func(a, b ValueReference) int {
		return cmp.Compare(a.Range.Start.Byte, b.Range.Start.Byte)
	})

	return refs, nil
}

// bodyValueReferences returns the value references in the attributes and nested blocks of body.
func bodyValueReferences(configPath string, body *hclsyntax.Body, depth int) ([]ValueReference, error) {
	if depth > maxBlockNesting {
		return nil, BlockNestingExceededError{ConfigPath: configPath, Limit: maxBlockNesting}
	}

	var refs []ValueReference

	for _, attr := range body.Attributes {
		var guards map[int]string

		for _, traversal := range attr.Expr.Variables() {
			if traversal.RootName() != MetadataValues {
				continue
			}

			step, ok := keyStep(traversal[1:])
			if !ok {
				continue
			}

			if guards == nil {
				var err error

				guards, err = guardedReads(attr.Expr)
				if err != nil {
					return nil, err
				}
			}

			rng := traversal.SourceRange()
			refs = append(refs, ValueReference{Name: step.name, Guard: guards[rng.Start.Byte], Range: rng})
		}
	}

	errs := make([]error, 0, len(body.Blocks))

	for _, block := range body.Blocks {
		if depth == 0 && block.Type == MetadataValues {
			continue
		}

		nested, err := bodyValueReferences(configPath, block.Body, depth+1)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		refs = append(refs, nested...)
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return refs, nil
}

// guardedReads returns the guard of each read of values in expr that a call to try or can encloses, keyed by the start
// byte of the read.
func guardedReads(expr hclsyntax.Expression) (map[int]string, error) {
	g := &guardWalker{
		args:  map[hclsyntax.Node]string{},
		reads: map[int]string{},
	}

	if diags := hclsyntax.Walk(expr, g); diags.HasErrors() {
		return nil, diags
	}

	return g.reads, nil
}

// guardWalker records the reads of values that a call to try or can encloses.
type guardWalker struct {
	// args maps each guarded argument of a try or can call to the name of the function.
	args map[hclsyntax.Node]string
	// reads maps the start byte of each guarded read of values to its innermost guard.
	reads map[int]string
	// guards is the stack of guards that enclose the node being walked, innermost last.
	guards []string
}

func (g *guardWalker) Enter(node hclsyntax.Node) hcl.Diagnostics {
	if guard, ok := g.args[node]; ok {
		g.guards = append(g.guards, guard)
	}

	switch node := node.(type) {
	case *hclsyntax.FunctionCallExpr:
		g.markArgs(node)
	case *hclsyntax.ScopeTraversalExpr:
		if len(g.guards) > 0 && node.Traversal.RootName() == MetadataValues {
			g.reads[node.Traversal.SourceRange().Start.Byte] = g.guards[len(g.guards)-1]
		}
	}

	return nil
}

func (g *guardWalker) Exit(node hclsyntax.Node) hcl.Diagnostics {
	if _, ok := g.args[node]; ok {
		g.guards = g.guards[:len(g.guards)-1]
	}

	return nil
}

// markArgs records the arguments of call that it guards: every argument of can, and every argument of try but the
// last, whose error try returns.
func (g *guardWalker) markArgs(call *hclsyntax.FunctionCallExpr) {
	switch call.Name {
	case FuncCan:
		for _, arg := range call.Args {
			g.args[arg] = FuncCan
		}
	case FuncTry:
		for _, arg := range call.Args[:max(len(call.Args)-1, 0)] {
			g.args[arg] = FuncTry
		}
	}
}
