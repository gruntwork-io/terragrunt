package config

import (
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

// ParsingContext is what a parse of a config reads: the run's settings, shared by every context derived from the one
// that holds them, and the state of the file being parsed.
//
// A context derived with a With method shares its settings unless the method changes one, so a write to a field of
// [RunSettings] goes only to a context from [NewParsingContext] or [ParsingContext.Clone].
//
// A [Store] resolves a context it is given as a clone, so a resolution leaves the caller's context unchanged.
//
// Note: context.Context should be passed explicitly as the first parameter to functions, not embedded in this struct.
type ParsingContext struct {
	// RunSettings is the settings of the run, which contexts derived from this one share.
	*RunSettings

	// includes is the include blocks of the file and the include block it is parsed through, nil until the include
	// blocks decode.
	includes *Includes

	// include is the include block the Store parses the file through, nil for the component's own config.
	include *IncludeConfig

	// FilesRead records every file the parse reads. Nil keeps no record.
	FilesRead *FilesRead

	// DecodedDependencies is the `dependency` value expressions in the file read, nil until it is resolved.
	DecodedDependencies *cty.Value

	// Values is the `values` value expressions in the file read.
	Values *cty.Value

	// Features is the `feature` value expressions in the file read.
	Features *cty.Value

	// Locals is the `local` value expressions in the file read.
	Locals *cty.Value

	// Path is the `path` value of the file being parsed.
	Path *cty.Value

	// dependencyOutputEnvKeys records environment keys set for dependency output resolution through
	// extra_arguments.env_vars. Inherited process values stay eligible for direct state reads until
	// output-specific configuration overrides them.
	dependencyOutputEnvKeys map[string]struct{}

	// catalogStubDir is the directory of the child config [ReadCatalogConfig] generates, which exists only in
	// memory, so run_cmd runs in its parent beside the root config. It is empty when no child was generated.
	catalogStubDir string

	// key is [Key.Context] for the file: the digest of the run's settings and of the chain of files the parse
	// went through to reach it. Set only when keyed is.
	key string

	// stackFile is the stack file that the context evaluates files for, and empty for the files of a unit.
	stackFile string

	// IAMRoleOptions is the IAM role the file's functions and dependency output fetches assume.
	IAMRoleOptions iam.RoleOptions

	// readConfigChain lists the configs being parsed by nested read_terragrunt_config calls,
	// outermost first: each read target, plus each config a dependency block parses in between.
	// Clones share it, so it is only ever extended with slices.Concat.
	readConfigChain []readConfigFrame

	// DependencyBlocks are the dependency blocks DecodedDependencies was resolved from.
	DependencyBlocks []Dependency

	// Parser configures the HCL parsers this context builds.
	Parser ParserSettings

	// ParseDepth is the number of parses the parse of the file is nested in, at most [MaxParseDepth].
	ParseDepth int

	// SkipOutputsResolution stops the parse fetching dependency outputs. A partial parse with no decoded
	// dependencies then evaluates `dependency` references to unknown values.
	SkipOutputsResolution bool

	// SkipAutoIncludeMerge is set on contexts that parse the files an autoinclude pulls in through its
	// own include blocks, so those files do not re-merge a sibling autoinclude. This bounds the merge to
	// the unit being parsed and prevents an autoinclude that includes another file from recursing.
	SkipAutoIncludeMerge bool

	// catalogOnly decodes only the catalog block, for [ReadCatalogConfig].
	catalogOnly bool

	// stubWorkingDirFunc makes get_working_dir return an empty string, for the parse
	// get_working_dir runs to find the terraform source.
	stubWorkingDirFunc bool

	// keyed reports that a [Store] keyed the context for its resolution.
	keyed bool

	// memoized reports that the Store shares the stages of the parse with other callers.
	memoized bool

	// merging marks a parse whose config only merges into the config of the including file.
	merging bool

	// inheritsDependencies marks an included file whose `dependency` value the including file resolves in
	// ResolveRunConfig.
	inheritsDependencies bool
}

// RunSettings is the settings of the run a [ParsingContext] parses for.
type RunSettings struct {
	// TerraformCliArgs is the tofu command and arguments the run executes.
	TerraformCliArgs *iacargs.IacArgs

	// EngineConfig is the engine block of the unit the run targets, nil when it configures none.
	EngineConfig *engine.EngineConfig

	// EngineOptions is the engine options passed on the CLI.
	EngineOptions *engine.EngineOptions

	// FeatureFlags contains explicit feature flag overrides supplied by the user.
	FeatureFlags map[string]string

	// Telemetry configures the run's traces and metrics.
	Telemetry *telemetry.Options

	// SourceMap replaces the prefix of a terraform source URL, from the --source-map flag.
	SourceMap map[string]string

	// TerragruntConfigPath is the path of the config being parsed.
	TerragruntConfigPath string

	// OriginalTerragruntConfigPath is the path of the config the run started from, which
	// get_original_terragrunt_dir() reads.
	OriginalTerragruntConfigPath string

	// WorkingDir is the directory of the config being parsed.
	WorkingDir string

	// RootWorkingDir is the working directory the run started in, the same for every unit and dependency.
	RootWorkingDir string

	// DownloadDir is the directory the run downloads terraform sources into.
	DownloadDir string

	// Source overrides the terraform source of the unit, from the --source flag.
	Source string

	// TerraformCommand is the name of the tofu command the run executes, such as plan.
	TerraformCommand string

	// OriginalTofuCommand is the tofu command the run started with, before any command it runs in its place.
	OriginalTofuCommand string

	// AuthProviderCmd is the command whose output gives the credentials for the parse, from the
	// --auth-provider-cmd flag.
	AuthProviderCmd string

	// TFPath is the tofu binary that --tf-path or a unit's terraform_binary named. Empty when neither did, and the
	// binary is then the default detected on PATH.
	TFPath string

	// ScaffoldRootFileName is the name of the root config a catalog reads, from the --root-file-name flag.
	ScaffoldRootFileName string

	// TerragruntStackConfigPath is the path of the stack file the run generates from.
	TerragruntStackConfigPath string

	// OriginalIAMRoleOptions is the IAM role options passed on the CLI.
	OriginalIAMRoleOptions iam.RoleOptions

	// Experiments is the experiments the run enables.
	Experiments experiment.Experiments

	// StrictControls is the strict controls the run enables.
	StrictControls strict.Controls

	// PartialParseDecodeList is the sections a partial parse decodes. Empty for a full parse.
	PartialParseDecodeList []PartialDecodeSectionType

	// VersionManagerFileName is the files whose contents key the cache of tofu -version checks.
	VersionManagerFileName []string

	// ProviderCacheOptions configures the provider cache.
	ProviderCacheOptions pcoptions.ProviderCacheOptions

	// MaxFoldersToCheck is the most parent directories a search such as find_in_parent_folders() checks before
	// it fails.
	MaxFoldersToCheck int

	// CASCloneDepth is the --depth CAS passes to git clone. A negative value clones the whole history.
	CASCloneDepth int

	// CASProbeTTL is how long CAS trusts a persisted probe of a branch, HEAD, or non-version tag before
	// querying the remote again. Zero re-queries on every run.
	CASProbeTTL time.Duration

	// TFPathExplicitlySet reports that --tf-path named the tofu binary, which then wins over a unit's
	// terraform_binary.
	TFPathExplicitlySet bool

	// SkipOutput skips fetching the outputs of every dependency.
	SkipOutput bool

	// ForwardTFStdout forwards tofu's output unformatted.
	ForwardTFStdout bool

	// JSONLogFormat writes logs as JSON.
	JSONLogFormat bool

	// Debug runs in debug mode.
	Debug bool

	// AutoInit runs tofu init when a command needs it.
	AutoInit bool

	// Headless reports that Terragrunt runs in headless mode, which changes how tofu output is logged.
	Headless bool

	// BackendBootstrap bootstraps the backend before using it.
	BackendBootstrap bool

	// CheckDependentUnits shows the units that depend on the unit and asks for confirmation before a destroy.
	CheckDependentUnits bool

	// NoDependencyFetchOutputFromState stops dependencies reading their outputs straight from state.
	NoDependencyFetchOutputFromState bool

	// NoStackValidate skips validating generated stacks.
	NoStackValidate bool

	// NoCAS stops sources resolving through the content addressable store.
	NoCAS bool

	// CASOffline stops CAS contacting a Git remote. Git sources are answered from the local store and the
	// persisted probe cache or fail.
	CASOffline bool

	// CASRefresh makes CAS ignore its persisted probe cache.
	CASRefresh bool

	// LogShowAbsPaths shows absolute paths in logs in place of paths relative to the working directory.
	LogShowAbsPaths bool

	// LogDisableErrorSummary leaves out the summary of errors at the end of the run.
	LogDisableErrorSummary bool
}

// NewParsingContext builds a parsing context from opts.
//
// The returned context keeps no record of the files it reads. Recording them
// costs a walk of every local module a config sources, and only a caller that
// surfaces the record has any use for it, so those call
// [ParsingContext.WithFileReadTracking] and the rest pay nothing.
func NewParsingContext(opts ...Option) *ParsingContext {
	pctx := &ParsingContext{
		RunSettings: &RunSettings{TerraformCliArgs: iacargs.New()},
	}

	for _, opt := range opts {
		opt(pctx)
	}

	pctx.Parser = DefaultParserSettings(pctx.StrictControls)

	return pctx
}

// Clone returns a copy of the ParsingContext with its own settings. Its maps and slices are deep-copied.
func (ctx *ParsingContext) Clone() *ParsingContext {
	clone := ctx.detach()
	clone.RunSettings = new(*ctx.RunSettings)

	if ctx.SourceMap != nil {
		clone.SourceMap = maps.Clone(ctx.SourceMap)
	}

	if ctx.EngineOptions != nil {
		eo := *ctx.EngineOptions
		clone.EngineOptions = &eo
	}

	clone.ProviderCacheOptions.RegistryNames = slices.Clone(ctx.ProviderCacheOptions.RegistryNames)

	return clone
}

// WithDecodeList returns a copy whose partial parses decode only the sections in decodeList.
func (ctx *ParsingContext) WithDecodeList(decodeList ...PartialDecodeSectionType) *ParsingContext {
	c := ctx.detach()
	c.RunSettings = new(*ctx.RunSettings)
	c.PartialParseDecodeList = decodeList

	return c
}

// decodes reports whether a parse with ctx decodes section: a full parse decodes every section, and a partial parse
// those of its decode list.
func (ctx *ParsingContext) decodes(section PartialDecodeSectionType) bool {
	return len(ctx.PartialParseDecodeList) == 0 || slices.Contains(ctx.PartialParseDecodeList, section)
}

// WithLocals returns a copy that exposes locals to expressions as `local`.
func (ctx *ParsingContext) WithLocals(locals *cty.Value) *ParsingContext {
	c := ctx.detach()
	c.Locals = locals

	return c
}

// WithValues returns a copy that exposes values to expressions as `values`.
func (ctx *ParsingContext) WithValues(values *cty.Value) *ParsingContext {
	c := ctx.detach()
	c.Values = values

	return c
}

// WithFeatures sets the feature flags to be used in evaluation context.
func (ctx *ParsingContext) WithFeatures(features *cty.Value) *ParsingContext {
	c := ctx.detach()
	c.Features = features

	return c
}

// WithTrackInclude returns a copy that resolves includes and include functions against trackInclude.
func (ctx *ParsingContext) WithTrackInclude(trackInclude *TrackInclude) *ParsingContext {
	c := ctx.detach()
	c.includes = newIncludes(trackInclude)

	return c
}

// WithParserSettings returns a copy whose parsers use s.
func (ctx *ParsingContext) WithParserSettings(s ParserSettings) *ParsingContext {
	c := ctx.detach()
	c.Parser = s
	c.Parser.HaltOnErrorOnlyInBlocks = slices.Clone(s.HaltOnErrorOnlyInBlocks)

	return c
}

// WithDiagnosticsSuppressed returns a copy whose parsers write diagnostics to stderr at debug
// level and discard them otherwise. This avoids false positive "There is no variable named
// dependency" errors while dependency outputs are not yet resolved.
func (ctx *ParsingContext) WithDiagnosticsSuppressed() *ParsingContext {
	c := ctx.detach()
	c.Parser.Diagnostics = DiagnosticsSuppressed

	return c
}

// WithDiagnosticsDiscarded returns a copy whose parsers discard diagnostics.
func (ctx *ParsingContext) WithDiagnosticsDiscarded() *ParsingContext {
	c := ctx.detach()
	c.Parser.Diagnostics = DiagnosticsDiscarded

	return c
}

// ParserOptions returns the [hclparse.Option] list for this context's parser settings.
func (ctx *ParsingContext) ParserOptions(l log.Logger, v *venv.Venv) []hclparse.Option {
	opts := ParserOptions(l, v, ctx.Parser)

	if !ctx.Experiments.Evaluate(experiment.PathVariable) {
		opts = slices.Insert(opts, 0, pathExperimentHintOption)
	}

	if !ctx.Experiments.Evaluate(experiment.CommandsVariable) {
		opts = slices.Insert(opts, 0, commandsExperimentHintOption)
	}

	return opts
}

// NewParser returns an HCL parser configured by this context's parser settings.
func (ctx *ParsingContext) NewParser(l log.Logger, v *venv.Venv) *hclparse.Parser {
	return hclparse.NewParser(ctx.ParserOptions(l, v)...)
}

// WithFileReadTracking returns a copy that records every file it reads, so that
// the caller can read them back off [ParsingContext.FilesRead] once parsing is
// done. Clones made from the returned context share the one record.
func (ctx *ParsingContext) WithFileReadTracking() *ParsingContext {
	c := ctx.detach()
	c.FilesRead = NewFilesRead()

	return c
}

// WithSkipOutputsResolution returns a copy for parses that do not fetch dependency outputs. A partial parse with no
// decoded dependencies then evaluates `dependency` references to unknown values.
func (ctx *ParsingContext) WithSkipOutputsResolution() *ParsingContext {
	c := ctx.detach()
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

	c := ctx.detach()
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

	c := ctx.detach()
	c.RunSettings = new(*ctx.RunSettings)

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
	c.Path = nil

	return l, c, nil
}

// WithStackConfigPath returns a new ParsingContext that evaluates the stack file at cfgPath and the files it reads.
func (ctx *ParsingContext) WithStackConfigPath(l log.Logger, cfgPath string) (log.Logger, *ParsingContext, error) {
	l, c, err := ctx.WithConfigPath(l, cfgPath)
	if err != nil {
		return l, nil, err
	}

	return l, c.forStackFile(c.TerragruntConfigPath), nil
}

// WithDependencyConfigPath returns a new ParsingContext for parsing a dependency as an independent unit.
//
// It performs the same path and logger updates as [ParsingContext.WithConfigPath], and resets
// OriginalTerragruntConfigPath to the dependency's path, so get_original_terragrunt_dir() resolves to the
// dependency's own directory. The dependency evaluates its own locals, feature flags, values, includes, and
// dependency blocks, never those of the config whose dependency it is, and assumes no IAM role that config
// assumed beyond the one the command started with. It decodes all of its blocks even when that config is read only
// for its catalog block.
func (ctx *ParsingContext) WithDependencyConfigPath(
	l log.Logger,
	cfgPath string,
) (log.Logger, *ParsingContext, error) {
	l, c, err := ctx.WithConfigPath(l, cfgPath)
	if err != nil {
		return l, nil, err
	}

	c.OriginalTerragruntConfigPath = c.TerragruntConfigPath
	c.stackFile = ""
	c.DependencyBlocks = nil
	c.DecodedDependencies = nil
	c.Locals = nil
	c.Features = nil
	c.Values = nil
	c.includes = nil
	c.catalogOnly = false
	c.catalogStubDir = ""

	if c.IAMRoleOptions != c.OriginalIAMRoleOptions {
		c.IAMRoleOptions = iam.RoleOptions{}
	}

	return l, c, nil
}

// derive returns a copy of ctx that shares its settings.
func (ctx *ParsingContext) derive() *ParsingContext {
	c := *ctx
	c.Parser.HaltOnErrorOnlyInBlocks = slices.Clone(ctx.Parser.HaltOnErrorOnlyInBlocks)

	if ctx.dependencyOutputEnvKeys != nil {
		c.dependencyOutputEnvKeys = maps.Clone(ctx.dependencyOutputEnvKeys)
	}

	return &c
}

// detach returns a copy of ctx that shares its settings, which a [Store] keys afresh when it resolves it.
func (ctx *ParsingContext) detach() *ParsingContext {
	c := ctx.derive()
	c.key = ""
	c.keyed = false
	c.memoized = false

	return c
}

// ReadConfigChain returns the configs being parsed by nested read_terragrunt_config calls, outermost first, as
// each config's path followed by the original config it is parsed under.
func (ctx *ParsingContext) ReadConfigChain() []string {
	const stringsPerFrame = 2

	chain := make([]string, 0, stringsPerFrame*len(ctx.readConfigChain))
	for _, frame := range ctx.readConfigChain {
		chain = append(chain, frame.path, frame.originalPath)
	}

	return chain
}

// readConfigFrame is one config on the read_terragrunt_config chain. A dependency block or a stack
// file is parsed under its own original config, so get_original_terragrunt_dir() can make the same
// file evaluate differently. Only a file parsed again under the same original config is a cycle.
type readConfigFrame struct {
	path         string
	originalPath string
}

// readConfigChainPaths returns the paths of chain followed by last, for a cycle error.
func readConfigChainPaths(chain []readConfigFrame, last readConfigFrame) []string {
	paths := make([]string, 0, len(chain)+1)
	for _, frame := range chain {
		paths = append(paths, frame.path)
	}

	return append(paths, last.path)
}

// forReadTarget returns a copy for parsing a config that read_terragrunt_config reads, without the evaluation state
// of the config that reads it.
//
// The reading config's dependency outputs are not the target's, so the target decodes its own dependency blocks, and
// dependency tracing reports that their outputs resolve (https://github.com/gruntwork-io/terragrunt/issues/5624). The
// target evaluates its own locals and feature flags, and its include and feature blocks would otherwise see the
// reading config's values.
func (ctx *ParsingContext) forReadTarget() *ParsingContext {
	c := ctx.detach()
	c.DependencyBlocks = nil
	c.DecodedDependencies = nil
	c.SkipOutputsResolution = false
	c.Locals = nil
	c.Features = nil

	return c
}
