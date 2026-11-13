package config

import (
	"cmp"
	"errors"
	"math"
	"math/big"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

const maxBlockNesting = 32

// DependencyOutputReference is a reference to a dependency output in a unit, with the range of the expression.
type DependencyOutputReference struct {
	Instance   hclparse.InstanceKey
	Dependency string
	Output     string
	Range      hcl.Range
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

	refs, err := bodyDependencyOutputReferences(file.ConfigPath, body, walk, 0)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(refs, func(a, b DependencyOutputReference) int {
		return cmp.Compare(a.Range.Start.Byte, b.Range.Start.Byte)
	})

	return refs, nil
}

type referenceWalk struct {
	expanded map[string]struct{}
	replaced map[string]struct{}
}

// bodyDependencyOutputReferences returns the references in the attributes and nested blocks of body.
func bodyDependencyOutputReferences(
	configPath string,
	body *hclsyntax.Body,
	walk referenceWalk,
	depth int,
) ([]DependencyOutputReference, error) {
	if depth > maxBlockNesting {
		return nil, BlockNestingExceededError{ConfigPath: configPath, Limit: maxBlockNesting}
	}

	var refs []DependencyOutputReference

	for _, attr := range body.Attributes {
		for _, traversal := range attr.Expr.Variables() {
			if ref, ok := dependencyOutputReference(traversal, walk.expanded); ok {
				refs = append(refs, ref)
			}
		}
	}

	errs := make([]error, 0, len(body.Blocks))

	for _, block := range body.Blocks {
		if depth == 0 && replacedDependencyBlock(block, walk.replaced) {
			continue
		}

		nested, err := bodyDependencyOutputReferences(configPath, block.Body, walk, depth+1)
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

type namedStep struct {
	name string
	rest hcl.Traversal
}

type instanceKeyStep struct {
	key  hclparse.InstanceKey
	rest hcl.Traversal
}

// dependencyOutputReference reads traversal as dependency.<name>.outputs.<output>, with an instance key when expanded.
func dependencyOutputReference(
	traversal hcl.Traversal,
	expanded map[string]struct{},
) (DependencyOutputReference, bool) {
	if traversal.RootName() != MetadataDependency {
		return DependencyOutputReference{}, false
	}

	dependency, ok := keyStep(traversal[1:])
	if !ok {
		return DependencyOutputReference{}, false
	}

	instance, ok := instanceStep(dependency.rest, expanded, dependency.name)
	if !ok {
		return DependencyOutputReference{}, false
	}

	outputs, ok := keyStep(instance.rest)
	if !ok || outputs.name != "outputs" {
		return DependencyOutputReference{}, false
	}

	output, ok := keyStep(outputs.rest)
	if !ok {
		return DependencyOutputReference{}, false
	}

	return DependencyOutputReference{
		Instance:   instance.key,
		Dependency: dependency.name,
		Output:     output.name,
		Range:      traversal.SourceRange(),
	}, true
}

// instanceStep returns the literal instance key after the dependency named name, none unless expanded lists name.
func instanceStep(steps hcl.Traversal, expanded map[string]struct{}, name string) (instanceKeyStep, bool) {
	if _, isExpanded := expanded[name]; !isExpanded {
		return instanceKeyStep{rest: steps}, true
	}

	if len(steps) == 0 {
		return instanceKeyStep{}, false
	}

	switch step := steps[0].(type) {
	case hcl.TraverseAttr:
		return instanceKeyStep{key: hclparse.InstanceKey{EachKey: new(step.Name)}, rest: steps[1:]}, true
	case hcl.TraverseIndex:
		key, ok := literalInstanceKey(step.Key)
		if !ok {
			return instanceKeyStep{}, false
		}

		return instanceKeyStep{key: key, rest: steps[1:]}, true
	default:
		return instanceKeyStep{}, false
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

// attributeStep returns the attribute step at the head of steps.
func attributeStep(steps hcl.Traversal) (namedStep, bool) {
	if len(steps) == 0 {
		return namedStep{}, false
	}

	attr, ok := steps[0].(hcl.TraverseAttr)
	if !ok {
		return namedStep{}, false
	}

	return namedStep{name: attr.Name, rest: steps[1:]}, true
}

// keyStep returns the step at the head of steps that names a key: an attribute or a known string index.
func keyStep(steps hcl.Traversal) (namedStep, bool) {
	if step, ok := attributeStep(steps); ok {
		return step, true
	}

	if len(steps) == 0 {
		return namedStep{}, false
	}

	index, ok := steps[0].(hcl.TraverseIndex)
	if !ok || index.Key.IsNull() || !index.Key.IsKnown() || !index.Key.Type().Equals(cty.String) {
		return namedStep{}, false
	}

	return namedStep{name: index.Key.AsString(), rest: steps[1:]}, true
}

// expandedLabels returns the labels of the dependency blocks that declare an expansion.
func (deps Dependencies) expandedLabels() map[string]struct{} {
	expanded := make(map[string]struct{}, len(deps))

	for i := range deps {
		if deps[i].Expansion != nil {
			expanded[deps[i].Name] = struct{}{}
		}
	}

	return expanded
}
