package discovery

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/zclconf/go-cty/cty/function"
)

const (
	// defaultDiscoveryWorkers is the default number of concurrent workers for discovery operations.
	defaultDiscoveryWorkers = 4

	// maxDiscoveryWorkers is the maximum number of workers (2x default to prevent excessive concurrency).
	maxDiscoveryWorkers = defaultDiscoveryWorkers * 2

	// defaultMaxDependencyDepth is the default maximum dependency depth for discovery.
	defaultMaxDependencyDepth = 1000

	// maxCycleRemovalAttempts is the maximum number of cycle removal attempts.
	maxCycleRemovalAttempts = 100
)

// DefaultConfigFilenames are the default Terragrunt config filenames used in discovery.
var DefaultConfigFilenames = []string{config.DefaultTerragruntConfigPath, config.DefaultStackFile}

// walkDirFunc returns the tree walk the discovery phases use, bound to the
// venv filesystem so discovery only sees what the venv exposes. The symlinks
// experiment swaps in the walk that descends into symlinked directories.
func walkDirFunc(
	v *venv.Venv,
	opts *options.TerragruntOptions,
) func(string, fs.WalkDirFunc) error {
	v.RequireFS()

	if opts == nil {
		panic("discovery.walkDirFunc: opts is nil")
	}

	if opts.Experiments.Evaluate(experiment.Symlinks) {
		return func(root string, fn fs.WalkDirFunc) error {
			return vfs.WalkDirWithSymlinks(v.FS, root, fn)
		}
	}

	return func(root string, fn fs.WalkDirFunc) error {
		return vfs.WalkDir(v.FS, root, fn)
	}
}

// stringSet is a thread-safe set of strings using map and RWMutex.
// This is more performant than sync.Map for string keys with simple bool values.
type stringSet struct {
	m  map[string]struct{}
	mu sync.RWMutex
}

// newStringSet creates a new stringSet.
func newStringSet() *stringSet {
	return &stringSet{
		m: make(map[string]struct{}),
	}
}

// LoadOrStore returns true if the key was already present (loaded),
// false if the key was newly stored.
func (s *stringSet) LoadOrStore(key string) (loaded bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.m[key]; ok {
		return true
	}

	s.m[key] = struct{}{}

	return false
}

// Load returns whether the key exists in the set.
func (s *stringSet) Load(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.m[key]

	return ok
}

// RelPathOrAbs returns target made relative to base. On filepath.Rel failure (Windows cross-volume, etc.), it warns
// and returns target unchanged so the entry still appears in output. The desc is included parenthetically in the
// warning to identify which path failed.
func RelPathOrAbs(l log.Logger, base, target, desc string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		l.Warnf(
			"could not make %q relative to %q (%s): %v; emitting as-is",
			target,
			base,
			desc,
			err,
		)

		return target
	}

	return rel
}

// isExternal checks if a component path is outside the given working directory.
// A path is considered external if it's not within or equal to the working directory.
// We conservatively evaluate paths as external if we cannot determine their absolute path.
func isExternal(fsys vfs.FS, workingDir string, componentPath string) bool {
	if workingDir == "" {
		return true
	}

	return !vfs.Within(fsys, workingDir, componentPath)
}

// componentFromDependencyPath returns a component for a dependency path. If the path already
// exists in the thread-safe components, it returns that. If the path contains a stack file,
// it creates a stack. Otherwise, it creates a unit.
func componentFromDependencyPath(
	fsys vfs.FS,
	path string,
	components *component.ThreadSafeComponents,
) component.Component {
	if existing := components.FindByPath(fsys, path); existing != nil {
		return existing
	}

	if _, err := fsys.Stat(filepath.Join(path, config.DefaultStackFile)); err == nil {
		return component.NewStack(path)
	}

	return component.NewUnit(path)
}

