package hclparse

import (
	"fmt"
	iofs "io/fs"
	"path/filepath"
	"strings"

	"errors"

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
//   - dependency.config_path is resolved (references unit.*.path / values.*)
//   - mock_outputs, inputs, and other content are partially evaluated the same way: stack-level local.* / values.* / unit.* / stack.* and function calls resolve, dependency.*.outputs.* is preserved
//   - a locals block is rejected at generate time (stack-level locals belong in terragrunt.stack.hcl)
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
	return a.ResolveForKind(evalCtx, KindUnit, "")
}

// ResolveForKind is Resolve with the component kind and parent name known, so a
// stack-level autoinclude can be validated against the unsupported pattern where
// an injected unit/stack consumes a sibling dependency's outputs through values.
func (a *AutoIncludeHCL) ResolveForKind(evalCtx *hcl.EvalContext, kind AutoIncludeKind, name string) (*AutoIncludeResolved, hcl.Diagnostics) {
	if a == nil || a.Remain == nil {
		return nil, nil
	}

	body, ok := a.Remain.(*hclsyntax.Body)
	if !ok {
		// Non-syntax body: return result with EvalCtx even though partial evaluation is not possible.
		return &AutoIncludeResolved{EvalCtx: evalCtx, RawBody: a.Remain}, nil
	}

	if kind == KindStack {
		// An injected unit/stack whose values reference dependency.* outputs is unsupported: those outputs are
		// runtime-only, so values cannot be a generate-time literal. Reject it with a precise cross-level error.
		if diags := validateStackAutoIncludeDepValues(body, name); diags.HasErrors() {
			return nil, diags
		}

		// A stack autoinclude injects only unit and stack blocks into the generated stack file, so a top-level dependency block is rejected here at generate time instead of producing a file the strict discovery decode later rejects.
		if diags := rejectStackAutoIncludeDependencyBlocks(body, name); diags.HasErrors() {
			return nil, diags
		}
	}

	// Reject a locals block; stack-level locals belong in terragrunt.stack.hcl.
	if diags := rejectAutoIncludeLocalsBlock(body, kind, name); diags.HasErrors() {
		return nil, diags
	}

	// Reject a nested autoinclude block; an autoinclude must not contain another autoinclude.
	if diags := rejectAutoIncludeNestedAutoInclude(body, kind, name); diags.HasErrors() {
		return nil, diags
	}

	deps := make([]AutoIncludeDependency, 0, len(body.Blocks))

	var diags hcl.Diagnostics

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

	// Best-effort: always return whichever dependency blocks resolved, plus accumulated diagnostics.
	return &AutoIncludeResolved{
		EvalCtx:      evalCtx,
		Dependencies: deps,
		RawBody:      a.Remain,
	}, diags
}

// resolveDependencyBlock extracts config_path from a dependency block; caller must ensure exactly one label.
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

	pathRange := configPathAttr.Expr.Range().Ptr()

	val, diags := configPathAttr.Expr.Value(evalCtx)
	if diags.HasErrors() {
		// Surface one clear error anchored at config_path; the raw diagnostics can carry a function-internal
		// subject (e.g. a function that re-parses the stack file) that otherwise renders as a misleading
		// top-level error pointing at an unrelated unit or stack block.
		return AutoIncludeDependency{}, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid config_path",
			Detail:   fmt.Sprintf("dependency %q config_path could not be evaluated at stack generate time: %s", name, configPathEvalReason(diags)),
			Subject:  pathRange,
		}}
	}

	// Null/unknown evaluate without HCL diagnostics; surface them as typed diags with source position so callers can detect the failure and editors can underline the offending expression.
	switch {
	case !val.IsKnown():
		return AutoIncludeDependency{}, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Unknown config_path",
			Detail:   fmt.Sprintf("dependency %q config_path evaluated to an unknown value", name),
			Subject:  pathRange,
		}}
	case val.IsNull():
		return AutoIncludeDependency{}, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Null config_path",
			Detail:   fmt.Sprintf("dependency %q config_path must not be null", name),
			Subject:  pathRange,
		}}
	case val.Type() != cty.String:
		return AutoIncludeDependency{}, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid config_path type",
			Detail:   fmt.Sprintf("dependency %q config_path must evaluate to a string", name),
			Subject:  pathRange,
		}}
	}

	return AutoIncludeDependency{
		Name:       name,
		ConfigPath: val.AsString(),
		Block:      block.AsHCLBlock(),
	}, nil
}

// configPathEvalReason returns a one-line reason from config_path evaluation diagnostics for the wrapped error.
func configPathEvalReason(diags hcl.Diagnostics) string {
	for _, d := range diags {
		if d.Detail != "" {
			return d.Detail
		}
	}

	return diags.Error()
}

// AutoIncludeDependencyPaths reads the autoinclude file in unitDir and returns resolved dependency config_path values. Returns EmptyArgError when unitDir is empty so callers can distinguish bad input from a missing file.
// It is off the production parse path (the partial-parse merge folds autoinclude dependencies into the config); it is retained for test-time introspection of generated autoinclude files.
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
		return nil, FileParseError{FilePath: path, Err: diags}
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
		return "", MalformedDependencyError{FilePath: autoIncludePath, Name: name, Reason: "config_path evaluation failed", Err: valDiags}
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

