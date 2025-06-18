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
	"github.com/gruntwork-io/terragrunt/shell"

	"github.com/gruntwork-io/terragrunt/internal/runner/common"

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
const existingModulesCacheName = "existingModules"

var existingModules = cache.NewCache[*common.UnitsMap](existingModulesCacheName)

// Runner implements the Stack interface and represents a stack of Terraform modules (i.e. folders with Terraform templates) that you can "spin up" or "spin down" in a single command
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

func (runner *Runner) GetStack() *common.Stack {
	return runner.Stack
}

// LogModuleDeployOrder will log the modules that will be deployed by this operation, in the order that the operations
// happen. For plan and apply, the order will be bottom to top (dependencies first), while for destroy the order will be
// in reverse.
func (runner *Runner) LogModuleDeployOrder(l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf("The runner at %s will be processed in the following order for command %s:\n", runner.Stack.TerragruntOptions.WorkingDir, terraformCommand)

	runGraph, err := runner.GetModuleRunGraph(terraformCommand)
	if err != nil {
		return err
	}

	for i, group := range runGraph {
		outStr += fmt.Sprintf("Group %d\n", i+1)
		for _, module := range group {
			outStr += fmt.Sprintf("- Unit %s\n", module.Path)
		}

		outStr += "\n"
	}

	l.Info(outStr)

	return nil
}

// JSONModuleDeployOrder will return the modules that will be deployed by a plan/apply operation, in the order
// that the operations happen.
func (runner *Runner) JSONModuleDeployOrder(terraformCommand string) (string, error) {
	runGraph, err := runner.GetModuleRunGraph(terraformCommand)
	if err != nil {
		return "", errors.New(err)
	}

	// Convert the module paths to a string array for JSON marshalling
	// The index should be the group number, and the value should be an array of module paths
	jsonGraph := make(map[string][]string)

	for i, group := range runGraph {
		groupNum := "Group " + strconv.Itoa(i+1)
		jsonGraph[groupNum] = make([]string, len(group))

		for j, module := range group {
			jsonGraph[groupNum][j] = module.Path
		}
	}

	j, err := json.MarshalIndent(jsonGraph, "", "  ")
	if err != nil {
		return "", errors.New(err)
	}

	return string(j), nil
}

// Graph creates a graphviz representation of the modules
func (runner *Runner) Graph(l log.Logger, opts *options.TerragruntOptions) {
	err := runner.Stack.Units.WriteDot(l, opts.Writer, opts)
	if err != nil {
		l.Warnf("Failed to graph dot: %v", err)
	}
}

func (runner *Runner) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	stackCmd := opts.TerraformCommand

	// prepare folder for output hierarchy if output folder is set
	if opts.OutputFolder != "" {
		for _, module := range runner.Stack.Units {
			planFile := module.OutputFile(l, opts)

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
		return runner.RunModulesIgnoreOrder(ctx, opts)
	case stackCmd == tf.CommandNameDestroy:
		return runner.RunModulesReverseOrder(ctx, opts)
	default:
		return runner.RunModules(ctx, opts)
	}
}

// We inspect the error streams to give an explicit message if the plan failed because there were references to
// remote states. `terraform plan` will fail if it tries to access remote state from dependencies and the plan
// has never been applied on the dependency.
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

// Sync the TerraformCliArgs for each module in the stack to match the provided terragruntOptions struct.
func (runner *Runner) syncTerraformCliArgs(l log.Logger, opts *options.TerragruntOptions) {
	for _, module := range runner.Stack.Units {
		module.TerragruntOptions.TerraformCliArgs = collections.MakeCopyOfList(opts.TerraformCliArgs)

		planFile := module.PlanFile(l, opts)

		if planFile != "" {
			l.Debugf("Using output file %s for module %s", planFile, module.TerragruntOptions.TerragruntConfigPath)

			if module.TerragruntOptions.TerraformCommand == tf.CommandNamePlan {
				// for plan command add -out=<file> to the terraform cli args
				module.TerragruntOptions.TerraformCliArgs = util.StringListInsert(module.TerragruntOptions.TerraformCliArgs, "-out="+planFile, len(module.TerragruntOptions.TerraformCliArgs))
			} else {
				module.TerragruntOptions.TerraformCliArgs = util.StringListInsert(module.TerragruntOptions.TerraformCliArgs, planFile, len(module.TerragruntOptions.TerraformCliArgs))
			}
		}
	}
}

