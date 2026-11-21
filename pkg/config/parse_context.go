package config

import (
	"strconv"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ParseContext is what a caller brings to a stage: the settings a parse reads, the sinks it writes its effects to, and
// the state of the file being parsed. settings never changes, and holds none of the sinks of the
// [ParsingContext] it came from.
type ParseContext struct {
	// settings is the settings the parse reads, with no sinks.
	settings *ParsingContext
	// sinks is where the parse writes its effects.
	sinks sinks
	// file is the state of the file being parsed.
	file fileScope
	// memoized reports that the Store shares the parse's stages with other callers.
	memoized bool
	// merging marks a parse whose config only merges into the config of the including file.
	merging bool
}

// sinks is where a stage writes its effects. Sinks belong to the caller, and a stage's result holds none.
type sinks struct {
	// filesRead records the files the stage reads. Nil keeps no record.
	filesRead *FilesRead
	// diagnostics receives the diagnostics of the stage's parsers.
	diagnostics func(*hcl.File, hcl.Diagnostics) (hcl.Diagnostics, error)
}

// fileScope is the state of the file a parse is in.
type fileScope struct {
	// key is [Key.Context] for the file: the digest of the run's settings and of the chain of files the parse
	// went through to reach it.
	key string
	// include is the include block the file is parsed through, nil for the component's own config.
	include *IncludeConfig
	// includes is the file's include blocks, nil until the base blocks decode.
	includes *Includes
	// values is the file's unit values.
	values *cty.Value
	// features is the file's feature flags.
	features *cty.Value
	// locals is the file's evaluated locals.
	locals *cty.Value
	// decodedDeps is the file's `dependency` value and the blocks it was resolved from, nil when the parse resolves
	// them.
	decodedDeps *dependencyResolution
	// iamRole overrides the run's IAM role options when set.
	iamRole *iam.RoleOptions
	// parser configures the parsers that read the file.
	parser ParserSettings
	// depth is the number of file parses above this one.
	depth int
	// skipAutoIncludeMerge skips merging the file's autoinclude.
	skipAutoIncludeMerge bool
	// inheritsDependencies marks an included file whose `dependency` value the including file resolves in ToV1.
	inheritsDependencies bool
}

// NewParseContext returns a ParseContext with run's settings and sinks, and the file state starting from run's values.
// The stages it reaches are shared only with callers whose environment and output discarding match those of v.
func NewParseContext(v *venv.Venv, run *ParsingContext) *ParseContext {
	key, memoized := runDigest(v, run)

	settings := run.Clone()
	settings.FilesRead = nil
	settings.Parser.DiagnosticsHandler = nil

	return &ParseContext{
		settings: settings,
		sinks:    sinks{filesRead: run.FilesRead, diagnostics: run.Parser.DiagnosticsHandler},
		memoized: memoized,
		file: fileScope{
			key:                  key,
			includes:             newIncludes(run.TrackInclude),
			values:               run.Values,
			features:             run.Features,
			locals:               run.Locals,
			decodedDeps:          runDependencies(run),
			parser:               settings.Parser,
			depth:                run.ParseDepth,
			skipAutoIncludeMerge: run.SkipAutoIncludeMerge,
		},
	}
}

// StoreContext returns [Key.Context] for the file pc parses, and false when the parse is not memoized.
func (pc *ParseContext) StoreContext() (string, bool) {
	return pc.file.key, pc.memoized
}

// withInclude returns a copy of pc for the file include pulls in, which has no include blocks until its base
// blocks decode. The file reads the `dependency` value of the including file unless the run supplied one. A nil
// include is a file no include block pulls in, which resolves its own dependencies. The copy drops any forMerge
// marking of pc.
func (pc *ParseContext) withInclude(include *IncludeConfig) *ParseContext {
	c := *pc
	c.file.include = include
	c.file.includes = nil
	c.file.inheritsDependencies = include != nil && c.file.decodedDeps == nil
	c.merging = false

	return &c
}

// forMerge returns a copy of pc for assembling a config that only merges into the config of the including file.
func (pc *ParseContext) forMerge() *ParseContext {
	c := *pc
	c.merging = true

	return &c
}

// forIncluded returns a copy of pc for the file include pulls in, one step further along the chain of files.
func (pc *ParseContext) forIncluded(include *IncludeConfig) *ParseContext {
	c := pc.withInclude(include)
	c.file.key = extend(
		c.file.key,
		"include",
		include.Name,
		include.Path,
		formatBoolPtr(include.Expose),
		formatStringPtr(include.MergeStrategy),
	)

	return c
}

// forAutoInclude returns a copy of pc for the file's autoinclude, which no include block pulls in, resolves its
// own dependencies, and merges no autoinclude of its own.
func (pc *ParseContext) forAutoInclude() *ParseContext {
	c := *pc
	c.file.include = nil
	c.file.decodedDeps = nil
	c.file.inheritsDependencies = false
	c.file.skipAutoIncludeMerge = true
	c.file.key = extend(c.file.key, "autoinclude")

	return &c
}

// withFile returns a copy of pc for the file whose state is file.
func (pc *ParseContext) withFile(file *fileScope) *ParseContext {
	c := *pc
	c.file = *file

	return &c
}

// withOwnReads returns a copy of pc that records its reads apart from the caller's. A caller that keeps no record
// still keeps none.
func (pc *ParseContext) withOwnReads() *ParseContext {
	c := *pc

	if pc.sinks.filesRead.Tracking() {
		c.sinks.filesRead = NewFilesRead()
	}

	return &c
}

// withBaseBlocks returns a copy of pc with the include blocks, feature flags, and locals baseBlocks decoded.
func (pc *ParseContext) withBaseBlocks(baseBlocks *DecodedBaseBlocks) *ParseContext {
	c := *pc
	c.file.includes = newIncludes(baseBlocks.TrackInclude)
	c.file.features = baseBlocks.FeatureFlags
	c.file.locals = baseBlocks.Locals

	return &c
}

// withValues returns a copy of pc with the file's unit values.
func (pc *ParseContext) withValues(values *cty.Value) *ParseContext {
	c := *pc
	c.file.values = values

	return &c
}

// withDecodedDependencies returns a copy of pc with the file's `dependency` value and the blocks it was resolved
// from.
func (pc *ParseContext) withDecodedDependencies(decodedDeps *dependencyResolution) *ParseContext {
	c := *pc
	c.file.decodedDeps = decodedDeps

	return &c
}

// withIAMRole returns a copy of pc with iamRole overriding the run's IAM role options.
func (pc *ParseContext) withIAMRole(iamRole iam.RoleOptions) *ParseContext {
	c := *pc
	c.file.iamRole = &iamRole

	return &c
}

// forIAMRole returns a copy of pc whose file evaluates with iamRole and memoizes apart from its resolution with pc.
// A partial parse of the file keeps iamRole and resolves no role of its own.
func (pc *ParseContext) forIAMRole(iamRole iam.RoleOptions) *ParseContext {
	c := pc.withIAMRole(iamRole)

	d := newDigest()
	d.str(c.file.key)
	d.str("iam-role")
	d.roleOptions(iamRole)
	c.file.key = d.sum()

	return c
}

// forTerragruntFlags returns a copy of pc for a partial parse of the file that decodes its Terragrunt flags and
// reports no diagnostics. It memoizes apart from pc.
func (pc *ParseContext) forTerragruntFlags() *ParseContext {
	c := pc.withDiagnosticsSuppressed()
	c.settings = pc.settings.WithDecodeList(TerragruntFlags)
	c.file.key = extend(c.file.key, "terragrunt-flags")

	return c
}

// withDiagnosticsSuppressed returns a copy of pc whose parsers report no diagnostics.
func (pc *ParseContext) withDiagnosticsSuppressed() *ParseContext {
	c := *pc
	c.file.parser.Diagnostics = DiagnosticsSuppressed
	c.file.key = extend(c.file.key, "diagnostics-suppressed")

	return &c
}

// incrementDepth returns a copy of pc one parse deeper.
//
// Returns [MaxParseDepthError] when pc is already past [MaxParseDepth].
func (pc *ParseContext) incrementDepth() (*ParseContext, error) {
	if pc.file.depth > MaxParseDepth {
		return nil, MaxParseDepthError{
			Depth: pc.file.depth,
			Max:   MaxParseDepth,
		}
	}

	c := *pc
	c.file.depth++
	c.file.key = extend(c.file.key, "depth", strconv.Itoa(c.file.depth))

	return &c, nil
}

// nilText is how a digest, a cache key, or a String method renders a nil value.
const nilText = "nil"

// formatBoolPtr renders p for a digest, telling nil from false.
func formatBoolPtr(p *bool) string {
	if p == nil {
		return nilText
	}

	return strconv.FormatBool(*p)
}

// formatStringPtr renders p for a digest, telling nil from the empty string.
func formatStringPtr(p *string) string {
	if p == nil {
		return nilText
	}

	return strconv.Quote(*p)
}

// parsingContext returns a [ParsingContext] of pc's settings with its sinks and file state applied.
func (pc *ParseContext) parsingContext() *ParsingContext {
	c := pc.settings.derive()
	c.FilesRead = pc.sinks.filesRead
	c.TrackInclude = pc.file.includes.v1()
	c.Values = pc.file.values
	c.Features = pc.file.features
	c.Locals = pc.file.locals
	c.Parser = pc.parserSettings()
	c.ParseDepth = pc.file.depth
	c.SkipAutoIncludeMerge = pc.file.skipAutoIncludeMerge
	c.DecodedDependencies = nil
	c.DependencyBlocks = nil

	if deps := pc.file.decodedDeps; deps != nil {
		c.DecodedDependencies = deps.value
		c.DependencyBlocks = deps.blocks
	}

	if pc.file.iamRole != nil {
		c.IAMRoleOptions = *pc.file.iamRole
	}

	return c
}

// parserOptions returns the options for the parsers that read pc's file, which log through l and write through v.
func (pc *ParseContext) parserOptions(l log.Logger, v *venv.Venv) []hclparse.Option {
	return ParserOptions(l, v, pc.parserSettings())
}

// parserSettings returns the file's parser settings with pc's diagnostics sink applied.
func (pc *ParseContext) parserSettings() ParserSettings {
	parser := pc.file.parser
	parser.DiagnosticsHandler = pc.sinks.diagnostics

	return parser
}
