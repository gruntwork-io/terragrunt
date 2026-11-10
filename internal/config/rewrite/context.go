package config

import (
	"strconv"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ParseContext is what a caller brings to a stage: the settings a parse reads, the sinks it writes its effects to, and
// the state of the file being parsed. settings never changes, and holds none of the sinks of the
// [pkgconfig.ParsingContext] it came from.
type ParseContext struct {
	settings *pkgconfig.ParsingContext
	sinks    sinks
	file     fileScope
	memoized bool
}

// sinks is where a stage writes its effects. Sinks belong to the caller, and a stage's result holds none.
type sinks struct {
	filesRead   *pkgconfig.FilesRead
	diagnostics func(*hcl.File, hcl.Diagnostics) (hcl.Diagnostics, error)
}

// fileScope is the per-file state pkg/config keeps on ParsingContext clones.
type fileScope struct {
	// key is [Key.Context] for the file: the digest of the run's settings and of the chain of files the parse
	// went through to reach it.
	key string
	// include is the include block the file is parsed through, nil for the component's own config.
	include *pkgconfig.IncludeConfig
	// includes is the file's include blocks, nil until the base blocks decode.
	includes *Includes
	// values is the file's unit values.
	values *cty.Value
	// features is the file's feature flags.
	features *cty.Value
	// locals is the file's evaluated locals.
	locals *cty.Value
	// decodedDeps is the `dependency` value the run supplied, nil when the parse resolves it.
	decodedDeps *cty.Value
	// iamRole overrides the run's IAM role options when set.
	iamRole *iam.RoleOptions
	// parser configures the parsers that read the file.
	parser pkgconfig.ParserSettings
	// depth is the number of file parses above this one.
	depth int
	// skipAutoIncludeMerge skips merging the file's autoinclude.
	skipAutoIncludeMerge bool
	// inheritsDependencies marks an included file whose `dependency` value the including file resolves in ToV1.
	inheritsDependencies bool
}

// NewParseContext returns a ParseContext with run's settings and sinks, and the file state starting from run's values.
func NewParseContext(run *pkgconfig.ParsingContext) *ParseContext {
	key, memoized := runDigest(run)

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
			decodedDeps:          run.DecodedDependencies,
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
// include is a file no include block pulls in, which resolves its own dependencies.
func (pc *ParseContext) withInclude(include *pkgconfig.IncludeConfig) *ParseContext {
	c := *pc
	c.file.include = include
	c.file.includes = nil
	c.file.inheritsDependencies = include != nil && c.file.decodedDeps == nil

	return &c
}

// forIncluded returns a copy of pc for the file include pulls in, one step further along the chain of files.
func (pc *ParseContext) forIncluded(include *pkgconfig.IncludeConfig) *ParseContext {
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
		c.sinks.filesRead = pkgconfig.NewFilesRead()
	}

	return &c
}

// withBaseBlocks returns a copy of pc with the include blocks, feature flags, and locals baseBlocks decoded.
func (pc *ParseContext) withBaseBlocks(baseBlocks *pkgconfig.DecodedBaseBlocks) *ParseContext {
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

// withDecodedDependencies returns a copy of pc with the file's `dependency` value.
func (pc *ParseContext) withDecodedDependencies(decodedDeps *cty.Value) *ParseContext {
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

// withDiagnosticsSuppressed returns a copy of pc whose parsers report no diagnostics.
func (pc *ParseContext) withDiagnosticsSuppressed() *ParseContext {
	c := *pc
	c.file.parser.Diagnostics = pkgconfig.DiagnosticsSuppressed
	c.file.key = extend(c.file.key, "diagnostics-suppressed")

	return &c
}

// incrementDepth returns a copy of pc one parse deeper.
//
// Returns [pkgconfig.MaxParseDepthError] when pc is already past [pkgconfig.MaxParseDepth].
func (pc *ParseContext) incrementDepth() (*ParseContext, error) {
	if pc.file.depth > pkgconfig.MaxParseDepth {
		return nil, pkgconfig.MaxParseDepthError{
			Depth: pc.file.depth,
			Max:   pkgconfig.MaxParseDepth,
		}
	}

	c := *pc
	c.file.depth++
	c.file.key = extend(c.file.key, "depth", strconv.Itoa(c.file.depth))

	return &c, nil
}

// formatBoolPtr renders p for a digest, telling nil from false.
func formatBoolPtr(p *bool) string {
	if p == nil {
		return "nil"
	}

	return strconv.FormatBool(*p)
}

// formatStringPtr renders p for a digest, telling nil from the empty string.
func formatStringPtr(p *string) string {
	if p == nil {
		return "nil"
	}

	return strconv.Quote(*p)
}

// parsingContext returns a [pkgconfig.ParsingContext] of pc's settings with its sinks and file state applied.
func (pc *ParseContext) parsingContext() *pkgconfig.ParsingContext {
	c := pc.settings.Clone()
	c.FilesRead = pc.sinks.filesRead
	c.TrackInclude = pc.file.includes.v1()
	c.Values = pc.file.values
	c.Features = pc.file.features
	c.Locals = pc.file.locals
	c.DecodedDependencies = pc.file.decodedDeps
	c.Parser = pc.parserSettings()
	c.ParseDepth = pc.file.depth
	c.SkipAutoIncludeMerge = pc.file.skipAutoIncludeMerge

	if pc.file.iamRole != nil {
		c.IAMRoleOptions = *pc.file.iamRole
	}

	return c
}

// parserOptions returns the options for the parsers that read pc's file, which log through l and write through v.
func (pc *ParseContext) parserOptions(l log.Logger, v *venv.Venv) []pkghclparse.Option {
	return pkgconfig.ParserOptions(l, v, pc.parserSettings())
}

// parserSettings returns the file's parser settings with pc's diagnostics sink applied.
func (pc *ParseContext) parserSettings() pkgconfig.ParserSettings {
	parser := pc.file.parser
	parser.DiagnosticsHandler = pc.sinks.diagnostics

	return parser
}
