// Package run contains the CLI command definition for interacting with OpenTofu/Terraform.
package run

import (
	"context"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/runner/graph"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	CommandName = "run"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	cmdFlags := NewFlags(l, opts, nil)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, nil), shared.NewGraphFlag(opts, nil))

	cmd := &clihelper.Command{
		Name:        CommandName,
		Usage:       "Run an OpenTofu/Terraform command.",
		UsageText:   "terragrunt run [options] -- <tofu/terraform command>",
		Description: "Run a command, passing arguments to an orchestrated tofu/terraform binary.\n\nThis is the explicit, and most flexible form of running an IaC command with Terragrunt. Shortcuts can be found in \"terragrunt --help\" for common use-cases.",
		Examples: []string{
			"# Run a plan\nterragrunt run -- plan\n# Shortcut:\n# terragrunt plan",
			"# Run output with -json flag\nterragrunt run -- output -json\n# Shortcut:\n# terragrunt output -json",
		},
		Flags:       cmdFlags,
		Subcommands: NewSubcommands(l, opts),
		Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
			tgOpts := opts.OptionsFromContext(ctx)

			if tgOpts.RunAll {
				return runall.Run(ctx, l, tgOpts)
			}

			if tgOpts.Graph {
				return graph.Run(ctx, l, tgOpts)
			}

			if len(cliCtx.Args()) == 0 {
				return clihelper.ShowCommandHelp(ctx, cliCtx)
			}

			return Action(l, opts)(ctx, cliCtx)
		},
	}

	return cmd
}

func NewSubcommands(l log.Logger, opts *options.TerragruntOptions) clihelper.Commands {
	var subcommands = make(clihelper.Commands, len(tf.CommandNames))

	for i, name := range tf.CommandNames {
		usage, visible := tf.CommandUsages[name]

		subcommand := &clihelper.Command{
			Name:       name,
			Usage:      usage,
			Hidden:     !visible,
			CustomHelp: ShowTFHelp(l, opts),
			Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
				return Action(l, opts)(ctx, cliCtx)
			},
		}
		subcommands[i] = subcommand
	}

	return subcommands
}

func Action(l log.Logger, opts *options.TerragruntOptions) clihelper.ActionFunc {
	return func(ctx context.Context, _ *clihelper.Context) error {
		if opts.TerraformCommand == tf.CommandNameDestroy {
			opts.CheckDependentUnits = opts.DestroyDependenciesCheck
		}

		r := report.NewReport().WithWorkingDir(opts.WorkingDir)

		tgOpts := opts.OptionsFromContext(ctx)

		if tgOpts.RunAll {
			return runall.Run(ctx, l, tgOpts)
		}

		if tgOpts.Graph {
			return graph.Run(ctx, l, tgOpts)
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

		runCfg := cfg.ToRunConfig()

		return run.Run(ctx, l, tgOpts, r, runCfg, credsGetter)
	}
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

	return tf.RunCommand(ctx, l, opts, opts.TerraformCliArgs...)
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
			if _, err := opts.ErrWriter.Write([]byte(unit.Path() + "\n")); err != nil {
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

// findDependentUnits finds dependent units for the given unit.
func findDependentUnits(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) []runcfg.DependentUnit {
	units := runner.FindDependentUnits(ctx, l, opts, cfg)

	modules := make([]runcfg.DependentUnit, len(units))
	for i, unit := range units {
		modules[i] = unit
	}

	return modules
}
