// Package discovery provides functionality for discovering Terragrunt configurations.
package discovery

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
)

type ConfigType string

const (
	ConfigTypeUnit  ConfigType = "unit"
	ConfigTypeStack ConfigType = "stack"
)

// DiscoveredConfig represents a discovered Terragrunt configuration.
type DiscoveredConfig struct {
	Type ConfigType `json:"type"`
	Path string     `json:"path"`
}

func (c *DiscoveredConfig) ConfigType() ConfigType {
	return c.Type
}

func (c *DiscoveredConfig) String() string {
	return string(c.Type) + ": " + c.Path
}

type DiscoveredConfigs []*DiscoveredConfig

type Discovery struct {
	// WorkingDir is the directory to search for Terragrunt configurations.
	WorkingDir string

	// Hidden determines whether to detect configurations in hidden directories.
	Hidden bool
}

func NewDiscoverySettings() *Discovery {
	return &Discovery{
		Hidden: false,
	}
}

func WithHidden(hidden bool) *Discovery {
	return &Discovery{
		Hidden: hidden,
	}
}

type DiscoveryOption func(*Discovery)

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

func (d *Discovery) Discover() (DiscoveredConfigs, error) {
	var units DiscoveredConfigs

	walkFn := func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return errors.New(err.Error())
		}

		if e.IsDir() {
			return nil
		}

		path, err = filepath.Rel(d.WorkingDir, path)
		if err != nil {
			return errors.New(err.Error())
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
		return nil, errors.New(err.Error())
	}

	return units, nil
}

func isInHiddenDirectory(path string) bool {
	parts := strings.Split(path, string(os.PathSeparator))
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}

	return false
}

func (c DiscoveredConfigs) Sort() DiscoveredConfigs {
	sort.Slice(c, func(i, j int) bool {
		return c[i].Path < c[j].Path
	})

	return c
}

func (c DiscoveredConfigs) Filter(configType ConfigType) DiscoveredConfigs {
	filtered := make(DiscoveredConfigs, 0, len(c))

	for _, config := range c {
		if config.Type == configType {
			filtered = append(filtered, config)
		}
	}

	return filtered
}

func (c DiscoveredConfigs) FilterByPath(path string) DiscoveredConfigs {
	filtered := make(DiscoveredConfigs, 0, len(c))

	for _, config := range c {
		if config.Path == path {
			filtered = append(filtered, config)
		}
	}

	return filtered
}

func (c DiscoveredConfigs) Paths() []string {
	paths := make([]string, 0, len(c))
	for _, config := range c {
		paths = append(paths, config.Path)
	}

	return paths
}
