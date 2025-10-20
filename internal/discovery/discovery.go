// Package discovery provides functionality for discovering Terragrunt configurations.
package discovery

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
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
	// skipOutputDiagnostics is a string used to identify diagnostics that reference outputs.
	skipOutputDiagnostics = "output"

	// skipNoVariableNamedDependencyDiagnostic is a string used to identify diagnostics
	// that reference the missing dependency variable.
	//
	// This is fine during discovery, as we don't need dependency outputs resolved.
	//
	// This is a hack, and there should be a better way of handling this...
	skipNoVariableNamedDependencyDiagnostic = `There is no variable named "dependency".`

	// skipNullValueDiagnostic is a string used to identify diagnostics about accessing
	// attributes on null values, which commonly happens during discovery when dependency
	// outputs or other values haven't been resolved yet, and the configuration is being
	// converted to a cty value.
	skipNullValueDiagnostic = "null value"

	// Default number of concurrent workers for discovery operations
	defaultDiscoveryWorkers = 4

	// Maximum number of workers (2x default to prevent excessive concurrency)
	maxDiscoveryWorkers = defaultDiscoveryWorkers * 2

	// Channel buffer multiplier for worker pools (larger buffers reduce blocking)
	channelBufferMultiplier = 4

	// Maximum hidden directory memoization entries (prevents unbounded memory growth)
	maxHiddenDirMemoSize = 1000

	// Default maximum dependency depth for discovery
	defaultMaxDependencyDepth = 1000

	// Maximum number of cycle removal attempts (prevents infinite loops)
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

	// filters contains filter queries for component selection
	filters filter.Filters

	// hiddenDirMemo is a memoization of hidden directories.
	hiddenDirMemo hiddenDirMemo

	// strictInclude determines whether to use strict include mode (only include directories that match includeDirs).
	strictInclude bool

	// excludeByDefault determines whether to exclude configurations by default (triggered by include flags).
	excludeByDefault bool

	// ignoreExternalDependencies determines whether to drop dependencies that are outside the working directory.
	ignoreExternalDependencies bool

	// maxDependencyDepth is the maximum depth of the dependency tree to discover.
	maxDependencyDepth int

	// numWorkers determines the number of concurrent workers for discovery operations.
	numWorkers int

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

	// useDefaultExcludes determines whether to use default exclude patterns.
	useDefaultExcludes bool
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

