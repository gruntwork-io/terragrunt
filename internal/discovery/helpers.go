package discovery

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	intHclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
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
			unitFile, stackFile := existing.Component.ConfigFile(), result.Component.ConfigFile()
			if result.Component.Kind() == component.UnitKind {
				unitFile, stackFile = result.Component.ConfigFile(), existing.Component.ConfigFile()
			}

			return NewCoexistenceError(path, unitFile, stackFile)
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
// It also checks for terragrunt.autoinclude.hcl in the same directory and extracts
// dependency config_path values from it, so the DAG correctly orders units.
func extractDependencyPaths(cfg *config.TerragruntConfig, c component.Component) ([]string, error) {
	if cfg == nil {
		return nil, nil
	}

	maxDedupLen := len(cfg.TerragruntDependencies)
	if cfg.Dependencies != nil {
		maxDedupLen += len(cfg.Dependencies.Paths)
	}

	deduped := make(map[string]struct{}, maxDedupLen)

	// Check for autoinclude file and extract its dependency paths for the DAG.
	autoIncludeDeps := extractAutoIncludeDependencyPaths(c.Path())
	for _, dep := range autoIncludeDeps {
		deduped[dep] = struct{}{}
	}

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

		depPath = util.ResolvePath(depPath)
		deduped[depPath] = struct{}{}
	}

	if cfg.Dependencies != nil {
		for _, dependency := range cfg.Dependencies.Paths {
			if !filepath.IsAbs(dependency) {
				dependency = filepath.Clean(filepath.Join(c.Path(), dependency))
			}

			dependency = util.ResolvePath(dependency)
			deduped[dependency] = struct{}{}
		}
	}

	depPaths := make([]string, 0, len(deduped))

	for depPath := range deduped {
		// When the stack-dependencies experiment is active and the dependency
		// path points to a stack (directory with terragrunt.stack.hcl), expand
		// it to all constituent unit paths so the DAG correctly blocks on each unit.
		if expandedPaths := ExpandStackDependency(depPath); len(expandedPaths) > 0 {
			depPaths = append(depPaths, expandedPaths...)
		} else {
			depPaths = append(depPaths, depPath)
		}
	}

	if len(errs) > 0 {
		return depPaths, errors.Join(errs...)
	}

	return depPaths, nil
}

// ExpandStackDependency checks if a dependency path points to a stack directory.
// If so, it reads the stack config and returns paths to all generated units
// within .terragrunt-stack/. Returns nil if not a stack.
func ExpandStackDependency(depPath string) []string {
	stackFile := filepath.Join(depPath, config.DefaultStackFile)

	if !util.FileExists(stackFile) {
		return nil
	}

	// Read the stack file to discover unit paths.
	// We only need the raw HCL to extract unit path attributes — no eval context needed.
	data, err := os.ReadFile(stackFile)
	if err != nil {
		return nil
	}

	unitPaths := ExtractUnitPathsFromStackFile(data, depPath)

	return unitPaths
}

// ExtractUnitPathsFromStackFile parses a stack file and returns absolute paths
// to each unit's generated directory under .terragrunt-stack/.
func ExtractUnitPathsFromStackFile(data []byte, stackDir string) []string {
	// Use a minimal HCL parse to extract unit blocks and their path attributes.
	// We import the internal hclparse package which can handle this.
	result, err := intHclparse.ParseStackFile(data, filepath.Join(stackDir, config.DefaultStackFile), stackDir, nil)
	if err != nil {
		return nil
	}

	paths := make([]string, 0, len(result.Units))

	for _, unit := range result.Units {
		unitPath := filepath.Join(stackDir, config.StackDir, unit.Path)
		paths = append(paths, unitPath)
	}

	return paths
}

// extractAutoIncludeDependencyPaths checks for a terragrunt.autoinclude.hcl file
// in the given unit directory and extracts dependency config_path values.
// This ensures the DAG sees dependencies defined in autoinclude files during
// graph construction, even though they're not in the main terragrunt.hcl.
func extractAutoIncludeDependencyPaths(unitDir string) []string {
	autoIncludePath := filepath.Join(unitDir, config.DefaultAutoIncludeFile)

	if !util.FileExists(autoIncludePath) {
		return nil
	}

	data, err := os.ReadFile(autoIncludePath)
	if err != nil {
		return nil
	}

	// Minimal HCL parse — just extract dependency blocks and their config_path.
	file, diags := hclsyntax.ParseConfig(data, autoIncludePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	var paths []string

	for _, block := range body.Blocks {
		if block.Type != "dependency" || len(block.Labels) == 0 {
			continue
		}

		configPathAttr, exists := block.Body.Attributes["config_path"]
		if !exists {
			continue
		}

		// Evaluate config_path — it's already a resolved string literal in the generated file.
		val, valDiags := configPathAttr.Expr.Value(nil)
		if valDiags.HasErrors() || val.Type() != cty.String {
			continue
		}

		depPath := val.AsString()
		if !filepath.IsAbs(depPath) {
			depPath = filepath.Clean(filepath.Join(unitDir, depPath))
		}

		paths = append(paths, util.ResolvePath(depPath))
	}

	return paths
}
