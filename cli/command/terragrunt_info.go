package command

import (
	"encoding/json"
	"fmt"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	cmdTerragruntInfo = "terragrunt-info"
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

func NewTerragruntInfoCommand(opts *options.TerragruntOptions) *cli.Command {
	command := &cli.Command{
		Name:   cmdTerragruntInfo,
		Usage:  "Emits limited terragrunt state on stdout and exits.",
		Action: func(ctx *cli.Context) error { return runHCLFmt(opts) },
	}

	return command
}

func runTerragruntInfo(terragruntOptions *options.TerragruntOptions) error {
	group := TerragruntInfoGroup{
		ConfigPath:       updatedTerragruntOptions.TerragruntConfigPath,
		DownloadDir:      updatedTerragruntOptions.DownloadDir,
		IamRole:          updatedTerragruntOptions.IAMRoleOptions.RoleARN,
		TerraformBinary:  updatedTerragruntOptions.TerraformPath,
		TerraformCommand: updatedTerragruntOptions.TerraformCommand,
		WorkingDir:       updatedTerragruntOptions.WorkingDir,
	}
	b, err := json.MarshalIndent(group, "", "  ")
	if err != nil {
		updatedTerragruntOptions.Logger.Errorf("JSON error marshalling terragrunt-info")
		return err
	}
	fmt.Fprintf(updatedTerragruntOptions.Writer, "%s\n", b)
	return nil

}
