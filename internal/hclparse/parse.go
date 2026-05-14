// Package hclparse provides two-phase HCL parsing for stack files with
// support for autoinclude blocks and deferred evaluation.
package hclparse

import (
	"errors"
	"fmt"
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
	// AutoIncludes maps component name -> resolved autoinclude (only for units/stacks
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
		filename := ""
		if input != nil {
			filename = input.Filename
		}

		panic(fmt.Sprintf("hclparse.ParseStackFile: fs is nil (filename=%q)", filename))
	}

	if input == nil {
		panic("hclparse.ParseStackFile: input is nil")
	}

	if input.StackDir == "" {
		panic(fmt.Sprintf("hclparse.ParseStackFile: input.StackDir is empty (filename=%q)", input.Filename))
	}

	file, diags := hclsyntax.ParseConfig(input.Src, input.Filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, FileParseError{FilePath: input.Filename, Detail: diags.Error()}
	}

	// Pass 1: decode unit/stack blocks. Autoinclude body captured as remain.
	stackFile := &StackFileHCL{}

	diags = gohcl.DecodeBody(file.Body, nil, stackFile)
	if diags.HasErrors() {
		return nil, FileDecodeError{Name: input.Filename, Detail: diags.Error()}
	}

	// Track per-autoinclude source bytes so the generator can slice expression bytes from the correct file even after include merging.
	srcByAutoInclude := map[*AutoIncludeHCL][]byte{}
	recordAutoIncludeSources(srcByAutoInclude, stackFile, input.Src)

	// Process includes: merge included units/stacks.
	if err := processStackIncludes(fs, stackFile, input.StackDir, srcByAutoInclude); err != nil {
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

	autoIncludes, err := resolveAutoIncludes(stackFile, evalCtx, srcByAutoInclude)
	if err != nil {
		return nil, err
	}

	return &ParseResult{
		Units:        stackFile.Units,
		Stacks:       stackFile.Stacks,
		AutoIncludes: autoIncludes,
	}, nil
}

// evaluateLocals evaluates the attributes of a locals block in dependency order.
//
// The algorithm builds a DAG of local-to-local references from each attribute's
// AST traversals, sorts it with Kahn's algorithm, and evaluates each attribute
// once its dependencies have been resolved. evalCtx.Variables[varLocal] is
// refreshed after every successful evaluation so later attributes resolve
// against the cumulative result.
//
// References to other namespaces (unit, stack, values, feature, dependency, …)
// are not edges in the graph: they are external to the locals block and must
// already be populated in evalCtx by the caller. If they are missing or fail
// to resolve, HCL's own diagnostics surface as LocalEvalError at evaluation time.
//
// Cycles among locals are reported as LocalsCycleError listing the participating
// names and their remaining edges.
func evaluateLocals(body hcl.Body, evalCtx *hcl.EvalContext) error {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		// Non-syntax bodies (e.g. from JSON configs) don't expose the AST we need
		// to build the dependency graph; fall through silently rather than error.
		return nil
	}

	attrs := syntaxBody.Attributes
	if len(attrs) == 0 {
		return nil
	}

	deps := buildLocalsGraph(attrs)

	return evaluateLocalsInOrder(attrs, deps, evalCtx)
}

// buildLocalsGraph returns each attribute's set of sibling-local dependencies,
// derived from `local.<name>` traversals in its expression AST. References to
// non-sibling names (e.g. `local.<undefined>`) are deliberately ignored so HCL
// can surface a precise diagnostic at eval time; only sibling-to-sibling edges
// drive evaluation order.
//
// A traversal rooted at `local` with no static attribute step — for example
// the bare reference `local` or a dynamic subscript like `local[expr]` — is
// promoted to "depends on every sibling," guaranteeing the attribute is
// evaluated last and observes the fully-populated local map (or surfaces a
// cycle if every attribute does this).
//
// Dependencies are deduped and sorted so the evaluation order is deterministic.
func buildLocalsGraph(attrs map[string]*hclsyntax.Attribute) map[string][]string {
	deps := make(map[string][]string, len(attrs))
	broadDeps := map[string]struct{}{}

	for name, attr := range attrs {
		seen := map[string]struct{}{}

		for _, traversal := range attr.Expr.Variables() {
			if traversal.RootName() != varLocal {
				continue
			}

			depName := firstAttrStep(traversal)
			if depName == "" {
				broadDeps[name] = struct{}{}

				continue
			}

			if _, sibling := attrs[depName]; !sibling {
				continue
			}

			if _, dup := seen[depName]; dup {
				continue
			}

			seen[depName] = struct{}{}
			deps[name] = append(deps[name], depName)
		}
	}

	for name := range broadDeps {
		existing := map[string]struct{}{}
		for _, d := range deps[name] {
			existing[d] = struct{}{}
		}

		for sib := range attrs {
			if sib == name {
				continue
			}

			if _, dup := existing[sib]; dup {
				continue
			}

			deps[name] = append(deps[name], sib)
		}
	}

	for name := range deps {
		slices.Sort(deps[name])
	}

	return deps
}

