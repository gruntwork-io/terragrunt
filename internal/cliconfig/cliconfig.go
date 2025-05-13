// Package cliconfig has the types representing and the logic to load CLI-level
// configuration settings.
package cliconfig

import (
	"encoding/json"
	"os"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/cli"
)

var _ cli.FlagConfigGetter = new(Config)

// Config is the structure of the configuration for the Terragrunt CLI.
type Config struct {
	Flags ConfigFlags `json:"flags"`
}

type ConfigFlag struct {
	Name     string   `json:"name"`
	Value    any      `json:"value"`
	Commands []string `json:"commands"`
}

type ConfigFlags []*ConfigFlag

func (flags ConfigFlags) Find(name string) *ConfigFlag {
	for _, flag := range flags {
		if flag.Name == name {
			return flag
		}
	}

	return nil
}

// LoadConfig returns the loaded configuration at the specified `path`.
func LoadConfig(cfgPath string) (*Config, error) {
	cfg := &Config{}

	if err := cfg.decode(cfgPath); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Get implements `cli.FlagConfigGetter` interface.
func (cfg *Config) Get(command *cli.Command, flag cli.Flag) (string, any) {
	cfgFlag := cfg.Flags.Find(flag.GetConfigKey())
	if cfgFlag == nil {
		return "", nil
	}

	if cfgFlag.Commands != nil && !slices.Contains(cfgFlag.Commands, command.Name) {
		return "", nil
	}

	return cfgFlag.Name, cfgFlag.Value
}

func (cfg *Config) decode(cfgPath string) error {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return NewFileReadError(cfgPath, err)
	}

	if data == nil {
		return nil
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return NewDecodeError(cfgPath, err)
	}

	return nil
}
