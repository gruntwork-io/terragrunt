// Package discovery provides functionality for discovering Terragrunt configurations.
package discovery

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/zclconf/go-cty/cty"
)

const (
	// ConfigTypeUnit is the type of Terragrunt configuration for a unit.
	ConfigTypeUnit ConfigType = "unit"
	// ConfigTypeStack is the type of Terragrunt configuration for a stack.
	ConfigTypeStack ConfigType = "stack"
)

// ConfigType is the type of Terragrunt configuration.
type ConfigType string

// Sort is the sort order of the discovered configurations.
type Sort string

// DiscoveredConfig represents a discovered Terragrunt configuration.
type DiscoveredConfig struct {
	Parsed           *config.TerragruntConfig
	DiscoveryContext *DiscoveryContext

	Type ConfigType
	Path string

	Dependencies DiscoveredConfigs

	External bool
}

// DiscoveryContext is the context in which
// a DiscoveredConfig was discovered.
//
// It's useful to know this information,
// because it can help us determine how the
// DiscoveredConfig should be processed later.
type DiscoveryContext struct {
	Cmd  string
	Args []string
}

// DiscoveredConfigs is a list of discovered Terragrunt configurations.
type DiscoveredConfigs []*DiscoveredConfig

// Discovery is the configuration for a Terragrunt discovery.
type Discovery struct {
	// discoveryContext is the context in which the discovery is happening.
	discoveryContext *DiscoveryContext

	// workingDir is the directory to search for Terragrunt configurations.
	workingDir string

	// sort determines the sort order of the discovered configurations.
	sort Sort

	// hiddenDirMemo is a memoization of hidden directories.
	hiddenDirMemo []string

	// maxDependencyDepth is the maximum depth of the dependency tree to discover.
	maxDependencyDepth int

	// hidden determines whether to detect configurations in hidden directories.
	hidden bool

	// requiresParse is true when the discovery requires parsing Terragrunt configurations.
	requiresParse bool

	// discoverDependencies determines whether to discover dependencies.
	discoverDependencies bool

	// parseExclude determines whether to parse exclude configurations.
	parseExclude bool

	// parseInclude determines whether to parse include configurations.
	parseInclude bool

	// discoverExternalDependencies determines whether to discover external dependencies.
	discoverExternalDependencies bool

	// suppressParseErrors determines whether to suppress errors when parsing Terragrunt configurations.
	suppressParseErrors bool
}

// DiscoveryOption is a function that modifies a Discovery.
type DiscoveryOption func(*Discovery)

// NewDiscovery creates a new Discovery.
func NewDiscovery(dir string, opts ...DiscoveryOption) *Discovery {
	discovery := &Discovery{
		workingDir: dir,
		hidden:     false,
	}

	for _, opt := range opts {
		opt(discovery)
	}

	return discovery
}

// WithHidden sets the Hidden flag to true.
func (d *Discovery) WithHidden() *Discovery {
	d.hidden = true

	return d
}

// WithSort sets the Sort flag to the given sort.
func (d *Discovery) WithSort(sort Sort) *Discovery {
	d.sort = sort

	return d
}

// WithDiscoverDependencies sets the DiscoverDependencies flag to true.
func (d *Discovery) WithDiscoverDependencies() *Discovery {
	d.discoverDependencies = true

	d.requiresParse = true

	if d.maxDependencyDepth == 0 {
		d.maxDependencyDepth = 1000
	}

	return d
}

// WithParseExclude sets the ParseExclude flag to true.
func (d *Discovery) WithParseExclude() *Discovery {
	d.parseExclude = true

	d.requiresParse = true

	return d
}

// WithParseInclude sets the ParseExclude flag to true.
func (d *Discovery) WithParseInclude() *Discovery {
	d.parseInclude = true

	d.requiresParse = true

	return d
}

// WithMaxDependencyDepth sets the MaxDependencyDepth flag to the given depth.
func (d *Discovery) WithMaxDependencyDepth(depth int) *Discovery {
	d.maxDependencyDepth = depth

	return d
}

// WithDiscoverExternalDependencies sets the DiscoverExternalDependencies flag to true.
func (d *Discovery) WithDiscoverExternalDependencies() *Discovery {
	d.discoverExternalDependencies = true

	return d
}

// WithSuppressParseErrors sets the SuppressParseErrors flag to true.
func (d *Discovery) WithSuppressParseErrors() *Discovery {
	d.suppressParseErrors = true

	return d
}

