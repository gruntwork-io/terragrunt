package hclparse

import (
	"fmt"
	iofs "io/fs"
	"path/filepath"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// stackFileName is the canonical filename of a Terragrunt stack file.
const stackFileName = "terragrunt.stack.hcl"

// valuesFileName is the canonical filename of the generated values file that stack
// generation writes next to a generated terragrunt.stack.hcl. It mirrors the
// constant pkg/config uses when writing the file; the two packages cannot share it
// because internal/hclparse must not depend on pkg/config.
const valuesFileName = "terragrunt.values.hcl"

// StackFileHCL is the parsed skeleton: locals, includes, and Remain.
type StackFileHCL struct {
	Remain   hcl.Body           `hcl:",remain"`
	Locals   *LocalsHCL         `hcl:"locals,block"`
	Includes []*StackIncludeHCL `hcl:"include,block"`
}

// LocalsHCL is the locals block shell.
type LocalsHCL struct {
	Remain hcl.Body `hcl:",remain"`
}

// StackIncludeHCL stores include path as a lazy expression.
type StackIncludeHCL struct {
	Path hcl.Expression `hcl:"path,attr"`
	Name string         `hcl:",label"`
}

// unitsAndStacksHCL is the phase-3 decode target for unit and stack blocks.
type unitsAndStacksHCL struct {
	Remain hcl.Body         `hcl:",remain"`
	Stacks []*StackBlockHCL `hcl:"stack,block"`
	Units  []*UnitBlockHCL  `hcl:"unit,block"`
}

// UnitBlockHCL is the eager unit block shape with deferred autoinclude content.
type UnitBlockHCL struct {
	Remain       hcl.Body        `hcl:",remain"`
	AutoInclude  *AutoIncludeHCL `hcl:"autoinclude,block"`
	NoStack      *bool           `hcl:"no_dot_terragrunt_stack,optional"`
	NoValidation *bool           `hcl:"no_validation,optional"`
	Values       *cty.Value      `hcl:"values,optional"`
	Source       string          `hcl:"source,attr"`
	Path         string          `hcl:"path,attr"`
	Name         string          `hcl:",label"`
}

// StackBlockHCL is the eager stack block shape with deferred autoinclude content.
type StackBlockHCL struct {
	Remain       hcl.Body        `hcl:",remain"`
	AutoInclude  *AutoIncludeHCL `hcl:"autoinclude,block"`
	NoStack      *bool           `hcl:"no_dot_terragrunt_stack,optional"`
	NoValidation *bool           `hcl:"no_validation,optional"`
	Values       *cty.Value      `hcl:"values,optional"`
	Source       string          `hcl:"source,attr"`
	Path         string          `hcl:"path,attr"`
	Name         string          `hcl:",label"`
}

// ComponentRef is a top-level unit or stack ref injected into the eval context
// as `unit.<name>` or `stack.<name>`. Each ref carries its label and its
// generated path; only the path is exposed in HCL.
type ComponentRef struct {
	Name string
	Path string
}

// BuildComponentRefMap converts component refs into an HCL object injected as
// the `unit` or `stack` variable in the eval context. Empty input returns
// EmptyObjectVal so typos surface as "Unsupported attribute" diagnostics.
//
// Output shape:
//
//	{
//	  "<name>": { "path": "<generated path>" }
//	}
func BuildComponentRefMap(refs []ComponentRef) cty.Value {
	if len(refs) == 0 {
		return cty.EmptyObjectVal
	}

	refMap := make(map[string]cty.Value, len(refs))

	for _, ref := range refs {
		refMap[ref.Name] = cty.ObjectVal(map[string]cty.Value{
			"path": cty.StringVal(ref.Path),
		})
	}

	return cty.ObjectVal(refMap)
}

// GeneratedComponentPath returns the on-disk path a unit or stack block in a
// terragrunt.stack.hcl generates to. stackDir is the directory containing the
// stack file, path is the block's path attribute, and noStack reports whether the
// block sets no_dot_terragrunt_stack, which hoists the component out of the
// .terragrunt-stack subdirectory.
func GeneratedComponentPath(stackDir, path string, noStack bool) string {
	if noStack {
		return filepath.Join(stackDir, path)
	}

	return filepath.Join(stackDir, StackDir, path)
}

// GeneratedPath returns the on-disk path this unit generates to under stackDir.
func (u *unitPathOnlyHCL) GeneratedPath(stackDir string) string {
	return GeneratedComponentPath(stackDir, u.Path, u.NoStack != nil && *u.NoStack)
}

// unitPathOnlyHCL is the discovery shape for unit name and path.
type unitPathOnlyHCL struct {
	Remain  hcl.Body `hcl:",remain"`
	NoStack *bool    `hcl:"no_dot_terragrunt_stack,optional"`
	Path    string   `hcl:"path,attr"`
	Name    string   `hcl:",label"`
}

// stackPathOnlyHCL is the discovery shape for stack name, path, and source; Source is lazy so non-literal sources don't block decode.
type stackPathOnlyHCL struct {
	Remain  hcl.Body       `hcl:",remain"`
	NoStack *bool          `hcl:"no_dot_terragrunt_stack,optional"`
	Source  hcl.Expression `hcl:"source,attr"`
	Path    string         `hcl:"path,attr"`
	Name    string         `hcl:",label"`
}

// GeneratedPath returns the on-disk path this stack generates to under stackDir.
func (s *stackPathOnlyHCL) GeneratedPath(stackDir string) string {
	return GeneratedComponentPath(stackDir, s.Path, s.NoStack != nil && *s.NoStack)
}

// discoveryDecode holds decoded unit and stack blocks for discovery.
type discoveryDecode struct {
	Remain hcl.Body            `hcl:",remain"`
	Stacks []*stackPathOnlyHCL `hcl:"stack,block"`
	Units  []*unitPathOnlyHCL  `hcl:"unit,block"`
}

// discoveryAutoIncludeDecode is the strict decode target for a stack autoinclude file: only unit and
// stack blocks are allowed at the top level. It has no ",remain" field, so stray top-level content
// (a misplaced attribute or a generate/remote_state/dependency block) is rejected here just as the
// full parse rejects it via the strict StackConfigFile, rather than being silently ignored.
type discoveryAutoIncludeDecode struct {
	Stacks []*stackPathOnlyHCL `hcl:"stack,block"`
	Units  []*unitPathOnlyHCL  `hcl:"unit,block"`
}

// ParseStackFileFromPath reads a terragrunt.stack.hcl from disk and runs ParseStackFile; returns (nil, nil) when the file is absent.
func ParseStackFileFromPath(fs vfs.FS, stackDir string) (*ParseResult, error) {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.ParseStackFileFromPath: fs is nil (stackDir=%q)", stackDir))
	}

	if stackDir == "" {
		panic("hclparse.ParseStackFileFromPath: stackDir is empty")
	}

	stackDir = util.ResolvePath(stackDir)
	stackFile := filepath.Join(stackDir, stackFileName)

	data, err := vfs.ReadFile(fs, stackFile)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return nil, nil
		}

		return nil, FileReadError{FilePath: stackFile, Err: err}
	}

	return ParseStackFile(fs, &ParseStackFileInput{
		Src:      data,
		Filename: stackFileName,
		StackDir: stackDir,
	})
}

