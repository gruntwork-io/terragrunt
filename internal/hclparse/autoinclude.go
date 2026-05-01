package hclparse

import (
	"fmt"
	iofs "io/fs"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// AutoIncludeHCL represents the first-phase parse of an autoinclude block.
// The entire body is captured as remain because it may contain references
// to unit.*.path / stack.*.path variables that are only available after
// the first parsing pass extracts all unit/stack names and paths.
//
// Example HCL:
//
//	autoinclude {
//	  dependency "vpc" {
//	    config_path = unit.vpc.path
//	  }
//	  inputs = {
//	    vpc_id = dependency.vpc.outputs.vpc_id
//	  }
//	}
type AutoIncludeHCL struct {
	Remain hcl.Body `hcl:",remain"`
}

// AutoIncludeResolved represents the second-phase resolved autoinclude content.
//
// After the first parse extracts unit/stack names and paths, the autoinclude
// body is partially evaluated:
//   - dependency.config_path is resolved (references unit.*.path)
//   - dependency remain (mock_outputs etc) is preserved for generation
//   - inputs and other blocks are partially evaluated (local.* resolved, dependency.* preserved)
//
// The RawBody is preserved for serializing the generated
// terragrunt.autoinclude.hcl file.
type AutoIncludeResolved struct {
	// EvalCtx is the HCL evaluation context used during resolution,
	// preserved so the generator can evaluate non-deferred expressions.
	EvalCtx *hcl.EvalContext
	// RawBody is the original autoinclude HCL body, preserved so
	// the generator can write non-dependency content (inputs, etc.)
	// directly from the AST without evaluating dependency.* references.
	RawBody hcl.Body
	// Kind is "unit" or "stack" — drives the generated filename (terragrunt.autoinclude.hcl vs terragrunt.autoinclude.stack.hcl).
	Kind         string
	Dependencies []AutoIncludeDependency
}

// AutoIncludeDependency represents a resolved dependency block from autoinclude.
// config_path has been evaluated (e.g. unit.vpc.path → "/abs/path/to/.terragrunt-stack/vpc").
// The original HCL block is preserved for writing all attributes (mock_outputs, etc.)
// into the generated file.
type AutoIncludeDependency struct {
	// Block is the original HCL block, preserved for serialization.
	Block      *hcl.Block
	Name       string
	ConfigPath string
}

// Resolve evaluates the autoinclude body using the provided eval context,
// which must contain unit.* and stack.* variables for path resolution.
//
// The resolution follows three levels:
//
//  1. First parse: autoinclude body captured as Remain (unit.*.path not yet available)
//  2. This method (second parse): dependency.config_path evaluated using unit/stack context.
//     All other dependency attributes (mock_outputs, etc.) are preserved as raw HCL.
//  3. inputs and other non-dependency content: NOT evaluated here.
//     They contain dependency.*.outputs.* which is runtime-only.
//     The RawBody is preserved so the generator can copy these from the AST.
func (a *AutoIncludeHCL) Resolve(evalCtx *hcl.EvalContext) (*AutoIncludeResolved, hcl.Diagnostics) {
	if a == nil || a.Remain == nil {
		return nil, nil
	}

	body, ok := a.Remain.(*hclsyntax.Body)
	if !ok {
		// Non-syntax body: return result with EvalCtx even though partial evaluation is not possible.
		return &AutoIncludeResolved{EvalCtx: evalCtx, RawBody: a.Remain}, nil
	}

	var (
		deps  []AutoIncludeDependency
		diags hcl.Diagnostics
	)

	for _, block := range body.Blocks {
		if block.Type != blockDependency || len(block.Labels) == 0 {
			continue
		}

		dep, depDiags := resolveDependencyBlock(block, evalCtx)
		diags = append(diags, depDiags...)

		if depDiags.HasErrors() {
			continue
		}

		deps = append(deps, dep)
	}

	if diags.HasErrors() {
		return nil, diags
	}

	return &AutoIncludeResolved{
		EvalCtx:      evalCtx,
		Dependencies: deps,
		RawBody:      a.Remain,
	}, nil
}

// resolveDependencyBlock extracts config_path from a dependency block
// by evaluating it against the eval context (which has unit/stack paths).
// The full block is preserved for generation.
func resolveDependencyBlock(block *hclsyntax.Block, evalCtx *hcl.EvalContext) (AutoIncludeDependency, hcl.Diagnostics) {
	name := block.Labels[0]

	// Decode only config_path from the block body, leaving everything else.
	configPathAttr, exists := block.Body.Attributes[attrConfigPath]
	if !exists {
		return AutoIncludeDependency{}, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Missing config_path",
			Detail:   "dependency block must have a config_path attribute",
			Subject:  block.DefRange().Ptr(),
		}}
	}

	val, diags := configPathAttr.Expr.Value(evalCtx)
	if diags.HasErrors() || !val.IsKnown() || val.IsNull() {
		return AutoIncludeDependency{}, diags
	}

	if val.Type() != cty.String {
		return AutoIncludeDependency{}, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid config_path type",
			Detail:   "dependency config_path must evaluate to a string",
			Subject:  configPathAttr.Expr.Range().Ptr(),
		}}
	}

	return AutoIncludeDependency{
		Name:       name,
		ConfigPath: val.AsString(),
		Block:      block.AsHCLBlock(),
	}, nil
}