// createComponentFromPath creates a component from a file path if it matches one of the config filenames.
// Returns nil if the file doesn't match any of the provided filenames.
func createComponentFromPath(
	path string,
	filenames []string,
	discoveryContext *component.DiscoveryContext,
) component.Component {
	base := filepath.Base(path)
	dir := filepath.Dir(path)

	componentOfBase := func(dir, base string) component.Component {
		if base == config.DefaultStackFile {
			return component.NewStack(dir)
		}

		return component.NewUnit(dir)
	}

	for _, fname := range filenames {
		if base != fname {
			continue
		}

		c := componentOfBase(dir, base)
		if unit, ok := c.(*component.Unit); ok {
			unit.SetConfigFile(base)
		}

		if discoveryContext != nil {
			discoveryCtx := discoveryContext.Copy()
			discoveryCtx.SuggestOrigin(component.OriginPathDiscovery)

			c.SetDiscoveryContext(discoveryCtx)
		}

		return c
	}

	return nil
}

// validateNoCoexistence checks that no directory has both a unit and a stack config file.
// Returns a CoexistenceError if a directory contains both.
func validateNoCoexistence(results []DiscoveryResult) error {
	seen := make(map[string]DiscoveryResult, len(results))

	for _, result := range results {
		path := result.Component.Path()

		if existing, ok := seen[path]; ok && existing.Component.Kind() != result.Component.Kind() {
			return NewCoexistenceError(existing.Component, result.Component)
		}

		seen[path] = result
	}

	return nil
}

// deduplicateResults removes duplicate components from results by path.
func deduplicateResults(results []DiscoveryResult) []DiscoveryResult {
	seen := make(map[string]struct{}, len(results))
	unique := make([]DiscoveryResult, 0, len(results))

	for _, result := range results {
		path := result.Component.Path()
		if _, exists := seen[path]; !exists {
			seen[path] = struct{}{}

			unique = append(unique, result)
		}
	}

	return unique
}

// resultsToComponents extracts the components from discovery results.
func resultsToComponents(results []DiscoveryResult) component.Components {
	components := make(component.Components, 0, len(results))
	for _, result := range results {
		components = append(components, result.Component)
	}

	return components
}

// sanitizeReadFiles clones, removes empty strings, sorts, and deduplicates the file list.
func sanitizeReadFiles(files []string) []string {
	if len(files) == 0 {
		return []string{}
	}

	files = slices.Clone(files)
	files = slices.DeleteFunc(files, func(file string) bool {
		return len(file) == 0
	})
	slices.Sort(files)

	return slices.Compact(files)
}

// extractDependencyPaths extracts all dependency paths from a Terragrunt configuration.
func extractDependencyPaths(fsys vfs.FS, cfg *config.TerragruntConfig, c component.Component) ([]string, error) {
	if cfg == nil {
		return nil, nil
	}

	maxDedupLen := len(cfg.TerragruntDependencies)
	if cfg.Dependencies != nil {
		maxDedupLen += len(cfg.Dependencies.Paths)
	}

	deduped := make(map[string]struct{}, maxDedupLen)

	errs := make([]error, 0, maxDedupLen)

	for _, dependency := range cfg.TerragruntDependencies {
		if dependency.Enabled != nil && !*dependency.Enabled {
			continue
		}

		if !config.IsValidConfigPath(dependency.ConfigPath) {
			errs = append(errs, fmt.Errorf(
				"skipping dependency %q in %q: "+
					"config_path could not be resolved",
				dependency.Name, c.Path()))

			continue
		}

		depPath := dependency.ConfigPath.AsString()
		if !filepath.IsAbs(depPath) {
			depPath = filepath.Clean(filepath.Join(c.Path(), depPath))
		}

		deduped[vfs.ResolveForCompare(fsys, depPath)] = struct{}{}
	}

	if cfg.Dependencies != nil {
		for _, dependency := range cfg.Dependencies.Paths {
			if !filepath.IsAbs(dependency) {
				dependency = filepath.Clean(filepath.Join(c.Path(), dependency))
			}

			deduped[vfs.ResolveForCompare(fsys, dependency)] = struct{}{}
		}
	}

	depPaths := make([]string, 0, len(deduped))

	for depPath := range deduped {
		depPaths = append(depPaths, depPath)
	}

	if len(errs) > 0 {
		return depPaths, errors.Join(errs...)
	}

	return depPaths, nil
}

