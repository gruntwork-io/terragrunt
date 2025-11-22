// Package runnerpool provides a runner implementation based on a pool pattern for executing multiple units concurrently.
package runnerpool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/types"

	"github.com/gruntwork-io/terragrunt/tf"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/component"
	tgerrors "github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/hashicorp/hcl/v2"
)

// Runner implements the Stack interface for runner pool execution.
type Runner struct {
	Stack       *component.Stack
	queue       *queue.Queue
	unitFilters []common.UnitFilter
}

// NewRunnerPoolStack creates a new stack from discovered units.
func NewRunnerPoolStack(
	ctx context.Context,
	l log.Logger,
	runnerOptions *types.RunnerOptions,
	discovered component.Components,
	opts ...common.Option,
) (common.StackRunner, error) {
	// Filter out Stack components - we only want Unit components
	// Stack components (terragrunt.stack.hcl files) are for stack generation, not execution
	nonStackComponents := make(component.Components, 0, len(discovered))
	for _, c := range discovered {
		if _, ok := c.(*component.Stack); !ok {
			nonStackComponents = append(nonStackComponents, c)
		}
	}

	if len(nonStackComponents) == 0 {
		l.Warnf("No units discovered. Creating an empty runner.")

		stack := component.NewStack("")
		stack.SetWorkingDir(runnerOptions.WorkingDir)
		// Convert to TerragruntOptions for config package (not yet refactored)
		terragruntOpts := runnerOptions.ToTerragruntOptions()
		stack.SetParserOptions(config.DefaultParserOptions(l, terragruntOpts))

		runner := &Runner{
			Stack: stack,
		}

		// Create an empty queue
		q, queueErr := queue.NewQueue(component.Components{})
		if queueErr != nil {
			return nil, queueErr
		}

		runner.queue = q

		return runner.WithOptions(opts...), nil
	}

	// Initialize stack with options
	// Discovery has already resolved units, applied filters, and set FlagExcluded
	stack := component.NewStack("")
	stack.SetWorkingDir(runnerOptions.WorkingDir)
	// Convert to TerragruntOptions for config package (not yet refactored)
	terragruntOpts := runnerOptions.ToTerragruntOptions()
	stack.SetParserOptions(config.DefaultParserOptions(l, terragruntOpts))

	runner := &Runner{
		Stack: stack,
	}

	// Apply options (including report)
	runner = runner.WithOptions(opts...)

	// Set units on stack (includes all units, even excluded ones for dependency tracking)
	runner.Stack.SetUnits(nonStackComponents)

	// Discovery has set FlagExcluded on units that should not be executed
	// Filter to only executable units for the queue
	executableComponents := make(component.Components, 0, len(nonStackComponents))
	for _, c := range nonStackComponents {
		if u, ok := c.(*component.Unit); ok && !u.FlagExcluded() {
			executableComponents = append(executableComponents, c)
		}
	}

	// Build queue with only executable units
	q, queueErr := queue.NewQueue(executableComponents)
	if queueErr != nil {
		return nil, queueErr
	}

	runner.queue = q

	return runner.WithOptions(opts...), nil
}

// Limit recursive descent when inspecting nested errors
const maxConfigurationErrorDepth = 100

// isConfigurationError checks if an error is a configuration/validation error
// that should always cause command failure regardless of fail-fast setting.
func isConfigurationError(err error) bool {
	return isConfigurationErrorDepth(err, 0)
}

func isConfigurationErrorDepth(err error, depth int) bool {
	if err == nil {
		return false
	}

	if depth >= maxConfigurationErrorDepth {
		return false
	}

	// Check for specific configuration error types
	if tgerrors.IsError(err, config.ConflictingRunCmdCacheOptionsError{}) {
		return true
	}

	// Inspect HCL diagnostics (structured errors) for run_cmd cache-option conflicts
	for _, unwrapped := range tgerrors.UnwrapErrors(err) {
		var diags hcl.Diagnostics
		if errors.As(unwrapped, &diags) {
			for _, d := range diags {
				if d != nil && d.Severity == hcl.DiagError && d.Summary == "Error in function call" {
					return true
				}
			}
		}
	}

	// Check wrapped errors in MultiError
	var multiErr *tgerrors.MultiError
	if errors.As(err, &multiErr) {
		for _, wrappedErr := range multiErr.WrappedErrors() {
			if isConfigurationErrorDepth(wrappedErr, depth+1) {
				return true
			}
		}
	}

	return false
}

