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

// tflint validates the binary's version based on the ruleset version.
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
		return errors.WithStackTrace(ErrorRunningTflint("Error while running tflint --init."))
	}

	args := []string{"tflint"}
	args = append(args, variables...)
	args = append(args, "--config", configFile)
	args = append(args, "--module")
	args = append(args, terragruntOptions.WorkingDir)

	terragruntOptions.Logger.Debugf("Running tflint with args %v", args)
	statusCode = cli.Run(args)
	terragruntOptions.Logger.Debugf("Status code %d", statusCode)

	if statusCode == cmd.ExitCodeError {
		return errors.WithStackTrace(ErrorRunningTflint("Error while running tflint."))
	}

	// export constant with the error message

	if statusCode == cmd.ExitCodeIssuesFound {
		terragruntOptions.Logger.Warnf("tflint found issues")
		return errors.WithStackTrace(TflintFoundIssues("tflint found issues"))
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
			return "", errors.WithStackTrace(TflintConfigNotFound("Could not find .tflint.hcl in the parent folders"))
		}

		fileToFind := util.JoinPath(currentDir, ".tflint.hcl")
		if util.FileExists(fileToFind) {
			return fileToFind, nil
		}

		previousDir = currentDir
	}

	return "", errors.WithStackTrace(TflintConfigNotFound("Could not find .tflint.hcl in the parent folders"))
}

// Custom error types

type ErrorRunningTflint string

func (err ErrorRunningTflint) Error() string {
	return "TODO"
}

type TflintFoundIssues string

func (err TflintFoundIssues) Error() string {
	return "TODO"
}

type TflintConfigNotFound string

func (err TflintConfigNotFound) Error() string {
	return "TODO"
}
