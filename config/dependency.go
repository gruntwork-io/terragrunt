package config

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/cli/tfsource"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const renderJsonCommand = "render-json"

type Dependency struct {
	Name                                string     `hcl:",label" cty:"name"`
	ConfigPath                          string     `hcl:"config_path,attr" cty:"config_path"`
	SkipOutputs                         *bool      `hcl:"skip_outputs,attr" cty:"skip"`
	MockOutputs                         *cty.Value `hcl:"mock_outputs,attr" cty:"mock_outputs"`
	MockOutputsAllowedTerraformCommands *[]string  `hcl:"mock_outputs_allowed_terraform_commands,attr" cty:"mock_outputs_allowed_terraform_commands"`

	// MockOutputsMergeWithState is deprecated. Use MockOutputsMergeStrategyWithState
	MockOutputsMergeWithState         *bool              `hcl:"mock_outputs_merge_with_state,attr" cty:"mock_outputs_merge_with_state"`
	MockOutputsMergeStrategyWithState *MergeStrategyType `hcl:"mock_outputs_merge_strategy_with_state" cty:"mock_outputs_merge_strategy_with_state"`

	// Used to store the rendered outputs for use when the config is imported or read with `read_terragrunt_config`
	RenderedOutputs *cty.Value `cty:"outputs"`
}

// DeepMerge will deep merge two Dependency configs, updating the target. Deep merge for Dependency configs is defined
// as follows:
// - For simple attributes (bools and strings), the source will override the target.
// - For MockOutputs, the two maps will be deeply merged together. This means that maps are recursively merged, while
//   lists are concatenated together.
// - For MockOutputsAllowedTerraformCommands, the source will be concatenated to the target.
// Note that RenderedOutputs is ignored in the deep merge operation.
func (targetDepConfig *Dependency) DeepMerge(sourceDepConfig Dependency) error {
	if sourceDepConfig.ConfigPath != "" {
		targetDepConfig.ConfigPath = sourceDepConfig.ConfigPath
	}

	if sourceDepConfig.SkipOutputs != nil {
		targetDepConfig.SkipOutputs = sourceDepConfig.SkipOutputs
	}

	if sourceDepConfig.MockOutputs != nil {
		if targetDepConfig.MockOutputs == nil {
			targetDepConfig.MockOutputs = sourceDepConfig.MockOutputs
		} else {
			newMockOutputs, err := deepMergeCtyMaps(*targetDepConfig.MockOutputs, *sourceDepConfig.MockOutputs)
			if err != nil {
				return err
			}
			targetDepConfig.MockOutputs = newMockOutputs
		}
	}

	if sourceDepConfig.MockOutputsAllowedTerraformCommands != nil {
		if targetDepConfig.MockOutputsAllowedTerraformCommands == nil {
			targetDepConfig.MockOutputsAllowedTerraformCommands = sourceDepConfig.MockOutputsAllowedTerraformCommands
		} else {
			mergedCmds := append(*targetDepConfig.MockOutputsAllowedTerraformCommands, *sourceDepConfig.MockOutputsAllowedTerraformCommands...)
			targetDepConfig.MockOutputsAllowedTerraformCommands = &mergedCmds
		}
	}

	return nil
}

// getMockOutputsMergeStrategy returns the MergeStrategyType following the deprecation of mock_outputs_merge_with_state
// - If mock_outputs_merge_strategy_with_state is not null. The value of mock_outputs_merge_strategy_with_state will be returned
// - If mock_outputs_merge_strategy_with_state is null and mock_outputs_merge_with_state is not null:
//   - mock_outputs_merge_with_state being true returns ShallowMerge
//   - mock_outputs_merge_with_state being false returns NoMerge
func (dependencyConfig Dependency) getMockOutputsMergeStrategy() MergeStrategyType {
	if dependencyConfig.MockOutputsMergeStrategyWithState == nil {
		if dependencyConfig.MockOutputsMergeWithState != nil && (*dependencyConfig.MockOutputsMergeWithState) {
			return ShallowMerge
		} else {
			return NoMerge
		}
	}
	return *dependencyConfig.MockOutputsMergeStrategyWithState
}

// Given a dependency config, we should only attempt to get the outputs if SkipOutputs is nil or false
func (dependencyConfig Dependency) shouldGetOutputs() bool {
	return dependencyConfig.SkipOutputs == nil || !(*dependencyConfig.SkipOutputs)
}

// Given a dependency config, we should only attempt to merge mocks outputs with the outputs if MockOutputsMergeWithState is not nil or true
func (dependencyConfig Dependency) shouldMergeMockOutputsWithState(terragruntOptions *options.TerragruntOptions) bool {
	allowedCommand :=
		dependencyConfig.MockOutputsAllowedTerraformCommands == nil ||
			len(*dependencyConfig.MockOutputsAllowedTerraformCommands) == 0 ||
			util.ListContainsElement(*dependencyConfig.MockOutputsAllowedTerraformCommands, terragruntOptions.OriginalTerraformCommand)
	return allowedCommand && dependencyConfig.getMockOutputsMergeStrategy() != NoMerge
}

