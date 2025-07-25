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

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/shell"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/cli/commands/run/creds"
	"github.com/gruntwork-io/terragrunt/cli/commands/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/tf"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const maxLevelsOfRecursion = 20
const existingUnitsCacheName = "existingUnits"

var existingUnits = cache.NewCache[*common.UnitsMap](existingUnitsCacheName)

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
	canonicalTerragruntConfigPaths, err := util.CanonicalPaths(terragruntConfigPaths, ".")
	if err != nil {
		return nil, err
	}

	unitsMap, err := runner.telemetryResolveUnits(ctx, l, canonicalTerragruntConfigPaths)
	if err != nil {
		return nil, err
	}

	externalDependencies, err := runner.telemetryResolveExternalDependencies(ctx, l, unitsMap)
	if err != nil {
		return nil, err
	}

	crossLinkedUnits, err := runner.telemetryCrosslinkDependencies(ctx, unitsMap, externalDependencies, canonicalTerragruntConfigPaths)
	if err != nil {
		return nil, err
	}

	withUnitsIncluded, err := runner.telemetryFlagIncludedDirs(ctx, crossLinkedUnits)
	if err != nil {
		return nil, err
	}

	withUnitsThatAreIncludedByOthers, err := runner.telemetryFlagUnitsThatAreIncluded(ctx, withUnitsIncluded)
	if err != nil {
		return nil, err
	}

	withExcludedUnits, err := runner.telemetryFlagExcludedUnits(ctx, l, withUnitsThatAreIncludedByOthers)
	if err != nil {
		return nil, err
	}

	withUnitsRead, err := runner.telemetryFlagUnitsThatRead(ctx, withExcludedUnits)
	if err != nil {
		return nil, err
	}

	withUnitsExcluded, err := runner.telemetryFlagExcludedDirs(ctx, l, withUnitsRead)
	if err != nil {
		return nil, err
	}

	return withUnitsExcluded, nil
}

// telemetryResolveUnits resolves Terraform units from the given Terragrunt configuration paths
func (runner *Runner) telemetryResolveUnits(ctx context.Context, l log.Logger, canonicalTerragruntConfigPaths []string) (common.UnitsMap, error) {
	var unitsMap common.UnitsMap

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_units", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(ctx context.Context) error {
		howThesePathsWereFound := "Terragrunt config file found in a subdirectory of " + runner.Stack.TerragruntOptions.WorkingDir

		result, err := runner.resolveUnits(ctx, l, canonicalTerragruntConfigPaths, howThesePathsWereFound)
		if err != nil {
			return err
		}

		unitsMap = result

		return nil
	})

	return unitsMap, err
}

// telemetryResolveExternalDependencies resolves external dependencies for the given units
func (runner *Runner) telemetryResolveExternalDependencies(ctx context.Context, l log.Logger, unitsMap common.UnitsMap) (common.UnitsMap, error) {
	var externalDependencies common.UnitsMap

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_external_dependencies_for_units", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(ctx context.Context) error {
		result, err := runner.resolveExternalDependenciesForUnits(ctx, l, unitsMap, common.UnitsMap{}, 0)
		if err != nil {
			return err
		}

		externalDependencies = result

		return nil
	})

	return externalDependencies, err
}

// telemetryCrosslinkDependencies cross-links dependencies between units
func (runner *Runner) telemetryCrosslinkDependencies(ctx context.Context, unitsMap, externalDependencies common.UnitsMap, canonicalTerragruntConfigPaths []string) (common.Units, error) {
	var crossLinkedUnits common.Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "crosslink_dependencies", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result, err := unitsMap.MergeMaps(externalDependencies).CrossLinkDependencies(canonicalTerragruntConfigPaths)
		if err != nil {
			return err
		}

		crossLinkedUnits = result

		return nil
	})

	return crossLinkedUnits, err
}

