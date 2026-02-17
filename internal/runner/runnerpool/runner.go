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
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/tf"

	"github.com/gruntwork-io/terragrunt/internal/util"

	"github.com/gruntwork-io/terragrunt/internal/component"
	tgerrors "github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Runner implements the Stack interface for runner pool execution.
type Runner struct {
	Stack *component.Stack
	queue *queue.Queue
}

// CloneUnitOptions clones TerragruntOptions for a specific unit.
// It handles CloneWithConfigPath, per-unit DownloadDir fallback, and OriginalTerragruntConfigPath.
// Returns the cloned options and logger, or the original logger if stackOpts is nil.
func CloneUnitOptions(
	stackOpts *options.TerragruntOptions,
	unit *component.Unit,
	canonicalConfigPath string,
	stackDefaultDownloadDir string,
	l log.Logger,
) (*options.TerragruntOptions, log.Logger, error) {
	clonedLogger, clonedOpts, err := stackOpts.CloneWithConfigPath(l, canonicalConfigPath)
	if err != nil {
		return nil, nil, err
	}

	// Override logger prefix with display path (relative to discovery context) for cleaner logs
	// unless --log-show-abs-paths is set
	if !stackOpts.LogShowAbsPaths {
		clonedLogger = clonedLogger.WithField(placeholders.WorkDirKeyName, unit.DisplayPath())
	}

	// Use a per-unit default download directory when the stack is using its own default
	// (i.e., no custom download dir was provided). This mirrors unit resolver behaviour
	// so each unit caches to its own .terragrunt-cache next to the config.
	if clonedOpts.DownloadDir == "" || (stackDefaultDownloadDir != "" && clonedOpts.DownloadDir == stackDefaultDownloadDir) {
		_, unitDefaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(canonicalConfigPath)
		if err != nil {
			return nil, nil, err
		}

		clonedOpts.DownloadDir = unitDefaultDownloadDir
	}

	clonedOpts.OriginalTerragruntConfigPath = canonicalConfigPath

	return clonedOpts, clonedLogger, nil
}

// BuildUnitOpts creates per-unit opts and logger for a single unit on demand.
// It computes the canonical config path, clones options, applies source overrides,
// and transfers discovery context command/args.
func BuildUnitOpts(l log.Logger, stackOpts *options.TerragruntOptions, unit *component.Unit) (*options.TerragruntOptions, log.Logger, error) {
	var stackDefaultDownloadDir string
	if stackOpts != nil {
		_, stackDefaultDownloadDir, _ = options.DefaultWorkingAndDownloadDirs(stackOpts.TerragruntConfigPath)
	}

	// Compute config path from already-canonical unit.Path() + unit.ConfigFile()
	configPath := unit.Path()
	if !strings.HasSuffix(configPath, ".hcl") && !strings.HasSuffix(configPath, ".json") {
		fileName := config.DefaultTerragruntConfigPath
		if unit.ConfigFile() != "" {
			fileName = unit.ConfigFile()
		}

		configPath = filepath.Join(unit.Path(), fileName)
	}

	// Clone options for this unit
	unitOpts, unitLogger, err := CloneUnitOptions(stackOpts, unit, configPath, stackDefaultDownloadDir, l)
	if err != nil {
		return nil, nil, err
	}

	// If --source is provided, compute the per-unit source
	if stackOpts != nil && stackOpts.Source != "" {
		unitConfig := unit.Config()
		if unitConfig != nil {
			unitSource, sourceErr := config.GetTerragruntSourceForModule(
				stackOpts.Source,
				configPath,
				unitConfig,
			)
			if sourceErr != nil {
				return nil, nil, tgerrors.Errorf("failed to compute source for unit %s: %w", unit.DisplayPath(), sourceErr)
			}

			if unitSource != "" {
				unitOpts.Source = unitSource
			}
		}
	}

	// Transfer discovery context command and args to unit options if available
	if discoveryCtx := unit.DiscoveryContext(); discoveryCtx != nil {
		if discoveryCtx.Cmd != "" {
			unitOpts.TerraformCommand = discoveryCtx.Cmd
		}

		if len(discoveryCtx.Args) > 0 {
			terraformCliArgs := make([]string, 0, 1+len(discoveryCtx.Args))
			if discoveryCtx.Cmd != "" {
				terraformCliArgs = append(terraformCliArgs, discoveryCtx.Cmd)
			}

			terraformCliArgs = append(terraformCliArgs, discoveryCtx.Args...)
			unitOpts.TerraformCliArgs = iacargs.New(terraformCliArgs...)
		}
	}

	return unitOpts, unitLogger, nil
}

