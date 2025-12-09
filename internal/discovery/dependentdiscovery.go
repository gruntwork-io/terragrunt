package discovery

import (
	"context"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/sync/errgroup"
)

// DependentDiscovery holds the state for dependent discovery traversal.
type DependentDiscovery struct {
	visitedDirs         map[string]struct{}
	knownComponentPaths map[string]struct{}
	discoveryContext    *component.DiscoveryContext
	opts                *options.TerragruntOptions
	mu                  *sync.RWMutex
	components          *component.ThreadSafeComponents
	gitRoot             string
	parserOptions       []hclparse.Option
	filenames           []string
	maxDepth            int
	numWorkers          int
	suppressParseErrors bool
	discoverExternal    bool
}

// NewDependentDiscovery creates a new DependentDiscovery with the given configuration.
func NewDependentDiscovery(components *component.ThreadSafeComponents) *DependentDiscovery {
	return &DependentDiscovery{
		components:          components,
		mu:                  &sync.RWMutex{},
		knownComponentPaths: make(map[string]struct{}),
		visitedDirs:         make(map[string]struct{}),
		filenames:           DefaultConfigFilenames,
		numWorkers:          runtime.NumCPU(),
	}
}

// WithMaxDepth sets the maximum depth for dependent discovery.
func (dd *DependentDiscovery) WithMaxDepth(maxDepth int) *DependentDiscovery {
	dd.maxDepth = maxDepth
	return dd
}

// WithSuppressParseErrors sets the SuppressParseErrors flag to true.
func (dd *DependentDiscovery) WithSuppressParseErrors() *DependentDiscovery {
	dd.suppressParseErrors = true
	return dd
}

// WithParserOptions sets custom HCL parser options for dependent discovery.
func (dd *DependentDiscovery) WithParserOptions(options []hclparse.Option) *DependentDiscovery {
	dd.parserOptions = options
	return dd
}

// WithDiscoveryContext sets the discovery context for dependent discovery.
func (dd *DependentDiscovery) WithDiscoveryContext(discoveryContext *component.DiscoveryContext) *DependentDiscovery {
	dd.discoveryContext = discoveryContext
	return dd
}

// WithNumWorkers sets the number of concurrent workers for dependent discovery operations.
func (dd *DependentDiscovery) WithNumWorkers(numWorkers int) *DependentDiscovery {
	dd.numWorkers = numWorkers
	return dd
}

// WithDiscoverExternalDependencies sets the discoverExternal flag to true,
// which determines whether to discover and include external dependencies in the final results.
func (dd *DependentDiscovery) WithDiscoverExternalDependencies() *DependentDiscovery {
	dd.discoverExternal = true
	return dd
}

// WithOpts sets the Terragrunt options for dependent discovery.
// If opts has a valid Parallelism setting, it will update numWorkers (unless explicitly set via WithNumWorkers).
func (dd *DependentDiscovery) WithOpts(opts *options.TerragruntOptions) *DependentDiscovery {
	dd.opts = opts
	// Update numWorkers from opts.Parallelism if it's valid and numWorkers is at default
	if opts != nil && opts.Parallelism != math.MaxInt32 && opts.Parallelism > 0 {
		dd.numWorkers = opts.Parallelism
	}

	return dd
}

// WithGitRoot sets the git root directory for dependent discovery.
func (dd *DependentDiscovery) WithGitRoot(gitRoot string) *DependentDiscovery {
	dd.gitRoot = gitRoot
	return dd
}

// WithFilenames sets the config filenames for dependent discovery.
func (dd *DependentDiscovery) WithFilenames(filenames []string) *DependentDiscovery {
	dd.filenames = filenames
	return dd
}

