package terragruntinfo

import (
	"encoding/json"
	"fmt"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

// Struct is output as JSON by 'terragrunt-info':
type TerragruntInfoGroup struct {
	ConfigPath       string
	DownloadDir      string
	IamRole          string
	TerraformBinary  string
	TerraformCommand string
	WorkingDir       string
}

func Run(opts *options.TerragruntOptions) error {
	task := terraform.NewTask(terraform.TaskTargetDownloadSource, runTerragruntInfo)

	return terraform.RunWithTask(opts, task)
}

func runTerragruntInfo(opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
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
	fmt.Fprintf(opts.Writer, "%s\n", b)

	return nil

}
