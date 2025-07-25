package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/huandu/go-clone"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

// PartialDecodeSectionType is an enum that is used to list out which blocks/sections of the terragrunt config should be
// decoded in a partial decode.
type PartialDecodeSectionType int

const (
	DependenciesBlock PartialDecodeSectionType = iota
	DependencyBlock
	TerraformBlock
	TerraformSource
	TerragruntFlags
	TerragruntInputs
	TerragruntVersionConstraints
	RemoteStateBlock
	FeatureFlagsBlock
	EngineBlock
	ExcludeBlock
	ErrorsBlock
)

// terragruntIncludeMultiple is a struct that can be used to only decode the include block with labels.
type terragruntIncludeMultiple struct {
	Remain  hcl.Body       `hcl:",remain"`
	Include IncludeConfigs `hcl:"include,block"`
}

// terragruntDependencies is a struct that can be used to only decode the dependencies block.
type terragruntDependencies struct {
	Dependencies *ModuleDependencies `hcl:"dependencies,block"`
	Remain       hcl.Body            `hcl:",remain"`
}

// terragruntFeatureFlags is a struct that can be used to store decoded feature flags.
type terragruntFeatureFlags struct {
	Remain       hcl.Body     `hcl:",remain"`
	FeatureFlags FeatureFlags `hcl:"feature,block"`
}

// terragruntErrors struct to decode errors block
type terragruntErrors struct {
	Errors *ErrorsConfig `hcl:"errors,block"`
	Remain hcl.Body      `hcl:",remain"`
}

// terragruntTerraform is a struct that can be used to only decode the terraform block.
type terragruntTerraform struct {
	Terraform *TerraformConfig `hcl:"terraform,block"`
	Remain    hcl.Body         `hcl:",remain"`
}

// terragruntTerraformSource is a struct that can be used to only decode the terraform block, and only the source
// attribute.
type terragruntTerraformSource struct {
	Terraform *terraformConfigSourceOnly `hcl:"terraform,block"`
	Remain    hcl.Body                   `hcl:",remain"`
}

// terraformConfigSourceOnly is a struct that can be used to decode only the source attribute of the terraform block.
type terraformConfigSourceOnly struct {
	Source *string  `hcl:"source,attr"`
	Remain hcl.Body `hcl:",remain"`
}

// terragruntFlags is a struct that can be used to only decode the flag attributes (skip and prevent_destroy)
type terragruntFlags struct {
	IamRole             *string  `hcl:"iam_role,attr"`
	IamWebIdentityToken *string  `hcl:"iam_web_identity_token,attr"`
	PreventDestroy      *bool    `hcl:"prevent_destroy,attr"`
	Skip                *bool    `hcl:"skip,attr"`
	Remain              hcl.Body `hcl:",remain"`
}

// terragruntVersionConstraints is a struct that can be used to only decode the attributes related to constraining the
// versions of terragrunt and terraform.
type terragruntVersionConstraints struct {
	TerragruntVersionConstraint *string  `hcl:"terragrunt_version_constraint,attr"`
	TerraformVersionConstraint  *string  `hcl:"terraform_version_constraint,attr"`
	TerraformBinary             *string  `hcl:"terraform_binary,attr"`
	Remain                      hcl.Body `hcl:",remain"`
}

// TerragruntDependency is a struct that can be used to only decode the dependency blocks in the terragrunt config
type TerragruntDependency struct {
	Remain       hcl.Body     `hcl:",remain"`
	Dependencies Dependencies `hcl:"dependency,block"`
}

// terragruntRemoteState is a struct that can be used to only decode the remote_state blocks in the terragrunt config
type terragruntRemoteState struct {
	RemoteState *remotestate.ConfigFile `hcl:"remote_state,block"`
	Remain      hcl.Body                `hcl:",remain"`
}

// terragruntInputs is a struct that can be used to only decode the inputs block.
type terragruntInputs struct {
	Inputs *cty.Value `hcl:"inputs,attr"`
	Remain hcl.Body   `hcl:",remain"`
}

