package config

import (
	"context"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	tflang "github.com/hashicorp/terraform/lang"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/config/hclparser"
	"github.com/gruntwork-io/terragrunt/options"
)

// Context provides various ctx to the evaluation ctx to enhance the parsing capabilities.
type Context struct {
	context.Context

	// TrackInclude represents contexts of included configurations.
	TrackInclude *TrackInclude

	// Locals are preevaluated variable bindings that can be used by reference in the code.
	Locals *cty.Value

	// DecodedDependencies are references of other terragrunt config. This contains the following attributes that map to
	// various fields related to that config:
	// - outputs: The map of outputs from the terraform state obtained by running `terragrunt output` on that target
	//            config.
	DecodedDependencies *cty.Value

	// PartialParseDecodeList is the list of sections that are being decoded in the current config. This can be used to
	// indicate/detect that the current parsing ctx is partial, meaning that not all configuration values are
	// expected to be available.
	PartialParseDecodeList []PartialDecodeSectionType

	// These functions have the highest priority and will overwrite any others with the same name
	PredefinedFunctions map[string]function.Function

	TerragruntOptions *options.TerragruntOptions

	ParserOptions []hclparser.Option

	ConvertToTerragruntConfigFunc func(ctx Context, configPath string, terragruntConfigFromFile *terragruntConfigFile) (cfg *TerragruntConfig, err error)
}

func NewContext(ctx context.Context, opts *options.TerragruntOptions) Context {
	return Context{
		Context:           ctx,
		TerragruntOptions: opts,
		ParserOptions:     DefaultParserOptions(opts),
	}
}
func (ctx Context) WithDecodeList(decodeList ...PartialDecodeSectionType) Context {
	ctx.PartialParseDecodeList = decodeList
	return ctx
}

func (ctx Context) WithTerragruntOptions(opts *options.TerragruntOptions) Context {
	ctx.TerragruntOptions = opts
	return ctx
}

