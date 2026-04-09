package hclparse

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

const (
	// StackDir is the directory name where generated stack components are placed.
	StackDir = ".terragrunt-stack"

	// HCL block type and attribute names.
	blockDependency = "dependency"
	attrConfigPath  = "config_path"

	// HCL variable root names used in eval context.
	varLocal      = "local"
	varValues     = "values"
	varUnit       = "unit"
	varStack      = "stack"
	varDependency = blockDependency
)

// ParseStackFileInput holds the input for ParseStackFile.
type ParseStackFileInput struct {
	Values   *cty.Value
	Filename string
	StackDir string
	Src      []byte
}

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
func ParseStackFile(input *ParseStackFileInput) (*ParseResult, error) {
	file, diags := hclsyntax.ParseConfig(input.Src, input.Filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, diags
	}

	// Pass 1: decode unit/stack blocks. Autoinclude body captured as remain.
	stackFile := &StackFileHCL{}

	diags = gohcl.DecodeBody(file.Body, nil, stackFile)
	if diags.HasErrors() {
		return nil, diags
	}

	// Process includes — merge included units/stacks.
	if err := processStackIncludes(stackFile, input.StackDir); err != nil {
		return nil, err
	}

	// Build component refs with absolute paths for the eval context.
	// StackDir is the directory containing the terragrunt.stack.hcl file.
	// Generated units go to StackDir/.terragrunt-stack/{unit.path}.
	stackTargetDir := filepath.Join(input.StackDir, StackDir)

	unitRefs := buildRefsWithAbsPath(stackTargetDir, stackFile.Units)
	stackRefs := buildStackRefsWithAbsPath(input.StackDir, stackTargetDir, stackFile.Stacks)

	// Pass 2: resolve autoinclude blocks using the eval context.
	evalCtx := BuildAutoIncludeEvalContext(unitRefs, stackRefs)

	// Add values to context if provided.
	if input.Values != nil {
		evalCtx.Variables[varValues] = *input.Values
	}

	// Evaluate locals block iteratively.
	if stackFile.Locals != nil {
		evaluateLocals(stackFile.Locals.Remain, evalCtx)
	}

	autoIncludes, err := resolveAutoIncludes(stackFile, evalCtx)
	if err != nil {
		return nil, err
	}

	return &ParseResult{
		Units:        stackFile.Units,
		Stacks:       stackFile.Stacks,
		AutoIncludes: autoIncludes,
	}, nil
}

// evaluateLocals iteratively evaluates attributes from a locals block body.
// On each pass, attributes that can be evaluated with the current context are
// resolved and added to the `local` variable. Iteration continues until no
// progress is made or the maximum iteration count is reached.
func evaluateLocals(body hcl.Body, evalCtx *hcl.EvalContext) {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return
	}

	attrs := syntaxBody.Attributes
	evaluated := make(map[string]cty.Value)

	const maxIter = 100

	for i := 0; i < maxIter; i++ {
		progress := false

		for name, attr := range attrs {
			if _, done := evaluated[name]; done {
				continue
			}

			val, diags := attr.Expr.Value(evalCtx)
			if !diags.HasErrors() {
				evaluated[name] = val
				progress = true
			}
		}

		if !progress {
			break
		}

		evalCtx.Variables[varLocal] = cty.ObjectVal(evaluated)
	}
}

// resolveAutoIncludes resolves autoinclude blocks for all units and stacks in the stack file.
func resolveAutoIncludes(stackFile *StackFileHCL, evalCtx *hcl.EvalContext) (map[string]*AutoIncludeResolved, error) {
	autoIncludes := make(map[string]*AutoIncludeResolved)

	for _, unit := range stackFile.Units {
		if unit.AutoInclude == nil {
			continue
		}

		resolved, err := resolveOneAutoInclude(unit.AutoInclude, evalCtx)
		if err != nil {
			return nil, err
		}

		if resolved != nil {
			autoIncludes[unit.Name] = resolved
		}
	}

	for _, stack := range stackFile.Stacks {
		if stack.AutoInclude == nil {
			continue
		}

		resolved, err := resolveOneAutoInclude(stack.AutoInclude, evalCtx)
		if err != nil {
			return nil, err
		}

		if resolved != nil {
			autoIncludes[stack.Name] = resolved
		}
	}

	return autoIncludes, nil
}

// resolveOneAutoInclude resolves a single autoinclude block and attaches the eval context.
func resolveOneAutoInclude(autoInclude *AutoIncludeHCL, evalCtx *hcl.EvalContext) (*AutoIncludeResolved, error) {
	resolved, diags := autoInclude.Resolve(evalCtx)
	if diags.HasErrors() {
		return nil, diags
	}

	if resolved != nil {
		resolved.EvalCtx = evalCtx
	}

	return resolved, nil
}

// processStackIncludes resolves include blocks by parsing the included files
// and merging their unit/stack blocks into the main stack file.
func processStackIncludes(stackFile *StackFileHCL, stackDir string) error {
	for _, inc := range stackFile.Includes {
		includePath := inc.Path
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(stackDir, includePath)
		}

		data, err := os.ReadFile(includePath)
		if err != nil {
			return fmt.Errorf("failed to read include %q: %w", inc.Name, err)
		}

		incFile, diags := hclsyntax.ParseConfig(data, includePath, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return fmt.Errorf("failed to parse include %q: %s", inc.Name, diags.Error())
		}

		included := &StackFileHCL{}
		if decodeDiags := gohcl.DecodeBody(incFile.Body, nil, included); decodeDiags.HasErrors() {
			return fmt.Errorf("failed to decode include %q: %s", inc.Name, decodeDiags.Error())
		}

		stackFile.Units = append(stackFile.Units, included.Units...)
		stackFile.Stacks = append(stackFile.Stacks, included.Stacks...)
	}

	return nil
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
