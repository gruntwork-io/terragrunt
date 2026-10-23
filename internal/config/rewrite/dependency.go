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
	// pctx is the owning file's context.
	pctx *pkgconfig.ParsingContext
	// file is the owning file.
	file *pkghclparse.File
	// blocks is the blocks of file, merged with those of its includes and autoinclude.
	blocks pkgconfig.Dependencies
}

// newDependencySet decodes the dependency blocks of file. It fetches no outputs.
func newDependencySet(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *pkgconfig.ParsingContext,
	file *pkghclparse.File,
) (*dependencySet, error) {
	decoded, err := pkgconfig.DecodeTerragruntDependencies(ctx, l, v, pctx, file)
	if err != nil {
		return nil, err
	}

	return &dependencySet{
		pctx:   pctx,
		file:   file,
		blocks: decoded.Dependencies,
	}, nil
}

// resolve fetches the outputs of every block and returns the value of the `dependency` variable.
//
// Returns [cty.DynamicVal] with the error when SkipOutput is set and resolution fails without a value.
func (s *dependencySet) resolve(ctx context.Context, l log.Logger, v *venv.Venv) (*cty.Value, error) {
	names := make([]string, 0, len(s.blocks))
	for _, dep := range s.blocks {
		names = append(names, dep.Name)
	}

	var result *cty.Value

	err := pkgconfig.TraceParseDependencies(
		ctx,
		l,
		s.file.ConfigPath,
		s.pctx.SkipOutputsResolution,
		len(s.blocks),
		names,
		func(ctx context.Context, l log.Logger) error {
			var encodeErr error

			result, encodeErr = pkgconfig.DependencyBlocksToCtyValue(ctx, l, v, s.pctx, s.blocks)

			return encodeErr
		},
	)
	if err != nil {
		return skipOutputFallback(s.pctx, result), err
	}

	return result, nil
}

// resolveDependencies decodes file's dependency blocks, fetches their outputs, and returns the `dependency` value
// with the blocks.
func resolveDependencies(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *pkgconfig.ParsingContext,
	file *pkghclparse.File,
) (*cty.Value, pkgconfig.Dependencies, error) {
	set, err := newDependencySet(ctx, l, v, pctx, file)
	if err != nil {
		return skipOutputFallback(pctx, nil), nil, err
	}

	value, err := set.resolve(ctx, l, v)

	return value, set.blocks, err
}

// skipOutputFallback returns the value a failed resolution leaves in the `dependency` variable.
func skipOutputFallback(pctx *pkgconfig.ParsingContext, value *cty.Value) *cty.Value {
	if !pctx.SkipOutput || value != nil {
		return value
	}

	dynamicVal := cty.DynamicVal

	return &dynamicVal
}
