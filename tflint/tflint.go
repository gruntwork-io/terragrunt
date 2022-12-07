package tflint

import (
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/terraform-linters/tflint/cmd"
)

func RunTflintWithOpts(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
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
	cli := cmd.NewCLI(terragruntOptions.Writer, terragruntOptions.ErrWriter)

	statusCode := cli.Run([]string{"tflint", "--init",
		"--config", configFile,
		terragruntOptions.WorkingDir})
	if statusCode != 0 {
		return errors.WithStackTrace(ErrorRunningTflint("--init"))
	}

	args := []string{"tflint"}
	args = append(args, variables...)
	args = append(args, "--config", configFile)
	args = append(args, "--module")
	args = append(args, terragruntOptions.WorkingDir)

	terragruntOptions.Logger.Debugf("Running tflint with args %v", args)
	statusCode = cli.Run(args)

	if statusCode == cmd.ExitCodeError {
		return errors.WithStackTrace(ErrorRunningTflint(fmt.Sprintf("running with variables %v", variables)))
	}

	if statusCode == cmd.ExitCodeIssuesFound {
		return errors.WithStackTrace(TflintFoundIssues("invalid rules"))
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
func findTflintConfigInProject(terragruntOptions *options.TerragruntOptions) (string, error) {
	previousDir := terragruntOptions.WorkingDir

	// To avoid getting into an accidental infinite loop (e.g. do to cyclical symlinks), set a max on the number of
	// parent folders we'll check
	for i := 0; i < terragruntOptions.MaxFoldersToCheck; i++ {
		currentDir := filepath.ToSlash(filepath.Dir(previousDir))
		terragruntOptions.Logger.Debugf("Finding .tflint.hcl file from %s and going to %s", previousDir, currentDir)
		if currentDir == previousDir {
			return "", errors.WithStackTrace(TflintConfigNotFound("Traversed all the day to the root"))
		}

		fileToFind := util.JoinPath(currentDir, ".tflint.hcl")
		if util.FileExists(fileToFind) {
			terragruntOptions.Logger.Debugf("Found .tflint.hcl in %s", fileToFind)
			return fileToFind, nil
		}

		previousDir = currentDir
	}

	return "", errors.WithStackTrace(TflintConfigNotFound(fmt.Sprintf("Exceeded maximum folders to check (%d)", terragruntOptions.MaxFoldersToCheck)))
}

// Custom error types

type ErrorRunningTflint string

func (e ErrorRunningTflint) Error() string {
	return fmt.Sprintf("Error while running tflint: %s", e)
}

type TflintFoundIssues string

func (e TflintFoundIssues) Error() string {
	return fmt.Sprintf("Tflint found issues in the project: %s")
}

type TflintConfigNotFound string

func (e TflintConfigNotFound) Error() string {
	return fmt.Sprintf("Could not find .tflint.hcl config file in the parent folders: %s", e)
}
