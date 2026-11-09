package config

import (
	"cmp"
	"math"
	"math/big"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// maxBlockNesting bounds the block nesting the reference walk descends into.
const maxBlockNesting = 32

// DependencyOutputReference is a reference in a config file to a named output of a dependency.
type DependencyOutputReference struct {
	// Instance is the literal key of the expanded instance the reference reads, or the zero key for a whole dependency.
	Instance hclparse.InstanceKey
	// Dependency is the dependency block label.
	Dependency string
	// Output is the referenced output name.
	Output string
	// Range is where the reference is in the file.
	Range hcl.Range
}

// Address returns the address of the dependency the reference reads, as [Dependency.Address] gives it.
func (r *DependencyOutputReference) Address() string {
	return r.Instance.Address(r.Dependency)
}

// DependencyOutputReferences returns the references in file to dependency outputs, or [BlockNestingExceededError].
func DependencyOutputReferences(
	file *hclparse.File,
	dependencies Dependencies,
	replaced map[string]struct{},
) ([]DependencyOutputReference, error) {
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, nil
	}

	walk := referenceWalk{expanded: dependencies.expandedLabels(), replaced: replaced}

	refs, err := bodyDependencyOutputReferences(file.ConfigPath, body, walk, 0, nil)
	if err != nil {
		return nil, err
	}

	// sort by position
	slices.SortFunc(refs, func(a, b DependencyOutputReference) int {
		return cmp.Compare(a.Range.Start.Byte, b.Range.Start.Byte)
	})

	return refs, nil
}

// referenceWalk is what the reference walk knows about the dependency blocks of a file.
type referenceWalk struct {
	// expanded are the labels of the dependency blocks that declare an expansion.
	expanded map[string]struct{}
	// replaced are the labels of the dependency blocks a sibling autoinclude replaces whole.
	replaced map[string]struct{}
}

// bodyDependencyOutputReferences appends the references of body's attributes and nested blocks to refs.
func bodyDependencyOutputReferences(
	configPath string,
	body *hclsyntax.Body,
	walk referenceWalk,
	depth int,
	refs []DependencyOutputReference,
) ([]DependencyOutputReference, error) {
	if depth > maxBlockNesting {
		return nil, BlockNestingExceededError{ConfigPath: configPath, Limit: maxBlockNesting}
	}

	for _, attr := range body.Attributes {
		for _, traversal := range attr.Expr.Variables() {
			if ref, ok := dependencyOutputReference(traversal, walk.expanded); ok {
				refs = append(refs, ref)
			}
		}
	}

	for _, block := range body.Blocks {
		if depth == 0 && replacedDependencyBlock(block, walk.replaced) {
			continue
		}

		var err error

		refs, err = bodyDependencyOutputReferences(configPath, block.Body, walk, depth+1, refs)
		if err != nil {
			return nil, err
		}
	}

	return refs, nil
}

// replacedDependencyBlock reports whether block is an unexpanded dependency block labeled in replaced.
func replacedDependencyBlock(block *hclsyntax.Block, replaced map[string]struct{}) bool {
	if block.Type != MetadataDependency || len(block.Labels) != 1 {
		return false
	}

	if _, ok := replaced[block.Labels[0]]; !ok {
		return false
	}

	return !slices.ContainsFunc(block.Body.Blocks, func(inner *hclsyntax.Block) bool {
		return inner.Type == hclparse.ExpansionBlockName
	})
}

// OutputDeclarationRanges returns where each output block of the outputs block in file is declared, by name.
func OutputDeclarationRanges(file *hclparse.File) map[string]hcl.Range {
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	ranges := map[string]hcl.Range{}

	for _, block := range body.Blocks {
		if block.Type != MetadataOutputs {
			continue
		}

		for _, output := range block.Body.Blocks {
			if output.Type != MetadataOutput || len(output.Labels) != 1 {
				continue
			}

			if _, seen := ranges[output.Labels[0]]; !seen {
				ranges[output.Labels[0]] = output.DefRange()
			}
		}
	}

	return ranges
}