// terragruntEngine is a struct that can only be used to decode the engine block.
type terragruntEngine struct {
	Engine *EngineConfig `hcl:"engine,block"`
	Remain hcl.Body      `hcl:",remain"`
}

// DecodeBaseBlocks takes in a parsed HCL2 file and decodes the base blocks. Base blocks are blocks that should always
// be decoded even in partial decoding, because they provide bindings that are necessary for parsing any block in the
// file. Currently base blocks are:
// - locals
// - features
// - include
func DecodeBaseBlocks(ctx *ParsingContext, l log.Logger, file *hclparse.File, includeFromChild *IncludeConfig) (*DecodedBaseBlocks, error) {
	errs := &errors.MultiError{}

	evalParsingContext, err := createTerragruntEvalContext(ctx, l, file.ConfigPath)
	if err != nil {
		return nil, err
	}

	// Decode just the `include` and `import` blocks, and verify that it's allowed here
	terragruntIncludeList, err := decodeAsTerragruntInclude(
		file,
		evalParsingContext,
	)
	if err != nil {
		errs = errs.Append(err)
	}

	trackInclude, err := getTrackInclude(ctx, terragruntIncludeList, includeFromChild)
	if err != nil {
		errs = errs.Append(err)
	}

	// set feature flags
	tgFlags := terragruntFeatureFlags{}
	// load default feature flags
	if err := file.Decode(&tgFlags, evalParsingContext); err != nil {
		return nil, err
	}
	// validate flags to have default value, collect errors
	flagErrs := &errors.MultiError{}

	for _, flag := range tgFlags.FeatureFlags {
		if flag.Default == nil {
			flagErr := fmt.Errorf("feature flag %s does not have a default value in %s", flag.Name, file.ConfigPath)
			flagErrs = flagErrs.Append(flagErr)
		}
	}

	if flagErrs.ErrorOrNil() != nil {
		errs = errs.Append(flagErrs)
	}

	flagsAsCtyVal, err := flagsAsCty(ctx, tgFlags.FeatureFlags)
	if err != nil {
		errs = errs.Append(err)
	}

	// Evaluate all the expressions in the locals block separately and generate the variables list to use in the
	// evaluation ctx.
	locals, err := EvaluateLocalsBlock(ctx.WithTrackInclude(trackInclude).WithFeatures(&flagsAsCtyVal), l, file)
	if err != nil {
		errs = errs.Append(err)
	}

	localsAsCtyVal, err := convertValuesMapToCtyVal(locals)
	if err != nil {
		return nil, err
	}

	return &DecodedBaseBlocks{
		TrackInclude: trackInclude,
		Locals:       &localsAsCtyVal,
		FeatureFlags: &flagsAsCtyVal,
	}, errs.ErrorOrNil()
}

func flagsAsCty(ctx *ParsingContext, tgFlags FeatureFlags) (cty.Value, error) {
	// extract all flags in map by name
	flagByName := map[string]*FeatureFlag{}
	for _, flag := range tgFlags {
		flagByName[flag.Name] = flag
	}

	evaluatedFlags, err := cliFlagsToCty(ctx, flagByName)
	if err != nil {
		return cty.NilVal, err
	}

	errs := &errors.MultiError{}

	for _, flag := range tgFlags {
		if _, exists := evaluatedFlags[flag.Name]; !exists {
			if flag.Default == nil {
				errs = errs.Append(fmt.Errorf("feature flag %s does not have a default value in %s", flag.Name, ctx.TerragruntOptions.TerragruntConfigPath))
				continue
			}

			contextFlag, err := flagToCtyValue(flag.Name, *flag.Default)

			if err != nil {
				return cty.NilVal, err
			}

			evaluatedFlags[flag.Name] = contextFlag
		}
	}

	flagsAsCtyVal, err := convertValuesMapToCtyVal(evaluatedFlags)

	if err != nil {
		return cty.NilVal, err
	}

	return flagsAsCtyVal, errs.ErrorOrNil()
}