func (dependencyConfig *Dependency) setRenderedOutputs(terragruntOptions *options.TerragruntOptions) error {
	if dependencyConfig == nil {
		return nil
	}

	if (*dependencyConfig).shouldGetOutputs() || (*dependencyConfig).shouldReturnMockOutputs(terragruntOptions) {
		outputVal, err := getTerragruntOutputIfAppliedElseConfiguredDefault(*dependencyConfig, terragruntOptions)
		if err != nil {
			return err
		}
		dependencyConfig.RenderedOutputs = outputVal
	}
	return nil
}

// jsonOutputCache is a map that maps config paths to the outputs so that they can be reused across calls for common
// modules. We use sync.Map to ensure atomic updates during concurrent access.
var jsonOutputCache = sync.Map{}

// outputLocks is a map that maps config paths to mutex locks to ensure we only have a single instance of terragrunt
// output running for a given dependent config. We use sync.Map to ensure atomic updates during concurrent access.
var outputLocks = sync.Map{}

// Decode the dependency blocks from the file, and then retrieve all the outputs from the remote state. Then encode the
// resulting map as a cty.Value object.
// TODO: In the future, consider allowing importing dependency blocks from included config
// NOTE FOR MAINTAINER: When implementing importation of other config blocks (e.g referencing inputs), carefully
//                      consider whether or not the implementation of the cyclic dependency detection still makes sense.
func decodeAndRetrieveOutputs(
	file *hcl.File,
	filename string,
	terragruntOptions *options.TerragruntOptions,
	trackInclude *TrackInclude,
	extensions EvalContextExtensions,
) (*cty.Value, error) {
	decodedDependency := terragruntDependency{}
	if err := decodeHcl(file, filename, &decodedDependency, terragruntOptions, extensions); err != nil {
		return nil, err
	}

	// Merge in included dependencies
	if trackInclude != nil {
		mergedDecodedDependency, err := handleIncludeForDependency(decodedDependency, trackInclude, terragruntOptions)
		if err != nil {
			return nil, err
		}
		decodedDependency = *mergedDecodedDependency
	}

	if err := checkForDependencyBlockCycles(filename, decodedDependency, terragruntOptions); err != nil {
		return nil, err
	}
	return dependencyBlocksToCtyValue(decodedDependency.Dependencies, terragruntOptions)
}

// Convert the list of parsed Dependency blocks into a list of module dependencies. Each output block should
// become a dependency of the current config, since that module has to be applied before we can read the output.
func dependencyBlocksToModuleDependencies(decodedDependencyBlocks []Dependency) *ModuleDependencies {
	if len(decodedDependencyBlocks) == 0 {
		return nil
	}

	paths := []string{}
	for _, decodedDependencyBlock := range decodedDependencyBlocks {
		configPath := decodedDependencyBlock.ConfigPath
		if util.IsFile(configPath) && filepath.Base(configPath) == DefaultTerragruntConfigPath {
			// dependencies system expects the directory containing the terragrunt.hcl file
			configPath = filepath.Dir(configPath)
		}
		paths = append(paths, configPath)
	}
	return &ModuleDependencies{Paths: paths}
}

// Check for cyclic dependency blocks to avoid infinite `terragrunt output` loops. To avoid reparsing the config, we
// kickstart the initial loop using what we already decoded.
func checkForDependencyBlockCycles(filename string, decodedDependency terragruntDependency, terragruntOptions *options.TerragruntOptions) error {
	visitedPaths := []string{}
	currentTraversalPaths := []string{filename}
	for _, dependency := range decodedDependency.Dependencies {
		dependencyPath := getCleanedTargetConfigPath(dependency.ConfigPath, filename)
		dependencyOptions := cloneTerragruntOptionsForDependency(terragruntOptions, dependencyPath)
		if err := checkForDependencyBlockCyclesUsingDFS(dependencyPath, &visitedPaths, &currentTraversalPaths, dependencyOptions); err != nil {
			return err
		}
	}
	return nil
}

// Helper function for checkForDependencyBlockCycles.
//
// Same implementation as configstack/graph.go:checkForCyclesUsingDepthFirstSearch, except walks the graph of
// dependencies by `dependency` blocks (which make explicit `terragrunt output` calls) instead of explicit dependencies.
func checkForDependencyBlockCyclesUsingDFS(
	currentConfigPath string,
	visitedPaths *[]string,
	currentTraversalPaths *[]string,
	terragruntOptions *options.TerragruntOptions,
) error {
	if util.ListContainsElement(*visitedPaths, currentConfigPath) {
		return nil
	}

	if util.ListContainsElement(*currentTraversalPaths, currentConfigPath) {
		return errors.WithStackTrace(DependencyCycle(append(*currentTraversalPaths, currentConfigPath)))
	}

	*currentTraversalPaths = append(*currentTraversalPaths, currentConfigPath)
	dependencyPaths, err := getDependencyBlockConfigPathsByFilepath(currentConfigPath, terragruntOptions)
	if err != nil {
		return err
	}
	for _, dependency := range dependencyPaths {
		nextPath := getCleanedTargetConfigPath(dependency, currentConfigPath)
		nextOptions := cloneTerragruntOptionsForDependency(terragruntOptions, nextPath)
		if err := checkForDependencyBlockCyclesUsingDFS(nextPath, visitedPaths, currentTraversalPaths, nextOptions); err != nil {
			return err
		}
	}

	*visitedPaths = append(*visitedPaths, currentConfigPath)
	*currentTraversalPaths = util.RemoveElementFromList(*currentTraversalPaths, currentConfigPath)

	return nil
}

