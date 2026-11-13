package config

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// dependencySet is the dependency blocks of a file that resolves its own dependencies.
type dependencySet struct {
	// included maps the name of each include block of the file to the dependency blocks of the file it names.
	included map[string]pkgconfig.Dependencies
	// configPath is the path of the owning file.
	configPath string
	// own is the blocks the file declares, with those of its autoinclude folded in.
	own pkgconfig.Dependencies
	// blocks is own merged with the blocks of the file's includes.
	blocks pkgconfig.Dependencies
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
	pc *ParseContext,
	file *pkghclparse.File,
	included pkgconfig.IncludedDependencies,
) (*dependencySet, error) {
	if c.evalVars == nil {
		return nil, c.evalErr
	}

	pctx := pc.parsingContext()

	evalCtx := dependencyBlockContext(pctx, c.evalContext(ctx, l, v, pctx))

	own, err := pkgconfig.DecodeOwnDependencies(ctx, l, v, pctx, file, evalCtx)
	if err != nil {
		return nil, err
	}

	set := &dependencySet{
		configPath: file.ConfigPath,
		own:        own,
		blocks:     own,
		included:   map[string]pkgconfig.Dependencies{},
	}

	if pctx.TrackInclude == nil {
		return set, nil
	}

	recording := func(
		ctx context.Context,
		l log.Logger,
		include *pkgconfig.IncludeConfig,
	) (pkgconfig.Dependencies, error) {
		blocks, err := included(ctx, l, include)
		if err != nil {
			return nil, err
		}

		set.included[include.Name] = blocks

		return blocks, nil
	}

	set.blocks, err = pkgconfig.MergeIncludedDependencies(ctx, l, pctx, own, recording)
	if err != nil {
		return nil, err
	}

	return set, nil
}

// resolve loads the config of every dependency of the file, fetches the outputs of every block with pctx, and
// returns the value of the `dependency` variable.
//
// Returns [cty.DynamicVal] with the error when resolution fails without a value.
func (s *dependencySet) resolve(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *pkgconfig.ParsingContext,
) (*cty.Value, error) {
	blocks, err := s.loaded(ctx, l, v, pctx)
	if err != nil {
		return failedDependencyValue(nil), err
	}

	names := make([]string, 0, len(blocks))
	for _, dep := range blocks {
		names = append(names, dep.Name)
	}

	var result *cty.Value

	err = pkgconfig.TraceParseDependencies(
		ctx,
		l,
		s.configPath,
		pctx.SkipOutputsResolution,
		len(blocks),
		names,
		func(ctx context.Context, l log.Logger) error {
			var encodeErr error

			result, encodeErr = pkgconfig.DependencyBlocksToCtyValue(ctx, l, v, pctx, blocks)

			return encodeErr
		},
	)
	if err != nil {
		return failedDependencyValue(result), err
	}

	return result, nil
}

// loaded returns the blocks of s with the config of each of the file's own dependencies loaded, merged with the
// blocks of the file's includes.
func (s *dependencySet) loaded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *pkgconfig.ParsingContext,
) (pkgconfig.Dependencies, error) {
	own, err := pkgconfig.LoadDependencyConfigs(ctx, l, v, pctx, s.own)
	if err != nil {
		return nil, err
	}

	if pctx.TrackInclude == nil {
		return own, nil
	}

	kept := func(_ context.Context, _ log.Logger, include *pkgconfig.IncludeConfig) (pkgconfig.Dependencies, error) {
		return s.included[include.Name], nil
	}

	return pkgconfig.MergeIncludedDependencies(ctx, l, pctx, own, kept)
}

// dependencyValue returns the `dependency` value ToV1 decodes c with: the run's when it supplied
// one, and otherwise the outputs of c's dependency blocks, fetched now. An included file whose including file
// resolved none decodes its own blocks first, as pkg/config does. It reports the files the parse read decoding the
// blocks to pc's sinks.
//
// Returns the value pkg/config falls back to with the error when the fetch fails. When the parse already reported
// that the blocks failed to decode, it returns that value and no error.
func (c *UnitConfig) dependencyValue(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pc *ParseContext,
) (*cty.Value, error) {
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
			pc,
			c.boundFile(l, v, pc),
			pkgconfig.ParsedIncludeDependencies(v, pctx),
		)
		if err != nil {
			return failedDependencyValue(nil), err
		}
	}

	if set == nil {
		return failedDependencyValue(nil), nil
	}

	return set.resolve(ctx, l, v, pctx)
}

// failedDependencyValue returns the value a failed resolution leaves in the `dependency` variable.
func failedDependencyValue(value *cty.Value) *cty.Value {
	if value != nil {
		return value
	}

	dynamicVal := cty.DynamicVal

	return &dynamicVal
}
