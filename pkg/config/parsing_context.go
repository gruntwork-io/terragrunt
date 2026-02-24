package config

import (
	"context"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/puzpuzpuz/xsync/v3"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// MaxParseDepth limits nested parsing to prevent stack overflow
	// from deeply recursive config structures (includes, dependencies, etc.).
	MaxParseDepth = 1000
)

// ParsingContext provides various variables that are used throughout all funcs and passed from function to function.
// Using `ParsingContext` makes the code more readable.
// Note: context.Context should be passed explicitly as the first parameter to functions, not embedded in this struct.
type ParsingContext struct {
	Writers writer.Writers

	TerraformCliArgs *iacargs.IacArgs
	TrackInclude     *TrackInclude
	Engine           *engine.EngineConfig
	EngineOptions    *engine.EngineOptions
	FeatureFlags     *xsync.MapOf[string, string]
	FilesRead        *[]string
	Telemetry        *telemetry.Options

	DecodedDependencies *cty.Value
	Values              *cty.Value
	Features            *cty.Value
	Locals              *cty.Value

	Env                 map[string]string
	SourceMap           map[string]string
	PredefinedFunctions map[string]function.Function

	ConvertToTerragruntConfigFunc func(ctx context.Context, pctx *ParsingContext, configPath string, terragruntConfigFromFile *terragruntConfigFile) (cfg *TerragruntConfig, err error)

	TerragruntConfigPath         string
	OriginalTerragruntConfigPath string
	WorkingDir                   string
	RootWorkingDir               string
	DownloadDir                  string
	Source                       string
	TerraformCommand             string
	OriginalTerraformCommand     string
	AuthProviderCmd              string
	TFPath                       string
	TofuImplementation           options.TerraformImplementationType

	IAMRoleOptions         iam.RoleOptions
	OriginalIAMRoleOptions iam.RoleOptions

	Experiments            experiment.Experiments
	StrictControls         strict.Controls
	PartialParseDecodeList []PartialDecodeSectionType
	ParserOptions          []hclparse.Option

	MaxFoldersToCheck int
	ParseDepth        int

	TFPathExplicitlySet bool
	SkipOutput          bool
	ForwardTFStdout     bool
	JSONLogFormat       bool
	Debug               bool
	AutoInit            bool
	Headless            bool
	BackendBootstrap    bool
	CheckDependentUnits bool

	NoDependencyFetchOutputFromState bool
	UsePartialParseConfigCache       bool
	SkipOutputsResolution            bool
	NoStackValidate                  bool
}

// populateFromOpts copies fields from TerragruntOptions into the flat fields.
func (ctx *ParsingContext) populateFromOpts(opts *options.TerragruntOptions) {
	ctx.TerragruntConfigPath = opts.TerragruntConfigPath
	ctx.OriginalTerragruntConfigPath = opts.OriginalTerragruntConfigPath
	ctx.WorkingDir = opts.WorkingDir
	ctx.RootWorkingDir = opts.RootWorkingDir
	ctx.DownloadDir = opts.DownloadDir
	ctx.TerraformCommand = opts.TerraformCommand
	ctx.OriginalTerraformCommand = opts.OriginalTerraformCommand
	ctx.TerraformCliArgs = opts.TerraformCliArgs
	ctx.Source = opts.Source
	ctx.SourceMap = opts.SourceMap
	ctx.Experiments = opts.Experiments
	ctx.StrictControls = opts.StrictControls
	ctx.FeatureFlags = opts.FeatureFlags
	ctx.Writers = opts.Writers
	ctx.Env = opts.Env
	ctx.IAMRoleOptions = opts.IAMRoleOptions
	ctx.OriginalIAMRoleOptions = opts.OriginalIAMRoleOptions
	ctx.UsePartialParseConfigCache = opts.UsePartialParseConfigCache
	ctx.MaxFoldersToCheck = opts.MaxFoldersToCheck
	ctx.NoDependencyFetchOutputFromState = opts.NoDependencyFetchOutputFromState
	ctx.SkipOutput = opts.SkipOutput
	ctx.TFPathExplicitlySet = opts.TFPathExplicitlySet
	ctx.AuthProviderCmd = opts.AuthProviderCmd
	ctx.Engine = opts.EngineConfig
	ctx.EngineOptions = opts.EngineOptions
	ctx.TFPath = opts.TFPath
	ctx.TofuImplementation = opts.TofuImplementation
	ctx.ForwardTFStdout = opts.ForwardTFStdout
	ctx.JSONLogFormat = opts.JSONLogFormat
	ctx.Debug = opts.Debug
	ctx.AutoInit = opts.AutoInit
	ctx.Headless = opts.Headless
	ctx.BackendBootstrap = opts.BackendBootstrap
	ctx.CheckDependentUnits = opts.CheckDependentUnits
	ctx.Telemetry = opts.Telemetry
	ctx.NoStackValidate = opts.NoStackValidate
}

