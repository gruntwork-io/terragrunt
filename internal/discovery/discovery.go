// Package discovery provides functionality for discovering Terragrunt configurations.
package discovery

import (
	"context"
	stderrs "errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
	"github.com/mattn/go-zglob"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/sync/errgroup"
)

const (
	// defaultDiscoveryWorkers is the default number of concurrent workers for discovery operations
	defaultDiscoveryWorkers = 4

	// maxDiscoveryWorkers is the maximum number of workers (2x default to prevent excessive concurrency)
	maxDiscoveryWorkers = defaultDiscoveryWorkers * 2

	// channelBufferMultiplier is the channel buffer multiplier for worker pools (larger buffers reduce blocking)
	channelBufferMultiplier = 4

	// maxHiddenDirMemoSize is the maximum number of hidden directory memoization entries (prevents unbounded memory growth)
	maxHiddenDirMemoSize = 1000

	// defaultMaxDependencyDepth is the default maximum dependency depth for discovery
	defaultMaxDependencyDepth = 1000

	// maxCycleRemovalAttempts is the maximum number of cycle removal attempts (prevents infinite loops)
	maxCycleRemovalAttempts = 100
)

// defaultExcludeDirs is the default directories where units should never be discovered.
var defaultExcludeDirs = []string{
	".git/**",
	".terraform/**",
	".terragrunt-cache/**",
}

// Sort is the sort order of the discovered configurations.
type Sort string

// Discovery is the configuration for a Terragrunt discovery.
type Discovery struct {
	// discoveryContext is the context in which the discovery is happening.
	discoveryContext *component.DiscoveryContext

	// workingDir is the directory to search for Terragrunt configurations.
	workingDir string

	// sort determines the sort order of the discovered configurations.
	sort Sort

	// compiledIncludePatterns are precompiled glob patterns for includeDirs.
	compiledIncludePatterns []CompiledPattern

	// compiledExcludePatterns are precompiled glob patterns for excludeDirs.
	compiledExcludePatterns []CompiledPattern

	// configFilenames is the list of config filenames to discover. If nil, defaults are used.
	configFilenames []string

	// includeDirs is a list of directory patterns to include in discovery (for strict include mode).
	includeDirs []string

	// excludeDirs is a list of directory patterns to exclude from discovery.
	excludeDirs []string

	// parserOptions are custom HCL parser options to use when parsing during discovery
	parserOptions []hclparse.Option

	// filters contains filter queries for component selection
	filters filter.Filters

	// dependencyTargetExpressions contains target expressions from graph filters that require dependency discovery
	dependencyTargetExpressions []filter.Expression

	// dependentTargetExpressions contains target expressions from graph filters that require dependent discovery
	dependentTargetExpressions []filter.Expression

	// hiddenDirMemo is a memoization of hidden directories.
	hiddenDirMemo hiddenDirMemo

	// numWorkers determines the number of concurrent workers for discovery operations.
	numWorkers int

	// maxDependencyDepth is the maximum depth of the dependency tree to discover.
	maxDependencyDepth int

	// discoverDependencies determines whether to discover dependencies.
	discoverDependencies bool

	// excludeByDefault determines whether to exclude configurations by default (triggered by include flags).
	excludeByDefault bool

	// noHidden determines whether to detect configurations in noHidden directories.
	noHidden bool

	// requiresParse is true when the discovery requires parsing Terragrunt configurations.
	requiresParse bool

	// strictInclude determines whether to use strict include mode (only include directories that match includeDirs).
	strictInclude bool

	// parseExclude determines whether to parse exclude configurations.
	parseExclude bool

	// parseInclude determines whether to parse include configurations.
	parseInclude bool

	// readFiles determines whether to parse for reading files.
	readFiles bool

	// discoverExternalDependencies determines whether to discover external dependencies.
	discoverExternalDependencies bool

	// suppressParseErrors determines whether to suppress errors when parsing Terragrunt configurations.
	suppressParseErrors bool

	// useDefaultExcludes determines whether to use default exclude patterns.
	useDefaultExcludes bool

	// filterFlagEnabled determines whether the filter flag experiment is active
	filterFlagEnabled bool

	// breakCycles determines whether to break cycles in the dependency graph if any exist.
	breakCycles bool
}

// DiscoveryOption is a function that modifies a Discovery.
type DiscoveryOption func(*Discovery)