// syncUnitCliArgs applies CLI argument synchronization for a single unit.
// It merges/clones flags from stackOpts and computes and appends the plan file if needed.
func syncUnitCliArgs(l log.Logger, stackOpts *options.TerragruntOptions, unitOpts *options.TerragruntOptions, unit *component.Unit) {
	discoveryCtx := unit.DiscoveryContext()
	if discoveryCtx != nil && len(discoveryCtx.Args) > 0 {
		// Merge stack-level flags that aren't already present
		unitOpts.TerraformCliArgs.MergeFlags(stackOpts.TerraformCliArgs)
	} else {
		unitOpts.TerraformCliArgs = stackOpts.TerraformCliArgs.Clone()
	}

	planFile := unit.PlanFile(stackOpts.RootWorkingDir, stackOpts.OutputFolder, unitOpts.JSONOutputFolder, unitOpts.TerraformCommand)

	if planFile != "" {
		l.Debugf("Using output file %s for unit %s", planFile, unitOpts.TerragruntConfigPath)

		// Check if plan file already exists in args
		if unitOpts.TerraformCliArgs.HasPlanFile() {
			return
		}

		if unitOpts.TerraformCommand == tf.CommandNamePlan {
			// for plan command add -out=<file> to the terraform cli args
			unitOpts.TerraformCliArgs.AppendFlag("-out=" + planFile)

			return
		}

		unitOpts.TerraformCliArgs.AppendArgument(planFile)
	}
}

// resolveUnitsFromDiscovery converts discovered components to units with canonical paths.
// Returns the resolved units. Per-unit opts/loggers are built on demand via BuildUnitOpts.
func resolveUnitsFromDiscovery(
	_ context.Context,
	l log.Logger,
	stack *component.Stack,
	discovered component.Components,
) ([]*component.Unit, error) {
	units := make([]*component.Unit, 0, len(discovered))

	for _, c := range discovered {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		// Store config from discovery context if available
		if unit.DiscoveryContext() != nil && unit.Config() == nil {
			// Config should already be set during discovery
			l.Debugf("Unit %s has no config from discovery", unit.DisplayPath())
		}

		units = append(units, unit)
	}

	return units, nil
}

// checkLocalStateWithGitRefs checks if any unit has a Git ref in its discovery context
// but no remote state configuration, and logs a warning if so.
func checkLocalStateWithGitRefs(l log.Logger, units []*component.Unit) {
	for _, unit := range units {
		discoveryCtx := unit.DiscoveryContext()
		if discoveryCtx == nil {
			continue
		}

		if discoveryCtx.Ref == "" {
			continue
		}

		unitConfig := unit.Config()
		if unitConfig == nil {
			continue
		}

		if unitConfig.RemoteState == nil || (unitConfig.RemoteState.Config != nil && unitConfig.RemoteState.BackendName == "local") {
			l.Warnf(
				"One or more units discovered using Git-based filter expressions (e.g. [HEAD~1...HEAD]) do not have a remote_state configuration. This may result in unexpected outcomes, such as outputs for dependencies returning empty. It is strongly recommended to use remote state when working with Git-based filter expressions.",
			)

			return
		}
	}
}

