package graph

import (
	"context"
	"errors"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"

	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"

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

	// Clone options and set RootWorkingDir to rootDir so discovery starts from the graph root
	// This allows discovering all modules including dependents (modules that depend on the working dir)
	graphOpts := opts.Clone()
	graphOpts.RootWorkingDir = rootDir

	stackOpts := []common.Option{}

	r := report.NewReport().WithWorkingDir(opts.WorkingDir)

	if l.Formatter().DisabledColors() || stdout.IsRedirected() {
		r.WithDisableColor()
	}

	if opts.ReportFormat != "" {
		r.WithFormat(opts.ReportFormat)
	}

	if opts.SummaryPerUnit {
		r.WithShowUnitLevelSummary()
	}

	stackOpts = append(stackOpts, common.WithReport(r))

	if opts.ReportSchemaFile != "" {
		defer r.WriteSchemaToFile(opts.ReportSchemaFile) //nolint:errcheck
	}

	if opts.ReportFile != "" {
		defer r.WriteToFile(opts.ReportFile) //nolint:errcheck
	}

	if !opts.SummaryDisable {
		defer r.WriteSummary(opts.Writer) //nolint:errcheck
	}

	stack, err := runner.FindStackInSubfolders(ctx, l, graphOpts, stackOpts...)
	if err != nil {
		return err
	}

	dependentModules := stack.ListStackDependentUnits()

	workDir := opts.WorkingDir
	modulesToInclude := dependentModules[workDir]
	// workdir to list too
	modulesToInclude = append(modulesToInclude, workDir)

	// include from stack only elements from modulesToInclude
	for _, module := range stack.GetStack().Units {
		module.FlagExcluded = true
		if util.ListContainsElement(modulesToInclude, module.Path) {
			module.FlagExcluded = false
		}
	}

	// Rebuild the queue with filtered units
	// The queue needs to be rebuilt because it was created before we set FlagExcluded
	if poolRunner, ok := stack.(*runnerpool.Runner); ok {
		// Get the discovered configs that correspond to non-excluded units
		var filteredDiscovered discovery.DiscoveredConfigs

		for _, unit := range stack.GetStack().Units {
			if !unit.FlagExcluded {
				// Find the corresponding discovered config
				// We need to reconstruct it from the unit
				filteredDiscovered = append(filteredDiscovered, &discovery.DiscoveredConfig{
					Path:   unit.Path,
					Parsed: &unit.Config,
					Type:   discovery.ConfigTypeUnit,
				})
			}
		}

		// Create a new queue with only the filtered units
		newQueue, err := queue.NewQueue(filteredDiscovered)
		if err != nil {
			return err
		}

		// Replace the queue in the runner
		// Note: This requires accessing the unexported queue field, which is not ideal
		// but necessary to make the filtering work
		poolRunner.SetQueue(newQueue)
	}

	return runall.RunAllOnStack(ctx, l, graphOpts, stack)
}