// CompiledPattern holds a precompiled glob pattern along with the original pattern string.
type CompiledPattern struct {
	Compiled interface{ Match(name string) bool }
	Original string
}

// DefaultConfigFilenames are the default Terragrunt config filenames used in discovery.
var DefaultConfigFilenames = []string{config.DefaultTerragruntConfigPath, config.DefaultStackFile}

// WithNoHidden sets the Hidden flag to true.
func (d *Discovery) WithNoHidden() *Discovery {
	d.noHidden = true

	return d
}

// WithSort sets the Sort flag to the given sort.
func (d *Discovery) WithSort(sort Sort) *Discovery {
	d.sort = sort

	return d
}

// WithDiscoverDependencies sets the DiscoverDependencies flag to true.
func (d *Discovery) WithDiscoverDependencies() *Discovery {
	d.WithRequiresParse()

	d.discoverDependencies = true

	if d.maxDependencyDepth == 0 {
		d.maxDependencyDepth = defaultMaxDependencyDepth
	}

	return d
}

// WithParseExclude sets the parseExclude flag to true.
func (d *Discovery) WithParseExclude() *Discovery {
	d.WithRequiresParse()

	d.parseExclude = true

	return d
}

// WithParseInclude sets the parseInclude flag to true.
func (d *Discovery) WithParseInclude() *Discovery {
	d.WithRequiresParse()

	d.parseInclude = true

	return d
}

// WithReadFiles sets the readFiles flag to true.
func (d *Discovery) WithReadFiles() *Discovery {
	d.WithRequiresParse()

	d.readFiles = true

	return d
}

