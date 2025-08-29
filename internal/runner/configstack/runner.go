// Package configstack provides the implementation of the Runner, which run units as groups.
package configstack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/config/hclparse"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/tf"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// Runner implements the Stack interface and represents a stack of Terraform units (i.e. folders with Terraform templates) that you can "spin up" or "spin down" in a single command
// (formerly Stack)
type Runner struct {
	Stack *common.Stack
}

// NewRunner creates a new Runner.
func NewRunner(l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...common.Option) *Runner {
	runner := &Runner{
		Stack: &common.Stack{
			TerragruntOptions: terragruntOptions,
			ParserOptions:     config.DefaultParserOptions(l, terragruntOptions),
		},
	}

	return runner.WithOptions(opts...)
}

// WithOptions updates the stack with the provided options.
func (runner *Runner) WithOptions(opts ...common.Option) *Runner {
	for _, opt := range opts {
		opt(runner)
	}

	return runner
}

// GetStack returns the queue used by this runner.
func (runner *Runner) GetStack() *common.Stack {
	return runner.Stack
}

// LogUnitDeployOrder will log the units that will be deployed by this operation, in the order that the operations
// happen. For plan and apply, the order will be bottom to top (dependencies first), while for destroy the order will be
// in reverse.
func (runner *Runner) LogUnitDeployOrder(l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf("The runner at %s will be processed in the following order for command %s:\n", runner.Stack.TerragruntOptions.WorkingDir, terraformCommand)

	runGraph, err := runner.GetUnitRunGraph(terraformCommand)
	if err != nil {
		return err
	}

	for i, group := range runGraph {
		outStr += fmt.Sprintf("Group %d\n", i+1)
		for _, unit := range group {
			outStr += fmt.Sprintf("- Unit %s\n", unit.Path)
		}

		outStr += "\n"
	}

	l.Info(outStr)

	return nil
}

// JSONUnitDeployOrder will return the units that will be deployed by a plan/apply operation, in the order
// that the operations happen.
func (runner *Runner) JSONUnitDeployOrder(terraformCommand string) (string, error) {
	runGraph, err := runner.GetUnitRunGraph(terraformCommand)
	if err != nil {
		return "", errors.New(err)
	}

	// Convert the unit paths to a string array for JSON marshalling
	// The index should be the group number, and the value should be an array of unit paths
	jsonGraph := make(map[string][]string)

	for i, group := range runGraph {
		groupNum := "Group " + strconv.Itoa(i+1)
		jsonGraph[groupNum] = make([]string, len(group))

		for j, unit := range group {
			jsonGraph[groupNum][j] = unit.Path
		}
	}

	j, err := json.MarshalIndent(jsonGraph, "", "  ")
	if err != nil {
		return "", errors.New(err)
	}

	return string(j), nil
}

