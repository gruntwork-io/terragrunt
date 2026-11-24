package config

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// selfPathAttr names the attribute of `unit.path` and `stack.path`.
	selfPathAttr = "path"
	// pathAttrDownload names `path.download`, the directory in which OpenTofu/Terraform runs.
	pathAttrDownload = "download"
	// pathAttrInclude names `path.include`, the path of the included file.
	pathAttrInclude = "include"
	// pathAttrRead names `path.read`, the path of the file read_terragrunt_config reads.
	pathAttrRead = "read"
	// pathExperimentHint tells how to enable `unit.path`, `stack.path` and the `path` variable.
	pathExperimentHint = " `unit.path`, `stack.path` and the `path` variable need the " + experiment.PathVariable +
		" experiment. Enable it with --experiment " + experiment.PathVariable + " or TG_EXPERIMENT=" +
		experiment.PathVariable + "."
)

// pathExperimentHintOption adds the experiment hint to the diagnostics of an unknown path value.
var pathExperimentHintOption = hclparse.WithDiagnosticsHandler(pathExperimentDiagnostics)

// pathReferences is the set of path values a file references.
type pathReferences struct {
	unit     bool
	stack    bool
	download bool
}

// PathVariables returns the `unit`, `stack` and `path` variables for file; returns nil when the experiment is disabled.
func PathVariables(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
	include *IncludeConfig,
) (map[string]cty.Value, error) {
	if !pctx.Experiments.Evaluate(experiment.PathVariable) {
		return nil, nil
	}

	attrs := basePathAttributes(pctx, include)

	if isStackConfigFile(pctx.TerragruntConfigPath) {
		return pathVariables(pctx, attrs), nil
	}

	refs := filePathReferences(file)

	if refs.unit || refs.stack || refs.download {
		markNotShareable(ctx)
	}

	if refs.download {
		moduleDir, err := unitModuleDir(ctx, l, v, s, pctx)
		if err != nil {
			return nil, fmt.Errorf("resolving path.download for %s: %w", file.ConfigPath, err)
		}

		attrs[pathAttrDownload] = cty.StringVal(moduleDir)
	}

	vars := pathVariables(pctx, attrs)

	if !refs.stack {
		return vars, nil
	}

	stackDir, err := enclosingStackDir(v.FS, pctx)
	if err != nil {
		return nil, fmt.Errorf("resolving stack.path for %s: %w", file.ConfigPath, err)
	}

	if stackDir != "" {
		vars[MetadataStack] = selfPath(stackDir)
	}

	return vars, nil
}

// withPathVariables returns a copy of pctx with the path variables for file, or pctx when the experiment is disabled.
func withPathVariables(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
	include *IncludeConfig,
) (*ParsingContext, error) {
	vars, err := PathVariables(ctx, l, v, s, pctx, file, include)
	if err != nil || vars == nil {
		return pctx, err
	}

	return pctx.withPathVars(vars), nil
}

// withValuesPathVariables returns pctx with the path variables for the values file in directory.
func withValuesPathVariables(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
	directory string,
) (*ParsingContext, error) {
	if !pctx.Experiments.Evaluate(experiment.PathVariable) {
		return pctx, nil
	}

	ofStack, err := vfs.FileExists(v.FS, filepath.Join(directory, DefaultStackFile))
	if err != nil {
		return nil, err
	}

	if !ofStack {
		return withPathVariables(ctx, l, v, s, pctx, file, nil)
	}

	return pctx.withPathVars(stackValuesPathVariables(pctx, directory)), nil
}

// evalPathVariables returns the path variables of pctx, built from the base values when no file supplied them.
func evalPathVariables(ctx context.Context, pctx *ParsingContext) map[string]cty.Value {
	if pctx.PathVars != nil {
		return pctx.PathVars
	}

	if !isStackConfigFile(pctx.TerragruntConfigPath) {
		markNotShareable(ctx)
	}

	return pathVariables(pctx, basePathAttributes(pctx, pctx.include))
}

// pathExperimentDiagnostics adds the experiment hint to each diagnostic of an unknown path value.
func pathExperimentDiagnostics(_ *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
	hintUnknownPath(diags)

	return diags, nil
}

// withPathExperimentHint adds the experiment hint to the diagnostics that err holds when the experiment is not enabled.
func withPathExperimentHint(enabled bool, err error) error {
	var diags hcl.Diagnostics

	if !enabled && errors.As(err, &diags) {
		hintUnknownPath(diags)
	}

	return err
}

