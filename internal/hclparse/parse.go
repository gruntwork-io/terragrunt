// Package hclparse parses terragrunt.stack.hcl in four phases: skeleton, locals, includes, unit/stack decode + autoinclude.
package hclparse

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
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
	// Values is passed as the `values` variable in the parse context.
	Values *cty.Value
	// Variables come from production parsing and are merged for parse.
	Variables map[string]cty.Value
	// Functions are copied from the production parser eval context.
	Functions map[string]function.Function
	// Filename is the basename (not full path) used for parse diagnostics.
	Filename string
	// StackDir is used to resolve include paths.
	StackDir string
	// Src is the raw stack file bytes.
	Src []byte
}

// ParseResult holds the output of ParseStackFile.
type ParseResult struct {
	AutoIncludes map[string]*AutoIncludeResolved
	Units        []*UnitBlockHCL
	Stacks       []*StackBlockHCL
}

// ParseStackFile runs the phase flow and returns partial results when decode partially succeeds.
func ParseStackFile(fs vfs.FS, input *ParseStackFileInput) (*ParseResult, error) {
	validateParseStackFileInput(fs, input)

	result := &ParseResult{AutoIncludes: map[string]*AutoIncludeResolved{}}

	// Phase 1 parses the skeleton and keeps unit/stack blocks in Remain.
	parsedStackFile, err := parseStackFileRoot(input.Src, input.Filename)
	if err != nil {
		return result, err
	}

	evalCtx := buildBaseEvalContext(input)

	// Phase 2 evaluates locals before includes are merged.
	if parsedStackFile.Locals != nil {
		if err := evaluateLocals(parsedStackFile.Locals.Remain, evalCtx); err != nil {
			return result, err
		}
	}

	// srcByFilename tracks source bytes by filename so the generator can slice expression bytes from the correct file even after include merging.
	srcByFilename := map[string][]byte{input.Filename: input.Src}

	// Phase 3 resolves include blocks and merges included Remain bodies.
	mergedRemain, err := mergeIncludes(fs, parsedStackFile, input.StackDir, evalCtx, srcByFilename)
	if err != nil {
		return result, err
	}

	// Phase 4 decodes unit/stack blocks and resolves autoincludes.
	decoded := &unitsAndStacksHCL{}
	if diags := gohcl.DecodeBody(mergedRemain, evalCtx, decoded); diags.HasErrors() {
		// Surface partial Units/Stacks before the error so LSP/IDE callers can inspect them.
		result.Units = decoded.Units
		result.Stacks = decoded.Stacks

		return result, FileDecodeError{Name: input.Filename, Err: diags}
	}

	result.Units = decoded.Units
	result.Stacks = decoded.Stacks

	if err := validateUniqueNames(decoded); err != nil {
		return result, err
	}

	stackTargetDir := filepath.Join(input.StackDir, StackDir)
	evalCtx.Variables[varUnit] = BuildComponentRefMap(buildUnitRefs(decoded.Units, stackTargetDir))
	evalCtx.Variables[varStack] = BuildComponentRefMap(buildStackRefs(decoded.Stacks, stackTargetDir))

	autoIncludes, err := resolveAutoIncludes(decoded.Units, decoded.Stacks, evalCtx, srcByFilename)
	if err != nil {
		return result, err
	}

	result.AutoIncludes = autoIncludes

	return result, nil
}

// validateParseStackFileInput panics on malformed parser input.
func validateParseStackFileInput(fs vfs.FS, input *ParseStackFileInput) {
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
}

// parseStackFileRoot parses only locals/include blocks and leaves units/stacks in Remain.
func parseStackFileRoot(src []byte, filename string) (*StackFileHCL, error) {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, FileParseError{FilePath: filename, Err: diags}
	}

	stackFile := &StackFileHCL{}
	if decodeDiags := gohcl.DecodeBody(file.Body, nil, stackFile); decodeDiags.HasErrors() {
		return nil, FileDecodeError{Name: filename, Err: decodeDiags}
	}

	return stackFile, nil
}