// Run executes the stack according to TerragruntOptions and returns the first
// error (or a joined error) once execution is finished.
func (r *Runner) Run(ctx context.Context, l log.Logger, opts *types.RunnerOptions) error {
	terraformCmd := opts.TerraformCommand

	if opts.OutputFolder != "" {
		for _, comp := range r.Stack.Units() {
			u, ok := comp.(*component.Unit)
			if !ok {
				continue
			}

			// Ensure execution options are set on the unit
			// Note: Units should already have ExecutionOptions set during discovery/setup
			planFile := u.GetOutputFile()
			if err := os.MkdirAll(filepath.Dir(planFile), os.ModePerm); err != nil {
				return err
			}
		}
	}

	if util.ListContainsElement(config.TerraformCommandsNeedInput, terraformCmd) {
		opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-input=false", 1)
		r.syncTerraformCliArgs(l, opts)
	}

	var planErrorBuffers []bytes.Buffer

	switch terraformCmd {
	case tf.CommandNameApply, tf.CommandNameDestroy:
		if opts.RunAllAutoApprove {
			opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-auto-approve", 1)
		}

		r.syncTerraformCliArgs(l, opts)
	case tf.CommandNameShow:
		r.syncTerraformCliArgs(l, opts)
	case tf.CommandNamePlan:
		planErrorBuffers = r.handlePlan()
		defer r.summarizePlanAllErrors(l, planErrorBuffers)
	}

	// Discovery has already reported all excluded units, so no need to report them here

	task := func(ctx context.Context, u *component.Unit) error {
		execOpts := u.ExecutionOptions()

		return telemetry.TelemeterFromContext(ctx).Collect(ctx, "runner_pool_task", map[string]any{
			"terraform_command":      execOpts.TerraformCommand,
			"terraform_cli_args":     execOpts.TerraformCliArgs,
			"working_dir":            execOpts.WorkingDir,
			"terragrunt_config_path": execOpts.TerragruntConfigPath,
		}, func(childCtx context.Context) error {
			unitRunner := common.NewUnitRunner(u)

			// Clone stack-level options with unit's config path to get unit-specific WorkingDir
			// This also creates a unit-specific logger with the proper prefix field
			unitLogger, unitOpts, err := opts.CloneWithConfigPath(l, execOpts.TerragruntConfigPath)
			if err != nil {
				return err
			}

			// Preserve the unit's TerraformCliArgs which may include plan file paths
			// set by syncTerraformCliArgs. The cloned opts don't have these unit-specific args.
			unitOpts.TerraformCliArgs = execOpts.TerraformCliArgs

			// Update OriginalTerragruntConfigPath to point to this unit's config.
			// This ensures get_original_terragrunt_dir() returns the correct path for each unit
			// during run --all operations. Each unit should treat its own config as the "original"
			// when running in the runner pool context.
			unitOpts.OriginalTerragruntConfigPath = execOpts.TerragruntConfigPath

			// Update the unit's execution options with the cloned options.
			// This ensures methods like GetOutputFile() and PlanFile() use the correct paths.
			// Convert to TerragruntOptions for the unit (component package not yet refactored)
			u.SetTerragruntOptions(unitOpts.ToTerragruntOptions())

			// Use the unit-specific logger with proper prefix for all unit operations
			return unitRunner.Run(childCtx, unitLogger, unitOpts, r.Stack.Report())
		})
	}

	r.queue.FailFast = opts.FailFast
	r.queue.IgnoreDependencyOrder = opts.IgnoreDependencyOrder
	// Allow continuing the queue when dependencies fail if requested via CLI
	r.queue.IgnoreDependencyErrors = opts.IgnoreDependencyErrors

	// Controller accepts Components directly and extracts Units internally
	controller := NewController(
		r.queue,
		r.Stack.Units(),
		WithRunner(task),
		WithMaxConcurrency(opts.Parallelism),
	)

	err := controller.Run(ctx, l)

	// Emit report entries for early exit units after controller completes
	if r.Stack.Report() != nil {
		// Build a quick lookup of queue entry status by path to avoid nested scans
		statusByPath := make(map[string]queue.Status, len(r.queue.Entries))
		for _, qe := range r.queue.Entries {
			statusByPath[qe.Component.Path()] = qe.Status
		}

		for _, entry := range r.queue.Entries {
			if entry.Status == queue.StatusEarlyExit {
				unitComp := r.Stack.FindUnitByPath(entry.Component.Path())
				if unitComp == nil {
					l.Warnf("Could not find unit for early exit entry: %s", entry.Component.Path())
					continue
				}

				unit, ok := unitComp.(*component.Unit)
				if !ok {
					continue
				}

				// Component paths are always absolute from ingestion (via util.CanonicalPath)
				run, reportErr := r.Stack.Report().EnsureRun(unit.Path())
				if reportErr != nil {
					l.Errorf("Error ensuring run for early exit unit %s: %v", unit.Path(), reportErr)
					continue
				}

				// Find the immediate failed or early-exited ancestor to set as cause
				// If a dependency failed, use it; otherwise if a dependency exited early, use it
				var failedAncestor string

				for _, dep := range entry.Component.Dependencies() {
					status := statusByPath[dep.Path()]
					if status == queue.StatusFailed {
						failedAncestor = filepath.Base(dep.Path())
						break
					}

					if status == queue.StatusEarlyExit && failedAncestor == "" {
						// Use early exit dependency as fallback
						failedAncestor = filepath.Base(dep.Path())
					}
				}

				endOpts := []report.EndOption{
					report.WithResult(report.ResultEarlyExit),
					report.WithReason(report.ReasonAncestorError),
				}
				if failedAncestor != "" {
					endOpts = append(endOpts, report.WithCauseAncestorExit(failedAncestor))
				}

				if endErr := r.Stack.Report().EndRun(run.Path, endOpts...); endErr != nil {
					l.Errorf("Error ending run for early exit unit %s: %v", unit.Path(), endErr)
				}
			}
		}
	}

	// Handle errors based on fail-fast mode and error type
	// Configuration errors always fail regardless of --fail-fast
	// Execution errors are suppressed when --fail-fast is not set
	if err != nil {
		if isConfigurationError(err) || opts.FailFast {
			// Configuration errors or fail-fast mode: propagate error
			return err
		}

		// Execution errors without fail-fast: log but don't fail
		l.Errorf("Run failed: %v", err)

		// Set detailed exit code if context has one
		exitCode := tf.DetailedExitCodeFromContext(ctx)
		if exitCode != nil {
			exitCode.Set(int(cli.ExitCodeGeneralError))
		}

		// Return nil to indicate success (no --fail-fast) but errors were logged
		return nil
	}

	return err
}

