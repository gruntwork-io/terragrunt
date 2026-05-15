// Package hclparse parses terragrunt.stack.hcl in phases: skeleton, locals, includes, unit/stack decode, autoincludes.
// Locals evaluate before include merge, and unit/stack vars become available after phase four.
package hclparse

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

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
	// Filename is used for parse diagnostics.
	Filename string
	// StackDir is used to resolve include paths.
	StackDir string
	// Src is the raw stack file bytes.
	Src []byte
}

// ParseResult holds the output of ParseStackFile.
type ParseResult struct {
	// AutoIncludes stores resolved autoincludes by component key.
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

	// srcByFilename tracks source bytes by filename.
	srcByFilename := map[string][]byte{input.Filename: input.Src}

	// Phase 3 resolves include blocks and merges included Remain bodies.
	mergedRemain, err := mergeIncludes(fs, parsedStackFile, input.StackDir, evalCtx, srcByFilename)
	if err != nil {
		return result, err
	}

	// Phase 4 decodes unit/stack blocks and resolves autoincludes.
	decoded := &unitsAndStacksHCL{}
	if diags := gohcl.DecodeBody(mergedRemain, evalCtx, decoded); diags.HasErrors() {
		// Keep successful results when other attributes fail.
		result.Units = decoded.Units
		result.Stacks = decoded.Stacks

		return result, FileDecodeError{Name: input.Filename, Detail: diags.Error(), Err: diags}
	}

	result.Units = decoded.Units
	result.Stacks = decoded.Stacks

	if err := validateUniqueNames(decoded); err != nil {
		return result, err
	}

	// Build unit/stack refs and inject them into the eval context.
	stackTargetDir := filepath.Join(input.StackDir, StackDir)
	unitRefs := buildUnitRefs(decoded.Units, stackTargetDir)
	stackRefs := buildStackRefs(fs, decoded.Stacks, input.StackDir, stackTargetDir)

	evalCtx.Variables[varUnit] = BuildComponentRefMap(unitRefs)
	evalCtx.Variables[varStack] = BuildComponentRefMap(stackRefs)

	autoIncludes, err := resolveAutoIncludes(decoded, evalCtx, srcByFilename)
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
		return nil, FileParseError{FilePath: filename, Detail: diags.Error()}
	}

	stackFile := &StackFileHCL{}
	if decodeDiags := gohcl.DecodeBody(file.Body, nil, stackFile); decodeDiags.HasErrors() {
		return nil, FileDecodeError{Name: filename, Detail: decodeDiags.Error(), Err: decodeDiags}
	}

	return stackFile, nil
}

// isReservedVarName reports whether a caller-supplied variable name collides with a parser-owned namespace and must be skipped when overlaying caller variables on the eval context.
func isReservedVarName(name string, _ cty.Value) bool {
	switch name {
	case varLocal, varUnit, varStack, varValues:
		return true
	}

	return false
}

