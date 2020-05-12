package cli

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/hashicorp/terraform/command"
	"os"
)

type CustomProvider struct {
	Source string
	Name   string
}

// 1. Download the given source URL, which should use Terraform's module source syntax, into a temporary folder
// 2. Copy the contents of terragruntOptions.WorkingDir into the temporary folder.
// 3. Set terragruntOptions.WorkingDir to the temporary folder.
//
// See the processTerraformSource method for how we determine the temporary folder so we can reuse it across multiple
// runs of Terragrunt to avoid downloading everything from scratch every time.
func downloadTerraformProvider(provider *CustomProvider, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	terraformSource, err := processTerraformSource(provider.Source, terragruntOptions)
	if err != nil {
		return err
	}

	if err := downloadTerraformSourceIfNecessary(terraformSource, terragruntOptions, terragruntConfig); err != nil {
		return err
	}

	binaryLocation := terragruntOptions.WorkingDir + command.DefaultPluginVendorDir + "/terraform-provider" + provider.Name

	if err := compileGoProject(terraformSource.WorkingDir, binaryLocation, terragruntOptions); err != nil {
		return err
	}

	return nil
}

func compileGoProject(location, output string, terragruntOptions *options.TerragruntOptions) error {

	tmpDir := os.TempDir()

	//TODO: Clone options
	clonedTerragruntOptions := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	clonedTerragruntOptions.Env["GOCACHE"] = tmpDir + "/terragrunt-cache/gocache"
	clonedTerragruntOptions.Env["GOPATH"] = tmpDir + "/terragrunt-cache/gopath"

	_, err := shell.RunShellCommandWithOutput(clonedTerragruntOptions, location, false, false,
		"go", "build", "-o", output)

	if err != nil {
		return err
	}

	return nil
}