// storeStackConfigs parses each discovered stack's config file and stores the
// result on the stack component. The parse phase only stores unit configs, so
// this runs as a separate pass over the final component set. Parsing is
// best-effort: a stack that fails to parse is left without config and
// consumers simply omit those details.
func storeStackConfigs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	components component.Components,
) {
	for _, c := range components {
		if ctx.Err() != nil {
			return
		}

		stack, ok := c.(*component.Stack)
		if !ok {
			continue
		}

		stackDir := stack.Path()
		stackFile := filepath.Join(stackDir, stack.ConfigFile())

		// A fresh context per stack scopes it to that stack's file and values, so
		// a config referencing values.* parses instead of failing on missing
		// values (and shows its definitions in consumers like browse).
		ctx, pctx := configbridge.NewParsingContext(ctx, l, v, opts)

		values, err := config.ReadValues(ctx, pctx, l, stackDir)
		if err != nil {
			l.Debugf("Skipping stack config %s: %v", stackFile, err)

			continue
		}

		cfg, err := config.ReadStackConfigFile(ctx, l, pctx, stackFile, values)
		if err != nil {
			l.Debugf("Skipping stack config %s: %v", stackFile, err)

			continue
		}

		stack.StoreConfig(cfg)
	}
}

// stackDependencyPaths expands stack directory dependency paths into their constituent unit paths.
// Autoinclude-declared dependencies are already folded into the parsed config by the partial-parse
// merge (which honors enabled/disabled and same-name override), so they arrive via depPaths here.
func stackDependencyPaths(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	depPaths []string,
) ([]string, error) {
	_, pctx := configbridge.NewParsingContext(ctx, l, v, opts)

	// Factory builds the dir-scoped function map for each stack dir visited during expansion.
	funcsFor := inthclparse.StackFuncFactory(
		func(stackDir string) (map[string]function.Function, error) {
			return config.EarlyStackParseFunctions(ctx, l, stackDir, pctx)
		},
	)

	// Expand stack dependency paths to individual unit paths.
	expanded := make([]string, 0, len(depPaths))

	for _, depPath := range depPaths {
		// Stat upfront so a non-directory dep path (e.g. another-name.hcl) is preserved instead of
		// being passed to the parser, which would reject it as ENOTDIR. The duplication of work is
		// intentional.
		info, statErr := v.FS.Stat(depPath)
		// Real I/O errors (permission denied, etc.) must surface so a malformed DAG isn't silently
		// produced; only ENOENT is treated as "keep the raw path".
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return nil, NewStackDependencyExpansionError(depPath, statErr)
		}

		if statErr != nil || !info.IsDir() {
			expanded = append(expanded, depPath)
			continue
		}

		unitPaths, err := inthclparse.UnitPathsFromStackDir(v.FS, depPath, funcsFor)
		if err != nil {
			return nil, NewStackDependencyExpansionError(depPath, err)
		}

		if len(unitPaths) > 0 {
			expanded = append(expanded, unitPaths...)

			continue
		}

		expanded = append(expanded, depPath)
	}

	// Deduplicate expanded paths.
	seen := make(map[string]struct{}, len(expanded))
	result := make([]string, 0, len(expanded))

	for _, p := range expanded {
		if _, exists := seen[p]; exists {
			continue
		}

		seen[p] = struct{}{}
		result = append(result, p)
	}

	return result, nil
}

// dependentWalkBoundary returns where the upstream dependent walk must stop:
// the user's discovery boundary when set, otherwise the detected git root. An
// empty result means the walk is unbounded (no boundary could be determined).
func (d *Discovery) dependentWalkBoundary() string {
	if d.discoveryBoundary != "" {
		return d.discoveryBoundary
	}

	return d.gitRoot
}

// evaluationContext hands filter evaluation the settings its graph traversal
// has to honor. Discover resolves the boundary and the working directory before
// any phase runs, so this carries absolute paths rather than raw user input.
func (d *Discovery) evaluationContext() filter.EvaluationContext {
	return filter.EvaluationContext{
		WorkingDir:         d.workingDir,
		ResolvedWorkingDir: d.resolvedWorkingDir,
		DiscoveryBoundary:  d.discoveryBoundary,
	}
}

