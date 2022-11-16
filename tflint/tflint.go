package tflint

import (
	"errors"
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/terraform-linters/tflint/cmd"
)

func RunTflintWithOpts(terragruntOptions *options.TerragruntOptions, args []string) error {
	cli := cmd.NewCLI(terragruntOptions.Writer, terragruntOptions.ErrWriter)

	workingDir := terragruntOptions.WorkingDir
	statusCode := cli.Run([]string{"tflint", "--init", workingDir})
	// how to parse tf vars?
	//      --var-file=FILE                                           Terraform variable file name
	//      --var='foo=bar'                                           Set a Terraform variable

	args = append(args, workingDir)
	fmt.Printf("Running tflint with args %v", args)
	statusCode = cli.Run(args)

	if statusCode != 0 {
		errorMsg := fmt.Sprintf("Error while running 'tflint'! Status code: %d", statusCode)
		return errors.New(errorMsg)
	}

	return nil
}