// Given the config path, return the list of config paths that are specified as dependency blocks in the config
func getDependencyBlockConfigPathsByFilepath(configPath string, terragruntOptions *options.TerragruntOptions) ([]string, error) {
	// This will automatically parse everything needed to parse the dependency block configs, and load them as
	// TerragruntConfig.Dependencies. Note that since we aren't passing in `DependenciesBlock` to the
	// PartialDecodeSectionType list, the Dependencies attribute will not include any dependencies specified via the
	// dependencies block.
	tgConfig, err := PartialParseConfigFile(configPath, terragruntOptions, nil, []PartialDecodeSectionType{DependencyBlock})
	if err != nil {
		return nil, err
	}
	if tgConfig.Dependencies == nil {
		return []string{}, nil
	}
	return tgConfig.Dependencies.Paths, nil
}

// Encode the list of dependency blocks into a single cty.Value object that maps the dependency block name to the
// encoded dependency mapping. The encoded dependency mapping should have the attributes:
// - outputs: The map of outputs of the corresponding terraform module that lives at the target config of the
//            dependency.
// This routine will go through the process of obtaining the outputs using `terragrunt output` from the target config.
func dependencyBlocksToCtyValue(dependencyConfigs []Dependency, terragruntOptions *options.TerragruntOptions) (*cty.Value, error) {
	paths := []string{}

	// dependencyMap is the top level map that maps dependency block names to the encoded version, which includes
	// various attributes for accessing information about the target config (including the module outputs).
	dependencyMap := map[string]cty.Value{}
	lock := sync.Mutex{}
	dependencyErrGroup, _ := errgroup.WithContext(context.Background())

	for _, dependencyConfig := range dependencyConfigs {
		dependencyConfig := dependencyConfig // https://golang.org/doc/faq#closures_and_goroutines
		dependencyErrGroup.Go(func() error {
			// Loose struct to hold the attributes of the dependency. This includes:
			// - outputs: The module outputs of the target config
			dependencyEncodingMap := map[string]cty.Value{}

			// Encode the outputs and nest under `outputs` attribute if we should get the outputs or the `mock_outputs`
			if err := dependencyConfig.setRenderedOutputs(terragruntOptions); err != nil {
				return err
			}
			if dependencyConfig.RenderedOutputs != nil {
				paths = append(paths, dependencyConfig.ConfigPath)
				dependencyEncodingMap["outputs"] = *dependencyConfig.RenderedOutputs
			}

			// Once the dependency is encoded into a map, we need to convert to a cty.Value again so that it can be fed to
			// the higher order dependency map.
			dependencyEncodingMapEncoded, err := gocty.ToCtyValue(dependencyEncodingMap, generateTypeFromValuesMap(dependencyEncodingMap))
			if err != nil {
				err = TerragruntOutputListEncodingError{Paths: paths, Err: err}
				return err
			}

			// Lock the map as only one goroutine should be writing to the map at a time
			lock.Lock()
			defer lock.Unlock()

			// Finally, feed the encoded dependency into the higher order map under the block name
			dependencyMap[dependencyConfig.Name] = dependencyEncodingMapEncoded
			return nil
		})
	}

	if err := dependencyErrGroup.Wait(); err != nil {
		return nil, err
	}

	// We need to convert the value map to a single cty.Value at the end so that it can be used in the execution context
	convertedOutput, err := gocty.ToCtyValue(dependencyMap, generateTypeFromValuesMap(dependencyMap))
	if err != nil {
		err = TerragruntOutputListEncodingError{Paths: paths, Err: err}
	}
	return &convertedOutput, errors.WithStackTrace(err)
}

