package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	telemetryDiscovery = "runner_pool_discovery"
	telemetryCreation  = "runner_pool_creation"
)

func doWithTelemetry(
	ctx context.Context,
	l log.Logger,
	name string,
	fields map[string]any,
	fn func(context.Context, log.Logger) error,
) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, l, name, fields, fn)
}

// resolveWorkingDir determines the canonical working directory for discovery.
func resolveWorkingDir(opts *options.TerragruntOptions) string {
	if opts.RootWorkingDir != "" {
		return opts.RootWorkingDir
	}

	return opts.WorkingDir
}

// buildConfigFilenames returns the list of config filenames to consider, including custom if provided.
func buildConfigFilenames(opts *options.TerragruntOptions) []string {
	configFilenames := append([]string{}, discovery.DefaultConfigFilenames...)
	customConfigName := filepath.Base(opts.TerragruntConfigPath)
	isCustom := !slices.Contains(discovery.DefaultConfigFilenames, customConfigName)

	if isCustom && customConfigName != "" && customConfigName != "." {
		configFilenames = append(configFilenames, customConfigName)
	}

	return configFilenames
}

func extractWorktrees(opts []Option) *worktrees.Worktrees {
	for _, opt := range opts {
		if wo, ok := opt.(WorktreeOption); ok {
			return wo.Worktrees
		}
	}

	return nil
}

func newBaseDiscovery(
	opts *options.TerragruntOptions,
	workingDir string,
	configFilenames []string,
	runnerOpts ...Option,
) *discovery.Discovery {
	anyOpts := make([]any, len(runnerOpts))
	for i, v := range runnerOpts {
		anyOpts[i] = v
	}

	d := discovery.
		NewDiscovery(workingDir).
		WithOptions(anyOpts...).
		WithConfigFilenames(configFilenames).
		WithRelationships().
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: workingDir,
			Cmd:        opts.TerraformCliArgs.First(),
			Args:       opts.TerraformCliArgs.Tail(),
		})

	return d
}

func prepareDiscovery(
	opts *options.TerragruntOptions,
	runnerOpts ...Option,
) *discovery.Discovery {
	workingDir := resolveWorkingDir(opts)
	configFilenames := buildConfigFilenames(opts)

	d := newBaseDiscovery(opts, workingDir, configFilenames, runnerOpts...)

	if len(opts.Filters) > 0 {
		d = d.WithFilters(opts.Filters)
	}

	if opts.DiscoveryBoundary != "" {
		d = d.WithDiscoveryBoundary(opts.DiscoveryBoundary)
	}

	// Apply worktrees for git filter expressions
	if w := extractWorktrees(runnerOpts); w != nil {
		d = d.WithWorktrees(w)
	}

	return d
}

// discoverWithRetry runs discovery once, under a telemetry span.
func discoverWithRetry(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	runnerOpts ...Option,
) (component.Components, error) {
	d := prepareDiscovery(opts, runnerOpts...)

	var discovered component.Components

	err := doWithTelemetry(ctx, l, telemetryDiscovery, map[string]any{
		"working_dir":       opts.WorkingDir,
		"terraform_command": opts.TerraformCommand,
	}, func(childCtx context.Context, l log.Logger) error {
		var discoveryErr error

		discovered, discoveryErr = d.Discover(childCtx, l, v, opts)
		if discoveryErr == nil {
			l.Debugf("Runner pool discovery found %d configs", len(discovered))
		}

		return discoveryErr
	})
	if err != nil {
		return nil, err
	}

	return discovered, nil
}

func createRunner(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	comps component.Components,
) (*Runner, error) {
	var rnr *Runner

	err := doWithTelemetry(ctx, l, telemetryCreation, map[string]any{
		"discovered_configs": len(comps),
		"terraform_command":  opts.TerraformCommand,
	}, func(childCtx context.Context, l log.Logger) error {
		var err2 error

		rnr, err2 = NewFromComponents(childCtx, l, opts, comps)

		return err2
	})
	if err != nil {
		return nil, err
	}

	return rnr, nil
}

// checkVersionConstraints checks every discovered unit concurrently and returns the
// first error encountered.
func checkVersionConstraints(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	units []*component.Unit,
) error {
	g, checkCtx := errgroup.WithContext(ctx)

	maxWorkers := min(runtime.GOMAXPROCS(0), opts.Parallelism)
	g.SetLimit(maxWorkers)

	for _, unit := range units {
		g.Go(func() error {
			unitOpts, unitLogger, err := BuildUnitOpts(l, opts, unit)
			if err != nil {
				return err
			}

			return CheckUnitVersionConstraints(
				checkCtx,
				l,
				v,
				unitOpts,
				unitLogger,
				unit,
			)
		})
	}

	return g.Wait()
}

// CheckUnitVersionConstraints checks a single unit against the tofu and Terragrunt version
// constraints its config declares. When discovery left the unit unparsed, it parses the
// unit's config file first.
func CheckUnitVersionConstraints(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	unitOpts *options.TerragruntOptions,
	unitLogger log.Logger,
	unit *component.Unit,
) error {
	unitConfig := unit.Config()

	if unitConfig == nil {
		configCtx, pctx := configbridge.NewParsingContext(ctx, l, v, unitOpts)
		pctx = pctx.WithDecodeList(
			config.TerragruntVersionConstraints,
			config.FeatureFlagsBlock,
		)

		var err error

		unitConfig, err = config.PartialParseConfigFile(
			configCtx,
			pctx,
			l,
			unitOpts.TerragruntConfigPath,
			nil,
		)
		if err != nil {
			return fmt.Errorf("failed to parse config for unit %s: %w", unit.DisplayPath(), err)
		}
	}

	if !unitOpts.TFPathExplicitlySet && unitConfig.TerraformBinary != "" {
		unitOpts.TFPath = unitConfig.TerraformBinary
	}

	if unitLogger != nil {
		l = unitLogger
	}

	_, ver, impl, err := run.PopulateTFVersion(ctx, l, v, run.PopulateTFVersionInput{
		TFOpts:       configbridge.TFRunOptsFromOpts(v.Env, unitOpts),
		WorkingDir:   unitOpts.WorkingDir,
		VersionFiles: unitOpts.VersionManagerFileName,
	})
	if err != nil {
		return fmt.Errorf(
			"failed to populate Terraform version for unit %s: %w",
			unit.DisplayPath(),
			err,
		)
	}

	unitOpts.TerraformVersion = ver
	unitOpts.TofuImplementation = impl

	terraformVersionConstraint := run.DefaultTerraformVersionConstraint
	if unitConfig.TerraformVersionConstraint != "" {
		terraformVersionConstraint = unitConfig.TerraformVersionConstraint
	}

	if err := run.CheckTerraformVersionMeetsConstraint(
		unitOpts.TerraformVersion,
		terraformVersionConstraint,
	); err != nil {
		return fmt.Errorf("terraform version check failed for unit %s: %w", unit.DisplayPath(), err)
	}

	if unitConfig.TerragruntVersionConstraint != "" {
		if err := run.CheckTerragruntVersionMeetsConstraint(
			unitOpts.TerragruntVersion,
			unitConfig.TerragruntVersionConstraint,
		); err != nil {
			return fmt.Errorf(
				"terragrunt version check failed for unit %s: %w",
				unit.DisplayPath(),
				err,
			)
		}
	}

	return nil
}
