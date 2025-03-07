// Package discovery provides functionality for discovering Terragrunt configurations.
package discovery

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

const (
	ConfigTypeUnit  ConfigType = "unit"
	ConfigTypeStack ConfigType = "stack"
)

// ConfigType is the type of Terragrunt configuration.
type ConfigType string

// DiscoveredConfig represents a discovered Terragrunt configuration.
type DiscoveredConfig struct {
	Type ConfigType `json:"type"`
	Path string     `json:"path"`
}

// DiscoveredConfigs is a list of discovered Terragrunt configurations.
type DiscoveredConfigs []*DiscoveredConfig

// Discovery is the configuration for a Terragrunt discovery.
type Discovery struct {
	// WorkingDir is the directory to search for Terragrunt configurations.
	WorkingDir string

	// Hidden determines whether to detect configurations in hidden directories.
	Hidden bool
}

// DiscoveryOption is a function that modifies a Discovery.
type DiscoveryOption func(*Discovery)

// NewDiscovery creates a new Discovery.
func NewDiscovery(dir string, opts ...DiscoveryOption) *Discovery {
	discovery := &Discovery{
		WorkingDir: dir,
		Hidden:     false,
	}

	for _, opt := range opts {
		opt(discovery)
	}

	return discovery
}

// NewDiscoverySettings creates a new Discovery with default settings.
func NewDiscoverySettings() *Discovery {
	return &Discovery{
		Hidden: false,
	}
}

// WithHidden sets the Hidden flag to true.
func (d *Discovery) WithHidden() *Discovery {
	d.Hidden = true

	return d
}

// String returns a string representation of a DiscoveredConfig.
func (c *DiscoveredConfig) String() string {
	return string(c.Type) + ": " + c.Path
}

// Discover discovers Terragrunt configurations in the WorkingDir.
func (d *Discovery) Discover() (DiscoveredConfigs, error) {
	var units DiscoveredConfigs

	walkFn := func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return errors.New(err)
		}

		if e.IsDir() {
			return nil
		}

		path, err = filepath.Rel(d.WorkingDir, path)
		if err != nil {
			return errors.New(err)
		}

		if !d.Hidden && isInHiddenDirectory(path) {
			return nil
		}

		switch filepath.Base(path) {
		case config.DefaultTerragruntConfigPath:
			units = append(units, &DiscoveredConfig{
				Type: ConfigTypeUnit,
				Path: filepath.Dir(path),
			})
		case config.DefaultStackFile:
			units = append(units, &DiscoveredConfig{
				Type: ConfigTypeStack,
				Path: filepath.Dir(path),
			})
		}

		return nil
	}

	if err := filepath.WalkDir(d.WorkingDir, walkFn); err != nil {
		return nil, errors.New(err)
	}

	return units, nil
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

// isInHiddenDirectory returns true if the path is in a hidden directory.
func isInHiddenDirectory(path string) bool {
	parts := strings.Split(path, string(os.PathSeparator))
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}

	return false
}
