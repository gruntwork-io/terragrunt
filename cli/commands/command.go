package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/env"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-version"
)

func InitialSetup(ctx *cli.Context, opts *options.TerragruntOptions) error {
	// Log the terragrunt version in debug mode. This helps with debugging issues and ensuring a specific version of  terragrunt used.
	defer opts.Logger.Debugf("Terragrunt Version: %s", opts.TerragruntVersion)

	// convert the rest flags (intended for terraform) to one prefix, e.g. `--input=true` to `-input=true`
	args := ctx.Args().Normalize(cli.OnePrefixFlag)

	opts.TerraformCommand = args.First()
	opts.TerraformCliArgs = args.Slice()

	opts.LogLevel = util.ParseLogLevel(opts.LogLevelStr)
	opts.Logger = util.CreateLogEntry("", opts.LogLevel)
	opts.Logger.Logger.SetOutput(ctx.App.ErrWriter)

	opts.Writer = ctx.App.Writer
	opts.ErrWriter = ctx.App.ErrWriter
	opts.Env = env.ParseEnvs(os.Environ())

	if opts.WorkingDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return errors.WithStackTrace(err)
		}
		opts.WorkingDir = currentDir
	} else {
		path, err := filepath.Abs(opts.WorkingDir)
		if err != nil {
			return errors.WithStackTrace(err)
		}
		opts.WorkingDir = path
	}
	opts.WorkingDir = filepath.ToSlash(opts.WorkingDir)

	if opts.DownloadDir == "" {
		opts.DownloadDir = util.JoinPath(opts.WorkingDir, options.TerragruntCacheDir)
	} else {
		path, err := filepath.Abs(opts.DownloadDir)
		if err != nil {
			return errors.WithStackTrace(err)
		}
		opts.DownloadDir = path
	}
	opts.DownloadDir = filepath.ToSlash(opts.DownloadDir)

	if opts.TerragruntConfigPath == "" {
		opts.TerragruntConfigPath = config.GetDefaultConfigPath(opts.WorkingDir)
	}
	opts.TerraformPath = filepath.ToSlash(opts.TerraformPath)

	terragruntVersion, err := version.NewVersion(ctx.App.Version)
	if err != nil {
		// Malformed Terragrunt version; set the version to 0.0
		if terragruntVersion, err = version.NewVersion("0.0"); err != nil {
			return errors.WithStackTrace(err)
		}
	}
	opts.TerragruntVersion = terragruntVersion

	jsonOutput := false
	for _, arg := range opts.TerraformCliArgs {
		if strings.EqualFold(arg, "-json") {
			jsonOutput = true
			break
		}
	}
	if opts.IncludeModulePrefix && !jsonOutput {
		opts.OutputPrefix = fmt.Sprintf("[%s] ", opts.WorkingDir)
	} else {
		opts.IncludeModulePrefix = false
	}

	if !opts.RunAllAutoApprove {
		// When running in no-auto-approve mode, set parallelism to 1 so that interactive prompts work.
		opts.Parallelism = 1
	}

	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath
	opts.OriginalTerraformCommand = opts.TerraformCommand
	opts.OriginalIAMRoleOptions = opts.IAMRoleOptions

	return nil
}
