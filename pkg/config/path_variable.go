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
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// pathAttrUnit names `path.unit`, the directory of the unit config.
	pathAttrUnit = "unit"
	// pathAttrStack names `path.stack`, the directory of the stack file.
	pathAttrStack = "stack"
	// pathAttrDownload names `path.download`, the directory in which OpenTofu/Terraform runs.
	pathAttrDownload = "download"
	// pathAttrInclude names `path.include`, the path of the included file.
	pathAttrInclude = "include"
	// pathAttrRead names `path.read`, the path of the file read_terragrunt_config reads.
	pathAttrRead = "read"
	// pathExperimentHint tells how to enable the `path` variable.
	pathExperimentHint = " The `path` variable needs the " + experiment.PathVariable + " experiment. Enable it with " +
		"--experiment " + experiment.PathVariable + " or TG_EXPERIMENT=" + experiment.PathVariable + "."
)

// pathExperimentHintOption adds the experiment hint to the diagnostics of an unknown `path` variable.
var pathExperimentHintOption = hclparse.WithDiagnosticsHandler(pathExperimentDiagnostics)

// pathReferences is the set of `path` attributes a file references.
type pathReferences struct {
	attrs map[string]struct{}
	whole bool
}

// PathVariable returns the `path` value for file; returns nil when the path-variable experiment is disabled.
func PathVariable(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
	include *IncludeConfig,
) (*cty.Value, error) {
	if !pctx.Experiments.Evaluate(experiment.PathVariable) {
		return nil, nil
	}

	attrs := basePathAttributes(pctx, include)

	if isStackConfigFile(pctx.TerragruntConfigPath) {
		return pathObject(attrs), nil
	}

	refs := filePathReferences(file)

	if refs.has(pathAttrUnit) || refs.has(pathAttrStack) || refs.has(pathAttrDownload) {
		markNotShareable(ctx)
	}

	if _, ofUnit := attrs[pathAttrUnit]; ofUnit && refs.has(pathAttrDownload) {
		moduleDir, err := unitModuleDir(ctx, l, v, s, pctx)
		if err != nil {
			return nil, fmt.Errorf("resolving path.download for %s: %w", file.ConfigPath, err)
		}

		attrs[pathAttrDownload] = cty.StringVal(moduleDir)
	}

	return pathObject(attrs), nil
}

// withPathVariable returns a copy of pctx with the `path` value for file, or pctx when the experiment is disabled.
func withPathVariable(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
	include *IncludeConfig,
) (*ParsingContext, error) {
	path, err := PathVariable(ctx, l, v, s, pctx, file, include)
	if err != nil || path == nil {
		return pctx, err
	}

	return pctx.withPath(path), nil
}

// withValuesPathVariable returns pctx with the `path` value for the values file in directory.
func withValuesPathVariable(
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
		return withPathVariable(ctx, l, v, s, pctx, file, nil)
	}

	return pctx.withPath(stackValuesPath(pctx, directory)), nil
}

// evalPathVariable returns the `path` value of pctx, built from the base attributes when no file supplied one.
func evalPathVariable(ctx context.Context, pctx *ParsingContext) cty.Value {
	if pctx.Path != nil {
		return *pctx.Path
	}

	if !isStackConfigFile(pctx.TerragruntConfigPath) {
		markNotShareable(ctx)
	}

	return *pathObject(basePathAttributes(pctx, pctx.include))
}

// pathExperimentDiagnostics adds the experiment hint to each diagnostic of an unknown `path` variable.
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

// hintUnknownPath adds the experiment hint to each diagnostic of an unknown `path` variable.
func hintUnknownPath(diags hcl.Diagnostics) {
	for _, diag := range diags {
		if readsUnknownPath(diag) && !strings.HasSuffix(diag.Detail, pathExperimentHint) {
			diag.Detail += pathExperimentHint
		}
	}
}

// readsUnknownPath reports whether diag, or a cause that diag holds, is for a reference to an unknown `path`.
func readsUnknownPath(diag *hcl.Diagnostic) bool {
	if causes, held := diag.Extra.(hcl.Diagnostics); held {
		return slices.ContainsFunc(causes, referencesUnknownPath)
	}

	return referencesUnknownPath(diag)
}

