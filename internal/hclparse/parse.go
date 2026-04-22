// Package hclparse provides two-phase HCL parsing for stack files with
// support for autoinclude blocks and deferred evaluation.
package hclparse

import (
	"errors"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
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
func ParseStackFile(fs vfs.FS, input *ParseStackFileInput) (*ParseResult, error) {
	if fs == nil {
		panic("hclparse.ParseStackFile: fs is nil; a vfs.FS is required to read included stack files (e.g. pass vfs.NewOSFS() for disk or vfs.NewMemMapFS() for tests)")
	}

	if input == nil {
		panic("hclparse.ParseStackFile: input is nil; caller must provide a *ParseStackFileInput with Src and StackDir set")
	}

	if input.StackDir == "" {
		panic("hclparse.ParseStackFile: input.StackDir is empty; StackDir is required to resolve relative include paths and compute generated unit directories")
	}

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

	// Process includes: merge included units/stacks.
	if err := processStackIncludes(fs, stackFile, input.StackDir); err != nil {
		return nil, err
	}

	// Build component refs with absolute paths for the eval context.
	// Default target is StackDir/.terragrunt-stack/{unit.path}, but units
	// with no_dot_terragrunt_stack go to StackDir/{unit.path} instead.
	stackTargetDir := filepath.Join(input.StackDir, StackDir)

	unitRefs := buildRefsWithAbsPath(stackTargetDir, stackFile.Units)
	stackRefs := buildStackRefsWithAbsPath(fs, input.StackDir, stackTargetDir, stackFile.Stacks, 0)

	// Pass 2: resolve autoinclude blocks using the eval context.
	evalCtx := BuildAutoIncludeEvalContext(unitRefs, stackRefs)

	// Add values to context if provided.
	if input.Values != nil {
		evalCtx.Variables[varValues] = *input.Values
	}

	// Evaluate locals block iteratively.
	if stackFile.Locals != nil {
		if err := evaluateLocals(stackFile.Locals.Remain, evalCtx); err != nil {
			return nil, err
		}
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
// Uses Variables() to pre-check whether each local's dependencies are satisfied
// before attempting evaluation. Shrinks the work set each pass. Returns an error
// if any locals cannot be evaluated (cycle or invalid reference).
func evaluateLocals(body hcl.Body, evalCtx *hcl.EvalContext) error {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		// Non-syntax bodies (e.g. from JSON configs) cannot be iteratively evaluated.
		return nil
	}

	remaining := make(map[string]*hclsyntax.Attribute, len(syntaxBody.Attributes))
	for name, attr := range syntaxBody.Attributes {
		remaining[name] = attr
	}

	evaluated := make(map[string]cty.Value, len(remaining))

	const maxLocalsIterations = 10000

	for i := 0; len(remaining) > 0 && i < maxLocalsIterations; i++ {
		progress, err := evaluateLocalsPass(remaining, evaluated, evalCtx)
		if err != nil {
			return err
		}

		if !progress {
			return localsEvalCycleError(remaining)
		}

		evalCtx.Variables[varLocal] = cty.ObjectVal(evaluated)
	}

	if len(remaining) > 0 {
		return LocalsMaxIterError{MaxIterations: maxLocalsIterations, Remaining: len(remaining)}
	}

	return nil
}

// evaluateLocalsPass attempts to evaluate all ready locals in a single pass.
// Returns true if at least one local was evaluated.
func evaluateLocalsPass(remaining map[string]*hclsyntax.Attribute, evaluated map[string]cty.Value, evalCtx *hcl.EvalContext) (bool, error) {
	progress := false

	for name, attr := range remaining {
		if !canEvalLocal(attr, evaluated) {
			continue
		}

		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return false, LocalEvalError{Name: name, Detail: diags.Error()}
		}

		evaluated[name] = val
		delete(remaining, name)

		progress = true
	}

	return progress, nil
}

// localsEvalCycleError builds an error listing the locals that could not be evaluated.
func localsEvalCycleError(remaining map[string]*hclsyntax.Attribute) error {
	names := make([]string, 0, len(remaining))
	for name := range remaining {
		names = append(names, name)
	}

	slices.Sort(names)

	return LocalsCycleError{Names: names}
}

// canEvalLocal checks whether all local.* dependencies of an attribute
// are already evaluated. Non-local references (unit, stack, values, etc.)
// are assumed available in the eval context.
func canEvalLocal(attr *hclsyntax.Attribute, evaluated map[string]cty.Value) bool {
	for _, traversal := range attr.Expr.Variables() {
		if traversal.RootName() != varLocal {
			continue
		}

		split := traversal.SimpleSplit()
		if len(split.Rel) == 0 {
			continue
		}

		step, ok := split.Rel[0].(hcl.TraverseAttr)
		if !ok {
			continue
		}

		if _, exists := evaluated[step.Name]; !exists {
			return false
		}
	}

	return true
}

// AutoIncludeKey returns the map key for an autoinclude entry, namespaced
// by component kind to prevent collisions between same-name units and stacks.
func AutoIncludeKey(kind, name string) string {
	return kind + ":" + name
}

// resolveAutoIncludes resolves autoinclude blocks for all units and stacks in the stack file.
// Keys are namespaced as "unit:name" and "stack:name" to prevent same-name collisions.
func resolveAutoIncludes(stackFile *StackFileHCL, evalCtx *hcl.EvalContext) (map[string]*AutoIncludeResolved, error) {
	autoIncludes := make(map[string]*AutoIncludeResolved)

	for _, unit := range stackFile.Units {
		if unit.AutoInclude == nil {
			continue
		}

		resolved, err := resolveAutoInclude(unit.AutoInclude, evalCtx)
		if err != nil {
			return nil, err
		}

		if resolved != nil {
			autoIncludes[AutoIncludeKey("unit", unit.Name)] = resolved
		}
	}

	for _, stack := range stackFile.Stacks {
		if stack.AutoInclude == nil {
			continue
		}

		resolved, err := resolveAutoInclude(stack.AutoInclude, evalCtx)
		if err != nil {
			return nil, err
		}

		if resolved != nil {
			autoIncludes[AutoIncludeKey("stack", stack.Name)] = resolved
		}
	}

	return autoIncludes, nil
}

// resolveAutoInclude resolves a single autoinclude block and attaches the eval context.
func resolveAutoInclude(autoInclude *AutoIncludeHCL, evalCtx *hcl.EvalContext) (*AutoIncludeResolved, error) {
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
func processStackIncludes(fs vfs.FS, stackFile *StackFileHCL, stackDir string) error {
	for _, inc := range stackFile.Includes {
		if err := mergeOneInclude(fs, stackFile, inc, stackDir); err != nil {
			return err
		}
	}

	if err := validateNoDuplicateUnits(stackFile.Units); err != nil {
		return err
	}

	return validateNoDuplicateStacks(stackFile.Stacks)
}

// mergeOneInclude reads and merges a single included stack file.
func mergeOneInclude(fs vfs.FS, stackFile *StackFileHCL, inc *StackIncludeHCL, stackDir string) error {
	includePath := inc.Path
	if !filepath.IsAbs(includePath) {
		includePath = filepath.Join(stackDir, includePath)
	}

	data, err := vfs.ReadFile(fs, includePath)
	if err != nil {
		return FileReadError{FilePath: inc.Path, Err: err}
	}

	incFile, diags := hclsyntax.ParseConfig(data, includePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return FileParseError{FilePath: inc.Path, Detail: diags.Error()}
	}

	included := &StackFileHCL{}
	if decodeDiags := gohcl.DecodeBody(incFile.Body, nil, included); decodeDiags.HasErrors() {
		return FileDecodeError{Name: inc.Name, Detail: decodeDiags.Error()}
	}

	if included.Locals != nil {
		return IncludeValidationError{IncludeName: inc.Name, Reason: "must not define locals"}
	}

	if len(included.Includes) > 0 {
		return IncludeValidationError{IncludeName: inc.Name, Reason: "must not define nested includes"}
	}

	stackFile.Units = append(stackFile.Units, included.Units...)
	stackFile.Stacks = append(stackFile.Stacks, included.Stacks...)

	return nil
}

// validateNoDuplicateUnits checks for duplicate unit names after include merge.
// Collects all duplicates and returns a single joined error.
func validateNoDuplicateUnits(units []*UnitBlockHCL) error {
	seen := make(map[string]struct{}, len(units))

	var errs []error

	for _, u := range units {
		if _, exists := seen[u.Name]; exists {
			errs = append(errs, DuplicateUnitNameError{Name: u.Name})

			continue
		}

		seen[u.Name] = struct{}{}
	}

	return errors.Join(errs...)
}

// validateNoDuplicateStacks checks for duplicate stack names after include merge.
// Collects all duplicates and returns a single joined error.
func validateNoDuplicateStacks(stacks []*StackBlockHCL) error {
	seen := make(map[string]struct{}, len(stacks))

	var errs []error

	for _, s := range stacks {
		if _, exists := seen[s.Name]; exists {
			errs = append(errs, DuplicateStackNameError{Name: s.Name})

			continue
		}

		seen[s.Name] = struct{}{}
	}

	return errors.Join(errs...)
}

// buildRefsWithAbsPath creates ComponentRef values with paths resolved
// to the absolute location under .terragrunt-stack/.
func buildRefsWithAbsPath(stackTargetDir string, units []*UnitBlockHCL) []ComponentRef {
	refs := make([]ComponentRef, 0, len(units))

	for _, u := range units {
		unitPath := filepath.Join(stackTargetDir, u.Path)

		if u.NoStack != nil && *u.NoStack {
			unitPath = filepath.Join(filepath.Dir(stackTargetDir), u.Path)
		}

		refs = append(refs, ComponentRef{
			Name: u.Name,
			Path: unitPath,
		})
	}

	return refs
}

// buildStackRefsWithAbsPath creates ComponentRef values for stack blocks.
// It also attempts to parse each stack's source to discover child units,
// enabling stack.stack_name.unit_name.path references.
// depth is threaded to prevent unbounded recursion in circular stacks.
func buildStackRefsWithAbsPath(fs vfs.FS, stackDir string, stackTargetDir string, stacks []*StackBlockHCL, depth int) []ComponentRef {
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

		ref.ChildRefs = discoverStackChildUnitsWithDepth(fs, sourceDir, stackGenPath, depth+1)

		refs = append(refs, ref)
	}

	return refs
}
