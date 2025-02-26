package stack

import (
	"context"
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
	opts.Logger.Infof("Generating stack from %s", opts.TerragruntStackConfigPath)
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, defaultStackFile)
	// process recursively stack directory
	if err := processStackDir(ctx, opts, opts.TerragruntStackConfigPath); err != nil {
		return errors.New(err)
	}
	return nil
}

func processStackDir(ctx context.Context, opts *options.TerragruntOptions, stackFilePath string) error {
	stackSourceDir := filepath.Dir(stackFilePath)
	stackFile, err := config.ReadStackConfigFile(ctx, opts, stackFilePath)
	if err != nil {
		return errors.Errorf("Failed to read stack file %s in %s %v", stackFilePath, stackSourceDir, err)
	}
	stackTargetDir := filepath.Join(stackSourceDir, stackDir)
	if err := os.MkdirAll(stackTargetDir, os.ModePerm); err != nil {
		return errors.Errorf("failed to create base directory: %s %v", stackTargetDir, err)
	}

	if err := generateUnits(ctx, opts, stackSourceDir, stackTargetDir, stackFile.Units); err != nil {
		return err
	}

	if err := generateStacks(ctx, opts, stackSourceDir, stackTargetDir, stackFile.Stacks); err != nil {
		return err
	}

	return nil
}

func generateUnits(ctx context.Context, opts *options.TerragruntOptions, stackSourceDir, stackTargetDir string, units []*config.Unit) error {
	for _, unit := range units {
		opts.Logger.Infof("Processing unit %s", unit.Name)

		destPath := filepath.Join(stackTargetDir, unit.Path)
		dest, err := filepath.Abs(destPath)

		if err != nil {
			return errors.Errorf("failed to get absolute path for destination '%s': %v", dest, err)
		}

		src := unit.Source
		opts.Logger.Debugf("Processing unit: %s (%s) to %s", unit.Name, src, dest)

		if err := copyFiles(ctx, opts, unit.Name, stackSourceDir, src, dest); err != nil {
			return err
		}

		// generate unit values file
		if err := config.WriteUnitValues(opts, unit, dest); err != nil {
			return errors.Errorf("Failed to write unit values %v %v", unit.Name, err)
		}
	}

	return nil
}

func generateStacks(ctx context.Context, opts *options.TerragruntOptions, stackSourceDir, stackTargetDir string, stacks []*config.Stack) error {
	var stackDirsToProcess = make(map[string]string, len(stacks))
	for _, stack := range stacks {
		opts.Logger.Infof("Processing stack %s", stack.Name)

		destPath := filepath.Join(stackTargetDir, stack.Path)
		dest, err := filepath.Abs(destPath)

		if err != nil {
			return errors.Errorf("Failed to get absolute path for destination '%s': %v", dest, err)
		}

		src := stack.Source
		opts.Logger.Debugf("Processing stack: %s (%s) to %s", stack.Name, src, dest)

		if err := copyFiles(ctx, opts, stack.Name, stackSourceDir, src, dest); err != nil {
			return err
		}
		stackDirsToProcess[src] = dest
	}

	// process stack dirs
	for src, dest := range stackDirsToProcess {
		if err := processStackDir(ctx, opts, src, dest); err != nil {
			return err
		}
	}

	return nil
}

func copyFiles(ctx context.Context, opts *options.TerragruntOptions, identifier, sourceDir, src, dest string) error {
	if isLocal(opts, sourceDir, src) {
		localSrc := filepath.Join(sourceDir, src)
		localSrc, err := filepath.Abs(localSrc)

		if err != nil {
			opts.Logger.Warnf("failed to get absolute path for source '%s': %v", identifier, err)
			// fallback to original source
			localSrc = src
		}

		if err := util.CopyFolderContentsWithFilter(opts.Logger, localSrc, dest, ManifestName, func(absolutePath string) bool {
			return true
		}); err != nil {
			return errors.Errorf("Failed to copy %s to %s %v", localSrc, dest, err)
		}
	} else {
		if err := os.MkdirAll(dest, os.ModePerm); err != nil {
			return errors.Errorf("Failed to create directory %s for %s %v", dest, identifier, err)
		}

		if _, err := getter.GetAny(ctx, dest, src); err != nil {
			return errors.Errorf("Failed to fetch %s %v", identifier, err)
		}
	}
	return nil
}

func isLocal(opts *options.TerragruntOptions, workingDir, src string) bool {
	// check initially if the source is a local file
	if util.FileExists(src) {
		return true
	}

	src = filepath.Join(workingDir, src)
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