// evaluateLocalsInOrder runs Kahn's algorithm over deps, evaluating each
// attribute once its dependencies are resolved. evalCtx.Variables[varLocal]
// is refreshed after every successful evaluation. Returns LocalsCycleError if
// any attribute remains unresolved after the queue drains.
func evaluateLocalsInOrder(
	attrs map[string]*hclsyntax.Attribute,
	deps map[string][]string,
	evalCtx *hcl.EvalContext,
) error {
	inDegree := make(map[string]int, len(attrs))
	dependents := make(map[string][]string, len(attrs))

	for name := range attrs {
		inDegree[name] = len(deps[name])

		for _, d := range deps[name] {
			dependents[d] = append(dependents[d], name)
		}
	}

	ready := make([]string, 0, len(attrs))

	for name, d := range inDegree {
		if d == 0 {
			ready = append(ready, name)
		}
	}

	slices.Sort(ready)

	evaluated := make(map[string]cty.Value, len(attrs))

	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]

		val, diags := attrs[name].Expr.Value(evalCtx)
		if diags.HasErrors() {
			return LocalEvalError{Name: name, Detail: diags.Error()}
		}

		evaluated[name] = val
		evalCtx.Variables[varLocal] = cty.ObjectVal(evaluated)

		var nextReady []string

		for _, dep := range dependents[name] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				nextReady = append(nextReady, dep)
			}
		}

		if len(nextReady) > 0 {
			slices.Sort(nextReady)
			ready = append(ready, nextReady...)
		}
	}

	if len(evaluated) == len(attrs) {
		return nil
	}

	return cycleErrorFor(attrs, deps, evaluated)
}

// cycleErrorFor builds a LocalsCycleError naming every attribute that did not
// evaluate and surfacing the local→local edges that survived among them. The
// edges help users locate the cycle without having to re-read the source.
func cycleErrorFor(
	attrs map[string]*hclsyntax.Attribute,
	deps map[string][]string,
	evaluated map[string]cty.Value,
) error {
	remaining := make([]string, 0, len(attrs)-len(evaluated))

	for name := range attrs {
		if _, ok := evaluated[name]; !ok {
			remaining = append(remaining, name)
		}
	}

	slices.Sort(remaining)

	cycleDeps := make(map[string][]string, len(remaining))

	for _, name := range remaining {
		for _, d := range deps[name] {
			if _, ok := evaluated[d]; ok {
				continue
			}

			cycleDeps[name] = append(cycleDeps[name], d)
		}

		slices.Sort(cycleDeps[name])
	}

	return LocalsCycleError{Names: remaining, Edges: cycleDeps}
}

// firstAttrStep returns the name of the first TraverseAttr after the traversal
// root, or the empty string if the traversal has no relative steps or the
// first step is not an attribute access (e.g. dynamic subscript, splat).
func firstAttrStep(traversal hcl.Traversal) string {
	split := traversal.SimpleSplit()
	if len(split.Rel) == 0 {
		return ""
	}

	step, ok := split.Rel[0].(hcl.TraverseAttr)
	if !ok {
		return ""
	}

	return step.Name
}

// AutoIncludeKey returns the map key for an autoinclude entry, namespaced by component kind to prevent collisions between same-name units and stacks.
func AutoIncludeKey(kind AutoIncludeKind, name string) string {
	return string(kind) + ":" + name
}