// resolveDir canonicalizes dir through the discovery filesystem, so that the
// paths a boundary is compared against are resolved by the same filesystem that
// produced them rather than by the OS underneath it. Resolution fails on a
// directory that cannot be walked to, so callers that do not require dir to
// exist have to say what an unwalkable path means to them.
func resolveDir(fsys vfs.FS, dir string) (string, error) {
	resolved, err := vfs.EvalSymlinks(fsys, dir)
	if err != nil {
		return "", err
	}

	return filepath.Clean(resolved), nil
}

// validateBoundaryDir reports whether resolved names an existing directory.
func validateBoundaryDir(fsys vfs.FS, resolved string) error {
	info, err := fsys.Stat(resolved)
	if err != nil {
		return NewDiscoveryBoundaryDirError(resolved, err)
	}

	if !info.IsDir() {
		return NewDiscoveryBoundaryDirError(resolved, errors.New("not a directory"))
	}

	return nil
}

// absBoundary canonicalizes a raw boundary value against the working directory.
func absBoundary(workingDir, boundary string) string {
	if filepath.IsAbs(boundary) {
		return filepath.Clean(boundary)
	}

	return filepath.Clean(filepath.Join(workingDir, boundary))
}

// resolveGraphBoundary canonicalizes a raw "(dir)" boundary value against
// the working directory and validates that it points at an existing directory.
// Returns the resolved absolute path.
func resolveGraphBoundary(fsys vfs.FS, workingDir, boundary string) (string, error) {
	resolved := absBoundary(workingDir, boundary)

	if err := validateBoundaryDir(fsys, resolved); err != nil {
		return "", err
	}

	return resolved, nil
}

// boundaryEnclosure states whether a discovery boundary has to enclose the
// working directory to be usable.
type boundaryEnclosure int

const (
	boundaryEnclosureOptional boundaryEnclosure = iota
	boundaryEnclosureRequired
)

// boundaryEnclosureFor reports what the given filters demand of a discovery
// boundary. Only the dependent walk starts at the working directory, so only
// it is defeated by a boundary that excludes the working directory. Dependency
// traversal starts at the matched components and follows their declared
// dependencies, so a boundary narrower than the working directory prunes that
// walk exactly as the inline "(dir)" operand does.
func boundaryEnclosureFor(filters filter.Filters) boundaryEnclosure {
	if filters.HasDependents() {
		return boundaryEnclosureRequired
	}

	return boundaryEnclosureOptional
}

// resolveDiscoveryBoundary canonicalizes a user-supplied discovery boundary against
// the working directory and validates that it is an existing directory, requiring
// it to contain the working directory only when enclosure demands it. Returns the
// resolved absolute path.
func resolveDiscoveryBoundary(
	fsys vfs.FS,
	workingDir, boundary string,
	enclosure boundaryEnclosure,
) (string, error) {
	canonical, err := util.CanonicalPath(boundary, workingDir)
	if err != nil {
		return "", NewDiscoveryBoundaryDirError(boundary, err)
	}

	resolved, err := resolveDir(fsys, canonical)
	if err != nil {
		return "", NewDiscoveryBoundaryDirError(canonical, err)
	}

	if err := validateBoundaryDir(fsys, resolved); err != nil {
		return "", err
	}

	if enclosure == boundaryEnclosureOptional {
		return resolved, nil
	}

	// Enclosure asks where the working directory sits, not whether it exists,
	// and discovery answers a working directory that holds nothing with an
	// empty result rather than an error. A path that cannot be walked to keeps
	// the spelling it was given.
	resolvedWorkingDir, err := resolveDir(fsys, workingDir)
	if err != nil {
		resolvedWorkingDir = filepath.Clean(workingDir)
	}

	rel, err := filepath.Rel(resolved, resolvedWorkingDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", NewDiscoveryBoundaryScopeError(resolved, workingDir)
	}

	return resolved, nil
}
