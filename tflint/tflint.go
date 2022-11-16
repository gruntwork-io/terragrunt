package tflint

import (
	"errors"
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/terraform-linters/tflint/cmd"
)

func RunTflintWithOpts(terragruntOptions *options.TerragruntOptions, args []string) error {
	// MARINA who is current dir?
	fmt.Println("RUNNING TFLINT")
	cli := cmd.NewCLI(terragruntOptions.Writer, terragruntOptions.ErrWriter)
	// sets dir as parameters.

	workingDir := terragruntOptions.WorkingDir
	statusCode := cli.Run([]string{"tflint", "--init", workingDir})
	// how to parse tf vars?
	//      --var-file=FILE                                           Terraform variable file name
	//      --var='foo=bar'                                           Set a Terraform variable

	// how to parse tflint config?
	// should the file live in the tg file, and parsed it in the config flag?
	// if it lives in the root, how to pass this? there are some functions with get_dir()

	args = append(args, workingDir)
	fmt.Printf("Running %v", args)
	statusCode = cli.Run(args)

	// lookup for config file
	// plugin "aws" {
	//    enabled = true
	//    version = "0.19.0"
	//    source  = "github.com/terraform-linters/tflint-ruleset-aws"
	//}

	if statusCode != 0 {
		errorMsg := fmt.Sprintf("Error while running 'tflint'! Status code: %d", statusCode)
		return errors.New(errorMsg)
	}

	return nil
}
