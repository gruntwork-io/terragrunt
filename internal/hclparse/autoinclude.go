package hclparse

import (
	"fmt"
	iofs "io/fs"
	"path/filepath"
	"strings"

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
	// SourceBytes are the bytes of the file RawBody was parsed from. Generation slices expressions by HCL byte ranges and must use these bytes, not the root stack file's bytes, when the autoinclude originated in an included file.
	SourceBytes []byte
	// Values is the resolved `values = {...}` attribute; only valid for stack-kind autoincludes (unit-kind rejected in resolveAutoInclude). Propagated into the nested stack's terragrunt.values.hcl. Nil when absent.
	Values *cty.Value
	// Kind is KindUnit or KindStack and drives the generated filename (terragrunt.autoinclude.hcl vs terragrunt.autoinclude.stack.hcl).
	Kind         AutoIncludeKind
	Dependencies []AutoIncludeDependency
}

// AutoIncludeDependency represents a resolved dependency block from autoinclude.
// config_path has been evaluated (e.g. unit.vpc.path -> "/abs/path/to/.terragrunt-stack/vpc").
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
// Callers that need to record the originating file's bytes on the returned
// AutoIncludeResolved (so generation can slice expressions from the correct
// source after include merging) should set SourceBytes on the result.
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
		if block.Type != blockDependency {
			continue
		}

		if len(block.Labels) != 1 {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid dependency block labels",
				Detail:   fmt.Sprintf("dependency block requires exactly one label, got %d", len(block.Labels)),
				Subject:  block.DefRange().Ptr(),
			})

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

	// Evaluate the optional `values` attribute against an augmented eval context that binds dependency mock_outputs so expressions like `dependency.X.outputs.Y` resolve. Mock_outputs are the only dependency data available at parse time; at runtime the production parser will resolve dependencies for real if the generated autoinclude file is re-parsed there.
	resolvedValues, valDiags := resolveAutoIncludeValues(body, evalCtx, deps)
	diags = append(diags, valDiags...)

	if diags.HasErrors() {
		return nil, diags
	}

	return &AutoIncludeResolved{
		EvalCtx:      evalCtx,
		Dependencies: deps,
		RawBody:      a.Remain,
		Values:       resolvedValues,
	}, nil
}

// resolveAutoIncludeValues extracts and evaluates the optional `values = {...}` attribute from the autoinclude body. The eval context is augmented with `dependency.<name>.outputs = mock_outputs` for each declared dependency so values that reference dependency outputs resolve to the mock value at parse time. Returns (nil, nil) when no values attribute is present.
func resolveAutoIncludeValues(body *hclsyntax.Body, evalCtx *hcl.EvalContext, deps []AutoIncludeDependency) (*cty.Value, hcl.Diagnostics) {
	valuesAttr, ok := body.Attributes[attrValues]
	if !ok {
		return nil, nil
	}

	augmented, depDiags := augmentEvalCtxWithDeps(evalCtx, body, deps)
	if depDiags.HasErrors() {
		return nil, depDiags
	}

	val, diags := valuesAttr.Expr.Value(augmented)
	if diags.HasErrors() {
		return nil, diags
	}

	return &val, nil
}

// augmentEvalCtxWithDeps returns a copy of evalCtx with `dependency.<name>` bound to an object whose `outputs` attribute is the resolved mock_outputs of each declared dependency. Dependencies without mock_outputs are bound with an empty outputs object so references through them do not panic.
func augmentEvalCtxWithDeps(evalCtx *hcl.EvalContext, body *hclsyntax.Body, deps []AutoIncludeDependency) (*hcl.EvalContext, hcl.Diagnostics) {
	depObj := make(map[string]cty.Value, len(deps))

	var diags hcl.Diagnostics

	for _, dep := range deps {
		mockOutputs, mockDiags := mockOutputsFromBody(body, dep.Name, evalCtx)
		diags = append(diags, mockDiags...)
		depObj[dep.Name] = cty.ObjectVal(map[string]cty.Value{"outputs": mockOutputs})
	}

	if diags.HasErrors() {
		return nil, diags
	}

	augmented := &hcl.EvalContext{
		Variables: make(map[string]cty.Value, len(evalCtx.Variables)+1),
		Functions: evalCtx.Functions,
	}

	for k, v := range evalCtx.Variables {
		augmented.Variables[k] = v
	}

	augmented.Variables[varDependency] = cty.ObjectVal(depObj)

	return augmented, nil
}

