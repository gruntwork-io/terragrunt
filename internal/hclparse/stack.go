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
	"github.com/zclconf/go-cty/cty/function"
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

// BuildComponentRefMap converts component refs into an HCL object injected as the `unit` or `stack` variable in the eval context.
// Empty input returns EmptyObjectVal so typos surface as "Unsupported attribute" diagnostics.
//
// Output shape for units:
//
//	{
//	  "unit_name": { "name": "unit_name", "path": "../relative/path" }
//	}
//
// Output shape for stacks with discovered children:
//
//	{
//	  "stack_name": {
//	    "name": "stack_name",
//	    "path": "/abs/path",
//	    "unit_name": { "name": "unit_name", "path": "/abs/path/to/unit" }
//	  }
//	}
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

// buildRefAttrs converts one ComponentRef and nested refs recursively; reserved keys "name" and "path" hold the component's own values.
func buildRefAttrs(ref ComponentRef) cty.Value {
	attrs := map[string]cty.Value{
		"name": cty.StringVal(ref.Name),
		"path": cty.StringVal(ref.Path),
	}

	for _, child := range ref.ChildRefs {
		if child.Name == "name" || child.Name == "path" {
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

// stackPathOnlyHCL is the discovery shape for stack name, path, and source; Source is lazy so non-literal sources don't block decode.
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

// DiscoverOption configures the discovery entry points UnitPathsFromStackDir
// and DiscoverStackChildUnits.
type DiscoverOption func(*discoverOptions)

type discoverOptions struct {
	funcsForDir func(dir string) map[string]function.Function
}

func applyDiscoverOptions(opts []DiscoverOption) discoverOptions {
	var cfg discoverOptions

	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

// WithDiscoveryFunctions installs an HCL function map used while decoding the
// stack file. Without this option, expressions resolve only against the
// Terraform stdlib scoped to the stack directory.
func WithDiscoveryFunctions(funcs map[string]function.Function) DiscoverOption {
	return func(c *discoverOptions) {
		c.funcsForDir = func(string) map[string]function.Function { return funcs }
	}
}

// WithDiscoveryFunctionsForDir installs a per-directory factory for the HCL
// function map. DiscoverStackChildUnits invokes it once per nested catalog;
// UnitPathsFromStackDir invokes it once with stackDir.
func WithDiscoveryFunctionsForDir(funcsForDir func(dir string) map[string]function.Function) DiscoverOption {
	return func(c *discoverOptions) { c.funcsForDir = funcsForDir }
}

// UnitPathsFromStackDir returns generated unit paths from discovery parsing.
func UnitPathsFromStackDir(fs vfs.FS, stackDir string, opts ...DiscoverOption) ([]string, error) {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.UnitPathsFromStackDir: fs is nil (stackDir=%q)", stackDir))
	}

	if stackDir == "" {
		panic("hclparse.UnitPathsFromStackDir: stackDir is empty")
	}

	stackDir = util.ResolvePath(stackDir)
	stackFile := filepath.Join(stackDir, stackFileName)

	cfg := applyDiscoverOptions(opts)

	var funcs map[string]function.Function
	if cfg.funcsForDir != nil {
		funcs = cfg.funcsForDir(stackDir)
	}

	units, _, err := decodeDiscovery(fs, stackDir, stackFile, funcs)
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

// DiscoverStackChildUnits enriches a stack ComponentRef with nested unit refs for stack.<name>.<unit>.path resolution; best-effort, returns nil ChildRefs on any read/parse failure.
func DiscoverStackChildUnits(fs vfs.FS, stackSourceDir, stackGenDir string, opts ...DiscoverOption) []ComponentRef {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.DiscoverStackChildUnits: fs is nil (stackSourceDir=%q, stackGenDir=%q)", stackSourceDir, stackGenDir))
	}

	if stackSourceDir == "" {
		panic(fmt.Sprintf("hclparse.DiscoverStackChildUnits: stackSourceDir is empty (stackGenDir=%q)", stackGenDir))
	}

	if stackGenDir == "" {
		panic(fmt.Sprintf("hclparse.DiscoverStackChildUnits: stackGenDir is empty (stackSourceDir=%q)", stackSourceDir))
	}

	cfg := applyDiscoverOptions(opts)

	return discoverStackChildUnitsWithDepth(fs, stackSourceDir, stackGenDir, 0, cfg.funcsForDir)
}

func discoverStackChildUnitsWithDepth(fs vfs.FS, stackSourceDir, stackGenDir string, depth int, funcsForDir func(dir string) map[string]function.Function) []ComponentRef {
	if depth > maxDiscoverDepth {
		return nil
	}

	stackFile := filepath.Join(stackSourceDir, stackFileName)

	var funcs map[string]function.Function
	if funcsForDir != nil {
		funcs = funcsForDir(stackSourceDir)
	}

	units, stacks, err := decodeDiscovery(fs, stackSourceDir, stackFile, funcs)
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

		// Only recurse when the source expression resolves against the injected
		// eval context; unresolvable expressions skip recursion silently so the
		// full parse can surface a diagnostic with the real call site.
		if nestedSourceDir, ok := resolveStackSource(s.Source, stackSourceDir, funcs); ok {
			if !filepath.IsAbs(nestedSourceDir) {
				nestedSourceDir = filepath.Join(stackSourceDir, nestedSourceDir)
			}

			ref.ChildRefs = discoverStackChildUnitsWithDepth(fs, nestedSourceDir, nestedGenPath, depth+1, funcsForDir)
		}

		refs = append(refs, ref)
	}

	return refs
}

// decodeDiscovery parses discovery targets and returns path-only unit and stack data.
//
// funcs is the function map injected into the discovery eval context. Passing
// nil falls back to the Terraform stdlib resolved against stackDir.
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

	evalCtx := discoveryEvalContext(stackDir, funcs)

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

// resolveStackSource returns the source string for nested stack discovery; falls back to the
// injected discovery eval context (or stdlib-only when funcs is nil) against baseDir,
// otherwise ("", false).
func resolveStackSource(expr hcl.Expression, baseDir string, funcs map[string]function.Function) (string, bool) {
	if s, ok := literalString(expr); ok {
		return s, true
	}

	if expr == nil {
		return "", false
	}

	val, diags := expr.Value(discoveryEvalContext(baseDir, funcs))
	if diags.HasErrors() || val.IsNull() || !val.IsKnown() || val.Type() != cty.String {
		return "", false
	}

	return val.AsString(), true
}

// literalString returns (val, true) only when expr is a plain string literal.
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

// discoveryEvalContext returns the eval context used while decoding
// terragrunt.stack.hcl for discovery. When funcs is non-nil it is used as-is;
// otherwise the Terraform stdlib resolved against baseDir is used.
func discoveryEvalContext(baseDir string, funcs map[string]function.Function) *hcl.EvalContext {
	if funcs == nil {
		tfscope := tflang.Scope{BaseDir: baseDir}
		funcs = tfscope.Functions()
	}

	return &hcl.EvalContext{
		Functions: funcs,
		Variables: map[string]cty.Value{},
	}
}