// maxStackRecursionDepth bounds nested-stack expansion so a pathological tree (a path
// escaping via "..", or a symlink loop EvalSymlinks cannot canonicalize) cannot recurse
// without end. Real generated nesting is only a handful of levels deep.
const maxStackRecursionDepth = 1000

// StackFuncFactory builds the HCL function map used while decoding the stack file
// in a given stack directory. Each nesting level rebuilds the map for its own dir
// so dir-sensitive functions (get_terragrunt_dir, find_in_parent_folders,
// get_repo_root, run_cmd, get_working_dir) resolve against the nested dir, not the
// top stack dir. Production callers wrap config.EarlyStackParseFunctions; tests that
// exercise only literal attributes return an empty map.
type StackFuncFactory func(stackDir string) (map[string]function.Function, error)

// UnitPathsFromStackDir returns generated unit paths from discovery parsing. Nested stacks
// are expanded recursively so a stack composed of sub-stacks yields the sub-stacks' units.
// funcsFor builds the dir-scoped HCL function map for each stack directory visited; it
// must be non-nil and must return a non-nil map.
func UnitPathsFromStackDir(fs vfs.FS, stackDir string, funcsFor StackFuncFactory) ([]string, error) {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.UnitPathsFromStackDir: fs is nil (stackDir=%q)", stackDir))
	}

	if stackDir == "" {
		panic("hclparse.UnitPathsFromStackDir: stackDir is empty")
	}

	if funcsFor == nil {
		panic(fmt.Sprintf("hclparse.UnitPathsFromStackDir: funcsFor is nil (stackDir=%q)", stackDir))
	}

	return unitPathsFromStackDir(fs, stackDir, funcsFor, make(map[string]struct{}), 0)
}