// buildBaseEvalContext builds the eval context for phases two to four.
func buildBaseEvalContext(input *ParseStackFileInput) *hcl.EvalContext {
	evalCtx := &hcl.EvalContext{
		Functions: make(map[string]function.Function, len(input.Functions)),
		Variables: make(map[string]cty.Value, len(input.Variables)),
	}

	maps.Copy(evalCtx.Functions, input.Functions)
	maps.Copy(evalCtx.Variables, input.Variables)

	// Strip namespaces the phased parser populates itself so unevaluated refs fail loudly instead of leaking caller values.
	maps.DeleteFunc(evalCtx.Variables, func(name string, _ cty.Value) bool {
		return name == varLocal || name == varUnit || name == varStack || name == varValues
	})

	if input.Values != nil {
		evalCtx.Variables[varValues] = *input.Values
	}

	return evalCtx
}

// validateUniqueNames reports duplicate unit and stack names.
func validateUniqueNames(decoded *unitsAndStacksHCL) error {
	var errs []error

	seenUnits := make(map[string]struct{}, len(decoded.Units))

	for _, u := range decoded.Units {
		if _, exists := seenUnits[u.Name]; exists {
			errs = append(errs, DuplicateUnitNameError{Name: u.Name})
			continue
		}

		seenUnits[u.Name] = struct{}{}
	}

	seenStacks := make(map[string]struct{}, len(decoded.Stacks))

	for _, s := range decoded.Stacks {
		if _, exists := seenStacks[s.Name]; exists {
			errs = append(errs, DuplicateStackNameError{Name: s.Name})
			continue
		}

		seenStacks[s.Name] = struct{}{}
	}

	return errors.Join(errs...)
}

// buildUnitRefs builds component refs for unit blocks; no_dot_terragrunt_stack hoists the unit out of .terragrunt-stack.
func buildUnitRefs(units []*UnitBlockHCL, stackTargetDir string) []ComponentRef {
	refs := make([]ComponentRef, 0, len(units))

	for _, u := range units {
		unitPath := filepath.Join(stackTargetDir, u.Path)
		if u.NoStack != nil && *u.NoStack {
			unitPath = filepath.Join(filepath.Dir(stackTargetDir), u.Path)
		}

		refs = append(refs, ComponentRef{Name: u.Name, Path: unitPath})
	}

	return refs
}

// buildStackRefs builds top-level component refs for stack blocks. ChildRefs
// stay empty here; callers that need stack.<name>.<unit>.path cross-references
// enrich the returned slice between ParseStackFile and ParseResult.Finalize.
func buildStackRefs(stacks []*StackBlockHCL, stackTargetDir string) []ComponentRef {
	refs := make([]ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		stackGenPath := filepath.Join(stackTargetDir, s.Path)
		if s.NoStack != nil && *s.NoStack {
			stackGenPath = filepath.Join(filepath.Dir(stackTargetDir), s.Path)
		}

		refs = append(refs, ComponentRef{Name: s.Name, Path: stackGenPath})
	}

	return refs
}

// maxLocalsIterations bounds the fixed-point loop in evaluateLocals as a safeguard against pathological inputs.
const maxLocalsIterations = 10000

// evaluateLocals resolves locals via fixed-point iteration.
func evaluateLocals(body hcl.Body, evalCtx *hcl.EvalContext) error {
	// In production the caller parses with hclsyntax.ParseConfig, so the body is always *hclsyntax.Body; surface the impossible-state assertion as an error rather than silently swallowing locals.
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return UnexpectedBodyTypeError{FilePath: "locals"}
	}

	attrs := syntaxBody.Attributes
	evaluated := make(map[string]cty.Value, len(attrs))
	evalCtx.Variables[varLocal] = localObject(evaluated)

	remaining := maps.Clone(attrs)

	for range maxLocalsIterations {
		if !attemptEvaluateLocals(remaining, evaluated, evalCtx) {
			return reportUnresolvedLocals(remaining, evalCtx)
		}

		if len(remaining) == 0 {
			return nil
		}
	}

	return LocalsCycleError{Names: slices.Sorted(maps.Keys(remaining))}
}

