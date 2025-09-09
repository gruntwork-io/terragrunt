// Package runnerpool provides a runner implementation based on a pool pattern for executing multiple units concurrently.
package runnerpool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/gruntwork-io/terragrunt/tf"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// Runner implements the Stack interface for runner pool execution.
type Runner struct {
	Stack            *common.Stack
	queue            *queue.Queue
	planErrorBuffers []bytes.Buffer
}

// NewRunnerPoolStack creates a new stack from discovered units.
func NewRunnerPoolStack(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, discovered discovery.DiscoveredConfigs, opts ...common.Option) (common.StackRunner, error) {
	if len(discovered) == 0 {
		return nil, common.ErrNoUnitsFound
	}

	// Initialize stack; queue will be constructed after resolving units so we can filter excludes first.
	stack := common.Stack{
		TerragruntOptions: terragruntOptions,
		ParserOptions:     config.DefaultParserOptions(l, terragruntOptions),
	}

	runner := &Runner{
		Stack: &stack,
	}

	// Collect all terragrunt.hcl paths for resolution.
	unitPaths := make([]string, 0, len(discovered))
	for _, cfg := range discovered {
		if cfg.Parsed == nil {
			// Skip configurations that could not be parsed
			l.Warnf("Skipping unit at %s due to parse error", cfg.Path)
			continue
		}

		terragruntConfigPath := config.GetDefaultConfigPath(cfg.Path)
		unitPaths = append(unitPaths, terragruntConfigPath)
	}

	// Resolve units (this applies include/exclude logic and sets FlagExcluded accordingly).
	unitResolver, err := common.NewUnitResolver(ctx, runner.Stack)
	if err != nil {
		return nil, err
	}

	unitsMap, err := unitResolver.ResolveTerraformModules(ctx, l, unitPaths)
	if err != nil {
		return nil, err
	}

	runner.Stack.Units = unitsMap

	// Build queue from discovered configs, excluding units flagged as excluded and pruning excluded dependencies.
	// This ensures excluded units are not shown in lists or scheduled at all.
	filtered := filterDiscoveredUnits(discovered, unitsMap)
	if len(filtered) == 0 {
		return nil, common.ErrNoUnitsFound
	}

	q, queueErr := queue.NewQueue(filtered)
	if queueErr != nil {
		return nil, queueErr
	}

	runner.queue = q

	return runner.WithOptions(opts...), nil
}

// Run executes the stack according to TerragruntOptions and returns the first
// error (or a joined error) once execution is finished.
func (r *Runner) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	terraformCmd := opts.TerraformCommand

	if opts.OutputFolder != "" {
		for _, u := range r.Stack.Units {
			planFile := u.OutputFile(l, opts)
			if err := os.MkdirAll(filepath.Dir(planFile), os.ModePerm); err != nil {
				return err
			}
		}
	}

	if util.ListContainsElement(config.TerraformCommandsNeedInput, terraformCmd) {
		opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-input=false", 1)
		r.syncTerraformCliArgs(l, opts)
	}

	var planDefer bool

	switch terraformCmd {
	case tf.CommandNameApply, tf.CommandNameDestroy:
		r.handleApplyDestroy(l, opts)
	case tf.CommandNameShow:
		r.handleShow(l, opts)
	case tf.CommandNamePlan:
		r.handlePlan()

		planDefer = true
	}

	if planDefer {
		defer r.summarizePlanAllErrors(l, r.planErrorBuffers)
	}

	// Emit report entries for excluded units (if experiment enabled). Queue already excludes them.
	if r.queue != nil && opts.Experiments.Evaluate(experiment.Report) && r.Stack.Report != nil {
		for _, u := range r.Stack.Units {
			if u.FlagExcluded {
				run, err := r.Stack.Report.EnsureRun(u.Path)
				if err != nil {
					l.Errorf("Error ensuring run for unit %s: %v", u.Path, err)
					continue
				}

				if err := r.Stack.Report.EndRun(
					run.Path,
					report.WithResult(report.ResultExcluded),
					report.WithReason(report.ReasonExcludeBlock),
				); err != nil {
					l.Errorf("Error ending run for unit %s: %v", u.Path, err)
				}
			}
		}
	}

	task := func(ctx context.Context, u *common.Unit) error {
		return telemetry.TelemeterFromContext(ctx).Collect(ctx, "runner_pool_task", map[string]any{
			"terraform_command":      u.TerragruntOptions.TerraformCommand,
			"terraform_cli_args":     u.TerragruntOptions.TerraformCliArgs,
			"working_dir":            u.TerragruntOptions.WorkingDir,
			"terragrunt_config_path": u.TerragruntOptions.TerragruntConfigPath,
		}, func(childCtx context.Context) error {
			unitRunner := common.NewUnitRunner(u)
			return unitRunner.Run(childCtx, u.TerragruntOptions, r.Stack.Report)
		})
	}

	r.queue.FailFast = opts.FailFast
	r.queue.IgnoreDependencyOrder = opts.IgnoreDependencyOrder
	controller := NewController(
		r.queue,
		r.Stack.Units,
		WithRunner(task),
		WithMaxConcurrency(opts.Parallelism),
	)

	return controller.Run(ctx, l)
}