// unitPathsFromStackDir is the bounded recursive worker. Termination is guaranteed two ways:
// visited skips any stack dir already expanded on this traversal (catches "." / ".." and
// ancestor symlink loops), and depth caps the chain length (backstop for symlink cycles
// EvalSymlinks reports as errors and therefore cannot collapse to a seen path).
func unitPathsFromStackDir(fs vfs.FS, stackDir string, funcsFor StackFuncFactory, visited map[string]struct{}, depth int) ([]string, error) {
	if depth > maxStackRecursionDepth {
		return nil, StackRecursionDepthExceededError{MaxDepth: maxStackRecursionDepth, StackDir: stackDir}
	}

	stackDir = util.ResolvePath(stackDir)

	if _, seen := visited[stackDir]; seen {
		return nil, nil
	}

	visited[stackDir] = struct{}{}

	stackFile := filepath.Join(stackDir, stackFileName)

	// Rebuild the function map for this dir so dir-sensitive functions resolve against it.
	funcs, err := funcsFor(stackDir)
	if err != nil {
		return nil, err
	}

	if funcs == nil {
		panic(fmt.Sprintf("hclparse.UnitPathsFromStackDir: funcsFor returned a nil map (stackDir=%q)", stackDir))
	}

	units, stacks, err := decodeDiscovery(fs, stackDir, stackFile, funcs)
	if err != nil {
		return nil, err
	}

	if len(units) == 0 && len(stacks) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, len(units))

	for _, unit := range units {
		paths = append(paths, unit.GeneratedPath(stackDir))
	}

	// Recurse into nested stacks so a stack composed of sub-stacks expands to the units they generate.
	for _, stack := range stacks {
		nestedDir := GeneratedComponentPath(stackDir, stack.Path, stack.NoStack != nil && *stack.NoStack)

		nestedPaths, nestedErr := unitPathsFromStackDir(fs, nestedDir, funcsFor, visited, depth+1)
		if nestedErr != nil {
			return nil, nestedErr
		}

		paths = append(paths, nestedPaths...)
	}

	return paths, nil
}

