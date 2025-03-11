// Package discovery provides functionality for discovering Terragrunt configurations.
package discovery

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// ConfigTypeUnit is the type of Terragrunt configuration for a unit.
	ConfigTypeUnit ConfigType = "unit"
	// ConfigTypeStack is the type of Terragrunt configuration for a stack.
	ConfigTypeStack ConfigType = "stack"

	// SortAlpha sorts the discovered configurations in alphabetical order.
	SortAlpha Sort = "alpha"
	// SortDAG sorts the discovered configurations in a topological sort order.
	SortDAG Sort = "dag"
)

// ConfigType is the type of Terragrunt configuration.
type ConfigType string

// Sort is the sort order of the discovered configurations.
type Sort string

// Exclude is the exclude configuration for a discovered configuration.
type Exclude struct {
	If                  string
	Actions             []string
	ExcludeDependencies bool
}

// DiscoveredConfig represents a discovered Terragrunt configuration.
type DiscoveredConfig struct {
	Type ConfigType
	Path string

	Dependencies DiscoveredConfigs
	Exclude      Exclude

	External bool
}

// DiscoveredConfigs is a list of discovered Terragrunt configurations.
type DiscoveredConfigs []*DiscoveredConfig

// Discovery is the configuration for a Terragrunt discovery.
type Discovery struct {
	// workingDir is the directory to search for Terragrunt configurations.
	workingDir string

	// hidden determines whether to detect configurations in hidden directories.
	hidden bool

	// sort determines the sort order of the discovered configurations.
	sort Sort

	// discoverDependencies determines whether to discover dependencies.
	discoverDependencies bool

	// discoverExternalDependencies determines whether to discover external dependencies.
	discoverExternalDependencies bool

	// maxDependencyDepth is the maximum depth of the dependency tree to discover.
	maxDependencyDepth int

	// hiddenDirMemo is a memoization of hidden directories.
	hiddenDirMemo []string
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

	if d.maxDependencyDepth == 0 {
		d.maxDependencyDepth = 1000
	}

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

// String returns a string representation of a DiscoveredConfig.
func (c *DiscoveredConfig) String() string {
	return c.Path
}

// isInHiddenDirectory returns true if the path is in a hidden directory.
func (d *Discovery) isInHiddenDirectory(path string) bool {
	for _, hiddenDir := range d.hiddenDirMemo {
		if strings.HasPrefix(path, hiddenDir) {
			return true
		}
	}

	hiddenPath := ""

	parts := strings.Split(path, string(os.PathSeparator))
	for _, part := range parts {
		hiddenPath = filepath.Join(hiddenPath, part)

		if strings.HasPrefix(part, ".") {
			d.hiddenDirMemo = append(d.hiddenDirMemo, hiddenPath)

			return true
		}
	}

	return false
}

// Discover discovers Terragrunt configurations in the WorkingDir.
func (d *Discovery) Discover(ctx context.Context, opts *options.TerragruntOptions) (DiscoveredConfigs, error) {
	var cfgs DiscoveredConfigs

	walkFn := func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return errors.New(err)
		}

		if e.IsDir() {
			return nil
		}

		if !d.hidden && d.isInHiddenDirectory(path) {
			return nil
		}

		switch filepath.Base(path) {
		case config.DefaultTerragruntConfigPath:
			cfgs = append(cfgs, &DiscoveredConfig{
				Type: ConfigTypeUnit,
				Path: filepath.Dir(path),
			})
		case config.DefaultStackFile:
			cfgs = append(cfgs, &DiscoveredConfig{
				Type: ConfigTypeStack,
				Path: filepath.Dir(path),
			})
		}

		return nil
	}

	if err := filepath.WalkDir(d.workingDir, walkFn); err != nil {
		return nil, errors.New(err)
	}

	if d.discoverDependencies {
		dependencyDiscovery := NewDependencyDiscovery(cfgs, d.maxDependencyDepth, d.discoverExternalDependencies)

		err := dependencyDiscovery.DiscoverAllDependencies(ctx, opts)
		if err != nil {
			return nil, errors.New(err)
		}
	}

	return cfgs, nil
}

type DependencyDiscovery struct {
	cfgs             DiscoveredConfigs
	depthRemaining   int
	discoverExternal bool
	mu               sync.RWMutex
}

func NewDependencyDiscovery(cfgs DiscoveredConfigs, depthRemaining int, discoverExternal bool) *DependencyDiscovery {
	return &DependencyDiscovery{
		cfgs:             cfgs,
		depthRemaining:   depthRemaining,
		discoverExternal: discoverExternal,
		mu:               sync.RWMutex{},
	}
}

func (d *DependencyDiscovery) DiscoverAllDependencies(ctx context.Context, opts *options.TerragruntOptions) error {
	errs := []error{}

	for _, cfg := range d.cfgs {
		if cfg.Type == ConfigTypeStack {
			continue
		}

		err := d.DiscoverDependencies(ctx, opts, cfg)
		if err != nil {
			errs = append(errs, errors.New(err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (d *DependencyDiscovery) DiscoverDependencies(ctx context.Context, opts *options.TerragruntOptions, dCfg *DiscoveredConfig) error {
	if d.depthRemaining <= 0 {
		return errors.New("max dependency depth reached while discovering dependencies")
	}

	opts = opts.Clone()
	opts.WorkingDir = dCfg.Path

	if dCfg.Type == ConfigTypeUnit {
		opts.TerragruntConfigPath = filepath.Join(opts.WorkingDir, config.DefaultTerragruntConfigPath)
	}

	parsingCtx := config.NewParsingContext(ctx, opts).WithDecodeList(
		config.DependenciesBlock,
		config.DependencyBlock,
		config.FeatureFlagsBlock,
		config.ExcludeBlock,
	)

	cfg, err := config.PartialParseConfigFile(parsingCtx, opts.TerragruntConfigPath, nil)
	if err != nil {
		return errors.New(err)
	}

	dependencyBlocks := cfg.TerragruntDependencies

	depPaths := make([]string, 0, len(dependencyBlocks))

	errs := []error{}

	for _, dependency := range dependencyBlocks {
		depPath := dependency.ConfigPath.AsString()

		if !filepath.IsAbs(depPath) {
			depPath = filepath.Join(opts.WorkingDir, depPath)
		}

		depPaths = append(depPaths, depPath)
	}

	if cfg.Dependencies != nil {
		for _, dependency := range cfg.Dependencies.Paths {
			if !filepath.IsAbs(dependency) {
				dependency = filepath.Join(opts.WorkingDir, dependency)
			}

			depPaths = append(depPaths, dependency)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
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

			d.cfgs = append(d.cfgs, ext)

			if d.discoverExternal {
				err := d.DiscoverDependencies(ctx, opts, ext)
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
	filtered := make(DiscoveredConfigs, 0, len(c))

	for _, config := range c {
		if config.Path == path {
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
