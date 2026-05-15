package hclparse

import (
	"fmt"
	iofs "io/fs"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	tflang "github.com/hashicorp/terraform/lang"
	"github.com/zclconf/go-cty/cty"
)

// StackFileName is the canonical filename of a Terragrunt stack file.
const StackFileName = "terragrunt.stack.hcl"

// StackFileHCL is the Phase 1 slurp of a terragrunt.stack.hcl file. Only the locals block, include blocks, and the rest of the body (Remain) are captured at this stage. Unit/stack blocks live inside Remain and are decoded later against a populated eval context.
type StackFileHCL struct {
	Remain   hcl.Body           `hcl:",remain"`
	Locals   *LocalsHCL         `hcl:"locals,block"`
	Includes []*StackIncludeHCL `hcl:"include,block"`
}

// LocalsHCL is the slurp of a locals block; its body is decoded later by the DAG-based locals evaluator.
type LocalsHCL struct {
	Remain hcl.Body `hcl:",remain"`
}

// StackIncludeHCL represents an include block. Path is captured as a lazy expression so it can reference `local.X` defined in the same file; it is evaluated after locals are resolved.
type StackIncludeHCL struct {
	Path hcl.Expression `hcl:"path,attr"`
	Name string         `hcl:",label"`
}

// unitsAndStacksHCL is the Phase 3 decode of Remain. Unit/stack fields are eager Go types because the eval context is populated (functions, variables, locals) before this decode runs.
type unitsAndStacksHCL struct {
	Remain hcl.Body         `hcl:",remain"`
	Stacks []*StackBlockHCL `hcl:"stack,block"`
	Units  []*UnitBlockHCL  `hcl:"unit,block"`
}

// UnitBlockHCL is the eager-decode shape of a unit block. AutoInclude.Remain stays lazy because its body can reference unit.*/stack.*/dependency.* which only resolve in later phases.
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

// StackBlockHCL is the eager-decode shape of a stack block. See UnitBlockHCL for AutoInclude rationale.
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

// ComponentRef holds the resolved path and name metadata for a unit or stack block. ChildRefs holds nested unit refs for stack components so stack.<name>.<unit>.path works at any nesting depth.
type ComponentRef struct {
	Name      string
	Path      string
	ChildRefs []ComponentRef
}

// BuildComponentRefMap creates a cty.Value object from a slice of ComponentRef for injection as the `unit` or `stack` HCL variable.
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

// buildRefAttrs builds the cty.Value for a single ComponentRef, recursively expanding ChildRefs so stack.A.B.C.path works at any depth. Recursion is bounded by maxDiscoverDepth at construction time.
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

// unitPathOnlyHCL is the discovery-only decode shape. It captures just the unit name and path; source/no_*/values/autoinclude are absorbed into Remain so generated nested stack files whose source contains terragrunt function calls still decode against a stdlib-only eval context.
type unitPathOnlyHCL struct {
	Remain  hcl.Body `hcl:",remain"`
	NoStack *bool    `hcl:"no_dot_terragrunt_stack,optional"`
	Path    string   `hcl:"path,attr"`
	Name    string   `hcl:",label"`
}

// stackPathOnlyHCL is the discovery-only decode shape for stack blocks. Source is captured here (unlike unit discovery) because nested-stack discovery needs to descend into the stack's source dir to enumerate its child units.
type stackPathOnlyHCL struct {
	Remain  hcl.Body `hcl:",remain"`
	NoStack *bool    `hcl:"no_dot_terragrunt_stack,optional"`
	Path    string   `hcl:"path,attr"`
	Source  string   `hcl:"source,attr"`
	Name    string   `hcl:",label"`
}

// discoveryDecode is the discovery slurp container for unit/stack blocks under Remain.
type discoveryDecode struct {
	Remain hcl.Body            `hcl:",remain"`
	Stacks []*stackPathOnlyHCL `hcl:"stack,block"`
	Units  []*unitPathOnlyHCL  `hcl:"unit,block"`
}

// ParseStackFileFromPath reads stackDir/terragrunt.stack.hcl from disk and performs a full parse for discovery use cases. Returns (nil, nil) when the stack file does not exist.
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

// UnitPathsFromStackDir parses the stack file in stackDir and returns the absolute generated path of each declared unit. Used by `terragrunt run --all` discovery on generated nested stacks. Discovery uses a path-only decode shape so source attributes that reference terragrunt-only functions do not block path resolution.
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

// DiscoverStackChildUnits parses a stack's source dir for stack.<name>.<unit>.path resolution. Best-effort: nested parse failures yield empty refs, never an error.
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

// decodeDiscovery slurps the stack file, evaluates locals, merges includes (same order as ParseStackFile), then decodes unit/stack blocks into path-only shapes. Returns (nil, nil, nil) when the stack file does not exist.
func decodeDiscovery(fs vfs.FS, stackDir, stackFile string) ([]*unitPathOnlyHCL, []*stackPathOnlyHCL, error) {
	data, err := vfs.ReadFile(fs, stackFile)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return nil, nil, nil
		}

		return nil, nil, FileReadError{FilePath: stackFile, Err: err}
	}

	slurp, err := slurpStackFile(data, stackFile)
	if err != nil {
		return nil, nil, err
	}

	evalCtx := stdlibEvalContext(stackDir)

	if slurp.Locals != nil {
		if err := evaluateLocals(slurp.Locals.Remain, evalCtx); err != nil {
			return nil, nil, err
		}
	}

	srcByFilename := map[string][]byte{stackFile: data}

	mergedRemain, err := mergeIncludes(fs, slurp, stackDir, evalCtx, srcByFilename)
	if err != nil {
		return nil, nil, err
	}

	decoded := &discoveryDecode{}
	if diags := decodeRemain(mergedRemain, evalCtx, decoded); diags != nil {
		return nil, nil, FileDecodeError{Name: stackFile, Detail: diags.Error(), Err: diags}
	}

	return decoded.Units, decoded.Stacks, nil
}

// stdlibEvalContext builds a minimal eval context wired with the terraform stdlib (matching the production parser's tflang.Scope setup in pkg/config/config_helpers.go). Used by discovery where no production eval context is available. Terragrunt path helpers are intentionally not bound here; expressions referencing them surface a clear evaluation error.
func stdlibEvalContext(baseDir string) *hcl.EvalContext {
	tfscope := tflang.Scope{BaseDir: baseDir}

	return &hcl.EvalContext{
		Functions: tfscope.Functions(),
		Variables: map[string]cty.Value{},
	}
}
