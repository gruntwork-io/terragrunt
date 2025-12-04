package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"

	"github.com/hashicorp/go-getter"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	renderJSONCommand = "render-json"
	renderCommand     = "render"
)

type Dependencies []Dependency

// Struct to hold the decoded dependency blocks.
type dependencyOutputCache struct {
	Enabled *bool
	Inputs  cty.Value
}

type Dependency struct {
	ConfigPath                          cty.Value  `hcl:"config_path,attr" cty:"config_path"`
	Enabled                             *bool      `hcl:"enabled,attr" cty:"enabled"`
	SkipOutputs                         *bool      `hcl:"skip_outputs,attr" cty:"skip"`
	MockOutputs                         *cty.Value `hcl:"mock_outputs,attr" cty:"mock_outputs"`
	MockOutputsAllowedTerraformCommands *[]string  `hcl:"mock_outputs_allowed_terraform_commands,attr" cty:"mock_outputs_allowed_terraform_commands"`

	// MockOutputsMergeWithState is deprecated. Use MockOutputsMergeStrategyWithState
	MockOutputsMergeWithState *bool `hcl:"mock_outputs_merge_with_state,attr" cty:"mock_outputs_merge_with_state"`

	MockOutputsMergeStrategyWithState *MergeStrategyType `hcl:"mock_outputs_merge_strategy_with_state" cty:"mock_outputs_merge_strategy_with_state"`

	// Used to store the rendered outputs for use when the config is imported or read with `read_terragrunt_config`
	RenderedOutputs *cty.Value `cty:"outputs"`

	Inputs *cty.Value `cty:"inputs"`
	Name   string     `hcl:",label" cty:"name"`
}

// DeepMerge will deep merge two Dependency configs, updating the target. Deep merge for Dependency configs is defined
// as follows:
//   - For simple attributes (bools and strings), the source will override the target.
//   - For MockOutputs, the two maps will be deeply merged together. This means that maps are recursively merged, while
//     lists are concatenated together.
//   - For MockOutputsAllowedTerraformCommands, the source will be concatenated to the target.
//
// Note that RenderedOutputs is ignored in the deep merge operation.
func (dep *Dependency) DeepMerge(sourceDepConfig Dependency) error {
	if sourceDepConfig.ConfigPath.AsString() != "" {
		dep.ConfigPath = sourceDepConfig.ConfigPath
	}

	if sourceDepConfig.Enabled != nil {
		dep.Enabled = sourceDepConfig.Enabled
	}

	if sourceDepConfig.SkipOutputs != nil {
		dep.SkipOutputs = sourceDepConfig.SkipOutputs
	}

	if sourceDepConfig.MockOutputs != nil {
		if dep.MockOutputs == nil {
			dep.MockOutputs = sourceDepConfig.MockOutputs
		} else {
			newMockOutputs, err := deepMergeCtyMaps(*dep.MockOutputs, *sourceDepConfig.MockOutputs)
			if err != nil {
				return err
			}

			dep.MockOutputs = newMockOutputs
		}
	}

	if sourceDepConfig.MockOutputsAllowedTerraformCommands != nil {
		if dep.MockOutputsAllowedTerraformCommands == nil {
			dep.MockOutputsAllowedTerraformCommands = sourceDepConfig.MockOutputsAllowedTerraformCommands
		} else {
			mergedCmds := append(*dep.MockOutputsAllowedTerraformCommands, *sourceDepConfig.MockOutputsAllowedTerraformCommands...)
			dep.MockOutputsAllowedTerraformCommands = &mergedCmds
		}
	}

	return nil
}

// getMockOutputsMergeStrategy returns the MergeStrategyType following the deprecation of mock_outputs_merge_with_state
// - If mock_outputs_merge_strategy_with_state is not null. The value of mock_outputs_merge_strategy_with_state will be returned
// - If mock_outputs_merge_strategy_with_state is null and mock_outputs_merge_with_state is not null:
//   - mock_outputs_merge_with_state being true returns ShallowMerge
//   - mock_outputs_merge_with_state being false returns NoMerge
func (dep Dependency) getMockOutputsMergeStrategy() MergeStrategyType {
	if dep.MockOutputsMergeStrategyWithState == nil {
		if dep.MockOutputsMergeWithState != nil && (*dep.MockOutputsMergeWithState) {
			return ShallowMerge
		} else {
			return NoMerge
		}
	}

	return *dep.MockOutputsMergeStrategyWithState
}

// Given a dependency config, we should only attempt to get the outputs if SkipOutputs is nil or false
func (dep Dependency) shouldGetOutputs(ctx *ParsingContext) bool {
	return !ctx.TerragruntOptions.SkipOutput && dep.isEnabled() && (dep.SkipOutputs == nil || !*dep.SkipOutputs)
}

// isEnabled returns true if the dependency is enabled
func (dep Dependency) isEnabled() bool {
	if dep.Enabled == nil {
		return true
	}

	return *dep.Enabled
}

// isDisabled returns true if the dependency is disabled
func (dep Dependency) isDisabled() bool {
	return !dep.isEnabled()
}

// Given a dependency config, we should only attempt to merge mocks outputs with the outputs if MockOutputsMergeWithState is not nil or true
func (dep Dependency) shouldMergeMockOutputsWithState(ctx *ParsingContext) bool {
	allowedCommand :=
		dep.MockOutputsAllowedTerraformCommands == nil ||
			len(*dep.MockOutputsAllowedTerraformCommands) == 0 ||
			util.ListContainsElement(*dep.MockOutputsAllowedTerraformCommands, ctx.TerragruntOptions.OriginalTerraformCommand)

	return allowedCommand && dep.getMockOutputsMergeStrategy() != NoMerge
}