// mockOutputsFromBody finds the dependency block by name in body, extracts its `mock_outputs` attribute, and returns the evaluated value. Returns cty.EmptyObjectVal when the dependency has no mock_outputs.
func mockOutputsFromBody(body *hclsyntax.Body, depName string, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	for _, block := range body.Blocks {
		if block.Type != blockDependency || len(block.Labels) != 1 || block.Labels[0] != depName {
			continue
		}

		attr, ok := block.Body.Attributes["mock_outputs"]
		if !ok {
			return cty.EmptyObjectVal, nil
		}

		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return cty.EmptyObjectVal, diags
		}

		return val, nil
	}

	return cty.EmptyObjectVal, nil
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

// AutoIncludeDependencyPaths reads the autoinclude file in unitDir and returns resolved dependency config_path values. Returns EmptyArgError when unitDir is empty so callers can distinguish bad input from a missing file.
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

	var errs []error

	for _, block := range body.Blocks {
		if block.Type != blockDependency {
			continue
		}

		if len(block.Labels) != 1 {
			errs = append(errs, MalformedDependencyError{
				FilePath: autoIncludePath,
				Name:     blockLabelsString(block),
				Reason:   fmt.Sprintf("dependency block requires exactly one label, got %d", len(block.Labels)),
			})

			continue
		}

		depPath, extractErr := extractDepPath(block, autoIncludePath, unitDir)
		if extractErr != nil {
			errs = append(errs, extractErr)

			continue
		}

		paths = append(paths, depPath)
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return paths, nil
}

// readAutoIncludeBody reads and parses an autoinclude file, returning (nil, nil) when the file does not exist.
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

// blockLabelsString joins a block's labels for error messages; returns "(unlabeled)" when there are none.
func blockLabelsString(block *hclsyntax.Block) string {
	if len(block.Labels) == 0 {
		return "(unlabeled)"
	}

	return strings.Join(block.Labels, " ")
}

// extractDepPath returns the resolved config_path for a dependency block. Caller must ensure the block has exactly one label.
func extractDepPath(block *hclsyntax.Block, autoIncludePath, unitDir string) (string, error) {
	name := block.Labels[0]

	configPathAttr, exists := block.Body.Attributes[attrConfigPath]
	if !exists {
		return "", MalformedDependencyError{FilePath: autoIncludePath, Name: name, Reason: "missing config_path attribute"}
	}

	val, valDiags := configPathAttr.Expr.Value(nil)
	if valDiags.HasErrors() {
		return "", MalformedDependencyError{FilePath: autoIncludePath, Name: name, Reason: "config_path: " + valDiags.Error(), Err: valDiags}
	}

	if !val.IsKnown() {
		return "", MalformedDependencyError{FilePath: autoIncludePath, Name: name, Reason: "config_path is unknown"}
	}

	if val.IsNull() {
		return "", MalformedDependencyError{FilePath: autoIncludePath, Name: name, Reason: "config_path is null"}
	}

	if val.Type() != cty.String {
		return "", MalformedDependencyError{FilePath: autoIncludePath, Name: name, Reason: "config_path must be a string, got " + val.Type().FriendlyName()}
	}

	depPath := val.AsString()
	if !filepath.IsAbs(depPath) {
		depPath = filepath.Clean(filepath.Join(unitDir, depPath))
	}

	return util.ResolvePath(depPath), nil
}