// cliFlagsToCty converts CLI feature flags to Cty values. It returns a map of flag names
// to their corresponding Cty values and any error encountered during conversion.
func cliFlagsToCty(ctx *ParsingContext, flagByName map[string]*FeatureFlag) (map[string]cty.Value, error) {
	if ctx.TerragruntOptions.FeatureFlags == nil {
		return make(map[string]cty.Value), nil
	}

	evaluatedFlags := make(map[string]cty.Value)

	var conversionErr error

	ctx.TerragruntOptions.FeatureFlags.Range(func(name, value string) bool {
		var flag cty.Value

		var err error

		if existingFlag, ok := flagByName[name]; ok {
			flag, err = flagToTypedCtyValue(name, existingFlag.Default.Type(), value)
		} else {
			flag, err = flagToCtyValue(name, value)
		}

		if err != nil {
			conversionErr = err

			return false
		}

		evaluatedFlags[name] = flag

		return true
	})

	if conversionErr != nil {
		return nil, conversionErr
	}

	return evaluatedFlags, nil
}

func PartialParseConfigFile(ctx *ParsingContext, l log.Logger, configPath string, include *IncludeConfig) (*TerragruntConfig, error) {
	hclCache := cache.ContextCache[*hclparse.File](ctx, HclCacheContextKey)

	fileInfo, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, TerragruntConfigNotFoundError{Path: configPath}
		}

		return nil, errors.New(err)
	}

	var (
		file     *hclparse.File
		cacheKey = fmt.Sprintf("configPath-%v-modTime-%v", configPath, fileInfo.ModTime().UnixMicro())
	)

	if cacheConfig, found := hclCache.Get(ctx, cacheKey); found {
		file = cacheConfig
	} else {
		file, err = hclparse.NewParser(ctx.ParserOptions...).ParseFromFile(configPath)
		if err != nil {
			return nil, err
		}
	}

	return TerragruntConfigFromPartialConfig(ctx, l, file, include)
}

// TerragruntConfigFromPartialConfig is a wrapper of PartialParseConfigString which checks for cached configs.
// filename, configString, includeFromChild and decodeList are used for the cache key,
// by getting the default value (%#v) through fmt.
func TerragruntConfigFromPartialConfig(ctx *ParsingContext, l log.Logger, file *hclparse.File, includeFromChild *IncludeConfig) (*TerragruntConfig, error) {
	var cacheKey = fmt.Sprintf("%#v-%#v-%#v-%#v", file.ConfigPath, file.Content(), includeFromChild, ctx.PartialParseDecodeList)

	terragruntConfigCache := cache.ContextCache[*TerragruntConfig](ctx, TerragruntConfigCacheContextKey)
	if ctx.TerragruntOptions.UsePartialParseConfigCache {
		if config, found := terragruntConfigCache.Get(ctx, cacheKey); found {
			l.Debugf("Cache hit for '%s' (partial parsing), decodeList: '%v'.", ctx.TerragruntOptions.TerragruntConfigPath, ctx.PartialParseDecodeList)

			deepCopy := clone.Clone(config).(*TerragruntConfig)

			return deepCopy, nil
		}

		l.Debugf("Cache miss for '%s' (partial parsing), decodeList: '%v'.", ctx.TerragruntOptions.TerragruntConfigPath, ctx.PartialParseDecodeList)
	}

	config, err := PartialParseConfig(ctx, l, file, includeFromChild)
	if err != nil {
		return config, err
	}

	if ctx.TerragruntOptions.UsePartialParseConfigCache {
		putConfig := clone.Clone(config).(*TerragruntConfig)
		terragruntConfigCache.Put(ctx, cacheKey, putConfig)
	}

	return config, nil
}

