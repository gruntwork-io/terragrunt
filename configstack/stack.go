package configstack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/terraform"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/sirupsen/logrus"
)

// Represents a stack of Terraform modules (i.e. folders with Terraform templates) that you can "spin up" or
// "spin down" in a single command
type Stack struct {
	parserOptions         []hclparse.Option
	terragruntOptions     *options.TerragruntOptions
	childTerragruntConfig *config.TerragruntConfig
	Modules               TerraformModules
}

// Find all the Terraform modules in the subfolders of the working directory of the given TerragruntOptions and
// assemble them into a Stack object that can be applied or destroyed in a single command
func FindStackInSubfolders(ctx context.Context, terragruntOptions *options.TerragruntOptions, opts ...Option) (*Stack, error) {
	var terragruntConfigFiles []string

	err := telemetry.Telemetry(ctx, terragruntOptions, "find_files_in_path", map[string]interface{}{
		"working_dir": terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		result, err := config.FindConfigFilesInPath(terragruntOptions.WorkingDir, terragruntOptions)
		if err != nil {
			return err
		}
		terragruntConfigFiles = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	stack := NewStack(terragruntOptions, opts...)
	if err := stack.createStackForTerragruntConfigPaths(ctx, terragruntConfigFiles); err != nil {
		return nil, err
	}
	return stack, nil
}

func NewStack(terragruntOptions *options.TerragruntOptions, opts ...Option) *Stack {
	stack := &Stack{
		terragruntOptions: terragruntOptions,
		parserOptions:     config.DefaultParserOptions(terragruntOptions),
	}
	return stack.WithOptions(opts...)
}

func (stack *Stack) WithOptions(opts ...Option) *Stack {
	for _, opt := range opts {
		*stack = opt(*stack)
	}
	return stack
}

// Render this stack as a human-readable string
func (stack *Stack) String() string {
	modules := []string{}
	for _, module := range stack.Modules {
		modules = append(modules, "  => "+module.String())
	}
	sort.Strings(modules)
	return fmt.Sprintf("Stack at %s:\n%s", stack.terragruntOptions.WorkingDir, strings.Join(modules, "\n"))
}

// LogModuleDeployOrder will log the modules that will be deployed by this operation, in the order that the operations
// happen. For plan and apply, the order will be bottom to top (dependencies first), while for destroy the order will be
// in reverse.
func (stack *Stack) LogModuleDeployOrder(logger *logrus.Entry, terraformCommand string) error {
	outStr := fmt.Sprintf("The stack at %s will be processed in the following order for command %s:\n", stack.terragruntOptions.WorkingDir, terraformCommand)
	runGraph, err := stack.GetModuleRunGraph(terraformCommand)
	if err != nil {
		return err
	}
	for i, group := range runGraph {
		outStr += fmt.Sprintf("Group %d\n", i+1)
		for _, module := range group {
			outStr += fmt.Sprintf("- Module %s\n", module.Path)
		}
		outStr += "\n"
	}
	logger.Info(outStr)
	return nil
}

// JsonModuleDeployOrder will return the modules that will be deployed by a plan/apply operation, in the order
// that the operations happen.
func (stack *Stack) JsonModuleDeployOrder(terraformCommand string) (string, error) {
	runGraph, err := stack.GetModuleRunGraph(terraformCommand)
	if err != nil {
		return "", errors.WithStackTrace(err)
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
		return "", errors.WithStackTrace(err)
	}
	return string(j), nil
}

// Graph creates a graphviz representation of the modules
func (stack *Stack) Graph(terragruntOptions *options.TerragruntOptions) {
	err := stack.Modules.WriteDot(terragruntOptions.Writer, terragruntOptions)
	if err != nil {
		terragruntOptions.Logger.Warnf("Failed to graph dot: %v", err)
	}
}

func (stack *Stack) Run(ctx context.Context, terragruntOptions *options.TerragruntOptions) error {
	stackCmd := terragruntOptions.TerraformCommand

	// prepare folder for output hierarchy if output folder is set
	if terragruntOptions.OutputFolder != "" {
		for _, module := range stack.Modules {
			planFile := module.outputFile(terragruntOptions)
			planDir := filepath.Dir(planFile)
			if err := os.MkdirAll(planDir, os.ModePerm); err != nil {
				return err
			}
		}
	}

	// For any command that needs input, run in non-interactive mode to avoid cominglint stdin across multiple
	// concurrent runs.
	if util.ListContainsElement(config.TERRAFORM_COMMANDS_NEED_INPUT, stackCmd) {
		// to support potential positional args in the args list, we append the input=false arg after the first element,
		// which is the target command.
		terragruntOptions.TerraformCliArgs = util.StringListInsert(terragruntOptions.TerraformCliArgs, "-input=false", 1)
		stack.syncTerraformCliArgs(terragruntOptions)
	}

	// For apply and destroy, run with auto-approve (unless explicitly disabled) due to the co-mingling of the prompts.
	// This is not ideal, but until we have a better way of handling interactivity with run-all, we take the evil of
	// having a global prompt (managed in cli/cli_app.go) be the gate keeper.
	switch stackCmd {
	case terraform.CommandNameApply, terraform.CommandNameDestroy:
		// to support potential positional args in the args list, we append the input=false arg after the first element,
		// which is the target command.
		if terragruntOptions.RunAllAutoApprove {
			terragruntOptions.TerraformCliArgs = util.StringListInsert(terragruntOptions.TerraformCliArgs, "-auto-approve", 1)
		}
		stack.syncTerraformCliArgs(terragruntOptions)
	case terraform.CommandNameShow:
		stack.syncTerraformCliArgs(terragruntOptions)
	case terraform.CommandNamePlan:
		// We capture the out stream for each module
		errorStreams := make([]bytes.Buffer, len(stack.Modules))
		for n, module := range stack.Modules {
			if !terragruntOptions.NonInteractive { // redirect output to ErrWriter in case of not NonInteractive mode
				module.TerragruntOptions.ErrWriter = io.MultiWriter(&errorStreams[n], module.TerragruntOptions.ErrWriter)
			} else {
				module.TerragruntOptions.ErrWriter = &errorStreams[n]
			}
		}
		defer stack.summarizePlanAllErrors(terragruntOptions, errorStreams)
	}

	switch {
	case terragruntOptions.IgnoreDependencyOrder:
		return stack.Modules.RunModulesIgnoreOrder(ctx, terragruntOptions, terragruntOptions.Parallelism)
	case stackCmd == terraform.CommandNameDestroy:
		return stack.Modules.RunModulesReverseOrder(ctx, terragruntOptions, terragruntOptions.Parallelism)
	default:
		return stack.Modules.RunModules(ctx, terragruntOptions, terragruntOptions.Parallelism)
	}
}

// We inspect the error streams to give an explicit message if the plan failed because there were references to
// remote states. `terraform plan` will fail if it tries to access remote state from dependencies and the plan
// has never been applied on the dependency.
func (stack *Stack) summarizePlanAllErrors(terragruntOptions *options.TerragruntOptions, errorStreams []bytes.Buffer) {
	for i, errorStream := range errorStreams {
		output := errorStream.String()

		if len(output) == 0 {
			// We get empty buffer if stack execution completed without errors, so skip that to avoid logging too much
			continue
		}

		terragruntOptions.Logger.Infoln(output)
		if strings.Contains(output, "Error running plan:") {
			if strings.Contains(output, ": Resource 'data.terraform_remote_state.") {
				var dependenciesMsg string
				if len(stack.Modules[i].Dependencies) > 0 {
					dependenciesMsg = fmt.Sprintf(" contains dependencies to %v and", stack.Modules[i].Config.Dependencies.Paths)
				}
				terragruntOptions.Logger.Infof("%v%v refers to remote state "+
					"you may have to apply your changes in the dependencies prior running terragrunt run-all plan.\n",
					stack.Modules[i].Path,
					dependenciesMsg,
				)
			}
		}
	}
}

// Sync the TerraformCliArgs for each module in the stack to match the provided terragruntOptions struct.
func (stack *Stack) syncTerraformCliArgs(terragruntOptions *options.TerragruntOptions) {
	for _, module := range stack.Modules {
		module.TerragruntOptions.TerraformCliArgs = collections.MakeCopyOfList(terragruntOptions.TerraformCliArgs)

		planFile := module.planFile(terragruntOptions)

		if planFile != "" {
			terragruntOptions.Logger.Debugf("Using output file %s for module %s", planFile, module.TerragruntOptions.TerragruntConfigPath)
			if module.TerragruntOptions.TerraformCommand == terraform.CommandNamePlan {
				// for plan command add -out=<file> to the terraform cli args
				module.TerragruntOptions.TerraformCliArgs = util.StringListInsert(module.TerragruntOptions.TerraformCliArgs, "-out="+planFile, len(module.TerragruntOptions.TerraformCliArgs))
			} else {
				module.TerragruntOptions.TerraformCliArgs = util.StringListInsert(module.TerragruntOptions.TerraformCliArgs, planFile, len(module.TerragruntOptions.TerraformCliArgs))
			}
		}
	}
}

func (stack *Stack) toRunningModules(terraformCommand string) (RunningModules, error) {
	switch terraformCommand {
	case terraform.CommandNameDestroy:
		return stack.Modules.ToRunningModules(ReverseOrder)
	default:
		return stack.Modules.ToRunningModules(NormalOrder)
	}
}

// GetModuleRunGraph converts the module list to a graph that shows the order in which the modules will be
// applied/destroyed. The return structure is a list of lists, where the nested list represents modules that can be
// deployed concurrently, and the outer list indicates the order. This will only include those modules that do NOT have
// the exclude flag set.
func (stack *Stack) GetModuleRunGraph(terraformCommand string) ([]TerraformModules, error) {
	moduleRunGraph, err := stack.toRunningModules(terraformCommand)
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
func (stack *Stack) createStackForTerragruntConfigPaths(ctx context.Context, terragruntConfigPaths []string) error {
	err := telemetry.Telemetry(ctx, stack.terragruntOptions, "create_stack_for_terragrunt_config_paths", map[string]interface{}{
		"working_dir": stack.terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {

		if len(terragruntConfigPaths) == 0 {
			return errors.WithStackTrace(NoTerraformModulesFound)
		}

		modules, err := stack.ResolveTerraformModules(ctx, terragruntConfigPaths)

		if err != nil {
			return errors.WithStackTrace(err)
		}
		stack.Modules = modules
		return nil
	})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	err = telemetry.Telemetry(ctx, stack.terragruntOptions, "check_for_cycles", map[string]interface{}{
		"working_dir": stack.terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		if err := stack.Modules.CheckForCycles(); err != nil {
			return errors.WithStackTrace(err)
		}
		return nil
	})
	if err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

// Go through each of the given Terragrunt configuration files and resolve the module that configuration file represents
// into a TerraformModule struct. Return the list of these TerraformModule structs.
func (stack *Stack) ResolveTerraformModules(ctx context.Context, terragruntConfigPaths []string) (TerraformModules, error) {
	canonicalTerragruntConfigPaths, err := util.CanonicalPaths(terragruntConfigPaths, ".")
	if err != nil {
		return nil, err
	}

	var modulesMap TerraformModulesMap
	err = telemetry.Telemetry(ctx, stack.terragruntOptions, "resolve_modules", map[string]interface{}{
		"working_dir": stack.terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		howThesePathsWereFound := "Terragrunt config file found in a subdirectory of " + stack.terragruntOptions.WorkingDir
		result, err := stack.resolveModules(ctx, canonicalTerragruntConfigPaths, howThesePathsWereFound)
		if err != nil {
			return err
		}
		modulesMap = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	var externalDependencies TerraformModulesMap
	err = telemetry.Telemetry(ctx, stack.terragruntOptions, "resolve_external_dependencies_for_modules", map[string]interface{}{
		"working_dir": stack.terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		result, err := stack.resolveExternalDependenciesForModules(ctx, modulesMap, TerraformModulesMap{}, 0)
		if err != nil {
			return err
		}
		externalDependencies = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	var crossLinkedModules TerraformModules
	err = telemetry.Telemetry(ctx, stack.terragruntOptions, "crosslink_dependencies", map[string]interface{}{
		"working_dir": stack.terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		result, err := modulesMap.mergeMaps(externalDependencies).crosslinkDependencies(canonicalTerragruntConfigPaths)
		if err != nil {
			return err
		}
		crossLinkedModules = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	var includedModules TerraformModules
	err = telemetry.Telemetry(ctx, stack.terragruntOptions, "flag_included_dirs", map[string]interface{}{
		"working_dir": stack.terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		includedModules = crossLinkedModules.flagIncludedDirs(stack.terragruntOptions)
		return nil
	})
	if err != nil {
		return nil, err
	}

	var includedModulesWithExcluded TerraformModules
	err = telemetry.Telemetry(ctx, stack.terragruntOptions, "flag_excluded_dirs", map[string]interface{}{
		"working_dir": stack.terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		includedModulesWithExcluded = includedModules.flagExcludedDirs(stack.terragruntOptions)
		return nil
	})
	if err != nil {
		return nil, err
	}

	var finalModules TerraformModules
	err = telemetry.Telemetry(ctx, stack.terragruntOptions, "flag_modules_that_dont_include", map[string]interface{}{
		"working_dir": stack.terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		result, err := includedModulesWithExcluded.flagModulesThatDontInclude(stack.terragruntOptions)
		if err != nil {
			return err
		}
		finalModules = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	return finalModules, nil
}

// Go through each of the given Terragrunt configuration files and resolve the module that configuration file represents
// into a TerraformModule struct. Note that this method will NOT fill in the Dependencies field of the TerraformModule
// struct (see the crosslinkDependencies method for that). Return a map from module path to TerraformModule struct.
func (stack *Stack) resolveModules(ctx context.Context, canonicalTerragruntConfigPaths []string, howTheseModulesWereFound string) (TerraformModulesMap, error) {
	modulesMap := TerraformModulesMap{}
	for _, terragruntConfigPath := range canonicalTerragruntConfigPaths {
		if !util.FileExists(terragruntConfigPath) {
			return nil, ProcessingModuleError{UnderlyingError: os.ErrNotExist, ModulePath: terragruntConfigPath, HowThisModuleWasFound: howTheseModulesWereFound}
		}

		var module *TerraformModule
		err := telemetry.Telemetry(ctx, stack.terragruntOptions, "resolve_terraform_module", map[string]interface{}{
			"config_path": terragruntConfigPath,
			"working_dir": stack.terragruntOptions.WorkingDir,
		}, func(childCtx context.Context) error {
			m, err := stack.resolveTerraformModule(ctx, terragruntConfigPath, modulesMap, howTheseModulesWereFound)
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
			var dependencies TerraformModulesMap
			err := telemetry.Telemetry(ctx, stack.terragruntOptions, "resolve_dependencies_for_module", map[string]interface{}{
				"config_path": terragruntConfigPath,
				"working_dir": stack.terragruntOptions.WorkingDir,
				"module_path": module.Path,
			}, func(childCtx context.Context) error {
				deps, err := stack.resolveDependenciesForModule(ctx, module, modulesMap, true)
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

// Create a TerraformModule struct for the Terraform module specified by the given Terragrunt configuration file path.
// Note that this method will NOT fill in the Dependencies field of the TerraformModule struct (see the
// crosslinkDependencies method for that).
func (stack *Stack) resolveTerraformModule(ctx context.Context, terragruntConfigPath string, modulesMap TerraformModulesMap, howThisModuleWasFound string) (*TerraformModule, error) {
	modulePath, err := util.CanonicalPath(filepath.Dir(terragruntConfigPath), ".")
	if err != nil {
		return nil, err
	}

	if _, ok := modulesMap[modulePath]; ok {
		return nil, nil
	}

	// Clone the options struct so we don't modify the original one. This is especially important as run-all operations
	// happen concurrently.
	opts := stack.terragruntOptions.Clone(terragruntConfigPath)

	// We need to reset the original path for each module. Otherwise, this path will be set to wherever you ran run-all
	// from, which is not what any of the modules will want.
	opts.OriginalTerragruntConfigPath = terragruntConfigPath

	// If `childTerragruntConfig.ProcessedIncludes` contains the path `terragruntConfigPath`, then this is a parent config
	// which implies that `TerragruntConfigPath` must refer to a child configuration file, and the defined `IncludeConfig` must contain the path to the file itself
	// for the built-in functions `read-terragrunt-config()`, `path_relative_to_include()` to work correctly.
	var includeConfig *config.IncludeConfig
	if stack.childTerragruntConfig != nil && stack.childTerragruntConfig.ProcessedIncludes.ContainsPath(terragruntConfigPath) {
		includeConfig = &config.IncludeConfig{Path: terragruntConfigPath}
		opts.TerragruntConfigPath = stack.terragruntOptions.OriginalTerragruntConfigPath
	}

	if collections.ListContainsElement(opts.ExcludeDirs, modulePath) {
		// module is excluded
		return &TerraformModule{Path: modulePath, TerragruntOptions: opts, FlagExcluded: true}, nil
	}

	parseCtx := config.NewParsingContext(ctx, opts).
		WithParseOption(stack.parserOptions).
		WithDecodeList(
			// Need for initializing the modules
			config.TerraformSource,

			// Need for parsing out the dependencies
			config.DependenciesBlock,
			config.DependencyBlock,
		)

	// Credentials have to be acquired before the config is parsed, as the config may contain interpolation functions
	// that require credentials to be available.
	credsGetter := creds.NewGetter()
	if err := credsGetter.ObtainAndUpdateEnvIfNecessary(ctx, opts, externalcmd.NewProvider(opts)); err != nil {
		return nil, err
	}

	// We only partially parse the config, only using the pieces that we need in this section. This config will be fully
	// parsed at a later stage right before the action is run. This is to delay interpolation of functions until right
	// before we call out to terraform.

	// TODO: Remove lint suppression
	terragruntConfig, err := config.PartialParseConfigFile( //nolint:contextcheck
		parseCtx,
		terragruntConfigPath,
		includeConfig,
	)
	if err != nil {
		return nil, errors.WithStackTrace(ProcessingModuleError{UnderlyingError: err, HowThisModuleWasFound: howThisModuleWasFound, ModulePath: terragruntConfigPath})
	}

	terragruntSource, err := config.GetTerragruntSourceForModule(stack.terragruntOptions.Source, modulePath, terragruntConfig)
	if err != nil {
		return nil, err
	}
	opts.Source = terragruntSource

	_, defaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(stack.terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	// If we're using the default download directory, put it into the same folder as the Terragrunt configuration file.
	// If we're not using the default, then the user has specified a custom download directory, and we leave it as-is.
	if stack.terragruntOptions.DownloadDir == defaultDownloadDir {
		_, downloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntConfigPath)
		if err != nil {
			return nil, err
		}
		stack.terragruntOptions.Logger.Debugf("Setting download directory for module %s to %s", modulePath, downloadDir)
		opts.DownloadDir = downloadDir
	}

	// Fix for https://github.com/gruntwork-io/terragrunt/issues/208
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(terragruntConfigPath), "*.tf"))
	if err != nil {
		return nil, err
	}
	if (terragruntConfig.Terraform == nil || terragruntConfig.Terraform.Source == nil || *terragruntConfig.Terraform.Source == "") && matches == nil {
		stack.terragruntOptions.Logger.Debugf("Module %s does not have an associated terraform configuration and will be skipped.", filepath.Dir(terragruntConfigPath))
		return nil, nil
	}

	if opts.IncludeModulePrefix {
		opts.OutputPrefix = fmt.Sprintf("[%v] ", modulePath)
	}

	return &TerraformModule{Path: modulePath, Config: *terragruntConfig, TerragruntOptions: opts}, nil
}

// resolveDependenciesForModule looks through the dependencies of the given module and resolve the dependency paths listed in the module's config.
// If `skipExternal` is true, the func returns only dependencies that are inside of the current working directory, which means they are part of the environment the
// user is trying to apply-all or destroy-all. Note that this method will NOT fill in the Dependencies field of the TerraformModule struct (see the crosslinkDependencies method for that).
func (stack *Stack) resolveDependenciesForModule(ctx context.Context, module *TerraformModule, modulesMap TerraformModulesMap, skipExternal bool) (TerraformModulesMap, error) {
	if module.Config.Dependencies == nil || len(module.Config.Dependencies.Paths) == 0 {
		return TerraformModulesMap{}, nil
	}

	key := fmt.Sprintf("%s-%s-%v-%v", module.Path, stack.terragruntOptions.WorkingDir, skipExternal, stack.terragruntOptions.TerraformCommand)
	if value, ok := existingModules.Get(ctx, key); ok {
		return *value, nil
	}

	externalTerragruntConfigPaths := []string{}
	for _, dependency := range module.Config.Dependencies.Paths {
		dependencyPath, err := util.CanonicalPath(dependency, module.Path)
		if err != nil {
			return TerraformModulesMap{}, err
		}

		if skipExternal && !util.HasPathPrefix(dependencyPath, stack.terragruntOptions.WorkingDir) {
			continue
		}

		terragruntConfigPath := config.GetDefaultConfigPath(dependencyPath)

		if _, alreadyContainsModule := modulesMap[dependencyPath]; !alreadyContainsModule {
			externalTerragruntConfigPaths = append(externalTerragruntConfigPaths, terragruntConfigPath)
		}
	}

	howThesePathsWereFound := fmt.Sprintf("dependency of module at '%s'", module.Path)
	result, err := stack.resolveModules(ctx, externalTerragruntConfigPaths, howThesePathsWereFound)
	if err != nil {
		return nil, err
	}

	existingModules.Put(ctx, key, &result)
	return result, nil
}

// Look through the dependencies of the modules in the given map and resolve the "external" dependency paths listed in
// each modules config (i.e. those dependencies not in the given list of Terragrunt config canonical file paths).
// These external dependencies are outside of the current working directory, which means they may not be part of the
// environment the user is trying to apply-all or destroy-all. Therefore, this method also confirms whether the user wants
// to actually apply those dependencies or just assume they are already applied. Note that this method will NOT fill in
// the Dependencies field of the TerraformModule struct (see the crosslinkDependencies method for that).
func (stack *Stack) resolveExternalDependenciesForModules(ctx context.Context, modulesMap, modulesAlreadyProcessed TerraformModulesMap, recursionLevel int) (TerraformModulesMap, error) {
	allExternalDependencies := TerraformModulesMap{}
	modulesToSkip := modulesMap.mergeMaps(modulesAlreadyProcessed)

	// Simple protection from circular dependencies causing a Stack Overflow due to infinite recursion
	if recursionLevel > maxLevelsOfRecursion {
		return allExternalDependencies, errors.WithStackTrace(InfiniteRecursionError{RecursionLevel: maxLevelsOfRecursion, Modules: modulesToSkip})
	}

	sortedKeys := modulesMap.getSortedKeys()
	for _, key := range sortedKeys {
		module := modulesMap[key]
		externalDependencies, err := stack.resolveDependenciesForModule(ctx, module, modulesToSkip, false)
		if err != nil {
			return externalDependencies, err
		}

		for _, externalDependency := range externalDependencies {
			if _, alreadyFound := modulesToSkip[externalDependency.Path]; alreadyFound {
				continue
			}

			shouldApply := false
			if !stack.terragruntOptions.IgnoreExternalDependencies {
				shouldApply, err = module.confirmShouldApplyExternalDependency(externalDependency, stack.terragruntOptions)
				if err != nil {
					return externalDependencies, err
				}
			}

			externalDependency.AssumeAlreadyApplied = !shouldApply
			allExternalDependencies[externalDependency.Path] = externalDependency
		}
	}

	if len(allExternalDependencies) > 0 {
		recursiveDependencies, err := stack.resolveExternalDependenciesForModules(ctx, allExternalDependencies, modulesMap, recursionLevel+1)
		if err != nil {
			return allExternalDependencies, err
		}
		return allExternalDependencies.mergeMaps(recursiveDependencies), nil
	}

	return allExternalDependencies, nil
}

// ListStackDependentModules - build a map with each module and its dependent modules
func (stack *Stack) ListStackDependentModules() map[string][]string {
	// build map of dependent modules
	// module path -> list of dependent modules
	var dependentModules = make(map[string][]string)

	// build initial mapping of dependent modules
	for _, module := range stack.Modules {

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