// WithDiscoveryContext sets the DiscoveryContext flag to the given context.
func (d *Discovery) WithDiscoveryContext(discoveryContext *DiscoveryContext) *Discovery {
	d.discoveryContext = discoveryContext

	return d
}

// String returns a string representation of a DiscoveredConfig.
func (c *DiscoveredConfig) String() string {
	return c.Path
}

// ContainsDependencyInAncestry returns true if the DiscoveredConfig or any of
// its dependencies contains the given path as a dependency.
func (c *DiscoveredConfig) ContainsDependencyInAncestry(path string) bool {
	for _, dep := range c.Dependencies {
		if dep.Path == path {
			return true
		}

		if dep.ContainsDependencyInAncestry(path) {
			return true
		}
	}

	return false
}

// Parse parses the discovered configurations.
func (c *DiscoveredConfig) Parse(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, suppressParseErrors bool) error {
	parseOpts := opts.Clone()
	parseOpts.WorkingDir = c.Path

	// Suppress logging to avoid cluttering the output.
	parseOpts.Writer = io.Discard
	parseOpts.ErrWriter = io.Discard
	parseOpts.SkipOutput = true

	filename := config.DefaultTerragruntConfigPath

	if c.Type == ConfigTypeStack {
		filename = config.DefaultStackFile
	}

	parseOpts.TerragruntConfigPath = filepath.Join(parseOpts.WorkingDir, filename)

	parsingCtx := config.NewParsingContext(ctx, l, parseOpts).WithDecodeList(
		config.DependenciesBlock,
		config.DependencyBlock,
		config.FeatureFlagsBlock,
		config.ExcludeBlock,
	)

	//nolint: contextcheck
	cfg, err := config.ParseConfigFile(parsingCtx, l, parseOpts.TerragruntConfigPath, nil)
	if err != nil {
		if !suppressParseErrors || cfg == nil {
			l.Debugf("Unrecoverable parse error for %s: %s", parseOpts.TerragruntConfigPath, err)

			return errors.New(err)
		}

		l.Debugf("Suppressing parse error for %s: %s", parseOpts.TerragruntConfigPath, err)
	}

	c.Parsed = cfg

	return nil
}

// isInHiddenDirectory returns true if the path is in a hidden directory.
func (d *Discovery) isInHiddenDirectory(path string) bool {
	for _, hiddenDir := range d.hiddenDirMemo {
		if strings.HasPrefix(path, hiddenDir) {
			return true
		}
	}

	hiddenPath := ""

	parts := strings.SplitSeq(path, string(os.PathSeparator))
	for part := range parts {
		hiddenPath = filepath.Join(hiddenPath, part)

		if strings.HasPrefix(part, ".") {
			d.hiddenDirMemo = append(d.hiddenDirMemo, hiddenPath)

			return true
		}
	}

	return false
}