// handleApplyDestroy handles logic for apply and destroy commands.
func (r *Runner) handleApplyDestroy(l log.Logger, opts *options.TerragruntOptions) {
	if opts.RunAllAutoApprove {
		opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-auto-approve", 1)
	}

	r.syncTerraformCliArgs(l, opts)
}

// handleShow handles logic for show command.
func (r *Runner) handleShow(l log.Logger, opts *options.TerragruntOptions) {
	r.syncTerraformCliArgs(l, opts)
}

// handlePlan handles logic for plan command, including error buffer setup and summary.
func (r *Runner) handlePlan() {
	r.planErrorBuffers = make([]bytes.Buffer, len(r.Stack.Units))
	for i, u := range r.Stack.Units {
		u.TerragruntOptions.ErrWriter = io.MultiWriter(&r.planErrorBuffers[i], u.TerragruntOptions.ErrWriter)
	}
}

// LogUnitDeployOrder logs the order of units to be processed for a given Terraform command.
func (r *Runner) LogUnitDeployOrder(l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf("The runner-pool runner at %s will be processed in the following order for command %s:\n", r.Stack.TerragruntOptions.WorkingDir, terraformCommand)
	for _, unit := range r.queue.Entries {
		outStr += fmt.Sprintf("- Unit %s\n", unit.Config.Path)
	}

	l.Info(outStr)

	return nil
}

// JSONUnitDeployOrder returns the order of units to be processed for a given Terraform command in JSON format.
func (r *Runner) JSONUnitDeployOrder(_ string) (string, error) {
	orderedUnits := make([]string, 0, len(r.queue.Entries))
	for _, unit := range r.queue.Entries {
		orderedUnits = append(orderedUnits, unit.Config.Path)
	}

	j, err := json.MarshalIndent(orderedUnits, "", "  ")
	if err != nil {
		return "", err
	}

	return string(j), nil
}

// ListStackDependentUnits returns a map of units and their dependent units in the stack.
func (r *Runner) ListStackDependentUnits() map[string][]string {
	dependentUnits := make(map[string][]string)

	for _, unit := range r.queue.Entries {
		if len(unit.Config.Dependencies) != 0 {
			for _, dep := range unit.Config.Dependencies {
				dependentUnits[dep.Path] = util.RemoveDuplicatesFromList(append(dependentUnits[dep.Path], unit.Config.Path))
			}
		}
	}

	for {
		noUpdates := true

		for unit, dependents := range dependentUnits {
			for _, dependent := range dependents {
				initialSize := len(dependentUnits[unit])
				list := util.RemoveDuplicatesFromList(append(dependentUnits[unit], dependentUnits[dependent]...))
				list = util.RemoveElementFromList(list, unit)
				dependentUnits[unit] = list

				if initialSize != len(dependentUnits[unit]) {
					noUpdates = false
				}
			}
		}

		if noUpdates {
			break
		}
	}

	return dependentUnits
}