func (runner *Runner) toRunningModules(terraformCommand string) (RunningModules, error) {
	switch terraformCommand {
	case tf.CommandNameDestroy:
		return ToRunningModules(runner.Stack.Units, ReverseOrder, runner.Stack.Report, runner.Stack.TerragruntOptions)
	default:
		return ToRunningModules(runner.Stack.Units, NormalOrder, runner.Stack.Report, runner.Stack.TerragruntOptions)
	}
}

// GetModuleRunGraph converts the module list to a graph that shows the order in which the modules will be
// applied/destroyed. The return structure is a list of lists, where the nested list represents modules that can be
// deployed concurrently, and the outer list indicates the order. This will only include those modules that do NOT have
// the exclude flag set.
func (runner *Runner) GetModuleRunGraph(terraformCommand string) ([]common.Units, error) {
	moduleRunGraph, err := runner.toRunningModules(terraformCommand)
	if err != nil {
		return nil, err
	}

	// Set maxDepth for the graph so that we don't get stuck in an infinite loop.
	const maxDepth = 1000
	groups := moduleRunGraph.toTerraformModuleGroups(maxDepth)

	return groups, nil
}

// Find all the Terraform modules in the folders that contain the given Terragrunt config files and assemble those
// modules into a Stack object that can be applied or destroyed in a single command
func (runner *Runner) createStackForTerragruntConfigPaths(ctx context.Context, l log.Logger, terragruntConfigPaths []string) error {
	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "create_stack_for_terragrunt_config_paths", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(ctx context.Context) error {
		if len(terragruntConfigPaths) == 0 {
			return errors.New(common.ErrNoTerraformModulesFound)
		}

		modules, err := runner.ResolveTerraformModules(ctx, l, terragruntConfigPaths)
		if err != nil {
			return errors.New(err)
		}

		runner.Stack.Units = modules

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
// and resolve the module that configuration file represents into a Unit struct.
// Return the list of these Unit structs.
func (runner *Runner) ResolveTerraformModules(ctx context.Context, l log.Logger, terragruntConfigPaths []string) (common.Units, error) {
	canonicalTerragruntConfigPaths, err := util.CanonicalPaths(terragruntConfigPaths, ".")
	if err != nil {
		return nil, err
	}

	var modulesMap common.UnitsMap

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_modules", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(ctx context.Context) error {
		howThesePathsWereFound := "Terragrunt config file found in a subdirectory of " + runner.Stack.TerragruntOptions.WorkingDir

		result, err := runner.resolveModules(ctx, l, canonicalTerragruntConfigPaths, howThesePathsWereFound)
		if err != nil {
			return err
		}

		modulesMap = result

		return nil
	})

	if err != nil {
		return nil, err
	}

	var externalDependencies common.UnitsMap

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_external_dependencies_for_modules", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(ctx context.Context) error {
		result, err := runner.resolveExternalDependenciesForModules(ctx, l, modulesMap, common.UnitsMap{}, 0)
		if err != nil {
			return err
		}

		externalDependencies = result

		return nil
	})
	if err != nil {
		return nil, err
	}

	var crossLinkedModules common.Units

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "crosslink_dependencies", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result, err := modulesMap.MergeMaps(externalDependencies).CrossLinkDependencies(canonicalTerragruntConfigPaths)
		if err != nil {
			return err
		}

		crossLinkedModules = result

		return nil
	})

	if err != nil {
		return nil, err
	}

	var withUnitsIncluded common.Units

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_included_dirs", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsIncluded = flagIncludedDirs(runner.Stack.TerragruntOptions, crossLinkedModules)
		return nil
	})

	if err != nil {
		return nil, err
	}

	var withUnitsThatAreIncludedByOthers common.Units

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_units_that_are_included", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result, err := flagUnitsThatAreIncluded(runner.Stack.TerragruntOptions, withUnitsIncluded)
		if err != nil {
			return err
		}

		withUnitsThatAreIncludedByOthers = result

		return nil
	})

	if err != nil {
		return nil, err
	}

	var withExcludedUnits common.Units

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_excluded_units", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result := flagExcludedUnits(l, runner.Stack.TerragruntOptions, withUnitsThatAreIncludedByOthers)
		withExcludedUnits = result

		return nil
	})

	if err != nil {
		return nil, err
	}

	var withUnitsRead common.Units

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_units_that_read", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsRead = flagUnitsThatRead(runner.Stack.TerragruntOptions, withExcludedUnits)
		return nil
	})

	if err != nil {
		return nil, err
	}

	var withModulesExcluded common.Units

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_excluded_dirs", map[string]any{
		"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withModulesExcluded = flagExcludedDirs(l, runner.Stack.TerragruntOptions, runner.Stack.Report, withUnitsRead)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return withModulesExcluded, nil
}

