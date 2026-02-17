package config

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/puzpuzpuz/xsync/v3"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
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
	// TerragruntOptions is kept temporarily during the migration. New code should
	// use the flat fields below instead of accessing TerragruntOptions directly.
	TerragruntOptions *options.TerragruntOptions

	// --- Path configuration ---
	TerragruntConfigPath         string
	OriginalTerragruntConfigPath string
	WorkingDir                   string
	RootWorkingDir               string
	DownloadDir                  string

	// --- Terraform command info ---
	TerraformCommand         string
	OriginalTerraformCommand string
	TerraformCliArgs         *iacargs.IacArgs
	Source                   string
	SourceMap                map[string]string

	// --- Feature control ---
	Experiments    experiment.Experiments
	StrictControls strict.Controls
	FeatureFlags   *xsync.MapOf[string, string]

	// --- I/O ---
	Writer    io.Writer
	ErrWriter io.Writer
	Env       map[string]string

	// --- IAM ---
	IAMRoleOptions         iam.RoleOptions
	OriginalIAMRoleOptions iam.RoleOptions

	// --- Behavior flags ---
	UsePartialParseConfigCache      bool
	MaxFoldersToCheck               int
	NoDependencyFetchOutputFromState bool
	SkipOutput                      bool
	TFPathExplicitlySet             bool
	LogShowAbsPaths                 bool
	AuthProviderCmd                 string

	// --- Engine ---
	Engine *options.EngineOptions

	// --- Parsing state fields ---

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

	// ParseDepth tracks the current parsing recursion depth.
	// This prevents stack overflow from deeply nested configs.
	ParseDepth int
}

// populateFromOpts copies fields from TerragruntOptions into the flat fields.
func (pctx *ParsingContext) populateFromOpts(opts *options.TerragruntOptions) {
	pctx.TerragruntConfigPath = opts.TerragruntConfigPath
	pctx.OriginalTerragruntConfigPath = opts.OriginalTerragruntConfigPath
	pctx.WorkingDir = opts.WorkingDir
	pctx.RootWorkingDir = opts.RootWorkingDir
	pctx.DownloadDir = opts.DownloadDir
	pctx.TerraformCommand = opts.TerraformCommand
	pctx.OriginalTerraformCommand = opts.OriginalTerraformCommand
	pctx.TerraformCliArgs = opts.TerraformCliArgs
	pctx.Source = opts.Source
	pctx.SourceMap = opts.SourceMap
	pctx.Experiments = opts.Experiments
	pctx.StrictControls = opts.StrictControls
	pctx.FeatureFlags = opts.FeatureFlags
	pctx.Writer = opts.Writer
	pctx.ErrWriter = opts.ErrWriter
	pctx.Env = opts.Env
	pctx.IAMRoleOptions = opts.IAMRoleOptions
	pctx.OriginalIAMRoleOptions = opts.OriginalIAMRoleOptions
	pctx.UsePartialParseConfigCache = opts.UsePartialParseConfigCache
	pctx.MaxFoldersToCheck = opts.MaxFoldersToCheck
	pctx.NoDependencyFetchOutputFromState = opts.NoDependencyFetchOutputFromState
	pctx.SkipOutput = opts.SkipOutput
	pctx.TFPathExplicitlySet = opts.TFPathExplicitlySet
	pctx.LogShowAbsPaths = opts.LogShowAbsPaths
	pctx.AuthProviderCmd = opts.AuthProviderCmd
	pctx.Engine = opts.Engine
}

func NewParsingContext(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (context.Context, *ParsingContext) {
	ctx = tf.ContextWithTerraformCommandHook(ctx, nil)

	filesRead := make([]string, 0)

	pctx := &ParsingContext{
		TerragruntOptions: opts,
		ParserOptions:     DefaultParserOptions(l, opts.StrictControls),
		FilesRead:         &filesRead,
	}
	pctx.populateFromOpts(opts)

	return ctx, pctx
}

// Clone returns a shallow copy of the ParsingContext.
func (ctx *ParsingContext) Clone() *ParsingContext {
	clone := *ctx
	return &clone
}

func (ctx *ParsingContext) WithDecodeList(decodeList ...PartialDecodeSectionType) *ParsingContext {
	c := ctx.Clone()
	c.PartialParseDecodeList = decodeList

	return c
}

func (ctx *ParsingContext) WithTerragruntOptions(opts *options.TerragruntOptions) *ParsingContext {
	c := ctx.Clone()
	c.TerragruntOptions = opts
	c.populateFromOpts(opts)

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
// During the transition period, it also updates TerragruntOptions to stay in sync.
func (ctx *ParsingContext) WithConfigPath(l log.Logger, configPath string) (log.Logger, *ParsingContext, error) {
	configPath = util.CleanPath(configPath)
	if !filepath.IsAbs(configPath) {
		absConfigPath, err := filepath.Abs(configPath)
		if err != nil {
			return l, nil, err
		}

		configPath = util.CleanPath(absConfigPath)
	}

	workingDir := filepath.Dir(configPath)

	if workingDir != ctx.WorkingDir {
		l = l.WithField(placeholders.WorkDirKeyName, workingDir)
	}

	c := ctx.Clone()
	c.TerragruntConfigPath = configPath
	c.WorkingDir = workingDir

	// Keep TerragruntOptions in sync during the transition period.
	if c.TerragruntOptions != nil {
		newOpts := c.TerragruntOptions.Clone()
		newOpts.TerragruntConfigPath = configPath
		newOpts.WorkingDir = workingDir
		c.TerragruntOptions = newOpts
	}

	return l, c, nil
}
