package config

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ParseContext is the state a parse reads: the run's settings plus the state of the file being parsed. The parse
// never mutates the run's [pkgconfig.ParsingContext], whose FilesRead collects every file the run reads.
type ParseContext struct {
	run  *pkgconfig.ParsingContext
	file fileScope
}

// fileScope is the per-file state pkg/config keeps on ParsingContext clones.
type fileScope struct {
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
	// decodedDeps is the resolved dependencies, nil until resolved.
	decodedDeps *cty.Value
	// iamRole overrides the run's IAM role options when set.
	iamRole *iam.RoleOptions
	// parser configures the parsers that read the file.
	parser pkgconfig.ParserSettings
	// depth is the number of file parses above this one.
	depth int
	// skipAutoIncludeMerge skips merging the file's autoinclude.
	skipAutoIncludeMerge bool
}

// NewParseContext returns a ParseContext for run, with the file state starting from run's values.
func NewParseContext(run *pkgconfig.ParsingContext) *ParseContext {
	return &ParseContext{
		run: run,
		file: fileScope{
			includes:             newIncludes(run.TrackInclude),
			values:               run.Values,
			features:             run.Features,
			locals:               run.Locals,
			decodedDeps:          run.DecodedDependencies,
			parser:               run.Parser,
			depth:                run.ParseDepth,
			skipAutoIncludeMerge: run.SkipAutoIncludeMerge,
		},
	}
}

// withInclude returns a copy of pc for the file include pulls in, which has no include blocks until its base
// blocks decode.
func (pc *ParseContext) withInclude(include *pkgconfig.IncludeConfig) *ParseContext {
	c := *pc
	c.file.include = include
	c.file.includes = nil

	return &c
}

// forAutoInclude returns a copy of pc for the file's autoinclude, which no include block pulls in, resolves its
// own dependencies, and merges no autoinclude of its own.
func (pc *ParseContext) forAutoInclude() *ParseContext {
	c := *pc
	c.file.include = nil
	c.file.decodedDeps = nil
	c.file.skipAutoIncludeMerge = true

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

// withDecodedDependencies returns a copy of pc with the file's resolved dependencies.
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

	return &c, nil
}

// parsingContext returns a clone of the run's [pkgconfig.ParsingContext] with the file state applied.
func (pc *ParseContext) parsingContext() *pkgconfig.ParsingContext {
	c := pc.run.Clone()
	c.TrackInclude = pc.file.includes.v1()
	c.Values = pc.file.values
	c.Features = pc.file.features
	c.Locals = pc.file.locals
	c.DecodedDependencies = pc.file.decodedDeps
	c.Parser = pc.file.parser
	c.ParseDepth = pc.file.depth
	c.SkipAutoIncludeMerge = pc.file.skipAutoIncludeMerge

	if pc.file.iamRole != nil {
		c.IAMRoleOptions = *pc.file.iamRole
	}

	return c
}

// parserOptions returns the options for the parsers that read pc's file, which log through l and write through v.
func (pc *ParseContext) parserOptions(l log.Logger, v *venv.Venv) []pkghclparse.Option {
	return pkgconfig.ParserOptions(l, v, pc.file.parser)
}