// telemetryFlagIncludedDirs flags directories that are included in the Terragrunt configuration
func (runner *Runner) telemetryFlagIncludedDirs(ctx context.Context, crossLinkedUnits common.Units) (common.Units, error) {
	var withUnitsIncluded common.Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_included_dirs", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsIncluded = flagIncludedDirs(runner.Stack.TerragruntOptions, crossLinkedUnits)
		return nil
	})

	return withUnitsIncluded, err
}

// telemetryFlagUnitsThatAreIncluded flags units that are included in the Terragrunt configuration
func (runner *Runner) telemetryFlagUnitsThatAreIncluded(ctx context.Context, withUnitsIncluded common.Units) (common.Units, error) {
	var withUnitsThatAreIncludedByOthers common.Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_units_that_are_included", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result, err := flagUnitsThatAreIncluded(runner.Stack.TerragruntOptions, withUnitsIncluded)
		if err != nil {
			return err
		}

		withUnitsThatAreIncludedByOthers = result

		return nil
	})

	return withUnitsThatAreIncludedByOthers, err
}

// telemetryFlagExcludedUnits flags units that are excluded in the Terragrunt configuration
func (runner *Runner) telemetryFlagExcludedUnits(ctx context.Context, l log.Logger, withUnitsThatAreIncludedByOthers common.Units) (common.Units, error) {
	var withExcludedUnits common.Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_excluded_units", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result := flagExcludedUnits(l, runner.Stack.TerragruntOptions, withUnitsThatAreIncludedByOthers)
		withExcludedUnits = result

		return nil
	})

	return withExcludedUnits, err
}

// telemetryFlagUnitsThatRead flags units that read files in the Terragrunt configuration
func (runner *Runner) telemetryFlagUnitsThatRead(ctx context.Context, withExcludedUnits common.Units) (common.Units, error) {
	var withUnitsRead common.Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_units_that_read", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsRead = flagUnitsThatRead(runner.Stack.TerragruntOptions, withExcludedUnits)
		return nil
	})

	return withUnitsRead, err
}

// telemetryFlagExcludedDirs flags directories that are excluded in the Terragrunt configuration
func (runner *Runner) telemetryFlagExcludedDirs(ctx context.Context, l log.Logger, withUnitsRead common.Units) (common.Units, error) {
	var withUnitsExcluded common.Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_excluded_dirs", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsExcluded = flagExcludedDirs(l, runner.Stack.TerragruntOptions, runner.Stack.Report, withUnitsRead)
		return nil
	})

	return withUnitsExcluded, err
}

// Go through each of the given Terragrunt configuration files and resolve the unit that configuration file represents
// into a Unit struct. Note that this method will NOT fill in the Dependencies field of the Unit
// struct (see the crosslinkDependencies method for that). Return a map from unit path to Unit struct.
func (runner *Runner) resolveUnits(ctx context.Context, l log.Logger, canonicalTerragruntConfigPaths []string, howTheseUnitsWereFound string) (common.UnitsMap, error) {
	unitsMap := common.UnitsMap{}

	for _, terragruntConfigPath := range canonicalTerragruntConfigPaths {
		if !util.FileExists(terragruntConfigPath) {
			return nil, common.ProcessingUnitError{UnderlyingError: os.ErrNotExist, UnitPath: terragruntConfigPath, HowThisUnitWasFound: howTheseUnitsWereFound}
		}

		var unit *common.Unit

		err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_terraform_unit", map[string]any{
			"config_path": terragruntConfigPath,
			"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
		}, func(ctx context.Context) error {
			m, err := runner.resolveTerraformUnit(ctx, l, terragruntConfigPath, unitsMap, howTheseUnitsWereFound)
			if err != nil {
				return err
			}

			unit = m

			return nil
		})

		if err != nil {
			return unitsMap, err
		}

		if unit != nil {
			unitsMap[unit.Path] = unit

			var dependencies common.UnitsMap

			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_dependencies_for_unit", map[string]any{
				"config_path": terragruntConfigPath,
				"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
				"unit_path":   unit.Path,
			}, func(ctx context.Context) error {
				deps, err := runner.resolveDependenciesForUnit(ctx, l, unit, unitsMap, true)
				if err != nil {
					return err
				}

				dependencies = deps

				return nil
			})
			if err != nil {
				return unitsMap, err
			}

			unitsMap = collections.MergeMaps(unitsMap, dependencies)
		}
	}

	return unitsMap, nil
}