// This will attempt to get the outputs from the target terragrunt config if it is applied. If it is not applied, the
// behavior is different depending on the configuration of the dependency:
// - If the dependency block indicates a mock_outputs attribute, this will return that.
//   If the dependency block indicates a mock_outputs_merge_strategy_with_state attribute, mock_outputs and state outputs will be merged following the merge strategy
// - If the dependency block does NOT indicate a mock_outputs attribute, this will return an error.
func getTerragruntOutputIfAppliedElseConfiguredDefault(dependencyConfig Dependency, terragruntOptions *options.TerragruntOptions) (*cty.Value, error) {
	if dependencyConfig.shouldGetOutputs() {
		outputVal, isEmpty, err := getTerragruntOutput(dependencyConfig, terragruntOptions)
		if err != nil {
			return nil, err
		}

		if !isEmpty && dependencyConfig.shouldMergeMockOutputsWithState(terragruntOptions) {
			mockMergeStrategy := dependencyConfig.getMockOutputsMergeStrategy()
			switch mockMergeStrategy {
			case NoMerge:
				return outputVal, nil
			case ShallowMerge:
				return shallowMergeCtyMaps(*outputVal, *dependencyConfig.MockOutputs)
			case DeepMergeMapOnly:
				return deepMergeCtyMapsMapOnly(*dependencyConfig.MockOutputs, *outputVal)
			default:
				return nil, errors.WithStackTrace(InvalidMergeStrategyType(mockMergeStrategy))
			}

		} else if !isEmpty {
			return outputVal, err
		}
	}

	// When we get no output, it can be an indication that either the module has no outputs or the module is not
	// applied. In either case, check if there are default output values to return. If yes, return that. Else,
	// return error.
	targetConfig := getCleanedTargetConfigPath(dependencyConfig.ConfigPath, terragruntOptions.TerragruntConfigPath)
	currentConfig := terragruntOptions.TerragruntConfigPath
	if dependencyConfig.shouldReturnMockOutputs(terragruntOptions) {
		terragruntOptions.Logger.Debugf("WARNING: config %s is a dependency of %s that has no outputs, but mock outputs provided and returning those in dependency output.",
			targetConfig,
			currentConfig,
		)
		return dependencyConfig.MockOutputs, nil
	}

	// At this point, we expect outputs to exist because there is a `dependency` block without skip_outputs = true, and
	// returning mocks is not allowed. So return a useful error message indicating that we expected outputs, but they
	// did not exist.
	err := TerragruntOutputTargetNoOutputs{
		targetConfig:  targetConfig,
		currentConfig: currentConfig,
	}
	return nil, err
}

// We should only return default outputs if the mock_outputs attribute is set, and if we are running one of the
// allowed commands when `mock_outputs_allowed_terraform_commands` is set as well.
func (dependencyConfig Dependency) shouldReturnMockOutputs(terragruntOptions *options.TerragruntOptions) bool {
	defaultOutputsSet := dependencyConfig.MockOutputs != nil
	allowedCommand :=
		dependencyConfig.MockOutputsAllowedTerraformCommands == nil ||
			len(*dependencyConfig.MockOutputsAllowedTerraformCommands) == 0 ||
			util.ListContainsElement(*dependencyConfig.MockOutputsAllowedTerraformCommands, terragruntOptions.OriginalTerraformCommand)
	return defaultOutputsSet && allowedCommand || isRenderJsonCommand(terragruntOptions)
}

// Return the output from the state of another module, managed by terragrunt. This function will parse the provided
// terragrunt config and extract the desired output from the remote state. Note that this will error if the targetted
// module hasn't been applied yet.
func getTerragruntOutput(dependencyConfig Dependency, terragruntOptions *options.TerragruntOptions) (*cty.Value, bool, error) {

	// target config check: make sure the target config exists
	targetConfig := getCleanedTargetConfigPath(dependencyConfig.ConfigPath, terragruntOptions.TerragruntConfigPath)
	if !util.FileExists(targetConfig) {
		return nil, true, errors.WithStackTrace(DependencyConfigNotFound{Path: targetConfig})
	}

	jsonBytes, err := getOutputJsonWithCaching(targetConfig, terragruntOptions)
	if err != nil {
		if !isRenderJsonCommand(terragruntOptions) {
			return nil, true, err
		}
		terragruntOptions.Logger.Warnf("Failed to read outputs from %s referenced in %s as %s, fallback to mock outputs. Error: %v", targetConfig, terragruntOptions.TerragruntConfigPath, dependencyConfig.Name, err)
		jsonBytes, err = json.Marshal(dependencyConfig.MockOutputs)
		if err != nil {
			return nil, true, err
		}
	}
	isEmpty := string(jsonBytes) == "{}"

	outputMap, err := terraformOutputJsonToCtyValueMap(targetConfig, jsonBytes)
	if err != nil {
		return nil, isEmpty, err
	}

	// We need to convert the value map to a single cty.Value at the end for use in the terragrunt config.
	convertedOutput, err := gocty.ToCtyValue(outputMap, generateTypeFromValuesMap(outputMap))
	if err != nil {
		err = TerragruntOutputEncodingError{Path: targetConfig, Err: err}
	}
	return &convertedOutput, isEmpty, errors.WithStackTrace(err)
}

// This function will true if terragrunt was invoked with renderJsonCommand
func isRenderJsonCommand(terragruntOptions *options.TerragruntOptions) bool {
	return util.ListContainsElement(terragruntOptions.TerraformCliArgs, renderJsonCommand)
}

// getOutputJsonWithCaching will run terragrunt output on the target config if it is not already cached.
func getOutputJsonWithCaching(targetConfig string, terragruntOptions *options.TerragruntOptions) ([]byte, error) {
	// Acquire synchronization lock to ensure only one instance of output is called per config.
	rawActualLock, _ := outputLocks.LoadOrStore(targetConfig, &sync.Mutex{})
	actualLock := rawActualLock.(*sync.Mutex)
	defer actualLock.Unlock()
	actualLock.Lock()

	// This debug log is useful for validating if the locking mechanism is working. If the locking mechanism is working,
	// we should only see one pair of logs at a time that begin with this statement, and then the relevant "terraform
	// output" log for the dependency.
	terragruntOptions.Logger.Debugf("Getting output of dependency %s for config %s", targetConfig, terragruntOptions.TerragruntConfigPath)

	// Look up if we have already run terragrunt output for this target config
	rawJsonBytes, hasRun := jsonOutputCache.Load(targetConfig)
	if hasRun {
		// Cache hit, so return cached output
		terragruntOptions.Logger.Debugf("%s was run before. Using cached output.", targetConfig)
		return rawJsonBytes.([]byte), nil
	}

	// Cache miss, so look up the output and store in cache
	newJsonBytes, err := getTerragruntOutputJson(terragruntOptions, targetConfig)
	if err != nil {
		return nil, err
	}
	jsonOutputCache.Store(targetConfig, newJsonBytes)
	return newJsonBytes, nil
}

