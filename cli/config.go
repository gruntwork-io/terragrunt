package cli

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

func configPath(opts *options.TerragruntOptions) (string, error) {
	if path := opts.CLIConfigFile; path != "" {
		path, err := filepath.Abs(path)
		if err != nil {
			return "", errors.New(err)
		}

		if util.FileExists(path) {
			return path, nil
		}
	}

	workingDir, err := filepath.Abs(opts.WorkingDir)
	if err != nil {
		return "", errors.New(err)
	}

	return cliconfig.DiscoveryPath(workingDir)
}

func loadConfig(flags cli.Flags, opts *options.TerragruntOptions) (*cliconfig.Config, error) {
	path, err := configPath(opts)
	if err != nil || path == "" {
		return nil, err
	}

	cfg, err := cliconfig.LoadConfig(path)
	if err != nil {
		return nil, errors.Errorf("could not load CLI config %s: %w", path, err)
	}

	opts.Logger.Debugf("Loaded CLI configuration file %s", path)

	return cfg, nil
}
