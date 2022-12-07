package tflint

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/terraform-linters/tflint/cmd"
)

// tflint validates the binary's version based on the ruleset version.
func RunTflintWithOpts(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	cli := cmd.NewCLI(terragruntOptions.Writer, terragruntOptions.ErrWriter)

	configFile, err := findTflintConfigInProject(terragruntOptions)

	if err != nil {
		return err
	}

	terragruntOptions.Logger.Debugf("Found .tflint.hcl file in %s", configFile)

	variables, err := inputsToTflintVar(terragruntConfig.Inputs)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Debugf("Initializing tflint in directory %s", terragruntOptions.WorkingDir)
	statusCode := cli.Run([]string{"tflint", "--init",
		"--config", configFile,
		terragruntOptions.WorkingDir})

	if statusCode != 0 {
		errorMsg := fmt.Sprintf("Error while running 'tflint'! Status code: %d", statusCode)
		return errors.New(errorMsg)
	}

	args := []string{"tflint"}
	args = append(args, variables...)
	args = append(args, "--config", configFile)
	args = append(args, "--module")
	args = append(args, terragruntOptions.WorkingDir)

	terragruntOptions.Logger.Debugf("Running tflint with args %v", args)
	statusCode = cli.Run(args)
	terragruntOptions.Logger.Debugf("Status code %d", statusCode)

	// 1 - real error
	// 2 - issues found
	// TODO TEST CASE FOR REAL ERRORS, e.g. invalid argument
	if statusCode == cmd.ExitCodeError {
		return errors.New("error while running tflint")
	}

	// export constant with the error message

	if statusCode == cmd.ExitCodeIssuesFound {
		terragruntOptions.Logger.Warnf("tflint found issues")
		//return errors.New("issues found")
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

// findTflintConfigInProjects looks for a .tflint.hcl file in the current folder or it's parents.
// TODO Should it search for ~/.tflint?? this is tflint's existing behaviour
func findTflintConfigInProject(terragruntOptions *options.TerragruntOptions) (string, error) {
	previousDir := terragruntOptions.WorkingDir

	// To avoid getting into an accidental infinite loop (e.g. do to cyclical symlinks), set a max on the number of
	// parent folders we'll check
	for i := 0; i < terragruntOptions.MaxFoldersToCheck; i++ {
		currentDir := filepath.ToSlash(filepath.Dir(previousDir))
		terragruntOptions.Logger.Debugf("Finding .tflint.hcl file from %s and going to %s", previousDir, currentDir)
		if currentDir == previousDir {
			return "", errors.New("error")
		}

		fileToFind := util.JoinPath(currentDir, ".tflint.hcl")
		if util.FileExists(fileToFind) {
			return fileToFind, nil
		}

		previousDir = currentDir
	}

	return "", errors.New("error")
}
