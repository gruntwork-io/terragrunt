package config

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type stackGeneratorKey struct{}
type stackGenerator func(context.Context, log.Logger, *venv.Venv, string) error

// WithStackGenerator supplies generation without making config readers depend on the generator.
func WithStackGenerator(
	ctx context.Context,
	generate func(context.Context, log.Logger, *venv.Venv, string) error,
) context.Context {
	return context.WithValue(ctx, stackGeneratorKey{}, stackGenerator(generate))
}

// GenerateStackDependency invokes the command generator for an explicit stack directory, if installed.
func GenerateStackDependency(ctx context.Context, l log.Logger, v *venv.Venv, path string) error {
	generate, ok := ctx.Value(stackGeneratorKey{}).(stackGenerator)
	if !ok {
		return nil
	}

	dir, err := util.CanonicalResolvedPath(v.FS, path, "")
	if err != nil {
		return err
	}

	if preparation, ok := ctx.Value(stackPreparationKey{}).(stackPreparation); ok {
		if err := preparation.wait(ctx, dir); err != nil {
			return err
		}
	}

	if _, err := v.FS.Stat(filepath.Join(dir, DefaultStackFile)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		// A dependency may name a unit's configuration file directly.
		if info, statErr := v.FS.Stat(dir); statErr == nil && !info.IsDir() {
			return nil
		}

		return err
	}

	return generate(ctx, l, v, dir)
}

type stackPreparationKey struct{}

type stackPreparation struct {
	register func(context.Context, []string) error
	wait     func(context.Context, string) error
}

// WithStackPreparation coordinates generated destinations before their files are read or written.
func WithStackPreparation(
	ctx context.Context,
	register func(context.Context, []string) error,
	wait func(context.Context, string) error,
) context.Context {
	return context.WithValue(ctx, stackPreparationKey{}, stackPreparation{register: register, wait: wait})
}

func registerStackOutputs(ctx context.Context, stack *StackConfig, opts *generateOpts) error {
	preparation, ok := ctx.Value(stackPreparationKey{}).(stackPreparation)
	if !ok {
		return nil
	}

	var destinations []string

	for _, unit := range stack.Units {
		if !unit.IsEnabled() {
			continue
		}

		dest, err := resolveDestPath(
			&componentToGenerate{path: unit.Path, noStack: unit.NoStack != nil && *unit.NoStack},
			opts,
		)
		if err != nil {
			return err
		}

		destinations = append(destinations, dest)
	}

	for _, nested := range stack.Stacks {
		if !nested.IsEnabled() {
			continue
		}

		dest, err := resolveDestPath(
			&componentToGenerate{path: nested.Path, noStack: nested.NoStack != nil && *nested.NoStack},
			opts,
		)
		if err != nil {
			return err
		}

		destinations = append(destinations, dest)
	}

	return preparation.register(ctx, destinations)
}