// Discover discovers dependents by traversing up the file system from starting components
// to find new Terragrunt configs in parent directories that depend on the starting components.
// It recursively discovers dependents of newly found dependents.
func (dd *DependentDiscovery) Discover(
	ctx context.Context,
	l log.Logger,
	startingComponents component.Components,
) error {
	var (
		errs []error
		mu   sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(dd.numWorkers)

	for _, c := range startingComponents {
		g.Go(func() error {
			err := dd.discoverDependents(ctx, l, c, filepath.Dir(c.Path()), dd.maxDepth)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// discoverDependents discovers dependents for a single target component by traversing up parent directories
// and walking down child directories to find Terragrunt configs that depend on the targetComponent.
func (dd *DependentDiscovery) discoverDependents(
	ctx context.Context,
	l log.Logger,
	target component.Component,
	currentDir string,
	depthRemaining int,
) error {
	if depthRemaining <= 0 {
		return errors.New("max dependent discovery depth reached")
	}

	if currentDir == filepath.Dir(currentDir) {
		return nil
	}

	if dd.gitRoot != "" && currentDir != dd.gitRoot && !strings.HasPrefix(currentDir, dd.gitRoot) {
		return nil
	}

	var (
		errs []error
		mu   sync.Mutex
	)

	g, walkCtx := errgroup.WithContext(ctx)
	g.SetLimit(dd.numWorkers)

	candidates := make(chan component.Component, dd.numWorkers)

	// Walk directory and discover candidates
	g.Go(func() error {
		defer close(candidates)

		walkFn := filepath.WalkDir
		if dd.opts != nil && dd.opts.Experiments.Evaluate(experiment.Symlinks) {
			walkFn = util.WalkDirWithSymlinks
		}

		err := walkFn(currentDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			select {
			case <-walkCtx.Done():
				return walkCtx.Err()
			default:
			}

			if d.IsDir() {
				if dd.isVisited(path) {
					return filepath.SkipDir
				}

				dd.markVisited(path)

				if err := skipDirIfIgnorable(path); err != nil {
					return err
				}

				return nil
			}

			base := filepath.Base(path)
			if !slices.Contains(dd.filenames, base) {
				return nil
			}

			candidate := dd.componentFromPath(path)

			select {
			case <-walkCtx.Done():
				return walkCtx.Err()
			case candidates <- candidate:
			}

			return nil
		})
		if err != nil {
			mu.Lock()

			errs = append(errs, err)

			mu.Unlock()
		}

		return nil
	})

	discoveredDependents := make(chan component.Component, dd.numWorkers)

	// Process candidates concurrently
	g.Go(func() error {
		defer close(discoveredDependents)

		// Resolve target path once for all candidates (handles symlinks like macOS /var -> /private/var)
		resolvedTargetPath := resolvePath(target.Path())

		for candidate := range candidates {
			if dd.discoveryContext != nil {
				candidate.SetDiscoveryContext(dd.discoveryContext)
			}

			if dd.isChecked(candidate) {
				continue
			}

			dd.markChecked(candidate)

			// Skip stacks for dependent discovery for now.
			if _, ok := candidate.(*component.Stack); ok {
				continue
			}

			unit, ok := candidate.(*component.Unit)
			if !ok {
				continue
			}

			cfg := unit.Config()
			if cfg == nil {
				err := Parse(candidate, walkCtx, l, dd.opts, dd.suppressParseErrors, dd.parserOptions)
				if err != nil {
					mu.Lock()

					errs = append(errs, err)

					mu.Unlock()

					continue
				}

				cfg = unit.Config()
			}

			deps, err := extractDependencyPaths(cfg, candidate)
			if err != nil {
				mu.Lock()

				errs = append(errs, errors.New(err))

				mu.Unlock()

				continue
			}

			dependsOnTarget := false

			for _, dep := range deps {
				c := dd.componentFromDependency(dep)
				if c == nil {
					continue
				}

				isExternal := isExternal(dd.discoveryContext.WorkingDir, c.Path())

				if isExternal {
					c.SetExternal()
				}

				candidate.AddDependency(c)

				// Use resolved paths for comparison to handle symlinks (e.g., macOS /var -> /private/var)
				if resolvePath(dep) == resolvedTargetPath {
					dependsOnTarget = true
				}
			}

			if dependsOnTarget {
				isExternal := isExternal(dd.discoveryContext.WorkingDir, candidate.Path())

				if !isExternal || dd.discoverExternal {
					dd.ensureComponent(candidate)

					select {
					case <-walkCtx.Done():
						return walkCtx.Err()
					case discoveredDependents <- candidate:
					}
				}
			}
		}

		return nil
	})

	// Process discovered dependents recursively
	g.Go(func() error {
		for dependent := range discoveredDependents {
			select {
			case <-walkCtx.Done():
				return walkCtx.Err()
			default:
			}

			g.Go(func() error {
				// Use a copy with fresh state maps for recursive discovery of discovered dependents
				recursiveDD := dd.copyForDiscoveredDependent()

				err := recursiveDD.discoverDependents(
					walkCtx,
					l,
					dependent,
					filepath.Dir(dependent.Path()),
					depthRemaining-1,
				)
				if err != nil {
					mu.Lock()

					errs = append(errs, err)

					mu.Unlock()
				}

				return nil
			})
		}

		return nil
	})

	// Traverse parent directory concurrently
	parentDir := filepath.Dir(currentDir)
	if parentDir != currentDir && depthRemaining > 0 {
		g.Go(func() error {
			err := dd.discoverDependents(walkCtx, l, target, parentDir, depthRemaining-1)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)

				mu.Unlock()
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// componentFromPath finds or creates a component based on a file path.
func (dd *DependentDiscovery) componentFromPath(path string) component.Component {
	componentPath := filepath.Dir(path)

	c := dd.components.FindByPath(componentPath)
	if c != nil {
		return c
	}

	base := filepath.Base(path)
	if base == config.DefaultStackFile {
		return component.NewStack(componentPath)
	}

	return component.NewUnit(componentPath)
}

// componentFromDependency finds or creates a component based on a dependency path.
func (dd *DependentDiscovery) componentFromDependency(path string) component.Component {
	c := dd.components.FindByPath(path)
	if c != nil {
		return c
	}

	if _, err := os.Stat(filepath.Join(path, config.DefaultStackFile)); err == nil {
		return component.NewStack(path)
	}

	return component.NewUnit(path)
}

// markVisited marks a path as visited, meaning it has been walked through.
func (dd *DependentDiscovery) markVisited(path string) {
	dd.mu.Lock()
	defer dd.mu.Unlock()

	dd.visitedDirs[path] = struct{}{}
}

// isVisited checks if a path has been visited, meaning it has been walked through.
func (dd *DependentDiscovery) isVisited(path string) bool {
	dd.mu.RLock()
	defer dd.mu.RUnlock()

	_, visited := dd.visitedDirs[path]

	return visited
}

// markChecked marks a component as checked, meaning it has been discovered, and checked for dependencies.
func (dd *DependentDiscovery) markChecked(component component.Component) {
	dd.mu.Lock()
	defer dd.mu.Unlock()

	dd.knownComponentPaths[component.Path()] = struct{}{}
}

// isChecked checks if a component has been checked, meaning it has been discovered, and checked for dependencies.
func (dd *DependentDiscovery) isChecked(component component.Component) bool {
	dd.mu.RLock()
	defer dd.mu.RUnlock()

	_, checked := dd.knownComponentPaths[component.Path()]

	return checked
}

// ensureComponent adds a component to the components list if it's not already present.
func (dd *DependentDiscovery) ensureComponent(component component.Component) {
	dd.components.EnsureComponent(component)
}

// copyForDiscoveredDependent creates a copy of the DependentDiscovery with fresh state maps
// for recursing on newly discovered dependents. This ensures that each recursive discovery
// starts with a clean visited/checked state while preserving configuration and shared mutex.
func (dd *DependentDiscovery) copyForDiscoveredDependent() *DependentDiscovery {
	dd.mu.RLock()
	defer dd.mu.RUnlock()

	dependentDiscovery := *dd

	// We reset these to ensure that each recursive discovery for a
	// discovered dependent starts with a clean visited/checked state
	dependentDiscovery.visitedDirs = make(map[string]struct{})
	dependentDiscovery.knownComponentPaths = make(map[string]struct{})

	return &dependentDiscovery
}
