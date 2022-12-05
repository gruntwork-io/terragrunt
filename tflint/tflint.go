package tflint

import (
	"errors"
	"fmt"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/terraform-linters/tflint/cmd"
)

func RunTflintWithOpts(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, args []string) error {
	cli := cmd.NewCLI(terragruntOptions.Writer, terragruntOptions.ErrWriter)

	variables, err := inputsToTflintVar(terragruntConfig.Inputs)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Debugf("Initializing tflint in directory %s", terragruntOptions.WorkingDir)
	statusCode := cli.Run([]string{"tflint", "--init", terragruntOptions.WorkingDir})
	if statusCode != 0 {
		errorMsg := fmt.Sprintf("Error while running 'tflint'! Status code: %d", statusCode)
		return errors.New(errorMsg)
	}

	args = append(args, variables...)
	args = append(args, terragruntOptions.WorkingDir)

	terragruntOptions.Logger.Debugf("Running tflint with args %v", args)
	statusCode = cli.Run(args)
	terragruntOptions.Logger.Debugf("Status code %d", statusCode)

	if statusCode != 0 {
		errorMsg := fmt.Sprintf("Error while running 'tflint'! Status code: %d", statusCode)
		return errors.New(errorMsg)
	}
	return nil
}

func inputsToTflintVar(inputs map[string]interface{}) ([]string, error) {
	var variables []string
	for key, value := range inputs {
		varValue, err := util.AsTerraformEnvVarJsonValue(value)
		if err != nil {
			return nil, err
		}

		newVar := fmt.Sprintf("--var=%s=%s", key, varValue)
		variables = append(variables, newVar)
	}
	return variables, nil
}
