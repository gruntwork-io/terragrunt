package config

import (
	"context"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

// ParsingContext provides various variables that are used throughout all funcs and passed from function to function.
// Using `ParsingContext` makes the code more readable.
type ParsingContext struct {
	context.Context

	TerragruntOptions *options.TerragruntOptions

	// TrackInclude represents contexts of included configurations.
	TrackInclude *TrackInclude

	// Locals are preevaluated variable bindings that can be used by reference in the code.
	Locals *cty.Value

	// DecodedDependencies are references of other terragrunt config. This contains the following attributes that map to
	// various fields related to that config:
	// - outputs: The map of outputs from the terraform state obtained by running `terragrunt output` on that target config.
	DecodedDependencies *cty.Value

	// PartialParseDecodeList is the list of sections that are being decoded in the current config. This can be used to
	// indicate/detect that the current parsing ctx is partial, meaning that not all configuration values are
	// expected to be available.
	PartialParseDecodeList []PartialDecodeSectionType

	// These functions have the highest priority and will overwrite any others with the same name
	PredefinedFunctions map[string]function.Function

	// `ParserOptions` is used to configure hcl Parser.
	ParserOptions []hclparse.Option

	// Set a custom converter to TerragruntConfig.
	// Used to read a "catalog" configuration where only certain blocks (`catalog`, `locals`) do not need to be converted, avoiding errors if any of the remaining blocks were not evaluated correctly.
	ConvertToTerragruntConfigFunc func(ctx *ParsingContext, configPath string, terragruntConfigFromFile *terragruntConfigFile) (cfg *TerragruntConfig, err error)
}

func NewParsingContext(ctx context.Context, opts *options.TerragruntOptions) *ParsingContext {
	ctx = shell.ContextWithTerraformCommandHook(ctx, nil)

	return &ParsingContext{
		Context:           ctx,
		TerragruntOptions: opts,
		ParserOptions:     DefaultParserOptions(opts),
	}
}
func (ctx ParsingContext) WithDecodeList(decodeList ...PartialDecodeSectionType) *ParsingContext {
	ctx.PartialParseDecodeList = decodeList
	return &ctx
}

func (ctx ParsingContext) WithTerragruntOptions(opts *options.TerragruntOptions) *ParsingContext {
	ctx.TerragruntOptions = opts
	return &ctx
}

func (ctx ParsingContext) WithLocals(locals *cty.Value) *ParsingContext {
	ctx.Locals = locals
	return &ctx
}

func (ctx ParsingContext) WithTrackInclude(trackInclude *TrackInclude) *ParsingContext {
	ctx.TrackInclude = trackInclude
	return &ctx
}

func (ctx ParsingContext) WithParseOption(parserOptions []hclparse.Option) *ParsingContext {
	ctx.ParserOptions = parserOptions
	return &ctx
}