// decodeDiscovery parses discovery targets and returns path-only unit and stack data.
//
// funcs is the function map injected into the discovery eval context; callers
// must supply a non-nil map (validated at the public entrypoint).
func decodeDiscovery(fs vfs.FS, stackDir, stackFile string, funcs map[string]function.Function) ([]*unitPathOnlyHCL, []*stackPathOnlyHCL, error) {
	data, err := vfs.ReadFile(fs, stackFile)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return nil, nil, nil
		}

		return nil, nil, FileReadError{FilePath: stackFile, Err: err}
	}

	parsedFile, err := parseStackFileRoot(data, stackFile)
	if err != nil {
		return nil, nil, err
	}

	evalCtx := &hcl.EvalContext{
		Functions: funcs,
		Variables: map[string]cty.Value{},
	}

	// Load the sibling terragrunt.values.hcl written by stack generation and publish it
	// as the `values` variable, so locals (and unit/stack attributes) referencing
	// values.* resolve during discovery exactly as they do in the full stack parse.
	// An absent file leaves the variable unset, matching a stack that received no values.
	values, err := readDiscoveryValues(fs, stackDir, funcs)
	if err != nil {
		return nil, nil, err
	}

	if values != nil {
		evalCtx.Variables[varValues] = *values
	}

	if parsedFile.Locals != nil {
		if err := evaluateLocals(parsedFile.Locals.Remain, evalCtx); err != nil {
			return nil, nil, err
		}
	}

	srcByFilename := map[string][]byte{stackFile: data}

	mergedRemain, err := mergeIncludes(fs, parsedFile, stackDir, evalCtx, srcByFilename)
	if err != nil {
		return nil, nil, err
	}

	decoded := &discoveryDecode{}
	if diags := gohcl.DecodeBody(mergedRemain, evalCtx, decoded); diags.HasErrors() {
		return nil, nil, FileDecodeError{Name: stackFile, Err: diags}
	}

	// Reject duplicate names within the base stack file itself before the autoinclude override merge, so an
	// override targeting a duplicated name cannot mask a base-file duplicate. The full parse rejects this at
	// processStackConfigIncludes, before its own autoinclude merge; discovery must match that ordering.
	if err := validateDiscoveryUniqueNames(decoded.Units, decoded.Stacks); err != nil {
		return nil, nil, err
	}

	// Publish the base unit.<name>.path / stack.<name>.path refs before decoding the autoinclude, so a sibling
	// autoinclude block whose path references them resolves during discovery exactly as it does in the full
	// stack parse (injectStackComponentRefs). Without this, discovery would reject a config the full parse accepts.
	evalCtx.Variables[VarUnit] = BuildComponentRefMap(buildDiscoveryUnitRefs(decoded.Units, stackDir))
	evalCtx.Variables[VarStack] = BuildComponentRefMap(buildDiscoveryStackRefs(decoded.Stacks, stackDir))

	// Merge units and stacks injected by a sibling terragrunt.autoinclude.stack.hcl, overriding same-name
	// base blocks the same way a full stack parse does. The autoinclude file's own names are validated for
	// uniqueness inside the merge.
	if err := mergeDiscoveryStackAutoInclude(fs, stackDir, evalCtx, decoded); err != nil {
		return nil, nil, err
	}

	// The merged set must still be unique. This is defensive: MergeNamed already produces unique names from
	// two individually-unique inputs, but the check guards future changes to the merge.
	if err := validateDiscoveryUniqueNames(decoded.Units, decoded.Stacks); err != nil {
		return nil, nil, err
	}

	return decoded.Units, decoded.Stacks, nil
}

// readDiscoveryValues reads the generated terragrunt.values.hcl next to a stack file
// and decodes its top-level attributes into a single HCL object, mirroring how
// pkg/config's ReadValues interprets the file during a full stack parse. An absent
// file returns (nil, nil) so the caller leaves the `values` variable unset; an
// existing but empty file returns an empty object, matching the full parse.
//
// funcs is the same dir-scoped function map used for the rest of discovery, so any
// function call in a (normally literal-only, generated) values file resolves
// against the stack directory being expanded.
func readDiscoveryValues(fs vfs.FS, stackDir string, funcs map[string]function.Function) (*cty.Value, error) {
	valuesPath := filepath.Join(stackDir, valuesFileName)

	data, err := vfs.ReadFile(fs, valuesPath)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return nil, nil
		}

		return nil, FileReadError{FilePath: valuesPath, Err: err}
	}

	file, parseDiags := hclsyntax.ParseConfig(data, valuesPath, hcl.Pos{Line: 1, Column: 1})
	if parseDiags.HasErrors() {
		return nil, FileParseError{FilePath: valuesPath, Err: parseDiags}
	}

	evalCtx := &hcl.EvalContext{
		Functions: funcs,
		Variables: map[string]cty.Value{},
	}

	// Decoding into a map evaluates every top-level attribute and rejects blocks,
	// the same shape pkg/config's ReadValues decodes.
	values := map[string]cty.Value{}
	if diags := gohcl.DecodeBody(file.Body, evalCtx, &values); diags.HasErrors() {
		return nil, FileDecodeError{Name: valuesPath, Err: diags}
	}

	result := cty.ObjectVal(values)

	return &result, nil
}