func NewParsingContext(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (context.Context, *ParsingContext) {
	ctx = tf.ContextWithTerraformCommandHook(ctx, nil)

	filesRead := make([]string, 0)

	pctx := &ParsingContext{
		ParserOptions: DefaultParserOptions(l, opts.StrictControls),
		FilesRead:     &filesRead,
	}
	pctx.populateFromOpts(opts)

	return ctx, pctx
}

// Clone returns a copy of the ParsingContext.
// Maps are deep-copied so that mutations (e.g. credential injection into Env)
// on a clone do not affect the original or other clones.
func (ctx *ParsingContext) Clone() *ParsingContext {
	clone := *ctx

	if ctx.Env != nil {
		clone.Env = maps.Clone(ctx.Env)
	}

	if ctx.SourceMap != nil {
		clone.SourceMap = maps.Clone(ctx.SourceMap)
	}

	if ctx.EngineOptions != nil {
		eo := *ctx.EngineOptions
		clone.EngineOptions = &eo
	}

	return &clone
}

func (ctx *ParsingContext) WithDecodeList(decodeList ...PartialDecodeSectionType) *ParsingContext {
	c := ctx.Clone()
	c.PartialParseDecodeList = decodeList

	return c
}

func (ctx *ParsingContext) WithLocals(locals *cty.Value) *ParsingContext {
	c := ctx.Clone()
	c.Locals = locals

	return c
}

func (ctx *ParsingContext) WithValues(values *cty.Value) *ParsingContext {
	c := ctx.Clone()
	c.Values = values

	return c
}

// WithFeatures sets the feature flags to be used in evaluation context.
func (ctx *ParsingContext) WithFeatures(features *cty.Value) *ParsingContext {
	c := ctx.Clone()
	c.Features = features

	return c
}

func (ctx *ParsingContext) WithTrackInclude(trackInclude *TrackInclude) *ParsingContext {
	c := ctx.Clone()
	c.TrackInclude = trackInclude

	return c
}

func (ctx *ParsingContext) WithParseOption(parserOptions []hclparse.Option) *ParsingContext {
	c := ctx.Clone()
	c.ParserOptions = parserOptions

	return c
}

// WithDiagnosticsSuppressed returns a new ParsingContext with diagnostics suppressed.
// Diagnostics are written to stderr in debug mode for troubleshooting, otherwise discarded.
// This avoids false positive "There is no variable named dependency" errors during parsing
// when dependency outputs haven't been resolved yet.
func (ctx *ParsingContext) WithDiagnosticsSuppressed(l log.Logger) *ParsingContext {
	var diagWriter = io.Discard
	if l.Level() >= log.DebugLevel {
		diagWriter = os.Stderr
	}

	c := ctx.Clone()
	c.ParserOptions = slices.Concat(ctx.ParserOptions, []hclparse.Option{hclparse.WithDiagnosticsWriter(diagWriter, true)})

	return c
}

func (ctx *ParsingContext) WithSkipOutputsResolution() *ParsingContext {
	c := ctx.Clone()
	c.SkipOutputsResolution = true

	return c
}

func (ctx *ParsingContext) WithDecodedDependencies(v *cty.Value) *ParsingContext {
	c := ctx.Clone()
	c.DecodedDependencies = v

	return c
}

// WithIncrementedDepth returns a new ParsingContext with incremented parse depth.
// Returns an error if the maximum depth would be exceeded.
func (ctx *ParsingContext) WithIncrementedDepth() (*ParsingContext, error) {
	if ctx.ParseDepth > MaxParseDepth {
		return nil, errors.New(MaxParseDepthError{
			Depth: ctx.ParseDepth,
			Max:   MaxParseDepth,
		})
	}

	c := ctx.Clone()
	c.ParseDepth = ctx.ParseDepth + 1

	return c, nil
}

// WithConfigPath returns a new ParsingContext with the config path updated.
// It normalizes the path to an absolute path, updates WorkingDir to the directory
// containing the config, and adjusts the logger's working directory field if it changed.
func (ctx *ParsingContext) WithConfigPath(l log.Logger, configPath string) (log.Logger, *ParsingContext, error) {
	configPath = filepath.Clean(configPath)
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Clean(filepath.Join(ctx.WorkingDir, configPath))
	}

	workingDir := filepath.Dir(configPath)

	if workingDir != ctx.WorkingDir {
		l = l.WithField(placeholders.WorkDirKeyName, workingDir)
	}

	c := ctx.Clone()
	c.TerragruntConfigPath = configPath
	c.WorkingDir = workingDir

	return l, c, nil
}
