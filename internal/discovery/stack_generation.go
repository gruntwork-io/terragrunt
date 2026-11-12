package discovery

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
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
	paths, err := UnitDependencyPaths(ctx, l, v, opts, configPath, config.DiagnosticsLogged)
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

// UnitDependencyPaths reads current dependency references without retaining parsed configuration.
func UnitDependencyPaths(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	configPath string,
	diagnostics config.DiagnosticsOutput,
) ([]string, error) {
	parseOpts := opts.Clone()
	parseOpts.WorkingDir = filepath.Dir(configPath)
	parseOpts.TerragruntConfigPath = configPath
	parseOpts.OriginalTerragruntConfigPath = configPath
	parseOpts.SkipOutput = true
	parseOpts.UsePartialParseConfigCache = false
	parseV := v.WithEnvCloned()
	if parseOpts.DiscoveryAuthProviderCmd {
		shellOpts := configbridge.ShellRunOptsFromOpts(parseV.Env, parseOpts)
		if _, err := creds.ObtainCredsForParsing(ctx, l, parseV, parseOpts.AuthProviderCmd, shellOpts); err != nil {
			return nil, err
		}
	}

	pctx := configbridge.NewParsingContext(parseOpts).
		WithDecodeList(config.DependenciesBlock, config.DependencyBlock, config.FeatureFlagsBlock).
		WithSkipOutputsResolution()
	pctx.Parser.Diagnostics = diagnostics

	cfg, err := config.PartialParseConfigFile(ctx, l, parseV, pctx, configPath, nil)
	if err != nil {
		return nil, err
	}

	return extractDependencyPaths(vfs.NewPathResolver(v.FS), cfg, component.NewUnit(filepath.Dir(configPath)))
}

type stackDiscoveryKey struct{}
type stackDiscovery func(context.Context, log.Logger, *venv.Venv, []string) (bool, error)

// WithStackDiscovery supplies a generation pass that does not populate the execution graph.
func WithStackDiscovery(
	ctx context.Context,
	discover func(context.Context, log.Logger, *venv.Venv, []string) (bool, error),
) context.Context {
	return context.WithValue(ctx, stackDiscoveryKey{}, stackDiscovery(discover))
}

func (d *Discovery) generateDependencyStacks(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	results *PhaseResults,
) (bool, error) {
	discover, ok := ctx.Value(stackDiscoveryKey{}).(stackDiscovery)
	if !ok || !d.discoverRelationships {
		return false, nil
	}

	paths := make([]string, 0, len(results.Discovered)+len(results.Candidates))

	for _, result := range slices.Concat(results.Discovered, results.Candidates) {
		unit, ok := result.Component.(*component.Unit)
		if !ok {
			continue
		}

		filename := unit.ConfigFile()
		if filename == "" {
			filename = config.DefaultTerragruntConfigPath
		}

		paths = append(paths, filepath.Join(unit.Path(), filename))
	}

	return discover(ctx, l, v, paths)
}
