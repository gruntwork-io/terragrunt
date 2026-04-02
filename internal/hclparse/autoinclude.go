package hclparse

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
)

// AutoIncludeHCL represents the first-phase parse of an autoinclude block.
// The entire body is captured as remain because it may contain references
// to unit.*.path / stack.*.path variables that are only available after
// the first parsing pass extracts all unit/stack names and paths.
//
// Example HCL:
//
//	autoinclude {
//	  dependency "vpc" {
//	    config_path = unit.vpc.path
//	  }
//	  inputs = {
//	    vpc_id = dependency.vpc.outputs.vpc_id
//	  }
//	}
type AutoIncludeHCL struct {
	Remain hcl.Body `hcl:",remain"`
}

// AutoIncludeResolved represents the second-phase resolved autoinclude content.
// Dependencies have their config_path evaluated, but inputs containing
// dependency.*.outputs.* references are preserved as raw HCL for generation
// into the terragrunt.autoinclude.hcl file.
type AutoIncludeResolved struct {
	// RawBody is the original HCL body, preserved for serialization
	// into terragrunt.autoinclude.hcl. This allows writing the content
	// with dependency.* references intact (not evaluated).
	RawBody      hcl.Body
	Dependencies []*AutoIncludeDependency
}

// AutoIncludeDependency represents a dependency block inside autoinclude.
// Only config_path is evaluated during resolution; mock_outputs and other
// fields are preserved as-is for the generated file.
type AutoIncludeDependency struct {
	Remain     hcl.Body `hcl:",remain"`
	Name       string   `hcl:",label"`
	ConfigPath string   `hcl:"config_path,attr"`
}

// autoIncludePartial is used for the second-phase partial decode of
// the autoinclude body. We extract dependency blocks (to resolve
// config_path) while capturing everything else in Remain.
type autoIncludePartial struct {
	Remain       hcl.Body                 `hcl:",remain"`
	Dependencies []*AutoIncludeDependency `hcl:"dependency,block"`
}

// Resolve evaluates the autoinclude body using the provided eval context,
// which should contain unit.* and stack.* variables for path resolution.
//
// The resolution is intentionally partial: dependency.config_path values
// are evaluated (they reference unit.*.path), but dependency.*.outputs.*
// references in inputs are NOT evaluated (they are runtime-only).
//
// Returns the resolved autoinclude with dependency config paths filled in,
// and the raw body preserved for file generation.
func (a *AutoIncludeHCL) Resolve(evalCtx *hcl.EvalContext) (*AutoIncludeResolved, hcl.Diagnostics) {
	if a == nil || a.Remain == nil {
		return nil, nil
	}

	partial := &autoIncludePartial{}

	diags := gohcl.DecodeBody(a.Remain, evalCtx, partial)
	if diags.HasErrors() {
		return nil, diags
	}

	return &AutoIncludeResolved{
		Dependencies: partial.Dependencies,
		RawBody:      a.Remain,
	}, nil
}

// BuildAutoIncludeEvalContext creates an HCL evaluation context with
// unit and stack path references for resolving autoinclude blocks.
//
// The context provides:
//   - unit.<name>.path - relative path of each unit in the stack
//   - unit.<name>.name - name label of each unit
//   - stack.<name>.path - relative path of each stack in the stack
//   - stack.<name>.name - name label of each stack
//
// Additional variables (locals, values) can be merged into the returned
// context by the caller.
func BuildAutoIncludeEvalContext(unitRefs, stackRefs []ComponentRef) *hcl.EvalContext {
	vars := map[string]cty.Value{
		"unit":  BuildComponentRefMap(unitRefs),
		"stack": BuildComponentRefMap(stackRefs),
	}

	return &hcl.EvalContext{
		Variables: vars,
	}
}
