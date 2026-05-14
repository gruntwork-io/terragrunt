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
	"github.com/zclconf/go-cty/cty/function"
)

const (
	// StackDir is the directory name where generated stack components are placed.
	StackDir = ".terragrunt-stack"

	// HCL block type and attribute names.
	blockDependency = "dependency"
	attrConfigPath  = "config_path"
	attrPath        = "path"
	attrSource      = "source"

	// HCL variable root names used in eval context.
	varLocal      = "local"
	varValues     = "values"
	varUnit       = "unit"
	varStack      = "stack"
	varDependency = blockDependency
)

// ParseStackFileInput holds the input for ParseStackFile.
type ParseStackFileInput struct {
	Values *cty.Value
	// Variables is the caller-provided HCL variable namespace from the production parser. Parser-owned variables
	// (`unit`, `stack`, `local`, and `values`) are overlaid by ParseStackFile so stale caller state cannot shadow the
	// stack file currently being parsed.
	Variables map[string]cty.Value
	// UnitRefs lets the caller supply already-resolved unit ComponentRefs (with paths, names, and any ChildRefs). When non-nil, the parser uses these for the autoinclude eval context instead of evaluating each unit's Path attribute itself, so non-literal source/path expressions in unrelated unit blocks do not block parsing.
	UnitRefs []ComponentRef
	// StackRefs is the analogous slice for stack blocks. Each ref's ChildRefs must already be populated when callers want stack.<name>.<unit>.path references to resolve.
	StackRefs []ComponentRef
	// Functions is the HCL function set the parser registers on the autoinclude eval context. Callers should pass the function map built by the production parser (pkg/config.createTerragruntEvalContext) so include paths and dependency.config_path expressions evaluate the same way they would in a unit's terragrunt.hcl. May be nil for callers (e.g. tests) that only use literal expressions.
	Functions map[string]function.Function
	Filename  string
	StackDir  string
	Src       []byte
}

// ParseResult holds the output of a two-pass parse of a terragrunt.stack.hcl file.
type ParseResult struct {
	// AutoIncludes maps a kind-namespaced component key (AutoIncludeKey: "unit:<name>" or "stack:<name>") to its resolved autoinclude. Namespacing prevents same-name unit/stack collisions.
	AutoIncludes map[string]*AutoIncludeResolved
	Units        []*UnitBlockHCL
	Stacks       []*StackBlockHCL
}