// Go through each of the given Terragrunt configuration files and resolve the module that configuration file represents
// into a Unit struct. Note that this method will NOT fill in the Dependencies field of the Unit
// struct (see the crosslinkDependencies method for that). Return a map from module path to Unit struct.
func (runner *Runner) resolveModules(ctx context.Context, l log.Logger, canonicalTerragruntConfigPaths []string, howTheseModulesWereFound string) (common.UnitsMap, error) {
	modulesMap := common.UnitsMap{}

	for _, terragruntConfigPath := range canonicalTerragruntConfigPaths {
		if !util.FileExists(terragruntConfigPath) {
			return nil, common.ProcessingModuleError{UnderlyingError: os.ErrNotExist, ModulePath: terragruntConfigPath, HowThisModuleWasFound: howTheseModulesWereFound}
		}

		var module *common.Unit

		err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_terraform_module", map[string]any{
			"config_path": terragruntConfigPath,
			"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
		}, func(ctx context.Context) error {
			m, err := runner.resolveTerraformModule(ctx, l, terragruntConfigPath, modulesMap, howTheseModulesWereFound)
			if err != nil {
				return err
			}

			module = m

			return nil
		})

		if err != nil {
			return modulesMap, err
		}

		if module != nil {
			modulesMap[module.Path] = module

			var dependencies common.UnitsMap

			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "resolve_dependencies_for_module", map[string]any{
				"config_path": terragruntConfigPath,
				"working_dir": runner.Stack.TerragruntOptions.WorkingDir,
				"module_path": module.Path,
			}, func(ctx context.Context) error {
				deps, err := runner.resolveDependenciesForModule(ctx, l, module, modulesMap, true)
				if err != nil {
					return err
				}

				dependencies = deps

				return nil
			})
			if err != nil {
				return modulesMap, err
			}

			modulesMap = collections.MergeMaps(modulesMap, dependencies)
		}
	}

	return modulesMap, nil
}

