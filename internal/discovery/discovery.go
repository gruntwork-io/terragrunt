// Package discovery provides functionality for discovering Terragrunt configurations.
package discovery

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/config/hclparse"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
	"github.com/mattn/go-zglob"
	"github.com/zclconf/go-cty/cty"
)

const (
	// ConfigTypeUnit is the type of Terragrunt configuration for a unit.
	ConfigTypeUnit ConfigType = "unit"
	// ConfigTypeStack is the type of Terragrunt configuration for a stack.
	ConfigTypeStack ConfigType = "stack"

	// skipOutputDiagnostics is a string used to identify diagnostics that reference outputs.
	skipOutputDiagnostics = "output"
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

	// configFilenames is the list of config filenames to discover. If nil, defaults are used.
	configFilenames []string

	// includeDirs is a list of directory patterns to include in discovery (for strict include mode).
	includeDirs []string

	// excludeDirs is a list of directory patterns to exclude from discovery.
	excludeDirs []string

	// compiledIncludePatterns are precompiled glob patterns for includeDirs.
	compiledIncludePatterns []CompiledPattern

	// compiledExcludePatterns are precompiled glob patterns for excludeDirs.
	compiledExcludePatterns []CompiledPattern

	// parserOptions are custom HCL parser options to use when parsing during discovery
	parserOptions []hclparse.Option

	// strictInclude determines whether to use strict include mode (only include directories that match includeDirs).
	strictInclude bool

	// excludeByDefault determines whether to exclude configurations by default (triggered by include flags).
	excludeByDefault bool

	// ignoreExternalDependencies determines whether to drop dependencies that are outside the working directory.
	ignoreExternalDependencies bool

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

// CompiledPattern holds a precompiled glob pattern along with the original pattern string.
type CompiledPattern struct {
	Original string
	Compiled interface{ Match(name string) bool }
}

// DefaultConfigFilenames are the default Terragrunt config filenames used in discovery.
var DefaultConfigFilenames = []string{config.DefaultTerragruntConfigPath, config.DefaultStackFile}

// NewDiscovery creates a new Discovery.
func NewDiscovery(dir string, opts ...DiscoveryOption) *Discovery {
	discovery := &Discovery{
		workingDir: dir,
		hidden:     false,
		includeDirs: []string{
			config.StackDir,
			filepath.Join(config.StackDir, "**"),
		},
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

// WithConfigFilenames sets the configFilenames field to the given list.
func (d *Discovery) WithConfigFilenames(filenames []string) *Discovery {
	d.configFilenames = filenames
	return d
}

// WithIncludeDirs sets include directory glob patterns used for filtering during discovery.
func (d *Discovery) WithIncludeDirs(dirs []string) *Discovery {
	d.includeDirs = dirs
	return d
}

// WithExcludeDirs sets exclude directory glob patterns used for filtering during discovery.
func (d *Discovery) WithExcludeDirs(dirs []string) *Discovery {
	d.excludeDirs = dirs
	return d
}

// WithParserOptions sets custom HCL parser options to use when parsing during discovery.
func (d *Discovery) WithParserOptions(options []hclparse.Option) *Discovery {
	d.parserOptions = options
	return d
}

// WithStrictInclude enables strict include mode.
func (d *Discovery) WithStrictInclude() *Discovery {
	d.strictInclude = true
	return d
}

// WithExcludeByDefault enables exclude-by-default behavior.
func (d *Discovery) WithExcludeByDefault() *Discovery {
	d.excludeByDefault = true
	return d
}

// WithIgnoreExternalDependencies drops dependencies outside of the working directory.
func (d *Discovery) WithIgnoreExternalDependencies() *Discovery {
	d.ignoreExternalDependencies = true
	return d
}

// compileIncludePatterns compiles the include directory patterns for faster matching.
func (d *Discovery) compileIncludePatterns(l log.Logger) {
	d.compiledIncludePatterns = make([]CompiledPattern, 0, len(d.includeDirs))
	for _, pattern := range d.includeDirs {
		if compiled, err := zglob.New(pattern); err == nil {
			d.compiledIncludePatterns = append(d.compiledIncludePatterns, CompiledPattern{
				Original: pattern,
				Compiled: compiled,
			})
		} else {
			l.Warnf("Failed to compile include pattern '%s': %v. Pattern will be ignored.", pattern, err)
		}
	}
}

// compileExcludePatterns compiles the exclude directory patterns for faster matching.
func (d *Discovery) compileExcludePatterns(l log.Logger) {
	d.compiledExcludePatterns = make([]CompiledPattern, 0, len(d.excludeDirs))
	for _, pattern := range d.excludeDirs {
		if compiled, err := zglob.New(pattern); err == nil {
			d.compiledExcludePatterns = append(d.compiledExcludePatterns, CompiledPattern{
				Original: pattern,
				Compiled: compiled,
			})
		} else {
			l.Warnf("Failed to compile exclude pattern '%s': %v. Pattern will be ignored.", pattern, err)
		}
	}
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
func (c *DiscoveredConfig) Parse(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, suppressParseErrors bool, parserOptions []hclparse.Option) error {
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

	// Apply custom parser options if provided via discovery
	if len(parserOptions) > 0 {
		parsingCtx = parsingCtx.WithParseOption(parserOptions)
	}

	if suppressParseErrors {
		// If suppressing parse errors, we want to filter diagnostics that contain references to outputs,
		// while leaving other diagnostics as is.
		parseOptions := append(parsingCtx.ParserOptions, hclparse.WithDiagnosticsHandler(func(file *hcl.File, hclDiags hcl.Diagnostics) (hcl.Diagnostics, error) {
			filteredDiags := hcl.Diagnostics{}

			for _, hclDiag := range hclDiags {
				containsOutputRef := strings.Contains(strings.ToLower(hclDiag.Summary), skipOutputDiagnostics) ||
					strings.Contains(strings.ToLower(hclDiag.Detail), skipOutputDiagnostics)

				if !containsOutputRef {
					filteredDiags = append(filteredDiags, hclDiag)
				}
			}

			return filteredDiags, nil
		}))
		parsingCtx = parsingCtx.WithParseOption(parseOptions)
	}

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

	// Set default config filenames if not set
	filenames := d.configFilenames
	if len(filenames) == 0 {
		filenames = DefaultConfigFilenames
	}

	// Prepare include/exclude glob patterns (canonicalized) for matching
	var includePatterns, excludePatterns []string

	if len(d.includeDirs) > 0 {
		for _, p := range d.includeDirs {
			if !filepath.IsAbs(p) {
				p = filepath.Join(d.workingDir, p)
			}

			includePatterns = append(includePatterns, util.CleanPath(p))
		}
	}

	if len(d.excludeDirs) > 0 {
		for _, p := range d.excludeDirs {
			if !filepath.IsAbs(p) {
				p = filepath.Join(d.workingDir, p)
			}

			excludePatterns = append(excludePatterns, util.CleanPath(p))
		}
	}

	// Compile patterns if not already compiled
	if len(d.compiledIncludePatterns) == 0 && len(includePatterns) > 0 {
		d.includeDirs = includePatterns
		d.compileIncludePatterns(l)
	}

	if len(d.compiledExcludePatterns) == 0 && len(excludePatterns) > 0 {
		d.excludeDirs = excludePatterns
		d.compileExcludePatterns(l)
	}

	processFn := func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return errors.New(err)
		}

		if info.IsDir() {
			return nil
		}

		// Apply include/exclude filters by directory path first
		dir := filepath.Dir(path)

		canonicalDir, canErr := util.CanonicalPath(dir, d.workingDir)
		if canErr == nil {
			for _, pattern := range d.compiledExcludePatterns {
				if pattern.Compiled.Match(canonicalDir) {
					l.Debugf("Path %s excluded by glob %s", canonicalDir, pattern.Original)
					return nil
				}
			}

			// Enforce include patterns only when strictInclude or excludeByDefault are set
			if d.strictInclude || d.excludeByDefault {
				included := false

				for _, pattern := range d.compiledIncludePatterns {
					if pattern.Compiled.Match(canonicalDir) {
						included = true
						break
					}
				}

				if !included {
					return nil
				}
			}
		}

		// Now enforce hidden directory check if still applicable
		if !d.hidden && d.isInHiddenDirectory(path) {
			// If the directory is hidden, allow it only if it matches an include pattern
			allowHidden := false

			if canErr == nil {
				// Always allow .terragrunt-stack contents
				cleanDir := util.CleanPath(canonicalDir)
				if strings.Contains(cleanDir, "/"+config.StackDir+"/") || strings.HasSuffix(cleanDir, "/"+config.StackDir) {
					allowHidden = true
				}

				if !allowHidden {
					// Use precompiled patterns for include matching in hidden directory check
					for _, pattern := range d.compiledIncludePatterns {
						if pattern.Compiled.Match(canonicalDir) {
							allowHidden = true
							break
						}
					}
				}
			}

			if !allowHidden {
				return nil
			}
		}

		base := filepath.Base(path)
		for _, fname := range filenames {
			if base == fname {
				cfgType := ConfigTypeUnit
				if fname == config.DefaultStackFile {
					cfgType = ConfigTypeStack
				}

				cfg := &DiscoveredConfig{
					Type: cfgType,
					Path: filepath.Dir(path),
				}
				if d.discoveryContext != nil {
					cfg.DiscoveryContext = d.discoveryContext
				}

				cfgs = append(cfgs, cfg)

				break
			}
		}

		return nil
	}

	walkFn := filepath.WalkDir
	if opts.Experiments.Evaluate(experiment.Symlinks) {
		walkFn = util.WalkDirWithSymlinks
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
			// Stack configurations don't need to be parsed for discovery purposes.
			// They don't have exclude blocks or dependencies.
			//
			// This might change in the future, but for now we'll just skip parsing.
			if cfg.Type == ConfigTypeStack {
				continue
			}

			err := cfg.Parse(ctx, l, opts, d.suppressParseErrors, d.parserOptions)
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

			if d.ignoreExternalDependencies {
				dependencyDiscovery = dependencyDiscovery.WithIgnoreExternal()
			}

			if d.suppressParseErrors {
				dependencyDiscovery = dependencyDiscovery.WithSuppressParseErrors()
			}

			// pass parser options
			if len(d.parserOptions) > 0 {
				dependencyDiscovery = dependencyDiscovery.WithParserOptions(d.parserOptions)
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
			if _, cycleErr := cfgs.CycleCheck(); cycleErr != nil {
				l.Warnf("Cycle detected in dependency graph, attempting removal of cycles.")

				l.Debugf("Cycle: %w", cycleErr)

				var removeErr error
				cfgs, removeErr = cfgs.RemoveCycles()
				if removeErr != nil {
					errs = append(errs, errors.New(removeErr))
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
	parserOptions       []hclparse.Option
	depthRemaining      int
	discoverExternal    bool
	suppressParseErrors bool
	ignoreExternal      bool
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

// WithIgnoreExternal drops dependencies outside the working directory.
func (d *DependencyDiscovery) WithIgnoreExternal() *DependencyDiscovery {
	d.ignoreExternal = true
	return d
}

// WithParserOptions sets custom HCL parser options for dependency discovery.
func (d *DependencyDiscovery) WithParserOptions(options []hclparse.Option) *DependencyDiscovery {
	d.parserOptions = options
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
		err := dCfg.Parse(ctx, l, opts, d.suppressParseErrors, d.parserOptions)
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
			// Skip external if requested
			if d.ignoreExternal {
				continue
			}

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
