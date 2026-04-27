package hclparse

import (
	"fmt"
	iofs "io/fs"
	"path/filepath"
	"syscall"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// StackFileHCL represents the first-phase parse of a terragrunt.stack.hcl file.
// The autoinclude body inside each unit/stack block is captured as hcl.Body
// via remain, allowing deferred evaluation once unit/stack path variables
// are available.
type StackFileHCL struct {
	Locals   *LocalsHCL         `hcl:"locals,block"`
	Includes []*StackIncludeHCL `hcl:"include,block"`
	Stacks   []*StackBlockHCL   `hcl:"stack,block"`
	Units    []*UnitBlockHCL    `hcl:"unit,block"`
}

// StackIncludeHCL represents an include block in a terragrunt.stack.hcl file.
// The path is evaluated immediately during the first parse pass.
type StackIncludeHCL struct {
	Name string `hcl:",label"`
	Path string `hcl:"path,attr"`
}

// UnitBlockHCL represents the first-phase parse of a unit block.
// Known attributes are decoded directly. The autoinclude block body
// is captured in Remain for second-phase evaluation.
type UnitBlockHCL struct {
	AutoInclude  *AutoIncludeHCL `hcl:"autoinclude,block"`
	NoStack      *bool           `hcl:"no_dot_terragrunt_stack,attr"`
	NoValidation *bool           `hcl:"no_validation,attr"`
	Values       *cty.Value      `hcl:"values,attr"`
	Name         string          `hcl:",label"`
	Source       string          `hcl:"source,attr"`
	Path         string          `hcl:"path,attr"`
}

// StackBlockHCL represents the first-phase parse of a stack block.
// Same remain pattern as UnitBlockHCL.
type StackBlockHCL struct {
	AutoInclude  *AutoIncludeHCL `hcl:"autoinclude,block"`
	NoStack      *bool           `hcl:"no_dot_terragrunt_stack,attr"`
	NoValidation *bool           `hcl:"no_validation,attr"`
	Values       *cty.Value      `hcl:"values,attr"`
	Name         string          `hcl:",label"`
	Source       string          `hcl:"source,attr"`
	Path         string          `hcl:"path,attr"`
}

// LocalsHCL captures the locals block body for iterative evaluation.
type LocalsHCL struct {
	Remain hcl.Body `hcl:",remain"`
}

// ComponentRef holds the path and name metadata for a unit or stack block,
// used to build the evaluation context for the second parsing phase.
type ComponentRef struct {
	Name string
	Path string
	// ChildRefs holds nested unit refs for stack components.
	// When a stack block references a source with a terragrunt.stack.hcl,
	// the child units within that stack are parsed and stored here.
	// This enables stack.stack_name.unit_name.path references.
	ChildRefs []ComponentRef
}

// BuildComponentRefMap creates a cty.Value map from a slice of ComponentRef.
// The resulting value is an object like:
//
//	{
//	  "unit_name": { "path": "../relative/path", "name": "unit_name" }
//	}
//
// For stack refs with children, it also includes nested unit refs:
//
//	{
//	  "stack_name": {
//	    "path": "/abs/path",
//	    "name": "stack_name",
//	    "unit_name": { "path": "/abs/path/to/unit", "name": "unit_name" }
//	  }
//	}
//
// This is injected into the HCL eval context as the `unit` or `stack` variable.
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

// buildRefAttrs builds the cty.Value for a single ComponentRef, recursively
// expanding ChildRefs so that stack.A.B.C.path works at any nesting depth.
// Recursion is bounded by maxDiscoverDepth in discoverStackChildUnitsWithDepth
// which limits the depth of ChildRefs trees at construction time.
func buildRefAttrs(ref ComponentRef) cty.Value {
	attrs := map[string]cty.Value{
		"path": cty.StringVal(ref.Path),
		"name": cty.StringVal(ref.Name),
	}

	for _, child := range ref.ChildRefs {
		if child.Name == "path" || child.Name == "name" {
			continue
		}

		attrs[child.Name] = buildRefAttrs(child)
	}

	return cty.ObjectVal(attrs)
}

// ExtractUnitRefs extracts ComponentRef values from parsed UnitBlockHCL slices.
func ExtractUnitRefs(units []*UnitBlockHCL) []ComponentRef {
	refs := make([]ComponentRef, 0, len(units))

	for _, u := range units {
		refs = append(refs, ComponentRef{
			Name: u.Name,
			Path: u.Path,
		})
	}

	return refs
}

// ExtractStackRefs extracts ComponentRef values from parsed StackBlockHCL slices.
func ExtractStackRefs(stacks []*StackBlockHCL) []ComponentRef {
	refs := make([]ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		refs = append(refs, ComponentRef{
			Name: s.Name,
			Path: s.Path,
		})
	}

	return refs
}

// ParseStackFileFromPath reads stackDir/terragrunt.stack.hcl from disk
// and performs a two-pass parse. Returns (nil, nil) when no stack file is
// reachable at stackDir: this includes the file not existing and the path
// itself not being a directory (e.g. callers in discovery may pass an
// arbitrary dependency path that turns out to be a regular file).
func ParseStackFileFromPath(fs vfs.FS, stackDir string) (*ParseResult, error) {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.ParseStackFileFromPath: fs is nil (stackDir=%q)", stackDir))
	}

	if stackDir == "" {
		panic("hclparse.ParseStackFileFromPath: stackDir is empty")
	}

	stackDir = util.ResolvePath(stackDir)
	stackFile := filepath.Join(stackDir, "terragrunt.stack.hcl")

	data, err := vfs.ReadFile(fs, stackFile)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return nil, nil
		}

		return nil, FileReadError{FilePath: stackFile, Err: err}
	}

	return ParseStackFile(fs, &ParseStackFileInput{
		Src:      data,
		Filename: stackFile,
		StackDir: stackDir,
	})
}

