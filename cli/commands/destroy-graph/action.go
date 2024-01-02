package destroy_graph

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(opts *options.TerragruntOptions) error {

	terragruntConfig, err := config.ReadTerragruntConfig(opts)
	if err != nil {
		return err
	}
	modules := configstack.FindWhereWorkingDirIsIncluded(opts, terragruntConfig)

	if _, err := opts.ErrWriter.Write([]byte("Detected dependent modules:\n")); err != nil {
		opts.Logger.Error(err)
		return err
	}
	for _, module := range modules {
		if _, err := opts.ErrWriter.Write([]byte(fmt.Sprintf("%s\n", module.Path))); err != nil {
			opts.Logger.Error(err)
			return err
		}
	}
	return nil
}
