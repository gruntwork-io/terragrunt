package run

import (
	"context"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/runner/graph"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Run runs the run command.
func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if opts.TerraformCommand == tf.CommandNameDestroy {
		opts.CheckDependentUnits = opts.DestroyDependenciesCheck
	}

	r := report.NewReport().WithWorkingDir(opts.WorkingDir)

	// Configure report colors.
	//
	// This doesn't actually do anything for single-unit runs, but it's
	// helpful to leave it in here for consistency, if we ever add
	// support for run summaries in single-unit runs.
	if l.Formatter().DisabledColors() || stdout.IsRedirected() {
		r.WithDisableColor()
	}

	if opts.ReportFormat != "" {
		r.WithFormat(opts.ReportFormat)
	}

	tgOpts := opts.OptionsFromContext(ctx)

	if tgOpts.RunAll {
		return runall.Run(ctx, l, tgOpts)
	}

	if tgOpts.Graph {
		return graph.Run(ctx, l, tgOpts)
	}

	if opts.ReportSchemaFile != "" {
		defer r.WriteSchemaToFile(opts.ReportSchemaFile) //nolint:errcheck
	}

	if opts.ReportFile != "" {
		defer r.WriteToFile(opts.ReportFile) //nolint:errcheck
	}

	if opts.TerraformCommand == "" {
		return errors.New(run.MissingCommand{})
	}

	// Early exit for version command to avoid expensive setup
	if opts.TerraformCommand == tf.CommandNameVersion {
		return runVersionCommand(ctx, l, opts)
	}

	// We need to get the credentials from auth-provider-cmd at the very beginning,
	// since the locals block may contain `get_aws_account_id()` func.
	credsGetter := creds.NewGetter()
	if err := credsGetter.ObtainAndUpdateEnvIfNecessary(
		ctx,
		l,
		opts,
		externalcmd.NewProvider(l, opts),
	); err != nil {
		return err
	}

	l, err := checkVersionConstraints(ctx, l, opts)
	if err != nil {
		return err
	}

	cfg, err := config.ReadTerragruntConfig(ctx, l, opts, config.DefaultParserOptions(l, opts))
	if err != nil {
		return err
	}

	if opts.CheckDependentUnits {
		allowDestroy := confirmActionWithDependentUnits(ctx, l, opts, cfg)
		if !allowDestroy {
			return nil
		}
	}

	runCfg := cfg.ToRunConfig(l)

	return run.Run(ctx, l, tgOpts, r, runCfg, credsGetter)
}

// isTerraformPath returns true if the TFPath ends with the default Terraform path.
// This is used by help.go to determine whether to show "Terraform" or "OpenTofu" in help text.
func isTerraformPath(opts *options.TerragruntOptions) bool {
	return strings.HasSuffix(opts.TFPath, options.TerraformDefaultPath)
}

// runVersionCommand runs the version command. We do this instead of going through the normal run flow because
// we can resolve `version` a lot more cheaply.
func runVersionCommand(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if !opts.TFPathExplicitlySet {
		if tfPath, err := getTFPathFromConfig(ctx, l, opts); err != nil {
			return err
		} else if tfPath != "" {
			opts.TFPath = tfPath
		}
	}

	return tf.RunCommand(ctx, l, opts, opts.TerraformCliArgs.Slice()...)
}

func getTFPathFromConfig(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (string, error) {
	if !util.FileExists(opts.TerragruntConfigPath) {
		l.Debugf("Did not find the config file %s", opts.TerragruntConfigPath)

		return "", nil
	}

	cfg, err := getTerragruntConfig(ctx, l, opts)
	if err != nil {
		return "", err
	}

	return cfg.TerraformBinary, nil
}

// CheckVersionConstraints checks the version constraints of both terragrunt and terraform.
// Note that as a side effect this will set the following settings on terragruntOptions:
// - TerraformPath
// - TerraformVersion
// - FeatureFlags
// TODO: Look into a way to refactor this function to avoid the side effect.
func checkVersionConstraints(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (log.Logger, error) {
	partialTerragruntConfig, err := getTerragruntConfig(ctx, l, opts)
	if err != nil {
		return l, err
	}

	// If the TFPath is not explicitly set, use the TFPath from the config if it is set.
	if !opts.TFPathExplicitlySet && partialTerragruntConfig.TerraformBinary != "" {
		opts.TFPath = partialTerragruntConfig.TerraformBinary
	}

	l, err = run.PopulateTFVersion(ctx, l, opts)
	if err != nil {
		return l, err
	}

	terraformVersionConstraint := run.DefaultTerraformVersionConstraint
	if partialTerragruntConfig.TerraformVersionConstraint != "" {
		terraformVersionConstraint = partialTerragruntConfig.TerraformVersionConstraint
	}

	if err := run.CheckTerraformVersion(terraformVersionConstraint, opts); err != nil {
		return l, err
	}

	if partialTerragruntConfig.TerragruntVersionConstraint != "" {
		if err := run.CheckTerragruntVersion(partialTerragruntConfig.TerragruntVersionConstraint, opts); err != nil {
			return l, err
		}
	}

	if partialTerragruntConfig.FeatureFlags != nil {
		// update feature flags for evaluation
		for _, flag := range partialTerragruntConfig.FeatureFlags {
			flagName := flag.Name

			defaultValue, err := flag.DefaultAsString()
			if err != nil {
				return l, err
			}

			if _, exists := opts.FeatureFlags.Load(flagName); !exists {
				opts.FeatureFlags.Store(flagName, defaultValue)
			}
		}
	}

	return l, nil
}

func getTerragruntConfig(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (*config.TerragruntConfig, error) {
	ctx, configCtx := config.NewParsingContext(ctx, l, opts)
	configCtx = configCtx.WithDecodeList(
		config.TerragruntVersionConstraints,
		config.FeatureFlagsBlock,
	)

	return config.PartialParseConfigFile(
		ctx,
		configCtx,
		l,
		opts.TerragruntConfigPath,
		nil,
	)
}

// confirmActionWithDependentUnits - Show warning with list of dependent modules from current module before destroy
func confirmActionWithDependentUnits(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) bool {
	units := findDependentUnits(ctx, l, opts, cfg)
	if len(units) != 0 {
		if _, err := opts.ErrWriter.Write([]byte("Detected dependent units:\n")); err != nil {
			l.Error(err)
			return false
		}

		for _, unit := range units {
			if _, err := opts.ErrWriter.Write([]byte(unit + "\n")); err != nil {
				l.Error(err)
				return false
			}
		}

		prompt := "WARNING: Are you sure you want to continue?"

		shouldRun, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
		if err != nil {
			l.Error(err)
			return false
		}

		return shouldRun
	}

	return true
}

// findDependentUnits finds dependent units for the given unit, and returns their paths.
func findDependentUnits(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) []string {
	units := runner.FindDependentUnits(ctx, l, opts, cfg)

	paths := make([]string, len(units))
	for i, unit := range units {
		paths[i] = unit.Path()
	}

	return paths
}
