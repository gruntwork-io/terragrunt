// This code embeds tflint, which is under an MPL license, and you can
// find its source code at https://github.com/terraform-linters/tflint

package tflint

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terratest/modules/collections"

	"github.com/gruntwork-io/terragrunt/shell"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/errors"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/terraform-linters/tflint/cmd"
)

// TFVarPrefix Prefix to use for terraform variables set with environment variables.
const TFVarPrefix = "TF_VAR_"
const ArgVarPrefix = "-var="
const ArgVarFilePrefix = "-var-file="
const TFExternalTFLint = "--terragrunt-external-tflint"

// RunTflintWithOpts runs tflint with the given options and returns an error if there are any issues.
func RunTflintWithOpts(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, hook config.Hook) error {
	configFile, err := findTflintConfigInProject(terragruntOptions)
	if err != nil {
		return err
	}
	terragruntOptions.Logger.Debugf("Found .tflint.hcl file in %s", configFile)

	variables, err := inputsToTflintVar(terragruntConfig.Inputs)
	if err != nil {
		return err
	}

	tfVariables, err := tfArgumentsToTflintVar(terragruntOptions, hook, terragruntConfig.Terraform)
	if err != nil {
		return err
	}
	variables = append(variables, tfVariables...)

	terragruntOptions.Logger.Debugf("Initializing tflint in directory %s", terragruntOptions.WorkingDir)
	cli, err := cmd.NewCLI(terragruntOptions.Writer, terragruntOptions.ErrWriter)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	tflintArgs, externalTfLint := tflintArguments(hook.Execute[1:])

	// tflint init
	initArgs := []string{"tflint", "--init", "--config", configFile, "--chdir", terragruntOptions.WorkingDir}
	if externalTfLint {
		terragruntOptions.Logger.Debugf("Running external tflint init with args %v", initArgs)
		_, err := shell.RunShellCommandWithOutput(terragruntOptions, terragruntOptions.WorkingDir, false, false, initArgs[0], initArgs[1:]...)
		if err != nil {
			return errors.WithStackTrace(ErrorRunningTflint{args: initArgs})
		}
	} else {
		terragruntOptions.Logger.Debugf("Running internal tflint init with args %v", initArgs)
		statusCode := cli.Run(initArgs)
		if statusCode != 0 {
			return errors.WithStackTrace(ErrorRunningTflint{args: initArgs})
		}
	}

	// tflint execution
	args := []string{"tflint"}
	args = append(args, "--config", configFile)
	args = append(args, "--chdir", terragruntOptions.WorkingDir)
	args = append(args, variables...)
	args = append(args, tflintArgs...)

	if externalTfLint {
		terragruntOptions.Logger.Debugf("Running external tflint with args %v", args)
		_, err := shell.RunShellCommandWithOutput(terragruntOptions, terragruntOptions.WorkingDir, false, false, args[0], args[1:]...)
		if err != nil {
			return errors.WithStackTrace(ErrorRunningTflint{args: args})
		}
		terragruntOptions.Logger.Info("Tflint has run successfully. No issues found.")
	} else {
		terragruntOptions.Logger.Debugf("Running internal tflint with args %v", args)
		statusCode := cli.Run(args)

		if statusCode == cmd.ExitCodeError {
			return errors.WithStackTrace(ErrorRunningTflint{args: initArgs})
		} else if statusCode == cmd.ExitCodeIssuesFound {
			return errors.WithStackTrace(IssuesFound{})
		} else if statusCode == cmd.ExitCodeOK {
			terragruntOptions.Logger.Info("Tflint has run successfully. No issues found.")
		} else {
			return errors.WithStackTrace(UnknownError{statusCode: statusCode})
		}
	}
	return nil
}

// tflintArguments check arguments for --terragrunt-external-tflint and returns filtered arguments and flag if should use external tflint
func tflintArguments(arguments []string) ([]string, bool) {
	externalTfLint := false
	var filteredArguments []string

	for _, arg := range arguments {
		if arg == TFExternalTFLint {
			externalTfLint = true
			continue
		}
		filteredArguments = append(filteredArguments, arg)
	}
	return filteredArguments, externalTfLint
}

// inputsToTflintVar converts the inputs map to a list of tflint variables.
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

// tfArgumentsToTflintVar converts variables from the terraform config to a list of tflint variables.
func tfArgumentsToTflintVar(terragruntOptions *options.TerragruntOptions, hook config.Hook, config *config.TerraformConfig) ([]string, error) {
	var variables []string

	for _, arg := range config.ExtraArgs {
		// use extra args which will be used on same command as hook
		if len(collections.ListIntersection(arg.Commands, hook.Commands)) == 0 {
			continue
		}
		if arg.EnvVars != nil {
			// extract env_vars
			for name, value := range *arg.EnvVars {
				if strings.HasPrefix(name, TFVarPrefix) {
					varName := strings.TrimPrefix(name, TFVarPrefix)
					varValue, err := util.AsTerraformEnvVarJsonValue(value)
					if err != nil {
						return nil, err
					}
					newVar := fmt.Sprintf("--var='%s=%s'", varName, varValue)
					variables = append(variables, newVar)
				}
			}
		}
		if arg.Arguments != nil {
			// extract variables and var files from arguments
			for _, value := range *arg.Arguments {
				if strings.HasPrefix(value, ArgVarPrefix) {
					varName := strings.TrimPrefix(value, ArgVarPrefix)
					newVar := fmt.Sprintf("--var='%s'", varName)
					variables = append(variables, newVar)
				}
				if strings.HasPrefix(value, ArgVarFilePrefix) {
					varName := strings.TrimPrefix(value, ArgVarFilePrefix)
					newVar := fmt.Sprintf("--var-file=%s", varName)
					variables = append(variables, newVar)
				}
			}
		}
		if arg.RequiredVarFiles != nil {
			// extract required variables
			for _, file := range *arg.RequiredVarFiles {
				newVar := fmt.Sprintf("--var-file=%s", file)
				variables = append(variables, newVar)
			}
		}

		if arg.OptionalVarFiles != nil {
			// extract optional variables
			for _, file := range util.RemoveDuplicatesFromListKeepLast(*arg.OptionalVarFiles) {
				if util.FileExists(file) {
					newVar := fmt.Sprintf("--var-file=%s", file)
					variables = append(variables, newVar)
				} else {
					terragruntOptions.Logger.Debugf("Skipping tflint var-file %s as it does not exist", file)
				}
			}
		}

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
