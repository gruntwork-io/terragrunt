// Package prepare provides functionality to prepare downloaded OpenTofu/Terraform source code
// for use with Terragrunt. This includes reading and parsing Terragrunt configuration, fetching
// credentials, downloading source code, generating configuration files, and initializing the
// OpenTofu/Terraform working directory.
//
// The preparation process follows a sequence of stages:
//  1. PrepareConfig - Reads configuration and fetches credentials
//  2. PrepareSource - Downloads terraform source if specified
//  3. PrepareGenerate - Generates configuration files (generate blocks and remote_state)
//  4. PrepareInputsAsEnvVars - Sets inputs as environment variables
//  5. PrepareInit - Runs terraform init if needed
package prepare

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Config holds the result of preparing a terragrunt configuration.
type Config struct {
	Cfg  *config.TerragruntConfig
	Opts *options.TerragruntOptions
}

// PrepareConfig reads and parses the terragrunt configuration, fetches credentials,
// and performs version constraint checks. This is the first stage of preparation.
func PrepareConfig(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (*Config, error) {
	// We need to get the credentials from auth-provider-cmd at the very beginning,
	// since the locals block may contain `get_aws_account_id()` func.
	credsGetter := creds.NewGetter()
	if err := credsGetter.ObtainAndUpdateEnvIfNecessary(ctx, l, opts.Env, externalcmd.NewProvider(l, opts.AuthProviderCmd, shell.RunOptionsFromOpts(opts))); err != nil {
		return nil, err
	}

	ctx, pctx := configbridge.NewParsingContext(ctx, l, opts)

	terragruntConfig, err := config.ReadTerragruntConfig(ctx, l, pctx, pctx.ParserOptions)
	if err != nil {
		return nil, err
	}

	return &Config{
		Cfg:  terragruntConfig,
		Opts: opts,
	}, nil
}

// PrepareSource downloads terraform source if specified in the configuration.
// It requires PrepareConfig to have been called first.
func PrepareSource(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
	r *report.Report,
) (*options.TerragruntOptions, error) {
	engine, err := cfg.EngineOptions()
	if err != nil {
		return nil, err
	}

	opts.Engine = engine

	errConfig, err := cfg.ErrorsConfig()
	if err != nil {
		return nil, err
	}

	opts.Errors = errConfig

	runCfg := cfg.ToRunConfig(l)

	l, optsClone, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	optsClone.TerraformCommand = run.CommandNameTerragruntReadConfig

	if err = optsClone.RunWithErrorHandling(ctx, l, r, func() error {
		return run.ProcessHooks(ctx, l, runCfg.Terraform.AfterHooks, run.NewOptions(optsClone), runCfg, nil, r)
	}); err != nil {
		return nil, err
	}

	// We merge the OriginalIAMRoleOptions into the one from the config, because the CLI passed IAMRoleOptions has
	// precedence.
	opts.IAMRoleOptions = iam.MergeRoleOptions(
		cfg.GetIAMRoleOptions(),
		opts.OriginalIAMRoleOptions,
	)

	credsGetter := creds.NewGetter()

	if err = opts.RunWithErrorHandling(ctx, l, r, func() error {
		return credsGetter.ObtainAndUpdateEnvIfNecessary(ctx, l, opts.Env, amazonsts.NewProvider(l, opts.IAMRoleOptions, opts.Env))
	}); err != nil {
		return nil, err
	}

	_, defaultDownloadDir, err := util.DefaultWorkingAndDownloadDirs(opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	// if the download dir hasn't been changed from default, and is set in the config,
	// then use it
	if opts.DownloadDir == defaultDownloadDir && runCfg.DownloadDir != "" {
		opts.DownloadDir = runCfg.DownloadDir
	}

	sourceURL, err := runcfg.GetTerraformSourceURL(opts, runCfg)
	if err != nil {
		return nil, err
	}

	runOpts := run.NewOptions(opts)

	var updatedRunOpts *run.Options

	// Always download/copy source to cache directory for consistency.
	// When no source is specified, sourceURL will be "." (current directory).
	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "download_terraform_source", map[string]any{
		"sourceUrl": sourceURL,
	}, func(ctx context.Context) error {
		updatedRunOpts, err = run.DownloadTerraformSource(ctx, l, sourceURL, runOpts, runCfg, r)
		return err
	})
	if err != nil {
		return nil, err
	}

	// DownloadTerraformSource returns *run.Options; sync the updated WorkingDir
	// back to a *options.TerragruntOptions clone for callers that expect that type.
	_, updatedTerragruntOptions, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	updatedTerragruntOptions.WorkingDir = updatedRunOpts.WorkingDir

	return updatedTerragruntOptions, nil
}

// PrepareGenerate handles code generation configs, both generate blocks and generate attribute of remote_state.
// It requires PrepareSource to have been called first.
func PrepareGenerate(l log.Logger, opts *options.TerragruntOptions, cfg *runcfg.RunConfig) error {
	return run.GenerateConfig(l, run.NewOptions(opts), cfg)
}

// PrepareInputsAsEnvVars sets terragrunt inputs as environment variables.
// It requires PrepareGenerate to have been called first.
func PrepareInputsAsEnvVars(l log.Logger, opts *options.TerragruntOptions, cfg *runcfg.RunConfig) error {
	runOpts := run.NewOptions(opts)

	// Check for terraform code
	if err := run.CheckFolderContainsTerraformCode(runOpts); err != nil {
		return err
	}

	return run.SetTerragruntInputsAsEnvVars(l, runOpts, cfg)
}

// PrepareInit runs terraform init if needed. This is the final preparation stage.
// It requires PrepareInputsAsEnvVars to have been called first.
func PrepareInit(
	ctx context.Context,
	l log.Logger,
	originalOpts, opts *options.TerragruntOptions,
	cfg *runcfg.RunConfig,
	r *report.Report,
) error {
	runOpts := run.NewOptions(opts)

	// Check for terraform code
	if err := run.CheckFolderContainsTerraformCode(runOpts); err != nil {
		return err
	}

	if err := run.SetTerragruntInputsAsEnvVars(l, runOpts, cfg); err != nil {
		return err
	}

	// Run terraform init via the non-init command preparation path
	return run.PrepareNonInitCommand(ctx, l, run.NewOptions(originalOpts), runOpts, cfg, r)
}