// Whenever executing a dependency module, we clone the original options, and reset:
//
// - The config path to the dependency module's config
// - The original config path to the dependency module's config
//
// That way, everything in that dependnecy happens within its own context.
func cloneTerragruntOptionsForDependency(terragruntOptions *options.TerragruntOptions, targetConfig string) *options.TerragruntOptions {
	targetOptions := terragruntOptions.Clone(targetConfig)
	targetOptions.OriginalTerragruntConfigPath = targetConfig
	// Clear IAMRoleOptions in case if it is different from one passed through CLI to allow dependencies to define own iam roles
	// https://github.com/gruntwork-io/terragrunt/issues/1853#issuecomment-940102676
	if targetOptions.IAMRoleOptions != targetOptions.OriginalIAMRoleOptions {
		targetOptions.IAMRoleOptions = options.IAMRoleOptions{}
	}
	return targetOptions
}

// Clone terragrunt options and update context for dependency block so that the outputs can be read correctly
func cloneTerragruntOptionsForDependencyOutput(terragruntOptions *options.TerragruntOptions, targetConfig string) (*options.TerragruntOptions, error) {
	targetOptions := cloneTerragruntOptionsForDependency(terragruntOptions, targetConfig)
	targetOptions.TerraformCommand = "output"
	targetOptions.TerraformCliArgs = []string{"output", "-json"}

	// DownloadDir needs to be updated to be in the context of the new config, if using default
	_, originalDefaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	// Using default, so compute new download dir and update
	if terragruntOptions.DownloadDir == originalDefaultDownloadDir {
		_, downloadDir, err := options.DefaultWorkingAndDownloadDirs(targetConfig)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		targetOptions.DownloadDir = downloadDir
	}

	// Validate and use TerragruntVersionConstraints.TerraformBinary for dependency
	partialTerragruntConfig, err := PartialParseConfigFile(
		targetConfig,
		targetOptions,
		nil,
		[]PartialDecodeSectionType{TerragruntVersionConstraints},
	)
	if err != nil {
		return nil, err
	}
	if partialTerragruntConfig.TerraformBinary != "" {
		targetOptions.TerraformPath = partialTerragruntConfig.TerraformBinary
	}

	// If the Source is set, then we need to recompute it in the context of the target config.
	if terragruntOptions.Source != "" {
		// We need the terraform source of the target config to compute the actual source to use
		partialParseIncludedConfig, err := PartialParseConfigFile(
			targetConfig,
			targetOptions,
			nil,
			[]PartialDecodeSectionType{TerraformBlock},
		)
		if err != nil {
			return nil, err
		}
		// Update the source value to be everything before "//" so that it can be recomputed
		moduleUrl, _ := getter.SourceDirSubdir(terragruntOptions.Source)

		// Finally, update the source to be the combined path between the terraform source in the target config, and the
		// value before "//" in the original terragrunt options.
		targetSource, err := GetTerragruntSourceForModule(moduleUrl, filepath.Dir(targetConfig), partialParseIncludedConfig)
		if err != nil {
			return nil, err
		}
		targetOptions.Source = targetSource
	}

	return targetOptions, nil
}

// Retrieve the outputs from the terraform state in the target configuration. This attempts to optimize the output
// retrieval if the following conditions are true:
// - State backends are managed with a `remote_state` block.
// - The `remote_state` block does not depend on any `dependency` outputs.
// If these conditions are met, terragrunt can optimize the retrieval to avoid recursively retrieving dependency outputs
// by directly pulling down the state file. Otherwise, terragrunt will fallback to running `terragrunt output` on the
// target module.
func getTerragruntOutputJson(terragruntOptions *options.TerragruntOptions, targetConfig string) ([]byte, error) {
	// Make a copy of the terragruntOptions so that we can reuse the same execution environment, but in the context of
	// the target config.
	targetTGOptions, err := cloneTerragruntOptionsForDependencyOutput(terragruntOptions, targetConfig)
	if err != nil {
		return nil, err
	}

	// First attempt to parse the `remote_state` blocks without parsing/getting dependency outputs. If this is possible,
	// proceed to routine that fetches remote state directly. Otherwise, fallback to calling `terragrunt output`
	// directly.
	remoteStateTGConfig, err := PartialParseConfigFile(targetConfig, targetTGOptions, nil, []PartialDecodeSectionType{RemoteStateBlock, TerragruntFlags})
	if err != nil || !canGetRemoteState(remoteStateTGConfig.RemoteState) {
		terragruntOptions.Logger.Debugf("Could not parse remote_state block from target config %s", targetConfig)
		terragruntOptions.Logger.Debugf("Falling back to terragrunt output.")
		return runTerragruntOutputJson(targetTGOptions, targetConfig)
	}

	// In optimization mode, see if there is already an init-ed folder that terragrunt can use, and if so, run
	// `terraform output` in the working directory.
	isInit, workingDir, err := terragruntAlreadyInit(targetTGOptions, targetConfig)
	if err != nil {
		return nil, err
	}
	if isInit {
		return getTerragruntOutputJsonFromInitFolder(targetTGOptions, workingDir, remoteStateTGConfig.GetIAMRoleOptions())
	}
	return getTerragruntOutputJsonFromRemoteState(targetTGOptions, targetConfig, remoteStateTGConfig.RemoteState, remoteStateTGConfig.GetIAMRoleOptions())
}

