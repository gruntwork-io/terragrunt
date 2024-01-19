package terragruntinfo

import (
	"encoding/json"
	"fmt"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(opts *options.TerragruntOptions) error {
	target := terraform.NewTargetWithErrorHandler(terraform.TargetPointDownloadSource, runTerragruntInfo, runErrorTerragruntInfo)

	return terraform.RunWithTarget(opts, target)
}

// Struct is output as JSON by 'terragrunt-info':
type TerragruntInfoGroup struct {
	ConfigPath       string
	DownloadDir      string
	IamRole          string
	TerraformBinary  string
	TerraformCommand string
	WorkingDir       string
}

func printTerragruntInfo(opts *options.TerragruntOptions) error {
	group := TerragruntInfoGroup{
		ConfigPath:       opts.TerragruntConfigPath,
		DownloadDir:      opts.DownloadDir,
		IamRole:          opts.IAMRoleOptions.RoleARN,
		TerraformBinary:  opts.TerraformPath,
		TerraformCommand: opts.TerraformCommand,
		WorkingDir:       opts.WorkingDir,
	}

	b, err := json.MarshalIndent(group, "", "  ")
	if err != nil {
		opts.Logger.Errorf("JSON error marshalling terragrunt-info")
		return err
	}
	if _, err := fmt.Fprintf(opts.Writer, "%s\n", b); err != nil {
		return err
	}
	return nil
}

func runTerragruntInfo(opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
	return printTerragruntInfo(opts)
}

func runErrorTerragruntInfo(opts *options.TerragruntOptions, cfg *config.TerragruntConfig, err error) error {
	opts.Logger.Debugf("Fetching terragrunt-info: %v", err)
	if err := printTerragruntInfo(opts); err != nil {
		opts.Logger.Errorf("Error printing terragrunt-info: %v", err)
	}
	return err
}