// Create a Unit struct for the Terraform module specified by the given Terragrunt configuration file path.
// Note that this method will NOT fill in the Dependencies field of the Unit struct (see the
// crosslinkDependencies method for that).
func (runner *Runner) resolveTerraformModule(ctx context.Context, l log.Logger, terragruntConfigPath string, modulesMap common.UnitsMap, howThisModuleWasFound string) (*common.Unit, error) {
	modulePath, err := util.CanonicalPath(filepath.Dir(terragruntConfigPath), ".")
	if err != nil {
		return nil, err
	}

	if _, ok := modulesMap[modulePath]; ok {
		return nil, nil
	}

	// Clone the options struct so we don't modify the original one. This is especially important as run --all operations
	// happen concurrently.
	l, opts, err := runner.Stack.TerragruntOptions.CloneWithConfigPath(l, terragruntConfigPath)
	if err != nil {
		return nil, err
	}

	// We need to reset the original path for each module. Otherwise, this path will be set to wherever you ran run --all
	// from, which is not what any of the modules will want.
	opts.OriginalTerragruntConfigPath = terragruntConfigPath

	// If `childTerragruntConfig.ProcessedIncludes` contains the path `terragruntConfigPath`, then this is a parent config
	// which implies that `TerragruntConfigPath` must refer to a child configuration file, and the defined `IncludeConfig` must contain the path to the file itself
	// for the built-in functions `read_terragrunt_config()`, `path_relative_to_include()` to work correctly.
	var includeConfig *config.IncludeConfig

	if runner.Stack.ChildTerragruntConfig != nil && runner.Stack.ChildTerragruntConfig.ProcessedIncludes.ContainsPath(terragruntConfigPath) {
		includeConfig = &config.IncludeConfig{
			Path: terragruntConfigPath,
		}
		opts.TerragruntConfigPath = runner.Stack.TerragruntOptions.OriginalTerragruntConfigPath
	}

	if collections.ListContainsElement(opts.ExcludeDirs, modulePath) {
		// module is excluded
		return &common.Unit{Path: modulePath, Logger: l, TerragruntOptions: opts, FlagExcluded: true}, nil
	}

	parseCtx := config.NewParsingContext(ctx, l, opts).
		WithParseOption(runner.Stack.ParserOptions).
		WithDecodeList(
			// Need for initializing the modules
			config.TerraformSource,

			// Need for parsing out the dependencies
			config.DependenciesBlock,
			config.DependencyBlock,
			config.FeatureFlagsBlock,
			config.ErrorsBlock,
		)

	// Credentials have to be acquired before the config is parsed, as the config may contain interpolation functions
	// that require credentials to be available.
	credsGetter := creds.NewGetter()
	if err := credsGetter.ObtainAndUpdateEnvIfNecessary(ctx, l, opts, externalcmd.NewProvider(l, opts)); err != nil {
		return nil, err
	}

	// We only partially parse the config, only using the pieces that we need in this section. This config will be fully
	// parsed at a later stage right before the action is run. This is to delay interpolation of functions until right
	// before we call out to terraform.

	// TODO: Remove lint suppression
	terragruntConfig, err := config.PartialParseConfigFile( //nolint:contextcheck
		parseCtx,
		l,
		terragruntConfigPath,
		includeConfig,
	)
	if err != nil {
		return nil, errors.New(common.ProcessingModuleError{
			UnderlyingError:       err,
			HowThisModuleWasFound: howThisModuleWasFound,
			ModulePath:            terragruntConfigPath,
		})
	}

	// Hack to persist readFiles. Need to discuss with team to see if there is a better way to handle this.
	runner.Stack.TerragruntOptions.CloneReadFiles(opts.ReadFiles)

	terragruntSource, err := config.GetTerragruntSourceForModule(runner.Stack.TerragruntOptions.Source, modulePath, terragruntConfig)
	if err != nil {
		return nil, err
	}

	opts.Source = terragruntSource

	_, defaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(runner.Stack.TerragruntOptions.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	// If we're using the default download directory, put it into the same folder as the Terragrunt configuration file.
	// If we're not using the default, then the user has specified a custom download directory, and we leave it as-is.
	if runner.Stack.TerragruntOptions.DownloadDir == defaultDownloadDir {
		_, downloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntConfigPath)
		if err != nil {
			return nil, err
		}

		l.Debugf("Setting download directory for module %s to %s", filepath.Dir(opts.TerragruntConfigPath), downloadDir)
		opts.DownloadDir = downloadDir
	}

	// Fix for https://github.com/gruntwork-io/terragrunt/issues/208
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(terragruntConfigPath), "*.tf"))
	if err != nil {
		return nil, err
	}

	if (terragruntConfig.Terraform == nil || terragruntConfig.Terraform.Source == nil || *terragruntConfig.Terraform.Source == "") && matches == nil {
		l.Debugf("Module %s does not have an associated terraform configuration and will be skipped.", filepath.Dir(terragruntConfigPath))
		return nil, nil
	}

	//TODO: fix linking Stack: runner
	return &common.Unit{Path: modulePath, Logger: l, Config: *terragruntConfig, TerragruntOptions: opts}, nil
}

