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
func WithStackGenerator(ctx context.Context, generate func(context.Context, log.Logger, *venv.Venv, string) error) context.Context {
	return context.WithValue(ctx, stackGeneratorKey{}, stackGenerator(generate))
}

func GenerateStackDependency(ctx context.Context, l log.Logger, v *venv.Venv, path string) error {
	generate, ok := ctx.Value(stackGeneratorKey{}).(stackGenerator)
	if !ok {
		return nil
	}
	dir, err := util.CanonicalResolvedPath(v.FS, path, "")
	if err != nil {
		return err
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