// buildBaseEvalContext builds the eval context for phases two to four.
func buildBaseEvalContext(input *ParseStackFileInput) *hcl.EvalContext {
	evalCtx := &hcl.EvalContext{
		Functions: map[string]function.Function{},
		Variables: map[string]cty.Value{},
	}

	maps.Copy(evalCtx.Functions, input.Functions)
	maps.Copy(evalCtx.Variables, input.Variables)
	maps.DeleteFunc(evalCtx.Variables, isReservedVarName)

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

// buildUnitRefs builds component refs for unit blocks.
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

// buildStackRefs builds component refs for stack blocks.
func buildStackRefs(fs vfs.FS, stacks []*StackBlockHCL, stackDir, stackTargetDir string) []ComponentRef {
	refs := make([]ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		stackGenPath := filepath.Join(stackTargetDir, s.Path)
		if s.NoStack != nil && *s.NoStack {
			stackGenPath = filepath.Join(filepath.Dir(stackTargetDir), s.Path)
		}

		ref := ComponentRef{Name: s.Name, Path: stackGenPath}

		if sourceDir, ok := localStackSourceDir(s.Source, stackDir); ok {
			ref.ChildRefs = discoverStackChildUnitsWithDepth(fs, sourceDir, stackGenPath, 0)
		}

		refs = append(refs, ref)
	}

	return refs
}

// localStackSourceDir returns local stack source directories and skips remote sources.
func localStackSourceDir(source, stackDir string) (string, bool) {
	if strings.HasPrefix(source, "file://") {
		p := strings.TrimPrefix(source, "file://")
		if !filepath.IsAbs(p) {
			p = filepath.Join(stackDir, p)
		}

		return filepath.Clean(p), true
	}

	if strings.Contains(source, "://") {
		return "", false
	}

	if strings.Contains(source, "::") {
		return "", false
	}

	if filepath.IsAbs(source) {
		return filepath.Clean(source), true
	}

	return filepath.Clean(filepath.Join(stackDir, source)), true
}

// evaluateLocals resolves locals in dependency order and catches cycles.
func evaluateLocals(body hcl.Body, evalCtx *hcl.EvalContext) error {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	deps := buildLocalsDeps(syntaxBody.Attributes)

	order, cycle := topoSortLocals(deps)
	if cycle != nil {
		return LocalsCycleError{Names: cycle}
	}

	return evaluateLocalsInOrder(syntaxBody.Attributes, order, evalCtx)
}

// buildLocalsDeps builds local dependencies for topo sorting.
func buildLocalsDeps(attrs map[string]*hclsyntax.Attribute) map[string]map[string]struct{} {
	deps := make(map[string]map[string]struct{}, len(attrs))

	for name, attr := range attrs {
		deps[name] = localDependsOn(attr, attrs)
	}

	return deps
}

// localDependsOn returns locals referenced by an attribute.
func localDependsOn(attr *hclsyntax.Attribute, declared map[string]*hclsyntax.Attribute) map[string]struct{} {
	depSet := make(map[string]struct{})

	for _, traversal := range attr.Expr.Variables() {
		if traversal.RootName() != varLocal {
			continue
		}

		name, ok := firstAttrStep(traversal)
		if !ok {
			continue
		}

		if _, exists := declared[name]; exists {
			depSet[name] = struct{}{}
		}
	}

	return depSet
}

// firstAttrStep returns the first attribute in a traversal.
func firstAttrStep(traversal hcl.Traversal) (string, bool) {
	split := traversal.SimpleSplit()
	if len(split.Rel) == 0 {
		return "", false
	}

	step, ok := split.Rel[0].(hcl.TraverseAttr)
	if !ok {
		return "", false
	}

	return step.Name, true
}

// evaluateLocalsInOrder evaluates locals and updates evalCtx locals.
func evaluateLocalsInOrder(attrs map[string]*hclsyntax.Attribute, order []string, evalCtx *hcl.EvalContext) error {
	evaluated := make(map[string]cty.Value, len(order))
	evalCtx.Variables[varLocal] = localObject(evaluated)

	for _, name := range order {
		val, diags := attrs[name].Expr.Value(evalCtx)
		if diags.HasErrors() {
			return LocalEvalError{Name: name, Detail: diags.Error(), Err: diags}
		}

		evaluated[name] = val
		evalCtx.Variables[varLocal] = localObject(evaluated)
	}

	return nil
}

// topoSortLocals returns local order and cycle information.
func topoSortLocals(deps map[string]map[string]struct{}) ([]string, []string) {
	s := newTopoState(deps)

	for _, name := range sortedKeys(deps) {
		if cycle := s.visit(name, nil); cycle != nil {
			return nil, cycle
		}
	}

	return s.order, nil
}

const (
	topoColorWhite = 0 // unvisited
	topoColorGray  = 1 // in current DFS path
	topoColorBlack = 2 // fully processed
)

// topoState stores DFS state and evaluation order.
type topoState struct {
	deps  map[string]map[string]struct{}
	color map[string]int
	order []string
}

func newTopoState(deps map[string]map[string]struct{}) *topoState {
	return &topoState{
		deps:  deps,
		color: make(map[string]int, len(deps)),
		order: make([]string, 0, len(deps)),
	}
}

// visit performs DFS and returns a cycle when one is detected.
func (s *topoState) visit(name string, path []string) []string {
	switch s.color[name] {
	case topoColorGray:
		return cycleSegment(path, name)
	case topoColorBlack:
		return nil
	}

	s.color[name] = topoColorGray
	path = append(path, name)

	for dep := range s.deps[name] {
		if cycle := s.visit(dep, path); cycle != nil {
			return cycle
		}
	}

	s.color[name] = topoColorBlack
	s.order = append(s.order, name)

	return nil
}

// cycleSegment returns the path slice that forms a dependency cycle.
func cycleSegment(path []string, name string) []string {
	if i := slices.Index(path, name); i >= 0 {
		return path[i:]
	}

	return path
}

// localObject builds the parsed local namespace value.
func localObject(evaluated map[string]cty.Value) cty.Value {
	if len(evaluated) == 0 {
		return cty.EmptyObjectVal
	}

	return cty.ObjectVal(evaluated)
}

// sortedKeys returns keys in deterministic order.
func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}