// StackAutoIncludeDepValuesError scans a stack autoinclude body for the unsupported cross-level pattern: an injected unit/stack whose values reference dependency outputs, which are not available at stack generate time. Returns the populated typed error, or nil when absent. Shared by the fail-fast generation check and the pkg/config backstop so the two cannot drift.
func StackAutoIncludeDepValuesError(body *hclsyntax.Body, stackName string) *StackAutoIncludeDependencyValuesError {
	for _, block := range body.Blocks {
		if block.Type != VarUnit && block.Type != VarStack {
			continue
		}

		valuesAttr, hasValues := block.Body.Attributes[varValues]
		if !hasValues {
			continue
		}

		if !valuesReferenceDependency(valuesAttr.Expr) {
			continue
		}

		return &StackAutoIncludeDependencyValuesError{
			StackName: stackName,
			UnitName:  blockLabelsString(block),
			Subject:   valuesAttr.Expr.Range().Ptr(),
		}
	}

	return nil
}

// validateStackAutoIncludeDepValues rejects a stack-level autoinclude whose injected unit/stack values reference dependency.* outputs, which are unavailable at stack generate time, whether or not that dependency is declared.
func validateStackAutoIncludeDepValues(body *hclsyntax.Body, stackName string) hcl.Diagnostics {
	typed := StackAutoIncludeDepValuesError(body, stackName)
	if typed == nil {
		return nil
	}

	return hcl.Diagnostics{{
		Severity: hcl.DiagError,
		Summary:  "stack autoinclude dependency outputs referenced by injected values",
		Detail:   typed.Error(),
		Subject:  typed.Subject,
		Extra:    *typed,
	}}
}

// rejectStackAutoIncludeDependencyBlocks rejects a top-level dependency block in a stack autoinclude, which is unsupported because a stack autoinclude injects only unit and stack blocks into the generated stack file.
func rejectStackAutoIncludeDependencyBlocks(body *hclsyntax.Body, stackName string) hcl.Diagnostics {
	var diags hcl.Diagnostics

	for _, block := range body.Blocks {
		if block.Type != blockDependency {
			continue
		}

		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "dependency block is not allowed in a stack autoinclude",
			Detail:   fmt.Sprintf("stack %q autoinclude declares dependency %q, but a stack autoinclude may inject only unit and stack blocks; declare the dependency inside the target unit's own autoinclude instead", stackName, blockLabelsString(block)),
			Subject:  block.DefRange().Ptr(),
		})
	}

	return diags
}

// valuesReferenceDependency reports whether expr references the dependency namespace in any form.
// RootName matches every traversal uniformly: dependency.foo, dependency["foo"], and the dynamic
// dependency[values.x] all report "dependency" as the root, so no per-form handling is needed.
func valuesReferenceDependency(expr hclsyntax.Expression) bool {
	return referencesRoot(expr, varDependency)
}

// referencesRoot reports whether expr contains any traversal whose root name equals root.
func referencesRoot(expr hclsyntax.Expression, root string) bool {
	for _, traversal := range expr.Variables() {
		if traversal.RootName() == root {
			return true
		}
	}

	return false
}

// rejectAutoIncludeLocalsBlock rejects a locals block defined anywhere in the autoinclude body for both kinds.
func rejectAutoIncludeLocalsBlock(body *hclsyntax.Body, kind AutoIncludeKind, name string) hcl.Diagnostics {
	block, err := findNestedBlock(body, blockLocals, 0)
	if err != nil {
		return blockDepthDiags(body, err)
	}

	if block == nil {
		return nil
	}

	typed := AutoIncludeLocalsBlockError{Subject: block.DefRange().Ptr(), Kind: string(kind), Component: name}

	return hcl.Diagnostics{{
		Severity: hcl.DiagError,
		Summary:  "locals block is not allowed inside autoinclude",
		Detail:   typed.Error(),
		Subject:  typed.Subject,
		Extra:    typed,
	}}
}

// rejectAutoIncludeNestedAutoInclude rejects an autoinclude block nested inside the autoinclude body, which would
// otherwise recurse. An injected unit or stack legitimately carries its own autoinclude for the next generate pass,
// so those subtrees are skipped.
func rejectAutoIncludeNestedAutoInclude(body *hclsyntax.Body, kind AutoIncludeKind, name string) hcl.Diagnostics {
	block, err := findNestedBlock(body, blockAutoInclude, 0)
	if err != nil {
		return blockDepthDiags(body, err)
	}

	if block == nil {
		return nil
	}

	typed := AutoIncludeNestedError{Subject: block.DefRange().Ptr(), Kind: string(kind), Component: name}

	return hcl.Diagnostics{{
		Severity: hcl.DiagError,
		Summary:  "autoinclude block is not allowed inside autoinclude",
		Detail:   typed.Error(),
		Subject:  typed.Subject,
		Extra:    typed,
	}}
}

// findNestedBlock returns the first block of blockType anywhere in body, bounded by maxBlockDepth; injected unit and
// stack subtrees are skipped because they are separate components generated in their own pass.
func findNestedBlock(body *hclsyntax.Body, blockType string, depth int) (*hclsyntax.Block, error) {
	if depth > maxBlockDepth {
		return nil, BlockDepthExceededError{MaxDepth: maxBlockDepth}
	}

	for _, block := range body.Blocks {
		if block.Type == blockType {
			return block, nil
		}

		if block.Type == VarUnit || block.Type == VarStack {
			continue
		}

		nested, err := findNestedBlock(block.Body, blockType, depth+1)
		if err != nil {
			return nil, err
		}

		if nested != nil {
			return nested, nil
		}
	}

	return nil, nil
}

// blockDepthDiags converts a block-nesting depth error into a diagnostic anchored at the autoinclude body.
func blockDepthDiags(body *hclsyntax.Body, err error) hcl.Diagnostics {
	return hcl.Diagnostics{{
		Severity: hcl.DiagError,
		Summary:  "autoinclude block nesting too deep",
		Detail:   err.Error(),
		Subject:  body.SrcRange.Ptr(),
	}}
}