// handlePlan handles logic for plan command, including error buffer setup and summary.
// Returns error buffers for each unit to capture stderr output for later analysis.
func (r *Runner) handlePlan() []bytes.Buffer {
	return slices.Collect(r.planBuffersIter())
}

// planBuffersIter returns an iterator that yields error buffers for each unit in the stack.
// For each unit, it configures a buffer to capture errors via MultiWriter and yields the buffer.
func (r *Runner) planBuffersIter() iter.Seq[bytes.Buffer] {
	return func(yield func(bytes.Buffer) bool) {
		for _, comp := range r.Stack.Units() {
			u, ok := comp.(*component.Unit)
			if !ok {
				continue
			}

			var buf bytes.Buffer

			execOpts := u.ExecutionOptions()
			if execOpts != nil {
				execOpts.ErrWriter = io.MultiWriter(&buf, execOpts.ErrWriter)
				u.SetExecutionOptions(execOpts)
			}

			if !yield(buf) {
				return
			}
		}
	}
}

// LogUnitDeployOrder logs the order of units to be processed for a given Terraform command.
func (r *Runner) LogUnitDeployOrder(l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf(
		"The runner-pool runner at %s will be processed in the following order for command %s:\n",
		r.Stack.WorkingDir(),
		terraformCommand,
	)

	for _, unit := range r.queue.Entries {
		outStr += fmt.Sprintf("- Unit %s\n", unit.Component.Path())
	}

	l.Info(outStr)

	return nil
}

