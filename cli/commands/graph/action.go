package graph

import (
	"context"
	"errors"
	"fmt"

	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

// Run the graph command.
func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	target := terraform.NewTarget(terraform.TargetPointParseConfig, graph)

	err := terraform.RunWithTarget(ctx, opts, target)
	if err != nil {
		return fmt.Errorf("error running graph command: %w", err)
	}

	return nil
}

var (
	// ErrNoConfig is returned when the terragrunt config is nil.
	ErrNoConfig = errors.New("terragrunt was not able to render the config as json because it received no config. This is almost certainly a bug in Terragrunt. Please open an issue on github.com/gruntwork-io/terragrunt with this message and the contents of your terragrunt.hcl") //nolint:lll
)

func graph(ctx context.Context, opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
	if cfg == nil {
		return ErrNoConfig
	}
	// consider root for graph identification passed destroy-graph-root argument
	rootDir := opts.GraphRoot

	// if destroy-graph-root is empty, use git to find top level dir.
	// may cause issues if in the same repo exist unrelated modules which will generate errors when scanning.
	if rootDir == "" {
		gitRoot, err := shell.GitTopLevelDir(ctx, opts, opts.WorkingDir)
		if err != nil {
			return fmt.Errorf("error when trying to find the root of the git repo: %w", err)
		}

		rootDir = gitRoot
	}

	rootOptions := opts.Clone(rootDir)
	rootOptions.WorkingDir = rootDir

	stack, err := configstack.FindStackInSubfolders(ctx, rootOptions)

	if err != nil {
		return fmt.Errorf("error when trying to find the stack in subfolders: %w", err)
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

	err = runall.OnStack(ctx, opts, stack)
	if err != nil {
		return fmt.Errorf("error when trying to run all on stack: %w", err)
	}

	return nil
}