// UnitPathsFromStackDir parses the stack file in stackDir and returns
// absolute paths to each unit's generated directory under .terragrunt-stack/.
// Returns (nil, nil) when the stack file does not exist; returns (nil, err)
// on parse errors so callers can distinguish "not a stack dir" from "malformed
// stack file" instead of silently treating the latter as a plain unit.
func UnitPathsFromStackDir(fs vfs.FS, stackDir string) ([]string, error) {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.UnitPathsFromStackDir: fs is nil (stackDir=%q)", stackDir))
	}

	if stackDir == "" {
		panic("hclparse.UnitPathsFromStackDir: stackDir is empty")
	}

	stackDir = util.ResolvePath(stackDir)

	result, err := ParseStackFileFromPath(fs, stackDir)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	paths := make([]string, 0, len(result.Units))
	for _, unit := range result.Units {
		unitPath := filepath.Join(stackDir, StackDir, unit.Path)

		if unit.NoStack != nil && *unit.NoStack {
			unitPath = filepath.Join(stackDir, unit.Path)
		}

		paths = append(paths, unitPath)
	}

	return paths, nil
}

// maxDiscoverDepth is the maximum recursion depth for DiscoverStackChildUnits
// to prevent infinite loops from circular stack references.
const maxDiscoverDepth = 1000

// DiscoverStackChildUnits parses a stack's source directory to find the
// terragrunt.stack.hcl within it and extracts unit paths. This enables
// stack.stack_name.unit_name.path references in autoinclude blocks.
//
// stackSourceDir is the directory where the stack's source files live
// (or will be generated). stackGenDir is the absolute path where this
// stack's units will be generated (.terragrunt-stack/stack_path/).
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

	result, err := ParseStackFileFromPath(fs, stackSourceDir)
	if err != nil || result == nil {
		return nil
	}

	childTargetDir := filepath.Join(stackGenDir, StackDir)
	refs := make([]ComponentRef, 0, len(result.Units)+len(result.Stacks))

	for _, u := range result.Units {
		unitPath := filepath.Join(childTargetDir, u.Path)

		if u.NoStack != nil && *u.NoStack {
			unitPath = filepath.Join(stackGenDir, u.Path)
		}

		refs = append(refs, ComponentRef{
			Name: u.Name,
			Path: unitPath,
		})
	}

	// Also discover nested stacks so stack.<name>.<nested_stack>.path works.
	for _, s := range result.Stacks {
		nestedGenPath := filepath.Join(childTargetDir, s.Path)

		if s.NoStack != nil && *s.NoStack {
			nestedGenPath = filepath.Join(stackGenDir, s.Path)
		}

		nestedSourceDir := s.Source
		if !filepath.IsAbs(nestedSourceDir) {
			nestedSourceDir = filepath.Join(stackSourceDir, nestedSourceDir)
		}

		ref := ComponentRef{
			Name:      s.Name,
			Path:      nestedGenPath,
			ChildRefs: discoverStackChildUnitsWithDepth(fs, nestedSourceDir, nestedGenPath, depth+1),
		}

		refs = append(refs, ref)
	}

	return refs
}
