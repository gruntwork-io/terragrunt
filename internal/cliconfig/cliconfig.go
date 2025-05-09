// Package cliconfig has the types representing and the logic to load CLI-level
// configuration settings.
package cliconfig

import (
	"encoding/json"
	"os"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cli"
)

var _ cli.FlagConfigGetter = new(Config)

// Config is the structure of the configuration for the Terragrunt CLI.
type Config struct {
	keysValues     map[string]any
	normalizedKeys map[string]string
	path           string
	usedKeys       []string
}

func LoadConfig(cfgPath string) (*Config, error) {
	cfg := &Config{
		keysValues:     make(map[string]any),
		normalizedKeys: make(map[string]string),
		path:           cfgPath,
	}

	if err := cfg.decode(cfgPath); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (cfg *Config) Path() string {
	return cfg.path
}

func (cfg *Config) ExtraKeys() []string {
	var extraKeys []string

	for key := range cfg.keysValues {
		if !slices.Contains(cfg.usedKeys, key) {
			extraKeys = append(extraKeys, key)
		}
	}

	return extraKeys
}

func (cfg *Config) Get(key string) (string, any) {
	if key = cfg.rawKey(key); key == "" {
		return "", nil
	}

	cfg.usedKeys = append(cfg.usedKeys, key)

	return key, cfg.keysValues[key]
}

func (cfg *Config) rawKey(key string) string {
	if key, ok := cfg.normalizedKeys[key]; ok {
		return key
	}

	return ""
}

func (cfg *Config) decode(cfgPath string) error {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return NewFileReadError(cfgPath, err)
	}

	if data == nil {
		return nil
	}

	if err := json.Unmarshal(data, &cfg.keysValues); err != nil {
		return NewDecodeError(cfgPath, err)
	}

	for key := range cfg.keysValues {
		normalizedKey := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		cfg.normalizedKeys[normalizedKey] = key
	}

	return nil
}
