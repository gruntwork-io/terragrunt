package discovery

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// GenerateUnitDependencies reads dependency paths without resolving outputs.
func GenerateUnitDependencies(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	configPath string,
) error {
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
		if err := config.GenerateStackDependency(ctx, l, v, path); err != nil {
			return err
		}
	}

	return nil
}