// hintUnknownPath adds the experiment hint to each diagnostic of an unknown path value.
func hintUnknownPath(diags hcl.Diagnostics) {
	for _, diag := range diags {
		if readsUnknownPath(diag) && !strings.HasSuffix(diag.Detail, pathExperimentHint) {
			diag.Detail += pathExperimentHint
		}
	}
}

// readsUnknownPath reports whether diag, or a cause that diag holds, is for a reference to an unknown path value.
func readsUnknownPath(diag *hcl.Diagnostic) bool {
	if causes, held := diag.Extra.(hcl.Diagnostics); held {
		return slices.ContainsFunc(causes, referencesUnknownPath)
	}

	return referencesUnknownPath(diag)
}

// referencesUnknownPath reports whether diag is for a reference to a path value where no scope defines `path`.
func referencesUnknownPath(diag *hcl.Diagnostic) bool {
	if diag.Expression == nil {
		return false
	}

	traversal, diags := hcl.AbsTraversalForExpr(diag.Expression)
	if diags.HasErrors() || !readsPathValue(traversal) {
		return false
	}

	for scope := diag.EvalContext; scope != nil; scope = scope.Parent() {
		if _, defined := scope.Variables[MetadataPath]; defined {
			return false
		}
	}

	return true
}

// autoIncludeUnitRoots returns the variable roots a unit autoinclude leaves for the generated unit to resolve.
func autoIncludeUnitRoots(pctx *ParsingContext) []string {
	if !pctx.Experiments.Evaluate(experiment.PathVariable) {
		return nil
	}

	return []string{MetadataPath, MetadataUnit + "." + selfPathAttr}
}

// readsPathValue reports whether traversal reads `unit.path`, `stack.path` or the `path` variable.
func readsPathValue(traversal hcl.Traversal) bool {
	root := traversal.RootName()
	if root == MetadataPath {
		return true
	}

	name, named := inthclparse.FirstAttribute(traversal)

	return (root == MetadataUnit || root == MetadataStack) && named && name == selfPathAttr && len(traversal[1:]) == 1
}

// add records the path value that traversal reads.
func (refs *pathReferences) add(traversal hcl.Traversal) {
	switch traversal.RootName() {
	case MetadataUnit:
		refs.unit = true
	case MetadataStack:
		refs.stack = true
	case MetadataPath:
		name, named := inthclparse.FirstAttribute(traversal)
		refs.download = refs.download || !named || name == pathAttrDownload
	}
}

// filePathReferences returns the path values the expressions of file reference, all of them for a file with errors.
func filePathReferences(file *hclparse.File) pathReferences {
	refs := pathReferences{}
	all := pathReferences{unit: true, stack: true, download: true}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		attrs, diags := file.Body.JustAttributes()
		if diags.HasErrors() {
			return all
		}

		for _, attr := range attrs {
			for _, traversal := range attr.Expr.Variables() {
				refs.add(traversal)
			}
		}

		return refs
	}

	diags := hclsyntax.VisitAll(body, func(node hclsyntax.Node) hcl.Diagnostics {
		attr, ok := node.(*hclsyntax.Attribute)
		if !ok {
			return nil
		}

		for _, traversal := range attr.Expr.Variables() {
			refs.add(traversal)
		}

		return nil
	})
	if diags.HasErrors() {
		return all
	}

	return refs
}

// pathVariables returns the `path` object of attrs with the `stack.path` of a stack file or the `unit.path` of a unit.
func pathVariables(pctx *ParsingContext, attrs map[string]cty.Value) map[string]cty.Value {
	vars := map[string]cty.Value{MetadataPath: cty.ObjectVal(attrs)}

	if isStackConfigFile(pctx.TerragruntConfigPath) {
		vars[MetadataStack] = selfPath(filepath.Dir(pctx.TerragruntConfigPath))

		return vars
	}

	vars[MetadataUnit] = selfPath(filepath.Dir(pctx.OriginalTerragruntConfigPath))
	vars[MetadataStack] = cty.EmptyObjectVal

	return vars
}

// basePathAttributes returns the `path` attributes that need no parse and no filesystem probe.
func basePathAttributes(pctx *ParsingContext, include *IncludeConfig) map[string]cty.Value {
	attrs := map[string]cty.Value{}

	if include != nil {
		attrs[pathAttrInclude] = cty.StringVal(filepath.Clean(include.Path))
	}

	if chain := pctx.readConfigChain; len(chain) > 0 && chain[len(chain)-1].path == pctx.TerragruntConfigPath {
		attrs[pathAttrRead] = cty.StringVal(pctx.TerragruntConfigPath)
	}

	return attrs
}