// resolveDependenciesForModule looks through the dependencies of the given module and resolve the dependency paths listed in the module's config.
// If `skipExternal` is true, the func returns only dependencies that are inside of the current working directory, which means they are part of the environment the
// user is trying to run --all apply or run --all destroy. Note that this method will NOT fill in the Dependencies field of the Unit struct (see the crosslinkDependencies method for that).
func (runner *Runner) resolveDependenciesForModule(ctx context.Context, l log.Logger, module *common.Unit, modulesMap common.UnitsMap, skipExternal bool) (common.UnitsMap, error) {
	if module.Config.Dependencies == nil || len(module.Config.Dependencies.Paths) == 0 {
		return common.UnitsMap{}, nil
	}

	key := fmt.Sprintf("%s-%s-%v-%v", module.Path, runner.Stack.TerragruntOptions.WorkingDir, skipExternal, runner.Stack.TerragruntOptions.TerraformCommand)
	if value, ok := existingModules.Get(ctx, key); ok {
		return *value, nil
	}

	externalTerragruntConfigPaths := []string{}

	for _, dependency := range module.Config.Dependencies.Paths {
		dependencyPath, err := util.CanonicalPath(dependency, module.Path)
		if err != nil {
			return common.UnitsMap{}, err
		}

		if skipExternal && !util.HasPathPrefix(dependencyPath, runner.Stack.TerragruntOptions.WorkingDir) {
			continue
		}

		terragruntConfigPath := config.GetDefaultConfigPath(dependencyPath)

		if _, alreadyContainsModule := modulesMap[dependencyPath]; !alreadyContainsModule {
			externalTerragruntConfigPaths = append(externalTerragruntConfigPaths, terragruntConfigPath)
		}
	}

	howThesePathsWereFound := fmt.Sprintf("dependency of module at '%s'", module.Path)

	result, err := runner.resolveModules(ctx, l, externalTerragruntConfigPaths, howThesePathsWereFound)
	if err != nil {
		return nil, err
	}

	existingModules.Put(ctx, key, &result)

	return result, nil
}

// Look through the dependencies of the modules in the given map and resolve the "external" dependency paths listed in
// each modules config (i.e. those dependencies not in the given list of Terragrunt config canonical file paths).
// These external dependencies are outside of the current working directory, which means they may not be part of the
// environment the user is trying to run --all apply or run --all destroy. Therefore, this method also confirms whether the user wants
// to actually apply those dependencies or just assume they are already applied. Note that this method will NOT fill in
// the Dependencies field of the Unit struct (see the crosslinkDependencies method for that).
func (runner *Runner) resolveExternalDependenciesForModules(ctx context.Context, l log.Logger, modulesMap, modulesAlreadyProcessed common.UnitsMap, recursionLevel int) (common.UnitsMap, error) {
	allExternalDependencies := common.UnitsMap{}
	modulesToSkip := modulesMap.MergeMaps(modulesAlreadyProcessed)

	// Simple protection from circular dependencies causing a Stack Overflow due to infinite recursion
	if recursionLevel > maxLevelsOfRecursion {
		return allExternalDependencies, errors.New(common.InfiniteRecursionError{RecursionLevel: maxLevelsOfRecursion, Modules: modulesToSkip})
	}

	sortedKeys := modulesMap.SortedKeys()
	for _, key := range sortedKeys {
		module := modulesMap[key]

		externalDependencies, err := runner.resolveDependenciesForModule(ctx, l, module, modulesToSkip, false)
		if err != nil {
			return externalDependencies, err
		}

		l, moduleOpts, err := runner.Stack.TerragruntOptions.CloneWithConfigPath(l, config.GetDefaultConfigPath(module.Path))
		if err != nil {
			return nil, err
		}

		for _, externalDependency := range externalDependencies {
			if _, alreadyFound := modulesToSkip[externalDependency.Path]; alreadyFound {
				continue
			}

			shouldApply := false
			if !runner.Stack.TerragruntOptions.IgnoreExternalDependencies {
				shouldApply, err = confirmShouldApplyExternalDependency(ctx, module, l, externalDependency, moduleOpts)
				if err != nil {
					return externalDependencies, err
				}
			}

			externalDependency.AssumeAlreadyApplied = !shouldApply
			allExternalDependencies[externalDependency.Path] = externalDependency
		}
	}

	if len(allExternalDependencies) > 0 {
		recursiveDependencies, err := runner.resolveExternalDependenciesForModules(ctx, l, allExternalDependencies, modulesMap, recursionLevel+1)
		if err != nil {
			return allExternalDependencies, err
		}

		return allExternalDependencies.MergeMaps(recursiveDependencies), nil
	}

	return allExternalDependencies, nil
}