// canGetRemoteState returns true if the remote state block is not nil and dependency optimization is not disabled
func canGetRemoteState(remoteState *remote.RemoteState) bool {
	return remoteState != nil && !remoteState.DisableDependencyOptimization
}

// terragruntAlreadyInit returns true if it detects that the module specified by the given terragrunt configuration is
// already initialized with the terraform source. This will also return the working directory where you can run
// terraform.
func terragruntAlreadyInit(terragruntOptions *options.TerragruntOptions, configPath string) (bool, string, error) {
	// We need to first determine the working directory where the terraform source should be located. This is dependent
	// on the source field of the terraform block in the config.
	terraformBlockTGConfig, err := PartialParseConfigFile(configPath, terragruntOptions, nil, []PartialDecodeSectionType{TerraformSource})
	if err != nil {
		return false, "", err
	}
	var workingDir string
	sourceUrl, err := GetTerraformSourceUrl(terragruntOptions, terraformBlockTGConfig)
	if err != nil {
		return false, "", err
	}
	if sourceUrl == "" || sourceUrl == "." {
		// When there is no source URL, there is no download process and the working dir is the same as the directory
		// where the config is.
		if util.IsDir(configPath) {
			workingDir = configPath
		} else {
			workingDir = filepath.Dir(configPath)
		}
	} else {
		terraformSource, err := tfsource.NewTerraformSource(sourceUrl, terragruntOptions.DownloadDir, terragruntOptions.WorkingDir, terragruntOptions.Logger)
		if err != nil {
			return false, "", err
		}
		// We're only interested in the computed working dir.
		workingDir = terraformSource.WorkingDir
	}
	// Terragrunt is already init-ed if the terraform state dir (.terraform) exists in the working dir.
	// NOTE: if the ref changes, the workingDir would be different as the download dir includes a base64 encoded hash of
	// the source URL with ref. This would ensure that this routine would not return true if the new ref is not already
	// init-ed.
	return util.FileExists(filepath.Join(workingDir, ".terraform")), workingDir, nil
}

// getTerragruntOutputJsonFromInitFolder will retrieve the outputs directly from the module's working directory without
// running init.
func getTerragruntOutputJsonFromInitFolder(terragruntOptions *options.TerragruntOptions, terraformWorkingDir string, iamRoleOpts options.IAMRoleOptions) ([]byte, error) {
	targetConfig := terragruntOptions.TerragruntConfigPath

	terragruntOptions.Logger.Debugf("Detected module %s is already init-ed. Retrieving outputs directly from working directory.", targetConfig)

	targetTGOptions, err := setupTerragruntOptionsForBareTerraform(terragruntOptions, terraformWorkingDir, targetConfig, iamRoleOpts)
	if err != nil {
		return nil, err
	}
	out, err := shell.RunTerraformCommandWithOutput(targetTGOptions, "output", "-json")
	if err != nil {
		return nil, err
	}
	jsonString := out.Stdout
	jsonBytes := []byte(strings.TrimSpace(jsonString))
	terragruntOptions.Logger.Debugf("Retrieved output from %s as json: %s", targetConfig, jsonString)
	return jsonBytes, nil
}