// mergeIncludes resolves include paths and merges included Remain bodies.
func mergeIncludes(fs vfs.FS, parsedFile *StackFileHCL, stackDir string, evalCtx *hcl.EvalContext, srcByFilename map[string][]byte) (hcl.Body, error) {
	bodies := []hcl.Body{parsedFile.Remain}

	for _, inc := range parsedFile.Includes {
		includedRemain, includedSrc, includedPath, err := mergeOneInclude(fs, inc, stackDir, evalCtx)
		if err != nil {
			return nil, err
		}

		srcByFilename[includedPath] = includedSrc

		bodies = append(bodies, includedRemain)
	}

	if len(bodies) == 1 {
		return bodies[0], nil
	}

	return hcl.MergeBodies(bodies), nil
}

// mergeOneInclude reads and parses one included file.
func mergeOneInclude(fs vfs.FS, inc *StackIncludeHCL, stackDir string, evalCtx *hcl.EvalContext) (hcl.Body, []byte, string, error) {
	pathVal, diags := inc.Path.Value(evalCtx)
	if diags.HasErrors() {
		return nil, nil, "", IncludeValidationError{IncludeName: inc.Name, Reason: "could not evaluate include path: " + diags.Error()}
	}

	switch {
	case pathVal.IsNull():
		return nil, nil, "", IncludeValidationError{IncludeName: inc.Name, Reason: "include path must not be null"}
	case !pathVal.IsKnown():
		return nil, nil, "", IncludeValidationError{IncludeName: inc.Name, Reason: "include path is unknown"}
	case pathVal.Type() != cty.String:
		return nil, nil, "", IncludeValidationError{IncludeName: inc.Name, Reason: "include path must be a string, got " + pathVal.Type().FriendlyName()}
	}

	includePath := pathVal.AsString()
	if includePath == "" {
		return nil, nil, "", IncludeValidationError{IncludeName: inc.Name, Reason: "include path must evaluate to a non-empty string"}
	}

	if !filepath.IsAbs(includePath) {
		includePath = filepath.Join(stackDir, includePath)
	}

	data, err := vfs.ReadFile(fs, includePath)
	if err != nil {
		return nil, nil, "", FileReadError{FilePath: includePath, Err: err}
	}

	included, err := parseStackFileRoot(data, includePath)
	if err != nil {
		return nil, nil, "", err
	}

	if included.Locals != nil {
		return nil, nil, "", IncludeValidationError{IncludeName: inc.Name, Reason: "must not define locals"}
	}

	if len(included.Includes) > 0 {
		return nil, nil, "", IncludeValidationError{IncludeName: inc.Name, Reason: "must not define nested includes"}
	}

	return included.Remain, data, includePath, nil
}

// autoIncludeSourceBytes returns the source bytes of the file an AutoIncludeHCL originated from. The mapping is by filename (each hcl.Body knows its file via Range().Filename), so the same map serves both the root and any included files.
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

// resolveAutoIncludes resolves autoinclude blocks for all units and stacks. Keys are namespaced as "unit:name" and "stack:name". srcByFilename provides the source bytes of each file (root + included) so generation can slice expression byte ranges from the correct file.
func resolveAutoIncludes(decoded *unitsAndStacksHCL, evalCtx *hcl.EvalContext, srcByFilename map[string][]byte) (map[string]*AutoIncludeResolved, error) {
	autoIncludes := make(map[string]*AutoIncludeResolved)

	for _, unit := range decoded.Units {
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

	for _, stack := range decoded.Stacks {
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