// Create an EvalContext for the HCL2 parser. We can define functions and variables in this ctx that the HCL2 parser
// will make available to the Terragrunt configuration during parsing.
func (ctx Context) CreateTerragruntEvalContext(configPath string) (*hcl.EvalContext, error) {
	tfscope := tflang.Scope{
		BaseDir: filepath.Dir(configPath),
	}

	terragruntFunctions := map[string]function.Function{
		FuncNameFindInParentFolders:                     wrapStringSliceToStringAsFuncImpl(ctx, findInParentFolders),
		FuncNamePathRelativeToInclude:                   wrapStringSliceToStringAsFuncImpl(ctx, pathRelativeToInclude),
		FuncNamePathRelativeFromInclude:                 wrapStringSliceToStringAsFuncImpl(ctx, pathRelativeFromInclude),
		FuncNameGetEnv:                                  wrapStringSliceToStringAsFuncImpl(ctx, getEnvironmentVariable),
		FuncNameRunCmd:                                  wrapStringSliceToStringAsFuncImpl(ctx, runCommand),
		FuncNameReadTerragruntConfig:                    readTerragruntConfigAsFuncImpl(ctx),
		FuncNameGetPlatform:                             wrapVoidToStringAsFuncImpl(ctx, getPlatform),
		FuncNameGetRepoRoot:                             wrapVoidToStringAsFuncImpl(ctx, getRepoRoot),
		FuncNameGetPathFromRepoRoot:                     wrapVoidToStringAsFuncImpl(ctx, getPathFromRepoRoot),
		FuncNameGetPathToRepoRoot:                       wrapVoidToStringAsFuncImpl(ctx, getPathToRepoRoot),
		FuncNameGetTerragruntDir:                        wrapVoidToStringAsFuncImpl(ctx, getTerragruntDir),
		FuncNameGetOriginalTerragruntDir:                wrapVoidToStringAsFuncImpl(ctx, getOriginalTerragruntDir),
		FuncNameGetTerraformCommand:                     wrapVoidToStringAsFuncImpl(ctx, getTerraformCommand),
		FuncNameGetTerraformCLIArgs:                     wrapVoidToStringSliceAsFuncImpl(ctx, getTerraformCliArgs),
		FuncNameGetParentTerragruntDir:                  wrapStringSliceToStringAsFuncImpl(ctx, getParentTerragruntDir),
		FuncNameGetAWSAccountID:                         wrapVoidToStringAsFuncImpl(ctx, getAWSAccountID),
		FuncNameGetAWSCallerIdentityArn:                 wrapVoidToStringAsFuncImpl(ctx, getAWSCallerIdentityARN),
		FuncNameGetAWSCallerIdentityUserID:              wrapVoidToStringAsFuncImpl(ctx, getAWSCallerIdentityUserID),
		FuncNameGetTerraformCommandsThatNeedVars:        wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_VARS),
		FuncNameGetTerraformCommandsThatNeedLocking:     wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_LOCKING),
		FuncNameGetTerraformCommandsThatNeedInput:       wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_INPUT),
		FuncNameGetTerraformCommandsThatNeedParallelism: wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_PARALLELISM),
		FuncNameSopsDecryptFile:                         wrapStringSliceToStringAsFuncImpl(ctx, sopsDecryptFile),
		FuncNameGetTerragruntSourceCLIFlag:              wrapVoidToStringAsFuncImpl(ctx, getTerragruntSourceCliFlag),
		FuncNameGetDefaultRetryableErrors:               wrapVoidToStringSliceAsFuncImpl(ctx, getDefaultRetryableErrors),
		FuncNameReadTfvarsFile:                          wrapStringSliceToStringAsFuncImpl(ctx, readTFVarsFile),
		FuncNameGetWorkingDir:                           wrapVoidToStringAsFuncImpl(ctx, getWorkingDir),

		// Map with HCL functions introduced in Terraform after v0.15.3, since upgrade to a later version is not supported
		// https://github.com/gruntwork-io/terragrunt/blob/master/go.mod#L22
		FuncNameStartsWith:  wrapStringSliceToBoolAsFuncImpl(ctx, startsWith),
		FuncNameEndsWith:    wrapStringSliceToBoolAsFuncImpl(ctx, endsWith),
		FuncNameStrContains: wrapStringSliceToBoolAsFuncImpl(ctx, strContains),
		FuncNameTimeCmp:     wrapStringSliceToNumberAsFuncImpl(ctx, timeCmp),
	}

	functions := map[string]function.Function{}
	for k, v := range tfscope.Functions() {
		functions[k] = v
	}
	for k, v := range terragruntFunctions {
		functions[k] = v
	}
	for k, v := range ctx.PredefinedFunctions {
		functions[k] = v
	}

	evalCtx := &hcl.EvalContext{
		Functions: functions,
	}
	evalCtx.Variables = map[string]cty.Value{}
	if ctx.Locals != nil {
		evalCtx.Variables["local"] = *ctx.Locals
	}

	if ctx.DecodedDependencies != nil {
		evalCtx.Variables["dependency"] = *ctx.DecodedDependencies
	}
	if ctx.TrackInclude != nil && len(ctx.TrackInclude.CurrentList) > 0 {
		// For each include block, check if we want to expose the included config, and if so, add under the include
		// variable.
		exposedInclude, err := includeMapAsCtyVal(ctx)
		if err != nil {
			return evalCtx, err
		}
		evalCtx.Variables[MetadataInclude] = exposedInclude
	}
	return evalCtx, nil
}

// DecodeBaseBlocks takes in a parsed HCL2 file and decodes the base blocks. Base blocks are blocks that should always
// be decoded even in partial decoding, because they provide bindings that are necessary for parsing any block in the
// file. Currently base blocks are:
// - locals
// - include
func (ctx Context) DecodeBaseBlocks(file *hclparser.File, includeFromChild *IncludeConfig) (Context, error) {
	evalContext, err := ctx.CreateTerragruntEvalContext(file.ConfigPath)
	if err != nil {
		return ctx, err
	}

	// Decode just the `include` and `import` blocks, and verify that it's allowed here
	terragruntIncludeList, err := decodeAsTerragruntInclude(
		file,
		evalContext,
	)
	if err != nil {
		return ctx, err
	}

	ctx.TrackInclude, err = getTrackInclude(ctx, terragruntIncludeList, includeFromChild)
	if err != nil {
		return ctx, err
	}

	// Evaluate all the expressions in the locals block separately and generate the variables list to use in the
	// evaluation ctx.
	locals, err := evaluateLocalsBlock(ctx, file)
	if err != nil {
		return ctx, err
	}

	localsAsCtyVal, err := convertValuesMapToCtyVal(locals)
	if err != nil {
		return ctx, err
	}
	ctx.Locals = &localsAsCtyVal

	return ctx, nil
}

func (ctx Context) NewHCLParser() *hclparser.Parser {
	return hclparser.New().WithOptions(ctx.ParserOptions...)
}
