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

// UnitPathsFromStackDir returns generated unit paths from discovery parsing.
// funcs is the HCL function map used while decoding the stack file; production
// callers build it via config.EarlyStackParseFunctions. The map must be
// non-nil but may be empty (tests that exercise only literal attributes can
// pass an empty map).
func UnitPathsFromStackDir(fs vfs.FS, stackDir string, funcs map[string]function.Function) ([]string, error) {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.UnitPathsFromStackDir: fs is nil (stackDir=%q)", stackDir))
	}

	if stackDir == "" {
		panic("hclparse.UnitPathsFromStackDir: stackDir is empty")
	}

	if funcs == nil {
		panic(fmt.Sprintf("hclparse.UnitPathsFromStackDir: funcs is nil (stackDir=%q)", stackDir))
	}

	stackDir = util.ResolvePath(stackDir)
	stackFile := filepath.Join(stackDir, stackFileName)

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
