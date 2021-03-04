package cli

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// printTerragruntInputInfo will collect all the terraform variables defined in the target module, and the terragrunt
// inputs that are configured, and compare the two to determine if there are any unused inputs or undefined required
// inputs.
func printTerragruntInputInfo(terragruntOptions *options.TerragruntOptions, workingConfig *config.TerragruntConfig) error {
	required, optional, err := terraformModuleVariables(terragruntOptions)
	if err != nil {
		return err
	}
	allVars := append(required, optional...)

	allInputs, err := getDefinedTerragruntInputs(terragruntOptions, workingConfig)
	if err != nil {
		return err
	}

	// Unused variables are those that are passed in by terragrunt, but is not defined in terraform.
	unusedVars := []string{}
	for _, varName := range allInputs {
		if !util.ListContainsElement(allVars, varName) {
			unusedVars = append(unusedVars, varName)
		}
	}

	// Missing variables are those that are required by the terraform config, but is not defined in terragrunt.
	missingVars := []string{}
	for _, varName := range required {
		if !util.ListContainsElement(allInputs, varName) {
			missingVars = append(missingVars, varName)
		}
	}

	// Now print out all the information
	if len(unusedVars) > 0 {
		terragruntOptions.Logger.Warn("The following inputs passed in by terragrunt are unused:\n")
		for _, varName := range unusedVars {
			terragruntOptions.Logger.Warnf("\t- %s", varName)
		}
		terragruntOptions.Logger.Warn("")
	} else {
		terragruntOptions.Logger.Info("All variables passed in by terragrunt are in use.")
	}

	if len(missingVars) > 0 {
		terragruntOptions.Logger.Warn("The following required inputs are missing:\n")
		for _, varName := range missingVars {
			terragruntOptions.Logger.Warnf("\t- %s", varName)
		}
		terragruntOptions.Logger.Warn("")
	} else {
		terragruntOptions.Logger.Info("All required inputs are passed in by terragrunt.")
	}

	// Return an error when there are misaligned inputs.
	if len(unusedVars) > 0 || len(missingVars) > 0 {
		return fmt.Errorf("Terragrunt configuration has misaligned inputs")
	}
	return nil
}

// getDefinedTerragruntInputs will return a list of names of all variables that are configured by terragrunt to be
// passed into terraform. Terragrunt can pass in inputs from:
// - var files defined on terraform.extra_arguments blocks.
// - env vars defined on terraform.extra_arguments blocks.
// - env vars from the external runtime calling terragrunt.
// - inputs blocks.
func getDefinedTerragruntInputs(terragruntOptions *options.TerragruntOptions, workingConfig *config.TerragruntConfig) ([]string, error) {
	envVarTFVars := getTerraformInputNamesFromEnvVar(terragruntOptions, workingConfig)
	inputsTFVars := getTerraformInputNamesFromConfig(workingConfig)
	varFileTFVars, err := getTerraformInputNamesFromVarFiles(terragruntOptions, workingConfig)
	if err != nil {
		return nil, err
	}

	// Dedup the input vars. We use a map as a set to accomplish this.
	tmpOut := map[string]bool{}
	for _, varName := range envVarTFVars {
		tmpOut[varName] = true
	}
	for _, varName := range inputsTFVars {
		tmpOut[varName] = true
	}
	for _, varName := range varFileTFVars {
		tmpOut[varName] = true
	}

	out := []string{}
	for varName, _ := range tmpOut {
		out = append(out, varName)
	}
	return out, nil
}

// getTerraformInputNamesFromEnvVar will check the runtime environment variables and the configured environment
// variables from extra_arguments blocks to see if there are any TF_VAR environment variables that set terraform
// variables. This will return the list of names of variables that are set in this way by the given terragrunt
// configuration.
func getTerraformInputNamesFromEnvVar(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []string {
	envVars := terragruntOptions.Env

	// Make sure to check if there are configured env vars in the parsed terragrunt config.
	if terragruntConfig.Terraform != nil {
		for _, arg := range terragruntConfig.Terraform.ExtraArgs {
			// only focus on plan args
			if arg.EnvVars != nil && util.ListContainsElement(arg.Commands, "plan") {
				for key, val := range *arg.EnvVars {
					envVars[key] = val
				}
			}
		}
	}

	out := []string{}
	for envName, _ := range envVars {
		if strings.HasPrefix(envName, TFVarPrefix) {
			out = append(out, strings.TrimPrefix(envName, fmt.Sprintf("%s_", TFVarPrefix)))
		}
	}
	return out
}

// getTerraformInputNamesFromConfig will return the list of names of variables configured by the inputs block in the
// terragrunt config.
func getTerraformInputNamesFromConfig(terragruntConfig *config.TerragruntConfig) []string {
	out := []string{}
	for inputName, _ := range terragruntConfig.Inputs {
		out = append(out, inputName)
	}
	return out
}

// getTerraformInputNamesFromVarFiles will return the list of names of variables configured by var files set in the
// extra_arguments block of the given terragrunt config.
func getTerraformInputNamesFromVarFiles(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) ([]string, error) {
	if terragruntConfig.Terraform == nil {
		return nil, nil
	}

	varFiles := []string{}
	for _, arg := range terragruntConfig.Terraform.ExtraArgs {
		// only focus on plan args
		if util.ListContainsElement(arg.Commands, "plan") {
			varFiles = append(varFiles, arg.GetVarFiles(terragruntOptions.Logger)...)
		}
	}

	varNames := []string{}
	for _, varFile := range varFiles {
		fileVars, err := getVarNamesFromVarFile(varFile)
		if err != nil {
			return nil, err
		}
		varNames = append(varNames, fileVars...)
	}

	return varNames, nil
}

// getVarNamesFromVarFile will parse the given terraform var file and return a list of names of variables that are
// configured in that var file.
func getVarNamesFromVarFile(varFile string) ([]string, error) {
	fileContents, err := ioutil.ReadFile(varFile)
	if err != nil {
		return nil, err
	}

	var variables map[string]interface{}
	if err := config.ParseAndDecodeVarFile(string(fileContents), varFile, &variables); err != nil {
		return nil, err
	}

	out := []string{}
	for varName, _ := range variables {
		out = append(out, varName)
	}
	return out, nil
}
