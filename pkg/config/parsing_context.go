package config

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	pcoptions "github.com/gruntwork-io/terragrunt/internal/providercache/options"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
)

const (
	// MaxParseDepth limits nested parsing to prevent stack overflow
	// from deeply recursive config structures (includes, dependencies, etc.).
	MaxParseDepth = 1000
)

// ErrParsingContextVenvNil is the panic value [NewParsingContext] raises when
// its venv argument is nil. Every caller is handed the bundle main.go built,
// so a nil points at a caller bug rather than a runtime condition.
var ErrParsingContextVenvNil = errors.New("config.NewParsingContext: venv must not be nil")

// ParsingContext provides various variables that are used throughout all funcs and passed from function to function.
// Using `ParsingContext` makes the code more readable.
// Note: context.Context should be passed explicitly as the first parameter to functions, not embedded in this struct.
type ParsingContext struct {
	// Venv is the virtualized environment used by HCL helper functions
	// that shell out (e.g. get_repo_root) or evaluate dependency outputs.
	// It also carries the shell environment and stdout/stderr writers.
	Venv *venv.Venv

	TerraformCliArgs *iacargs.IacArgs
	TrackInclude     *TrackInclude
	EngineConfig     *engine.EngineConfig
	EngineOptions    *engine.EngineOptions

	// FeatureFlags contains explicit feature flag overrides supplied by the user.
	FeatureFlags map[string]string

	FilesRead *FilesRead
	Telemetry *telemetry.Options

	DecodedDependencies *cty.Value
	Values              *cty.Value
	Features            *cty.Value
	Locals              *cty.Value

	SourceMap map[string]string
	// dependencyOutputEnvKeys records environment keys set for dependency output resolution through
	// extra_arguments.env_vars. Inherited process values stay eligible for direct state reads until
	// output-specific configuration overrides them.
	dependencyOutputEnvKeys map[string]struct{}

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
	ScaffoldRootFileName         string
	TerragruntStackConfigPath    string
	TofuImplementation           tfimpl.Type

	IAMRoleOptions         iam.RoleOptions
	OriginalIAMRoleOptions iam.RoleOptions

	Experiments            experiment.Experiments
	StrictControls         strict.Controls
	PartialParseDecodeList []PartialDecodeSectionType

	// Parser configures the HCL parsers this context builds.
	Parser ParserSettings

	ReadConfigChain []string

	ProviderCacheOptions pcoptions.ProviderCacheOptions

	MaxFoldersToCheck int
	ParseDepth        int
	CASCloneDepth     int
	CASProbeTTL       time.Duration

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
	NoCAS                            bool
	CASOffline                       bool
	CASRefresh                       bool
	LogShowAbsPaths                  bool
	LogDisableErrorSummary           bool

	// skipAutoIncludeMerge is set on contexts that parse the files an autoinclude pulls in through its
	// own include blocks, so those files do not re-merge a sibling autoinclude. This bounds the merge to
	// the unit being parsed and prevents an autoinclude that includes another file from recursing.
	skipAutoIncludeMerge bool

	// catalogOnly decodes only the catalog block, for [ReadCatalogConfig].
	catalogOnly bool

	// stubWorkingDirFunc makes get_working_dir return an empty string, for the parse
	// get_working_dir runs to find the terraform source.
	stubWorkingDirFunc bool
}

// NewParsingContext builds a parsing context whose file reads, subprocesses,
// and decryption all travel on v.
//
// The returned context keeps no record of the files it reads. Recording them
// costs a walk of every local module a config sources, and only a caller that
// surfaces the record has any use for it, so those call
// [ParsingContext.WithFileReadTracking] and the rest pay nothing.
func NewParsingContext(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts ...Option,
) (context.Context, *ParsingContext) {
	if v == nil {
		panic(ErrParsingContextVenvNil)
	}

	pctx := &ParsingContext{
		TerraformCliArgs: iacargs.New(),
		Venv:             v,
	}

	for _, opt := range opts {
		opt(pctx)
	}

	pctx.Parser = DefaultParserSettings(ctx, pctx.StrictControls)

	return ctx, pctx
}