// Create a Unit struct for the Terraform unit specified by the given Terragrunt configuration file path.
// Note that this method will NOT fill in the Dependencies field of the Unit struct (see the
// crosslinkDependencies method for that).
func (runner *Runner) resolveTerraformUnit(ctx context.Context, l log.Logger, terragruntConfigPath string, unitsMap common.UnitsMap, howThisUnitWasFound string) (*common.Unit, error) {
	unitPath, err := runner.resolveUnitPath(terragruntConfigPath)
	if err != nil {
		return nil, err
	}

	if _, ok := unitsMap[unitPath]; ok {
		return nil, nil
	}

	l, opts, err := runner.cloneOptionsWithConfigPath(l, terragruntConfigPath)
	if err != nil {
		return nil, err
	}

	includeConfig := runner.setupIncludeConfig(terragruntConfigPath, opts)

	if collections.ListContainsElement(opts.ExcludeDirs, unitPath) {
		return &common.Unit{Path: unitPath, Logger: l, TerragruntOptions: opts, FlagExcluded: true}, nil
	}

	parseCtx := runner.createParsingContext(ctx, l, opts)

	if err := runner.acquireCredentials(ctx, l, opts); err != nil {
		return nil, err
	}

	terragruntConfig, err := runner.partialParseConfig(ctx, parseCtx, l, terragruntConfigPath, includeConfig, howThisUnitWasFound)
	if err != nil {
		return nil, err
	}

	runner.Stack.TerragruntOptions.CloneReadFiles(opts.ReadFiles)

	terragruntSource, err := config.GetTerragruntSourceForModule(runner.Stack.TerragruntOptions.Source, unitPath, terragruntConfig)
	if err != nil {
		return nil, err
	}

	opts.Source = terragruntSource

	if err := runner.setupDownloadDir(terragruntConfigPath, opts, l); err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(terragruntConfigPath), "*.tf"))
	if err != nil {
		return nil, err
	}

	if (terragruntConfig.Terraform == nil || terragruntConfig.Terraform.Source == nil || *terragruntConfig.Terraform.Source == "") && matches == nil {
		l.Debugf("Unit %s does not have an associated terraform configuration and will be skipped.", filepath.Dir(terragruntConfigPath))
		return nil, nil
	}

	return &common.Unit{Path: unitPath, Logger: l, Config: *terragruntConfig, TerragruntOptions: opts}, nil
}

func (runner *Runner) resolveUnitPath(terragruntConfigPath string) (string, error) {
	return util.CanonicalPath(filepath.Dir(terragruntConfigPath), ".")
}

func (runner *Runner) cloneOptionsWithConfigPath(l log.Logger, terragruntConfigPath string) (log.Logger, *options.TerragruntOptions, error) {
	l, opts, err := runner.Stack.TerragruntOptions.CloneWithConfigPath(l, terragruntConfigPath)
	if err != nil {
		return l, nil, err
	}

	opts.OriginalTerragruntConfigPath = terragruntConfigPath

	return l, opts, nil
}

func (runner *Runner) setupIncludeConfig(terragruntConfigPath string, opts *options.TerragruntOptions) *config.IncludeConfig {
	var includeConfig *config.IncludeConfig
	if runner.Stack.ChildTerragruntConfig != nil && runner.Stack.ChildTerragruntConfig.ProcessedIncludes.ContainsPath(terragruntConfigPath) {
		includeConfig = &config.IncludeConfig{
			Path: terragruntConfigPath,
		}
		opts.TerragruntConfigPath = runner.Stack.TerragruntOptions.OriginalTerragruntConfigPath
	}

	return includeConfig
}

