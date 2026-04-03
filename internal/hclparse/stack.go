package hclparse

import (
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
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
	// ChildRefs holds nested unit refs for stack components.
	// When a stack block references a source with a terragrunt.stack.hcl,
	// the child units within that stack are parsed and stored here.
	// This enables stack.stack_name.unit_name.path references.
	ChildRefs []ComponentRef
}

// BuildComponentRefMap creates a cty.Value map from a slice of ComponentRef.
// The resulting value is an object like:
//
//	{
//	  "unit_name": { "path": "../relative/path", "name": "unit_name" }
//	}
//
// For stack refs with children, it also includes nested unit refs:
//
//	{
//	  "stack_name": {
//	    "path": "/abs/path",
//	    "name": "stack_name",
//	    "unit_name": { "path": "/abs/path/to/unit", "name": "unit_name" }
//	  }
//	}
//
// This is injected into the HCL eval context as the `unit` or `stack` variable.
func BuildComponentRefMap(refs []ComponentRef) cty.Value {
	if len(refs) == 0 {
		return cty.EmptyObjectVal
	}

	refMap := make(map[string]cty.Value, len(refs))

	for _, ref := range refs {
		attrs := map[string]cty.Value{
			"path": cty.StringVal(ref.Path),
			"name": cty.StringVal(ref.Name),
		}

		// Add child unit refs for stacks (enables stack.stack_name.unit_name.path)
		for _, child := range ref.ChildRefs {
			attrs[child.Name] = cty.ObjectVal(map[string]cty.Value{
				"path": cty.StringVal(child.Path),
				"name": cty.StringVal(child.Name),
			})
		}

		refMap[ref.Name] = cty.ObjectVal(attrs)
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

// DiscoverStackChildUnits parses a stack's source directory to find the
// terragrunt.stack.hcl within it and extracts unit paths. This enables
// stack.stack_name.unit_name.path references in autoinclude blocks.
//
// stackSourceDir is the directory where the stack's source files live
// (or will be generated). stackGenDir is the absolute path where this
// stack's units will be generated (.terragrunt-stack/stack_path/).
func DiscoverStackChildUnits(stackSourceDir, stackGenDir string) []ComponentRef {
	stackFile := filepath.Join(stackSourceDir, "terragrunt.stack.hcl")

	data, err := os.ReadFile(stackFile)
	if err != nil {
		return nil
	}

	file, diags := hclsyntax.ParseConfig(data, stackFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil
	}

	parsed := &StackFileHCL{}
	if diags := gohcl.DecodeBody(file.Body, nil, parsed); diags.HasErrors() {
		return nil
	}

	childTargetDir := filepath.Join(stackGenDir, StackDir)
	refs := make([]ComponentRef, 0, len(parsed.Units))

	for _, u := range parsed.Units {
		unitPath := filepath.Join(childTargetDir, u.Path)

		// When no_dot_terragrunt_stack is set, the unit is placed directly
		// under the stack's generated directory, not under .terragrunt-stack/.
		if u.NoStack != nil && *u.NoStack {
			unitPath = filepath.Join(stackGenDir, u.Path)
		}

		refs = append(refs, ComponentRef{
			Name: u.Name,
			Path: unitPath,
		})
	}

	return refs
}
