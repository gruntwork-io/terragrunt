package hclparse

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// StackFileHCL represents the first-phase parse of a terragrunt.stack.hcl file.
// The autoinclude body inside each unit/stack block is captured as hcl.Body
// via remain, allowing deferred evaluation once unit/stack path variables
// are available.
type StackFileHCL struct {
	Locals *LocalsHCL       `hcl:"locals,block"`
	Stacks []*StackBlockHCL `hcl:"stack,block"`
	Units  []*UnitBlockHCL  `hcl:"unit,block"`
}

// UnitBlockHCL represents the first-phase parse of a unit block.
// Known attributes are decoded directly. The autoinclude block body
// is captured in Remain for second-phase evaluation.
type UnitBlockHCL struct {
	AutoInclude  *AutoIncludeHCL `hcl:"autoinclude,block"`
	NoStack      *bool           `hcl:"no_dot_terragrunt_stack,attr"`
	NoValidation *bool           `hcl:"no_validation,attr"`
	Values       *cty.Value      `hcl:"values,attr"`
	Name         string          `hcl:",label"`
	Source       string          `hcl:"source,attr"`
	Path         string          `hcl:"path,attr"`
}

// StackBlockHCL represents the first-phase parse of a stack block.
// Same remain pattern as UnitBlockHCL.
type StackBlockHCL struct {
	AutoInclude  *AutoIncludeHCL `hcl:"autoinclude,block"`
	NoStack      *bool           `hcl:"no_dot_terragrunt_stack,attr"`
	NoValidation *bool           `hcl:"no_validation,attr"`
	Values       *cty.Value      `hcl:"values,attr"`
	Name         string          `hcl:",label"`
	Source       string          `hcl:"source,attr"`
	Path         string          `hcl:"path,attr"`
}

// LocalsHCL captures the locals block body for iterative evaluation.
type LocalsHCL struct {
	Remain hcl.Body `hcl:",remain"`
}

// ComponentRef holds the path and name metadata for a unit or stack block,
// used to build the evaluation context for the second parsing phase.
type ComponentRef struct {
	Name string
	Path string
}

// BuildComponentRefMap creates a cty.Value map from a slice of ComponentRef.
// The resulting value is an object like:
//
//	{
//	  "unit_name": { "path": "../relative/path", "name": "unit_name" }
//	}
//
// This is injected into the HCL eval context as the `unit` or `stack` variable.
func BuildComponentRefMap(refs []ComponentRef) cty.Value {
	if len(refs) == 0 {
		return cty.EmptyObjectVal
	}

	refMap := make(map[string]cty.Value, len(refs))

	for _, ref := range refs {
		refMap[ref.Name] = cty.ObjectVal(map[string]cty.Value{
			"path": cty.StringVal(ref.Path),
			"name": cty.StringVal(ref.Name),
		})
	}

	return cty.ObjectVal(refMap)
}

// ExtractUnitRefs extracts ComponentRef values from parsed UnitBlockHCL slices.
func ExtractUnitRefs(units []*UnitBlockHCL) []ComponentRef {
	refs := make([]ComponentRef, 0, len(units))

	for _, u := range units {
		refs = append(refs, ComponentRef{
			Name: u.Name,
			Path: u.Path,
		})
	}

	return refs
}

// ExtractStackRefs extracts ComponentRef values from parsed StackBlockHCL slices.
func ExtractStackRefs(stacks []*StackBlockHCL) []ComponentRef {
	refs := make([]ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		refs = append(refs, ComponentRef{
			Name: s.Name,
			Path: s.Path,
		})
	}

	return refs
}