func (runner *Runner) createParsingContext(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) *config.ParsingContext {
	return config.NewParsingContext(ctx, l, opts).
		WithParseOption(runner.Stack.ParserOptions).
		WithDecodeList(
			config.TerraformSource,
			config.DependenciesBlock,
			config.DependencyBlock,
			config.FeatureFlagsBlock,
			config.ErrorsBlock,
		)
}

func (runner *Runner) acquireCredentials(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	credsGetter := creds.NewGetter()
	return credsGetter.ObtainAndUpdateEnvIfNecessary(ctx, l, opts, externalcmd.NewProvider(l, opts))
}

// nolint:unparam
func (runner *Runner) partialParseConfig(_ context.Context, parseCtx *config.ParsingContext, l log.Logger, terragruntConfigPath string, includeConfig *config.IncludeConfig, howThisUnitWasFound string) (*config.TerragruntConfig, error) {
	terragruntConfig, err := config.PartialParseConfigFile( //nolint:contextcheck
		parseCtx,
		l,
		terragruntConfigPath,
		includeConfig,
	)
	if err != nil {
		return nil, errors.New(common.ProcessingUnitError{
			UnderlyingError:     err,
			HowThisUnitWasFound: howThisUnitWasFound,
			UnitPath:            terragruntConfigPath,
		})
	}

	return terragruntConfig, nil
}

func (runner *Runner) setupDownloadDir(terragruntConfigPath string, opts *options.TerragruntOptions, l log.Logger) error {
	_, defaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(runner.Stack.TerragruntOptions.TerragruntConfigPath)
	if err != nil {
		return err
	}

	if runner.Stack.TerragruntOptions.DownloadDir == defaultDownloadDir {
		_, downloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntConfigPath)
		if err != nil {
			return err
		}

		l.Debugf("Setting download directory for unit %s to %s", filepath.Dir(opts.TerragruntConfigPath), downloadDir)
		opts.DownloadDir = downloadDir
	}

	return nil
}

// resolveDependenciesForUnit looks through the dependencies of the given unit and resolve the dependency paths listed in the unit's config.
// If `skipExternal` is true, the func returns only dependencies that are inside of the current working directory, which means they are part of the environment the
// user is trying to run --all apply or run --all destroy. Note that this method will NOT fill in the Dependencies field of the Unit struct (see the crosslinkDependencies method for that).
func (runner *Runner) resolveDependenciesForUnit(ctx context.Context, l log.Logger, unit *common.Unit, unitsMap common.UnitsMap, skipExternal bool) (common.UnitsMap, error) {
	if unit.Config.Dependencies == nil || len(unit.Config.Dependencies.Paths) == 0 {
		return common.UnitsMap{}, nil
	}

	key := fmt.Sprintf("%s-%s-%v-%v", unit.Path, runner.Stack.TerragruntOptions.WorkingDir, skipExternal, runner.Stack.TerragruntOptions.TerraformCommand)
	if value, ok := existingUnits.Get(ctx, key); ok {
		return *value, nil
	}

	externalTerragruntConfigPaths := []string{}

	for _, dependency := range unit.Config.Dependencies.Paths {
		dependencyPath, err := util.CanonicalPath(dependency, unit.Path)
		if err != nil {
			return common.UnitsMap{}, err
		}

		if skipExternal && !util.HasPathPrefix(dependencyPath, runner.Stack.TerragruntOptions.WorkingDir) {
			continue
		}

		terragruntConfigPath := config.GetDefaultConfigPath(dependencyPath)

		if _, alreadyContainsUnit := unitsMap[dependencyPath]; !alreadyContainsUnit {
			externalTerragruntConfigPaths = append(externalTerragruntConfigPaths, terragruntConfigPath)
		}
	}

	howThesePathsWereFound := fmt.Sprintf("dependency of unit at '%s'", unit.Path)

	result, err := runner.resolveUnits(ctx, l, externalTerragruntConfigPaths, howThesePathsWereFound)
	if err != nil {
		return nil, err
	}

	existingUnits.Put(ctx, key, &result)

	return result, nil
}

