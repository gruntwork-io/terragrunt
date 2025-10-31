package discovery

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// DependencyDiscovery is the configuration for a DependencyDiscovery.
type DependencyDiscovery struct {
	discoveryContext    *component.DiscoveryContext
	components          *component.ThreadSafeComponents
	parserOptions       []hclparse.Option
	maxDepth            int
	discoverExternal    bool
	suppressParseErrors bool
	seenComponents      map[string]struct{}
	workingDir          string
}

func NewDependencyDiscovery(components *component.ThreadSafeComponents) *DependencyDiscovery {
	return &DependencyDiscovery{
		components:     components,
		seenComponents: make(map[string]struct{}),
	}
}

// WithMaxDepth sets the maximum depth for dependency discovery.
func (dd *DependencyDiscovery) WithMaxDepth(maxDepth int) *DependencyDiscovery {
	dd.maxDepth = maxDepth
	return dd
}

// WithSuppressParseErrors sets the SuppressParseErrors flag to true.
func (dd *DependencyDiscovery) WithSuppressParseErrors() *DependencyDiscovery {
	dd.suppressParseErrors = true

	return dd
}

// WithDiscoverExternalDependencies sets the discoverExternal flag to true,
// which determines whether to discover and include external dependencies in the final results.
func (dd *DependencyDiscovery) WithDiscoverExternalDependencies() *DependencyDiscovery {
	dd.discoverExternal = true

	return dd
}

// WithParserOptions sets custom HCL parser options for dependency discovery.
func (dd *DependencyDiscovery) WithParserOptions(options []hclparse.Option) *DependencyDiscovery {
	dd.parserOptions = options
	return dd
}

func (dd *DependencyDiscovery) WithDiscoveryContext(discoveryContext *component.DiscoveryContext) *DependencyDiscovery {
	dd.discoveryContext = discoveryContext

	return dd
}

// WithWorkingDir sets the working directory for determining if dependencies are external.
func (dd *DependencyDiscovery) WithWorkingDir(workingDir string) *DependencyDiscovery {
	dd.workingDir = workingDir
	return dd
}

func (dd *DependencyDiscovery) DiscoverAllDependencies(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	startingComponents component.Components,
) error {
	errs := []error{}

	for _, c := range startingComponents {
		dd.seenComponents[c.Path()] = struct{}{}

		if _, ok := c.(*component.Stack); ok {
			continue
		}

		err := dd.DiscoverDependencies(ctx, l, opts, c, dd.maxDepth)
		if err != nil {
			errs = append(errs, errors.New(err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (dd *DependencyDiscovery) DiscoverDependencies(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	dComponent component.Component,
	depthRemaining int,
) error {
	if depthRemaining <= 0 {
		return errors.New("max dependency depth reached while discovering dependencies")
	}

	// Stack configs don't have dependencies (at least for now),
	// so we can return early.
	if _, ok := dComponent.(*component.Stack); ok {
		return nil
	}

	unit, ok := dComponent.(*component.Unit)
	if !ok {
		return errors.New("expected Unit component but got different type")
	}

	// This should only happen if we're discovering an ancestor dependency.
	if unit.Config() == nil {
		err := Parse(dComponent, ctx, l, opts, dd.suppressParseErrors, dd.parserOptions)
		if err != nil {
			return errors.New(err)
		}
	}

	terragruntCfg := unit.Config()

	errs := []error{}
	depPaths, extractErrs := extractDependencyPaths(terragruntCfg, dComponent)
	errs = append(errs, extractErrs...)

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if len(depPaths) == 0 {
		return nil
	}

	deduped := make(map[string]struct{}, len(depPaths))

	for _, depPath := range depPaths {
		deduped[depPath] = struct{}{}
	}

	for depPath := range deduped {
		depComponent := dd.dependencyToDiscover(dComponent, depPath)
		if depComponent == nil {
			continue
		}

		err := dd.DiscoverDependencies(ctx, l, opts, depComponent, depthRemaining-1)
		if err != nil {
			errs = append(errs, errors.New(err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// dependencyToDiscover resolves a dependency path to a component that also needs to have its dependencies discovered.
//
// It handles checking if the component already exists from a prior phase of discovery, creating a new component if not,
// marking as external if it's outside the working directory of discovery, and linking dependencies.
// Returns nil if the dependency shouldn't be involved in discovery any further (e.g., already processed or ignored).
func (dd *DependencyDiscovery) dependencyToDiscover(
	dComponent component.Component,
	depPath string,
) component.Component {
	c := dd.components.FindByPath(depPath)
	if c != nil {
		dd.seenComponents[depPath] = struct{}{}
		dComponent.AddDependency(c)
		return c
	}

	_, seen := dd.seenComponents[depPath]
	if seen {
		c := dd.components.FindByPath(depPath)
		if c != nil {
			dComponent.AddDependency(c)
			return nil
		}

		return nil
	}

	dd.seenComponents[depPath] = struct{}{}

	isExternal := dd.isExternal(depPath)

	depComponent := component.NewUnit(depPath)

	if isExternal {
		depComponent.SetExternal()
	}

	if dd.discoveryContext != nil {
		depComponent.SetDiscoveryContext(dd.discoveryContext)
	}

	dComponent.AddDependency(depComponent)

	if !isExternal || dd.discoverExternal {
		dd.components.AddComponent(depComponent)
		return depComponent
	}

	return nil
}

// isExternal checks if a component path is outside the working directory.
// A path is considered external if it's not within or equal to the working directory.
func (dd *DependencyDiscovery) isExternal(componentPath string) bool {
	if dd.workingDir == "" {
		return true
	}

	workingDirAbs, err := filepath.Abs(dd.workingDir)
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

	relPath = filepath.Clean(relPath)

	return strings.HasPrefix(relPath, "..")
}