// Discover discovers Terragrunt configurations in the WorkingDir.
func (d *Discovery) Discover(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (DiscoveredConfigs, error) {
	var cfgs DiscoveredConfigs

	processFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.New(err)
		}

		if info.IsDir() {
			return nil
		}

		if !d.hidden && d.isInHiddenDirectory(path) {
			return nil
		}

		switch filepath.Base(path) {
		case config.DefaultTerragruntConfigPath:
			cfg := &DiscoveredConfig{
				Type: ConfigTypeUnit,
				Path: filepath.Dir(path),
			}

			if d.discoveryContext != nil {
				cfg.DiscoveryContext = d.discoveryContext
			}

			cfgs = append(cfgs, cfg)
		case config.DefaultStackFile:
			cfg := &DiscoveredConfig{
				Type: ConfigTypeStack,
				Path: filepath.Dir(path),
			}

			if d.discoveryContext != nil {
				cfg.DiscoveryContext = d.discoveryContext
			}

			cfgs = append(cfgs, cfg)
		}

		return nil
	}

	walkFn := filepath.Walk
	if opts.Experiments.Evaluate(experiment.Symlinks) {
		walkFn = util.WalkWithSymlinks
	}

	if err := walkFn(d.workingDir, processFn); err != nil {
		return cfgs, errors.New(err)
	}

	errs := []error{}

	// We do an initial parse loop if we know we need to parse configurations,
	// as we might need to parse configurations for multiple reasons.
	// e.g. dependencies, exclude, etc.
	if d.requiresParse {
		for _, cfg := range cfgs {
			err := cfg.Parse(ctx, l, opts, d.suppressParseErrors)
			if err != nil {
				errs = append(errs, errors.New(err))
			}
		}
	}

	if d.discoverDependencies {
		err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "discover_dependencies", map[string]any{
			"working_dir":                    d.workingDir,
			"config_count":                   len(cfgs),
			"discover_external_dependencies": d.discoverExternalDependencies,
			"max_dependency_depth":           d.maxDependencyDepth,
		}, func(ctx context.Context) error {
			dependencyDiscovery := NewDependencyDiscovery(cfgs, d.maxDependencyDepth)

			if d.discoveryContext != nil {
				dependencyDiscovery = dependencyDiscovery.WithDiscoveryContext(d.discoveryContext)
			}

			if d.discoverExternalDependencies {
				dependencyDiscovery = dependencyDiscovery.WithDiscoverExternalDependencies()
			}

			if d.suppressParseErrors {
				dependencyDiscovery = dependencyDiscovery.WithSuppressParseErrors()
			}

			err := dependencyDiscovery.DiscoverAllDependencies(ctx, l, opts)
			if err != nil {
				l.Warnf("Parsing errors where encountered while discovering dependencies. They were suppressed, and can be found in the debug logs.")

				l.Debugf("Errors: %w", err)
			}

			cfgs = dependencyDiscovery.cfgs

			return nil
		})
		if err != nil {
			return cfgs, errors.New(err)
		}

		err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "discovery_cycle_check", map[string]any{
			"working_dir":  d.workingDir,
			"config_count": len(cfgs),
		}, func(ctx context.Context) error {
			if _, err := cfgs.CycleCheck(); err != nil {
				l.Warnf("Cycle detected in dependency graph, attempting removal of cycles.")

				l.Debugf("Cycle: %w", err)

				cfgs, err = cfgs.RemoveCycles()
				if err != nil {
					errs = append(errs, errors.New(err))
				}
			}

			return nil
		})
		if err != nil {
			return cfgs, errors.New(err)
		}
	}

	if len(errs) > 0 {
		return cfgs, errors.Join(errs...)
	}

	return cfgs, nil
}

// DependencyDiscovery is the configuration for a DependencyDiscovery.
type DependencyDiscovery struct {
	discoveryContext    *DiscoveryContext
	cfgs                DiscoveredConfigs
	depthRemaining      int
	discoverExternal    bool
	suppressParseErrors bool
}

// DependencyDiscoveryOption is a function that modifies a DependencyDiscovery.
type DependencyDiscoveryOption func(*DependencyDiscovery)

func NewDependencyDiscovery(cfgs DiscoveredConfigs, depthRemaining int) *DependencyDiscovery {
	return &DependencyDiscovery{
		cfgs:           cfgs,
		depthRemaining: depthRemaining,
	}
}

// WithSuppressParseErrors sets the SuppressParseErrors flag to true.
func (d *DependencyDiscovery) WithSuppressParseErrors() *DependencyDiscovery {
	d.suppressParseErrors = true

	return d
}

// WithDiscoverExternalDependencies sets the DiscoverExternalDependencies flag to true.
func (d *DependencyDiscovery) WithDiscoverExternalDependencies() *DependencyDiscovery {
	d.discoverExternal = true

	return d
}

func (d *DependencyDiscovery) WithDiscoveryContext(discoveryContext *DiscoveryContext) *DependencyDiscovery {
	d.discoveryContext = discoveryContext

	return d
}