// resolveAutoIncludes resolves autoinclude blocks for all units and stacks in the stack file. Keys are namespaced as "unit:name" and "stack:name" to prevent same-name collisions. srcByAutoInclude maps each AutoInclude pointer to the source bytes of the file it was parsed from so generation can slice expressions from the correct file after include merging.
func resolveAutoIncludes(stackFile *StackFileHCL, evalCtx *hcl.EvalContext, srcByAutoInclude map[*AutoIncludeHCL][]byte) (map[string]*AutoIncludeResolved, error) {
	autoIncludes := make(map[string]*AutoIncludeResolved)

	for _, unit := range stackFile.Units {
		if unit.AutoInclude == nil {
			continue
		}

		resolved, err := resolveAutoInclude(unit.AutoInclude, evalCtx, KindUnit, srcByAutoInclude[unit.AutoInclude])
		if err != nil {
			return nil, err
		}

		if resolved != nil {
			autoIncludes[AutoIncludeKey(KindUnit, unit.Name)] = resolved
		}
	}

	for _, stack := range stackFile.Stacks {
		if stack.AutoInclude == nil {
			continue
		}

		resolved, err := resolveAutoInclude(stack.AutoInclude, evalCtx, KindStack, srcByAutoInclude[stack.AutoInclude])
		if err != nil {
			return nil, err
		}

		if resolved != nil {
			autoIncludes[AutoIncludeKey(KindStack, stack.Name)] = resolved
		}
	}

	return autoIncludes, nil
}

// resolveAutoInclude resolves a single autoinclude block, attaches the eval context, tags it with the component kind so the generator picks the right filename, and records the originating file's bytes for include-aware expression slicing.
func resolveAutoInclude(autoInclude *AutoIncludeHCL, evalCtx *hcl.EvalContext, kind AutoIncludeKind, sourceBytes []byte) (*AutoIncludeResolved, error) {
	resolved, diags := autoInclude.Resolve(evalCtx)
	if diags.HasErrors() {
		return nil, diags
	}

	if resolved != nil {
		resolved.EvalCtx = evalCtx
		resolved.Kind = kind
		resolved.SourceBytes = sourceBytes
	}

	return resolved, nil
}

// processStackIncludes resolves include blocks by parsing the included files and merging their unit/stack blocks into the main stack file. srcByAutoInclude is populated with per-block source bytes from each included file.
func processStackIncludes(fs vfs.FS, stackFile *StackFileHCL, stackDir string, srcByAutoInclude map[*AutoIncludeHCL][]byte) error {
	for _, inc := range stackFile.Includes {
		if err := mergeOneInclude(fs, stackFile, inc, stackDir, srcByAutoInclude); err != nil {
			return err
		}
	}

	if err := validateNoDuplicateUnits(stackFile.Units); err != nil {
		return err
	}

	return validateNoDuplicateStacks(stackFile.Stacks)
}

// mergeOneInclude reads and merges a single included stack file.
func mergeOneInclude(fs vfs.FS, stackFile *StackFileHCL, inc *StackIncludeHCL, stackDir string, srcByAutoInclude map[*AutoIncludeHCL][]byte) error {
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

	// Record per-autoinclude source bytes for the included file so generation slices the correct source after units/stacks are merged into the root.
	recordAutoIncludeSources(srcByAutoInclude, included, data)

	stackFile.Units = append(stackFile.Units, included.Units...)
	stackFile.Stacks = append(stackFile.Stacks, included.Stacks...)

	return nil
}

// recordAutoIncludeSources maps each AutoInclude pointer in stackFile to its source bytes; relies on gohcl.DecodeBody allocating fresh struct pointers (pointer-keyed identity).
func recordAutoIncludeSources(srcByAutoInclude map[*AutoIncludeHCL][]byte, stackFile *StackFileHCL, src []byte) {
	for _, u := range stackFile.Units {
		if u != nil && u.AutoInclude != nil {
			srcByAutoInclude[u.AutoInclude] = src
		}
	}

	for _, s := range stackFile.Stacks {
		if s != nil && s.AutoInclude != nil {
			srcByAutoInclude[s.AutoInclude] = src
		}
	}
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

// buildStackRefsWithAbsPath builds ComponentRef values for stack blocks and discovers their child units.
func buildStackRefsWithAbsPath(fs vfs.FS, stackDir string, stackTargetDir string, stacks []*StackBlockHCL, depth int) []ComponentRef {
	refs := make([]ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		stackGenPath := filepath.Join(stackTargetDir, s.Path)

		if s.NoStack != nil && *s.NoStack {
			stackGenPath = filepath.Join(filepath.Dir(stackTargetDir), s.Path)
		}

		sourceDir := s.Source
		if !filepath.IsAbs(sourceDir) {
			sourceDir = filepath.Join(stackDir, sourceDir)
		}

		refs = append(refs, ComponentRef{
			Name:      s.Name,
			Path:      stackGenPath,
			ChildRefs: discoverStackChildUnitsWithDepth(fs, sourceDir, stackGenPath, depth+1),
		})
	}

	return refs
}
