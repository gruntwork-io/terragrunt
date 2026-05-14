// Package hclparse provides phased HCL parsing for terragrunt.stack.hcl files: slurp → DAG locals → include merge → eager unit/stack decode → autoinclude resolution, sharing one progressively-populated eval context.
package hclparse

import (
	"errors"
	"fmt"
	"path/filepath"

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

	// HCL variable root names used in eval context.
	varLocal      = "local"
	varValues     = "values"
	varUnit       = "unit"
	varStack      = "stack"
	varDependency = blockDependency
)

// ParseStackFileInput holds the input for ParseStackFile.
type ParseStackFileInput struct {
	// Values is the `values` overlay registered as the `values` HCL variable so unit/stack/include expressions can reference `values.<key>`.
	Values *cty.Value
	// Variables is the caller-provided HCL variable namespace from the production parser. Parser-owned variables (`unit`, `stack`, `local`, and `values`) are overlaid by ParseStackFile so stale caller state cannot shadow the stack file currently being parsed.
	Variables map[string]cty.Value
	// Functions is the HCL function set the parser registers on the eval context. Callers should pass the function map built by the production parser (pkg/config.createTerragruntEvalContext) so every Terragrunt function call resolves. May be nil for callers (e.g. tests) that only use literal expressions.
	Functions map[string]function.Function
	// Filename is the path passed to HCL diagnostics for source-location reporting.
	Filename string
	// StackDir is the directory of the stack file being parsed; used to resolve include paths relative to the parent file.
	StackDir string
	// Src is the raw bytes of the stack file being parsed.
	Src []byte
}

// ParseResult holds the output of ParseStackFile.
type ParseResult struct {
	// AutoIncludes maps a kind-namespaced component key (AutoIncludeKey: "unit:<name>" or "stack:<name>") to its resolved autoinclude. Namespacing prevents same-name unit/stack collisions.
	AutoIncludes map[string]*AutoIncludeResolved
	Units        []*UnitBlockHCL
	Stacks       []*StackBlockHCL
}

