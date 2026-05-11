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
// Path is captured as a lazy expression so non-literal expressions (e.g. format(...)) are evaluated only when the include is processed.
type StackIncludeHCL struct {
	Path hcl.Expression `hcl:"path,attr"`
	Name string         `hcl:",label"`
}

// UnitBlockHCL represents the first-phase parse of a unit block. Source and Path are captured as lazy expressions so non-literal expressions in unrelated unit attributes do not block decoding; callers evaluate them via EvalString when needed. Remain absorbs every other attribute (notably `values = {...}`, which the production parser handles via pkg/config.Unit.Values) so gohcl decoding does not reject them.
type UnitBlockHCL struct {
	Remain       hcl.Body        `hcl:",remain"`
	AutoInclude  *AutoIncludeHCL `hcl:"autoinclude,block"`
	NoStack      *bool           `hcl:"no_dot_terragrunt_stack,optional"`
	NoValidation *bool           `hcl:"no_validation,optional"`
	Source       hcl.Expression  `hcl:"source,optional"`
	Path         hcl.Expression  `hcl:"path,optional"`
	Name         string          `hcl:",label"`
}

// StackBlockHCL represents the first-phase parse of a stack block. Source and Path are captured as lazy expressions; see UnitBlockHCL. Remain absorbs every other attribute (notably `values = {...}`, which the production parser handles via pkg/config.Stack.Values).
type StackBlockHCL struct {
	Remain       hcl.Body        `hcl:",remain"`
	AutoInclude  *AutoIncludeHCL `hcl:"autoinclude,block"`
	NoStack      *bool           `hcl:"no_dot_terragrunt_stack,optional"`
	NoValidation *bool           `hcl:"no_validation,optional"`
	Source       hcl.Expression  `hcl:"source,optional"`
	Path         hcl.Expression  `hcl:"path,optional"`
	Name         string          `hcl:",label"`
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

// ExtractUnitRefs extracts ComponentRef values from parsed UnitBlockHCL slices. evalCtx is used to evaluate each unit's lazy Path expression; pass nil for literal-only paths.
func ExtractUnitRefs(units []*UnitBlockHCL, evalCtx *hcl.EvalContext) []ComponentRef {
	refs := make([]ComponentRef, 0, len(units))

	for _, u := range units {
		path, err := EvalString(u.Path, evalCtx, attrPath)
		if err != nil {
			continue
		}

		refs = append(refs, ComponentRef{
			Name: u.Name,
			Path: path,
		})
	}

	return refs
}

// ExtractStackRefs extracts ComponentRef values from parsed StackBlockHCL slices. evalCtx is used to evaluate each stack's lazy Path expression; pass nil for literal-only paths.
func ExtractStackRefs(stacks []*StackBlockHCL, evalCtx *hcl.EvalContext) []ComponentRef {
	refs := make([]ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		path, err := EvalString(s.Path, evalCtx, attrPath)
		if err != nil {
			continue
		}

		refs = append(refs, ComponentRef{
			Name: s.Name,
			Path: path,
		})
	}

	return refs
}

// ParseStackFileFromPath reads stackDir/terragrunt.stack.hcl from disk and performs a two-pass parse. Returns (nil, nil) only when the stack file does not exist. Callers that may pass non-directory paths must filter those before calling.
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

// UnitPathsFromStackDir parses the stack file in stackDir and returns paths to each unit's generated directory. Returns (nil, err) on parse errors so callers can distinguish "not a stack dir" from "malformed stack file". Evaluates each unit's lazy Path expression against the terraform stdlib so generated stack files containing terragrunt function calls still resolve (units whose Path cannot be evaluated are skipped, best-effort).
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

	evalCtx := stdlibEvalContext(stackDir)

	paths := make([]string, 0, len(result.Units))

	for _, unit := range result.Units {
		unitRelPath, evalErr := EvalString(unit.Path, evalCtx, attrPath)
		if evalErr != nil {
			continue
		}

		unitPath := filepath.Join(stackDir, StackDir, unitRelPath)

		if unit.NoStack != nil && *unit.NoStack {
			unitPath = filepath.Join(stackDir, unitRelPath)
		}

		paths = append(paths, unitPath)
	}

	return paths, nil
}

// maxDiscoverDepth is the maximum recursion depth for DiscoverStackChildUnits
// to prevent infinite loops from circular stack references.
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

	result, err := ParseStackFileFromPath(fs, stackSourceDir)
	if err != nil || result == nil {
		// Nested-stack discovery is intentionally best-effort: it only enriches chained `stack.<name>.<unit>.path` refs. Any user reference to an undiscovered child surfaces later as an HCL eval diagnostic.
		return nil
	}

	evalCtx := stdlibEvalContext(stackSourceDir)

	childTargetDir := filepath.Join(stackGenDir, StackDir)
	refs := make([]ComponentRef, 0, len(result.Units)+len(result.Stacks))

	for _, u := range result.Units {
		unitRelPath, evalErr := EvalString(u.Path, evalCtx, attrPath)
		if evalErr != nil {
			continue
		}

		unitPath := filepath.Join(childTargetDir, unitRelPath)

		if u.NoStack != nil && *u.NoStack {
			unitPath = filepath.Join(stackGenDir, unitRelPath)
		}

		refs = append(refs, ComponentRef{
			Name: u.Name,
			Path: unitPath,
		})
	}

	for _, s := range result.Stacks {
		stackRelPath, evalErr := EvalString(s.Path, evalCtx, attrPath)
		if evalErr != nil {
			continue
		}

		nestedGenPath := filepath.Join(childTargetDir, stackRelPath)

		if s.NoStack != nil && *s.NoStack {
			nestedGenPath = filepath.Join(stackGenDir, stackRelPath)
		}

		nestedSourceDir, sourceErr := EvalString(s.Source, evalCtx, attrSource)
		if sourceErr != nil {
			continue
		}

		if !filepath.IsAbs(nestedSourceDir) {
			nestedSourceDir = filepath.Join(stackSourceDir, nestedSourceDir)
		}

		refs = append(refs, ComponentRef{
			Name:      s.Name,
			Path:      nestedGenPath,
			ChildRefs: discoverStackChildUnitsWithDepth(fs, nestedSourceDir, nestedGenPath, depth+1),
		})
	}

	return refs
}

// stdlibEvalContext builds a minimal eval context wired with the terraform stdlib (matching the production parser's tflang.Scope setup in pkg/config/config_helpers.go). This lets stack files that reference terraform-stdlib functions (e.g. format, jsonencode) resolve in contexts where no pctx is available, such as discovery on generated stack files.
func stdlibEvalContext(baseDir string) *hcl.EvalContext {
	tfscope := tflang.Scope{BaseDir: baseDir}

	return &hcl.EvalContext{
		Functions: tfscope.Functions(),
		Variables: map[string]cty.Value{},
	}
}