// ListStackDependentModules - build a map with each module and its dependent modules
func (runner *Runner) ListStackDependentModules() map[string][]string {
	// build map of dependent modules
	// module path -> list of dependent modules
	var dependentModules = make(map[string][]string)

	// build initial mapping of dependent modules
	for _, module := range runner.Stack.Units {
		if len(module.Dependencies) != 0 {
			for _, dep := range module.Dependencies {
				dependentModules[dep.Path] = util.RemoveDuplicatesFromList(append(dependentModules[dep.Path], module.Path))
			}
		}
	}

	// Floyd–Warshall inspired approach to find dependent modules
	// merge map slices by key until no more updates are possible

	// Example:
	// Initial setup:
	// dependentModules["module1"] = ["module2", "module3"]
	// dependentModules["module2"] = ["module3"]
	// dependentModules["module3"] = ["module4"]
	// dependentModules["module4"] = ["module5"]

	// After first iteration: (module1 += module4, module2 += module4, module3 += module5)
	// dependentModules["module1"] = ["module2", "module3", "module4"]
	// dependentModules["module2"] = ["module3", "module4"]
	// dependentModules["module3"] = ["module4", "module5"]
	// dependentModules["module4"] = ["module5"]

	// After second iteration: (module1 += module5, module2 += module5)
	// dependentModules["module1"] = ["module2", "module3", "module4", "module5"]
	// dependentModules["module2"] = ["module3", "module4", "module5"]
	// dependentModules["module3"] = ["module4", "module5"]
	// dependentModules["module4"] = ["module5"]

	// Done, no more updates and in map we have all dependent modules for each module.

	for {
		noUpdates := true

		for module, dependents := range dependentModules {
			for _, dependent := range dependents {
				initialSize := len(dependentModules[module])
				// merge without duplicates
				list := util.RemoveDuplicatesFromList(append(dependentModules[module], dependentModules[dependent]...))
				list = util.RemoveElementFromList(list, module)

				dependentModules[module] = list
				if initialSize != len(dependentModules[module]) {
					noUpdates = false
				}
			}
		}

		if noUpdates {
			break
		}
	}

	return dependentModules
}

// Modules returns the Terraform modules in the stack.
func (runner *Runner) Modules() common.Units {
	return runner.Stack.Units
}

// FindModuleByPath finds a module by its path.
func (runner *Runner) FindModuleByPath(path string) *common.Unit {
	for _, module := range runner.Stack.Units {
		if module.Path == path {
			return module
		}
	}

	return nil
}

// Confirm with the user whether they want Terragrunt to assume the given dependency of the given module is already
// applied. If the user selects "yes", then Terragrunt will apply that module as well.
// Note that we skip the prompt for `run --all destroy` calls. Given the destructive and irreversible nature of destroy, we don't
// want to provide any risk to the user of accidentally destroying an external dependency unless explicitly included
// with the --queue-include-external or --queue-include-dir flags.
func confirmShouldApplyExternalDependency(ctx context.Context, unit *common.Unit, l log.Logger, dependency *common.Unit, opts *options.TerragruntOptions) (bool, error) {
	if opts.IncludeExternalDependencies {
		l.Debugf("The --queue-include-external flag is set, so automatically including all external dependencies, and will run this command against module %s, which is a dependency of module %s.", dependency.Path, unit.Path)
		return true, nil
	}

	if opts.NonInteractive {
		l.Debugf("The --non-interactive flag is set. To avoid accidentally affecting external dependencies with a run --all command, will not run this command against module %s, which is a dependency of module %s.", dependency.Path, unit.Path)
		return false, nil
	}

	stackCmd := opts.TerraformCommand
	if stackCmd == "destroy" {
		l.Debugf("run --all command called with destroy. To avoid accidentally having destructive effects on external dependencies with run --all command, will not run this command against module %s, which is a dependency of module %s.", dependency.Path, unit.Path)
		return false, nil
	}

	l.Infof("Module %s has external dependency %s", unit.Path, dependency.Path)

	return shell.PromptUserForYesNo(ctx, l, "Should Terragrunt apply the external dependency?", opts)
}