// attemptEvaluateLocals runs one fixed-point pass; returns true if at least one local was evaluated.
func attemptEvaluateLocals(remaining map[string]*hclsyntax.Attribute, evaluated map[string]cty.Value, evalCtx *hcl.EvalContext) bool {
	progress := false

	for _, name := range slices.Sorted(maps.Keys(remaining)) {
		val, diags := remaining[name].Expr.Value(evalCtx)
		if diags.HasErrors() {
			continue
		}

		evaluated[name] = val
		evalCtx.Variables[varLocal] = localObject(evaluated)

		delete(remaining, name)

		progress = true
	}

	return progress
}

// reportUnresolvedLocals classifies a stuck set of locals as either a cycle or a hard eval error.
func reportUnresolvedLocals(remaining map[string]*hclsyntax.Attribute, evalCtx *hcl.EvalContext) error {
	sortedNames := slices.Sorted(maps.Keys(remaining))

	for _, name := range sortedNames {
		_, diags := remaining[name].Expr.Value(evalCtx)
		if !diagsAreLocalForwardRefOnly(diags, remaining) {
			return LocalEvalError{Name: name, Err: diags}
		}
	}

	return LocalsCycleError{Names: sortedNames}
}

// diagsAreLocalForwardRefOnly reports whether every diagnostic references only still-unresolved locals.
func diagsAreLocalForwardRefOnly(diags hcl.Diagnostics, remaining map[string]*hclsyntax.Attribute) bool {
	for _, d := range diags {
		if d.Expression == nil {
			return false
		}

		hasForwardRef := false

		for _, t := range d.Expression.Variables() {
			if t.RootName() != varLocal {
				return false
			}

			localName, ok := localTraversalName(t)
			if !ok {
				return false
			}

			if _, exists := remaining[localName]; !exists {
				return false
			}

			hasForwardRef = true
		}

		if !hasForwardRef {
			return false
		}
	}

	return true
}

func localTraversalName(t hcl.Traversal) (string, bool) {
	parts := t.SimpleSplit()
	if len(parts.Rel) == 0 {
		return "", false
	}

	attr, ok := parts.Rel[0].(hcl.TraverseAttr)
	if !ok {
		return "", false
	}

	return attr.Name, true
}

// diagAt builds a single-diagnostic slice anchored at rng so callers using errors.As(err, &hcl.Diagnostics{}) get the offending expression's source position.
func diagAt(rng hcl.Range, summary string) hcl.Diagnostics {
	return hcl.Diagnostics{{
		Severity: hcl.DiagError,
		Summary:  summary,
		Subject:  rng.Ptr(),
	}}
}

// localObject builds the parsed local namespace value.
func localObject(evaluated map[string]cty.Value) cty.Value {
	if len(evaluated) == 0 {
		return cty.EmptyObjectVal
	}

	return cty.ObjectVal(evaluated)
}

// resolvedInclude is the result of parsing one include block: the included file's Remain body, its raw source bytes, and the absolute path used in HCL diagnostics.
type resolvedInclude struct {
	Remain hcl.Body
	Path   string
	Src    []byte
}

// mergeIncludes resolves include paths and merges included Remain bodies.
func mergeIncludes(fs vfs.FS, parsedFile *StackFileHCL, stackDir string, evalCtx *hcl.EvalContext, srcByFilename map[string][]byte) (hcl.Body, error) {
	bodies := []hcl.Body{parsedFile.Remain}

	for _, inc := range parsedFile.Includes {
		resolved, err := mergeOneInclude(fs, inc, stackDir, evalCtx)
		if err != nil {
			return nil, err
		}

		srcByFilename[resolved.Path] = resolved.Src

		bodies = append(bodies, resolved.Remain)
	}

	if len(bodies) == 1 {
		return bodies[0], nil
	}

	return hcl.MergeBodies(bodies), nil
}

