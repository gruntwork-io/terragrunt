package discovery

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

type stackGeneratorKey struct{}
type stackGenerator func(context.Context, log.Logger, *venv.Venv, string) error

// WithStackGenerator supplies generation without making discovery depend on the generator.
func WithStackGenerator(ctx context.Context, generate func(context.Context, log.Logger, *venv.Venv, string) error) context.Context {
	return context.WithValue(ctx, stackGeneratorKey{}, stackGenerator(generate))
}

func generateStackDependency(ctx context.Context, l log.Logger, v *venv.Venv, path string) error {
	generate, ok := ctx.Value(stackGeneratorKey{}).(stackGenerator)
	if !ok {
		return nil
	}
	dir, err := util.CanonicalResolvedPath(v.FS, path, "")
	if err != nil {
		return err
	}
	if _, err := v.FS.Stat(filepath.Join(dir, config.DefaultStackFile)); err != nil {
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

// GenerateUnitDependencies reads dependency paths without resolving outputs.
func GenerateUnitDependencies(ctx context.Context, l log.Logger, v *venv.Venv, opts *options.TerragruntOptions, configPath string) error {
	parseOpts := opts.Clone()
	parseOpts.WorkingDir = filepath.Dir(configPath)
	parseOpts.TerragruntConfigPath = configPath
	parseOpts.OriginalTerragruntConfigPath = configPath
	parseOpts.SkipOutput = true
	pctx := configbridge.NewParsingContext(parseOpts).
		WithDecodeList(config.DependenciesBlock, config.DependencyBlock, config.FeatureFlagsBlock).
		WithSkipOutputsResolution()
	cfg, err := config.PartialParseConfigFile(ctx, l, v.WithEnvCloned(), pctx, configPath, nil)
	if err != nil {
		return err
	}
	paths, err := extractDependencyPaths(vfs.NewPathResolver(v.FS), cfg, component.NewUnit(filepath.Dir(configPath)))
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := generateStackDependency(ctx, l, v, path); err != nil {
			return err
		}
	}
	return nil
}