// NewDiscovery creates a new Discovery.
func NewDiscovery(dir string, opts ...DiscoveryOption) *Discovery {
	numWorkers := max(min(runtime.NumCPU(), maxDiscoveryWorkers), defaultDiscoveryWorkers)

	discovery := &Discovery{
		workingDir: dir,
		hidden:     false,
		includeDirs: []string{
			config.StackDir,
			filepath.Join(config.StackDir, "**"),
		},
		numWorkers:         numWorkers,
		useDefaultExcludes: true,
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
		d.maxDependencyDepth = defaultMaxDependencyDepth
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

// WithIgnoreExternalDependencies drops dependencies outside of the working directory.
func (d *Discovery) WithIgnoreExternalDependencies() *Discovery {
	d.ignoreExternalDependencies = true
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
func (d *Discovery) WithFilters(filters filter.Filters) *Discovery {
	d.filters = filters
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
func String(c *component.Component) string {
	return c.Path
}

// ContainsDependencyInAncestry returns true if the Component or any of
// its dependencies contains the given path as a dependency.
func ContainsDependencyInAncestry(c *component.Component, path string) bool {
	for _, dep := range c.Dependencies {
		if dep.Path == path {
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
	c *component.Component,
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	suppressParseErrors bool,
	parserOptions []hclparse.Option,
) error {
	parseOpts := opts.Clone()
	parseOpts.WorkingDir = c.Path

	// Suppress logging to avoid cluttering the output.
	parseOpts.Writer = io.Discard
	parseOpts.ErrWriter = io.Discard
	parseOpts.SkipOutput = true

	// If the user provided a specific terragrunt config path and it is not a directory,
	// use its base name as the file to parse. This allows users to run terragrunt with
	// a specific config file instead of the default terragrunt.hcl.
	// Otherwise, use the default terragrunt.hcl filename.
	filename := config.DefaultTerragruntConfigPath
	if opts.TerragruntConfigPath != "" && !util.IsDir(opts.TerragruntConfigPath) {
		filename = filepath.Base(opts.TerragruntConfigPath)
	}

	// For stack configurations, always use the default stack config filename
	if c.Kind == component.Stack {
		filename = config.DefaultStackFile
	}

	parseOpts.TerragruntConfigPath = filepath.Join(parseOpts.WorkingDir, filename)

	parsingCtx := config.NewParsingContext(ctx, l, parseOpts).WithDecodeList(
		config.DependenciesBlock,
		config.DependencyBlock,
		config.FeatureFlagsBlock,
		config.ExcludeBlock,
	).WithSkipOutputsResolution()

	// Apply custom parser options if provided via discovery
	if len(parserOptions) > 0 {
		parsingCtx = parsingCtx.WithParseOption(parserOptions)
	}

	if suppressParseErrors {
		// If suppressing parse errors, we want to filter diagnostics that contain references to outputs,
		// while leaving other diagnostics as is.
		parseOptions := append(
			parsingCtx.ParserOptions,
			hclparse.WithDiagnosticsHandler(func(
				file *hcl.File,
				hclDiags hcl.Diagnostics,
			) (hcl.Diagnostics, error) {
				filteredDiags := hcl.Diagnostics{}

				for _, hclDiag := range hclDiags {
					filterOut := strings.Contains(strings.ToLower(hclDiag.Summary), skipOutputDiagnostics) ||
						strings.Contains(strings.ToLower(hclDiag.Detail), skipOutputDiagnostics) ||
						strings.Contains(hclDiag.Detail, skipNoVariableNamedDependencyDiagnostic) ||
						strings.Contains(strings.ToLower(hclDiag.Summary), skipNullValueDiagnostic) ||
						strings.Contains(strings.ToLower(hclDiag.Detail), skipNullValueDiagnostic)

					if !filterOut {
						filteredDiags = append(filteredDiags, hclDiag)
					}
				}

				// If all diagnostics were filtered out, return nil instead of an empty slice
				// to prevent the parser from treating it as an error
				if len(filteredDiags) == 0 {
					return nil, nil
				}

				return filteredDiags, nil
			}))
		parsingCtx = parsingCtx.WithParseOption(parseOptions)
	}

	// Use PartialParseConfigFile instead of ParseConfigFile to avoid evaluating generate blocks during discovery.
	// During discovery, we set WithSkipOutputsResolution() to skip resolving dependency outputs for performance.
	// However, generate blocks may reference dependency outputs (e.g., dependency.other.outputs.x).
	// If we use ParseConfigFile here, it would evaluate generate blocks with unknown dependency values,
	// causing "Unsuitable value: value must be known" errors.
	// PartialParseConfigFile respects the WithDecodeList() and only decodes the blocks we need for discovery
	// (dependencies, feature flags, excludes), skipping generate blocks. Generate blocks will be properly
	// evaluated later when each unit runs individually with full dependency resolution.
	// See: https://github.com/gruntwork-io/terragrunt/issues/4962
	//nolint: contextcheck
	cfg, err := config.PartialParseConfigFile(parsingCtx, l, parseOpts.TerragruntConfigPath, nil)
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
	ok := d.hiddenDirMemo.contains(path)
	if ok {
		return true
	}

	// Quick check: if path doesn't contain "." after first character, it's not hidden
	if !strings.Contains(path[1:], string(os.PathSeparator)+".") {
		return false
	}

	hiddenPath := ""
	parts := strings.SplitSeq(path, string(os.PathSeparator))

	for part := range parts {
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
	results := make(chan *component.Component, d.numWorkers*channelBufferMultiplier)

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

	cfgs := make(component.Components, 0, len(results))

	for config := range results {
		cfgs = append(cfgs, config)
	}

	if err := g.Wait(); err != nil {
		return cfgs, err
	}

	return cfgs, nil
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
			return d.shouldSkipDirectory(path, l)
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

// shouldSkipDirectory determines if a directory should be skipped during traversal.
func (d *Discovery) shouldSkipDirectory(path string, l log.Logger) error {
	base := filepath.Base(path)

	switch base {
	case ".git", ".terraform", ".terragrunt-cache":
		return filepath.SkipDir
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

// configWorker processes file paths and determines if they are Terragrunt configurations.
func (d *Discovery) configWorker(
	ctx context.Context,
	l log.Logger,
	filePaths <-chan string,
	results chan<- *component.Component,
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
func (d *Discovery) processFile(path string, l log.Logger, filenames []string) *component.Component {
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
			componentKind := component.Unit
			if fname == config.DefaultStackFile {
				componentKind = component.Stack
			}

			cfg := &component.Component{
				Kind: componentKind,
				Path: filepath.Dir(path),
			}
			if d.discoveryContext != nil {
				cfg.DiscoveryContext = d.discoveryContext
			}

			return cfg
		}
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
	componentsToParse := make([]*component.Component, 0, len(components))
	for _, c := range components {
		// Stack configurations don't need to be parsed for discovery purposes.
		// They don't have exclude blocks or dependencies.
		if c.Kind == component.Stack {
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
	componentChan := make(chan *component.Component, d.numWorkers*channelBufferMultiplier)
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
	componentChan <-chan *component.Component,
	errorChan chan<- error,
) error {
	for cfg := range componentChan {
		// Context cancellation check
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := Parse(cfg, ctx, l, opts, d.suppressParseErrors, d.parserOptions)

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

	// Apply filters if configured and not doing dependency discovery
	// When dependency discovery is enabled, we defer filtering until after dependencies are discovered
	if len(d.filters) > 0 && !d.discoverDependencies {
		filtered, err := d.filters.Evaluate(components)
		if err != nil {
			errs = append(errs, errors.New(err))
		} else {
			components = filtered
		}
	}

	if d.discoverDependencies {
		err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "discover_dependencies", map[string]any{
			"working_dir":                    d.workingDir,
			"config_count":                   len(components),
			"discover_external_dependencies": d.discoverExternalDependencies,
			"max_dependency_depth":           d.maxDependencyDepth,
		}, func(ctx context.Context) error {
			dependencyDiscovery := NewDependencyDiscovery(components, d.maxDependencyDepth)

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

			components = dependencyDiscovery.cfgs

			return nil
		})
		if err != nil {
			return components, errors.New(err)
		}

		err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "discovery_cycle_check", map[string]any{
			"working_dir":  d.workingDir,
			"config_count": len(components),
		}, func(ctx context.Context) error {
			if _, cycleErr := components.CycleCheck(); cycleErr != nil {
				l.Warnf("Cycle detected in dependency graph, attempting removal of cycles.")

				l.Debugf("Cycle: %w", cycleErr)

				var removeErr error

				components, removeErr = RemoveCycles(components)
				if removeErr != nil {
					errs = append(errs, errors.New(removeErr))
				}
			}

			return nil
		})
		if err != nil {
			return components, errors.New(err)
		}
	}

	// Apply filters at the end if dependency discovery was enabled
	if len(d.filters) > 0 && d.discoverDependencies {
		filtered, err := d.filters.Evaluate(components)
		if err != nil {
			errs = append(errs, errors.New(err))
		} else {
			components = filtered
		}
	}

	if len(errs) > 0 {
		return components, errors.Join(errs...)
	}

	return components, nil
}

// DependencyDiscovery is the configuration for a DependencyDiscovery.
type DependencyDiscovery struct {
	discoveryContext    *component.DiscoveryContext
	cfgs                component.Components
	parserOptions       []hclparse.Option
	depthRemaining      int
	discoverExternal    bool
	suppressParseErrors bool
	ignoreExternal      bool
}

// DependencyDiscoveryOption is a function that modifies a DependencyDiscovery.
type DependencyDiscoveryOption func(*DependencyDiscovery)

func NewDependencyDiscovery(cfgs component.Components, depthRemaining int) *DependencyDiscovery {
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

func (d *DependencyDiscovery) WithDiscoveryContext(discoveryContext *component.DiscoveryContext) *DependencyDiscovery {
	d.discoveryContext = discoveryContext

	return d
}

func (d *DependencyDiscovery) DiscoverAllDependencies(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	errs := []error{}

	for _, cfg := range d.cfgs {
		if cfg.Kind == component.Stack {
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

func (d *DependencyDiscovery) DiscoverDependencies(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, dCfg *component.Component) error {
	if d.depthRemaining <= 0 {
		return errors.New("max dependency depth reached while discovering dependencies")
	}

	d.depthRemaining--

	defer func() { d.depthRemaining++ }()

	// Stack configs don't have dependencies (at least for now),
	// so we can return early.
	if dCfg.Kind == component.Stack {
		return nil
	}

	// This should only happen if we're discovering an ancestor dependency.
	if dCfg.Parsed == nil {
		err := Parse(dCfg, ctx, l, opts, d.suppressParseErrors, d.parserOptions)
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

			ext := &component.Component{
				Kind:     component.Unit,
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

// RemoveCycles removes cycles from the dependency graph.
func RemoveCycles(c component.Components) (component.Components, error) {
	var (
		err error
		cfg *component.Component
	)

	for range maxCycleRemovalAttempts {
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