func (dep *Dependency) setRenderedOutputs(ctx *ParsingContext, l log.Logger) error {
	if dep == nil {
		return nil
	}

	if dep.shouldGetOutputs(ctx) || dep.shouldReturnMockOutputs(ctx) {
		outputVal, err := getTerragruntOutputIfAppliedElseConfiguredDefault(ctx, l, *dep)
		if err != nil {
			return err
		}

		dep.RenderedOutputs = outputVal
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
//
//	consider whether or not the implementation of the cyclic dependency detection still makes sense.
func decodeAndRetrieveOutputs(ctx *ParsingContext, l log.Logger, file *hclparse.File) (*cty.Value, error) {
	evalParsingContext, err := createTerragruntEvalContext(ctx, l, file.ConfigPath)
	if err != nil {
		return nil, err
	}

	decodedDependency := TerragruntDependency{}
	if err := file.Decode(&decodedDependency, evalParsingContext); err != nil {
		return nil, err
	}

	// In normal operation, if a dependency block does not have a `config_path` attribute, decoding returns an error since this attribute is required, but the `hclvalidate` command suppresses decoding errors and this causes a cycle between modules, so we need to filter out dependencies without a defined `config_path`.
	decodedDependency.Dependencies = decodedDependency.Dependencies.FilteredWithoutConfigPath()

	// Validate that dependency config_path is not an empty string.
	// Skip null/unknown values and non-strings (which can appear during partial decode or hclvalidate).
	for _, dep := range decodedDependency.Dependencies {
		if dep.isDisabled() {
			continue
		}

		if isEmptyKnownString(dep.ConfigPath) {
			return nil, fmt.Errorf("dependency %q has empty config_path in %s; set a non-empty config_path or disable the dependency", dep.Name, file.ConfigPath)
		}
	}

	if err := checkForDependencyBlockCycles(ctx, l, file.ConfigPath, decodedDependency); err != nil {
		return nil, err
	}

	updatedDependencies, err := decodeDependencies(ctx, l, decodedDependency)
	if err != nil {
		return nil, err
	}

	decodedDependency = *updatedDependencies

	// Merge in included dependencies
	if ctx.TrackInclude != nil {
		mergedDecodedDependency, err := handleIncludeForDependency(ctx, l, decodedDependency)
		if err != nil {
			return nil, err
		}

		decodedDependency = *mergedDecodedDependency
	}

	return dependencyBlocksToCtyValue(ctx, l, decodedDependency.Dependencies)
}

// decodeDependencies decode dependencies and fetch inputs
func decodeDependencies(ctx *ParsingContext, l log.Logger, decodedDependency TerragruntDependency) (*TerragruntDependency, error) {
	updatedDependencies := TerragruntDependency{}
	depCache := cache.ContextCache[*dependencyOutputCache](ctx, DependencyOutputCacheContextKey)

	for _, dep := range decodedDependency.Dependencies {
		depPath := getCleanedTargetConfigPath(dep.ConfigPath.AsString(), ctx.TerragruntOptions.TerragruntConfigPath)
		if dep.isEnabled() && util.FileExists(depPath) {
			cacheKey := ctx.TerragruntOptions.WorkingDir + depPath

			cachedDependency, found := depCache.Get(ctx, cacheKey)
			if !found {
				l, depOpts, err := cloneTerragruntOptionsForDependency(ctx, l, depPath)
				if err != nil {
					return nil, err
				}

				depCtx := ctx.WithDecodeList(TerragruntFlags).WithTerragruntOptions(depOpts)

				if depConfig, err := PartialParseConfigFile(depCtx, l, depPath, nil); err == nil {
					inputsCty, err := convertToCtyWithJSON(depConfig.Inputs)
					if err != nil {
						return nil, err
					}

					cachedValue := dependencyOutputCache{
						Enabled: dep.Enabled,
						Inputs:  inputsCty,
					}
					depCache.Put(ctx, cacheKey, &cachedValue)

					dep.Inputs = &inputsCty
				} else {
					l.Warnf("Error reading partial config for dependency %s: %v", dep.Name, err)
				}
			} else {
				dep.Enabled = cachedDependency.Enabled
				dep.Inputs = &cachedDependency.Inputs
			}
		}

		updatedDependencies.Dependencies = append(updatedDependencies.Dependencies, dep)
	}

	return &updatedDependencies, nil
}

// Convert the list of parsed Dependency blocks into a list of module dependencies. Each output block should
// become a dependency of the current config, since that module has to be applied before we can read the output.
func dependencyBlocksToModuleDependencies(l log.Logger, decodedDependencyBlocks []Dependency) *ModuleDependencies {
	if len(decodedDependencyBlocks) == 0 {
		return nil
	}

	paths := []string{}

	for _, decodedDependencyBlock := range decodedDependencyBlocks {
		// skip dependency if is not enabled
		if !decodedDependencyBlock.isEnabled() {
			continue
		}

		// Skip if ConfigPath is not a known string value (can happen during discovery phase)
		if decodedDependencyBlock.ConfigPath.IsNull() ||
			!decodedDependencyBlock.ConfigPath.IsWhollyKnown() ||
			!decodedDependencyBlock.ConfigPath.Type().Equals(cty.String) {
			l.Debugf("Skipping dependency %q: ConfigPath is not a valid known string value", decodedDependencyBlock.Name)
			continue
		}

		paths = append(paths, decodedDependencyBlock.ConfigPath.AsString())
	}

	return &ModuleDependencies{Paths: paths}
}

// Check for cyclic dependency blocks to avoid infinite `terragrunt output` loops. To avoid reparsing the config, we
// kickstart the initial loop using what we already decoded.
func checkForDependencyBlockCycles(ctx *ParsingContext, l log.Logger, configPath string, decodedDependency TerragruntDependency) error {
	visitedPaths := []string{}
	currentTraversalPaths := []string{configPath}

	for _, dependency := range decodedDependency.Dependencies {
		if dependency.isDisabled() {
			continue
		}

		dependencyPath := getCleanedTargetConfigPath(dependency.ConfigPath.AsString(), configPath)

		l, dependencyOpts, err := cloneTerragruntOptionsForDependency(ctx, l, dependencyPath)
		if err != nil {
			return err
		}

		dependencyContext := ctx.WithTerragruntOptions(dependencyOpts)
		if err := checkForDependencyBlockCyclesUsingDFS(dependencyContext, l, dependencyPath, &visitedPaths, &currentTraversalPaths); err != nil {
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
	ctx *ParsingContext,
	l log.Logger,
	dependencyPath string,
	visitedPaths *[]string,
	currentTraversalPaths *[]string,
) error {
	if util.ListContainsElement(*visitedPaths, dependencyPath) {
		return nil
	}

	if util.ListContainsElement(*currentTraversalPaths, dependencyPath) {
		return errors.New(DependencyCycleError(append(*currentTraversalPaths, dependencyPath)))
	}

	*currentTraversalPaths = append(*currentTraversalPaths, dependencyPath)

	dependencyPaths, err := getDependencyBlockConfigPathsByFilepath(ctx, l, dependencyPath)
	if err != nil {
		return err
	}

	for _, dependency := range dependencyPaths {
		dependencyPath := getCleanedTargetConfigPath(dependency, dependencyPath)

		l, dependencyOpts, err := cloneTerragruntOptionsForDependency(ctx, l, dependencyPath)
		if err != nil {
			return err
		}

		dependencyContext := ctx.WithTerragruntOptions(dependencyOpts)
		if err := checkForDependencyBlockCyclesUsingDFS(dependencyContext, l, dependencyPath, visitedPaths, currentTraversalPaths); err != nil {
			return err
		}
	}

	*visitedPaths = append(*visitedPaths, dependencyPath)
	*currentTraversalPaths = util.RemoveElementFromList(*currentTraversalPaths, dependencyPath)

	return nil
}

// Given the config path, return the list of config paths that are specified as dependency blocks in the config
func getDependencyBlockConfigPathsByFilepath(ctx *ParsingContext, l log.Logger, configPath string) ([]string, error) {
	// This will automatically parse everything needed to parse the dependency block configs, and load them as
	// TerragruntConfig.Dependencies. Note that since we aren't passing in `DependenciesBlock` to the
	// PartialDecodeSectionType list, the Dependencies attribute will not include any dependencies specified via the
	// dependencies block.
	tgConfig, err := PartialParseConfigFile(ctx.WithDecodeList(DependencyBlock), l, configPath, nil)
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
//   - outputs: The map of outputs of the corresponding terraform module that lives at the target config of the
//     dependency.
//
// This routine will go through the process of obtaining the outputs using `terragrunt output` from the target config.
func dependencyBlocksToCtyValue(ctx *ParsingContext, l log.Logger, dependencyConfigs []Dependency) (*cty.Value, error) {
	paths := []string{}

	// dependencyMap is the top level map that maps dependency block names to the encoded version, which includes
	// various attributes for accessing information about the target config (including the module outputs).
	dependencyMap := map[string]cty.Value{}
	lock := sync.Mutex{}
	dependencyErrGroup, _ := errgroup.WithContext(ctx)

	for _, dependencyConfig := range dependencyConfigs {
		dependencyErrGroup.Go(func() error {
			// Loose struct to hold the attributes of the dependency. This includes:
			// - outputs: The module outputs of the target config
			dependencyEncodingMap := map[string]cty.Value{}

			// Encode the outputs and nest under `outputs` attribute if we should get the outputs or the `mock_outputs`
			if err := dependencyConfig.setRenderedOutputs(ctx, l); err != nil {
				return err
			}

			if dependencyConfig.RenderedOutputs != nil {
				lock.Lock()

				paths = append(paths, dependencyConfig.ConfigPath.AsString())

				lock.Unlock()

				dependencyEncodingMap["outputs"] = *dependencyConfig.RenderedOutputs
			}

			if dependencyConfig.Inputs != nil {
				dependencyEncodingMap["inputs"] = *dependencyConfig.Inputs
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

	// We need to convert the value map to a single cty.Value at the end so that it can be used in the execution ctx
	convertedOutput, err := gocty.ToCtyValue(dependencyMap, generateTypeFromValuesMap(dependencyMap))
	if err != nil {
		err = TerragruntOutputListEncodingError{Paths: paths, Err: err}
	}

	return &convertedOutput, errors.New(err)
}

// This will attempt to get the outputs from the target terragrunt config if it is applied. If it is not applied, the
// behavior is different depending on the configuration of the dependency:
//   - If the dependency block indicates a mock_outputs attribute, this will return that.
//     If the dependency block indicates a mock_outputs_merge_strategy_with_state attribute, mock_outputs and state outputs will be merged following the merge strategy
//   - If the dependency block does NOT indicate a mock_outputs attribute, this will return an error.
func getTerragruntOutputIfAppliedElseConfiguredDefault(ctx *ParsingContext, l log.Logger, dependencyConfig Dependency) (*cty.Value, error) {
	if dependencyConfig.isDisabled() {
		l.Debugf("Skipping outputs reading for disabled dependency %s", dependencyConfig.Name)
		return dependencyConfig.MockOutputs, nil
	}

	if dependencyConfig.shouldGetOutputs(ctx) {
		outputVal, isEmpty, err := getTerragruntOutput(ctx, l, dependencyConfig)
		if err != nil {
			return nil, err
		}

		if !isEmpty && dependencyConfig.shouldMergeMockOutputsWithState(ctx) && dependencyConfig.MockOutputs != nil {
			mockMergeStrategy := dependencyConfig.getMockOutputsMergeStrategy()

			// TODO: Make this exhaustive
			switch mockMergeStrategy { // nolint:exhaustive
			case NoMerge:
				return outputVal, nil
			case ShallowMerge:
				return shallowMergeCtyMaps(*outputVal, *dependencyConfig.MockOutputs)
			case DeepMergeMapOnly:
				return deepMergeCtyMapsMapOnly(*dependencyConfig.MockOutputs, *outputVal)
			default:
				return nil, errors.New(InvalidMergeStrategyTypeError(mockMergeStrategy))
			}
		} else if !isEmpty {
			return outputVal, err
		}
	}

	// When we get no output, it can be an indication that either the module has no outputs or the module is not
	// applied. In either case, check if there are default output values to return. If yes, return that. Else,
	// return error.
	targetConfig := getCleanedTargetConfigPath(dependencyConfig.ConfigPath.AsString(), ctx.TerragruntOptions.TerragruntConfigPath)

	if dependencyConfig.shouldReturnMockOutputs(ctx) {
		l.Warnf("Config %s is a dependency of %s that has no outputs, but mock outputs provided and returning those in dependency output.",
			targetConfig,
			ctx.TerragruntOptions.TerragruntConfigPath,
		)

		return dependencyConfig.MockOutputs, nil
	}

	// At this point, we expect outputs to exist because there is a `dependency` block without skip_outputs = true, and
	// returning mocks is not allowed. So return a useful error message indicating that we expected outputs, but they
	// did not exist.
	err := TerragruntOutputTargetNoOutputs{
		targetName:    dependencyConfig.Name,
		targetPath:    dependencyConfig.ConfigPath.AsString(),
		targetConfig:  targetConfig,
		currentConfig: ctx.TerragruntOptions.TerragruntConfigPath,
	}

	return nil, err
}

// We should only return default outputs if the mock_outputs attribute is set, and if we are running one of the
// allowed commands when `mock_outputs_allowed_terraform_commands` is set as well.
func (dep Dependency) shouldReturnMockOutputs(ctx *ParsingContext) bool {
	if dep.isDisabled() {
		return true
	}

	defaultOutputsSet := dep.MockOutputs != nil

	allowedCommand :=
		dep.MockOutputsAllowedTerraformCommands == nil ||
			len(*dep.MockOutputsAllowedTerraformCommands) == 0 ||
			util.ListContainsElement(*dep.MockOutputsAllowedTerraformCommands, ctx.TerragruntOptions.OriginalTerraformCommand)

	return defaultOutputsSet && allowedCommand || isRenderJSONCommand(ctx) || isRenderCommand(ctx)
}

// Return the output from the state of another module, managed by terragrunt. This function will parse the provided
// terragrunt config and extract the desired output from the remote state. Note that this will error if the targeted
// module hasn't been applied yet.
func getTerragruntOutput(ctx *ParsingContext, l log.Logger, dependencyConfig Dependency) (*cty.Value, bool, error) {
	// target config check: make sure the target config exists
	targetConfigPath := getCleanedTargetConfigPath(dependencyConfig.ConfigPath.AsString(), ctx.TerragruntOptions.TerragruntConfigPath)

	if !util.FileExists(targetConfigPath) {
		return nil, true, errors.New(DependencyConfigNotFound{Path: targetConfigPath})
	}

	jsonBytes, err := getOutputJSONWithCaching(ctx, l, targetConfigPath)
	if err != nil {
		if !isRenderJSONCommand(ctx) && !isRenderCommand(ctx) && !isAwsS3NoSuchKey(err) {
			return nil, true, err
		}

		l.Warnf("Failed to read outputs from %s referenced in %s as %s, fallback to mock outputs. Error: %v", targetConfigPath, ctx.TerragruntOptions.TerragruntConfigPath, dependencyConfig.Name, err)

		jsonBytes, err = json.Marshal(dependencyConfig.MockOutputs)
		if err != nil {
			return nil, true, err
		}
	}

	isEmpty := string(jsonBytes) == "{}"

	outputMap, err := TerraformOutputJSONToCtyValueMap(targetConfigPath, jsonBytes)
	if err != nil {
		return nil, isEmpty, err
	}

	// We need to convert the value map to a single cty.Value at the end for use in the terragrunt config.
	convertedOutput, err := gocty.ToCtyValue(outputMap, generateTypeFromValuesMap(outputMap))
	if err != nil {
		err = TerragruntOutputEncodingError{Path: targetConfigPath, Err: err}
	}

	return &convertedOutput, isEmpty, errors.New(err)
}

func isAwsS3NoSuchKey(err error) bool {
	if err != nil {
		errStr := err.Error()
		return strings.Contains(errStr, "NoSuchKey") || strings.Contains(errStr, "NotFound")
	}

	return false
}

// isRenderJSONCommand This function will true if terragrunt was invoked with render-json
func isRenderJSONCommand(ctx *ParsingContext) bool {
	return util.ListContainsElement(ctx.TerragruntOptions.TerraformCliArgs, renderJSONCommand)
}

// isRenderCommand will return true if terragrunt was invoked with render
func isRenderCommand(ctx *ParsingContext) bool {
	return util.ListContainsElement(ctx.TerragruntOptions.TerraformCliArgs, renderCommand)
}

// getOutputJSONWithCaching will run terragrunt output on the target config if it is not already cached.
func getOutputJSONWithCaching(ctx *ParsingContext, l log.Logger, targetConfig string) ([]byte, error) {
	// Acquire synchronization lock to ensure only one instance of output is called per config.
	rawActualLock, _ := outputLocks.LoadOrStore(targetConfig, &sync.Mutex{})

	actualLock := rawActualLock.(*sync.Mutex)
	defer actualLock.Unlock()

	actualLock.Lock()

	// This debug log is useful for validating if the locking mechanism is working. If the locking mechanism is working,
	// we should only see one pair of logs at a time that begin with this statement, and then the relevant "terraform
	// output" log for the dependency.
	l.Debugf("Getting output of dependency %s for config %s", targetConfig, ctx.TerragruntOptions.TerragruntConfigPath)

	// Look up if we have already run terragrunt output for this target config
	rawJSONBytes, hasRun := jsonOutputCache.Load(targetConfig)
	if hasRun {
		// Cache hit, so return cached output
		l.Debugf("%s was run before. Using cached output.", targetConfig)
		return rawJSONBytes.([]byte), nil
	}

	// Cache miss, so look up the output and store in cache
	newJSONBytes, err := getTerragruntOutputJSON(ctx, l, targetConfig)
	if err != nil {
		return nil, err
	}

	// When AWS Client Side Monitoring (CSM) is enabled the aws-sdk-go displays log as a plaintext "Enabling CSM" to stdout, even if the `output -json` flag is specified. The final output looks like this: "2023/05/04 20:22:43 Enabling CSM{...omitted json string...}", and and prevents proper json parsing. Since there is no way to disable this log, the only way out is to filter.
	// Related AWS code: https://github.com/aws/aws-sdk-go/blob/81d1cbbc6a2028023aff7bcab0fe1be320cd39f7/aws/session/session.go#L444
	// Related issues: https://github.com/gruntwork-io/terragrunt/issues/2233 https://github.com/hashicorp/terraform-provider-aws/issues/23620
	if index := bytes.IndexByte(newJSONBytes, byte('{')); index > 0 {
		newJSONBytes = newJSONBytes[index:]
	}

	jsonOutputCache.Store(targetConfig, newJSONBytes)

	return newJSONBytes, nil
}

// Whenever executing a dependency module, we clone the original options, and reset:
//
// - The config path to the dependency module's config
// - The original config path to the dependency module's config
//
// That way, everything in that dependency happens within its own ctx.
func cloneTerragruntOptionsForDependency(ctx *ParsingContext, l log.Logger, targetConfigPath string) (log.Logger, *options.TerragruntOptions, error) {
	l, targetOptions, err := ctx.TerragruntOptions.CloneWithConfigPath(l, targetConfigPath)
	if err != nil {
		return l, nil, err
	}

	targetOptions.OriginalTerragruntConfigPath = targetConfigPath

	// Always use the dependency's default download directory for parsing
	// dependencies, as the dependency's cache is where its state and modules exist.
	targetOptions.DownloadDir = filepath.Join(filepath.Dir(targetConfigPath), util.TerragruntCacheDir)

	// Clear IAMRoleOptions in case if it is different from one passed through CLI to allow dependencies to define own iam roles
	// https://github.com/gruntwork-io/terragrunt/issues/1853#issuecomment-940102676
	if targetOptions.IAMRoleOptions != targetOptions.OriginalIAMRoleOptions {
		targetOptions.IAMRoleOptions = options.IAMRoleOptions{}
	}

	return l, targetOptions, nil
}

// Clone terragrunt options and update ctx for dependency block so that the outputs can be read correctly
func cloneTerragruntOptionsForDependencyOutput(ctx *ParsingContext, l log.Logger, targetConfig string) (log.Logger, *options.TerragruntOptions, error) {
	l, targetOptions, err := cloneTerragruntOptionsForDependency(ctx, l, targetConfig)
	if err != nil {
		return l, nil, err
	}

	targetOptions.ForwardTFStdout = false
	// just read outputs, so no need to check for dependent modules
	targetOptions.CheckDependentModules = false
	targetOptions.TerraformCommand = "output"
	targetOptions.TerraformCliArgs = []string{"output", "-json"}

	// DownloadDir needs to be the dependency's default download directory
	// because that's where the dependency's state was created when it was applied.
	// We always use the dependency's default since outputs must be read from where
	// the state exists, regardless of any stack-level download directory settings.
	_, downloadDir, err := options.DefaultWorkingAndDownloadDirs(targetConfig)
	if err != nil {
		return l, nil, errors.New(err)
	}

	targetOptions.DownloadDir = downloadDir

	targetParsingContext := ctx.WithTerragruntOptions(targetOptions)
	// Validate and use TerragruntVersionConstraints.TerraformBinary for dependency
	partialTerragruntConfig, err := PartialParseConfigFile(
		targetParsingContext.WithDecodeList(DependencyBlock),
		l,
		targetConfig,
		nil,
	)
	if err != nil {
		return l, nil, err
	}

	// Only override TFPath if it was not explicitly set by the user via CLI or environment variable
	if !targetOptions.TFPathExplicitlySet && partialTerragruntConfig.TerraformBinary != "" {
		targetOptions.TFPath = partialTerragruntConfig.TerraformBinary
	}

	// If the Source is set, then we need to recompute it in the ctx of the target config.
	if ctx.TerragruntOptions.Source != "" {
		// We need the terraform source of the target config to compute the actual source to use
		partialParseIncludedConfig, err := PartialParseConfigFile(
			targetParsingContext.WithDecodeList(TerraformBlock),
			l,
			targetConfig,
			nil,
		)
		if err != nil {
			return l, nil, err
		}
		// Update the source value to be everything before "//" so that it can be recomputed
		moduleURL, _ := getter.SourceDirSubdir(ctx.TerragruntOptions.Source)

		// Finally, update the source to be the combined path between the terraform source in the target config, and the
		// value before "//" in the original terragrunt options.
		targetSource, err := GetTerragruntSourceForModule(moduleURL, filepath.Dir(targetConfig), partialParseIncludedConfig)
		if err != nil {
			return l, nil, err
		}

		targetOptions.Source = targetSource
	}

	return l, targetOptions, nil
}

// Retrieve the outputs from the terraform state in the target configuration. This attempts to optimize the output
// retrieval if the following conditions are true:
// - State backends are managed with a `remote_state` block.
// - The `remote_state` block does not depend on any `dependency` outputs.
// If these conditions are met, terragrunt can optimize the retrieval to avoid recursively retrieving dependency outputs
// by directly pulling down the state file. Otherwise, terragrunt will fallback to running `terragrunt output` on the
// target module.
func getTerragruntOutputJSON(ctx *ParsingContext, l log.Logger, targetConfig string) ([]byte, error) {
	// Make a copy of the terragruntOptions so that we can reuse the same execution environment, but in the ctx of
	// the target config.
	l, targetTGOptions, err := cloneTerragruntOptionsForDependencyOutput(ctx, l, targetConfig)
	if err != nil {
		return nil, err
	}

	ctx = ctx.WithTerragruntOptions(targetTGOptions)

	// First attempt to parse the `remote_state` blocks without parsing/getting dependency outputs. If this is possible,
	// proceed to routine that fetches remote state directly. Otherwise, fallback to calling `terragrunt output`
	// directly.

	// we need to suspend logging diagnostic errors on this attempt
	parseOptions := append(ctx.ParserOptions, hclparse.WithDiagnosticsWriter(io.Discard, true))

	remoteStateTGConfig, err := PartialParseConfigFile(
		ctx.WithParseOption(parseOptions).WithDecodeList(
			RemoteStateBlock,
			TerragruntFlags,
			EngineBlock,
		),
		l,
		targetConfig,
		nil,
	)
	canGet := canGetRemoteState(remoteStateTGConfig.RemoteState)

	if err != nil || !canGet {
		l, targetOpts, err := cloneTerragruntOptionsForDependency(ctx, l, targetConfig)
		if err != nil {
			return nil, err
		}

		l.Debugf("Could not parse remote_state block from target config %s", targetOpts.TerragruntConfigPath)
		l.Debugf("Falling back to terragrunt output.")

		return runTerragruntOutputJSON(ctx, l, targetConfig)
	}

	// In optimization mode, see if there is already an init-ed folder that terragrunt can use, and if so, run
	// `terraform output` in the working directory.
	isInit, workingDir, err := terragruntAlreadyInit(l, targetTGOptions, targetConfig, ctx)
	if err != nil {
		return nil, err
	}

	// Fetch engine options so they can be passed to the dependency functions
	engineOpts, err := remoteStateTGConfig.EngineOptions()
	if err != nil {
		return nil, err
	}

	ctx.TerragruntOptions.Engine = engineOpts

	if isInit {
		return getTerragruntOutputJSONFromInitFolder(ctx, l, workingDir, remoteStateTGConfig.GetIAMRoleOptions())
	}

	return getTerragruntOutputJSONFromRemoteState(
		ctx,
		l,
		targetConfig,
		remoteStateTGConfig.RemoteState,
		remoteStateTGConfig.GetIAMRoleOptions(),
	)
}

// canGetRemoteState returns true if the remote state block is not nil and dependency optimization is not disabled
func canGetRemoteState(remoteState *remotestate.RemoteState) bool {
	return remoteState != nil && !remoteState.DisableDependencyOptimization
}

// terragruntAlreadyInit returns true if it detects that the module specified by the given terragrunt configuration is
// already initialized with the terraform source. This will also return the working directory where you can run
// terraform.
func terragruntAlreadyInit(l log.Logger, opts *options.TerragruntOptions, configPath string, ctx *ParsingContext) (bool, string, error) {
	// We need to first determine the working directory where the terraform source should be located. This is dependent
	// on the source field of the terraform block in the config.
	terraformBlockTGConfig, err := PartialParseConfigFile(ctx.WithDecodeList(TerraformSource), l, configPath, nil)
	if err != nil {
		return false, "", err
	}

	var workingDir string

	sourceURL, err := GetTerraformSourceURL(opts, terraformBlockTGConfig)
	if err != nil {
		return false, "", err
	}

	if sourceURL == "" || sourceURL == "." {
		// When there is no source URL, there is no download process and the working dir is the same as the directory
		// where the config is.
		if util.IsDir(configPath) {
			workingDir = configPath
		} else {
			workingDir = filepath.Dir(configPath)
		}
	} else {
		walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)

		terraformSource, err := tf.NewSource(l, sourceURL, opts.DownloadDir, opts.WorkingDir, walkWithSymlinks)
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

// getTerragruntOutputJSONFromInitFolder will retrieve the outputs directly from the module's working directory without
// running init.
func getTerragruntOutputJSONFromInitFolder(ctx *ParsingContext, l log.Logger, terraformWorkingDir string, iamRoleOpts options.IAMRoleOptions) ([]byte, error) {
	targetConfigPath := ctx.TerragruntOptions.TerragruntConfigPath

	targetTGOptions, err := setupTerragruntOptionsForBareTerraform(ctx, l, terraformWorkingDir, targetConfigPath, iamRoleOpts)
	if err != nil {
		return nil, err
	}

	l.Debugf("Detected module %s is already init-ed. Retrieving outputs directly from working directory.", targetTGOptions.TerragruntConfigPath)

	out, err := tf.RunCommandWithOutput(ctx, l, targetTGOptions, tf.CommandNameOutput, "-json")
	if err != nil {
		return nil, err
	}

	jsonString := strings.TrimSpace(out.Stdout.String())
	jsonBytes := []byte(jsonString)

	l.Debugf("Retrieved output from %s as json: %s", targetConfigPath, jsonString)

	return jsonBytes, nil
}

// getTerragruntOutputJSONFromRemoteState will retrieve the outputs directly by using just the remote state block. This
// uses terraform's feature where `output` and `init` can work without the real source, as long as you have the
// `backend` configured.
// To do this, this function will:
// - Create a temporary folder
// - Generate the backend.tf file with the backend configuration from the remote_state block
// - Copy the provider lock file, if there is one in the dependency's working directory
// - Run terraform init and terraform output
// - Clean up folder once json file is generated
// NOTE: terragruntOptions should be in the ctx of the targetConfig already.
func getTerragruntOutputJSONFromRemoteState(
	ctx *ParsingContext,
	l log.Logger,
	targetConfigPath string,
	remoteState *remotestate.RemoteState,
	iamRoleOpts options.IAMRoleOptions,
) ([]byte, error) {
	l.Debugf("Detected remote state block with generate config. Resolving dependency by pulling remote state.")
	// Create working directory where we will run terraform in. We will create the temporary directory in the download
	// directory for consistency with other file generation capabilities of terragrunt. Make sure it is cleaned up
	// before the function returns.
	if err := util.EnsureDirectory(ctx.TerragruntOptions.DownloadDir); err != nil {
		return nil, err
	}

	tempWorkDir, err := os.MkdirTemp(ctx.TerragruntOptions.DownloadDir, "")
	if err != nil {
		return nil, err
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			l.Warnf("Failed to remove %s: %v", path, err)
		}
	}(tempWorkDir)

	l.Debugf("Setting dependency working directory to %s", tempWorkDir)

	targetTGOptions, err := setupTerragruntOptionsForBareTerraform(ctx, l, tempWorkDir, targetConfigPath, iamRoleOpts)
	if err != nil {
		return nil, err
	}

	ctx = ctx.WithTerragruntOptions(targetTGOptions)

	// To speed up dependencies processing it is possible to retrieve its output directly from the backend without init dependencies
	if ctx.TerragruntOptions.FetchDependencyOutputFromState {
		switch backend := remoteState.BackendName; backend {
		case s3backend.BackendName:
			jsonBytes, err := getTerragruntOutputJSONFromRemoteStateS3(
				ctx,
				l,
				targetTGOptions,
				remoteState,
			)
			if err != nil {
				return nil, err
			}

			l.Debugf("Retrieved output from %s as json: %s using s3 bucket", targetTGOptions.TerragruntConfigPath, jsonBytes)

			return jsonBytes, nil
		default:
			l.Errorf("FetchDependencyOutputFromState is not supported for backend %s, falling back to normal method", backend)
		}
	}

	// Generate the backend configuration in the working dir. If no generate config is set on the remote state block,
	// set a temporary generate config so we can generate the backend code.
	if remoteState.Generate == nil {
		remoteState.Generate = &remotestate.ConfigGenerate{
			Path:     "backend.tf",
			IfExists: codegen.ExistsOverwriteTerragruntStr,
		}
	}

	if err := remoteState.GenerateOpenTofuCode(l, targetTGOptions); err != nil {
		return nil, err
	}

	l.Debugf("Generated remote state configuration in working dir %s", tempWorkDir)

	// Check for a provider lock file and copy it to the working dir if it exists.
	terragruntDir := filepath.Dir(ctx.TerragruntOptions.TerragruntConfigPath)
	if err := CopyLockFile(l, ctx.TerragruntOptions, terragruntDir, tempWorkDir); err != nil {
		return nil, err
	}

	// The working directory is now set up to interact with the state, so pull it down to get the json output.

	// First run init to setup the backend configuration so that we can run output.
	if err := runTerraformInitForDependencyOutput(ctx, l, tempWorkDir, targetConfigPath); err != nil {
		return nil, err
	}

	// Now that the backend is initialized, run terraform output to get the data and return it.
	out, err := tf.RunCommandWithOutput(ctx, l, targetTGOptions, tf.CommandNameOutput, "-json")
	if err != nil {
		return nil, err
	}

	jsonString := strings.TrimSpace(out.Stdout.String())
	jsonBytes := []byte(jsonString)
	l.Debugf("Retrieved output from %s as json: %s", targetConfigPath, jsonString)

	return jsonBytes, nil
}

// getTerragruntOutputJSONFromRemoteStateS3 pulls the output directly from an S3 bucket without calling Terraform
func getTerragruntOutputJSONFromRemoteStateS3(ctx *ParsingContext, l log.Logger, opts *options.TerragruntOptions, remoteState *remotestate.RemoteState) ([]byte, error) {
	l.Debugf("Fetching outputs directly from s3://%s/%s", remoteState.BackendConfig["bucket"], remoteState.BackendConfig["key"])

	s3ConfigExtended, err := s3backend.Config(remoteState.BackendConfig).ParseExtendedS3Config()
	if err != nil {
		return nil, err
	}

	sessionConfig := s3ConfigExtended.GetAwsSessionConfig()

	s3Client, err := awshelper.CreateS3Client(ctx, l, sessionConfig, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	result, err := s3Client.GetObject(ctx.Context, &s3.GetObjectInput{
		Bucket: aws.String(fmt.Sprintf("%s", remoteState.BackendConfig["bucket"])),
		Key:    aws.String(fmt.Sprintf("%s", remoteState.BackendConfig["key"])),
	})
	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			l.Warnf("Failed to close remote state response %v", err)
		}
	}(result.Body)

	steateBody, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}

	jsonState := string(steateBody)
	jsonMap := make(map[string]any)

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
func setupTerragruntOptionsForBareTerraform(ctx *ParsingContext, l log.Logger, workingDir string, configPath string, iamRoleOpts options.IAMRoleOptions) (*options.TerragruntOptions, error) {
	// Here we clone the terragrunt options again since we need to make further modifications to it to allow running
	// terraform directly.
	// Set the terraform working dir to the tempdir, and set stdout writer to io.Discard so that output content is
	// not logged.
	l, targetTGOptions, err := cloneTerragruntOptionsForDependency(ctx, l, configPath)
	if err != nil {
		return nil, err
	}

	targetTGOptions.WorkingDir = workingDir
	targetTGOptions.Writer = io.Discard
	targetTGOptions.Engine = ctx.TerragruntOptions.Engine

	// If the target config has an IAM role directive and it was not set on the command line, set it to
	// the one we retrieved from the config.
	targetTGOptions.IAMRoleOptions = options.MergeIAMRoleOptions(iamRoleOpts, targetTGOptions.OriginalIAMRoleOptions)

	// Make sure to assume any roles set by TG_IAM_ROLE
	if err := creds.NewGetter().ObtainAndUpdateEnvIfNecessary(ctx, l, targetTGOptions,
		externalcmd.NewProvider(l, targetTGOptions),
		amazonsts.NewProvider(l, targetTGOptions),
	); err != nil {
		return nil, err
	}

	return targetTGOptions, nil
}

// runTerragruntOutputJSON uses terragrunt running functions to extract the json output from the target config.
// NOTE: targetTGOptions should be in the ctx of the targetConfig.
func runTerragruntOutputJSON(ctx *ParsingContext, l log.Logger, targetConfig string) ([]byte, error) {
	// Update the stdout buffer so we can capture the output
	var stdoutBuffer bytes.Buffer

	stdoutBufferWriter := bufio.NewWriter(&stdoutBuffer)

	newOpts := *ctx.TerragruntOptions
	// explicit disable json formatting and prefixing to read json output
	newOpts.ForwardTFStdout = false
	newOpts.JSONLogFormat = false
	newOpts.Writer = stdoutBufferWriter
	ctx = ctx.WithTerragruntOptions(&newOpts)

	err := ctx.TerragruntOptions.RunTerragrunt(ctx, l, ctx.TerragruntOptions, report.NewReport())
	if err != nil {
		return nil, errors.New(err)
	}

	err = stdoutBufferWriter.Flush()
	if err != nil {
		return nil, errors.New(err)
	}

	jsonString := strings.TrimSpace(stdoutBuffer.String())
	jsonBytes := []byte(jsonString)

	l.Debugf("Retrieved output from %s as json: %s", targetConfig, jsonString)

	return jsonBytes, nil
}

// TerraformOutputJSONToCtyValueMap takes the terraform output json and converts to a mapping between output keys to the
// parsed cty.Value encoding of the json objects.
func TerraformOutputJSONToCtyValueMap(targetConfigPath string, jsonBytes []byte) (map[string]cty.Value, error) {
	// When getting all outputs, terraform returns a json with the data containing metadata about the types, so we
	// can't quite return the data directly. Instead, we will need further processing to get the output we want.
	// To do so, we first Unmarshal the json into a simple go map to a OutputMeta struct.
	type OutputMeta struct {
		Type      json.RawMessage `json:"type"`
		Value     json.RawMessage `json:"value"`
		Sensitive bool            `json:"sensitive"`
	}

	var outputs map[string]OutputMeta

	err := json.Unmarshal(jsonBytes, &outputs)
	if err != nil {
		return nil, errors.New(TerragruntOutputParsingError{Path: targetConfigPath, Err: err})
	}

	flattenedOutput := map[string]cty.Value{}

	for k, v := range outputs {
		outputType, err := ctyjson.UnmarshalType(v.Type)
		if err != nil {
			return nil, errors.New(TerragruntOutputParsingError{Path: targetConfigPath, Err: err})
		}

		outputVal, err := ctyjson.Unmarshal(v.Value, outputType)
		if err != nil {
			return nil, errors.New(TerragruntOutputParsingError{Path: targetConfigPath, Err: err})
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
func runTerraformInitForDependencyOutput(ctx *ParsingContext, l log.Logger, workingDir string, targetConfigPath string) error {
	stderr := bytes.Buffer{}

	l, initTGOptions, err := cloneTerragruntOptionsForDependency(ctx, l, targetConfigPath)
	if err != nil {
		return err
	}

	initTGOptions.WorkingDir = workingDir
	initTGOptions.ErrWriter = &stderr

	if err = tf.RunCommand(ctx, l, initTGOptions, tf.CommandNameInit, "-get=false"); err != nil {
		l.Debugf("Ignoring expected error from dependency init call")
		l.Debugf("Init call stderr:")
		l.Debugf(stderr.String())
	}

	return nil
}

func (deps Dependencies) FilteredWithoutConfigPath() Dependencies {
	var filteredDeps Dependencies

	for _, dep := range deps {
		if !dep.ConfigPath.IsNull() {
			filteredDeps = append(filteredDeps, dep)
		}
	}

	return filteredDeps
}

// isEmptyKnownString returns true when v is a fully-known string whose trimmed value is empty.
func isEmptyKnownString(v cty.Value) bool {
	return v.Type().Equals(cty.String) && v.IsWhollyKnown() && strings.TrimSpace(v.AsString()) == ""
}