// WithRequiresParse sets the requiresParse flag to true.
func (d *Discovery) WithRequiresParse() *Discovery {
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
func (d *Discovery) WithDiscoveryContext(discoveryContext *component.DiscoveryContext) *Discovery {
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

// SetParseOptions implements common.ParseOptionsSetter allowing discovery to receive
// HCL parser options via generic option plumbing.
func (d *Discovery) SetParseOptions(options []hclparse.Option) {
	d.parserOptions = options
}

// WithOptions ingests runner options and applies any discovery-relevant settings.
// Currently, it extracts HCL parser options provided via common.ParseOptionsProvider
// and forwards them to discovery's parser configuration.
func (d *Discovery) WithOptions(opts ...common.Option) *Discovery {
	var parserOptions []hclparse.Option

	for _, opt := range opts {
		if p, ok := opt.(common.ParseOptionsProvider); ok {
			if po := p.GetParseOptions(); len(po) > 0 {
				parserOptions = append(parserOptions, po...)
			}
		}
	}

	if len(parserOptions) > 0 {
		d = d.WithParserOptions(parserOptions)
	}

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

// WithNumWorkers sets the number of concurrent workers for discovery operations.
func (d *Discovery) WithNumWorkers(numWorkers int) *Discovery {
	d.numWorkers = numWorkers
	return d
}

// WithoutDefaultExcludes disables the use of default exclude patterns (e.g. .git, .terraform, .terragrunt-cache).
func (d *Discovery) WithoutDefaultExcludes() *Discovery {
	d.useDefaultExcludes = false
	return d
}

// WithFilters sets filter queries for component selection.
// When filters are set, only components matching the filters will be included.
//
// WithFilters also determines whether certain aspects of the discovery configuration allows for optimizations or
// adjustments to discovery are required. e.g. exclude by default if there are any positive filters.
func (d *Discovery) WithFilters(filters filter.Filters) *Discovery {
	d.filters = filters

	d.WithFilterFlagEnabled()

	// If there are any positive filters, we need to exclude by default,
	// and only include components if they match filters.
	if d.filters.HasPositiveFilter() {
		d.WithExcludeByDefault()
	}

	// Collect target expressions from graph filters for selective graph traversal.
	d.dependencyTargetExpressions = d.filters.RequiresDependencyDiscovery()
	d.dependentTargetExpressions = d.filters.RequiresDependentDiscovery()

	// When working with graph filters, we always perform discovery of components,
	// regardless of whether or not they are external. We can filter them out after the fact if necessary.
	if len(d.dependencyTargetExpressions) > 0 || len(d.dependentTargetExpressions) > 0 {
		d.discoverExternalDependencies = true
	}

	// If any filters require parsing, we need to opt-in to parsing.
	if _, ok := d.filters.RequiresParse(); ok {
		d.WithRequiresParse()
	}

	return d
}

// WithFilterFlagEnabled sets whether the filter flag experiment is enabled.
// This changes how discovery processes components during file traversal.
func (d *Discovery) WithFilterFlagEnabled() *Discovery {
	d.filterFlagEnabled = true
	return d
}

// WithBreakCycles sets the BreakCycles flag to true.
func (d *Discovery) WithBreakCycles() *Discovery {
	d.breakCycles = true
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

// String returns a string representation of a Component.
// String returns the path of the Component.
func String(c component.Component) string {
	return c.Path()
}

// ContainsDependencyInAncestry returns true if the Component or any of
// its dependencies contains the given path as a dependency.
func ContainsDependencyInAncestry(c component.Component, path string) bool {
	for _, dep := range c.Dependencies() {
		if dep.Path() == path {
			return true
		}

		if ContainsDependencyInAncestry(dep, path) {
			return true
		}
	}

	return false
}

// Parse parses the discovered configuration.
func Parse(
	c component.Component,
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	suppressParseErrors bool,
	parserOptions []hclparse.Option,
) error {
	parseOpts := opts.Clone()

	// Determine working directory and config filename, supporting file paths and stack kind
	componentPath := c.Path()

	var workingDir, configFilename string

	// Defaults assume a directory path
	workingDir = componentPath
	configFilename = config.DefaultTerragruntConfigPath

	// If the path points directly to a file, split dir and filename
	if util.FileExists(componentPath) && !util.IsDir(componentPath) {
		workingDir = filepath.Dir(componentPath)
		configFilename = filepath.Base(componentPath)
	} else {
		// Allow user-specified config filename when provided as a file path
		if p := opts.TerragruntConfigPath; p != "" && !util.IsDir(p) {
			configFilename = filepath.Base(p)
		}
		// Stacks always use the default stack filename
		if c.Kind() == component.StackKind {
			configFilename = config.DefaultStackFile
		}
	}

	parseOpts.WorkingDir = workingDir

	// Suppress logging to avoid cluttering the output.
	parseOpts.Writer = io.Discard
	parseOpts.ErrWriter = io.Discard
	parseOpts.SkipOutput = true

	parseOpts.TerragruntConfigPath = filepath.Join(parseOpts.WorkingDir, configFilename)
	parseOpts.OriginalTerragruntConfigPath = parseOpts.TerragruntConfigPath

	parsingCtx := config.NewParsingContext(ctx, l, parseOpts).WithDecodeList(
		config.TerraformSource,
		config.DependenciesBlock,
		config.DependencyBlock,
		config.TerragruntFlags,
		config.FeatureFlagsBlock,
		config.ExcludeBlock,
		config.ErrorsBlock,
	).WithSkipOutputsResolution()

	// Apply custom parser options if provided via discovery
	if len(parserOptions) > 0 {
		parsingCtx = parsingCtx.WithParseOption(parserOptions)
	}

	if suppressParseErrors {
		// Suppressing parse errors to avoid false positive errors
		parseOptions := append(
			parsingCtx.ParserOptions,
			hclparse.WithDiagnosticsHandler(func(
				file *hcl.File,
				hclDiags hcl.Diagnostics,
			) (hcl.Diagnostics, error) {
				l.Debugf("Suppresed parsing errors %w", hclDiags)

				return nil, nil
			}))
		parsingCtx = parsingCtx.WithParseOption(parseOptions)
	}

	var (
		cfg *config.TerragruntConfig
		err error
	)

	// Set a list with partial blocks used to do discovery
	parsingCtx = parsingCtx.WithDecodeList(
		config.TerraformSource,
		config.DependenciesBlock,
		config.DependencyBlock,
		config.TerragruntFlags,
		config.FeatureFlagsBlock,
		config.ExcludeBlock,
		config.ErrorsBlock,
	)

	//nolint: contextcheck
	cfg, err = config.PartialParseConfigFile(parsingCtx, l, parseOpts.TerragruntConfigPath, nil)
	if err != nil {
		// Treat include-only/no-settings configs as non-fatal during discovery when suppression is enabled
		if suppressParseErrors && containsNoSettingsError(err) {
			l.Debugf("Skipping include-only config during discovery: %s", parseOpts.TerragruntConfigPath)
			return nil
		}

		if !suppressParseErrors || cfg == nil {
			l.Debugf("Unrecoverable parse error for %s: %s", parseOpts.TerragruntConfigPath, err)

			return errors.New(err)
		}

		l.Debugf("Suppressing parse error for %s: %s", parseOpts.TerragruntConfigPath, err)
	}

	// Store the parsed configuration
	// Only Units are parsed during discovery; Stacks are not
	if unit, ok := c.(*component.Unit); ok {
		unit.StoreConfig(cfg)
	}

	// Populate the Reading field with files read during parsing.
	// The parsing context tracks all files that were read.
	if parsingCtx.FilesRead != nil {
		c.SetReading(*parsingCtx.FilesRead...)
	}

	return nil
}

// isInHiddenDirectory returns true if the path is in a hidden directory.
func (d *Discovery) isInHiddenDirectory(path string) bool {
	ok := d.hiddenDirMemo.contains(path)
	if ok {
		return true
	}

	// Quick check: if path doesn't contain "." after first character, it's not hidden
	if !strings.Contains(path[1:], string(os.PathSeparator)+".") {
		return false
	}

	hiddenPath := ""

	for part := range strings.SplitSeq(path, string(os.PathSeparator)) {
		if hiddenPath != "" {
			hiddenPath = filepath.Join(hiddenPath, part)
		} else {
			hiddenPath = part
		}

		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			d.hiddenDirMemo.append(hiddenPath)

			return true
		}
	}

	return false
}

// discoverConcurrently performs concurrent file discovery with worker pools using errgroup.
func (d *Discovery) discoverConcurrently(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	filenames []string,
) (component.Components, error) {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(d.numWorkers + 1) // +1 for the file walker

	filePaths := make(chan string, d.numWorkers*channelBufferMultiplier)
	results := make(chan component.Component, d.numWorkers*channelBufferMultiplier)

	g.Go(func() error {
		defer close(filePaths)
		return d.walkDirectoryConcurrently(ctx, l, opts, filePaths)
	})

	for range d.numWorkers {
		g.Go(func() error {
			return d.configWorker(ctx, l, filePaths, results, filenames)
		})
	}

	// Close results channel when all workers are done
	go func() {
		defer close(results)

		_ = g.Wait() // We handle errors in the main thread below
	}()

	components := make(component.Components, 0, len(results))

	for config := range results {
		components = append(components, config)
	}

	if err := g.Wait(); err != nil {
		return components, err
	}

	return components, nil
}

// walkDirectoryConcurrently walks the directory tree and sends file paths to workers.
func (d *Discovery) walkDirectoryConcurrently(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	filePaths chan<- string,
) error {
	walkFn := filepath.WalkDir
	if opts.Experiments.Evaluate(experiment.Symlinks) {
		walkFn = util.WalkDirWithSymlinks
	}

	processFn := func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if info.IsDir() {
			return d.skipDirIfIgnorable(l, path)
		}

		select {
		case filePaths <- path:
		case <-ctx.Done():
			return ctx.Err()
		}

		return nil
	}

	return walkFn(d.workingDir, processFn)
}

// skipDirIfIgnorable determines if a directory should be skipped during traversal.
func (d *Discovery) skipDirIfIgnorable(l log.Logger, path string) error {
	if err := skipDirIfIgnorable(path); err != nil {
		return err
	}

	if d.noHidden {
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") && base != "." && base != ".." {
			return filepath.SkipDir
		}
	}

	// When the filter flag is enabled, let the filters control discovery instead of exclude patterns
	if d.filterFlagEnabled {
		return nil
	}

	canonicalDir, canErr := util.CanonicalPath(path, d.workingDir)
	if canErr == nil {
		for _, pattern := range d.compiledExcludePatterns {
			if pattern.Compiled.Match(canonicalDir) {
				l.Debugf("Directory %s excluded by glob %s", canonicalDir, pattern.Original)
				return filepath.SkipDir
			}
		}
	}

	return nil
}

// isInStackDirectory checks if the given clean directory path contains the stack directory.
//
// This function assumes that the path will be normalized for forward slashes before being
// passed in.
func isInStackDirectory(cleanDir string) bool {
	for part := range strings.SplitSeq(cleanDir, "/") {
		if part == config.StackDir {
			return true
		}
	}

	return false
}

// configWorker processes file paths and determines if they are Terragrunt configurations.
func (d *Discovery) configWorker(
	ctx context.Context,
	l log.Logger,
	filePaths <-chan string,
	results chan<- component.Component,
	filenames []string,
) error {
	for path := range filePaths {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		config := d.processFile(path, l, filenames)

		if config != nil {
			select {
			case results <- config:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return nil
}

// processFile processes a single file to determine if it's a Terragrunt configuration.
func (d *Discovery) processFile(path string, l log.Logger, filenames []string) component.Component {
	dir := filepath.Dir(path)

	canonicalDir, canErr := util.CanonicalPath(dir, d.workingDir)
	if canErr == nil {
		// Eventually, this is going to be removed entirely, as filter evaluation
		// will be all that's needed.
		if !d.filterFlagEnabled {
			for _, pattern := range d.compiledExcludePatterns {
				if pattern.Compiled.Match(canonicalDir) {
					l.Debugf("Path %s excluded by glob %s", canonicalDir, pattern.Original)
					return nil
				}
			}
		}

		if d.filterFlagEnabled {
			c := d.createComponentFromPath(path, filenames)
			if c == nil {
				return nil
			}

			// Check for hidden directories before returning
			if d.noHidden && d.isInHiddenDirectory(path) {
				// Always allow .terragrunt-stack contents
				cleanDir := util.CleanPath(canonicalDir)
				if !isInStackDirectory(cleanDir) {
					return nil
				}
			}

			shouldEvaluateFiltersNow := !d.discoverDependencies
			if shouldEvaluateFiltersNow {
				if _, requiresParsing := d.filters.RequiresDiscovery(); !requiresParsing {
					filtered, err := d.filters.Evaluate(l, component.Components{c})
					if err != nil {
						l.Debugf("Error evaluating filters for %s: %v", c.Path(), err)
						return nil
					}

					if len(filtered) == 0 {
						return nil
					}
				}
			}

			return c
		}

		// Everything after this point is only relevant when the filter flag is disabled.
		// It should be removed once the filter flag is generally available.

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
	if d.noHidden && d.isInHiddenDirectory(path) {
		// If the directory is hidden, allow it only if it matches an include pattern
		allowHidden := false

		if canErr == nil {
			// Always allow .terragrunt-stack contents
			cleanDir := util.CleanPath(canonicalDir)
			allowHidden = isInStackDirectory(cleanDir)

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

	return d.createComponentFromPath(path, filenames)
}

// createComponentFromPath creates a component from a file path if it matches one of the config filenames.
// Returns nil if the file doesn't match any of the provided filenames.
func (d *Discovery) createComponentFromPath(path string, filenames []string) component.Component {
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

		if d.discoveryContext != nil {
			c.SetDiscoveryContext(d.discoveryContext)
		}

		return c
	}

	return nil
}

// parseConcurrently parses components concurrently to improve performance using errgroup.
func (d *Discovery) parseConcurrently(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	components component.Components,
) []error {
	// Filter out configs that don't need parsing
	// Pre-allocate with estimated capacity to reduce reallocation
	componentsToParse := make(component.Components, 0, len(components))
	for _, c := range components {
		// Stack configurations don't need to be parsed for discovery purposes.
		// They don't have exclude blocks or dependencies.
		if _, ok := c.(*component.Stack); ok {
			continue
		}

		componentsToParse = append(componentsToParse, c)
	}

	if len(componentsToParse) == 0 {
		return nil
	}

	// Use errgroup for better error handling and synchronization
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(d.numWorkers)

	// Use channels to coordinate parsing work
	componentChan := make(chan component.Component, d.numWorkers*channelBufferMultiplier)
	errorChan := make(chan error, len(componentsToParse))

	// Start component sender
	g.Go(func() error {
		defer close(componentChan)

		for _, c := range componentsToParse {
			select {
			case componentChan <- c:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	})

	// Start parser workers
	for range d.numWorkers {
		g.Go(func() error {
			return d.parseWorker(ctx, l, opts, componentChan, errorChan)
		})
	}

	// Close error channel when all workers are done
	go func() {
		defer close(errorChan)

		_ = g.Wait() // We handle errors in the main thread below
	}()

	// Collect errors
	var errs []error

	for err := range errorChan {
		if err != nil {
			errs = append(errs, errors.New(err))
		}
	}

	// Wait for completion and get any errgroup errors
	if err := g.Wait(); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// parseWorker is a worker that parses configurations concurrently.
func (d *Discovery) parseWorker(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	componentChan <-chan component.Component,
	errorChan chan<- error,
) error {
	for c := range componentChan {
		// Context cancellation check
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := Parse(c, ctx, l, opts, d.suppressParseErrors, d.parserOptions)

		// Send error or handle context cancellation
		select {
		case errorChan <- err:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// Discover discovers Terragrunt configurations in the WorkingDir.
func (d *Discovery) Discover(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
) (component.Components, error) {
	// Set default config filenames if not set
	filenames := d.configFilenames
	if len(filenames) == 0 {
		filenames = DefaultConfigFilenames
	}

	// Prepare include/exclude glob patterns (canonicalized) for matching
	var includePatterns, excludePatterns []string

	// Add default excludes if enabled
	if d.useDefaultExcludes {
		excludePatterns = append(excludePatterns, defaultExcludeDirs...)
	}

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

	// Use concurrent discovery for better performance
	components, err := d.discoverConcurrently(ctx, l, opts, filenames)
	if err != nil {
		return components, err
	}

	errs := []error{}

	// We do an initial parse loop if we know we need to parse configurations,
	// as we might need to parse configurations for multiple reasons.
	// e.g. dependencies, exclude, etc.
	if d.requiresParse {
		parseErrs := d.parseConcurrently(ctx, l, opts, components)
		errs = append(errs, parseErrs...)
	}

	dependencyStartingComponents, err := d.determineDependencyStartingComponents(l, components)
	if err != nil {
		errs = append(errs, err)
	}

	dependentStartingComponents, err := d.determineDependentStartingComponents(l, components)
	if err != nil {
		errs = append(errs, err)
	}

	var threadSafeComponents *component.ThreadSafeComponents

	shouldRunDependencyDiscovery := len(dependencyStartingComponents) > 0
	shouldRunDependentDiscovery := len(dependentStartingComponents) > 0

	if shouldRunDependencyDiscovery || shouldRunDependentDiscovery {
		threadSafeComponents = component.NewThreadSafeComponents(components)

		// Run dependency and dependent discovery concurrently.
		g, discoveryCtx := errgroup.WithContext(ctx)
		g.SetLimit(2) //nolint:mnd

		if shouldRunDependencyDiscovery {
			g.Go(func() error {
				return telemetry.TelemeterFromContext(ctx).Collect(ctx, "discover_dependencies", map[string]any{
					"working_dir":                    d.workingDir,
					"config_count":                   len(components),
					"starting_component_count":       len(dependencyStartingComponents),
					"discover_external_dependencies": d.discoverExternalDependencies,
					"max_dependency_depth":           d.maxDependencyDepth,
				}, func(ctx context.Context) error {
					dependencyDiscovery := NewDependencyDiscovery(threadSafeComponents).
						WithMaxDepth(d.maxDependencyDepth).
						WithWorkingDir(d.workingDir).
						WithNumWorkers(d.numWorkers)

					if d.discoveryContext != nil {
						dependencyDiscovery = dependencyDiscovery.WithDiscoveryContext(d.discoveryContext)
					}

					if d.discoverExternalDependencies {
						dependencyDiscovery = dependencyDiscovery.WithDiscoverExternalDependencies()
					}

					if d.suppressParseErrors {
						dependencyDiscovery = dependencyDiscovery.WithSuppressParseErrors()
					}

					// pass parser options
					if len(d.parserOptions) > 0 {
						dependencyDiscovery = dependencyDiscovery.WithParserOptions(d.parserOptions)
					}

					discoveryErr := dependencyDiscovery.DiscoverAllDependencies(discoveryCtx, l, opts, dependencyStartingComponents)
					if discoveryErr != nil {
						if !d.suppressParseErrors {
							return discoveryErr
						}

						l.Warnf("Parsing errors where encountered while discovering dependencies. They were suppressed, and can be found in the debug logs.")

						l.Debugf("Errors: %v", discoveryErr)
					}

					return nil
				})
			})
		}

		if shouldRunDependentDiscovery {
			g.Go(func() error {
				return telemetry.TelemeterFromContext(ctx).Collect(ctx, "discover_dependents", map[string]any{
					"working_dir":              d.workingDir,
					"config_count":             len(components),
					"starting_component_count": len(dependentStartingComponents),
					"max_dependency_depth":     d.maxDependencyDepth,
				}, func(ctx context.Context) error {
					dependentDiscovery := NewDependentDiscovery(threadSafeComponents).
						WithMaxDepth(d.maxDependencyDepth).
						WithWorkingDir(d.workingDir).
						WithNumWorkers(d.numWorkers)

					if d.discoveryContext != nil {
						dependentDiscovery = dependentDiscovery.WithDiscoveryContext(d.discoveryContext)
					}

					if d.suppressParseErrors {
						dependentDiscovery = dependentDiscovery.WithSuppressParseErrors()
					}

					if d.discoverExternalDependencies {
						dependentDiscovery = dependentDiscovery.WithDiscoverExternalDependencies()
					}

					// pass parser options
					if len(d.parserOptions) > 0 {
						dependentDiscovery = dependentDiscovery.WithParserOptions(d.parserOptions)
					}

					// Set runtime values before discovery
					dependentDiscovery = dependentDiscovery.WithOpts(opts)

					if len(d.configFilenames) > 0 {
						dependentDiscovery = dependentDiscovery.WithFilenames(d.configFilenames)
					}

					// Compute git root if we have starting components
					if len(dependentStartingComponents) > 0 {
						startingPath := dependentStartingComponents[0].Path()
						if gitRootPath, gitErr := shell.GitTopLevelDir(discoveryCtx, l, opts, startingPath); gitErr == nil {
							dependentDiscovery = dependentDiscovery.WithGitRoot(gitRootPath)
						}
					}

					discoveryErr := dependentDiscovery.DiscoverAllDependents(discoveryCtx, l, dependentStartingComponents)
					if discoveryErr != nil {
						if !d.suppressParseErrors {
							return discoveryErr
						}

						l.Warnf("Parsing errors where encountered while discovering dependents. They were suppressed, and can be found in the debug logs.")

						l.Debugf("Errors: %w", discoveryErr)
					}

					return nil
				})
			})
		}

		if discoveryGroupErr := g.Wait(); discoveryGroupErr != nil {
			return components, errors.New(discoveryGroupErr)
		}

		components = threadSafeComponents.ToComponents()

		err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "discovery_cycle_check", map[string]any{
			"working_dir":  d.workingDir,
			"config_count": len(components),
		}, func(ctx context.Context) error {
			if _, cycleErr := components.CycleCheck(); cycleErr != nil {
				l.Warnf("Cycle detected in dependency graph, attempting removal of cycles.")

				l.Debugf("Cycle: %w", cycleErr)

				var removeErr error

				if d.breakCycles {
					components, removeErr = RemoveCycles(components)
					if removeErr != nil {
						errs = append(errs, errors.New(removeErr))
					}
				}
			}

			return nil
		})
		if err != nil {
			return components, errors.New(err)
		}
	}

	if len(d.filters) > 0 {
		filtered, evaluateErr := d.filters.Evaluate(l, components)
		if evaluateErr != nil {
			errs = append(errs, errors.New(evaluateErr))
		}

		components = filtered
	}

	if d.filterFlagEnabled && d.discoverDependencies {
		relationshipDiscovery := NewRelationshipDiscovery(&components).
			WithMaxDepth(d.maxDependencyDepth).
			WithNumWorkers(d.numWorkers)

		if len(d.parserOptions) > 0 {
			relationshipDiscovery = relationshipDiscovery.WithParserOptions(d.parserOptions)
		}

		err = relationshipDiscovery.DiscoverAllRelationships(ctx, l, opts, components)
		if err != nil {
			if !d.suppressParseErrors {
				errs = append(errs, errors.New(err))
			} else {
				l.Warnf("Parsing errors where encountered while discovering relationships. They were suppressed, and can be found in the debug logs.")

				l.Debugf("Errors: %w", err)
			}
		}
	}

	if len(errs) > 0 {
		return components, errors.Join(errs...)
	}

	return components, nil
}

// determineDependencyStartingComponents determines the starting components for dependency discovery.
// It uses filter expressions if provided, otherwise returns all components if dependency discovery is enabled.
func (d *Discovery) determineDependencyStartingComponents(
	l log.Logger,
	components component.Components,
) (component.Components, error) {
	var (
		startingComponents component.Components
		errs               []error
	)

	// In the legacy discovery path, it's a binary all or nothing.
	if !d.filterFlagEnabled {
		if d.discoverDependencies {
			return components, nil
		}

		return startingComponents, nil
	}

	// In the filter flag enabled path:
	// - If we have dependency target expressions, use them to determine starting components
	// - If we don't have dependency target expressions but discoverDependencies is enabled,
	//   use all components as starting points (to discover dependencies of all components)
	if len(d.dependencyTargetExpressions) == 0 {
		return startingComponents, nil
	}

	seenPaths := make(map[string]struct{})

	for _, targetExpr := range d.dependencyTargetExpressions {
		matched, err := filter.Evaluate(l, targetExpr, components)
		if err != nil {
			errs = append(errs, errors.New(err))
			continue
		}

		for _, c := range matched {
			path := c.Path()
			if _, seen := seenPaths[path]; !seen {
				startingComponents = append(startingComponents, c)
				seenPaths[path] = struct{}{}
			}
		}
	}

	if len(errs) > 0 {
		return startingComponents, errors.Join(errs...)
	}

	return startingComponents, nil
}

// determineDependentStartingComponents determines the starting components for dependent discovery.
// It uses filter expressions to determine which components should be used as starting points.
func (d *Discovery) determineDependentStartingComponents(
	l log.Logger,
	components component.Components,
) (component.Components, error) {
	var (
		startingComponents component.Components
		errs               []error
	)

	// If there are no dependent target expressions, return empty starting components
	if len(d.dependentTargetExpressions) == 0 {
		return startingComponents, nil
	}

	seenPaths := make(map[string]struct{})

	for _, targetExpr := range d.dependentTargetExpressions {
		matched, err := filter.Evaluate(l, targetExpr, components)
		if err != nil {
			errs = append(errs, errors.New(err))
			continue
		}

		for _, c := range matched {
			path := c.Path()
			if _, seen := seenPaths[path]; !seen {
				startingComponents = append(startingComponents, c)
				seenPaths[path] = struct{}{}
			}
		}
	}

	if len(errs) > 0 {
		return startingComponents, errors.Join(errs...)
	}

	return startingComponents, nil
}

// extractDependencyPaths extracts all dependency paths from a Terragrunt configuration.
// It returns the list of absolute dependency paths and any errors encountered during extraction.
func extractDependencyPaths(cfg *config.TerragruntConfig, component component.Component) ([]string, error) {
	deduped := make(map[string]struct{})

	var errs []error

	for _, dependency := range cfg.TerragruntDependencies {
		if dependency.Enabled != nil && !*dependency.Enabled {
			continue
		}

		if dependency.ConfigPath.Type() != cty.String {
			errs = append(errs, errors.New("dependency config path is not a string"))
			continue
		}

		depPath := dependency.ConfigPath.AsString()
		if !filepath.IsAbs(depPath) {
			depPath = filepath.Clean(filepath.Join(component.Path(), depPath))
		}

		deduped[depPath] = struct{}{}
	}

	if cfg.Dependencies != nil {
		for _, dependency := range cfg.Dependencies.Paths {
			if !filepath.IsAbs(dependency) {
				dependency = filepath.Clean(filepath.Join(component.Path(), dependency))
			}

			deduped[dependency] = struct{}{}
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

// RemoveCycles removes cycles from the dependency graph.
func RemoveCycles(components component.Components) (component.Components, error) {
	var (
		err error
		c   component.Component
	)

	for range maxCycleRemovalAttempts {
		if c, err = components.CycleCheck(); err == nil {
			break
		}

		// Cfg should never be nil if err is not nil,
		// but we do this check to avoid a nil pointer dereference
		// if our assumptions change in the future.
		if c == nil {
			break
		}

		components = components.RemoveByPath(c.Path())
	}

	return components, err
}

// hiddenDirMemo provides thread-safe memoization of hidden directories.
type hiddenDirMemo struct {
	entries []string
	mu      sync.RWMutex
}

// append adds a path to the memo if there's space available.
func (h *hiddenDirMemo) append(path string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.entries) < maxHiddenDirMemoSize {
		h.entries = append(h.entries, path)
	}
}

// contains checks if any of the memoized hidden directories is a prefix of the given path.
func (h *hiddenDirMemo) contains(path string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, hiddenDir := range h.entries {
		if strings.HasPrefix(path, hiddenDir) {
			return true
		}
	}

	return false
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

// containsNoSettingsError returns true if the provided error (possibly a joined/wrapped error)
// contains a config-level error indicating there were no Terragrunt configuration settings
// (e.g., include-only file) that should be treated as non-fatal during discovery.
func containsNoSettingsError(err error) bool {
	for _, e := range errors.UnwrapErrors(err) {
		var target config.CouldNotResolveTerragruntConfigInFileError
		if stderrs.As(e, &target) {
			return true
		}
	}

	return false
}