// RunModules runs the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func (runner *Runner) RunModules(ctx context.Context, opts *options.TerragruntOptions) error {
	runningModules, err := ToRunningModules(runner.Stack.Units, NormalOrder, runner.Stack.Report, opts)
	if err != nil {
		return err
	}

	return runningModules.runModules(ctx, opts, runner.Stack.Report, opts.Parallelism)
}

// RunModulesReverseOrder runs the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in the reverse order of their inter-dependencies, using
// as much concurrency as possible.
func (runner *Runner) RunModulesReverseOrder(ctx context.Context, opts *options.TerragruntOptions) error {
	runningModules, err := ToRunningModules(runner.Stack.Units, ReverseOrder, runner.Stack.Report, opts)
	if err != nil {
		return err
	}

	return runningModules.runModules(ctx, opts, runner.Stack.Report, opts.Parallelism)
}

// RunModulesIgnoreOrder runs the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed without caring for inter-dependencies.
func (runner *Runner) RunModulesIgnoreOrder(ctx context.Context, opts *options.TerragruntOptions) error {
	runningModules, err := ToRunningModules(runner.Stack.Units, IgnoreOrder, runner.Stack.Report, opts)
	if err != nil {
		return err
	}

	return runningModules.runModules(ctx, opts, runner.Stack.Report, opts.Parallelism)
}

// ToRunningModules converts the list of modules to a map from module path to a runningModule struct. This struct contains information
// about executing the module, such as whether it has finished running or not and any errors that happened. Note that
// this does NOT actually run the module. For that, see the RunModules method.
func ToRunningModules(units common.Units, dependencyOrder DependencyOrder, r *report.Report, opts *options.TerragruntOptions) (RunningModules, error) {
	runningModules := RunningModules{}
	for _, module := range units {
		runningModules[module.Path] = newRunningModule(module)
	}

	crossLinkedModules, err := runningModules.crossLinkDependencies(dependencyOrder)
	if err != nil {
		return crossLinkedModules, err
	}

	return crossLinkedModules.RemoveFlagExcluded(r, opts.Experiments.Evaluate(experiment.Report))
}

// flagIncludedDirs includes all units by default.
//
// However, when anything that triggers ExcludeByDefault is set, the function will instead
// selectively include only the units that are in the list specified via the IncludeDirs option.
func flagIncludedDirs(opts *options.TerragruntOptions, modules common.Units) common.Units {
	if !opts.ExcludeByDefault {
		return modules
	}

	for _, module := range modules {
		if module.FindModuleInPath(opts.IncludeDirs) {
			module.FlagExcluded = false
		} else {
			module.FlagExcluded = true
		}
	}

	// Mark all affected dependencies as included before proceeding if not in strict include mode.
	if !opts.StrictInclude {
		for _, module := range modules {
			if !module.FlagExcluded {
				for _, dependency := range module.Dependencies {
					dependency.FlagExcluded = false
				}
			}
		}
	}

	return modules
}

// flagUnitsThatAreIncluded iterates over a module slice and flags all modules that include at least one file in
// the specified include list on the TerragruntOptions ModulesThatInclude attribute.
func flagUnitsThatAreIncluded(opts *options.TerragruntOptions, modules common.Units) (common.Units, error) {
	// The two flags ModulesThatInclude and UnitsReading should both be considered when determining which
	// units to include in the run queue.
	unitsThatInclude := append(opts.ModulesThatInclude, opts.UnitsReading...) //nolint:gocritic

	// If no unitsThatInclude is specified return the modules list instantly
	if len(unitsThatInclude) == 0 {
		return modules, nil
	}

	modulesThatIncludeCanonicalPaths := []string{}

	for _, includePath := range unitsThatInclude {
		canonicalPath, err := util.CanonicalPath(includePath, opts.WorkingDir)
		if err != nil {
			return nil, err
		}

		modulesThatIncludeCanonicalPaths = append(modulesThatIncludeCanonicalPaths, canonicalPath)
	}

	for _, module := range modules {
		for _, includeConfig := range module.Config.ProcessedIncludes {
			// resolve include config to canonical path to compare with modulesThatIncludeCanonicalPath
			// https://github.com/gruntwork-io/terragrunt/issues/1944
			canonicalPath, err := util.CanonicalPath(includeConfig.Path, module.Path)
			if err != nil {
				return nil, err
			}

			if util.ListContainsElement(modulesThatIncludeCanonicalPaths, canonicalPath) {
				module.FlagExcluded = false
			}
		}

		// Also search module dependencies and exclude if the dependency path doesn't include any of the specified
		// paths, using a similar logic.
		for _, dependency := range module.Dependencies {
			if dependency.FlagExcluded {
				continue
			}

			for _, includeConfig := range dependency.Config.ProcessedIncludes {
				canonicalPath, err := util.CanonicalPath(includeConfig.Path, module.Path)
				if err != nil {
					return nil, err
				}

				if util.ListContainsElement(modulesThatIncludeCanonicalPaths, canonicalPath) {
					dependency.FlagExcluded = false
				}
			}
		}
	}

	return modules, nil
}