// Clone returns a copy of the ParsingContext.
// Maps and the embedded Venv (including its Writers pointer and Env map)
// are deep-copied so that mutations on a clone (credential injection,
// writer redirection, and so on) do not affect the original or other clones.
func (ctx *ParsingContext) Clone() *ParsingContext {
	clone := *ctx

	if ctx.Venv != nil {
		v := *ctx.Venv
		if v.Env != nil {
			v.Env = maps.Clone(v.Env)
		}

		if v.Writers != nil {
			w := *v.Writers
			v.Writers = &w
		}

		clone.Venv = &v
	}

	if ctx.SourceMap != nil {
		clone.SourceMap = maps.Clone(ctx.SourceMap)
	}

	if ctx.EngineOptions != nil {
		eo := *ctx.EngineOptions
		clone.EngineOptions = &eo
	}

	clone.Parser.HaltOnErrorOnlyInBlocks = slices.Clone(ctx.Parser.HaltOnErrorOnlyInBlocks)

	clone.ProviderCacheOptions.RegistryNames = slices.Clone(ctx.ProviderCacheOptions.RegistryNames)

	if ctx.dependencyOutputEnvKeys != nil {
		clone.dependencyOutputEnvKeys = maps.Clone(ctx.dependencyOutputEnvKeys)
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

// WithParserSettings returns a copy whose parsers use s.
func (ctx *ParsingContext) WithParserSettings(s ParserSettings) *ParsingContext {
	c := ctx.Clone()
	c.Parser = s
	c.Parser.HaltOnErrorOnlyInBlocks = slices.Clone(s.HaltOnErrorOnlyInBlocks)

	return c
}

// WithDiagnosticsSuppressed returns a copy whose parsers write diagnostics to stderr at debug
// level and discard them otherwise. This avoids false positive "There is no variable named
// dependency" errors while dependency outputs are not yet resolved.
func (ctx *ParsingContext) WithDiagnosticsSuppressed() *ParsingContext {
	c := ctx.Clone()
	c.Parser.Diagnostics = DiagnosticsSuppressed

	return c
}

// WithDiagnosticsDiscarded returns a copy whose parsers discard diagnostics.
func (ctx *ParsingContext) WithDiagnosticsDiscarded() *ParsingContext {
	c := ctx.Clone()
	c.Parser.Diagnostics = DiagnosticsDiscarded

	return c
}

// ParserOptions returns the [hclparse.Option] list for this context's parser settings.
func (ctx *ParsingContext) ParserOptions(l log.Logger) []hclparse.Option {
	return ParserOptions(l, ctx.Venv, ctx.Parser)
}

// NewParser returns an HCL parser configured by this context's parser settings.
func (ctx *ParsingContext) NewParser(l log.Logger) *hclparse.Parser {
	return hclparse.NewParser(ctx.ParserOptions(l)...)
}

// WithFileReadTracking returns a copy that records every file it reads, so that
// the caller can read them back off [ParsingContext.FilesRead] once parsing is
// done. Clones made from the returned context share the one record.
func (ctx *ParsingContext) WithFileReadTracking() *ParsingContext {
	c := ctx.Clone()
	c.FilesRead = NewFilesRead()

	return c
}

func (ctx *ParsingContext) WithSkipOutputsResolution() *ParsingContext {
	c := ctx.Clone()
	c.SkipOutputsResolution = true

	return c
}

// WithIncrementedDepth returns a new ParsingContext with incremented parse depth.
// Returns an error if the maximum depth would be exceeded.
func (ctx *ParsingContext) WithIncrementedDepth() (*ParsingContext, error) {
	if ctx.ParseDepth > MaxParseDepth {
		return nil, MaxParseDepthError{
			Depth: ctx.ParseDepth,
			Max:   MaxParseDepth,
		}
	}

	c := ctx.Clone()
	c.ParseDepth = ctx.ParseDepth + 1

	return c, nil
}

// WithConfigPath returns a new ParsingContext targeting a different config file.
//
// It normalizes cfgPath to an absolute path, sets TerragruntConfigPath and
// WorkingDir accordingly, and updates the logger when the working directory changes.
//
// OriginalTerragruntConfigPath is preserved so that get_original_terragrunt_dir()
// continues to resolve to the caller. This is the correct behavior when one config
// reads another via read_terragrunt_config.
//
// To parse a dependency as an independent unit, use [ParsingContext.WithDependencyConfigPath].
func (ctx *ParsingContext) WithConfigPath(
	l log.Logger,
	cfgPath string,
) (log.Logger, *ParsingContext, error) {
	cfgPath = filepath.Clean(cfgPath)
	if !filepath.IsAbs(cfgPath) {
		cfgPath = filepath.Clean(filepath.Join(ctx.WorkingDir, cfgPath))
	}

	workingDir := filepath.Dir(cfgPath)

	if workingDir != ctx.WorkingDir {
		l = l.WithField(placeholders.WorkDirKeyName, workingDir)
	}

	c := ctx.Clone()

	// Keep DownloadDir in sync with TerragruntConfigPath: if the current context was
	// using the default download dir for the old config, update it to the default for
	// the new config. This ensures that when read_terragrunt_config() or dependency
	// processing switches to a different module, DownloadDir reflects the new module's
	// default rather than an inherited stale default from an ancestor. User-set custom
	// dirs (which won't match any module's default) are preserved unchanged.
	_, defaultDir := util.DefaultWorkingAndDownloadDirs(ctx.TerragruntConfigPath)
	if filepath.Clean(c.DownloadDir) == filepath.Clean(defaultDir) {
		_, c.DownloadDir = util.DefaultWorkingAndDownloadDirs(cfgPath)
	}

	c.TerragruntConfigPath = cfgPath
	c.WorkingDir = workingDir

	return l, c, nil
}

// WithDependencyConfigPath returns a new ParsingContext for parsing a dependency
// as an independent unit.
//
// It performs the same path and logger updates as [ParsingContext.WithConfigPath],
// and additionally resets OriginalTerragruntConfigPath to the dependency's path.
// This ensures that get_original_terragrunt_dir() resolves to the dependency's
// own directory rather than the caller's.
func (ctx *ParsingContext) WithDependencyConfigPath(
	l log.Logger,
	cfgPath string,
) (log.Logger, *ParsingContext, error) {
	l, c, err := ctx.WithConfigPath(l, cfgPath)
	if err != nil {
		return l, nil, err
	}

	c.OriginalTerragruntConfigPath = c.TerragruntConfigPath

	return l, c, nil
}
