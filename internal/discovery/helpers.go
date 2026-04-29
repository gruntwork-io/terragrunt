package discovery

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
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

// isExternal checks if a component path is outside the given working directory.
// A path is considered external if it's not within or equal to the working directory.
// We conservatively evaluate paths as external if we cannot determine their absolute path.
func isExternal(workingDir string, componentPath string) bool {
	if workingDir == "" {
		return true
	}

	workingDirClean := filepath.Clean(workingDir)
	componentPathClean := filepath.Clean(componentPath)

	workingDirResolved, err := filepath.EvalSymlinks(workingDirClean)
	if err != nil {
		workingDirResolved = workingDirClean
	}

	componentPathResolved, err := filepath.EvalSymlinks(componentPathClean)
	if err != nil {
		componentPathResolved = componentPathClean
	}

	relPath, err := filepath.Rel(workingDirResolved, componentPathResolved)
	if err != nil {
		return true
	}

	return relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator))
}

// componentFromDependencyPath returns a component for a dependency path. If the path already
// exists in the thread-safe components, it returns that. If the path contains a stack file,
// it creates a stack. Otherwise, it creates a unit.
func componentFromDependencyPath(path string, components *component.ThreadSafeComponents) component.Component {
	if existing := components.FindByPath(path); existing != nil {
		return existing
	}

	if _, err := os.Stat(filepath.Join(path, config.DefaultStackFile)); err == nil {
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
func extractDependencyPaths(cfg *config.TerragruntConfig, c component.Component) ([]string, error) {
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
			errs = append(errs, errors.Errorf("skipping dependency %q in %q: config_path could not be resolved", dependency.Name, c.Path()))
			continue
		}

		depPath := dependency.ConfigPath.AsString()
		if !filepath.IsAbs(depPath) {
			depPath = filepath.Clean(filepath.Join(c.Path(), depPath))
		}

		deduped[util.ResolvePath(depPath)] = struct{}{}
	}

	if cfg.Dependencies != nil {
		for _, dependency := range cfg.Dependencies.Paths {
			if !filepath.IsAbs(dependency) {
				dependency = filepath.Clean(filepath.Join(c.Path(), dependency))
			}

			deduped[util.ResolvePath(dependency)] = struct{}{}
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

// stackDependencyPaths returns additional dependency paths from autoinclude
// files and expands stack directory paths into constituent unit paths.
// Only called when the StackDependencies experiment is enabled.
func stackDependencyPaths(fs vfs.FS, depPaths []string, c component.Component) ([]string, error) {
	// Add dependencies declared in autoinclude files.
	autoIncludeDeps, err := inthclparse.AutoIncludeDependencyPaths(fs, c.Path())
	if err != nil {
		return nil, err
	}

	for _, dep := range autoIncludeDeps {
		depPaths = append(depPaths, util.ResolvePath(dep))
	}

	// Expand stack dependency paths to individual unit paths.
	expanded := make([]string, 0, len(depPaths))

	for _, depPath := range depPaths {
		// Only expand directories: dependency paths can also point at non-default-named config files (e.g. another-name.hcl), which the stack-file parser would otherwise reject as "not a directory".
		if info, statErr := fs.Stat(depPath); statErr != nil || !info.IsDir() {
			expanded = append(expanded, depPath)
			continue
		}

		unitPaths, err := inthclparse.UnitPathsFromStackDir(fs, depPath)
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
