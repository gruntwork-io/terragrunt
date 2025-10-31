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
	"reflect"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/gruntwork-io/terragrunt/tf"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// Runner implements the Stack interface for runner pool execution.
type Runner struct {
	Stack       *common.Stack
	queue       *queue.Queue
	unitFilters []common.UnitFilter
}

// SetQueue replaces the runner's queue with a new one.
// This is useful for filtering the queue after initial discovery.
func (r *Runner) SetQueue(q *queue.Queue) {
	r.queue = q
}

// NewRunnerPoolStack creates a new stack from discovered units.
func NewRunnerPoolStack(
	ctx context.Context,
	l log.Logger,
	terragruntOptions *options.TerragruntOptions,
	discovered component.Components,
	opts ...common.Option,
) (common.StackRunner, error) {
	// Filter out Stack components - we only want Unit components
	// Stack components (terragrunt.stack.hcl files) are for stack generation, not execution
	nonStackComponents := make(component.Components, 0, len(discovered))
	for _, c := range discovered {
		if c.Kind() != component.StackKind {
			nonStackComponents = append(nonStackComponents, c)
		}
	}

	if len(nonStackComponents) == 0 {
		l.Warnf("No units discovered. Creating an empty runner.")

		stack := common.Stack{
			TerragruntOptions: terragruntOptions,
			ParserOptions:     config.DefaultParserOptions(l, terragruntOptions),
		}

		runner := &Runner{
			Stack: &stack,
		}

		// Create an empty queue
		q, queueErr := queue.NewQueue(component.Components{})
		if queueErr != nil {
			return nil, queueErr
		}

		runner.queue = q

		return runner.WithOptions(opts...), nil
	}

	// Initialize stack; queue will be constructed after resolving units so we can filter excludes first.
	stack := common.Stack{
		TerragruntOptions: terragruntOptions,
		ParserOptions:     config.DefaultParserOptions(l, terragruntOptions),
	}

	runner := &Runner{
		Stack: &stack,
	}

	// Apply options (including report) BEFORE resolving units so that
	// the report is available during unit resolution for tracking exclusions
	runner = runner.WithOptions(opts...)

	// Resolve units (this applies to include/exclude logic and sets FlagExcluded accordingly).
	unitResolver, err := common.NewUnitResolver(ctx, runner.Stack)
	if err != nil {
		return nil, err
	}

	// Add unit filters to the resolver
	if len(runner.unitFilters) > 0 {
		unitResolver = unitResolver.WithFilters(runner.unitFilters...)
	}

	// Use discovery-based resolution (no legacy fallback needed since discovery parses all required blocks)
	// Use nonStackComponents which has Stack components filtered out
	unitsMap, err := unitResolver.ResolveFromDiscovery(ctx, l, nonStackComponents)
	if err != nil {
		return nil, err
	}

	runner.Stack.Units = unitsMap

	if isDestroyCommand(terragruntOptions) {
		applyPreventDestroyExclusions(l, unitsMap)
	}

	// Build queue from discovered configs, excluding units flagged as excluded and pruning excluded dependencies.
	// This ensures excluded units are not shown in lists or scheduled at all.
	filtered := FilterDiscoveredUnits(discovered, unitsMap)

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

	var planErrorBuffers []bytes.Buffer

	switch terraformCmd {
	case tf.CommandNameApply, tf.CommandNameDestroy:
		r.handleApplyDestroy(l, opts)
	case tf.CommandNameShow:
		r.handleShow(l, opts)
	case tf.CommandNamePlan:
		planErrorBuffers = r.handlePlan()
		defer r.summarizePlanAllErrors(l, planErrorBuffers)
	}

	// Emit report entries for excluded units that haven't been reported yet.
	// Units excluded by CLI flags or exclude blocks are already reported during unit resolution,
	// but we still need to report units excluded by other mechanisms (e.g., external dependencies).
	if r.Stack.Report != nil {
		for _, u := range r.Stack.Units {
			if u.FlagExcluded {
				// Ensure path is absolute for reporting
				unitPath := u.Path
				if !filepath.IsAbs(unitPath) {
					var err error

					unitPath, err = filepath.Abs(unitPath)
					if err != nil {
						l.Errorf("Error getting absolute path for unit %s: %v", u.Path, err)
						continue
					}
				}

				run, err := r.Stack.Report.EnsureRun(unitPath)
				if err != nil {
					l.Errorf("Error ensuring run for unit %s: %v", unitPath, err)
					continue
				}

				// Only report exclusion if it hasn't been reported yet
				// Units excluded by --queue-exclude-dir or exclude blocks are already reported
				// during unit resolution with the correct reason
				if run.Result == "" {
					// Determine the reason for exclusion
					// External dependencies that are assumed already applied are excluded with --queue-exclude-external
					reason := report.ReasonExcludeBlock
					if u.AssumeAlreadyApplied {
						reason = report.ReasonExcludeExternal
					}

					if err := r.Stack.Report.EndRun(
						run.Path,
						report.WithResult(report.ResultExcluded),
						report.WithReason(reason),
					); err != nil {
						l.Errorf("Error ending run for unit %s: %v", unitPath, err)
					}
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
	// Allow continuing the queue when dependencies fail if requested via CLI
	r.queue.IgnoreDependencyErrors = opts.IgnoreDependencyErrors
	controller := NewController(
		r.queue,
		r.Stack.Units,
		WithRunner(task),
		WithMaxConcurrency(opts.Parallelism),
	)

	err := controller.Run(ctx, l)

	// Emit report entries for early exit units after controller completes
	if r.Stack.Report != nil {
		// Build a quick lookup of queue entry status by path to avoid nested scans
		statusByPath := make(map[string]queue.Status, len(r.queue.Entries))
		for _, qe := range r.queue.Entries {
			statusByPath[qe.Component.Path()] = qe.Status
		}

		for _, entry := range r.queue.Entries {
			if entry.Status == queue.StatusEarlyExit {
				unit := r.getUnitByPath(entry.Component.Path())
				if unit == nil {
					l.Warnf("Could not find unit for early exit entry: %s", entry.Component.Path())
					continue
				}

				// Ensure path is absolute for reporting
				unitPath := unit.Path
				if !filepath.IsAbs(unitPath) {
					var absErr error

					unitPath, absErr = filepath.Abs(unitPath)
					if absErr != nil {
						l.Errorf("Error getting absolute path for unit %s: %v", unit.Path, absErr)
						continue
					}
				}

				run, reportErr := r.Stack.Report.EnsureRun(unitPath)
				if reportErr != nil {
					l.Errorf("Error ensuring run for early exit unit %s: %v", unitPath, reportErr)
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

				if endErr := r.Stack.Report.EndRun(run.Path, endOpts...); endErr != nil {
					l.Errorf("Error ending run for early exit unit %s: %v", unitPath, endErr)
				}
			}
		}
	}

	return err
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
// Returns error buffers for each unit to capture stderr output for later analysis.
func (r *Runner) handlePlan() []bytes.Buffer {
	planErrorBuffers := make([]bytes.Buffer, len(r.Stack.Units))
	for i, u := range r.Stack.Units {
		u.TerragruntOptions.ErrWriter = io.MultiWriter(&planErrorBuffers[i], u.TerragruntOptions.ErrWriter)
	}

	return planErrorBuffers
}

// getUnitByPath returns the unit with the given path, or nil if not found.
func (r *Runner) getUnitByPath(path string) *common.Unit {
	for _, u := range r.Stack.Units {
		if u.Path == path {
			return u
		}
	}

	return nil
}

// LogUnitDeployOrder logs the order of units to be processed for a given Terraform command.
func (r *Runner) LogUnitDeployOrder(l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf(
		"The runner-pool runner at %s will be processed in the following order for command %s:\n",
		r.Stack.TerragruntOptions.WorkingDir,
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

// FilterDiscoveredUnits removes configs for units flagged as excluded and prunes dependencies
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
func FilterDiscoveredUnits(discovered component.Components, units common.Units) component.Components {
	// Build allowlist from non-excluded unit paths
	allowed := make(map[string]struct{}, len(units))
	for _, u := range units {
		if !u.FlagExcluded {
			allowed[u.Path] = struct{}{}
		}
	}

	// First pass: keep only allowed configs and prune their dependencies to allowed ones
	filtered := make(component.Components, 0, len(discovered))
	present := make(map[string]*component.Unit, len(discovered))

	for _, c := range discovered {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		if _, ok := allowed[unit.Path()]; !ok {
			// Drop configs that map to excluded/missing units
			continue
		}

		copyCfg := component.NewUnit(unit.Path())
		copyCfg.SetDiscoveryContext(unit.DiscoveryContext())
		copyCfg.SetReading(unit.Reading()...)

		if unit.External() {
			copyCfg.SetExternal()
		}

		if len(unit.Dependencies()) > 0 {
			for _, dep := range unit.Dependencies() {
				if _, ok := allowed[dep.Path()]; ok {
					copyCfg.AddDependency(dep)
				}
			}
		}

		filtered = append(filtered, copyCfg)
		present[copyCfg.Path()] = copyCfg
	}

	// Ensure every allowed unit exists in the filtered set, even if discovery didn't include it (or it was pruned)
	for _, u := range units {
		if u.FlagExcluded {
			continue
		}

		if _, ok := present[u.Path]; ok {
			continue
		}

		// Create a minimal discovered config for the missing unit
		copyCfg := component.NewUnit(u.Path)

		filtered = append(filtered, copyCfg)
		present[u.Path] = copyCfg
	}

	// Augment dependencies from resolved units to ensure DAG edges are complete
	for _, u := range units {
		if u.FlagExcluded {
			continue
		}

		cfg := present[u.Path]
		if cfg == nil {
			continue
		}

		// Build a set of existing dependency paths on cfg to avoid duplicates
		existing := make(map[string]struct{}, len(cfg.Dependencies()))
		for _, dep := range cfg.Dependencies() {
			existing[dep.Path()] = struct{}{}
		}

		// Add any missing allowed dependencies from the resolved unit graph
		for _, depUnit := range u.Dependencies {
			if depUnit == nil {
				continue
			}

			if _, ok := allowed[depUnit.Path]; !ok {
				continue
			}

			if _, ok := existing[depUnit.Path]; ok {
				continue
			}

			// Ensure the dependency config exists in the filtered set
			depCfg, ok := present[depUnit.Path]
			if !ok {
				depCfg = component.NewUnit(depUnit.Path)
				filtered = append(filtered, depCfg)
				present[depUnit.Path] = depCfg
			}

			cfg.AddDependency(depCfg)
		}
	}

	return filtered
}

// WithOptions updates the stack with the provided options.
func (r *Runner) WithOptions(opts ...common.Option) *Runner {
	for _, opt := range opts {
		opt.Apply(r)
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
// Uses reflection to compare filter pointers for deduplication.
func containsFilter(filters []common.UnitFilter, target common.UnitFilter) bool {
	targetValue := reflect.ValueOf(target)
	targetPtr := targetValue.Pointer()

	for _, existing := range filters {
		existingValue := reflect.ValueOf(existing)

		// Compare by pointer for function-type filters
		if existingValue.Pointer() == targetPtr {
			return true
		}

		// For struct-based filters, compare by value equality
		if reflect.DeepEqual(existing, target) {
			return true
		}
	}

	return false
}

// isDestroyCommand checks if the current command is a destroy operation
func isDestroyCommand(opts *options.TerragruntOptions) bool {
	return opts.TerraformCommand == tf.CommandNameDestroy ||
		util.ListContainsElement(opts.TerraformCliArgs, "-"+tf.CommandNameDestroy)
}

// applyPreventDestroyExclusions excludes units with prevent_destroy=true and their dependencies
// from being destroyed. This prevents accidental destruction of protected infrastructure.
func applyPreventDestroyExclusions(l log.Logger, units common.Units) {
	// First pass: identify units with prevent_destroy=true
	protectedUnits := make(map[string]bool)

	for _, unit := range units {
		if unit.Config.PreventDestroy != nil && *unit.Config.PreventDestroy {
			protectedUnits[unit.Path] = true
			unit.FlagExcluded = true
			l.Debugf("Unit %s is protected by prevent_destroy flag", unit.Path)
		}
	}

	if len(protectedUnits) == 0 {
		return
	}

	// Second pass: find all dependencies of protected units
	// We need to prevent destruction of any unit that a protected unit depends on
	dependencyPaths := make(map[string]bool)

	for _, unit := range units {
		if protectedUnits[unit.Path] {
			collectDependencies(unit, dependencyPaths)
		}
	}

	// Third pass: mark dependencies as excluded
	for _, unit := range units {
		if dependencyPaths[unit.Path] && !protectedUnits[unit.Path] {
			unit.FlagExcluded = true
			l.Debugf("Unit %s is excluded because it's a dependency of a protected unit", unit.Path)
		}
	}
}

// maxDependencyTraversalDepth bounds the depth of dependency traversal to prevent excessive recursion.
const maxDependencyTraversalDepth = 256

// collectDependencies collects dependency paths for a unit with a bounded recursion depth.
func collectDependencies(unit *common.Unit, paths map[string]bool) {
	collectDependenciesBounded(unit, paths, 0)
}

// collectDependenciesBounded recursively collects all dependency paths for a unit up to maxDependencyTraversalDepth.
func collectDependenciesBounded(unit *common.Unit, paths map[string]bool, depth int) {
	if depth >= maxDependencyTraversalDepth {
		return
	}

	for _, dep := range unit.Dependencies {
		if !paths[dep.Path] {
			paths[dep.Path] = true
			collectDependenciesBounded(dep, paths, depth+1)
		}
	}
}