// ParseStackFile runs the four-phase parse documented at the package level. The eval context is built once and progressively populated as each phase succeeds, so every attribute is its natural Go type (no lazy hcl.Expression on unit/stack blocks).
func ParseStackFile(fs vfs.FS, input *ParseStackFileInput) (*ParseResult, error) {
	validateParseStackFileInput(fs, input)

	// Phase 1: slurp.
	slurp, err := slurpStackFile(input.Src, input.Filename)
	if err != nil {
		return nil, err
	}

	evalCtx := buildBaseEvalContext(input)

	// Phase 2: locals.
	if slurp.Locals != nil {
		if err := evaluateLocals(slurp.Locals.Remain, evalCtx); err != nil {
			return nil, err
		}
	}

	// srcByFilename maps each source file (root + each included file) to its bytes so the autoinclude generator can slice expression byte ranges from the right file.
	srcByFilename := map[string][]byte{input.Filename: input.Src}

	// Phase 3: includes. Each included file's Remain is appended via hcl.MergeBodies. Source bytes from each included file are recorded in srcByFilename.
	mergedRemain, err := mergeIncludes(fs, slurp, input.StackDir, evalCtx, srcByFilename)
	if err != nil {
		return nil, err
	}

	// Phase 4: eager decode of unit/stack blocks.
	decoded := &unitsAndStacksHCL{}
	if diags := decodeRemain(mergedRemain, evalCtx, decoded); diags != nil {
		return nil, FileDecodeError{Name: input.Filename, Detail: diags.Error(), Err: diags}
	}

	if err := validateUniqueNames(decoded); err != nil {
		return nil, err
	}

	// Build unit.*/stack.* refs from the decoded blocks and add them to the eval context for autoinclude resolution.
	stackTargetDir := filepath.Join(input.StackDir, StackDir)
	unitRefs := buildUnitRefs(decoded.Units, stackTargetDir)
	stackRefs := buildStackRefs(fs, decoded.Stacks, input.StackDir, stackTargetDir)

	evalCtx.Variables[varUnit] = BuildComponentRefMap(unitRefs)
	evalCtx.Variables[varStack] = BuildComponentRefMap(stackRefs)

	autoIncludes, err := resolveAutoIncludes(decoded, evalCtx, srcByFilename)
	if err != nil {
		return nil, err
	}

	return &ParseResult{
		Units:        decoded.Units,
		Stacks:       decoded.Stacks,
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

// slurpStackFile parses the bytes with hclsyntax and decodes only the top-level locals/include blocks plus Remain. Unit/stack blocks fall through to Remain and are not evaluated here.
func slurpStackFile(src []byte, filename string) (*StackFileHCL, error) {
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

// buildBaseEvalContext composes the eval context used by Phases 2-4: caller-supplied functions and variables (minus parser-owned namespaces), plus the optional `values` overlay.
func buildBaseEvalContext(input *ParseStackFileInput) *hcl.EvalContext {
	evalCtx := &hcl.EvalContext{
		Functions: map[string]function.Function{},
		Variables: map[string]cty.Value{},
	}

	for name, fn := range input.Functions {
		evalCtx.Functions[name] = fn
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

	// Seed unit/stack as empty objects so autoinclude bodies that reference unit.<X>.path before refs are built (Phase 4) get a clear "Unsupported attribute" diagnostic instead of "Unknown variable".
	if _, ok := evalCtx.Variables[varUnit]; !ok {
		evalCtx.Variables[varUnit] = cty.EmptyObjectVal
	}

	if _, ok := evalCtx.Variables[varStack]; !ok {
		evalCtx.Variables[varStack] = cty.EmptyObjectVal
	}

	return evalCtx
}

// decodeRemain wraps gohcl.DecodeBody and returns a non-nil hcl.Diagnostics only when there are errors.
func decodeRemain(body hcl.Body, evalCtx *hcl.EvalContext, target any) hcl.Diagnostics {
	diags := gohcl.DecodeBody(body, evalCtx, target)
	if diags.HasErrors() {
		return diags
	}

	return nil
}

// validateUniqueNames returns a joined error if any unit or stack name is declared more than once after include merging.
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

// buildUnitRefs builds ComponentRef values for each unit block, with the path resolved to the absolute generated location under .terragrunt-stack/.
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

// buildStackRefs builds ComponentRef values for each stack block. Child unit refs are discovered by recursively parsing the stack's source dir (best-effort; non-local sources are skipped).
func buildStackRefs(fs vfs.FS, stacks []*StackBlockHCL, stackDir, stackTargetDir string) []ComponentRef {
	refs := make([]ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		stackGenPath := filepath.Join(stackTargetDir, s.Path)
		if s.NoStack != nil && *s.NoStack {
			stackGenPath = filepath.Join(filepath.Dir(stackTargetDir), s.Path)
		}

		ref := ComponentRef{Name: s.Name, Path: stackGenPath}

		sourceDir := s.Source
		if !filepath.IsAbs(sourceDir) {
			sourceDir = filepath.Join(stackDir, sourceDir)
		}

		ref.ChildRefs = discoverStackChildUnitsWithDepth(fs, sourceDir, stackGenPath, 0)

		refs = append(refs, ref)
	}

	return refs
}

// evaluateLocals resolves locals in DAG order: build a graph of local.X references, evaluate in topological order. Each local is evaluated exactly once. Cycles are detected structurally (no iteration cap).
func evaluateLocals(body hcl.Body, evalCtx *hcl.EvalContext) error {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	// deps[name] = set of local names this local depends on.
	deps := make(map[string]map[string]struct{}, len(syntaxBody.Attributes))

	for name, attr := range syntaxBody.Attributes {
		depSet := make(map[string]struct{})

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

			if _, declared := syntaxBody.Attributes[step.Name]; declared {
				depSet[step.Name] = struct{}{}
			}
		}

		deps[name] = depSet
	}

	order, cycle := topoSortLocals(deps)
	if cycle != nil {
		return LocalsCycleError{Names: cycle}
	}

	evaluated := make(map[string]cty.Value, len(syntaxBody.Attributes))

	evalCtx.Variables[varLocal] = localObject(evaluated)

	for _, name := range order {
		attr := syntaxBody.Attributes[name]

		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return LocalEvalError{Name: name, Detail: diags.Error(), Err: diags}
		}

		evaluated[name] = val
		evalCtx.Variables[varLocal] = localObject(evaluated)
	}

	return nil
}

// topoSortLocals returns the evaluation order for locals (dependencies first). If a cycle is present, returns (nil, cycleNames) where cycleNames are sorted names involved in the cycle.
func topoSortLocals(deps map[string]map[string]struct{}) ([]string, []string) {
	const (
		colorWhite = 0 // unvisited
		colorGray  = 1 // in current DFS path
		colorBlack = 2 // fully processed
	)

	color := make(map[string]int, len(deps))
	order := make([]string, 0, len(deps))

	var visit func(name string, path []string) []string

	visit = func(name string, path []string) []string {
		switch color[name] {
		case colorGray:
			return path
		case colorBlack:
			return nil
		}

		color[name] = colorGray
		path = append(path, name)

		for dep := range deps[name] {
			if cycle := visit(dep, path); cycle != nil {
				return cycle
			}
		}

		color[name] = colorBlack
		order = append(order, name)

		return nil
	}

	names := sortedKeys(deps)

	for _, name := range names {
		if cycle := visit(name, nil); cycle != nil {
			return nil, cycle
		}
	}

	return order, nil
}

// localObject builds a cty.Value for the `local` namespace from the map of evaluated locals. cty.ObjectVal panics on an empty map, so an empty input returns cty.EmptyObjectVal.
func localObject(evaluated map[string]cty.Value) cty.Value {
	if len(evaluated) == 0 {
		return cty.EmptyObjectVal
	}

	return cty.ObjectVal(evaluated)
}

// sortedKeys returns the keys of a map[string]X in sorted order. Used to make iteration deterministic for cycle detection and error messages.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}

	return keys
}

// mergeIncludes evaluates each include block's path expression, parses the included file, and merges its Remain into the parent's. Included files must not declare locals or nested includes. Returns the merged Remain that Phase 4 decodes. srcByFilename is updated with the included file's bytes keyed by filename.
func mergeIncludes(fs vfs.FS, slurp *StackFileHCL, stackDir string, evalCtx *hcl.EvalContext, srcByFilename map[string][]byte) (hcl.Body, error) {
	bodies := []hcl.Body{slurp.Remain}

	for _, inc := range slurp.Includes {
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

// mergeOneInclude reads and slurps a single included file. Returns (includedRemain, includedSrc, includedPath, err) where includedPath is the absolute filename used for HCL diagnostics.
func mergeOneInclude(fs vfs.FS, inc *StackIncludeHCL, stackDir string, evalCtx *hcl.EvalContext) (hcl.Body, []byte, string, error) {
	pathVal, diags := inc.Path.Value(evalCtx)
	if diags.HasErrors() {
		return nil, nil, "", IncludeValidationError{IncludeName: inc.Name, Reason: "could not evaluate include path: " + diags.Error()}
	}

	if pathVal.IsNull() || !pathVal.IsKnown() || pathVal.Type() != cty.String {
		return nil, nil, "", IncludeValidationError{IncludeName: inc.Name, Reason: "include path must evaluate to a non-empty string"}
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

	included, err := slurpStackFile(data, includePath)
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
