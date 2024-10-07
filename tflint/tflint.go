// Package tflint embeds execution of tflint, which is under an MPL license, and you can
// find its source code at https://github.com/terraform-linters/tflint
package tflint

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terratest/modules/collections"

	"github.com/gruntwork-io/terragrunt/shell"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/terraform-linters/tflint/cmd"
)

const (
	// tfVarPrefix Prefix to use for terraform variables set with environment variables.
	tfVarPrefix      = "TF_VAR_"
	argVarPrefix     = "-var="
	argVarFilePrefix = "-var-file="
	tfExternalTFLint = "--terragrunt-external-tflint"
)

// RunTflintWithOpts runs tflint with the given options and returns an error if there are any issues.
func RunTflintWithOpts(ctx context.Context, opts *options.TerragruntOptions, config *config.TerragruntConfig, hook config.Hook) error {
	// try to fetch configuration file from hook parameters
	configFile := tflintConfigFilePath(hook.Execute)
	if configFile == "" {
		// find .tflint.hcl configuration in project files if it is not provided in arguments
		projectConfigFile, err := findTflintConfigInProject(opts)
		if err != nil {
			return err
		}

		configFile = projectConfigFile
	}

	opts.Logger.Debugf("Using .tflint.hcl file in %s", configFile)

	variables, err := InputsToTflintVar(config.Inputs)
	if err != nil {
		return err
	}

	tfVariables, err := tfArgumentsToTflintVar(opts, hook, config.Terraform)
	if err != nil {
		return err
	}

	variables = append(variables, tfVariables...)

	opts.Logger.Debugf("Initializing tflint in directory %s", opts.WorkingDir)

	cli, err := cmd.NewCLI(opts.Writer, opts.ErrWriter)
	if err != nil {
		return errors.New(err)
	}

	tflintArgs, externalTfLint := tflintArguments(hook.Execute[1:])

	// tflint init
	initArgs := []string{"tflint", "--init", "--config", configFile, "--chdir", opts.WorkingDir}
	if externalTfLint {
		opts.Logger.Debugf("Running external tflint init with args %v", initArgs)

		_, err := shell.RunShellCommandWithOutput(ctx, opts, opts.WorkingDir, false, false,
			initArgs[0], initArgs[1:]...)
		if err != nil {
			return errors.New(ErrorRunningTflint{args: initArgs})
		}
	} else {
		opts.Logger.Debugf("Running internal tflint init with args %v", initArgs)

		statusCode := cli.Run(initArgs)
		if statusCode != 0 {
			return errors.New(ErrorRunningTflint{args: initArgs})
		}
	}

	// tflint execution
	args := []string{
		"tflint",
		"--config", configFile,
		"--chdir", opts.WorkingDir,
	}
	args = append(args, variables...)
	args = append(args, tflintArgs...)

	if externalTfLint {
		opts.Logger.Debugf("Running external tflint with args %v", args)

		_, err := shell.RunShellCommandWithOutput(ctx, opts, opts.WorkingDir, false, false,
			args[0], args[1:]...)
		if err != nil {
			return errors.New(ErrorRunningTflint{args: args})
		}

		opts.Logger.Info("Tflint has run successfully. No issues found.")
	} else {
		opts.Logger.Debugf("Running internal tflint with args %v", args)
		statusCode := cli.Run(args)

		switch statusCode {
		case cmd.ExitCodeError:
			return errors.New(ErrorRunningTflint{args: initArgs})
		case cmd.ExitCodeIssuesFound:
			return errors.New(IssuesFound{})
		case cmd.ExitCodeOK:
			opts.Logger.Info("Tflint has run successfully. No issues found.")
		default:
			return errors.New(UnknownError{statusCode: statusCode})
		}
	}

	return nil
}

// tflintArguments filters args for --terragrunt-external-tflint, returning filtered args and a flag for using
// external tflint.
func tflintArguments(arguments []string) ([]string, bool) {
	externalTfLint := false
	filteredArguments := make([]string, 0, len(arguments))

	for _, arg := range arguments {
		if arg == tfExternalTFLint {
			externalTfLint = true
			continue
		}

		filteredArguments = append(filteredArguments, arg)
	}

	return filteredArguments, externalTfLint
}

