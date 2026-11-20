package config

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// inheritableVariables are the variables whose value a parse of a dependency inherits from the config whose dependency
// it is, until the dependency's own value replaces it.
var inheritableVariables = []string{MetadataLocal, MetadataFeatureFlag, MetadataDependency}

// getDependencyBlockConfigPathsByFilepath returns the config paths of the dependency blocks of the config at cfgPath,
// which pctx parses as a dependency. Every read records the files the parse read on pctx.FilesRead.
//
// A command reads them once for every config whose parse can't read a value pctx inherits, and a concurrent read of
// the same config waits for the first. A config whose parse can read one is read again with pctx each time.
func getDependencyBlockConfigPathsByFilepath(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	cfgPath string,
) ([]string, error) {
	scanPctx := pctx.forDependencyScan().WithDecodeList(DependencyBlock).WithDiagnosticsSuppressed()

	key := fmt.Sprintf("%#v", dependencyPathsKey{
		read:                  NewReadTerragruntConfigKey(v, scanPctx),
		skipOutputsResolution: scanPctx.SkipOutputsResolution,
	})
	scans := cache.ContextCache[*dependencyPathsScan](ctx, DependencyPathsCacheContextKey)

	scan, found := scans.Get(ctx, key)
	if !found || !scan.scanned && !scan.inherits(pctx) {
		var err error

		scan, err = scanDependencyPaths(ctx, l, v, pctx, scanPctx, cfgPath, key, scans)
		if err != nil {
			return nil, err
		}
	}

	if scan.inherits(pctx) {
		return readDependencyPaths(ctx, l, v, pctx.WithDecodeList(DependencyBlock).WithDiagnosticsSuppressed(), cfgPath)
	}

	for _, path := range scan.filesRead {
		pctx.FilesRead.Add(path)
	}

	return scan.paths, nil
}

// dependencyPathsKey is the key [getDependencyBlockConfigPathsByFilepath] caches the dependency paths of a config
// under: what reading a config with read_terragrunt_config is keyed on, and whether dependency outputs are skipped.
type dependencyPathsKey struct {
	read                  ReadTerragruntConfigKey
	skipOutputsResolution bool
}

// dependencyPathsScan is what a command knows about the dependency paths of a config: the inheritable variables its
// parse reads, and once scanned, its dependency paths with the files the parse that found them read.
type dependencyPathsScan struct {
	reads     map[string]struct{}
	paths     []string
	filesRead []string
	scanned   bool
}

// inherits reports whether the parse of the config with pctx reads a value pctx inherits from the config whose
// dependency it is.
func (scan *dependencyPathsScan) inherits(pctx *ParsingContext) bool {
	inherited := map[string]bool{
		MetadataLocal:       pctx.Locals != nil,
		MetadataFeatureFlag: pctx.Features != nil,
		MetadataDependency:  pctx.DecodedDependencies != nil || pctx.DependencyBlocks != nil,
	}

	for name := range scan.reads {
		if inherited[name] {
			return true
		}
	}

	return false
}

// scanDependencyPaths returns what the command knows about the dependency paths of the config at cfgPath, with the
// paths parsed with scanPctx unless the parse with pctx reads a value pctx inherits. It caches the result in scans
// under key, unless a concurrent call holding the lock for key cached it first.
func scanDependencyPaths(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	scanPctx *ParsingContext,
	cfgPath string,
	key string,
	scans *cache.Cache[*dependencyPathsScan],
) (*dependencyPathsScan, error) {
	locks := ctx.Value(DependencyPathsLocksContextKey).(*util.KeyLocks)

	if err := locks.LockContext(ctx, key); err != nil {
		return nil, fmt.Errorf("waiting for a concurrent read of the dependencies of %s: %w", cfgPath, err)
	}
	defer locks.Unlock(key)

	scan, found := scans.Get(ctx, key)
	if !found {
		scan = &dependencyPathsScan{reads: inheritedReads(ctx, l, v, scanPctx, cfgPath)}
		scans.Put(ctx, key, scan)
	}

	if scan.scanned || scan.inherits(pctx) {
		return scan, nil
	}

	tracked := scanPctx.WithFileReadTracking()

	paths, err := readDependencyPaths(ctx, l, v, tracked, cfgPath)
	if err != nil {
		return nil, err
	}

	scan = &dependencyPathsScan{reads: scan.reads, paths: paths, filesRead: tracked.FilesRead.Paths(), scanned: true}
	scans.Put(ctx, key, scan)

	return scan, nil
}