// PartialParseConfigString partially parses and decodes the provided string. Which blocks/attributes to decode is
// controlled by the function parameter decodeList. These blocks/attributes are parsed and set on the output
// TerragruntConfig. Valid values are:
//   - DependenciesBlock: Parses the `dependencies` block in the config
//   - DependencyBlock: Parses the `dependency` block in the config
//   - TerraformBlock: Parses the `terraform` block in the config
//   - TerragruntFlags: Parses the boolean flags `prevent_destroy` and `skip` in the config
//   - TerragruntVersionConstraints: Parses the attributes related to constraining terragrunt and terraform versions in
//     the config.
//   - RemoteStateBlock: Parses the `remote_state` block in the config
//   - FeatureFlagsBlock: Parses the `feature` block in the config
//   - EngineBlock: Parses the `engine` block in the config
//   - ExcludeBlock : Parses the `exclude` block in the config
//
// Note that the following blocks are always decoded:
// - locals
// - include
// Note also that the following blocks are never decoded in a partial parse:
// - inputs
func PartialParseConfigString(ctx *ParsingContext, l log.Logger, configPath, configString string, include *IncludeConfig) (*TerragruntConfig, error) {
	file, err := hclparse.NewParser(ctx.ParserOptions...).ParseFromString(configString, configPath)
	if err != nil {
		return nil, err
	}

	return PartialParseConfig(ctx, l, file, include)
}