// getTerragruntOutputJsonFromRemoteState will retrieve the outputs directly by using just the remote state block. This
// uses terraform's feature where `output` and `init` can work without the real source, as long as you have the
// `backend` configured.
// To do this, this function will:
// - Create a temporary folder
// - Generate the backend.tf file with the backend configuration from the remote_state block
// - Run terraform init and terraform output
// - Clean up folder once json file is generated
// NOTE: terragruntOptions should be in the context of the targetConfig already.
func getTerragruntOutputJsonFromRemoteState(
	terragruntOptions *options.TerragruntOptions,
	targetConfig string,
	remoteState *remote.RemoteState,
	iamRoleOpts options.IAMRoleOptions,
) ([]byte, error) {
	terragruntOptions.Logger.Debugf("Detected remote state block with generate config. Resolving dependency by pulling remote state.")
	// Create working directory where we will run terraform in. We will create the temporary directory in the download
	// directory for consistency with other file generation capabilities of terragrunt. Make sure it is cleaned up
	// before the function returns.
	if err := util.EnsureDirectory(terragruntOptions.DownloadDir); err != nil {
		return nil, err
	}
	tempWorkDir, err := ioutil.TempDir(terragruntOptions.DownloadDir, "")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempWorkDir)
	terragruntOptions.Logger.Debugf("Setting dependency working directory to %s", tempWorkDir)

	targetTGOptions, err := setupTerragruntOptionsForBareTerraform(terragruntOptions, tempWorkDir, targetConfig, iamRoleOpts)
	if err != nil {
		return nil, err
	}

	// To speed up dependencies processing it is possible to retrieve its output directly from the backend without init dependencies
	if terragruntOptions.FetchDependencyOutputFromState {
		switch backend := remoteState.Backend; backend {
		case "s3":
			jsonBytes, err := getTerragruntOutputJsonFromRemoteStateS3(
				targetTGOptions,
				remoteState,
			)
			if err != nil {
				return nil, err
			}
			terragruntOptions.Logger.Debugf("Retrieved output from %s as json: %s using s3 bucket", targetConfig, jsonBytes)
			return jsonBytes, nil
		default:
			terragruntOptions.Logger.Errorf("FetchDependencyOutputFromState is not supported for backend %s, falling back to normal method", backend)
		}
	}

	// Generate the backend configuration in the working dir. If no generate config is set on the remote state block,
	// set a temporary generate config so we can generate the backend code.
	if remoteState.Generate == nil {
		remoteState.Generate = &remote.RemoteStateGenerate{
			Path:     "backend.tf",
			IfExists: codegen.ExistsOverwriteTerragruntStr,
		}
	}
	if err := remoteState.GenerateTerraformCode(targetTGOptions); err != nil {
		return nil, err
	}
	terragruntOptions.Logger.Debugf("Generated remote state configuration in working dir %s", tempWorkDir)

	// The working directory is now set up to interact with the state, so pull it down to get the json output.

	// First run init to setup the backend configuration so that we can run output.
	runTerraformInitForDependencyOutput(targetTGOptions, tempWorkDir, targetConfig)

	// Now that the backend is initialized, run terraform output to get the data and return it.
	out, err := shell.RunTerraformCommandWithOutput(targetTGOptions, "output", "-json")
	if err != nil {
		return nil, err
	}
	jsonString := out.Stdout
	jsonBytes := []byte(strings.TrimSpace(jsonString))
	terragruntOptions.Logger.Debugf("Retrieved output from %s as json: %s", targetConfig, jsonString)

	return jsonBytes, nil

}

// getTerragruntOutputJsonFromRemoteStateS3 pulls the output directly from an S3 bucket without calling Terraform
func getTerragruntOutputJsonFromRemoteStateS3(
	terragruntOptions *options.TerragruntOptions,
	remoteState *remote.RemoteState,
) ([]byte, error) {
	terragruntOptions.Logger.Debugf("Fetching outputs directly from s3://%s/%s", remoteState.Config["bucket"], remoteState.Config["key"])

	s3ConfigExtended, err := remote.ParseExtendedS3Config(remoteState.Config)
	if err != nil {
		return nil, err
	}

	sessionConfig := s3ConfigExtended.GetAwsSessionConfig()

	s3Client, err := remote.CreateS3Client(sessionConfig, terragruntOptions)
	if err != nil {
		return nil, err
	}

	result, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(fmt.Sprintf("%s", remoteState.Config["bucket"])),
		Key:    aws.String(fmt.Sprintf("%s", remoteState.Config["key"])),
	})

	if err != nil {
		return nil, err
	}

	defer result.Body.Close()
	steateBody, err := ioutil.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}
	jsonState := string(steateBody)
	jsonMap := make(map[string]interface{})
	err = json.Unmarshal([]byte(jsonState), &jsonMap)
	if err != nil {
		return nil, err
	}
	jsonOutputs, err := json.Marshal(jsonMap["outputs"])
	if err != nil {
		return nil, err
	}
	return jsonOutputs, nil
}

// setupTerragruntOptionsForBareTerraform sets up a new TerragruntOptions struct that can be used to run terraform
// without going through the full RunTerragrunt operation.
func setupTerragruntOptionsForBareTerraform(originalOptions *options.TerragruntOptions, workingDir string, configPath string, iamRoleOpts options.IAMRoleOptions) (*options.TerragruntOptions, error) {
	// Here we clone the terragrunt options again since we need to make further modifications to it to allow running
	// terraform directly.
	// Set the terraform working dir to the tempdir, and set stdout writer to ioutil.Discard so that output content is
	// not logged.
	targetTGOptions := cloneTerragruntOptionsForDependency(originalOptions, configPath)
	targetTGOptions.WorkingDir = workingDir
	targetTGOptions.Writer = ioutil.Discard

	// If the target config has an IAM role directive and it was not set on the command line, set it to
	// the one we retrieved from the config.
	targetTGOptions.IAMRoleOptions = options.MergeIAMRoleOptions(iamRoleOpts, targetTGOptions.OriginalIAMRoleOptions)

	// Make sure to assume any roles set by TERRAGRUNT_IAM_ROLE
	if err := aws_helper.AssumeRoleAndUpdateEnvIfNecessary(targetTGOptions); err != nil {
		return nil, err
	}
	return targetTGOptions, nil
}

