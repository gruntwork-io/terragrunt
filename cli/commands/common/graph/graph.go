package graph

import (
	"context"
	"errors"

	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	cfg, err := config.ReadTerragruntConfig(ctx, l, opts, config.DefaultParserOptions(l, opts))
	if err != nil {
		return err
	}

	if cfg == nil {
		return errors.New("terragrunt was not able to render the config as json because it received no config. This is almost certainly a bug in Terragrunt. Please open an issue on github.com/gruntwork-io/terragrunt with this message and the contents of your terragrunt.hcl")
	}
	// consider root for graph identification passed destroy-graph-root argument
	rootDir := opts.GraphRoot

	// if destroy-graph-root is empty, use git to find top level dir.
	// may cause issues if in the same repo exist unrelated modules which will generate errors when scanning.
	if rootDir == "" {
		gitRoot, err := shell.GitTopLevelDir(ctx, l, opts, opts.WorkingDir)
		if err != nil {
			return err
		}

		rootDir = gitRoot
	}

	l, rootOptions, err := opts.CloneWithConfigPath(l, rootDir)
	if err != nil {
		return err
	}

	rootOptions.WorkingDir = rootDir

	stack, err := configstack.FindStackInSubfolders(ctx, l, rootOptions)
	if err != nil {
		return err
	}

	dependentModules := stack.ListStackDependentModules()

	workDir := opts.WorkingDir
	modulesToInclude := dependentModules[workDir]
	// workdir to list too
	modulesToInclude = append(modulesToInclude, workDir)

	// include from stack only elements from modulesToInclude
	for _, module := range stack.Modules {
		module.FlagExcluded = true
		if util.ListContainsElement(modulesToInclude, module.Path) {
			module.FlagExcluded = false
		}
	}

	return runall.RunAllOnStack(ctx, l, opts, stack)
}