// mergeOneInclude reads and parses one included file.
func mergeOneInclude(fs vfs.FS, inc *StackIncludeHCL, stackDir string, evalCtx *hcl.EvalContext) (resolvedInclude, error) {
	pathVal, diags := inc.Path.Value(evalCtx)
	if diags.HasErrors() {
		return resolvedInclude{}, IncludeValidationError{
			IncludeName: inc.Name,
			Reason:      "could not evaluate include path: " + diags.Error(),
			Err:         diags,
		}
	}

	pathRange := inc.Path.Range()

	switch {
	case pathVal.IsNull():
		return resolvedInclude{}, IncludeValidationError{IncludeName: inc.Name, Reason: "include path must not be null", Err: diagAt(pathRange, "include path must not be null")}
	case !pathVal.IsKnown():
		return resolvedInclude{}, IncludeValidationError{IncludeName: inc.Name, Reason: "include path is unknown", Err: diagAt(pathRange, "include path is unknown")}
	case pathVal.Type() != cty.String:
		reason := "include path must be a string, got " + pathVal.Type().FriendlyName()
		return resolvedInclude{}, IncludeValidationError{IncludeName: inc.Name, Reason: reason, Err: diagAt(pathRange, reason)}
	}

	includePath := pathVal.AsString()
	if includePath == "" {
		reason := "include path must evaluate to a non-empty string"
		return resolvedInclude{}, IncludeValidationError{IncludeName: inc.Name, Reason: reason, Err: diagAt(pathRange, reason)}
	}

	if !filepath.IsAbs(includePath) {
		includePath = filepath.Join(stackDir, includePath)
	}

	data, err := vfs.ReadFile(fs, includePath)
	if err != nil {
		return resolvedInclude{}, FileReadError{FilePath: includePath, Err: err}
	}

	included, err := parseStackFileRoot(data, includePath)
	if err != nil {
		return resolvedInclude{}, err
	}

	if included.Locals != nil {
		return resolvedInclude{}, IncludeValidationError{IncludeName: inc.Name, Reason: "must not define locals"}
	}

	if len(included.Includes) > 0 {
		return resolvedInclude{}, IncludeValidationError{IncludeName: inc.Name, Reason: "must not define nested includes"}
	}

	return resolvedInclude{Remain: included.Remain, Src: data, Path: includePath}, nil
}

// autoIncludeSourceBytes returns the source bytes of the file an AutoIncludeHCL originated from.
func autoIncludeSourceBytes(srcByFilename map[string][]byte, autoInclude *AutoIncludeHCL) []byte {
	if autoInclude == nil || autoInclude.Remain == nil {
		return nil
	}

	syntaxBody, ok := autoInclude.Remain.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	return srcByFilename[syntaxBody.Range().Filename]
}

// AutoIncludeKey returns the map key for an autoinclude entry, namespaced by component kind to prevent collisions between same-name units and stacks.
func AutoIncludeKey(kind AutoIncludeKind, name string) string {
	return string(kind) + ":" + name
}

// resolveAutoIncludes resolves autoinclude blocks for all units and stacks; keys are namespaced as "unit:name" and "stack:name".
func resolveAutoIncludes(units []*UnitBlockHCL, stacks []*StackBlockHCL, evalCtx *hcl.EvalContext, srcByFilename map[string][]byte) (map[string]*AutoIncludeResolved, error) {
	autoIncludes := make(map[string]*AutoIncludeResolved)

	for _, unit := range units {
		if unit.AutoInclude == nil {
			continue
		}

		resolved, err := resolveAutoInclude(unit.AutoInclude, evalCtx, KindUnit, autoIncludeSourceBytes(srcByFilename, unit.AutoInclude))
		if err != nil {
			return nil, err
		}

		if resolved != nil {
			autoIncludes[AutoIncludeKey(KindUnit, unit.Name)] = resolved
		}
	}

	for _, stack := range stacks {
		if stack.AutoInclude == nil {
			continue
		}

		resolved, err := resolveAutoInclude(stack.AutoInclude, evalCtx, KindStack, autoIncludeSourceBytes(srcByFilename, stack.AutoInclude))
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
