// Package discoveredconfig provides types for representing discovered Terragrunt configurations.
// This package contains only data types and their associated methods, with no discovery logic.
// It exists separately from the discovery package to allow other packages (like filter) to
// depend on these types without creating circular dependencies.
package discoveredconfig

import (
	"sort"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

const (
	// ConfigTypeUnit is the type of Terragrunt configuration for a unit.
	ConfigTypeUnit ConfigType = "unit"
	// ConfigTypeStack is the type of Terragrunt configuration for a stack.
	ConfigTypeStack ConfigType = "stack"
)

// ConfigType is the type of Terragrunt configuration.
type ConfigType string

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
