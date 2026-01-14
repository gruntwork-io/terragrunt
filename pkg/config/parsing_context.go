package config

import (
	"context"
	"io"
	"os"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/tf"
)

// ParsingContext provides various variables that are used throughout all funcs and passed from function to function.
// Using `ParsingContext` makes the code more readable.
// Note: context.Context should be passed explicitly as the first parameter to functions, not embedded in this struct.
type ParsingContext struct {
	TerragruntOptions *options.TerragruntOptions

	// TrackInclude represents contexts of included configurations.
	TrackInclude *TrackInclude

	// Locals are pre-evaluated variable bindings that can be used by reference in the code.
	Locals *cty.Value

	// Features are the feature flags that are enabled for the current terragrunt config.
	Features *cty.Value

	// Values of the unit.
	Values *cty.Value

	// DecodedDependencies are references of other terragrunt config. This contains the following attributes that map to
	// various fields related to that config:
	// - outputs: The map of outputs from the terraform state obtained by running `terragrunt output` on that target config.
	DecodedDependencies *cty.Value

	// These functions have the highest priority and will overwrite any others with the same name
	PredefinedFunctions map[string]function.Function

	// Set a custom converter to TerragruntConfig.
	// Used to read a "catalog" configuration where only certain blocks (`catalog`, `locals`) do not need to be converted, avoiding errors if any of the remaining blocks were not evaluated correctly.
	ConvertToTerragruntConfigFunc func(ctx context.Context, pctx *ParsingContext, configPath string, terragruntConfigFromFile *terragruntConfigFile) (cfg *TerragruntConfig, err error)

	// FilesRead tracks files that were read during parsing (absolute paths).
	// This is a pointer so that it's shared across all parsing context copies.
	FilesRead *[]string

	// PartialParseDecodeList is the list of sections that are being decoded in the current config. This can be used to
	// indicate/detect that the current parsing ctx is partial, meaning that not all configuration values are
	// expected to be available.
	PartialParseDecodeList []PartialDecodeSectionType

	// ParserOptions is used to configure hcl Parser.
	ParserOptions []hclparse.Option

	// SkipOutputsResolution is used to optionally opt-out of resolving outputs.
	SkipOutputsResolution bool
}

func NewParsingContext(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (context.Context, *ParsingContext) {
	ctx = tf.ContextWithTerraformCommandHook(ctx, nil)

	filesRead := make([]string, 0)

	return ctx, &ParsingContext{
		TerragruntOptions: opts,
		ParserOptions:     DefaultParserOptions(l, opts),
		FilesRead:         &filesRead,
	}
}

// Clone returns a shallow copy of the ParsingContext.
func (ctx ParsingContext) Clone() *ParsingContext {
	return &ctx
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

func (ctx ParsingContext) WithValues(values *cty.Value) *ParsingContext {
	ctx.Values = values
	return &ctx
}

// WithFeatures sets the feature flags to be used in evaluation context.
func (ctx ParsingContext) WithFeatures(features *cty.Value) *ParsingContext {
	ctx.Features = features

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

// WithDiagnosticsSuppressed returns a new ParsingContext with diagnostics suppressed.
// Diagnostics are written to stderr in debug mode for troubleshooting, otherwise discarded.
// This avoids false positive "There is no variable named dependency" errors during parsing
// when dependency outputs haven't been resolved yet.
func (ctx ParsingContext) WithDiagnosticsSuppressed(l log.Logger) *ParsingContext {
	var diagWriter = io.Discard
	if l.Level() >= log.DebugLevel {
		diagWriter = os.Stderr
	}

	opts := append(ctx.ParserOptions, hclparse.WithDiagnosticsWriter(diagWriter, true))
	ctx.ParserOptions = opts

	return &ctx
}

func (ctx ParsingContext) WithSkipOutputsResolution() *ParsingContext {
	ctx.SkipOutputsResolution = true
	return &ctx
}

func (ctx ParsingContext) WithDecodedDependencies(v *cty.Value) *ParsingContext {
	ctx.DecodedDependencies = v
	return &ctx
}