// mergeDiscoveryStackAutoInclude merges the units and stacks declared by a sibling
// terragrunt.autoinclude.stack.hcl, overriding same-name base blocks and appending new ones, so discovery
// expands the same components a full stack parse materializes via mergeStackAutoIncludeFile. An absent
// autoinclude file is a no-op. Discovery applies the same rejections as the full parse so it never adds DAG
// edges the full parse would reject: the dependency-values backstop, and a strict decode that allows only
// unit and stack blocks at the top level.
func mergeDiscoveryStackAutoInclude(fs vfs.FS, stackDir string, evalCtx *hcl.EvalContext, decoded *discoveryDecode) error {
	autoIncludePath := filepath.Join(stackDir, AutoIncludeStackFile)

	data, err := vfs.ReadFile(fs, autoIncludePath)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return nil
		}

		return FileReadError{FilePath: autoIncludePath, Err: err}
	}

	file, parseDiags := hclsyntax.ParseConfig(data, autoIncludePath, hcl.Pos{Line: 1, Column: 1})
	if parseDiags.HasErrors() {
		return FileParseError{FilePath: autoIncludePath, Err: parseDiags}
	}

	syntaxBody, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return UnexpectedBodyTypeError{FilePath: autoIncludePath}
	}

	// Run the same dep-values backstop as the full parse so a stale autoinclude whose injected values
	// reference dependency outputs is rejected, not silently turned into discovery DAG edges.
	if typed := StackAutoIncludeDepValuesError(syntaxBody, filepath.Base(stackDir)); typed != nil {
		return *typed
	}

	// Strict decode rejecting any top-level content other than unit and stack blocks (the struct has no ",remain").
	autoDecoded := &discoveryAutoIncludeDecode{}
	if diags := gohcl.DecodeBody(file.Body, evalCtx, autoDecoded); diags.HasErrors() {
		return FileDecodeError{Name: autoIncludePath, Err: diags}
	}

	// Reject duplicate names within the autoinclude file itself, mirroring the base-file rejection, so a
	// stale or hand-edited autoinclude cannot silently collapse two same-name blocks into one.
	if err := validateDiscoveryUniqueNames(autoDecoded.Units, autoDecoded.Stacks); err != nil {
		return err
	}

	// A same-name injected unit/stack overrides the base block wholesale, matching the full stack parse.
	decoded.Units = util.MergeNamed(decoded.Units, autoDecoded.Units, unitPathName)
	decoded.Stacks = util.MergeNamed(decoded.Stacks, autoDecoded.Stacks, stackPathName)

	return nil
}

// buildDiscoveryUnitRefs builds the unit.<name>.path refs from the discovery unit decode.
func buildDiscoveryUnitRefs(units []*unitPathOnlyHCL, stackDir string) []ComponentRef {
	refs := make([]ComponentRef, 0, len(units))

	for _, u := range units {
		if u == nil {
			continue
		}

		refs = append(refs, ComponentRef{Name: u.Name, Path: u.GeneratedPath(stackDir)})
	}

	return refs
}

// buildDiscoveryStackRefs builds the stack.<name>.path refs from the discovery stack decode.
func buildDiscoveryStackRefs(stacks []*stackPathOnlyHCL, stackDir string) []ComponentRef {
	refs := make([]ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		if s == nil {
			continue
		}

		refs = append(refs, ComponentRef{Name: s.Name, Path: s.GeneratedPath(stackDir)})
	}

	return refs
}

// unitPathName returns a discovery unit's block name, or an empty string for a nil entry so MergeNamed leaves it untouched.
func unitPathName(u *unitPathOnlyHCL) string {
	if u == nil {
		return ""
	}

	return u.Name
}

// stackPathName returns a discovery stack's block name, or an empty string for a nil entry so MergeNamed leaves it untouched.
func stackPathName(s *stackPathOnlyHCL) string {
	if s == nil {
		return ""
	}

	return s.Name
}

// validateDiscoveryUniqueNames reports duplicate unit and stack names from the path-only discovery decode.
func validateDiscoveryUniqueNames(units []*unitPathOnlyHCL, stacks []*stackPathOnlyHCL) error {
	seenUnits := make(map[string]struct{}, len(units))
	seenStacks := make(map[string]struct{}, len(stacks))

	var errs []error

	for _, unit := range units {
		if _, ok := seenUnits[unit.Name]; ok {
			errs = append(errs, DuplicateUnitNameError{Name: unit.Name})
			continue
		}

		seenUnits[unit.Name] = struct{}{}
	}

	for _, stack := range stacks {
		if _, ok := seenStacks[stack.Name]; ok {
			errs = append(errs, DuplicateStackNameError{Name: stack.Name})
			continue
		}

		seenStacks[stack.Name] = struct{}{}
	}

	return errors.Join(errs...)
}