// Run execute configstack.
func (runner *Runner) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	stackCmd := opts.TerraformCommand

	// prepare folder for output hierarchy if output folder is set
	if opts.OutputFolder != "" {
		for _, unit := range runner.Stack.Units {
			planFile := unit.OutputFile(l, opts)

			planDir := filepath.Dir(planFile)
			if err := os.MkdirAll(planDir, os.ModePerm); err != nil {
				return err
			}
		}
	}

	// For any command that needs input, run in non-interactive mode to avoid cominglint stdin across multiple
	// concurrent runs.
	if util.ListContainsElement(config.TerraformCommandsNeedInput, stackCmd) {
		// to support potential positional args in the args list, we append the input=false arg after the first element,
		// which is the target command.
		opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-input=false", 1)
		runner.syncTerraformCliArgs(l, opts)
	}

	// For apply and destroy, run with auto-approve (unless explicitly disabled) due to the co-mingling of the prompts.
	// This is not ideal, but until we have a better way of handling interactivity with run --all, we take the evil of
	// having a global prompt (managed in cli/cli_app.go) be the gate keeper.
	switch stackCmd {
	case tf.CommandNameApply, tf.CommandNameDestroy:
		// to support potential positional args in the args list, we append the input=false arg after the first element,
		// which is the target command.
		if opts.RunAllAutoApprove {
			opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-auto-approve", 1)
		}

		runner.syncTerraformCliArgs(l, opts)
	case tf.CommandNameShow:
		runner.syncTerraformCliArgs(l, opts)
	case tf.CommandNamePlan:
		// We capture the out stream for each unit
		errorStreams := make([]bytes.Buffer, len(runner.Stack.Units))

		for n, unit := range runner.Stack.Units {
			unit.TerragruntOptions.ErrWriter = io.MultiWriter(&errorStreams[n], unit.TerragruntOptions.ErrWriter)
		}

		defer runner.summarizePlanAllErrors(l, errorStreams)
	}

	switch {
	case opts.IgnoreDependencyOrder:
		return runner.RunUnitsIgnoreOrder(ctx, opts)
	case stackCmd == tf.CommandNameDestroy:
		return runner.RunUnitsReverseOrder(ctx, opts)
	default:
		return runner.RunUnits(ctx, opts)
	}
}

// summarizePlanAllErrors inspects the error streams collected from running 'terraform plan' on multiple units.
// It logs a specific message if a plan failed due to unresolved remote state references, which typically occurs
// when a dependency's state has not yet been applied. For each unit, if the error output contains an error
// related to remote state, it logs an informational message suggesting that the user may need to apply changes
// in the dependencies before running 'terragrunt run --all plan'.
func (runner *Runner) summarizePlanAllErrors(l log.Logger, errorStreams []bytes.Buffer) {
	for i, errorStream := range errorStreams {
		output := errorStream.String()

		if len(output) == 0 {
			// We get empty buffer if runner execution completed without errors, so skip that to avoid logging too much
			continue
		}

		unit := runner.Stack.Units[i]

		if strings.Contains(output, "Error running plan:") && strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
			var dependenciesMsg string
			if len(unit.Dependencies) > 0 {
				dependenciesMsg = fmt.Sprintf(" contains dependencies to %v and", unit.Config.Dependencies.Paths)
			}

			l.Infof("%v%v refers to remote state "+
				"you may have to apply your changes in the dependencies prior running terragrunt run --all plan.\n",
				unit.Path,
				dependenciesMsg,
			)
		}
	}
}

// syncTerraformCliArgs synchronizes the Terraform CLI arguments for each unit in the stack to match
// the provided TerragruntOptions. It also ensures that the appropriate plan or output file arguments
// are set for each unit, depending on the Terraform command being executed. This guarantees that all
// units use consistent CLI arguments and output file locations during execution.
func (runner *Runner) syncTerraformCliArgs(l log.Logger, opts *options.TerragruntOptions) {
	for _, unit := range runner.Stack.Units {
		unit.TerragruntOptions.TerraformCliArgs = collections.MakeCopyOfList(opts.TerraformCliArgs)

		planFile := unit.PlanFile(l, opts)

		if planFile != "" {
			l.Debugf("Using output file %s for unit %s", planFile, unit.TerragruntOptions.TerragruntConfigPath)

			if unit.TerragruntOptions.TerraformCommand == tf.CommandNamePlan {
				// for plan command add -out=<file> to the terraform cli args
				unit.TerragruntOptions.TerraformCliArgs = util.StringListInsert(unit.TerragruntOptions.TerraformCliArgs, "-out="+planFile, len(unit.TerragruntOptions.TerraformCliArgs))
			} else {
				unit.TerragruntOptions.TerraformCliArgs = util.StringListInsert(unit.TerragruntOptions.TerraformCliArgs, planFile, len(unit.TerragruntOptions.TerraformCliArgs))
			}
		}
	}
}