// referencesUnknownPath reports whether diag is for a reference to `path` that no scope defines.
func referencesUnknownPath(diag *hcl.Diagnostic) bool {
	if diag.Expression == nil {
		return false
	}

	traversal, diags := hcl.AbsTraversalForExpr(diag.Expression)
	if diags.HasErrors() || traversal.RootName() != MetadataPath {
		return false
	}

	for scope := diag.EvalContext; scope != nil; scope = scope.Parent() {
		if _, defined := scope.Variables[MetadataPath]; defined {
			return false
		}
	}

	return true
}

// has reports whether the file references attr, alone or through the whole `path` object.
func (refs pathReferences) has(attr string) bool {
	if refs.whole {
		return true
	}

	_, found := refs.attrs[attr]

	return found
}

// add records the `path` attribute traversal reads.
func (refs *pathReferences) add(traversal hcl.Traversal) {
	if traversal.RootName() != MetadataPath {
		return
	}

	name, named := pathAttributeName(traversal)
	if !named {
		refs.whole = true

		return
	}

	refs.attrs[name] = struct{}{}
}

// pathAttributeName returns the attribute a traversal rooted at `path` reads, and false when it names none.
func pathAttributeName(traversal hcl.Traversal) (string, bool) {
	steps := traversal.SimpleSplit().Rel
	if len(steps) == 0 {
		return "", false
	}

	switch step := steps[0].(type) {
	case hcl.TraverseAttr:
		return step.Name, true
	case hcl.TraverseIndex:
		if step.Key.Type() == cty.String && step.Key.IsKnown() && !step.Key.IsNull() {
			return step.Key.AsString(), true
		}
	}

	return "", false
}

// filePathReferences returns the `path` attributes the expressions of file reference.
func filePathReferences(file *hclparse.File) pathReferences {
	refs := pathReferences{attrs: map[string]struct{}{}}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		attrs, diags := file.Body.JustAttributes()
		refs.whole = diags.HasErrors()

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
	refs.whole = refs.whole || diags.HasErrors()

	return refs
}

// basePathAttributes returns the `path` attributes that need no parse and no filesystem probe.
func basePathAttributes(pctx *ParsingContext, include *IncludeConfig) map[string]cty.Value {
	attrs := map[string]cty.Value{pathAttrUnit: cty.StringVal(filepath.Dir(pctx.OriginalTerragruntConfigPath))}

	if stackFile, ofStack := evaluatedStackFile(pctx); ofStack {
		attrs = map[string]cty.Value{pathAttrStack: cty.StringVal(filepath.Dir(stackFile))}
	}

	if include != nil {
		attrs[pathAttrInclude] = cty.StringVal(filepath.Clean(include.Path))
	}

	if chain := pctx.readConfigChain; len(chain) > 0 && chain[len(chain)-1].path == pctx.TerragruntConfigPath {
		attrs[pathAttrRead] = cty.StringVal(pctx.TerragruntConfigPath)
	}

	return attrs
}

// stackValuesPath returns the `path` value of the values file that belongs to the stack file in stackDir.
func stackValuesPath(pctx *ParsingContext, stackDir string) *cty.Value {
	attrs := map[string]cty.Value{pathAttrStack: cty.StringVal(stackDir)}

	read, found := basePathAttributes(pctx, nil)[pathAttrRead]
	if found && filepath.Dir(pctx.TerragruntConfigPath) == stackDir {
		attrs[pathAttrRead] = read
	}

	return pathObject(attrs)
}

// pathObject returns attrs as the `path` object value.
func pathObject(attrs map[string]cty.Value) *cty.Value {
	value := cty.ObjectVal(attrs)

	return &value
}

// evaluatedStackFile returns the stack file that pctx evaluates a file for, and false for a file of a unit.
func evaluatedStackFile(pctx *ParsingContext) (string, bool) {
	if isStackConfigFile(pctx.TerragruntConfigPath) {
		return pctx.TerragruntConfigPath, true
	}

	if isStackConfigFile(pctx.OriginalTerragruntConfigPath) {
		return pctx.OriginalTerragruntConfigPath, true
	}

	chain := pctx.readConfigChain
	if pctx.viaDependency || len(chain) == 0 {
		return "", false
	}

	return chain[0].path, isStackConfigFile(chain[0].path)
}

// isStackConfigFile reports whether path names a stack file or a stack autoinclude file.
func isStackConfigFile(path string) bool {
	base := filepath.Base(path)

	return base == DefaultStackFile || base == DefaultAutoIncludeStackFile
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
