package hclparse

import (
	"fmt"
	iofs "io/fs"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	tflang "github.com/hashicorp/terraform/lang"
	"github.com/zclconf/go-cty/cty"
)

// stackFileName is the canonical filename of a Terragrunt stack file.
const stackFileName = "terragrunt.stack.hcl"

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

// ComponentRef holds name, path, and child refs.
type ComponentRef struct {
	Name      string
	Path      string
	ChildRefs []ComponentRef
}

// BuildComponentRefMap converts component refs into an HCL object; empty input returns EmptyObjectVal so typos surface as "Unsupported attribute" diagnostics.
func BuildComponentRefMap(refs []ComponentRef) cty.Value {
	if len(refs) == 0 {
		return cty.EmptyObjectVal
	}

	refMap := make(map[string]cty.Value, len(refs))

	for _, ref := range refs {
		refMap[ref.Name] = buildRefAttrs(ref)
	}

	return cty.ObjectVal(refMap)
}

// buildRefAttrs converts one ComponentRef and nested refs recursively. The reserved attribute key "path" holds the component's own path; a nested child ref named "path" cannot be expressed in this namespace and is silently dropped (top-level components are not affected because the unit/stack map keys do not collide with "path").
func buildRefAttrs(ref ComponentRef) cty.Value {
	attrs := map[string]cty.Value{
		"path": cty.StringVal(ref.Path),
	}

	for _, child := range ref.ChildRefs {
		if child.Name == "path" {
			continue
		}

		attrs[child.Name] = buildRefAttrs(child)
	}

	return cty.ObjectVal(attrs)
}

// unitPathOnlyHCL is the discovery shape for unit name and path.
type unitPathOnlyHCL struct {
	Remain  hcl.Body `hcl:",remain"`
	NoStack *bool    `hcl:"no_dot_terragrunt_stack,optional"`
	Path    string   `hcl:"path,attr"`
	Name    string   `hcl:",label"`
}

// stackPathOnlyHCL is the discovery shape for stack name, path, and source. Source is a lazy hcl.Expression so a non-literal source (e.g. `${get_terragrunt_dir()}/...`) in a nested stack file does not block decode against the stdlib-only discovery eval context; the source is only evaluated when recursion needs it, and unresolvable sources skip recursion silently (consistent with best-effort discovery). Path is intentionally eager (string) - function calls in path are not exercised by any current fixture; if discovery ever needs to tolerate them, this type should be widened the same way Source was.
type stackPathOnlyHCL struct {
	Remain  hcl.Body       `hcl:",remain"`
	NoStack *bool          `hcl:"no_dot_terragrunt_stack,optional"`
	Source  hcl.Expression `hcl:"source,attr"`
	Path    string         `hcl:"path,attr"`
	Name    string         `hcl:",label"`
}

// discoveryDecode holds decoded unit and stack blocks for discovery.
type discoveryDecode struct {
	Remain hcl.Body            `hcl:",remain"`
	Stacks []*stackPathOnlyHCL `hcl:"stack,block"`
	Units  []*unitPathOnlyHCL  `hcl:"unit,block"`
}

// ParseStackFileFromPath reads a terragrunt.stack.hcl from disk and runs ParseStackFile against it; returns (nil, nil) if the file does not exist (caller-level discovery uses this signal to skip).
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

// UnitPathsFromStackDir returns generated unit paths from discovery parsing.
func UnitPathsFromStackDir(fs vfs.FS, stackDir string) ([]string, error) {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.UnitPathsFromStackDir: fs is nil (stackDir=%q)", stackDir))
	}

	if stackDir == "" {
		panic("hclparse.UnitPathsFromStackDir: stackDir is empty")
	}

	stackDir = util.ResolvePath(stackDir)
	stackFile := filepath.Join(stackDir, stackFileName)

	units, _, err := decodeDiscovery(fs, stackDir, stackFile)
	if err != nil {
		return nil, err
	}

	if units == nil {
		return nil, nil
	}

	paths := make([]string, 0, len(units))

	for _, unit := range units {
		unitPath := filepath.Join(stackDir, StackDir, unit.Path)
		if unit.NoStack != nil && *unit.NoStack {
			unitPath = filepath.Join(stackDir, unit.Path)
		}

		paths = append(paths, unitPath)
	}

	return paths, nil
}

// maxDiscoverDepth bounds recursion when walking nested stack catalogs during best-effort discovery; nested stack ref enrichment beyond this depth returns nil ChildRefs.
const maxDiscoverDepth = 1000

// DiscoverStackChildUnits parses child stack directories with best-effort behavior. Discovery enriches a stack ComponentRef with nested unit refs so `stack.<name>.<unit>.path` resolves at autoinclude eval time; it is NOT a source of truth. When a nested stack file cannot be read or parsed, the function returns nil ChildRefs rather than propagating the error. Any user reference to an undiscovered child surfaces later as a clear HCL "Unsupported attribute" diagnostic on the autoinclude expression, which is where the user can act on it.
func DiscoverStackChildUnits(fs vfs.FS, stackSourceDir, stackGenDir string) []ComponentRef {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.DiscoverStackChildUnits: fs is nil (stackSourceDir=%q, stackGenDir=%q)", stackSourceDir, stackGenDir))
	}

	if stackSourceDir == "" {
		panic(fmt.Sprintf("hclparse.DiscoverStackChildUnits: stackSourceDir is empty (stackGenDir=%q)", stackGenDir))
	}

	if stackGenDir == "" {
		panic(fmt.Sprintf("hclparse.DiscoverStackChildUnits: stackGenDir is empty (stackSourceDir=%q)", stackSourceDir))
	}

	return discoverStackChildUnitsWithDepth(fs, stackSourceDir, stackGenDir, 0)
}

