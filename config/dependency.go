package config

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

type Dependency struct {
	Name                                string     `hcl:",label" cty:"name"`
	ConfigPath                          string     `hcl:"config_path,attr" cty:"config_path"`
	SkipOutputs                         *bool      `hcl:"skip_outputs,attr" cty:"skip"`
	MockOutputs                         *cty.Value `hcl:"mock_outputs,attr" cty:"mock_outputs"`
	MockOutputsAllowedTerraformCommands *[]string  `hcl:"mock_outputs_allowed_terraform_commands,attr" cty:"mock_outputs_allowed_terraform_commands"`

	// Used to store the rendered outputs for use when the config is imported or read with `read_terragrunt_config`
	RenderedOutputs *cty.Value `cty:"outputs"`
}

// Given a dependency config, we should only attempt to get the outputs if SkipOutputs is nil or false
func (dependencyConfig Dependency) shouldGetOutputs() bool {
	return dependencyConfig.SkipOutputs == nil || !(*dependencyConfig.SkipOutputs)
}

func (dependencyConfig *Dependency) setRenderedOutputs(terragruntOptions *options.TerragruntOptions) error {
	if (*dependencyConfig).shouldGetOutputs() || shouldReturnMockOutputs(*dependencyConfig, terragruntOptions) {
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
	extensions EvalContextExtensions,
) (*cty.Value, error) {
	decodedDependency := terragruntDependency{}
	if err := decodeHcl(file, filename, &decodedDependency, terragruntOptions, extensions); err != nil {
		return nil, err
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
		dependencyOptions := terragruntOptions.Clone(dependencyPath)
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
		nextOptions := terragruntOptions.Clone(nextPath)
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
// - If the dependency block does NOT indicate a mock_outputs attribute, this will return an error.
func getTerragruntOutputIfAppliedElseConfiguredDefault(dependencyConfig Dependency, terragruntOptions *options.TerragruntOptions) (*cty.Value, error) {
	if dependencyConfig.shouldGetOutputs() {
		outputVal, isEmpty, err := getTerragruntOutput(dependencyConfig, terragruntOptions)
		if err != nil {
			return nil, err
		}

		if !isEmpty {
			return outputVal, err
		}
	}

	// When we get no output, it can be an indication that either the module has no outputs or the module is not
	// applied. In either case, check if there are default output values to return. If yes, return that. Else,
	// return error.
	targetConfig := getCleanedTargetConfigPath(dependencyConfig.ConfigPath, terragruntOptions.TerragruntConfigPath)
	currentConfig := terragruntOptions.TerragruntConfigPath
	if shouldReturnMockOutputs(dependencyConfig, terragruntOptions) {
		util.Debugf(
			terragruntOptions.Logger,
			"WARNING: config %s is a dependency of %s that has no outputs, but mock outputs provided and returning those in dependency output.",
			targetConfig,
			currentConfig,
		)
		return dependencyConfig.MockOutputs, nil
	}

	err := TerragruntOutputTargetNoOutputs{
		targetConfig:  targetConfig,
		currentConfig: currentConfig,
	}
	return nil, err
}

// We should only return default outputs if the mock_outputs attribute is set, and if we are running one of the
// allowed commands when `mock_outputs_allowed_terraform_commands` is set as well.
func shouldReturnMockOutputs(dependencyConfig Dependency, terragruntOptions *options.TerragruntOptions) bool {
	defaultOutputsSet := dependencyConfig.MockOutputs != nil
	allowedCommand :=
		dependencyConfig.MockOutputsAllowedTerraformCommands == nil ||
			len(*dependencyConfig.MockOutputsAllowedTerraformCommands) == 0 ||
			util.ListContainsElement(*dependencyConfig.MockOutputsAllowedTerraformCommands, terragruntOptions.TerraformCommand)
	return defaultOutputsSet && allowedCommand
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
		return nil, true, err
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
	util.Debugf(terragruntOptions.Logger, "Getting output of dependency %s for config %s", targetConfig, terragruntOptions.TerragruntConfigPath)

	// Look up if we have already run terragrunt output for this target config
	rawJsonBytes, hasRun := jsonOutputCache.Load(targetConfig)
	if hasRun {
		// Cache hit, so return cached output
		util.Debugf(terragruntOptions.Logger, "%s was run before. Using cached output.", targetConfig)
		return rawJsonBytes.([]byte), nil
	}

	// Cache miss, so look up the output and store in cache
	newJsonBytes, err := runTerragruntOutputJson(terragruntOptions, targetConfig)
	if err != nil {
		return nil, err
	}
	jsonOutputCache.Store(targetConfig, newJsonBytes)
	return newJsonBytes, nil
}

// Clone terragrunt options and update context for dependency block so that the outputs can be read correctly
func cloneTerragruntOptionsForDependencyOutput(terragruntOptions *options.TerragruntOptions, targetConfig string) (*options.TerragruntOptions, error) {
	targetOptions := terragruntOptions.Clone(targetConfig)
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

	return targetOptions, nil
}

// runTerragruntOutputJson uses terragrunt running functions to extract the json output from the target config. Make a
// copy of the terragruntOptions so that we can reuse the same execution environment.
func runTerragruntOutputJson(terragruntOptions *options.TerragruntOptions, targetConfig string) ([]byte, error) {
	targetOptions, err := cloneTerragruntOptionsForDependencyOutput(terragruntOptions, targetConfig)
	if err != nil {
		return nil, err
	}

	// Update the stdout buffer so we can capture the output
	var stdoutBuffer bytes.Buffer
	stdoutBufferWriter := bufio.NewWriter(&stdoutBuffer)
	targetOptions.Writer = stdoutBufferWriter

	err = targetOptions.RunTerragrunt(targetOptions)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	stdoutBufferWriter.Flush()
	jsonString := stdoutBuffer.String()
	jsonBytes := []byte(strings.TrimSpace(jsonString))
	util.Debugf(terragruntOptions.Logger, "Retrieved output from %s as json: %s", targetConfig, jsonString)
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
