package hclparse

import (
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

const (
	// StackDir is the directory name where generated stack components are placed.
	StackDir = ".terragrunt-stack"
)

// ParseResult holds the output of a two-pass parse of a terragrunt.stack.hcl file.
type ParseResult struct {
	// AutoIncludes maps component name → resolved autoinclude (only for units/stacks
	// that had an autoinclude block). Dependencies have config_path resolved.
	AutoIncludes map[string]*AutoIncludeResolved
	// Units from the first-pass parse (name, source, path, values decoded).
	Units []*UnitBlockHCL
	// Stacks from the first-pass parse.
	Stacks []*StackBlockHCL
}

// ParseStackFile performs a two-pass parse of a terragrunt.stack.hcl file.
//
// Pass 1: Parse unit/stack blocks to extract names, sources, and paths.
// The autoinclude body is captured as hcl.Body via remain (not evaluated).
//
// Between passes: Build eval context with unit.<name>.path and stack.<name>.path
// variables. Paths are resolved to absolute paths under .terragrunt-stack/.
//
// Pass 2: For each unit/stack with an autoinclude block, resolve the autoinclude
// body using the eval context. dependency.config_path is evaluated (references
// unit.*.path), while inputs are left unevaluated (contain dependency.*.outputs.*).
func ParseStackFile(src []byte, filename string, stackDir string) (*ParseResult, error) {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, diags
	}

	// Pass 1: decode unit/stack blocks. Autoinclude body captured as remain.
	stackFile := &StackFileHCL{}

	diags = gohcl.DecodeBody(file.Body, nil, stackFile)
	if diags.HasErrors() {
		return nil, diags
	}

	// Build component refs with absolute paths for the eval context.
	// stackDir is the directory containing the terragrunt.stack.hcl file.
	// Generated units go to stackDir/.terragrunt-stack/{unit.path}.
	stackTargetDir := filepath.Join(stackDir, StackDir)

	unitRefs := buildRefsWithAbsPath(stackTargetDir, stackFile.Units)
	stackRefs := buildStackRefsWithAbsPath(stackDir, stackTargetDir, stackFile.Stacks)

	// Pass 2: resolve autoinclude blocks using the eval context.
	evalCtx := BuildAutoIncludeEvalContext(unitRefs, stackRefs)
	autoIncludes := make(map[string]*AutoIncludeResolved)

	for _, unit := range stackFile.Units {
		if unit.AutoInclude == nil {
			continue
		}

		resolved, resolveDiags := unit.AutoInclude.Resolve(evalCtx)
		if resolveDiags.HasErrors() {
			return nil, resolveDiags
		}

		if resolved != nil {
			autoIncludes[unit.Name] = resolved
		}
	}

	for _, stack := range stackFile.Stacks {
		if stack.AutoInclude == nil {
			continue
		}

		resolved, resolveDiags := stack.AutoInclude.Resolve(evalCtx)
		if resolveDiags.HasErrors() {
			return nil, resolveDiags
		}

		if resolved != nil {
			autoIncludes[stack.Name] = resolved
		}
	}

	return &ParseResult{
		Units:        stackFile.Units,
		Stacks:       stackFile.Stacks,
		AutoIncludes: autoIncludes,
	}, nil
}

// buildRefsWithAbsPath creates ComponentRef values with paths resolved
// to the absolute location under .terragrunt-stack/.
func buildRefsWithAbsPath(stackTargetDir string, units []*UnitBlockHCL) []ComponentRef {
	refs := make([]ComponentRef, 0, len(units))

	for _, u := range units {
		refs = append(refs, ComponentRef{
			Name: u.Name,
			Path: filepath.Join(stackTargetDir, u.Path),
		})
	}

	return refs
}

// buildStackRefsWithAbsPath creates ComponentRef values for stack blocks.
// It also attempts to parse each stack's source to discover child units,
// enabling stack.stack_name.unit_name.path references.
func buildStackRefsWithAbsPath(stackDir string, stackTargetDir string, stacks []*StackBlockHCL) []ComponentRef {
	refs := make([]ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		stackGenPath := filepath.Join(stackTargetDir, s.Path)

		ref := ComponentRef{
			Name: s.Name,
			Path: stackGenPath,
		}

		// Resolve the source to find nested units within this stack.
		// The source may be relative to the stack file's directory.
		sourceDir := s.Source
		if !filepath.IsAbs(sourceDir) {
			sourceDir = filepath.Join(stackDir, sourceDir)
		}

		ref.ChildRefs = DiscoverStackChildUnits(sourceDir, stackGenPath)

		refs = append(refs, ref)
	}

	return refs
}