// JSONUnitDeployOrder returns the order of units to be processed for a given Terraform command in JSON format.
func (r *Runner) JSONUnitDeployOrder(_ string) (string, error) {
	orderedUnits := make([]string, 0, len(r.queue.Entries))
	for _, unit := range r.queue.Entries {
		orderedUnits = append(orderedUnits, unit.Component.Path())
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
		if len(unit.Component.Dependencies()) != 0 {
			for _, dep := range unit.Component.Dependencies() {
				dependentUnits[dep.Path()] = util.RemoveDuplicatesFromList(append(dependentUnits[dep.Path()], unit.Component.Path()))
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
func (r *Runner) syncTerraformCliArgs(l log.Logger, opts *types.RunnerOptions) {
	for _, comp := range r.Stack.Units() {
		unit, ok := comp.(*component.Unit)
		if !ok {
			continue
		}

		execOpts := unit.ExecutionOptions()
		if execOpts == nil {
			continue
		}

		execOpts.TerraformCliArgs = collections.MakeCopyOfList(opts.TerraformCliArgs)
		unit.SetExecutionOptions(execOpts)

		planFile := unit.PlanFile()
		if planFile != "" {
			l.Debugf("Using output file %s for unit %s", planFile, execOpts.TerragruntConfigPath)

			if execOpts.TerraformCommand == tf.CommandNamePlan {
				// for plan command add -out=<file> to the terraform cli args
				execOpts.TerraformCliArgs = append(execOpts.TerraformCliArgs, "-out="+planFile)
				unit.SetExecutionOptions(execOpts)

				continue
			}

			execOpts.TerraformCliArgs = append(execOpts.TerraformCliArgs, planFile)
			unit.SetExecutionOptions(execOpts)
		}
	}
}

// summarizePlanAllErrors summarizes all errors encountered during the plan phase across all units in the stack.
func (r *Runner) summarizePlanAllErrors(l log.Logger, errorStreams []bytes.Buffer) {
	unitIndex := 0

	for _, comp := range r.Stack.Units() {
		unit, ok := comp.(*component.Unit)
		if !ok || unitIndex >= len(errorStreams) {
			continue
		}

		errorStream := errorStreams[unitIndex]
		unitIndex++

		output := errorStream.String()

		if len(output) == 0 {
			// We get Finished buffer if runner execution completed without errors, so skip that to avoid logging too much
			continue
		}

		if strings.Contains(output, "Error running plan:") && strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
			var dependenciesMsg string

			deps := unit.Dependencies()
			if len(deps) > 0 {
				cfg := unit.Config()
				if cfg != nil && cfg.Dependencies != nil {
					dependenciesMsg = fmt.Sprintf(" contains dependencies to %v and", cfg.Dependencies.Paths)
				} else {
					dependenciesMsg = " contains dependencies and"
				}
			}

			l.Infof("%v%v refers to remote State "+
				"you may have to apply your changes in the dependencies prior running terragrunt run --all plan.\n",
				unit.Path(),
				dependenciesMsg,
			)
		}
	}
}

// All filtering logic has been moved to discovery package.
// Discovery now returns only executable units (FlagExcluded=false).
// See internal/discovery/unit_post_filter.go for the filtering implementation.

// WithOptions updates the stack with the provided options.
func (r *Runner) WithOptions(opts ...common.Option) *Runner {
	for _, opt := range opts {
		opt.Apply(r)
	}

	return r
}

// GetStack returns the stack associated with the runner.
func (r *Runner) GetStack() *component.Stack {
	return r.Stack
}

// SetParseOptions sets the ParseOptions for the stack.
func (r *Runner) SetParseOptions(parserOptions []hclparse.Option) {
	r.Stack.SetParserOptions(parserOptions)
}

// SetReport sets the report for the stack.
func (r *Runner) SetReport(report *report.Report) {
	r.Stack.SetReport(report)
}

// SetUnitFilters sets the unit filters for the runner.
// Filters are deduplicated before appending to prevent duplicate filter application.
func (r *Runner) SetUnitFilters(filters ...common.UnitFilter) {
	for _, filter := range filters {
		if !containsFilter(r.unitFilters, filter) {
			r.unitFilters = append(r.unitFilters, filter)
		}
	}
}

// GetUnitFilters returns the unit filters configured for the runner.
// This is primarily used for testing purposes.
func (r *Runner) GetUnitFilters() []common.UnitFilter {
	return r.unitFilters
}

// containsFilter checks if a filter already exists in the filters slice.
// Uses DeepEqual to compare filters by both pointer identity and value equality.
func containsFilter(filters []common.UnitFilter, target common.UnitFilter) bool {
	for _, existing := range filters {
		// DeepEqual handles both pointer equality and value equality,
		// so we don't need separate pointer comparison
		if reflect.DeepEqual(existing, target) {
			return true
		}
	}

	return false
}

// isDestroyCommand, applyPreventDestroyExclusions, and related helper functions
// have been moved to internal/discovery/unit_post_filter.go