// syncTerraformCliArgs syncs the Terraform CLI arguments for each unit in the stack based on the provided Terragrunt options.
func (r *Runner) syncTerraformCliArgs(l log.Logger, opts *options.TerragruntOptions) {
	for _, unit := range r.Stack.Units {
		unit.TerragruntOptions.TerraformCliArgs = collections.MakeCopyOfList(opts.TerraformCliArgs)

		planFile := unit.PlanFile(l, opts)
		if planFile != "" {
			l.Debugf("Using output file %s for unit %s", planFile, unit.TerragruntOptions.TerragruntConfigPath)

			if unit.TerragruntOptions.TerraformCommand == tf.CommandNamePlan {
				// for plan command add -out=<file> to the terraform cli args
				unit.TerragruntOptions.TerraformCliArgs = append(unit.TerragruntOptions.TerraformCliArgs, "-out="+planFile)
				continue
			}

			unit.TerragruntOptions.TerraformCliArgs = append(unit.TerragruntOptions.TerraformCliArgs, planFile)
		}
	}
}

// summarizePlanAllErrors summarizes all errors encountered during the plan phase across all units in the stack.
func (r *Runner) summarizePlanAllErrors(l log.Logger, errorStreams []bytes.Buffer) {
	for i, errorStream := range errorStreams {
		output := errorStream.String()

		if len(output) == 0 {
			// We get Finished buffer if runner execution completed without errors, so skip that to avoid logging too much
			continue
		}

		if strings.Contains(output, "Error running plan:") && strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
			var dependenciesMsg string

			if len(r.Stack.Units[i].Dependencies) > 0 {
				if r.Stack.Units[i].Config.Dependencies != nil {
					dependenciesMsg = fmt.Sprintf(" contains dependencies to %v and", r.Stack.Units[i].Config.Dependencies.Paths)
				} else {
					dependenciesMsg = " contains dependencies and"
				}
			}

			l.Infof("%v%v refers to remote State "+
				"you may have to apply your changes in the dependencies prior running terragrunt run --all plan.\n",
				r.Stack.Units[i].Path,
				dependenciesMsg,
			)
		}
	}
}

// filterDiscoveredUnits removes configs for units flagged as excluded and prunes dependencies
// that point to excluded units. This keeps the execution queue and any user-facing listings
// free from units not intended to run.
//
// Inputs:
//   - discovered: raw discovery results (paths and dependency edges)
//   - units: resolved units (slice), where exclude rules have already been applied
//
// Behavior:
//   - A config is included only if there's a corresponding unit and its FlagExcluded is false.
//   - For each included config, its Dependencies list is filtered to only include included configs.
//   - The function returns a new slice with shallow-copied entries so the original discovery
//     results remain unchanged.
func filterDiscoveredUnits(discovered discovery.DiscoveredConfigs, units common.Units) discovery.DiscoveredConfigs {
	// Build allowlist from non-excluded unit paths
	allowed := make(map[string]struct{}, len(units))
	for _, u := range units {
		if !u.FlagExcluded {
			allowed[u.Path] = struct{}{}
		}
	}

	filtered := make(discovery.DiscoveredConfigs, 0, len(discovered))

	for _, cfg := range discovered {
		if _, ok := allowed[cfg.Path]; !ok {
			// Drop configs that map to excluded/missing units
			continue
		}

		// Shallow copy to avoid mutating original discovery graph
		copyCfg := &discovery.DiscoveredConfig{
			Parsed:           cfg.Parsed,
			DiscoveryContext: cfg.DiscoveryContext,
			Type:             cfg.Type,
			Path:             cfg.Path,
			External:         cfg.External,
		}

		if cfg.Dependencies != nil {
			deps := make(discovery.DiscoveredConfigs, 0, len(cfg.Dependencies))
			for _, dep := range cfg.Dependencies {
				if _, ok := allowed[dep.Path]; ok {
					deps = append(deps, dep)
				}
			}

			copyCfg.Dependencies = deps
		}

		filtered = append(filtered, copyCfg)
	}

	return filtered
}

// WithOptions updates the stack with the provided options.
func (r *Runner) WithOptions(opts ...common.Option) *Runner {
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// GetStack returns the stack associated with the runner.
func (r *Runner) GetStack() *common.Stack {
	return r.Stack
}

// SetTerragruntConfig sets the config for the stack.
func (r *Runner) SetTerragruntConfig(config *config.TerragruntConfig) {
	r.Stack.ChildTerragruntConfig = config
}

// SetParseOptions sets the ParseOptions for the stack.
func (r *Runner) SetParseOptions(parserOptions []hclparse.Option) {
	r.Stack.ParserOptions = parserOptions
}

// SetReport sets the report for the stack.
func (r *Runner) SetReport(report *report.Report) {
	r.Stack.Report = report
}
