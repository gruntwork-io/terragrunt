package stack

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/hashicorp/go-getter/v2"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	ManifestName = ".terragrunt-stack-manifest"
)

func generateStack(ctx context.Context, opts *options.TerragruntOptions) error {
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, defaultStackFile)
	opts.Logger.Infof("Generating stack from %s", opts.TerragruntStackConfigPath)
	stackFile, err := config.ReadStackConfigFile(ctx, opts)

	if err != nil {
		return errors.New(err)
	}

	if err := processStackFile(ctx, opts, stackFile); err != nil {
		return errors.New(err)
	}

	return nil
}

func processStackFile(ctx context.Context, opts *options.TerragruntOptions, stackFile *config.StackConfigFile) error {
	baseDir := filepath.Join(opts.WorkingDir, stackDir)
	if err := os.MkdirAll(baseDir, dirPerm); err != nil {
		return errors.New(fmt.Errorf("failed to create base directory: %w", err))
	}

	for _, unit := range stackFile.Units {
		opts.Logger.Infof("Processing unit %s", unit.Name)

		destPath := filepath.Join(baseDir, unit.Path)
		dest, err := filepath.Abs(destPath)

		if err != nil {
			return errors.New(fmt.Errorf("failed to get absolute path for destination '%s': %w", dest, err))
		}

		src := unit.Source
		opts.Logger.Debugf("Processing unit: %s (%s) to %s", unit.Name, src, dest)

		if isLocal(opts, src) {
			src = filepath.Join(opts.WorkingDir, unit.Source)
			src, err = filepath.Abs(src)

			if err != nil {
				opts.Logger.Warnf("failed to get absolute path for source '%s': %v", unit.Source, err)
				src = unit.Source
			}

			if err := util.CopyFolderContentsWithFilter(opts.Logger, src, dest, ManifestName, func(absolutePath string) bool {
				return true
			}); err != nil {
				return errors.New(err)
			}
		} else {
			if err := os.MkdirAll(dest, dirPerm); err != nil {
				return errors.New(err)
			}

			if _, err := getter.GetAny(ctx, dest, src); err != nil {
				return errors.New(err)
			}
		}
	}

	return nil
}

func isLocal(opts *options.TerragruntOptions, src string) bool {
	// check initially if the source is a local file
	src = filepath.Join(opts.WorkingDir, src)
	if util.FileExists(src) {
		return true
	}
	// check path through getters
	req := &getter.Request{
		Src: src,
	}
	for _, g := range getter.Getters {
		recognized, err := getter.Detect(req, g)
		if err != nil {
			opts.Logger.Debugf("Error detecting getter for %s: %v", src, err)
			continue
		}

		if recognized {
			break
		}
	}

	return strings.HasPrefix(req.Src, "file://")
}