// configFilePathArgument return configuration file specified in --config argument
func tflintConfigFilePath(arguments []string) string {
	for i, arg := range arguments {
		if arg == "--config" && len(arguments) > i+1 {
			return arguments[i+1]
		}
	}

	return ""
}

// InputsToTflintVar converts the inputs map to a list of tflint variables.
func InputsToTflintVar(inputs map[string]interface{}) ([]string, error) {
	variables := make([]string, 0, len(inputs))

	for key, value := range inputs {
		varValue, err := util.AsTerraformEnvVarJSONValue(value)
		if err != nil {
			return nil, err
		}

		newVar := fmt.Sprintf("--var=%s=%s", key, varValue)
		variables = append(variables, newVar)
	}

	return variables, nil
}

// tfArgumentsToTflintVar converts variables from the terraform config to a list of tflint variables.
func tfArgumentsToTflintVar(terragruntOptions *options.TerragruntOptions, hook config.Hook,
	config *config.TerraformConfig) ([]string, error) {
	var variables []string

	for _, arg := range config.ExtraArgs {
		// use extra args which will be used on same command as hook
		if len(collections.ListIntersection(arg.Commands, hook.Commands)) == 0 {
			continue
		}

		if arg.EnvVars != nil {
			// extract env_vars
			for name, value := range *arg.EnvVars {
				if strings.HasPrefix(name, tfVarPrefix) {
					varName := strings.TrimPrefix(name, tfVarPrefix)

					varValue, err := util.AsTerraformEnvVarJSONValue(value)
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
				if strings.HasPrefix(value, argVarPrefix) {
					varName := strings.TrimPrefix(value, argVarPrefix)
					newVar := fmt.Sprintf("--var='%s'", varName)
					variables = append(variables, newVar)
				}

				if strings.HasPrefix(value, argVarFilePrefix) {
					varName := strings.TrimPrefix(value, argVarFilePrefix)
					newVar := "--var-file=" + varName
					variables = append(variables, newVar)
				}
			}
		}

		if arg.RequiredVarFiles != nil {
			// extract required variables
			for _, file := range *arg.RequiredVarFiles {
				newVar := "--var-file=" + file
				variables = append(variables, newVar)
			}
		}

		if arg.OptionalVarFiles != nil {
			// extract optional variables
			for _, file := range util.RemoveDuplicatesFromListKeepLast(*arg.OptionalVarFiles) {
				if util.FileExists(file) {
					newVar := "--var-file=" + file
					variables = append(variables, newVar)
				} else {
					terragruntOptions.Logger.Debugf("Skipping tflint var-file %s as it does not exist", file)
				}
			}
		}
	}

	return variables, nil
}

// findTflintConfigInProject looks for a .tflint.hcl file in the current folder or it's parents.
func findTflintConfigInProject(terragruntOptions *options.TerragruntOptions) (string, error) {
	previousDir := terragruntOptions.WorkingDir

	// To avoid getting into an accidental infinite loop (e.g. do to cyclical symlinks), set a max on the number of
	// parent folders we'll check
	for i := 0; i < terragruntOptions.MaxFoldersToCheck; i++ {
		currentDir := filepath.ToSlash(filepath.Dir(previousDir))
		terragruntOptions.Logger.Debugf("Finding .tflint.hcl file from %s and going to %s", previousDir, currentDir)

		if currentDir == previousDir {
			return "", errors.New(ConfigNotFound{cause: "Traversed all the day to the root"})
		}

		fileToFind := util.JoinPath(previousDir, ".tflint.hcl")
		if util.FileExists(fileToFind) {
			terragruntOptions.Logger.Debugf("Found .tflint.hcl in %s", fileToFind)
			return fileToFind, nil
		}

		previousDir = currentDir
	}

	return "", errors.New(ConfigNotFound{
		cause: fmt.Sprintf("Exceeded maximum folders to check (%d)", terragruntOptions.MaxFoldersToCheck),
	})
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
	return "Could not find .tflint.hcl config file in the parent folders: " + err.cause
}
