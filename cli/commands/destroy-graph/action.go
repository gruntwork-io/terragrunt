package destroy_graph

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

func Run(opts *options.TerragruntOptions) error {

	rootDir, err := shell.GitTopLevelDir(opts, opts.WorkingDir)
	if err != nil {
		return err
	}

	rootOptions, err := options.NewTerragruntOptionsForTest(rootDir)
	if err != nil {
		return err
	}
	stack, err := configstack.FindStackInSubfolders(rootOptions, nil)
	if err != nil {
		return err
	}

	fmt.Printf("%v\n", stack)

	fmt.Printf("modules: \n%v\n", stack.Modules)

	return nil

}