// stackValuesPathVariables returns the path variables of the values file that belongs to the stack file in stackDir.
func stackValuesPathVariables(pctx *ParsingContext, stackDir string) map[string]cty.Value {
	attrs := map[string]cty.Value{}

	read, found := basePathAttributes(pctx, nil)[pathAttrRead]
	if found && filepath.Dir(pctx.TerragruntConfigPath) == stackDir {
		attrs[pathAttrRead] = read
	}

	return map[string]cty.Value{
		MetadataStack: selfPath(stackDir),
		MetadataPath:  cty.ObjectVal(attrs),
	}
}

// selfPath returns the object that holds dir as its `path` attribute.
func selfPath(dir string) cty.Value {
	return cty.ObjectVal(map[string]cty.Value{selfPathAttr: cty.StringVal(dir)})
}

// isStackConfigFile reports whether path names a stack file or a stack autoinclude file.
func isStackConfigFile(path string) bool {
	base := filepath.Base(path)

	return base == DefaultStackFile || base == DefaultAutoIncludeStackFile
}

// enclosingStackDir returns the nearest directory at or above the unit that holds a stack file, or "" when none does.
func enclosingStackDir(fsys vfs.FS, pctx *ParsingContext) (string, error) {
	dir := filepath.Dir(pctx.OriginalTerragruntConfigPath)

	for range pctx.MaxFoldersToCheck {
		exists, err := vfs.FileExists(fsys, filepath.Join(dir, DefaultStackFile))
		if err != nil {
			return "", err
		}

		if exists {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}

		dir = parent
	}

	return "", ParentFileNotFoundError{
		Path:  pctx.OriginalTerragruntConfigPath,
		File:  DefaultStackFile,
		Cause: fmt.Sprintf("Exceeded maximum folders to check (%d)", pctx.MaxFoldersToCheck),
	}
}

// unitModuleDir returns the directory in which OpenTofu/Terraform runs for the unit pctx parses.
func unitModuleDir(ctx context.Context, l log.Logger, v *venv.Venv, s *Store, pctx *ParsingContext) (string, error) {
	if pctx.stubWorkingDirFunc {
		return "", nil
	}

	l, unitPctx, err := pctx.WithConfigPath(l, pctx.OriginalTerragruntConfigPath)
	if err != nil {
		return "", err
	}

	unitPctx = unitPctx.forReadTarget().WithDecodeList(moduleDirAttributes)
	unitPctx.stubWorkingDirFunc = true
	unitPctx.readConfigChain = nil

	cfg, err := s.ResolvePartialConfig(ctx, l, v, unitPctx, unitPctx.TerragruntConfigPath)
	if err != nil {
		return "", err
	}

	sourceURL, err := GetTerraformSourceURL(
		unitPctx.Source,
		unitPctx.SourceMap,
		unitPctx.OriginalTerragruntConfigPath,
		cfg,
	)
	if err != nil {
		return "", err
	}

	inPlace, err := runcfg.ShouldSkipCache(
		unitPctx.Experiments.Evaluate(experiment.NoDotTerragruntCache),
		unitPctx.Source,
		unitPctx.TerragruntConfigPath,
		cfg.ToRunConfig(l, v.FS),
	)
	if err != nil {
		l.Debugf("Resolving path.download of %s in the cache directory: %v", unitPctx.TerragruntConfigPath, err)
	}

	if inPlace {
		return unitPctx.WorkingDir, nil
	}

	sourceURL = tf.RewriteLegacyGCSPublicSource(ctx, l, sourceURL, unitPctx.StrictControls)

	source, err := tf.NewSource(
		l,
		v.FS,
		sourceURL,
		unitDownloadDir(unitPctx, cfg),
		unitPctx.WorkingDir,
		unitPctx.Experiments.Evaluate(experiment.Symlinks),
	)
	if err != nil {
		return "", err
	}

	return source.WorkingDir, nil
}

// unitDownloadDir returns the download directory that the run of the unit uses.
func unitDownloadDir(pctx *ParsingContext, cfg *TerragruntConfig) string {
	_, defaultDownloadDir := util.DefaultWorkingAndDownloadDirs(pctx.TerragruntConfigPath)

	if pctx.DownloadDir == defaultDownloadDir && cfg.DownloadDir != "" {
		return cfg.DownloadDir
	}

	return pctx.DownloadDir
}