// Look through the dependencies of the units in the given map and resolve the "external" dependency paths listed in
// each units config (i.e. those dependencies not in the given list of Terragrunt config canonical file paths).
// These external dependencies are outside of the current working directory, which means they may not be part of the
// environment the user is trying to run --all apply or run --all destroy. Therefore, this method also confirms whether the user wants
// to actually apply those dependencies or just assume they are already applied. Note that this method will NOT fill in
// the Dependencies field of the Unit struct (see the crosslinkDependencies method for that).
func (runner *Runner) resolveExternalDependenciesForUnits(ctx context.Context, l log.Logger, unitsMap, unitsAlreadyProcessed common.UnitsMap, recursionLevel int) (common.UnitsMap, error) {
	allExternalDependencies := common.UnitsMap{}
	unitsToSkip := unitsMap.MergeMaps(unitsAlreadyProcessed)

	// Simple protection from circular dependencies causing a Stack Overflow due to infinite recursion
	if recursionLevel > maxLevelsOfRecursion {
		return allExternalDependencies, errors.New(common.InfiniteRecursionError{RecursionLevel: maxLevelsOfRecursion, Units: unitsToSkip})
	}

	sortedKeys := unitsMap.SortedKeys()
	for _, key := range sortedKeys {
		unit := unitsMap[key]

		externalDependencies, err := runner.resolveDependenciesForUnit(ctx, l, unit, unitsToSkip, false)
		if err != nil {
			return externalDependencies, err
		}

		l, unitOpts, err := runner.Stack.TerragruntOptions.CloneWithConfigPath(l, config.GetDefaultConfigPath(unit.Path))
		if err != nil {
			return nil, err
		}

		for _, externalDependency := range externalDependencies {
			if _, alreadyFound := unitsToSkip[externalDependency.Path]; alreadyFound {
				continue
			}

			shouldApply := false
			if !runner.Stack.TerragruntOptions.IgnoreExternalDependencies {
				shouldApply, err = confirmShouldApplyExternalDependency(ctx, unit, l, externalDependency, unitOpts)
				if err != nil {
					return externalDependencies, err
				}
			}

			externalDependency.AssumeAlreadyApplied = !shouldApply
			allExternalDependencies[externalDependency.Path] = externalDependency
		}
	}

	if len(allExternalDependencies) > 0 {
		recursiveDependencies, err := runner.resolveExternalDependenciesForUnits(ctx, l, allExternalDependencies, unitsMap, recursionLevel+1)
		if err != nil {
			return allExternalDependencies, err
		}

		return allExternalDependencies.MergeMaps(recursiveDependencies), nil
	}

	return allExternalDependencies, nil
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

// Confirm with the user whether they want Terragrunt to assume the given dependency of the given unit is already
// applied. If the user selects "yes", then Terragrunt will apply that unit as well.
// Note that we skip the prompt for `run --all destroy` calls. Given the destructive and irreversible nature of destroy, we don't
// want to provide any risk to the user of accidentally destroying an external dependency unless explicitly included
// with the --queue-include-external or --queue-include-dir flags.
func confirmShouldApplyExternalDependency(ctx context.Context, unit *common.Unit, l log.Logger, dependency *common.Unit, opts *options.TerragruntOptions) (bool, error) {
	if opts.IncludeExternalDependencies {
		l.Debugf("The --queue-include-external flag is set, so automatically including all external dependencies, and will run this command against unit %s, which is a dependency of unit %s.", dependency.Path, unit.Path)
		return true, nil
	}

	if opts.NonInteractive {
		l.Debugf("The --non-interactive flag is set. To avoid accidentally affecting external dependencies with a run --all command, will not run this command against unit %s, which is a dependency of unit %s.", dependency.Path, unit.Path)
		return false, nil
	}

	stackCmd := opts.TerraformCommand
	if stackCmd == "destroy" {
		l.Debugf("run --all command called with destroy. To avoid accidentally having destructive effects on external dependencies with run --all command, will not run this command against unit %s, which is a dependency of unit %s.", dependency.Path, unit.Path)
		return false, nil
	}

	l.Infof("Unit %s has external dependency %s", unit.Path, dependency.Path)

	return shell.PromptUserForYesNo(ctx, l, "Should Terragrunt apply the external dependency?", opts)
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

// flagIncludedDirs includes all units by default.
//
// However, when anything that triggers ExcludeByDefault is set, the function will instead
// selectively include only the units that are in the list specified via the IncludeDirs option.
func flagIncludedDirs(opts *options.TerragruntOptions, units common.Units) common.Units {
	if !opts.ExcludeByDefault {
		return units
	}

	for _, unit := range units {
		if unit.FindUnitInPath(opts.IncludeDirs) {
			unit.FlagExcluded = false
		} else {
			unit.FlagExcluded = true
		}
	}

	// Mark all affected dependencies as included before proceeding if not in strict include mode.
	if !opts.StrictInclude {
		for _, unit := range units {
			if !unit.FlagExcluded {
				for _, dependency := range unit.Dependencies {
					dependency.FlagExcluded = false
				}
			}
		}
	}

	return units
}

// flagUnitsThatAreIncluded iterates over a unit slice and flags all units that include at least one file in
// the specified include list on the TerragruntOptions ModulesThatInclude attribute.
func flagUnitsThatAreIncluded(opts *options.TerragruntOptions, units common.Units) (common.Units, error) {
	unitsThatInclude := append(opts.ModulesThatInclude, opts.UnitsReading...) //nolint:gocritic

	if len(unitsThatInclude) == 0 {
		return units, nil
	}

	unitsThatIncludeCanonicalPaths := []string{}

	for _, includePath := range unitsThatInclude {
		canonicalPath, err := util.CanonicalPath(includePath, opts.WorkingDir)
		if err != nil {
			return nil, err
		}

		unitsThatIncludeCanonicalPaths = append(unitsThatIncludeCanonicalPaths, canonicalPath)
	}

	for _, unit := range units {
		if err := flagUnitIncludes(unit, unitsThatIncludeCanonicalPaths); err != nil {
			return nil, err
		}

		if err := flagUnitDependencies(unit, unitsThatIncludeCanonicalPaths); err != nil {
			return nil, err
		}
	}

	return units, nil
}

func flagUnitIncludes(unit *common.Unit, canonicalPaths []string) error {
	for _, includeConfig := range unit.Config.ProcessedIncludes {
		canonicalPath, err := util.CanonicalPath(includeConfig.Path, unit.Path)
		if err != nil {
			return err
		}

		if util.ListContainsElement(canonicalPaths, canonicalPath) {
			unit.FlagExcluded = false
		}
	}

	return nil
}

func flagUnitDependencies(unit *common.Unit, canonicalPaths []string) error {
	for _, dependency := range unit.Dependencies {
		if dependency.FlagExcluded {
			continue
		}

		if err := flagDependencyIncludes(dependency, unit.Path, canonicalPaths); err != nil {
			return err
		}
	}

	return nil
}

func flagDependencyIncludes(dependency *common.Unit, unitPath string, canonicalPaths []string) error {
	for _, includeConfig := range dependency.Config.ProcessedIncludes {
		canonicalPath, err := util.CanonicalPath(includeConfig.Path, unitPath)
		if err != nil {
			return err
		}

		if util.ListContainsElement(canonicalPaths, canonicalPath) {
			dependency.FlagExcluded = false
		}
	}

	return nil
}

// flagExcludedUnits iterates over a unit slice and flags all units that are excluded based on the exclude block.
func flagExcludedUnits(l log.Logger, opts *options.TerragruntOptions, units common.Units) common.Units {
	for _, unit := range units {
		excludeConfig := unit.Config.Exclude

		if excludeConfig == nil {
			continue
		}

		if !excludeConfig.IsActionListed(opts.TerraformCommand) {
			continue
		}

		if excludeConfig.If {
			l.Debugf("Unit %s is excluded by exclude block", unit.Path)
			unit.FlagExcluded = true
		}

		if excludeConfig.ExcludeDependencies != nil && *excludeConfig.ExcludeDependencies {
			l.Debugf("Excluding dependencies for unit %s by exclude block", unit.Path)

			for _, dependency := range unit.Dependencies {
				dependency.FlagExcluded = true
			}
		}
	}

	return units
}

// flagUnitsThatRead iterates over a unit slice and flags all units that read at least one file in the specified
// file list in the TerragruntOptions UnitsReading attribute.
func flagUnitsThatRead(opts *options.TerragruntOptions, units common.Units) common.Units {
	// If no UnitsThatRead is specified return the units list instantly
	if len(opts.UnitsReading) == 0 {
		return units
	}

	for _, path := range opts.UnitsReading {
		if !filepath.IsAbs(path) {
			path = filepath.Join(opts.WorkingDir, path)
			path = filepath.Clean(path)
		}

		for _, unit := range units {
			if opts.DidReadFile(path, unit.Path) {
				unit.FlagExcluded = false
			}
		}
	}

	return units
}

// flagExcludedDirs iterates over a unit slice and flags all entries as excluded listed in the queue-exclude-dir CLI flag.
func flagExcludedDirs(l log.Logger, opts *options.TerragruntOptions, r *report.Report, units common.Units) common.Units {
	// If we don't have any excludes, we don't need to do anything.
	if len(opts.ExcludeDirs) == 0 {
		return units
	}

	for _, unit := range units {
		if unit.FindUnitInPath(opts.ExcludeDirs) {
			// Mark unit itself as excluded
			unit.FlagExcluded = true

			if opts.Experiments.Evaluate(experiment.Report) {
				// TODO: Make an upsert option for ends,
				// so that I don't have to do this every time.
				run, err := r.GetRun(unit.Path)
				if err != nil {
					run, err = report.NewRun(unit.Path)
					if err != nil {
						l.Errorf("Error creating run for unit %s: %v", unit.Path, err)

						continue
					}

					if err := r.AddRun(run); err != nil {
						l.Errorf("Error adding run for unit %s: %v", unit.Path, err)

						continue
					}
				}

				if err := r.EndRun(
					run.Path,
					report.WithResult(report.ResultExcluded),
					report.WithReason(report.ReasonExcludeDir),
				); err != nil {
					l.Errorf("Error ending run for unit %s: %v", unit.Path, err)

					continue
				}
			}
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range unit.Dependencies {
			if dependency.FindUnitInPath(opts.ExcludeDirs) {
				dependency.FlagExcluded = true

				if opts.Experiments.Evaluate(experiment.Report) {
					run, err := r.GetRun(dependency.Path)
					if err != nil {
						l.Errorf("Error getting run for dependency %s: %v", dependency.Path, err)
						continue
					}

					if err := r.EndRun(
						run.Path,
						report.WithResult(report.ResultExcluded),
						report.WithReason(report.ReasonExcludeDir),
					); err != nil {
						l.Errorf("Error ending run for dependency %s: %v", dependency.Path, err)
						continue
					}
				}
			}
		}
	}

	return units
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
