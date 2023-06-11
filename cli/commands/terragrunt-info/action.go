package terragruntinfo

import (
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
	// group := TerragruntInfoGroup{
	// 	ConfigPath:       updatedTerragruntOptions.TerragruntConfigPath,
	// 	DownloadDir:      updatedTerragruntOptions.DownloadDir,
	// 	IamRole:          updatedTerragruntOptions.IAMRoleOptions.RoleARN,
	// 	TerraformBinary:  updatedTerragruntOptions.TerraformPath,
	// 	TerraformCommand: updatedTerragruntOptions.TerraformCommand,
	// 	WorkingDir:       updatedTerragruntOptions.WorkingDir,
	// }
	// b, err := json.MarshalIndent(group, "", "  ")
	// if err != nil {
	// 	updatedTerragruntOptions.Logger.Errorf("JSON error marshalling terragrunt-info")
	// 	return err
	// }
	// fmt.Fprintf(updatedTerragruntOptions.Writer, "%s\n", b)
	return nil

}