func discoverStackChildUnitsWithDepth(fs vfs.FS, stackSourceDir, stackGenDir string, depth int) []ComponentRef {
	if depth > maxDiscoverDepth {
		return nil
	}

	stackFile := filepath.Join(stackSourceDir, stackFileName)

	units, stacks, err := decodeDiscovery(fs, stackSourceDir, stackFile)
	if err != nil || (units == nil && stacks == nil) {
		return nil
	}

	childTargetDir := filepath.Join(stackGenDir, StackDir)
	refs := make([]ComponentRef, 0, len(units)+len(stacks))

	for _, u := range units {
		unitPath := filepath.Join(childTargetDir, u.Path)
		if u.NoStack != nil && *u.NoStack {
			unitPath = filepath.Join(stackGenDir, u.Path)
		}

		refs = append(refs, ComponentRef{Name: u.Name, Path: unitPath})
	}

	for _, s := range stacks {
		nestedGenPath := filepath.Join(childTargetDir, s.Path)
		if s.NoStack != nil && *s.NoStack {
			nestedGenPath = filepath.Join(stackGenDir, s.Path)
		}

		ref := ComponentRef{Name: s.Name, Path: nestedGenPath}

		// Recursion eligibility: accept either a plain string literal (fast path) or an expression that evaluates against the Terraform stdlib eval context. Terragrunt-only functions and parser-owned namespaces (local/values/unit/stack) cannot be resolved here and silently skip recursion; the autoinclude/run-all paths surface the missing ref later as an "Unsupported attribute" diagnostic. Remote sources (go-getter URLs like `git::`, `https://`, ...) fall through to discoverStackChildUnitsWithDepth, where vfs.ReadFile yields iofs.ErrNotExist on the synthesized path and discovery silently returns nil.
		if nestedSourceDir, ok := resolveStackSource(s.Source, stackSourceDir); ok {
			if !filepath.IsAbs(nestedSourceDir) {
				nestedSourceDir = filepath.Join(stackSourceDir, nestedSourceDir)
			}

			ref.ChildRefs = discoverStackChildUnitsWithDepth(fs, nestedSourceDir, nestedGenPath, depth+1)
		}

		refs = append(refs, ref)
	}

	return refs
}

// decodeDiscovery parses discovery targets and returns path-only unit and stack data.
func decodeDiscovery(fs vfs.FS, stackDir, stackFile string) ([]*unitPathOnlyHCL, []*stackPathOnlyHCL, error) {
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

	evalCtx := stdlibEvalContext(stackDir)

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

	if err := validateDiscoveryUniqueNames(decoded.Units, decoded.Stacks); err != nil {
		return nil, nil, err
	}

	return decoded.Units, decoded.Stacks, nil
}

// validateDiscoveryUniqueNames reports duplicate unit and stack names from the path-only discovery decode. The check is duplicated here (sibling of validateUniqueNames in parse.go) because discovery uses its own decode shape (unitPathOnlyHCL / stackPathOnlyHCL with a lazy hcl.Expression source) that does not satisfy the unitsAndStacksHCL signature; without this guard a duplicate-named stack would silently overwrite the earlier entry in BuildComponentRefMap, masking the conflict from autoinclude eval.
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

// resolveStackSource returns the source string for nested stack discovery. Accepts plain string literals via literalString, and falls back to evaluating expr against the Terraform stdlib eval context anchored at baseDir so `format(...)`, `replace(...)`, `pathexpand(...)` and similar stdlib-only sources are also enriched. Terragrunt-specific functions (get_terragrunt_dir, find_in_parent_folders, etc.) are NOT available at this early phase, and parser-owned namespaces (local/values/unit/stack) require the full parse context; both cases return ("", false) so discovery silently skips the unresolvable nested stack and the missing ref surfaces later as a clear HCL diagnostic.
func resolveStackSource(expr hcl.Expression, baseDir string) (string, bool) {
	if s, ok := literalString(expr); ok {
		return s, true
	}

	if expr == nil {
		return "", false
	}

	val, diags := expr.Value(stdlibEvalContext(baseDir))
	if diags.HasErrors() || val.IsNull() || !val.IsKnown() || val.Type() != cty.String {
		return "", false
	}

	return val.AsString(), true
}

// literalString returns (val, true) only if expr is a plain string literal - no variable refs, no function calls. Templates and ternaries (even constant-foldable ones) return ("", false) because Value(nil) errors without an eval context; discovery callers then skip recursion into that nested stack.
func literalString(expr hcl.Expression) (string, bool) {
	if expr == nil {
		return "", false
	}

	if len(expr.Variables()) > 0 {
		return "", false
	}

	val, diags := expr.Value(nil)
	if diags.HasErrors() || val.IsNull() || !val.IsKnown() || val.Type() != cty.String {
		return "", false
	}

	return val.AsString(), true
}

// stdlibEvalContext returns a stdlib-only eval context for discovery; parser-owned namespaces are intentionally out of scope (non-literal source/path is skipped via literalString).
func stdlibEvalContext(baseDir string) *hcl.EvalContext {
	tfscope := tflang.Scope{BaseDir: baseDir}

	return &hcl.EvalContext{
		Functions: tfscope.Functions(),
		Variables: map[string]cty.Value{},
	}
}
