package config

import (
	"strconv"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// StoreContext returns [Key.Context] for a resolution of pctx through v, and false when the resolution is not
// memoized. The resolutions it reaches are shared only with callers whose environment and output discarding match
// those of v.
func StoreContext(v *venv.Venv, pctx *ParsingContext) (string, bool) {
	return runDigest(v, pctx)
}

// forStore returns ctx as a [Store] resolves it: ctx itself when the Store keyed it, and otherwise a clone of it keyed
// for a resolution through v, which no include block pulls in.
func (ctx *ParsingContext) forStore(v *venv.Venv) *ParsingContext {
	if ctx.keyed {
		return ctx
	}

	c := ctx.Clone()
	c.key, c.memoized = runDigest(v, ctx)
	c.keyed = true
	c.include = nil
	c.inheritsDependencies = false
	c.merging = false

	return c
}

// withInclude returns a copy of ctx for the file include pulls in, which has no include blocks until its base
// blocks decode. The file reads the `dependency` value of the including file unless the run supplied one. A nil
// include is a file no include block pulls in, which resolves its own dependencies. The copy drops any forMerge
// marking of ctx.
func (ctx *ParsingContext) withInclude(include *IncludeConfig) *ParsingContext {
	c := ctx.derive()
	c.include = include
	c.includes = nil
	c.inheritsDependencies = include != nil && c.DecodedDependencies == nil
	c.merging = false

	return c
}

// forMerge returns a copy of ctx for assembling a config that only merges into the config of the including file.
func (ctx *ParsingContext) forMerge() *ParsingContext {
	c := ctx.derive()
	c.merging = true

	return c
}

// forIncluded returns a copy of ctx for the file include pulls in, one step further along the chain of files.
func (ctx *ParsingContext) forIncluded(include *IncludeConfig) *ParsingContext {
	c := ctx.withInclude(include)
	c.key = extend(
		c.key,
		"include",
		include.Name,
		include.Path,
		formatBoolPtr(include.Expose),
		formatStringPtr(include.MergeStrategy),
	)

	return c
}

// forAutoInclude returns a copy of ctx for the file's autoinclude, which no include block pulls in, resolves its
// own dependencies, and merges no autoinclude of its own.
func (ctx *ParsingContext) forAutoInclude() *ParsingContext {
	c := ctx.derive()
	c.include = nil
	c.DecodedDependencies = nil
	c.DependencyBlocks = nil
	c.inheritsDependencies = false
	c.SkipAutoIncludeMerge = true
	c.key = extend(c.key, "autoinclude")

	return c
}

// scope returns the state of the file ctx parses, without the settings and sinks of ctx, for a result to keep.
func (ctx *ParsingContext) scope() ParsingContext {
	c := *ctx
	c.RunSettings = nil
	c.FilesRead = nil
	c.Parser.DiagnosticsHandler = nil

	return c
}

// withFile returns a copy of ctx in the state of the file of file, a [ParsingContext.scope] of an earlier context.
func (ctx *ParsingContext) withFile(file *ParsingContext) *ParsingContext {
	c := ctx.derive()
	c.key = file.key
	c.include = file.include
	c.includes = file.includes
	c.Values = file.Values
	c.PathVars = file.PathVars
	c.Features = file.Features
	c.Locals = file.Locals
	c.DecodedDependencies = file.DecodedDependencies
	c.DependencyBlocks = file.DependencyBlocks
	c.IAMRoleOptions = file.IAMRoleOptions
	c.Parser = file.Parser
	c.Parser.DiagnosticsHandler = ctx.Parser.DiagnosticsHandler
	c.ParseDepth = file.ParseDepth
	c.SkipAutoIncludeMerge = file.SkipAutoIncludeMerge
	c.inheritsDependencies = file.inheritsDependencies

	return c
}

// withOwnReads returns a copy of ctx that records its reads apart from the caller's. A caller that keeps no record
// still keeps none.
func (ctx *ParsingContext) withOwnReads() *ParsingContext {
	c := ctx.derive()

	if ctx.FilesRead.Tracking() {
		c.FilesRead = NewFilesRead()
	}

	return c
}

// withBaseBlocks returns a copy of ctx with the include blocks, feature flags, and locals baseBlocks decoded.
func (ctx *ParsingContext) withBaseBlocks(baseBlocks *DecodedBaseBlocks) *ParsingContext {
	c := ctx.derive()
	c.includes = newIncludes(baseBlocks.TrackInclude)
	c.Features = baseBlocks.FeatureFlags
	c.Locals = baseBlocks.Locals

	return c
}

// withUnitValues returns a copy of ctx with the file's unit values.
func (ctx *ParsingContext) withUnitValues(values *cty.Value) *ParsingContext {
	c := ctx.derive()
	c.Values = values

	return c
}

// withPathVars returns a copy of ctx with the path variables of the file.
func (ctx *ParsingContext) withPathVars(vars map[string]cty.Value) *ParsingContext {
	c := ctx.derive()
	c.PathVars = vars

	return c
}

// withDecodedDependencies returns a copy of ctx with the file's `dependency` value and the blocks it was resolved
// from, none when decodedDeps is nil.
func (ctx *ParsingContext) withDecodedDependencies(decodedDeps *dependencyResolution) *ParsingContext {
	c := ctx.derive()
	c.DecodedDependencies = nil
	c.DependencyBlocks = nil

	if decodedDeps != nil {
		c.DecodedDependencies = decodedDeps.value
		c.DependencyBlocks = decodedDeps.blocks
	}

	return c
}

// decodedDependencies returns the file's `dependency` value with the blocks it was resolved from, and nil when the
// parse resolves them.
func (ctx *ParsingContext) decodedDependencies() *dependencyResolution {
	if ctx.DecodedDependencies == nil {
		return nil
	}

	return &dependencyResolution{value: ctx.DecodedDependencies, blocks: ctx.DependencyBlocks}
}

// withIAMRole returns a copy of ctx that evaluates the file with iamRole.
func (ctx *ParsingContext) withIAMRole(iamRole iam.RoleOptions) *ParsingContext {
	c := ctx.derive()
	c.IAMRoleOptions = iamRole

	return c
}

// forIAMRole returns a copy of ctx whose file evaluates with iamRole and memoizes apart from its resolution with ctx.
// A partial parse of the file keeps iamRole and resolves no role of its own.
func (ctx *ParsingContext) forIAMRole(iamRole iam.RoleOptions) *ParsingContext {
	c := ctx.withIAMRole(iamRole)

	d := newDigest()
	d.str(c.key)
	d.str("iam-role")
	d.roleOptions(iamRole)
	c.key = d.sum()

	return c
}

// forTerragruntFlags returns a copy of ctx for a partial parse of the file that decodes its Terragrunt flags and
// reports no diagnostics. It memoizes apart from ctx.
func (ctx *ParsingContext) forTerragruntFlags() *ParsingContext {
	c := ctx.forDiagnosticsSuppressed()
	c.RunSettings = new(*ctx.RunSettings)
	c.PartialParseDecodeList = []PartialDecodeSectionType{TerragruntFlags}
	c.key = extend(c.key, "terragrunt-flags")

	return c
}

// forDiagnosticsSuppressed returns a copy of ctx whose parsers report no diagnostics. It memoizes apart from ctx.
func (ctx *ParsingContext) forDiagnosticsSuppressed() *ParsingContext {
	c := ctx.derive()
	c.Parser.Diagnostics = DiagnosticsSuppressed
	c.key = extend(c.key, "diagnostics-suppressed")

	return c
}

// incrementDepth returns a copy of ctx one parse deeper.
//
// Returns [MaxParseDepthError] when ctx is already past [MaxParseDepth].
func (ctx *ParsingContext) incrementDepth() (*ParsingContext, error) {
	if ctx.ParseDepth > MaxParseDepth {
		return nil, MaxParseDepthError{
			Depth: ctx.ParseDepth,
			Max:   MaxParseDepth,
		}
	}

	c := ctx.derive()
	c.ParseDepth++
	c.key = extend(c.key, "depth", strconv.Itoa(c.ParseDepth))

	return c, nil
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