// BuildAutoIncludeEvalContext creates an HCL evaluation context with
// unit and stack path references for resolving autoinclude blocks.
//
// The context provides:
//   - unit.<name>.path - resolved path of each unit in the stack
//   - unit.<name>.name - name label of each unit
//   - stack.<name>.path - resolved path of each stack in the stack
//   - stack.<name>.name - name label of each stack
//
// Additional variables (locals, values) can be merged into the returned
// context by the caller.
func BuildAutoIncludeEvalContext(unitRefs, stackRefs []ComponentRef) *hcl.EvalContext {
	vars := map[string]cty.Value{
		varUnit:  BuildComponentRefMap(unitRefs),
		varStack: BuildComponentRefMap(stackRefs),
	}

	return &hcl.EvalContext{
		Variables: vars,
	}
}

// AutoIncludeDependencyPaths reads the terragrunt.autoinclude.hcl file in
// unitDir and returns resolved dependency config_path values.
// Returns (nil, nil) if the file does not exist or has no dependencies.
// Panics when fs is nil (programmer error). Returns EmptyArgError when unitDir
// is empty so callers can distinguish bad input from a missing file.
func AutoIncludeDependencyPaths(fs vfs.FS, unitDir string) ([]string, error) {
	if fs == nil {
		panic(fmt.Sprintf("hclparse.AutoIncludeDependencyPaths: fs is nil (unitDir=%q)", unitDir))
	}

	if unitDir == "" {
		return nil, EmptyArgError{Func: "AutoIncludeDependencyPaths", Arg: "unitDir"}
	}

	unitDir = util.ResolvePath(unitDir)
	autoIncludePath := filepath.Join(unitDir, AutoIncludeFile)

	body, err := readAutoIncludeBody(fs, autoIncludePath)
	if err != nil || body == nil {
		return nil, err
	}

	paths := make([]string, 0, len(body.Blocks))

	for _, block := range body.Blocks {
		if depPath, ok := extractDependencyConfigPath(block, unitDir); ok {
			paths = append(paths, depPath)
		}
	}

	return paths, nil
}

// readAutoIncludeBody reads and parses the autoinclude file at path.
// Returns (nil, nil) when the file does not exist.
func readAutoIncludeBody(fs vfs.FS, path string) (*hclsyntax.Body, error) {
	data, err := vfs.ReadFile(fs, path)
	if errors.Is(err, iofs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, FileReadError{FilePath: path, Err: err}
	}

	file, diags := hclsyntax.ParseConfig(data, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, FileParseError{FilePath: path, Detail: diags.Error()}
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, UnexpectedBodyTypeError{FilePath: path}
	}

	return body, nil
}

// extractDependencyConfigPath returns the resolved absolute config_path for a
// dependency block, or ("", false) if the block is not a valid dependency or
// config_path cannot be evaluated to a string.
//
// The nil eval context passed to Expr.Value is intentional: GenerateAutoIncludeFile
// always writes config_path as a literal quoted string via writeDependencyBlock
// (see generate.go), so no variable resolution is required. If that contract is
// ever relaxed to emit interpolations, callers must pass a real eval context
// here or the dependency will be silently dropped from the DAG.
func extractDependencyConfigPath(block *hclsyntax.Block, unitDir string) (string, bool) {
	if block.Type != blockDependency || len(block.Labels) == 0 {
		return "", false
	}

	configPathAttr, exists := block.Body.Attributes[attrConfigPath]
	if !exists {
		return "", false
	}

	val, diags := configPathAttr.Expr.Value(nil)
	if diags.HasErrors() || !val.IsKnown() || val.IsNull() || val.Type() != cty.String {
		return "", false
	}

	depPath := val.AsString()
	if !filepath.IsAbs(depPath) {
		depPath = filepath.Clean(filepath.Join(unitDir, depPath))
	}

	return util.ResolvePath(depPath), true
}