// NewRunnerPoolStack creates a new stack from discovered units.
func NewRunnerPoolStack(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	discovered component.Components,
	runnerOpts ...common.Option,
) (common.StackRunner, error) {
	// Filter out Stack components - we only want Unit components
	// Stack components (terragrunt.stack.hcl files) are for stack generation, not execution
	nonStackComponents := make(component.Components, 0, len(discovered))
	for _, c := range discovered {
		if _, ok := c.(*component.Unit); ok {
			nonStackComponents = append(nonStackComponents, c)
		}
	}

	if len(nonStackComponents) == 0 {
		l.Warnf("No units discovered. Creating an empty runner.")

		stack := component.NewStack(opts.WorkingDir)

		rnr := &Runner{
			Stack: stack,
		}

		// Create an empty queue
		q, queueErr := queue.NewQueue(component.Components{})
		if queueErr != nil {
			return nil, queueErr
		}

		rnr.queue = q

		return rnr.WithOptions(runnerOpts...), nil
	}

	// Initialize stack; queue will be constructed after resolving units so we can filter excludes first.
	stack := component.NewStack(opts.WorkingDir)

	rnr := &Runner{
		Stack: stack,
	}

	// Apply options (including report) BEFORE resolving units so that
	// the report is available during unit resolution for tracking exclusions
	rnr = rnr.WithOptions(runnerOpts...)

	// Resolve units from discovery - canonicalizes paths
	units, err := resolveUnitsFromDiscovery(ctx, l, rnr.Stack, nonStackComponents)
	if err != nil {
		return nil, err
	}

	// Check for units with Git refs but no remote state configuration
	checkLocalStateWithGitRefs(l, units)
	rnr.Stack.Units = units

	if opts.TerraformCliArgs.IsDestroyCommand(opts.TerraformCommand) {
		applyPreventDestroyExclusions(l, units)
	}

	// Apply filter-allow-destroy exclusions for plan and apply commands
	if opts.TerraformCommand == tf.CommandNamePlan || opts.TerraformCommand == tf.CommandNameApply {
		applyFilterAllowDestroyExclusions(l, opts, units)
	}

	// Build queue from resolved units (which have canonical absolute paths).
	// Filter out excluded units so they are not shown in lists or scheduled.
	filtered := filterUnitsToComponents(units)

	q, queueErr := queue.NewQueue(filtered)
	if queueErr != nil {
		return nil, queueErr
	}

	// Set units map on queue to enable checking dependencies not in queue
	// (e.g., when using --queue-strict-include or --filter)
	unitsMap := make(map[string]*component.Unit, len(units))
	for _, u := range units {
		if u != nil && u.Path() != "" {
			unitsMap[u.Path()] = u
		}
	}

	q.SetUnitsMap(unitsMap)

	rnr.queue = q

	return rnr.WithOptions(runnerOpts...), nil
}

// filterUnitsToComponents converts resolved units to Components.
// Excluded units that are assumed already applied are kept in the queue
// so their dependents can run (they will be immediately marked as succeeded).
// Only truly excluded units are filtered out.
func filterUnitsToComponents(units []*component.Unit) component.Components {
	result := make(component.Components, 0, len(units))
	for _, u := range units {
		if u.Excluded() {
			// Truly excluded - skip entirely
			continue
		}

		result = append(result, u)
	}

	return result
}