func (runner *Runner) toRunningUnits(terraformCommand string) (RunningUnits, error) {
	switch terraformCommand {
	case tf.CommandNameDestroy:
		return ToRunningUnits(runner.Stack.Units, ReverseOrder, runner.Stack.Report, runner.Stack.TerragruntOptions)
	default:
		return ToRunningUnits(runner.Stack.Units, NormalOrder, runner.Stack.Report, runner.Stack.TerragruntOptions)
	}
}

// GetUnitRunGraph converts the unit list to a graph that shows the order in which the units will be
// applied/destroyed. The return structure is a list of lists, where the nested list represents units that can be
// deployed concurrently, and the outer list indicates the order. This will only include those units that do NOT have
// the exclude flag set.
func (runner *Runner) GetUnitRunGraph(terraformCommand string) ([]common.Units, error) {
	unitRunGraph, err := runner.toRunningUnits(terraformCommand)
	if err != nil {
		return nil, err
	}

	// Set maxDepth for the graph so that we don't get stuck in an infinite loop.
	const maxDepth = 1000
	groups := unitRunGraph.toTerraformUnitGroups(maxDepth)

	return groups, nil
}

// createStackForTerragruntConfigPaths discovers all Terraform units from the given Terragrunt config file paths,
// assembles them into a stack, and checks for dependency cycles. Updates the Runner's stack with the resolved units.
// Returns an error if discovery or validation fails.
func (runner *Runner) createStackForTerragruntConfigPaths(ctx context.Context, l log.Logger, terragruntConfigPaths []string) error {
	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "create_stack_for_terragrunt_config_paths", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(ctx context.Context) error {
		if len(terragruntConfigPaths) == 0 {
			return errors.New(common.ErrNoUnitsFound)
		}

		units, err := runner.ResolveTerraformModules(ctx, l, terragruntConfigPaths)
		if err != nil {
			return errors.New(err)
		}

		runner.Stack.Units = units

		return nil
	})
	if err != nil {
		return errors.New(err)
	}

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "check_for_cycles", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		if err := runner.Stack.Units.CheckForCycles(); err != nil {
			return errors.New(err)
		}

		return nil
	})

	if err != nil {
		return errors.New(err)
	}

	return nil
}

// ResolveTerraformModules goes through each of the given Terragrunt configuration files
// and resolve the unit that configuration file represents into a Unit struct.
// Return the list of these Unit structs.
func (runner *Runner) ResolveTerraformModules(ctx context.Context, l log.Logger, terragruntConfigPaths []string) (common.Units, error) {
	unitResolver, err := common.NewUnitResolver(ctx, runner.Stack)
	if err != nil {
		return nil, err
	}

	return unitResolver.ResolveTerraformModules(ctx, l, terragruntConfigPaths)
}

