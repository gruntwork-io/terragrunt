package stack

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	getter "github.com/hashicorp/go-getter/v2"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	generate      = "generate"
	stackCacheDir = ".terragrunt-stack"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, subCommand string) error {
	if subCommand == "" {
		return errors.New("No subCommand specified")
	}

	switch subCommand {
	case generate:
		{
			return generateStack(ctx, opts)
		}
	}

	return nil
}

func generateStack(ctx context.Context, opts *options.TerragruntOptions) error {
	//TODO: update stack path
	opts.TerragrungStackConfigPath = filepath.Join(opts.WorkingDir, "terragrunt.stack.hcl")
	stackFile, err := config.ReadStackConfigFile(ctx, opts)
	if err != nil {
		return err
	}

	if err := processStackFile(ctx, opts, stackFile); err != nil {
		return err
	}

	return nil
}
func processStackFile(ctx context.Context, opts *options.TerragruntOptions, stackFile *config.StackConfigFile) error {
	baseDir := filepath.Join(opts.WorkingDir, stackCacheDir)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return errors.New(fmt.Errorf("failed to create base directory: %w", err))
	}

	//client := getter.Client{
	//	Getters:         getter.Getters,
	//	Decompressors:   getter.Decompressors,
	//	DisableSymlinks: true,
	//}

	for _, unit := range stackFile.Units {
		destPath := filepath.Join(baseDir, unit.Path)
		dest, err := filepath.Abs(destPath)
		if err != nil {
			return errors.New(fmt.Errorf("failed to get absolute path for destination '%s': %w", destPath, err))
		}

		src := unit.Source
		src, err = filepath.Abs(src)
		if err != nil {
			opts.Logger.Warnf("failed to get absolute path for source '%s': %v", unit.Source, err)
			src = unit.Source
		}
		opts.Logger.Infof("Processing unit: %s (%s) to %s", unit.Name, src, dest)

		if _, err := getter.GetAny(ctx, dest, src); err != nil {
			return fmt.Errorf("failed to fetch source '%s' to destination '%s': %w", unit.Source, destPath, err)
		}
	}

	return nil
}
