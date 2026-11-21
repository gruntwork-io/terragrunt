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
	// Range is the source range of the whole traversal.
	Range hcl.Range
}

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
		for _, traversal := range attr.Expr.Variables() {
			if traversal.RootName() != MetadataValues {
				continue
			}

			step, ok := keyStep(traversal[1:])
			if !ok {
				continue
			}

			refs = append(refs, ValueReference{Name: step.name, Range: traversal.SourceRange()})
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