// ListStackDependentUnits - build a map with each unit and its dependent units
func (runner *Runner) ListStackDependentUnits() map[string][]string {
	// build map of dependent units
	// unit path -> list of dependent units
	var dependentUnits = make(map[string][]string)

	// build initial mapping of dependent units
	for _, unit := range runner.Stack.Units {
		if len(unit.Dependencies) != 0 {
			for _, dep := range unit.Dependencies {
				dependentUnits[dep.Path] = util.RemoveDuplicatesFromList(append(dependentUnits[dep.Path], unit.Path))
			}
		}
	}

	// Floyd–Warshall inspired approach to find dependent units
	// merge map slices by key until no more updates are possible

	// Example:
	// Initial setup:
	// dependentUnits["unit1"] = ["unit2", "unit3"]
	// dependentUnits["unit2"] = ["unit3"]
	// dependentUnits["unit3"] = ["unit4"]
	// dependentUnits["unit4"] = ["unit5"]

	// After first iteration: (unit1 += unit4, unit2 += unit4, unit3 += unit5)
	// dependentUnits["unit1"] = ["unit2", "unit3", "unit4"]
	// dependentUnits["unit2"] = ["unit3", "unit4"]
	// dependentUnits["unit3"] = ["unit4", "unit5"]
	// dependentUnits["unit4"] = ["unit5"]

	// After second iteration: (unit1 += unit5, unit2 += unit5)
	// dependentUnits["unit1"] = ["unit2", "unit3", "unit4", "unit5"]
	// dependentUnits["unit2"] = ["unit3", "unit4", "unit5"]
	// dependentUnits["unit3"] = ["unit4", "unit5"]
	// dependentUnits["unit4"] = ["unit5"]

	// Done, no more updates and in map we have all dependent units for each unit.

	for {
		noUpdates := true

		for unit, dependents := range dependentUnits {
			for _, dependent := range dependents {
				initialSize := len(dependentUnits[unit])
				// merge without duplicates
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

// Units returns the Terraform units in the stack.
func (runner *Runner) Units() common.Units {
	return runner.Stack.Units
}

// RunUnits runs the given map of unit path to runningUnit. To "run" a unit, execute the runTerragrunt command in its
// TerragruntOptions object. The units will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func (runner *Runner) RunUnits(ctx context.Context, opts *options.TerragruntOptions) error {
	runningUnits, err := ToRunningUnits(runner.Stack.Units, NormalOrder, runner.Stack.Report, opts)
	if err != nil {
		return err
	}

	return runningUnits.runUnits(ctx, opts, runner.Stack.Report, opts.Parallelism)
}

// RunUnitsReverseOrder runs the given map of unit path to runningUnit. To "run" a unit, execute the runTerragrunt command in its
// TerragruntOptions object. The units will be executed in the reverse order of their inter-dependencies, using
// as much concurrency as possible.
func (runner *Runner) RunUnitsReverseOrder(ctx context.Context, opts *options.TerragruntOptions) error {
	runningUnits, err := ToRunningUnits(runner.Stack.Units, ReverseOrder, runner.Stack.Report, opts)
	if err != nil {
		return err
	}

	return runningUnits.runUnits(ctx, opts, runner.Stack.Report, opts.Parallelism)
}

// RunUnitsIgnoreOrder runs the given map of unit path to runningUnit. To "run" a unit, execute the runTerragrunt command in its
// TerragruntOptions object. The units will be executed without caring for inter-dependencies.
func (runner *Runner) RunUnitsIgnoreOrder(ctx context.Context, opts *options.TerragruntOptions) error {
	runningUnits, err := ToRunningUnits(runner.Stack.Units, IgnoreOrder, runner.Stack.Report, opts)
	if err != nil {
		return err
	}

	return runningUnits.runUnits(ctx, opts, runner.Stack.Report, opts.Parallelism)
}

// ToRunningUnits converts the list of units to a map from unit path to a runningUnit struct. This struct contains information
// about executing the unit, such as whether it has finished running or not and any errors that happened. Note that
// this does NOT actually run the unit. For that, see the RunUnits method.
func ToRunningUnits(units common.Units, dependencyOrder DependencyOrder, r *report.Report, opts *options.TerragruntOptions) (RunningUnits, error) {
	runningUnits := RunningUnits{}
	for _, unit := range units {
		runningUnits[unit.Path] = NewDependencyController(unit)
	}

	crossLinkedUnits, err := runningUnits.crossLinkDependencies(dependencyOrder)
	if err != nil {
		return crossLinkedUnits, err
	}

	return crossLinkedUnits.RemoveFlagExcluded(r, opts.Experiments.Evaluate(experiment.Report))
}

// SetTerragruntConfig sets the report for the stack.
func (runner *Runner) SetTerragruntConfig(config *config.TerragruntConfig) {
	runner.Stack.ChildTerragruntConfig = config
}

// SetParseOptions sets the report for the stack.
func (runner *Runner) SetParseOptions(parserOptions []hclparse.Option) {
	runner.Stack.ParserOptions = parserOptions
}

// SetReport sets the report for the stack.
func (runner *Runner) SetReport(report *report.Report) {
	runner.Stack.Report = report
}