func (d *DependencyDiscovery) DiscoverAllDependencies(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	errs := []error{}

	for _, cfg := range d.cfgs {
		if cfg.Type == ConfigTypeStack {
			continue
		}

		err := d.DiscoverDependencies(ctx, l, opts, cfg)
		if err != nil {
			errs = append(errs, errors.New(err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (d *DependencyDiscovery) DiscoverDependencies(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, dCfg *DiscoveredConfig) error {
	if d.depthRemaining <= 0 {
		return errors.New("max dependency depth reached while discovering dependencies")
	}

	// Stack configs don't have dependencies (at least for now),
	// so we can return early.
	if dCfg.Type == ConfigTypeStack {
		return nil
	}

	// This should only happen if we're discovering an ancestor dependency.
	if dCfg.Parsed == nil {
		err := dCfg.Parse(ctx, l, opts, d.suppressParseErrors)
		if err != nil {
			return errors.New(err)
		}
	}

	dependencyBlocks := dCfg.Parsed.TerragruntDependencies

	depPaths := make([]string, 0, len(dependencyBlocks))

	errs := []error{}

	for _, dependency := range dependencyBlocks {
		if dependency.ConfigPath.Type() != cty.String {
			errs = append(errs, errors.New("dependency config path is not a string"))

			continue
		}

		depPath := dependency.ConfigPath.AsString()

		if !filepath.IsAbs(depPath) {
			depPath = filepath.Join(dCfg.Path, depPath)
		}

		depPaths = append(depPaths, depPath)
	}

	if dCfg.Parsed.Dependencies != nil {
		for _, dependency := range dCfg.Parsed.Dependencies.Paths {
			if !filepath.IsAbs(dependency) {
				dependency = filepath.Join(dCfg.Path, dependency)
			}

			depPaths = append(depPaths, dependency)
		}
	}

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
		external := true

		for _, c := range d.cfgs {
			if c.Path == depPath {
				external = false

				dCfg.Dependencies = append(dCfg.Dependencies, c)

				continue
			}
		}

		if external {
			ext := &DiscoveredConfig{
				Type:     ConfigTypeUnit,
				Path:     depPath,
				External: true,
			}

			if d.discoveryContext != nil {
				ext.DiscoveryContext = d.discoveryContext
			}

			dCfg.Dependencies = append(dCfg.Dependencies, ext)

			if d.discoverExternal {
				d.cfgs = append(d.cfgs, ext)

				err := d.DiscoverDependencies(ctx, l, opts, ext)
				if err != nil {
					errs = append(errs, errors.New(err))
				}
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// Sort sorts the DiscoveredConfigs by path.
func (c DiscoveredConfigs) Sort() DiscoveredConfigs {
	sort.Slice(c, func(i, j int) bool {
		return c[i].Path < c[j].Path
	})

	return c
}

// Filter filters the DiscoveredConfigs by config type.
func (c DiscoveredConfigs) Filter(configType ConfigType) DiscoveredConfigs {
	filtered := make(DiscoveredConfigs, 0, len(c))

	for _, config := range c {
		if config.Type == configType {
			filtered = append(filtered, config)
		}
	}

	return filtered
}

// FilterByPath filters the DiscoveredConfigs by path.
func (c DiscoveredConfigs) FilterByPath(path string) DiscoveredConfigs {
	filtered := make(DiscoveredConfigs, 0, 1)

	for _, config := range c {
		if config.Path == path {
			filtered = append(filtered, config)
		}
	}

	return filtered
}

// RemoveByPath removes the DiscoveredConfig with the given path from the DiscoveredConfigs.
func (c DiscoveredConfigs) RemoveByPath(path string) DiscoveredConfigs {
	filtered := make(DiscoveredConfigs, 0, len(c)-1)

	for _, config := range c {
		if config.Path != path {
			filtered = append(filtered, config)
		}
	}

	return filtered
}

// Paths returns the paths of the DiscoveredConfigs.
func (c DiscoveredConfigs) Paths() []string {
	paths := make([]string, 0, len(c))
	for _, config := range c {
		paths = append(paths, config.Path)
	}

	return paths
}

// CycleCheck checks for cycles in the dependency graph.
// If a cycle is detected, it returns the first DiscoveredConfig that is part of the cycle, and an error.
// If no cycle is detected, it returns nil and nil.
func (c DiscoveredConfigs) CycleCheck() (*DiscoveredConfig, error) {
	visited := make(map[string]bool)
	inPath := make(map[string]bool)

	var checkCycle func(cfg *DiscoveredConfig) error

	checkCycle = func(cfg *DiscoveredConfig) error {
		if inPath[cfg.Path] {
			return errors.New("cycle detected in dependency graph at path: " + cfg.Path)
		}

		if visited[cfg.Path] {
			return nil
		}

		visited[cfg.Path] = true
		inPath[cfg.Path] = true

		for _, dep := range cfg.Dependencies {
			if err := checkCycle(dep); err != nil {
				return err
			}
		}

		inPath[cfg.Path] = false

		return nil
	}

	for _, cfg := range c {
		if !visited[cfg.Path] {
			if err := checkCycle(cfg); err != nil {
				return cfg, err
			}
		}
	}

	return nil, nil
}

// RemoveCycles removes cycles from the dependency graph.
func (c DiscoveredConfigs) RemoveCycles() (DiscoveredConfigs, error) {
	const maxCycleChecks = 100

	var (
		err error
		cfg *DiscoveredConfig
	)

	for range maxCycleChecks {
		if cfg, err = c.CycleCheck(); err == nil {
			break
		}

		// Cfg should never be nil if err is not nil,
		// but we do this check to avoid a nil pointer dereference
		// if our assumptions change in the future.
		if cfg == nil {
			break
		}

		c = c.RemoveByPath(cfg.Path)
	}

	return c, err
}
