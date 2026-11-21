package config

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// dependencyResolution is the value of the `dependency` variable and the blocks it was resolved from.
type dependencyResolution struct {
	value  *cty.Value
	blocks Dependencies
}

// runDependencies returns the `dependency` value run supplied with its blocks, and nil when run supplied none.
func runDependencies(run *ParsingContext) *dependencyResolution {
	if run.DecodedDependencies == nil {
		return nil
	}

	return &dependencyResolution{value: run.DecodedDependencies, blocks: run.DependencyBlocks}
}

// dependencySet is the dependency blocks of a file that resolves its own dependencies.
type dependencySet struct {
	// included maps the name of each include block of the file to the dependency blocks of the file it names.
	included map[string]Dependencies
	// configPath is the path of the owning file.
	configPath string
	// own is the blocks the file declares, with those of its autoinclude folded in.
	own Dependencies
	// blocks is own merged with the blocks of the file's includes.
	blocks Dependencies
}

// newDependencySet decodes the dependency blocks of file, which holds c's syntax tree, with pc and the variables
// c's blocks decoded with. It merges in the blocks included gives for each file c includes, reads no dependency's
// config, and fetches no outputs.
//
// Returns the error of building the eval context when the parse failed to build one.
func (c *UnitConfig) newDependencySet(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParseContext,
	file *hclparse.File,
	included IncludedDependencies,
	fns *parseFunctions,
) (*dependencySet, error) {
	if c.evalVars == nil {
		return nil, c.evalErr
	}

	pctx := pc.parsingContext()

	evalCtx := &hcl.EvalContext{Variables: c.evalVars}
	if declaresDependencies(file.Body) {
		evalCtx = dependencyBlockContext(pctx, c.evalContext(fns, pctx))
	}

	own, err := DecodeOwnDependencies(ctx, l, v, s, pctx, file, evalCtx)
	if err != nil {
		return nil, err
	}

	set := &dependencySet{
		configPath: file.ConfigPath,
		own:        own,
		blocks:     own,
		included:   map[string]Dependencies{},
	}

	if pctx.TrackInclude == nil {
		return set, nil
	}

	recording := func(
		ctx context.Context,
		l log.Logger,
		include *IncludeConfig,
	) (Dependencies, error) {
		blocks, err := included(ctx, l, include)
		if err != nil {
			return nil, err
		}

		set.included[include.Name] = blocks

		return blocks, nil
	}

	set.blocks, err = MergeIncludedDependencies(ctx, l, pctx, own, recording)
	if err != nil {
		return nil, err
	}

	return set, nil
}

// resolve loads the config of every dependency of the file, fetches the outputs of every block with pctx, and
// returns the value of the `dependency` variable with the blocks it was resolved from.
//
// Returns [cty.DynamicVal] with the error when resolution fails without a value.
func (set *dependencySet) resolve(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
) (*dependencyResolution, error) {
	blocks, err := set.loaded(ctx, l, v, s, pctx)
	if err != nil {
		return &dependencyResolution{value: failedDependencyValue(nil)}, err
	}

	names := make([]string, 0, len(blocks))
	for _, dep := range blocks {
		names = append(names, dep.Name)
	}

	var result *cty.Value

	err = TraceParseDependencies(
		ctx,
		l,
		set.configPath,
		pctx.SkipOutputsResolution,
		len(blocks),
		names,
		func(ctx context.Context, l log.Logger) error {
			var encodeErr error

			result, encodeErr = DependencyBlocksToCtyValue(ctx, l, v, s, pctx, blocks)

			return encodeErr
		},
	)
	if err != nil {
		return &dependencyResolution{value: failedDependencyValue(result), blocks: blocks}, err
	}

	return &dependencyResolution{value: result, blocks: blocks}, nil
}

// loaded returns the blocks of set with the config of each of the file's own dependencies loaded, merged with the
// blocks of the file's includes.
func (set *dependencySet) loaded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
) (Dependencies, error) {
	own, err := LoadDependencyConfigs(ctx, l, v, s, pctx, set.own)
	if err != nil {
		return nil, err
	}

	if pctx.TrackInclude == nil {
		return own, nil
	}

	kept := func(_ context.Context, _ log.Logger, include *IncludeConfig) (Dependencies, error) {
		return set.included[include.Name], nil
	}

	return MergeIncludedDependencies(ctx, l, pctx, own, kept)
}

// dependencyValue returns the `dependency` value ToV1 decodes c with, and the blocks it was resolved from: the
// run's when it supplied one, and otherwise the outputs of c's dependency blocks, fetched now. An included file
// whose including file resolved none decodes its own blocks first, as pkg/config does. It reports the files the
// parse read decoding the blocks to pc's sinks.
//
// Returns the value pkg/config falls back to with the error when the fetch fails. When the parse already reported
// that the blocks failed to decode, it returns that value and no error.
func (c *UnitConfig) dependencyValue(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParseContext,
) (*dependencyResolution, error) {
	if pc.file.decodedDeps != nil {
		return pc.file.decodedDeps, nil
	}

	for _, path := range c.dependencyReads {
		pc.sinks.filesRead.Add(path)
	}

	pctx := pc.parsingContext()
	set := c.deps

	if pc.file.inheritsDependencies {
		var err error

		set, err = c.newDependencySet(
			ctx,
			l,
			v,
			s,
			pc,
			c.boundFile(l, v, pc),
			ParsedIncludeDependencies(v, s, pctx),
			c.functions(ctx, l, v, s),
		)
		if err != nil {
			return &dependencyResolution{value: failedDependencyValue(nil)}, err
		}
	}

	if set == nil {
		return &dependencyResolution{value: failedDependencyValue(nil)}, nil
	}

	return set.resolve(ctx, l, v, s, pctx)
}

// failedDependencyValue returns the value a failed resolution leaves in the `dependency` variable.
func failedDependencyValue(value *cty.Value) *cty.Value {
	if value != nil {
		return value
	}

	dynamicVal := cty.DynamicVal

	return &dynamicVal
}
