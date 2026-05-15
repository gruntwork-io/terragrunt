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

// StackFileName is the canonical filename of a Terragrunt stack file.
const StackFileName = "terragrunt.stack.hcl"

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

// BuildComponentRefMap converts component refs into an HCL object.
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

// buildRefAttrs converts one ComponentRef and nested refs recursively.
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

// unitPathOnlyHCL is the discovery shape for unit name and path.
type unitPathOnlyHCL struct {
	Remain  hcl.Body `hcl:",remain"`
	NoStack *bool    `hcl:"no_dot_terragrunt_stack,optional"`
	Path    string   `hcl:"path,attr"`
	Name    string   `hcl:",label"`
}

// stackPathOnlyHCL is the discovery shape for stack name, path, and source.
type stackPathOnlyHCL struct {
	Remain  hcl.Body `hcl:",remain"`
	NoStack *bool    `hcl:"no_dot_terragrunt_stack,optional"`
	Path    string   `hcl:"path,attr"`
	Source  string   `hcl:"source,attr"`
	Name    string   `hcl:",label"`
}

// discoveryDecode holds decoded unit and stack blocks for discovery.
type discoveryDecode struct {
	Remain hcl.Body            `hcl:",remain"`
	Stacks []*stackPathOnlyHCL `hcl:"stack,block"`
	Units  []*unitPathOnlyHCL  `hcl:"unit,block"`
}

// ParseStackFileFromPath parses terragrunt.stack.hcl for stack discovery.
func ParseStackFileFromPath(fs vfs.FS, stackDir string) (*ParseResult, error) {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.ParseStackFileFromPath: fs is nil (stackDir=%q)", stackDir))
	}

	if stackDir == "" {
		panic("hclparse.ParseStackFileFromPath: stackDir is empty")
	}

	stackDir = util.ResolvePath(stackDir)
	stackFile := filepath.Join(stackDir, StackFileName)

	data, err := vfs.ReadFile(fs, stackFile)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
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

// UnitPathsFromStackDir returns generated unit paths from discovery parsing.
func UnitPathsFromStackDir(fs vfs.FS, stackDir string) ([]string, error) {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.UnitPathsFromStackDir: fs is nil (stackDir=%q)", stackDir))
	}

	if stackDir == "" {
		panic("hclparse.UnitPathsFromStackDir: stackDir is empty")
	}

	stackDir = util.ResolvePath(stackDir)
	stackFile := filepath.Join(stackDir, StackFileName)

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

const maxDiscoverDepth = 1000

// DiscoverStackChildUnits parses child stack directories with best-effort behavior.
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

	stackFile := filepath.Join(stackSourceDir, StackFileName)

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

		var childRefs []ComponentRef

		if nestedSourceDir, ok := localStackSourceDir(s.Source, stackSourceDir); ok {
			childRefs = discoverStackChildUnitsWithDepth(fs, nestedSourceDir, nestedGenPath, depth+1)
		}

		refs = append(refs, ComponentRef{
			Name:      s.Name,
			Path:      nestedGenPath,
			ChildRefs: childRefs,
		})
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
		return nil, nil, FileDecodeError{Name: stackFile, Detail: diags.Error(), Err: diags}
	}

	return decoded.Units, decoded.Stacks, nil
}

// stdlibEvalContext returns a minimal Terraform stdlib eval context for discovery.
func stdlibEvalContext(baseDir string) *hcl.EvalContext {
	tfscope := tflang.Scope{BaseDir: baseDir}

	return &hcl.EvalContext{
		Functions: tfscope.Functions(),
		Variables: map[string]cty.Value{},
	}
}
