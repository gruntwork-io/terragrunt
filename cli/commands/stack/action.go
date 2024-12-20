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

	for _, unit := range stackFile.Units {
		destPath := filepath.Join(baseDir, unit.Path)
		src := unit.Source
		src, err := filepath.Abs(src)
		if err != nil {
			return errors.New(fmt.Errorf("failed to get absolute path for source '%s': %w", unit.Source, err))
		}
		opts.Logger.Infof("Processing unit: %s (%s) to %s", unit.Name, src, destPath)

		req := &getter.Request{
			Src:             src,
			Dst:             destPath,
			GetMode:         getter.ModeAny,
			Copy:            true,
			DisableSymlinks: true,
			Umask:           0755,
		}

		if _, err := getter.DefaultClient.Get(ctx, req); err != nil {
			return fmt.Errorf("failed to fetch source '%s' to destination '%s': %w", unit.Source, destPath, err)
		}
	}

	return nil
}
