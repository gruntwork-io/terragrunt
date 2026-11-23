package config

import (
	"cmp"
	"errors"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/gruntwork-io/terragrunt/internal/valuesblock"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// ValueReference is a read of values.<Name> in a config file.
type ValueReference struct {
	// Name is the value the reference reads. For values.a.b, it is a.
	Name string
	// Guard is the innermost call that returns a fallback in place of an error from the read. The last argument of try
	// is not guarded by that try.
	Guard Guard
	// Range is the source range of the whole traversal.
	Range hcl.Range
}

// TryDefault is a read of values.<Name> inside try() in the config of a generated component without a values block,
// where try() stands in for a default that a values block would declare.
type TryDefault struct {
	// Kind is the kind of the component.
	Kind valuesblock.Kind
	// Source is the source of the component, with any credentials redacted.
	Source string
	// Name is the value the config reads.
	Name string
}

// Guard is a function whose call returns a fallback in place of an error from a read of values inside it.
type Guard string

const (
	// GuardNone is the guard of a read that no try or can call encloses.
	GuardNone Guard = ""
	// GuardTry is the try function, which returns its first argument that evaluates without an error.
	GuardTry Guard = "try"
	// GuardCan is the can function, which returns whether its argument evaluates without an error.
	GuardCan Guard = "can"
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
		var guards map[int]Guard

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
func guardedReads(expr hclsyntax.Expression) (map[int]Guard, error) {
	g := &guardWalker{
		args:  map[hcl.Range]Guard{},
		reads: map[int]Guard{},
	}

	if diags := hclsyntax.Walk(expr, g); diags.HasErrors() {
		return nil, diags
	}

	return g.reads, nil
}

// guardWalker records the reads of values that a call to try or can encloses.
type guardWalker struct {
	// args maps the range of each guarded argument of a try or can call to its guard. Nodes cannot be the keys, since
	// a for expression walks a [hclsyntax.ChildScope], which holds a map and so panics as a map key.
	args map[hcl.Range]Guard
	// reads maps the start byte of each guarded read of values to its innermost guard.
	reads map[int]Guard
	// guards is the stack of guards that enclose the node being walked, innermost last.
	guards []Guard
}

// Enter pushes the guard of node when node is a guarded argument, marks the arguments node guards when it is a try or
// can call, and records node with the innermost guard when it is a guarded read of values.
func (g *guardWalker) Enter(node hclsyntax.Node) hcl.Diagnostics {
	if guard, ok := g.args[node.Range()]; ok {
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

// Exit pops the guard that [guardWalker.Enter] pushed for node.
func (g *guardWalker) Exit(node hclsyntax.Node) hcl.Diagnostics {
	if _, ok := g.args[node.Range()]; ok {
		g.guards = g.guards[:len(g.guards)-1]
	}

	return nil
}

// markArgs records the arguments of call that it guards: every argument of can, and every argument of try but the
// last, whose error try returns.
func (g *guardWalker) markArgs(call *hclsyntax.FunctionCallExpr) {
	switch Guard(call.Name) {
	case GuardNone:
	case GuardCan:
		for _, arg := range call.Args {
			g.args[arg.Range()] = GuardCan
		}
	case GuardTry:
		for _, arg := range call.Args[:max(len(call.Args)-1, 0)] {
			g.args[arg.Range()] = GuardTry
		}
	}
}