func PartialParseConfig(ctx *ParsingContext, l log.Logger, file *hclparse.File, includeFromChild *IncludeConfig) (*TerragruntConfig, error) {
	errs := &errors.MultiError{}

	ctx = ctx.WithTrackInclude(nil)

	// read unit files and add to context
	unitValues, err := ReadValues(ctx.Context, l, ctx.TerragruntOptions, filepath.Dir(file.ConfigPath))
	if err != nil {
		return nil, err
	}

	ctx = ctx.WithValues(unitValues)

	// Decode just the Base blocks. See the function docs for DecodeBaseBlocks for more info on what base blocks are.
	// Initialize evaluation ctx extensions from base blocks.
	baseBlocks, err := DecodeBaseBlocks(ctx, l, file, includeFromChild)
	if err != nil {
		errs = errs.Append(err)
	}

	if baseBlocks != nil {
		ctx = ctx.WithTrackInclude(baseBlocks.TrackInclude)
		ctx = ctx.WithFeatures(baseBlocks.FeatureFlags)
		ctx = ctx.WithLocals(baseBlocks.Locals)
	}

	// Set parsed Locals on the parsed config
	output, err := convertToTerragruntConfig(ctx, file.ConfigPath, &terragruntConfigFile{})
	if err != nil {
		return nil, err
	}

	output.IsPartial = true

	evalParsingContext, err := createTerragruntEvalContext(ctx, l, file.ConfigPath)
	if err != nil {
		return nil, err
	}

	// Now loop through each requested block / component to decode from the terragrunt config, decode them, and merge
	// them into the output TerragruntConfig struct.
	for _, decode := range ctx.PartialParseDecodeList {
		switch decode {
		case DependenciesBlock:
			decoded := terragruntDependencies{}

			err := file.Decode(&decoded, evalParsingContext)
			if err != nil {
				return nil, err
			}

			// If we already decoded some dependencies, merge them in. Otherwise, set as the new list.
			if output.Dependencies != nil {
				output.Dependencies.Merge(decoded.Dependencies)
			} else {
				output.Dependencies = decoded.Dependencies
			}

		case TerraformBlock:
			decoded := terragruntTerraform{}

			err := file.Decode(&decoded, evalParsingContext)
			if err != nil {
				return nil, err
			}

			output.Terraform = decoded.Terraform

		case TerraformSource:
			decoded := terragruntTerraformSource{}

			err := file.Decode(&decoded, evalParsingContext)
			if err != nil {
				return nil, err
			}

			if decoded.Terraform != nil {
				output.Terraform = &TerraformConfig{Source: decoded.Terraform.Source}
			}

		case DependencyBlock:
			decoded := TerragruntDependency{}

			err := file.Decode(&decoded, evalParsingContext)
			if err != nil {
				return nil, err
			}

			// In normal operation, if a dependency block does not have a `config_path` attribute, decoding returns an error since this attribute is required, but the `hclvalidate` command suppresses decoding errors and this causes a cycle between modules, so we need to filter out dependencies without a defined `config_path`.
			decoded.Dependencies = decoded.Dependencies.FilteredWithoutConfigPath()

			output.TerragruntDependencies = decoded.Dependencies
			// Convert dependency blocks into module dependency lists. If we already decoded some dependencies,
			// merge them in. Otherwise, set as the new list.
			dependencies := dependencyBlocksToModuleDependencies(decoded.Dependencies)
			if output.Dependencies != nil {
				output.Dependencies.Merge(dependencies)
			} else {
				output.Dependencies = dependencies
			}

		case EngineBlock:
			decoded := terragruntEngine{}

			err := file.Decode(&decoded, evalParsingContext)
			if err != nil {
				return nil, err
			}

			output.Engine = decoded.Engine

		case TerragruntFlags:
			decoded := terragruntFlags{}

			err := file.Decode(&decoded, evalParsingContext)
			if err != nil {
				return nil, err
			}

			if decoded.PreventDestroy != nil {
				output.PreventDestroy = decoded.PreventDestroy
			}

			if decoded.Skip != nil {
				output.Skip = decoded.Skip
			}

			if decoded.IamRole != nil {
				output.IamRole = *decoded.IamRole
			}

			if decoded.IamWebIdentityToken != nil {
				output.IamWebIdentityToken = *decoded.IamWebIdentityToken
			}
		case TerragruntInputs:
			allControls := ctx.TerragruntOptions.StrictControls

			skipDependenciesInputs := allControls.Find(controls.SkipDependenciesInputs)
			if skipDependenciesInputs == nil {
				return nil, errors.New("failed to find control " + controls.SkipDependenciesInputs)
			}

			skipDependenciesInputs.SuppressWarning()

			if err := skipDependenciesInputs.Evaluate(ctx); err != nil {
				l.Debugf(
					"Skipping inputs parse from %v in dependency for better performance, due to usage of %s strict control",
					file.ConfigPath,
					controls.SkipDependenciesInputs,
				)

				break
			}

			decoded := terragruntInputs{}

			if _, ok := evalParsingContext.Variables[MetadataDependency]; !ok {
				// Decode just the `dependency` blocks, retrieving the outputs from the target terragrunt config in the process.
				retrievedOutputs, err := decodeAndRetrieveOutputs(ctx, l, file)
				if err != nil {
					return nil, err
				}

				evalParsingContext.Variables[MetadataDependency] = *retrievedOutputs
			}

			if err := file.Decode(&decoded, evalParsingContext); err != nil {
				var diagErr hcl.Diagnostics
				ok := errors.As(err, &diagErr)

				// in case of render-json command and inputs reference error, we update the inputs with default value
				if !ok || !isRenderJSONCommand(ctx) || !isRenderCommand(ctx) || !isAttributeAccessError(diagErr) {
					return nil, err
				}

				l.Warnf("Failed to decode inputs %v", diagErr)
			}

			if decoded.Inputs != nil {
				inputs, err := ctyhelper.ParseCtyValueToMap(*decoded.Inputs)
				if err != nil {
					return nil, err
				}

				output.Inputs = inputs
			}

		case TerragruntVersionConstraints:
			decoded := terragruntVersionConstraints{}

			err := file.Decode(&decoded, evalParsingContext)
			if err != nil {
				return nil, err
			}

			if decoded.TerragruntVersionConstraint != nil {
				output.TerragruntVersionConstraint = *decoded.TerragruntVersionConstraint
			}

			if decoded.TerraformVersionConstraint != nil {
				output.TerraformVersionConstraint = *decoded.TerraformVersionConstraint
			}

			// If the TFPath is not explicitly set, use the TFPath from the config if it is set.
			if !ctx.TerragruntOptions.TFPathExplicitlySet && decoded.TerraformBinary != nil {
				output.TerraformBinary = *decoded.TerraformBinary
			}

		case RemoteStateBlock:
			decoded := terragruntRemoteState{}

			err := file.Decode(&decoded, evalParsingContext)
			if err != nil {
				return nil, err
			}

			if decoded.RemoteState != nil {
				config, err := decoded.RemoteState.Config()
				if err != nil {
					return nil, err
				}

				output.RemoteState = remotestate.New(config)
			}
		case FeatureFlagsBlock:
			decoded := terragruntFeatureFlags{}
			err := file.Decode(&decoded, evalParsingContext)

			if err != nil {
				return nil, err
			}

			if output.FeatureFlags != nil {
				flags, err := deepMergeFeatureBlocks(output.FeatureFlags, decoded.FeatureFlags)
				if err != nil {
					return nil, err
				}

				output.FeatureFlags = flags
			} else {
				output.FeatureFlags = decoded.FeatureFlags
			}

		case ExcludeBlock:
			decoded, err := processExcludes(ctx, l, output, file)
			if err != nil {
				return nil, err
			}

			if output.Exclude != nil {
				output.Exclude.Merge(decoded.Exclude)
			} else {
				output.Exclude = decoded.Exclude
			}

		case ErrorsBlock:
			decoded := terragruntErrors{}
			err := file.Decode(&decoded, evalParsingContext)

			if err != nil {
				return nil, err
			}

			if output.Errors != nil {
				output.Errors.Merge(decoded.Errors)
			} else {
				output.Errors = decoded.Errors
			}

		default:
			return nil, InvalidPartialBlockName{decode}
		}
	}

	errsContainsIncludeErr := false

	for _, err := range errs.WrappedErrors() {
		if errors.As(err, &TooManyLevelsOfInheritanceError{}) {
			errsContainsIncludeErr = true
		}
	}

	// If this file includes another, parse and merge the partial blocks. Otherwise, just return this config.
	// If there have been errors during this parse, don't attempt to parse the included config.
	if len(ctx.TrackInclude.CurrentList) > 0 && !errsContainsIncludeErr {
		config, err := handleInclude(ctx, l, output, true)
		if err != nil {
			errs = errs.Append(err)
		}
		// Saving processed includes into configuration, direct assignment since nested includes aren't supported
		config.ProcessedIncludes = ctx.TrackInclude.CurrentMap

		output = config
	}

	if errs.ErrorOrNil() != nil {
		return output, errs.ErrorOrNil()
	}

	return processExcludes(ctx, l, output, file)
}

