// This code embeds tflint, which is under an MPL license, and you can
// find its source code at https://github.com/terraform-linters/tflint

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
	cli, err := cmd.NewCLI(terragruntOptions.Writer, terragruntOptions.ErrWriter)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// tflint init
	initArgs := []string{"tflint", "--init", "--config", configFile, terragruntOptions.WorkingDir}
	statusCode := cli.Run(initArgs)
	if statusCode != 0 {
		return errors.WithStackTrace(ErrorRunningTflint{args: initArgs})
	}

	// tflint execution
	args := []string{"tflint"}
	args = append(args, variables...)
	args = append(args, "--config", configFile)
	args = append(args, "--chdir", terragruntOptions.WorkingDir)

	terragruntOptions.Logger.Debugf("Running tflint with args %v", args)
	statusCode = cli.Run(args)

	if statusCode == cmd.ExitCodeError {
		return errors.WithStackTrace(ErrorRunningTflint{args: initArgs})
	} else if statusCode == cmd.ExitCodeIssuesFound {
		return errors.WithStackTrace(IssuesFound{})
	} else if statusCode == cmd.ExitCodeOK {
		terragruntOptions.Logger.Info("Tflint has run successfully. No issues found.")
	} else {
		return errors.WithStackTrace(UnknownError{statusCode: statusCode})
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
			return "", errors.WithStackTrace(ConfigNotFound{cause: "Traversed all the day to the root"})
		}

		fileToFind := util.JoinPath(previousDir, ".tflint.hcl")
		if util.FileExists(fileToFind) {
			terragruntOptions.Logger.Debugf("Found .tflint.hcl in %s", fileToFind)
			return fileToFind, nil
		}

		previousDir = currentDir
	}

	return "", errors.WithStackTrace(ConfigNotFound{cause: fmt.Sprintf("Exceeded maximum folders to check (%d)", terragruntOptions.MaxFoldersToCheck)})
}

// Custom error types

type ErrorRunningTflint struct {
	args []string
}

func (err ErrorRunningTflint) Error() string {
	return fmt.Sprintf("Error while running tflint with args: %v", err.args)
}

type IssuesFound struct{}

func (err IssuesFound) Error() string {
	return "Tflint found issues in the project. Check for the tflint logs."
}

type UnknownError struct {
	statusCode int
}

func (err UnknownError) Error() string {
	return fmt.Sprintf("Unknown status code from tflint: %d", err.statusCode)
}

type ConfigNotFound struct {
	cause string
}

func (err ConfigNotFound) Error() string {
	return fmt.Sprintf("Could not find .tflint.hcl config file in the parent folders: %s", err.cause)
}
