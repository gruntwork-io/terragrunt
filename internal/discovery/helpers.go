package discovery

import (
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

const (
	// defaultDiscoveryWorkers is the default number of concurrent workers for discovery operations.
	defaultDiscoveryWorkers = 4

	// maxDiscoveryWorkers is the maximum number of workers (2x default to prevent excessive concurrency).
	maxDiscoveryWorkers = defaultDiscoveryWorkers * 2

	// channelBufferMultiplier is the channel buffer multiplier for worker pools.
	channelBufferMultiplier = 4

	// defaultMaxDependencyDepth is the default maximum dependency depth for discovery.
	defaultMaxDependencyDepth = 1000

	// maxCycleRemovalAttempts is the maximum number of cycle removal attempts.
	maxCycleRemovalAttempts = 100
)

// DefaultConfigFilenames are the default Terragrunt config filenames used in discovery.
var DefaultConfigFilenames = []string{config.DefaultTerragruntConfigPath, config.DefaultStackFile}

// resolvePath resolves symlinks in a path for consistent comparison across platforms.
// On macOS, /var is a symlink to /private/var, so paths must be resolved.
func resolvePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}

	return resolved
}

// isExternal checks if a component path is outside the given working directory.
// A path is considered external if it's not within or equal to the working directory.
// We conservatively evaluate paths as external if we cannot determine their absolute path.
func isExternal(workingDir string, componentPath string) bool {
	if workingDir == "" {
		return true
	}

	workingDirAbs, err := filepath.Abs(workingDir)
	if err != nil {
		return true
	}

	componentPathAbs, err := filepath.Abs(componentPath)
	if err != nil {
		return true
	}

	workingDirResolved, err := filepath.EvalSymlinks(workingDirAbs)
	if err != nil {
		workingDirResolved = workingDirAbs
	}

	componentPathResolved, err := filepath.EvalSymlinks(componentPathAbs)
	if err != nil {
		componentPathResolved = componentPathAbs
	}

	relPath, err := filepath.Rel(workingDirResolved, componentPathResolved)
	if err != nil {
		return true
	}

	return strings.HasPrefix(relPath, "..")
}

// skipDirIfIgnorable checks if an entire directory should be skipped based on the fact that it's
// in a directory that should never have components discovered in it.
func skipDirIfIgnorable(path string) error {
	base := filepath.Base(path)

	switch base {
	case ".git", ".terraform", ".terragrunt-cache":
		return filepath.SkipDir
	}

	return nil
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

// mergeResults merges discovered and candidate results from a phase output.
func mergeResults(output PhaseOutput) ([]DiscoveryResult, []DiscoveryResult, []error) {
	var (
		discovered []DiscoveryResult
		candidates []DiscoveryResult
		errs       []error
	)

	// Drain all channels
	done := false
	for !done {
		select {
		case result, ok := <-output.Discovered:
			if ok {
				discovered = append(discovered, result)
			}
		case result, ok := <-output.Candidates:
			if ok {
				candidates = append(candidates, result)
			}
		case err, ok := <-output.Errors:
			if ok && err != nil {
				errs = append(errs, err)
			}
		case <-output.Done:
			done = true
		}
	}

	// Drain remaining items after done signal
	for result := range output.Discovered {
		discovered = append(discovered, result)
	}

	for result := range output.Candidates {
		candidates = append(candidates, result)
	}

	for err := range output.Errors {
		if err != nil {
			errs = append(errs, err)
		}
	}

	return discovered, candidates, errs
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
