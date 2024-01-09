package config

import (
	"context"

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

	ConvertToTerragruntConfigFunc func(ctx *Context, configPath string, terragruntConfigFromFile *terragruntConfigFile) (cfg *TerragruntConfig, err error)
}

func NewContext(ctx context.Context, opts *options.TerragruntOptions) *Context {
	return &Context{
		Context:           ctx,
		TerragruntOptions: opts,
		ParserOptions:     DefaultParserOptions(opts),
	}
}
func (ctx Context) WithDecodeList(decodeList ...PartialDecodeSectionType) *Context {
	ctx.PartialParseDecodeList = decodeList
	return &ctx
}

func (ctx Context) WithTerragruntOptions(opts *options.TerragruntOptions) *Context {
	ctx.TerragruntOptions = opts
	return &ctx
}

func (ctx Context) WithLocals(locals *cty.Value) *Context {
	ctx.Locals = locals
	return &ctx
}

func (ctx Context) WithTrackInclude(trackInclude *TrackInclude) *Context {
	ctx.TrackInclude = trackInclude
	return &ctx
}
