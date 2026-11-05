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
	// configPath is the path of the owning file.
	configPath string
	// blocks is the blocks of file, merged with those of its includes and autoinclude.
	blocks pkgconfig.Dependencies
}

// newDependencySet decodes the dependency blocks of file with pctx. It fetches no outputs.
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
		configPath: file.ConfigPath,
		blocks:     decoded.Dependencies,
	}, nil
}

// resolve fetches the outputs of every block with pctx and returns the value of the `dependency` variable.
//
// Returns [cty.DynamicVal] with the error when resolution fails without a value.
func (s *dependencySet) resolve(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *pkgconfig.ParsingContext,
) (*cty.Value, error) {
	names := make([]string, 0, len(s.blocks))
	for _, dep := range s.blocks {
		names = append(names, dep.Name)
	}

	var result *cty.Value

	err := pkgconfig.TraceParseDependencies(
		ctx,
		l,
		s.configPath,
		pctx.SkipOutputsResolution,
		len(s.blocks),
		names,
		func(ctx context.Context, l log.Logger) error {
			var encodeErr error

			result, encodeErr = pkgconfig.DependencyBlocksToCtyValue(ctx, l, v, pctx, s.blocks)

			return encodeErr
		},
	)
	if err != nil {
		return failedDependencyValue(result), err
	}

	return result, nil
}

// dependencyValue returns the `dependency` value ToV1 decodes c with: the run's when it supplied
// one, and otherwise the outputs of c's dependency blocks, fetched now. An included file whose including file
// resolved none decodes its own blocks first, as pkg/config does.
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

	pctx := pc.parsingContext()
	set := c.deps

	if pc.file.inheritsDependencies {
		var err error

		set, err = newDependencySet(ctx, l, v, pctx, c.boundFile(l, v, pc))
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