// processExcludes evaluate exclude blocks and merge them into the config.
func processExcludes(ctx *ParsingContext, l log.Logger, config *TerragruntConfig, file *hclparse.File) (*TerragruntConfig, error) {
	flagsAsCtyVal, err := flagsAsCty(ctx, config.FeatureFlags)
	if err != nil {
		return nil, err
	}

	excludeConfig, err := evaluateExcludeBlocks(ctx.WithFeatures(&flagsAsCtyVal), l, file)
	if err != nil {
		return nil, err
	}

	if excludeConfig == nil {
		return config, nil
	}

	if config.Exclude != nil {
		config.Exclude.Merge(excludeConfig)
	} else {
		config.Exclude = excludeConfig
	}

	return config, nil
}

func partialParseIncludedConfig(ctx *ParsingContext, l log.Logger, includedConfig *IncludeConfig) (*TerragruntConfig, error) {
	if includedConfig.Path == "" {
		return nil, errors.New(IncludedConfigMissingPathError(ctx.TerragruntOptions.TerragruntConfigPath))
	}

	includePath := includedConfig.Path

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(filepath.Dir(ctx.TerragruntOptions.TerragruntConfigPath), includePath)
	}

	return PartialParseConfigFile(
		ctx,
		l,
		includePath,
		includedConfig,
	)
}

// This decodes only the `include` blocks of a terragrunt config, so its value can be used while decoding the rest of
// the config.
// For consistency, `include` in the call to `file.Decode` is always assumed to be nil. Either it really is nil (parsing
// the child config), or it shouldn't be used anyway (the parent config shouldn't have an include block).
func decodeAsTerragruntInclude(file *hclparse.File, evalParsingContext *hcl.EvalContext) (IncludeConfigs, error) {
	tgInc := terragruntIncludeMultiple{}
	if err := file.Decode(&tgInc, evalParsingContext); err != nil {
		return nil, err
	}

	return tgInc.Include, nil
}

// Custom error types

type InvalidPartialBlockName struct {
	sectionCode PartialDecodeSectionType
}

func (err InvalidPartialBlockName) Error() string {
	return fmt.Sprintf("Unrecognized partial block code %d. This is most likely an error in terragrunt. Please file a bug report on the project repository.", err.sectionCode)
}