// ParseStackFile performs a two-pass parse of a terragrunt.stack.hcl file.
//
// Pass 1: Decode unit/stack blocks. Source/Path are captured as lazy hcl.Expression values (non-literal expressions in unrelated unit attributes do not block decoding). The autoinclude body is captured as hcl.Body via remain (not evaluated).
//
// Between passes: Build eval context with unit.<name>.path and stack.<name>.path variables. When the caller supplies pre-resolved UnitRefs/StackRefs, those drive the refs; otherwise paths are evaluated against a stdlib-only context and refs are refreshed after include merging.
//
// Pass 2: For each unit/stack with an autoinclude block, resolve the autoinclude body using the eval context. dependency.config_path is evaluated (references unit.*.path / stack.*.<unit>.path), while inputs are left unevaluated (contain dependency.*.outputs.* which is runtime-only).
func ParseStackFile(fs vfs.FS, input *ParseStackFileInput) (*ParseResult, error) {
	validateParseStackFileInput(fs, input)

	stackFile, err := decodeStackFile(input)
	if err != nil {
		return nil, err
	}

	// Track per-autoinclude source bytes so the generator can slice expression bytes from the correct file even after include merging.
	srcByAutoInclude := map[*AutoIncludeHCL][]byte{}
	recordAutoIncludeSources(srcByAutoInclude, stackFile, input.Src)

	stackTargetDir := filepath.Join(input.StackDir, StackDir)
	callerSuppliedRefs := input.UnitRefs != nil || input.StackRefs != nil

	unitRefs, stackRefs, err := initialRefs(fs, input, stackFile, stackTargetDir, callerSuppliedRefs)
	if err != nil {
		return nil, err
	}

	evalCtx := buildParseEvalContext(input, unitRefs, stackRefs)

	// Evaluate root locals first so include paths can reference local.X. Locals cannot reference unit.X.path / stack.X.path (refs are not bound yet); enforcing one direction keeps the parser single-pass.
	if stackFile.Locals != nil {
		if err := evaluateLocals(stackFile.Locals.Remain, evalCtx); err != nil {
			return nil, err
		}
	}

	if err := processStackIncludes(fs, stackFile, input.StackDir, evalCtx, srcByAutoInclude); err != nil {
		return nil, err
	}

	// Bootstrap path: refresh refs and the unit/stack eval variables now that included units/stacks have been merged into stackFile. Caller-supplied refs already reflect the post-include layout (the production parser does include merging upstream) and must not be overwritten.
	if !callerSuppliedRefs {
		if err := refreshBootstrapRefs(fs, input, stackFile, stackTargetDir, evalCtx); err != nil {
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

// validateParseStackFileInput panics on programmer errors so callers get a stack trace at the call site rather than a downstream nil-deref.
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

// decodeStackFile parses the stack file bytes and decodes the top-level structure into a StackFileHCL.
func decodeStackFile(input *ParseStackFileInput) (*StackFileHCL, error) {
	file, diags := hclsyntax.ParseConfig(input.Src, input.Filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, FileParseError{FilePath: input.Filename, Detail: diags.Error()}
	}

	if err := validateRequiredBlockAttrs(file.Body); err != nil {
		return nil, err
	}

	stackFile := &StackFileHCL{}
	if decodeDiags := gohcl.DecodeBody(file.Body, nil, stackFile); decodeDiags.HasErrors() {
		return nil, FileDecodeError{Name: input.Filename, Detail: decodeDiags.Error()}
	}

	return stackFile, nil
}

func validateRequiredBlockAttrs(body hcl.Body) error {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	for _, block := range syntaxBody.Blocks {
		switch block.Type {
		case "unit", "stack":
			for _, attr := range []string{attrSource, attrPath} {
				if _, exists := block.Body.Attributes[attr]; !exists {
					return FileDecodeError{Name: blockName(block), Detail: fmt.Sprintf("missing required %q attribute", attr)}
				}
			}
		default:
			continue
		}
	}

	return nil
}

func blockName(block *hclsyntax.Block) string {
	if len(block.Labels) == 0 {
		return block.Type
	}

	return fmt.Sprintf("%s %q", block.Type, block.Labels[0])
}

// initialRefs returns the unit/stack ComponentRef slices used to seed the autoinclude eval context. When the caller supplied UnitRefs/StackRefs (post-include refs from the production parser), use them verbatim. Otherwise evaluate each unit/stack's lazy Path against a stdlib-only eval context so include path expressions can use unit.X.path / stack.X.path for what is already declared in the root file.
func initialRefs(fs vfs.FS, input *ParseStackFileInput, stackFile *StackFileHCL, stackTargetDir string, callerSuppliedRefs bool) ([]ComponentRef, []ComponentRef, error) {
	if callerSuppliedRefs {
		return input.UnitRefs, input.StackRefs, nil
	}

	bootstrapCtx := stdlibEvalContext(input.StackDir)

	unitRefs, err := buildRefsWithAbsPath(stackTargetDir, stackFile.Units, bootstrapCtx)
	if err != nil {
		return nil, nil, err
	}

	stackRefs, err := buildStackRefsWithAbsPath(fs, input.StackDir, stackTargetDir, stackFile.Stacks, 0, bootstrapCtx)
	if err != nil {
		return nil, nil, err
	}

	return unitRefs, stackRefs, nil
}

// buildParseEvalContext composes the autoinclude eval context: unit/stack ref objects, any caller-supplied function set, and the caller's `values` overlay.
func buildParseEvalContext(input *ParseStackFileInput, unitRefs, stackRefs []ComponentRef) *hcl.EvalContext {
	evalCtx := BuildAutoIncludeEvalContext(unitRefs, stackRefs)
	if input.Functions != nil {
		evalCtx.Functions = input.Functions
	}

	for name, value := range input.Variables {
		switch name {
		case varLocal, varUnit, varStack, varValues:
			continue
		default:
			evalCtx.Variables[name] = value
		}
	}

	if input.Values != nil {
		evalCtx.Variables[varValues] = *input.Values
	}

	return evalCtx
}

// refreshBootstrapRefs re-evaluates unit/stack Path expressions and updates evalCtx.Variables[varUnit/varStack] after include merging, so autoinclude expressions can reference units/stacks contributed by included files.
func refreshBootstrapRefs(fs vfs.FS, input *ParseStackFileInput, stackFile *StackFileHCL, stackTargetDir string, evalCtx *hcl.EvalContext) error {
	bootstrapCtx := stdlibEvalContext(input.StackDir)

	unitRefs, err := buildRefsWithAbsPath(stackTargetDir, stackFile.Units, bootstrapCtx)
	if err != nil {
		return err
	}

	stackRefs, err := buildStackRefsWithAbsPath(fs, input.StackDir, stackTargetDir, stackFile.Stacks, 0, bootstrapCtx)
	if err != nil {
		return err
	}

	evalCtx.Variables[varUnit] = BuildComponentRefMap(unitRefs)
	evalCtx.Variables[varStack] = BuildComponentRefMap(stackRefs)

	return nil
}

// EvalString evaluates expr against ctx and returns the resulting string. Returns ("", nil) when expr is nil (attribute absent). When ctx is nil the expression must be a constant; if expr references variables, returns an error. attrName is used in error messages.
func EvalString(expr hcl.Expression, ctx *hcl.EvalContext, attrName string) (string, error) {
	if expr == nil {
		return "", nil
	}

	if ctx == nil && len(expr.Variables()) > 0 {
		return "", EmptyArgError{Func: "EvalString", Arg: attrName + " requires eval context"}
	}

	val, diags := expr.Value(ctx)
	if diags.HasErrors() {
		return "", FileDecodeError{Name: attrName, Detail: diags.Error()}
	}

	if val.IsNull() {
		return "", FileDecodeError{Name: attrName, Detail: "must not be null"}
	}

	if !val.IsKnown() {
		return "", nil
	}

	if val.Type() != cty.String {
		return "", FileDecodeError{Name: attrName, Detail: "expected string, got " + val.Type().FriendlyName()}
	}

	return val.AsString(), nil
}

// evaluateLocals iteratively evaluates root locals against evalCtx in dependency order. Returns an error on eval failure, dependency cycle, or iteration overflow. Locals are evaluated before include processing so include paths and other attributes can reference local.X; locals therefore cannot reference unit.X.path / stack.X.path (refs are not bound yet).
func evaluateLocals(body hcl.Body, evalCtx *hcl.EvalContext) error {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	evaluated := make(map[string]cty.Value, len(syntaxBody.Attributes))
	remaining := make(map[string]*hclsyntax.Attribute, len(syntaxBody.Attributes))

	for name, attr := range syntaxBody.Attributes {
		remaining[name] = attr
	}

	const maxLocalsIterations = 10000

	for i := 0; len(remaining) > 0 && i < maxLocalsIterations; i++ {
		progress, err := evaluateLocalsPass(remaining, evaluated, evalCtx)
		if err != nil {
			return err
		}

		if !progress {
			return localsEvalCycleError(remaining)
		}

		evalCtx.Variables[varLocal] = localObject(evaluated)
	}

	if len(remaining) > 0 {
		return LocalsMaxIterError{MaxIterations: maxLocalsIterations, Remaining: len(remaining)}
	}

	evalCtx.Variables[varLocal] = localObject(evaluated)

	return nil
}

// evaluateLocalsPass evaluates every ready local once. Refreshes evalCtx.Variables[varLocal] after each successful evaluation so locals iterated later in the same pass see locals resolved earlier (defeats map-iteration randomness).
func evaluateLocalsPass(remaining map[string]*hclsyntax.Attribute, evaluated map[string]cty.Value, evalCtx *hcl.EvalContext) (bool, error) {
	progress := false

	evalCtx.Variables[varLocal] = localObject(evaluated)

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

		evalCtx.Variables[varLocal] = localObject(evaluated)

		progress = true
	}

	return progress, nil
}

// localObject builds a cty.Value for the `local` namespace from the map of evaluated locals. cty.ObjectVal panics on an empty map, so an empty input returns cty.EmptyObjectVal.
func localObject(evaluated map[string]cty.Value) cty.Value {
	if len(evaluated) == 0 {
		return cty.EmptyObjectVal
	}

	return cty.ObjectVal(evaluated)
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

// processStackIncludes resolves include blocks by parsing the included files and merging their unit/stack blocks into the main stack file. Each include block's path is evaluated lazily against evalCtx so non-literal include paths (e.g. format(...)) work. srcByAutoInclude is populated with per-block source bytes from each included file.
func processStackIncludes(fs vfs.FS, stackFile *StackFileHCL, stackDir string, evalCtx *hcl.EvalContext, srcByAutoInclude map[*AutoIncludeHCL][]byte) error {
	for _, inc := range stackFile.Includes {
		if err := mergeOneInclude(fs, stackFile, inc, stackDir, evalCtx, srcByAutoInclude); err != nil {
			return err
		}
	}

	if err := validateNoDuplicateUnits(stackFile.Units); err != nil {
		return err
	}

	return validateNoDuplicateStacks(stackFile.Stacks)
}

// mergeOneInclude reads and merges a single included stack file. Evaluates the include's path expression using the supplied eval context (so values/locals/functions are available).
func mergeOneInclude(fs vfs.FS, stackFile *StackFileHCL, inc *StackIncludeHCL, stackDir string, evalCtx *hcl.EvalContext, srcByAutoInclude map[*AutoIncludeHCL][]byte) error {
	includePath, err := EvalString(inc.Path, evalCtx, attrPath)
	if err != nil {
		return IncludeValidationError{IncludeName: inc.Name, Reason: "could not evaluate include path: " + err.Error()}
	}

	if includePath == "" {
		return IncludeValidationError{IncludeName: inc.Name, Reason: "include path must evaluate to a non-empty string"}
	}

	if !filepath.IsAbs(includePath) {
		includePath = filepath.Join(stackDir, includePath)
	}

	data, err := vfs.ReadFile(fs, includePath)
	if err != nil {
		return FileReadError{FilePath: includePath, Err: err}
	}

	incFile, diags := hclsyntax.ParseConfig(data, includePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return FileParseError{FilePath: includePath, Detail: diags.Error()}
	}

	if err := validateRequiredBlockAttrs(incFile.Body); err != nil {
		return IncludeValidationError{IncludeName: inc.Name, Reason: err.Error()}
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

// buildRefsWithAbsPath creates ComponentRef values with paths resolved to the absolute location under .terragrunt-stack/. Returns an error when any unit's Path expression cannot be evaluated against evalCtx so callers on the bootstrap path see the root cause rather than a misleading downstream "Unknown variable: unit" diagnostic.
func buildRefsWithAbsPath(stackTargetDir string, units []*UnitBlockHCL, evalCtx *hcl.EvalContext) ([]ComponentRef, error) {
	refs := make([]ComponentRef, 0, len(units))

	for _, u := range units {
		path, err := EvalString(u.Path, evalCtx, attrPath)
		if err != nil {
			return nil, RefEvalError{Kind: "unit", Name: u.Name, Attr: attrPath, Err: err}
		}

		unitPath := filepath.Join(stackTargetDir, path)

		if u.NoStack != nil && *u.NoStack {
			unitPath = filepath.Join(filepath.Dir(stackTargetDir), path)
		}

		refs = append(refs, ComponentRef{
			Name: u.Name,
			Path: unitPath,
		})
	}

	return refs, nil
}

// buildStackRefsWithAbsPath builds ComponentRef values for stack blocks and discovers their child units. Returns an error when any stack's Path or Source expression cannot be evaluated against evalCtx so callers on the bootstrap path see the root cause rather than a misleading downstream "Unknown variable: stack" diagnostic.
func buildStackRefsWithAbsPath(fs vfs.FS, stackDir, stackTargetDir string, stacks []*StackBlockHCL, depth int, evalCtx *hcl.EvalContext) ([]ComponentRef, error) {
	refs := make([]ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		path, err := EvalString(s.Path, evalCtx, attrPath)
		if err != nil {
			return nil, RefEvalError{Kind: "stack", Name: s.Name, Attr: attrPath, Err: err}
		}

		stackGenPath := filepath.Join(stackTargetDir, path)

		if s.NoStack != nil && *s.NoStack {
			stackGenPath = filepath.Join(filepath.Dir(stackTargetDir), path)
		}

		sourceDir, sourceErr := EvalString(s.Source, evalCtx, attrSource)
		if sourceErr != nil {
			return nil, RefEvalError{Kind: "stack", Name: s.Name, Attr: attrSource, Err: sourceErr}
		}

		if !filepath.IsAbs(sourceDir) {
			sourceDir = filepath.Join(stackDir, sourceDir)
		}

		refs = append(refs, ComponentRef{
			Name:      s.Name,
			Path:      stackGenPath,
			ChildRefs: discoverStackChildUnitsWithDepth(fs, sourceDir, stackGenPath, depth+1),
		})
	}

	return refs, nil
}