// dependencyOutputReference reads traversal as dependency.<name>.outputs.<output>, with an instance key when expanded.
func dependencyOutputReference(
	traversal hcl.Traversal,
	expanded map[string]struct{},
) (DependencyOutputReference, bool) {
	if traversal.RootName() != MetadataDependency {
		return DependencyOutputReference{}, false
	}

	name, rest, ok := keyStep(traversal[1:])
	if !ok {
		return DependencyOutputReference{}, false
	}

	var instance hclparse.InstanceKey

	if _, isExpanded := expanded[name]; isExpanded {
		instance, rest, ok = instanceStep(rest)
		if !ok {
			return DependencyOutputReference{}, false
		}
	}

	outputs, rest, ok := keyStep(rest)
	if !ok || outputs != "outputs" {
		return DependencyOutputReference{}, false
	}

	output, _, ok := keyStep(rest)
	if !ok {
		return DependencyOutputReference{}, false
	}

	return DependencyOutputReference{
		Instance:   instance,
		Dependency: name,
		Output:     output,
		Range:      traversal.SourceRange(),
	}, true
}

// instanceStep returns the instance key the first step of steps names, the steps after it, and whether it is literal.
func instanceStep(steps hcl.Traversal) (hclparse.InstanceKey, hcl.Traversal, bool) {
	if len(steps) == 0 {
		return hclparse.InstanceKey{}, nil, false
	}

	switch step := steps[0].(type) {
	case hcl.TraverseAttr:
		return hclparse.InstanceKey{EachKey: new(step.Name)}, steps[1:], true
	case hcl.TraverseIndex:
		key, ok := literalInstanceKey(step.Key)
		if !ok {
			return hclparse.InstanceKey{}, nil, false
		}

		return key, steps[1:], true
	default:
		return hclparse.InstanceKey{}, nil, false
	}
}

// literalInstanceKey returns the instance key a literal string or whole number index names, and whether it is one.
func literalInstanceKey(key cty.Value) (hclparse.InstanceKey, bool) {
	if key.IsNull() || !key.IsKnown() {
		return hclparse.InstanceKey{}, false
	}

	switch {
	case key.Type().Equals(cty.String):
		return hclparse.InstanceKey{EachKey: new(key.AsString())}, true
	case key.Type().Equals(cty.Number):
		whole, accuracy := key.AsBigFloat().Int64()
		if accuracy != big.Exact || whole < 0 || whole > math.MaxInt {
			return hclparse.InstanceKey{}, false
		}

		return hclparse.InstanceKey{CountIndex: new(int(whole))}, true
	default:
		return hclparse.InstanceKey{}, false
	}
}

// attributeStep returns the name of the attribute step at the head of steps and the steps after it.
func attributeStep(steps hcl.Traversal) (string, hcl.Traversal, bool) {
	if len(steps) == 0 {
		return "", nil, false
	}

	attr, ok := steps[0].(hcl.TraverseAttr)
	if !ok {
		return "", nil, false
	}

	return attr.Name, steps[1:], true
}

// keyStep returns the name the step at the head of steps addresses: an attribute name or a known string index.
func keyStep(steps hcl.Traversal) (string, hcl.Traversal, bool) {
	if name, rest, ok := attributeStep(steps); ok {
		return name, rest, true
	}

	if len(steps) == 0 {
		return "", nil, false
	}

	index, ok := steps[0].(hcl.TraverseIndex)
	if !ok || index.Key.IsNull() || !index.Key.IsKnown() || !index.Key.Type().Equals(cty.String) {
		return "", nil, false
	}

	return index.Key.AsString(), steps[1:], true
}

// expandedLabels returns the labels of the dependency blocks that declare an expansion.
func (deps Dependencies) expandedLabels() map[string]struct{} {
	expanded := map[string]struct{}{}

	for i := range deps {
		if deps[i].Expansion != nil {
			expanded[deps[i].Name] = struct{}{}
		}
	}

	return expanded
}