// Run executes the stack according to TerragruntOptions and returns the first
// error (or a joined error) once execution is finished.
func (rnr *Runner) Run(ctx context.Context, l log.Logger, stackOpts *options.TerragruntOptions, r *report.Report) error {
	terraformCmd := stackOpts.TerraformCommand

	if stackOpts.OutputFolder != "" {
		for _, u := range rnr.Stack.Units {
			planFile := u.OutputFile(stackOpts.RootWorkingDir, stackOpts.OutputFolder)
			if err := os.MkdirAll(filepath.Dir(planFile), os.ModePerm); err != nil {
				return err
			}
		}
	}

	// Mutate stackOpts CLI args at the top level - these get cloned into per-unit opts later
	if slices.Contains(config.TerraformCommandsNeedInput, terraformCmd) {
		stackOpts.TerraformCliArgs.InsertFlag(0, "-input=false")
	}

	needsCliSync := false
	isPlan := false

	switch terraformCmd {
	case tf.CommandNameApply, tf.CommandNameDestroy:
		if stackOpts.RunAllAutoApprove {
			stackOpts.TerraformCliArgs.InsertFlag(0, "-auto-approve")
		}

		needsCliSync = true
	case tf.CommandNameShow:
		needsCliSync = true
	case tf.CommandNamePlan:
		isPlan = true
		needsCliSync = true
	}

	if slices.Contains(config.TerraformCommandsNeedInput, terraformCmd) {
		needsCliSync = true
	}

	// Pre-allocate plan error buffers keyed by unit path
	var planErrorBuffers map[string]*bytes.Buffer
	if isPlan {
		planErrorBuffers = make(map[string]*bytes.Buffer, len(rnr.Stack.Units))
		for _, u := range rnr.Stack.Units {
			planErrorBuffers[u.Path()] = &bytes.Buffer{}
		}

		defer rnr.summarizePlanAllErrors(l, planErrorBuffers)
	}

	// Emit report entries for excluded units that haven't been reported yet.
	// Units excluded by CLI flags or exclude blocks are already reported during unit resolution,
	// but we still need to report units excluded by other mechanisms (e.g., external dependencies).
	if r != nil {
		for _, u := range rnr.Stack.Units {
			if u.Excluded() {
				// Ensure path is absolute for reporting
				unitPath := u.AbsolutePath(l)

				// Pass the discovery context fields for worktree scenarios
				var ensureOpts []report.EndOption

				if discoveryCtx := u.DiscoveryContext(); discoveryCtx != nil {
					ensureOpts = append(
						ensureOpts,
						report.WithDiscoveryWorkingDir(discoveryCtx.WorkingDir),
						report.WithRef(discoveryCtx.Ref),
						report.WithCmd(discoveryCtx.Cmd),
						report.WithArgs(discoveryCtx.Args),
					)
				}

				run, err := r.EnsureRun(l, unitPath, ensureOpts...)
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

					if err := r.EndRun(
						l,
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

	task := func(ctx context.Context, u *component.Unit) error {
		// Build per-unit opts and logger on demand
		unitOpts, unitLogger, err := BuildUnitOpts(l, stackOpts, u)
		if err != nil {
			return tgerrors.Errorf("failed to build opts for unit %s: %w", u.Path(), err)
		}

		// Sync CLI args from stackOpts into unit opts
		if needsCliSync {
			syncUnitCliArgs(l, stackOpts, unitOpts, u)
		}

		// Wrap ErrWriter with plan error buffer for plan commands
		if isPlan {
			if buf := planErrorBuffers[u.Path()]; buf != nil {
				unitOpts.ErrWriter = io.MultiWriter(buf, unitOpts.ErrWriter)
			}
		}

		return telemetry.TelemeterFromContext(ctx).Collect(ctx, "runner_pool_task", map[string]any{
			"terraform_command":      unitOpts.TerraformCommand,
			"terraform_cli_args":     unitOpts.TerraformCliArgs,
			"working_dir":            unitOpts.WorkingDir,
			"terragrunt_config_path": unitOpts.TerragruntConfigPath,
		}, func(childCtx context.Context) error {
			// Wrap the writer to buffer unit-scoped output
			unitWriter := NewUnitWriter(unitOpts.Writer)
			unitOpts.Writer = unitWriter
			unitRunner := common.NewUnitRunner(u)

			// Get credentials BEFORE config parsing — sops_decrypt_file() and
			// get_aws_account_id() in locals need auth-provider credentials
			// available in opts.Env during HCL evaluation.
			// See https://github.com/gruntwork-io/terragrunt/issues/5515
			credsGetter, err := creds.ObtainCredsForParsing(childCtx, unitLogger, unitOpts)
			if err != nil {
				return err
			}

			cfg, err := config.ReadTerragruntConfig(
				childCtx,
				unitLogger,
				unitOpts,
				config.DefaultParserOptions(unitLogger, unitOpts),
			)
			if err != nil {
				return err
			}

			runCfg := cfg.ToRunConfig(unitLogger)

			err = unitRunner.Run(
				childCtx,
				unitLogger,
				unitOpts,
				r,
				runCfg,
				credsGetter,
			)

			// Flush any remaining buffered output
			if flushErr := unitWriter.Flush(); flushErr != nil && err == nil {
				err = flushErr
			}

			return err
		})
	}

	rnr.queue.FailFast = stackOpts.FailFast
	rnr.queue.IgnoreDependencyOrder = stackOpts.IgnoreDependencyOrder
	// Allow continuing the queue when dependencies fail if requested via CLI
	rnr.queue.IgnoreDependencyErrors = stackOpts.IgnoreDependencyErrors
	controller := NewController(
		rnr.queue,
		rnr.Stack.Units,
		WithRunner(task),
		WithMaxConcurrency(stackOpts.Parallelism),
	)

	err := controller.Run(ctx, l)

	// Emit report entries for early exit and failed units after controller completes
	if r != nil {
		// Build a quick lookup of queue entry status by path to avoid nested scans
		statusByPath := make(map[string]queue.Status, len(rnr.queue.Entries))
		for _, qe := range rnr.queue.Entries {
			statusByPath[qe.Component.Path()] = qe.Status
		}

		for _, entry := range rnr.queue.Entries {
			// Handle both early exit and failed units to ensure they're in the report
			if entry.Status == queue.StatusEarlyExit || entry.Status == queue.StatusFailed {
				unit := rnr.Stack.FindUnitByPath(entry.Component.Path())
				if unit == nil {
					l.Warnf("Could not find unit for entry: %s", entry.Component.Path())
					continue
				}

				// Ensure path is absolute for reporting
				unitPath := unit.AbsolutePath(l)

				// Pass the discovery context fields for worktree scenarios
				var ensureOpts []report.EndOption

				if discoveryCtx := unit.DiscoveryContext(); discoveryCtx != nil {
					ensureOpts = append(
						ensureOpts,
						report.WithDiscoveryWorkingDir(discoveryCtx.WorkingDir),
						report.WithRef(discoveryCtx.Ref),
						report.WithCmd(discoveryCtx.Cmd),
						report.WithArgs(discoveryCtx.Args),
					)
				}

				run, reportErr := r.EnsureRun(l, unitPath, ensureOpts...)
				if reportErr != nil {
					l.Errorf("Error ensuring run for unit %s: %v", unitPath, reportErr)
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

				switch entry.Status { //nolint:exhaustive
				case queue.StatusEarlyExit:
					endOpts := []report.EndOption{
						report.WithResult(report.ResultEarlyExit),
						report.WithReason(report.ReasonAncestorError),
					}
					if failedAncestor != "" {
						endOpts = append(endOpts, report.WithCauseAncestorExit(failedAncestor))
					}

					if endErr := r.EndRun(l, run.Path, endOpts...); endErr != nil {
						l.Errorf("Error ending run for early exit unit %s: %v", unitPath, endErr)
					}
				case queue.StatusFailed:
					// For failed units, check if they failed due to dependency errors
					// If so, mark them as early exit; otherwise mark as failed
					endOpts := []report.EndOption{
						report.WithResult(report.ResultFailed),
						report.WithReason(report.ReasonRunError),
					}
					if failedAncestor != "" {
						// If a dependency failed, treat this as early exit due to ancestor error
						endOpts = []report.EndOption{
							report.WithResult(report.ResultEarlyExit),
							report.WithReason(report.ReasonAncestorError),
							report.WithCauseAncestorExit(failedAncestor),
						}
					}

					if endErr := r.EndRun(l, run.Path, endOpts...); endErr != nil {
						l.Errorf("Error ending run for failed unit %s: %v", unitPath, endErr)
					}
				}
			}
		}
	}

	return err
}

// LogUnitDeployOrder logs the order of units to be processed for a given Terraform command.
func (rnr *Runner) LogUnitDeployOrder(l log.Logger, opts *options.TerragruntOptions) error {
	outStr := fmt.Sprintf(
		"Unit queue will be processed for %s in this order:\n",
		opts.TerraformCommand,
	)

	// For destroy commands, reflect the actual processing order (reverse of apply order).
	// NOTE: This is display-only. The queue scheduler dynamically handles destroy order via
	// IsUp() checks - dependents must complete before their dependencies are processed.
	entries := slices.Clone(rnr.queue.Entries)
	if opts.TerraformCliArgs.IsDestroyCommand(opts.TerraformCommand) {
		slices.Reverse(entries)
	}

	// Use absolute paths if --log-show-abs-paths is set
	showAbsPaths := opts.LogShowAbsPaths

	var outStrSb strings.Builder

	for _, unit := range entries {
		unitPath := unit.Component.DisplayPath()
		if showAbsPaths {
			unitPath = unit.Component.Path()
		}

		fmt.Fprintf(&outStrSb, "- Unit %s\n", unitPath)
	}

	outStr += outStrSb.String()

	l.Info(outStr)

	return nil
}

// JSONUnitDeployOrder returns the order of units to be processed for a given Terraform command in JSON format.
func (rnr *Runner) JSONUnitDeployOrder(opts *options.TerragruntOptions) (string, error) {
	// Use absolute paths if --log-show-abs-paths is set
	showAbsPaths := opts != nil && opts.LogShowAbsPaths

	orderedUnits := make([]string, 0, len(rnr.queue.Entries))
	for _, unit := range rnr.queue.Entries {
		unitPath := unit.Component.DisplayPath()
		if showAbsPaths {
			unitPath = unit.Component.Path()
		}

		orderedUnits = append(orderedUnits, unitPath)
	}

	j, err := json.MarshalIndent(orderedUnits, "", "  ")
	if err != nil {
		return "", err
	}

	return string(j), nil
}

// ListStackDependentUnits returns a map of units and their dependent units in the stack.
func (rnr *Runner) ListStackDependentUnits() map[string][]string {
	dependentUnits := make(map[string][]string)

	for _, unit := range rnr.queue.Entries {
		if len(unit.Component.Dependencies()) != 0 {
			for _, dep := range unit.Component.Dependencies() {
				dependentUnits[dep.Path()] = util.RemoveDuplicates(append(dependentUnits[dep.Path()], unit.Component.Path()))
			}
		}
	}

	for {
		noUpdates := true

		for unit, dependents := range dependentUnits {
			for _, dependent := range dependents {
				initialSize := len(dependentUnits[unit])
				list := util.RemoveDuplicates(append(dependentUnits[unit], dependentUnits[dependent]...))
				list = slices.DeleteFunc(list, func(path string) bool { return path == unit })
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

// summarizePlanAllErrors summarizes all errors encountered during the plan phase across all units in the stack.
func (rnr *Runner) summarizePlanAllErrors(l log.Logger, errorStreams map[string]*bytes.Buffer) {
	for _, unit := range rnr.Stack.Units {
		buf := errorStreams[unit.Path()]
		if buf == nil {
			continue
		}

		output := buf.String()

		if len(output) == 0 {
			// We get Finished buffer if runner execution completed without errors, so skip that to avoid logging too much
			continue
		}

		if strings.Contains(output, "Error running plan:") && strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
			var dependenciesMsg string

			if len(unit.Dependencies()) > 0 {
				cfg := unit.Config()
				if cfg != nil && cfg.Dependencies != nil && len(cfg.Dependencies.Paths) > 0 {
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

// FilterDiscoveredUnits removes configs for units flagged as excluded and prunes dependencies
// that point to excluded units. This keeps the execution queue and any user-facing listings
// free from units not intended to run.
//
// Inputs:
//   - discovered: raw discovery results (paths and dependency edges)
//   - units: resolved units (slice), where exclude rules have already been applied
//
// Behavior:
//   - A config is included only if there's a corresponding unit and it is not excluded.
//   - For each included config, its Dependencies list is filtered to only include included configs.
//   - The function returns a new slice with shallow-copied entries so the original discovery
//     results remain unchanged.
func FilterDiscoveredUnits(discovered component.Components, units []*component.Unit) component.Components {
	// Build allowlist from non-excluded unit paths (already canonical from resolveUnitsFromDiscovery)
	allowed := make(map[string]struct{}, len(units))
	for _, u := range units {
		if !u.Excluded() {
			allowed[u.Path()] = struct{}{}
		}
	}

	// First pass: keep only allowed configs and prune their dependencies to allowed ones
	// NOTE: Unit paths should already be canonical after resolveUnitsFromDiscovery modified them
	filtered := make(component.Components, 0, len(discovered))
	present := make(map[string]*component.Unit, len(discovered))

	for _, c := range discovered {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		// Path should already be canonical from resolveUnitsFromDiscovery
		unitPath := unit.Path()

		if _, ok := allowed[unitPath]; !ok {
			// Drop configs that map to excluded/missing units
			continue
		}

		// Create new unit with the path (already canonical)
		copyCfg := component.NewUnit(unitPath)
		copyCfg.SetDiscoveryContext(unit.DiscoveryContext())
		copyCfg.SetReading(unit.Reading()...)

		if unit.External() {
			copyCfg.SetExternal()
		}

		if len(unit.Dependencies()) > 0 {
			for _, dep := range unit.Dependencies() {
				// Dependency paths should also be canonical
				depPath := dep.Path()
				if _, ok := allowed[depPath]; ok {
					// Create dependency with the path
					depCfg := component.NewUnit(depPath)
					copyCfg.AddDependency(depCfg)
				}
			}
		}

		filtered = append(filtered, copyCfg)
		present[copyCfg.Path()] = copyCfg
	}

	// Ensure every allowed unit exists in the filtered set, even if discovery didn't include it (or it was pruned)
	for _, u := range units {
		if u.Excluded() {
			continue
		}

		if _, ok := present[u.Path()]; ok {
			continue
		}

		// Create a minimal discovered config for the missing unit
		copyCfg := component.NewUnit(u.Path())

		filtered = append(filtered, copyCfg)
		present[u.Path()] = copyCfg
	}

	// Augment dependencies from resolved units to ensure DAG edges are complete
	for _, u := range units {
		if u.Excluded() {
			continue
		}

		cfg := present[u.Path()]
		if cfg == nil {
			continue
		}

		// Build a set of existing dependency paths on cfg to avoid duplicates
		existing := make(map[string]struct{}, len(cfg.Dependencies()))
		for _, dep := range cfg.Dependencies() {
			existing[dep.Path()] = struct{}{}
		}

		// Add any missing allowed dependencies from the resolved unit graph
		for _, dep := range u.Dependencies() {
			depUnit, okDep := dep.(*component.Unit)
			if !okDep || depUnit == nil {
				continue
			}

			if _, allowedOK := allowed[depUnit.Path()]; !allowedOK {
				continue
			}

			if _, existsOK := existing[depUnit.Path()]; existsOK {
				continue
			}

			// Ensure the dependency config exists in the filtered set
			depCfg, presentOK := present[depUnit.Path()]
			if !presentOK {
				depCfg = component.NewUnit(depUnit.Path())
				filtered = append(filtered, depCfg)
				present[depUnit.Path()] = depCfg
			}

			cfg.AddDependency(depCfg)
		}
	}

	return filtered
}

// WithOptions updates the stack with the provided options.
func (rnr *Runner) WithOptions(opts ...common.Option) *Runner {
	for _, opt := range opts {
		opt.Apply(rnr)
	}

	return rnr
}

// GetStack returns the stack associated with the runner.
func (rnr *Runner) GetStack() *component.Stack {
	return rnr.Stack
}

// applyPreventDestroyExclusions excludes units with prevent_destroy=true and their dependencies
// from being destroyed. This prevents accidental destruction of protected infrastructure.
func applyPreventDestroyExclusions(l log.Logger, units []*component.Unit) {
	// First pass: identify units with prevent_destroy=true
	protectedUnits := make(map[string]bool)

	for _, unit := range units {
		cfg := unit.Config()
		if cfg != nil && cfg.PreventDestroy != nil && *cfg.PreventDestroy {
			protectedUnits[unit.Path()] = true
			unit.SetExcluded(true)

			l.Debugf("Unit %s is protected by prevent_destroy flag", unit.Path())
		}
	}

	if len(protectedUnits) == 0 {
		return
	}

	// Second pass: find all dependencies of protected units
	// We need to prevent destruction of any unit that a protected unit depends on
	dependencyPaths := make(map[string]bool)

	for _, unit := range units {
		if protectedUnits[unit.Path()] {
			collectDependencies(unit, dependencyPaths)
		}
	}

	// Third pass: mark dependencies as excluded
	for _, unit := range units {
		if dependencyPaths[unit.Path()] && !protectedUnits[unit.Path()] {
			unit.SetExcluded(true)

			l.Debugf("Unit %s is excluded because it's a dependency of a protected unit", unit.Path())
		}
	}
}

// maxDependencyTraversalDepth bounds the depth of dependency traversal to prevent excessive recursion.
const maxDependencyTraversalDepth = 256

// applyFilterAllowDestroyExclusions excludes units with destroy runs from Git-based filters
// when the --filter-allow-destroy flag is not set. This prevents accidental destruction
// of infrastructure when using Git-based filters.
func applyFilterAllowDestroyExclusions(l log.Logger, opts *options.TerragruntOptions, units []*component.Unit) {
	if opts.FilterAllowDestroy {
		return
	}

	for _, unit := range units {
		discoveryCtx := unit.DiscoveryContext()
		if discoveryCtx == nil {
			continue
		}

		if discoveryCtx.Ref != "" && iacargs.New(discoveryCtx.Args...).IsDestroyCommand(discoveryCtx.Cmd) {
			unit.SetExcluded(true)

			l.Warnf("The `%s` unit was removed in the `%s` Git reference, but the `--filter-allow-destroy` flag was not used. The unit will be excluded during applies unless --filter-allow-destroy is used.", unit.DisplayPath(), discoveryCtx.Ref)
		}
	}
}

// collectDependencies collects dependency paths for a unit with a bounded recursion depth.
func collectDependencies(unit *component.Unit, paths map[string]bool) {
	collectDependenciesBounded(unit, paths, 0)
}

// collectDependenciesBounded recursively collects all dependency paths for a unit up to maxDependencyTraversalDepth.
func collectDependenciesBounded(unit *component.Unit, paths map[string]bool, depth int) {
	if depth >= maxDependencyTraversalDepth {
		return
	}

	for _, dep := range unit.Dependencies() {
		depUnit, ok := dep.(*component.Unit)
		if !ok {
			continue
		}

		if !paths[depUnit.Path()] {
			paths[depUnit.Path()] = true
			collectDependenciesBounded(depUnit, paths, depth+1)
		}
	}
}
