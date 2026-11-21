package config

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// inheritableVariables are the variables whose value a parse of a dependency inherits from the config whose dependency
// it is, until the dependency's own value replaces it.
var inheritableVariables = []string{MetadataLocal, MetadataFeatureFlag, MetadataDependency}

// getDependencyBlockConfigPathsByFilepath returns the config paths of the dependency blocks of the config at cfgPath,
// which pctx parses as a dependency. Every read records the files the parse read on pctx.FilesRead.
//
// A command scans them once for every config whose parse can't read a value pctx inherits, and a concurrent read of
// the same config waits for the scan. A config whose parse can read one, a config whose scan fails, and a read made
// while a scan of any config runs on the same call path read them again with pctx.
func getDependencyBlockConfigPathsByFilepath(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	cfgPath string,
) ([]string, error) {
	inheritingPctx := pctx.WithDecodeList(DependencyBlock).WithDiagnosticsSuppressed()

	if _, scanning := ctx.Value(scanningDependencyPathsKey{}).(bool); scanning {
		paths, _, err := readDependencyPaths(ctx, l, v, inheritingPctx, cfgPath)

		return paths, err
	}

	scanPctx := pctx.forDependencyScan().WithDecodeList(DependencyBlock).WithDiagnosticsSuppressed()

	key := fmt.Sprintf("%#v", dependencyPathsKey{
		read:                  NewReadTerragruntConfigKey(v, scanPctx),
		skipOutputsResolution: scanPctx.SkipOutputsResolution,
	})

	flight, leads := ctx.Value(DependencyPathsCacheContextKey).(*dependencyPathsScans).join(key)
	if leads {
		scanCtx := context.WithValue(ctx, scanningDependencyPathsKey{}, true)
		flight.scan, flight.err = scanDependencyPaths(scanCtx, l, v, scanPctx, cfgPath)
		close(flight.done)
	}

	select {
	case <-flight.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if flight.err != nil || flight.scan.inherits(pctx) {
		paths, _, err := readDependencyPaths(ctx, l, v, inheritingPctx, cfgPath)

		return paths, err
	}

	for _, path := range flight.scan.filesRead {
		pctx.FilesRead.Add(path)
	}

	return flight.scan.paths, nil
}

// scanningDependencyPathsKey is the context key whose value marks a call path a scan of dependency paths runs on.
type scanningDependencyPathsKey struct{}

// dependencyPathsScans is the scans of dependency paths a command ran or is running, by key. Only a call path no scan
// runs on waits for a scan, and a scan never waits for one, so no wait can wait on itself.
type dependencyPathsScans struct {
	flights map[string]*dependencyPathsFlight
	mu      sync.Mutex
}

// dependencyPathsFlight is one scan of dependency paths. Its scan and err are set before done closes.
type dependencyPathsFlight struct {
	done chan struct{}
	scan *dependencyPathsScan
	err  error
}

// newDependencyPathsScans returns an empty set of scans.
func newDependencyPathsScans() *dependencyPathsScans {
	return &dependencyPathsScans{flights: map[string]*dependencyPathsFlight{}}
}

// join returns the scan under key, and true when the caller created it and must run it.
func (scans *dependencyPathsScans) join(key string) (*dependencyPathsFlight, bool) {
	scans.mu.Lock()
	defer scans.mu.Unlock()

	if flight, ok := scans.flights[key]; ok {
		return flight, false
	}

	flight := &dependencyPathsFlight{done: make(chan struct{})}
	scans.flights[key] = flight

	return flight, true
}

// dependencyPathsKey is the key [getDependencyBlockConfigPathsByFilepath] caches the dependency paths of a config
// under: what reading a config with read_terragrunt_config is keyed on, and whether dependency outputs are skipped.
type dependencyPathsKey struct {
	read                  ReadTerragruntConfigKey
	skipOutputsResolution bool
}

// dependencyPathsScan is the dependency paths of a config parsed without inherited values, with the files that parse
// read and the inheritable variables a parse of the config can read.
type dependencyPathsScan struct {
	reads     map[string]struct{}
	paths     []string
	filesRead []string
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

// scanDependencyPaths reads the dependency paths of the config at cfgPath with scanPctx, which inherits no values.
func scanDependencyPaths(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	scanPctx *ParsingContext,
	cfgPath string,
) (*dependencyPathsScan, error) {
	tracked := scanPctx.WithFileReadTracking()

	paths, cfg, err := readDependencyPaths(ctx, l, v, tracked, cfgPath)
	if err != nil {
		return nil, err
	}

	return &dependencyPathsScan{
		reads:     inheritedReads(ctx, l, v, scanPctx, cfgPath, cfg.ProcessedIncludes),
		paths:     paths,
		filesRead: tracked.FilesRead.Paths(),
	}, nil
}

// readDependencyPaths parses the dependency blocks of the config at cfgPath with pctx and returns their config paths
// with the config the parse returned.
func readDependencyPaths(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	cfgPath string,
) ([]string, *TerragruntConfig, error) {
	tgConfig, err := PartialParseConfigFile(ctx, l, v, pctx, cfgPath, nil)
	if err != nil {
		return nil, nil, err
	}

	paths, unresolved := tgConfig.DependencyPaths()
	for _, name := range unresolved {
		l.Debugf("Skipping dependency %q of %s in cycle check: config_path is not a known string", name, cfgPath)
	}

	return paths, tgConfig, nil
}

// inheritedReads returns the inheritable variables a parse of the dependency blocks of the config at cfgPath can read
// before the config's own value replaces them: in the include and feature blocks of the config and of each file it
// merges, in their dependency blocks, and anywhere in a file it exposes. includes is the includes a partial parse of
// the config processed. A config in HCL JSON, and one that includes a file in HCL JSON or a file that fails to parse,
// reads them all.
func inheritedReads(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	cfgPath string,
	includes map[string]IncludeConfig,
) map[string]struct{} {
	all := map[string]struct{}{}
	for _, name := range inheritableVariables {
		all[name] = struct{}{}
	}

	pctx = pctx.WithDiagnosticsDiscarded()

	reads := map[string]struct{}{}

	read := func(path string, exposed bool) bool {
		file, err := configFile(ctx, l, v, pctx, path)
		if err != nil {
			return false
		}

		native, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			return false
		}

		addBlockReads(reads, native, exposed)

		return true
	}

	if !read(cfgPath, false) {
		return all
	}

	for _, include := range includes {
		strategy, err := include.GetMergeStrategy()
		if err == nil && strategy == NoMerge && !include.GetExpose() {
			continue
		}

		if !read(include.Path, include.GetExpose()) {
			return all
		}
	}

	if autoIncludePath, ok := siblingAutoIncludePath(pctx, cfgPath); ok && vfs.Exists(v.FS, autoIncludePath) {
		if !read(autoIncludePath, false) {
			return all
		}
	}

	return reads
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
	walkNodes(node, func(node hclsyntax.Node) {
		traversal, ok := node.(*hclsyntax.ScopeTraversalExpr)
		if ok && slices.Contains(inheritableVariables, traversal.Traversal.RootName()) {
			reads[traversal.Traversal.RootName()] = struct{}{}
		}
	})
}