// flagExcludedUnits iterates over a module slice and flags all modules that are excluded based on the exclude block.
func flagExcludedUnits(l log.Logger, opts *options.TerragruntOptions, modules common.Units) common.Units {
	for _, module := range modules {
		excludeConfig := module.Config.Exclude

		if excludeConfig == nil {
			continue
		}

		if !excludeConfig.IsActionListed(opts.TerraformCommand) {
			continue
		}

		if excludeConfig.If {
			l.Debugf("Module %s is excluded by exclude block", module.Path)
			module.FlagExcluded = true
		}

		if excludeConfig.ExcludeDependencies != nil && *excludeConfig.ExcludeDependencies {
			l.Debugf("Excluding dependencies for module %s by exclude block", module.Path)

			for _, dependency := range module.Dependencies {
				dependency.FlagExcluded = true
			}
		}
	}

	return modules
}

// flagUnitsThatRead iterates over a module slice and flags all modules that read at least one file in the specified
// file list in the TerragruntOptions UnitsReading attribute.
func flagUnitsThatRead(opts *options.TerragruntOptions, modules common.Units) common.Units {
	// If no UnitsThatRead is specified return the modules list instantly
	if len(opts.UnitsReading) == 0 {
		return modules
	}

	for _, path := range opts.UnitsReading {
		if !filepath.IsAbs(path) {
			path = filepath.Join(opts.WorkingDir, path)
			path = filepath.Clean(path)
		}

		for _, module := range modules {
			if opts.DidReadFile(path, module.Path) {
				module.FlagExcluded = false
			}
		}
	}

	return modules
}

// flagExcludedDirs iterates over a module slice and flags all entries as excluded listed in the queue-exclude-dir CLI flag.
func flagExcludedDirs(l log.Logger, opts *options.TerragruntOptions, r *report.Report, modules common.Units) common.Units {
	// If we don't have any excludes, we don't need to do anything.
	if len(opts.ExcludeDirs) == 0 {
		return modules
	}

	for _, module := range modules {
		if module.FindModuleInPath(opts.ExcludeDirs) {
			// Mark module itself as excluded
			module.FlagExcluded = true

			if opts.Experiments.Evaluate(experiment.Report) {
				// TODO: Make an upsert option for ends,
				// so that I don't have to do this every time.
				run, err := r.GetRun(module.Path)
				if err != nil {
					run, err = report.NewRun(module.Path)
					if err != nil {
						l.Errorf("Error creating run for unit %s: %v", module.Path, err)

						continue
					}

					if err := r.AddRun(run); err != nil {
						l.Errorf("Error adding run for unit %s: %v", module.Path, err)

						continue
					}
				}

				if err := r.EndRun(
					run.Path,
					report.WithResult(report.ResultExcluded),
					report.WithReason(report.ReasonExcludeDir),
				); err != nil {
					l.Errorf("Error ending run for unit %s: %v", module.Path, err)

					continue
				}
			}
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range module.Dependencies {
			if dependency.FindModuleInPath(opts.ExcludeDirs) {
				dependency.FlagExcluded = true

				if opts.Experiments.Evaluate(experiment.Report) {
					run, err := r.GetRun(dependency.Path)
					if err != nil {
						return modules
					}

					if err := r.EndRun(
						run.Path,
						report.WithResult(report.ResultExcluded),
						report.WithReason(report.ReasonExcludeDir),
					); err != nil {
						return modules
					}
				}
			}
		}
	}

	return modules
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