// runTerragruntOutputJson uses terragrunt running functions to extract the json output from the target config.
// NOTE: targetTGOptions should be in the context of the targetConfig.
func runTerragruntOutputJson(targetTGOptions *options.TerragruntOptions, targetConfig string) ([]byte, error) {
	// Update the stdout buffer so we can capture the output
	var stdoutBuffer bytes.Buffer
	stdoutBufferWriter := bufio.NewWriter(&stdoutBuffer)
	targetTGOptions.Writer = stdoutBufferWriter

	err := targetTGOptions.RunTerragrunt(targetTGOptions)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	stdoutBufferWriter.Flush()
	jsonString := stdoutBuffer.String()
	jsonBytes := []byte(strings.TrimSpace(jsonString))
	targetTGOptions.Logger.Debugf("Retrieved output from %s as json: %s", targetConfig, jsonString)
	return jsonBytes, nil
}

// terraformOutputJsonToCtyValueMap takes the terraform output json and converts to a mapping between output keys to the
// parsed cty.Value encoding of the json objects.
func terraformOutputJsonToCtyValueMap(targetConfig string, jsonBytes []byte) (map[string]cty.Value, error) {
	// When getting all outputs, terraform returns a json with the data containing metadata about the types, so we
	// can't quite return the data directly. Instead, we will need further processing to get the output we want.
	// To do so, we first Unmarshal the json into a simple go map to a OutputMeta struct.
	type OutputMeta struct {
		Sensitive bool            `json:"sensitive"`
		Type      json.RawMessage `json:"type"`
		Value     json.RawMessage `json:"value"`
	}
	var outputs map[string]OutputMeta
	err := json.Unmarshal(jsonBytes, &outputs)
	if err != nil {
		return nil, errors.WithStackTrace(TerragruntOutputParsingError{Path: targetConfig, Err: err})
	}
	flattenedOutput := map[string]cty.Value{}
	for k, v := range outputs {
		outputType, err := ctyjson.UnmarshalType(v.Type)
		if err != nil {
			return nil, errors.WithStackTrace(TerragruntOutputParsingError{Path: targetConfig, Err: err})
		}
		outputVal, err := ctyjson.Unmarshal(v.Value, outputType)
		if err != nil {
			return nil, errors.WithStackTrace(TerragruntOutputParsingError{Path: targetConfig, Err: err})
		}
		flattenedOutput[k] = outputVal
	}
	return flattenedOutput, nil
}

// ClearOutputCache clears the output cache. Useful during testing.
func ClearOutputCache() {
	jsonOutputCache = sync.Map{}
}

// runTerraformInitForDependencyOutput will run terraform init in a mode that doesn't pull down plugins or modules. Note
// that this will cause the command to fail for most modules as terraform init does a validation check to make sure the
// plugins are available, even though we don't need it for our purposes (terraform output does not depend on any of the
// plugins being available). As such this command will ignore errors in the init command.
// To help with debuggability, the errors will be printed to the console when TG_LOG=debug is set.
func runTerraformInitForDependencyOutput(terragruntOptions *options.TerragruntOptions, workingDir string, targetConfig string) {
	stderr := bytes.Buffer{}
	initTGOptions := cloneTerragruntOptionsForDependency(terragruntOptions, targetConfig)
	initTGOptions.WorkingDir = workingDir
	initTGOptions.ErrWriter = &stderr
	err := shell.RunTerraformCommand(initTGOptions, "init", "-get=false")
	if err != nil {
		terragruntOptions.Logger.Debugf("Ignoring expected error from dependency init call")
		terragruntOptions.Logger.Debugf("Init call stderr:")
		terragruntOptions.Logger.Debugf(stderr.String())
	}
}

// Custom error types

type DependencyConfigNotFound struct {
	Path string
}

func (err DependencyConfigNotFound) Error() string {
	return fmt.Sprintf("%s does not exist", err.Path)
}

type TerragruntOutputParsingError struct {
	Path string
	Err  error
}

func (err TerragruntOutputParsingError) Error() string {
	return fmt.Sprintf("Could not parse output from terragrunt config %s. Underlying error: %s", err.Path, err.Err)
}

type TerragruntOutputEncodingError struct {
	Path string
	Err  error
}

func (err TerragruntOutputEncodingError) Error() string {
	return fmt.Sprintf("Could not encode output from terragrunt config %s. Underlying error: %s", err.Path, err.Err)
}

type TerragruntOutputListEncodingError struct {
	Paths []string
	Err   error
}

func (err TerragruntOutputListEncodingError) Error() string {
	return fmt.Sprintf("Could not encode output from list of terragrunt configs %v. Underlying error: %s", err.Paths, err.Err)
}

type TerragruntOutputTargetNoOutputs struct {
	targetConfig  string
	currentConfig string
}

func (err TerragruntOutputTargetNoOutputs) Error() string {
	return fmt.Sprintf(
		"%s is a dependency of %s but detected no outputs. Either the target module has not been applied yet, or the module has no outputs. If this is expected, set the skip_outputs flag to true on the dependency block.",
		err.targetConfig,
		err.currentConfig,
	)
}

type DependencyCycle []string

func (err DependencyCycle) Error() string {
	return fmt.Sprintf("Found a dependency cycle between modules: %s", strings.Join([]string(err), " -> "))
}
