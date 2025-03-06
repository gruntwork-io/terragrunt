// Package discovery provides functionality for discovering Terragrunt configurations.

package discovery

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/gruntwork-io/terragrunt/options"
)

type ConfigType string

const (
	ConfigTypeUnit  ConfigType = "unit"
	ConfigTypeStack ConfigType = "stack"
)

// DiscoveredConfig represents a discovered Terragrunt configuration.
type DiscoveredConfig struct {
	config ConfigType
	path   string
}

func (c *DiscoveredConfig) Path() string {
	return c.path
}

func (c *DiscoveredConfig) ConfigType() ConfigType {
	return c.config
}

func (c *DiscoveredConfig) String() string {
	return string(c.config) + ": " + c.path
}

type DiscoveredConfigs []*DiscoveredConfig

func DiscoverConfigs(opts *options.TerragruntOptions) (DiscoveredConfigs, error) {
	var units DiscoveredConfigs

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.New(err.Error())
		}

		if info.IsDir() {
			return nil
		}

		if filepath.Base(path) == filepath.Base(opts.TerragruntConfigPath) {
			relPath, err := filepath.Rel(opts.WorkingDir, path)
			if err != nil {
				return errors.New(err.Error())
			}

			units = append(units, &DiscoveredConfig{
				config: ConfigTypeUnit,
				path:   filepath.Dir(relPath),
			})
		}

		if filepath.Base(path) == filepath.Base(opts.TerragruntStackConfigPath) {
			relPath, err := filepath.Rel(opts.WorkingDir, path)
			if err != nil {
				return errors.New(err.Error())
			}

			units = append(units, &DiscoveredConfig{
				config: ConfigTypeStack,
				path:   filepath.Dir(relPath),
			})
		}

		return nil
	}

	if err := filepath.Walk(opts.WorkingDir, walkFn); err != nil {
		return nil, errors.New(err.Error())
	}

	return units, nil
}

func (c DiscoveredConfigs) Sort() DiscoveredConfigs {
	sort.Slice(c, func(i, j int) bool {
		return c[i].path < c[j].path
	})

	return c
}

func (c DiscoveredConfigs) Filter(configType ConfigType) DiscoveredConfigs {
	filtered := make(DiscoveredConfigs, 0, len(c))
	for _, config := range c {
		if config.config == configType {
			filtered = append(filtered, config)
		}
	}

	return filtered
}

func (c DiscoveredConfigs) FilterByPath(path string) DiscoveredConfigs {
	filtered := make(DiscoveredConfigs, 0, len(c))
	for _, config := range c {
		if config.path == path {
			filtered = append(filtered, config)
		}
	}

	return filtered
}

func (c DiscoveredConfigs) Paths() []string {
	paths := make([]string, 0, len(c))
	for _, config := range c {
		paths = append(paths, config.path)
	}

	return paths
}
