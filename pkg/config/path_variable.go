package config

import (
	"context"
	"fmt"
	"path/filepath"

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
	// pathAttrModule names `path.module`, the directory in which OpenTofu/Terraform runs.
	pathAttrModule = "module"
	// pathAttrInclude names `path.include`, the path of the included file.
	pathAttrInclude = "include"
	// pathAttrRead names `path.read`, the path of the file read_terragrunt_config reads.
	pathAttrRead = "read"
)

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

	if refs.has(pathAttrUnit) || refs.has(pathAttrStack) || refs.has(pathAttrModule) {
		markNotShareable(ctx)
	}

	if refs.has(pathAttrStack) {
		stackDir, err := enclosingStackDir(v.FS, pctx)
		if err != nil {
			return nil, fmt.Errorf("resolving path.stack for %s: %w", file.ConfigPath, err)
		}

		if stackDir != "" {
			attrs[pathAttrStack] = cty.StringVal(stackDir)
		}
	}

	if refs.has(pathAttrModule) {
		moduleDir, err := unitModuleDir(ctx, l, v, pctx)
		if err != nil {
			return nil, fmt.Errorf("resolving path.module for %s: %w", file.ConfigPath, err)
		}

		attrs[pathAttrModule] = cty.StringVal(moduleDir)
	}

	return pathObject(attrs), nil
}

// withPathVariable returns a copy of pctx with the `path` value for file, or pctx when the experiment is disabled.
func withPathVariable(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	file *hclparse.File,
	include *IncludeConfig,
) (*ParsingContext, error) {
	path, err := PathVariable(ctx, l, v, pctx, file, include)
	if err != nil || path == nil {
		return pctx, err
	}

	c := pctx.Clone()
	c.Path = path

	return c, nil
}

// withValuesPathVariable returns pctx with the `path` value for the values file in directory.
func withValuesPathVariable(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
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
		return withPathVariable(ctx, l, v, pctx, file, nil)
	}

	c := pctx.Clone()
	c.Path = stackValuesPath(pctx, directory)

	return c, nil
}

// evalPathVariable returns the `path` value of pctx, built from the base attributes when no file supplied one.
func evalPathVariable(ctx context.Context, pctx *ParsingContext) cty.Value {
	if pctx.Path != nil {
		return *pctx.Path
	}

	if !isStackConfigFile(pctx.TerragruntConfigPath) {
		markNotShareable(ctx)
	}

	var include *IncludeConfig
	if pctx.TrackInclude != nil {
		include = pctx.TrackInclude.Original
	}

	return *pathObject(basePathAttributes(pctx, include))
}

// pathCacheKeySuffix returns the part of a parse cache key that covers the unit `path` reads.
func pathCacheKeySuffix(pctx *ParsingContext) string {
	if !pctx.Experiments.Evaluate(experiment.PathVariable) {
		return ""
	}

	return "-" + pctx.OriginalTerragruntConfigPath
}

// autoIncludeUnitRoots returns the variable roots a unit autoinclude leaves for the generated unit to resolve.
func autoIncludeUnitRoots(pctx *ParsingContext) []string {
	if !pctx.Experiments.Evaluate(experiment.PathVariable) {
		return nil
	}

	return []string{MetadataPath}
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

	if isStackConfigFile(pctx.TerragruntConfigPath) {
		attrs = map[string]cty.Value{pathAttrStack: cty.StringVal(filepath.Dir(pctx.TerragruntConfigPath))}
	}

	if include != nil {
		attrs[pathAttrInclude] = cty.StringVal(include.Path)
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
func unitModuleDir(ctx context.Context, l log.Logger, v *venv.Venv, pctx *ParsingContext) (string, error) {
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

	cfg, err := PartialParseConfigFile(ctx, l, v, unitPctx, unitPctx.TerragruntConfigPath, nil)
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
		l.Debugf("Resolving path.module of %s in the cache directory: %v", unitPctx.TerragruntConfigPath, err)
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