// readDependencyPaths parses the dependency blocks of the config at cfgPath with pctx and returns their config paths.
func readDependencyPaths(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	cfgPath string,
) ([]string, error) {
	tgConfig, err := PartialParseConfigFile(ctx, l, v, pctx, cfgPath, nil)
	if err != nil {
		return nil, err
	}

	paths, unresolved := tgConfig.DependencyPaths()
	for _, name := range unresolved {
		l.Debugf("Skipping dependency %q of %s in cycle check: config_path is not a known string", name, cfgPath)
	}

	return paths, nil
}

// inheritedReads returns the inheritable variables a parse of the dependency blocks of the config at cfgPath can read
// before the config's own value replaces them: in the include and feature blocks of the config and of each file it
// includes, in their dependency blocks, and anywhere in a file it exposes. A config that fails to parse, and one in HCL
// JSON, reads them all.
func inheritedReads(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	cfgPath string,
) map[string]struct{} {
	all := map[string]struct{}{}
	for _, name := range inheritableVariables {
		all[name] = struct{}{}
	}

	file, err := parsedConfigFile(ctx, l, v, pctx.WithDiagnosticsDiscarded(), cfgPath)
	if err != nil {
		return all
	}

	body, native := file.Body.(*hclsyntax.Body)
	if !native {
		return all
	}

	reads := map[string]struct{}{}
	addBlockReads(reads, body, false)

	for _, included := range readIncludes(ctx, l, v, pctx, file, body) {
		includedBody, err := configBody(ctx, l, v, pctx.WithDiagnosticsDiscarded(), included.path)
		if err != nil {
			continue
		}

		nativeIncluded, ok := includedBody.(*hclsyntax.Body)
		if !ok {
			return all
		}

		addBlockReads(reads, nativeIncluded, included.exposed)
	}

	return reads
}

// readInclude is a file a parse of a config reads because one of its include blocks names it.
type readInclude struct {
	path    string
	exposed bool
}

// readIncludes returns the files the include blocks of file, whose body is body, name and a parse reads: those it
// merges or exposes, and its sibling autoinclude. An include block that fails to decode or names an invalid
// merge_strategy reads nothing.
func readIncludes(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	file *hclparse.File,
	body *hclsyntax.Body,
) []readInclude {
	var included []readInclude

	if slices.ContainsFunc(body.Blocks, isIncludeBlock) {
		evalPctx := pctx.WithTrackInclude(nil).WithDiagnosticsDiscarded()

		evalCtx, err := CreateTerragruntEvalContext(ctx, l, v, evalPctx, file.ConfigPath, file.FunctionCalls())
		if err == nil {
			for _, include := range nativeIncludes(body, evalCtx) {
				strategy, err := include.GetMergeStrategy()
				if err != nil || strategy == NoMerge && !include.GetExpose() {
					continue
				}

				path := include.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(filepath.Dir(file.ConfigPath), path)
				}

				included = append(included, readInclude{path: path, exposed: include.GetExpose()})
			}
		}
	}

	if autoIncludePath, ok := siblingAutoIncludePath(pctx, file.ConfigPath); ok && vfs.Exists(v.FS, autoIncludePath) {
		included = append(included, readInclude{path: autoIncludePath})
	}

	return included
}

// addBlockReads adds to reads the inheritable variables body references in its include, feature, and dependency
// blocks, or anywhere when anywhere is true.
func addBlockReads(reads map[string]struct{}, body *hclsyntax.Body, anywhere bool) {
	if anywhere {
		addReads(reads, body)

		return
	}

	for _, block := range body.Blocks {
		switch block.Type {
		case MetadataInclude, MetadataFeatureFlag, MetadataDependency:
			addReads(reads, block.Body)
		}
	}
}

// addReads adds to reads each inheritable variable an expression in node references.
func addReads(reads map[string]struct{}, node hclsyntax.Node) {
	diags := hclsyntax.VisitAll(node, func(node hclsyntax.Node) hcl.Diagnostics {
		traversal, ok := node.(*hclsyntax.ScopeTraversalExpr)
		if ok && slices.Contains(inheritableVariables, traversal.Traversal.RootName()) {
			reads[traversal.Traversal.RootName()] = struct{}{}
		}

		return nil
	})
	if diags.HasErrors() {
		panic("addReads: a visitor that reports nothing reported " + diags.Error())
	}
}
