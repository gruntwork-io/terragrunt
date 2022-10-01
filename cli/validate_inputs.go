package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/google/shlex"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const validateInputsHelp = `
   Usage: terragrunt validate-inputs [OPTIONS]

   Description:
   Checks if the terragrunt configured inputs align with the terraform defined variables.
   
   Options:
   --terragrunt-strict-validate		Enable strict mode for validation. When strict mode is turned on, an error will be returned if required inputs are missing OR if unused variables are passed to Terragrunt.
`

// validateTerragruntInputs will collect all the terraform variables defined in the target module, and the terragrunt
// inputs that are configured, and compare the two to determine if there are any unused inputs or undefined required
// inputs.
func validateTerragruntInputs(terragruntOptions *options.TerragruntOptions, workingConfig *config.TerragruntConfig) error {
	required, optional, err := terraformModuleVariables(terragruntOptions)
	if err != nil {
		return err
	}
	allVars := append(required, optional...)

	allInputs, err := getDefinedTerragruntInputs(terragruntOptions, workingConfig)
	if err != nil {
		return err
	}

	// Unused variables are those that are passed in by terragrunt, but are not defined in terraform.
	unusedVars := []string{}
	for _, varName := range allInputs {
		if !util.ListContainsElement(allVars, varName) {
			unusedVars = append(unusedVars, varName)
		}
	}

	// Missing variables are those that are required by the terraform config, but not defined in terragrunt.
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
		terragruntOptions.Logger.Debug(fmt.Sprintf("Strict mode enabled: %t", terragruntOptions.ValidateStrict))
	}

	if len(missingVars) > 0 {
		terragruntOptions.Logger.Error("The following required inputs are missing:\n")
		for _, varName := range missingVars {
			terragruntOptions.Logger.Errorf("\t- %s", varName)
		}
		terragruntOptions.Logger.Error("")
	} else {
		terragruntOptions.Logger.Info("All required inputs are passed in by terragrunt")
		terragruntOptions.Logger.Debug(fmt.Sprintf("Strict mode enabled: %t", terragruntOptions.ValidateStrict))
	}

	// Return an error when there are misaligned inputs. Terragrunt strict mode defaults to false. When it is false,
	// an error will only be returned if required inputs are missing. When strict mode is true, an error will be
	// returned if required inputs are missing OR if any unused variables are passed
	if len(missingVars) > 0 || len(unusedVars) > 0 && terragruntOptions.ValidateStrict {
		return fmt.Errorf(fmt.Sprintf("Terragrunt configuration has misaligned inputs. Strict mode enabled: %t.", terragruntOptions.ValidateStrict))
	} else if len(unusedVars) > 0 {
		terragruntOptions.Logger.Warn("Terragrunt configuration has misaligned inputs, but running in relaxed mode so ignoring.")
	}

	return nil
}

// getDefinedTerragruntInputs will return a list of names of all variables that are configured by terragrunt to be
// passed into terraform. Terragrunt can pass in inputs from:
// - var files defined on terraform.extra_arguments blocks.
// - -var and -var-file args passed in on extra_arguments CLI args.
// - env vars defined on terraform.extra_arguments blocks.
// - env vars from the external runtime calling terragrunt.
// - inputs blocks.
// - automatically injected terraform vars (terraform.tfvars, terraform.tfvars.json, *.auto.tfvars, *.auto.tfvars.json)
func getDefinedTerragruntInputs(terragruntOptions *options.TerragruntOptions, workingConfig *config.TerragruntConfig) ([]string, error) {
	envVarTFVars := getTerraformInputNamesFromEnvVar(terragruntOptions, workingConfig)
	inputsTFVars := getTerraformInputNamesFromConfig(workingConfig)
	varFileTFVars, err := getTerraformInputNamesFromVarFiles(terragruntOptions, workingConfig)
	if err != nil {
		return nil, err
	}
	cliArgsTFVars, err := getTerraformInputNamesFromCLIArgs(terragruntOptions, workingConfig)
	if err != nil {
		return nil, err
	}
	autoVarFileTFVars, err := getTerraformInputNamesFromAutomaticVarFiles(terragruntOptions)
	if err != nil {
		return nil, err
	}

	// Dedupe the input vars. We use a map as a set to accomplish this.
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
	for _, varName := range cliArgsTFVars {
		tmpOut[varName] = true
	}
	for _, varName := range autoVarFileTFVars {
		tmpOut[varName] = true
	}

	out := []string{}
	for varName := range tmpOut {
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
			if arg.EnvVars != nil {
				for key, val := range *arg.EnvVars {
					envVars[key] = val
				}
			}
		}
	}

	out := []string{}
	for envName := range envVars {
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
	for inputName := range terragruntConfig.Inputs {
		out = append(out, inputName)
	}
	return out
}

// getTerraformInputNamesFromVarFiles will return the list of names of variables configured by var files set in the
// extra_arguments block required_var_files and optional_var_files settings of the given terragrunt config.
func getTerraformInputNamesFromVarFiles(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) ([]string, error) {
	if terragruntConfig.Terraform == nil {
		return nil, nil
	}

	varFiles := []string{}
	for _, arg := range terragruntConfig.Terraform.ExtraArgs {
		varFiles = append(varFiles, arg.GetVarFiles(terragruntOptions.Logger)...)
	}

	return getVarNamesFromVarFiles(varFiles)
}

// getTerraformInputNamesFromCLIArgs will return the list of names of variables configured by -var and -var-file CLI
// args that are passed in via the configured arguments attribute in the extra_arguments block of the given terragrunt
// config and those that are directly passed in via the CLI.
func getTerraformInputNamesFromCLIArgs(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) ([]string, error) {
	inputNames, varFiles, err := getVarFlagsFromArgList(terragruntOptions.TerraformCliArgs)
	if err != nil {
		return inputNames, err
	}

	if terragruntConfig.Terraform != nil {
		for _, arg := range terragruntConfig.Terraform.ExtraArgs {
			if arg.Arguments != nil {
				vars, rawVarFiles, err := getVarFlagsFromArgList(*arg.Arguments)
				if err != nil {
					return inputNames, err
				}
				inputNames = append(inputNames, vars...)
				varFiles = append(varFiles, rawVarFiles...)
			}
		}
	}

	fileVars, err := getVarNamesFromVarFiles(varFiles)
	if err != nil {
		return inputNames, err
	}
	inputNames = append(inputNames, fileVars...)

	return inputNames, nil
}

// getTerraformInputNamesFromAutomaticVarFiles returns all the variables names
func getTerraformInputNamesFromAutomaticVarFiles(terragruntOptions *options.TerragruntOptions) ([]string, error) {
	base := terragruntOptions.WorkingDir
	automaticVarFiles := []string{}

	tfTFVarsFile := filepath.Join(base, "terraform.tfvars")
	if util.FileExists(tfTFVarsFile) {
		automaticVarFiles = append(automaticVarFiles, tfTFVarsFile)
	}

	tfTFVarsJsonFile := filepath.Join(base, "terraform.tfvars.json")
	if util.FileExists(tfTFVarsJsonFile) {
		automaticVarFiles = append(automaticVarFiles, tfTFVarsJsonFile)
	}

	varFiles, err := filepath.Glob(filepath.Join(base, "*.auto.tfvars"))
	if err != nil {
		return nil, err
	}
	automaticVarFiles = append(automaticVarFiles, varFiles...)
	jsonVarFiles, err := filepath.Glob(filepath.Join(base, "*.auto.tfvars.json"))
	if err != nil {
		return nil, err
	}
	automaticVarFiles = append(automaticVarFiles, jsonVarFiles...)
	return getVarNamesFromVarFiles(automaticVarFiles)
}

// getVarNamesFromVarFiles will parse all the given var files and returns a list of names of variables that are
// configured in all of them combined together.
func getVarNamesFromVarFiles(varFiles []string) ([]string, error) {
	inputNames := []string{}
	for _, varFile := range varFiles {
		fileVars, err := getVarNamesFromVarFile(varFile)
		if err != nil {
			return inputNames, err
		}
		inputNames = append(inputNames, fileVars...)
	}
	return inputNames, nil
}

// getVarNamesFromVarFile will parse the given terraform var file and return a list of names of variables that are
// configured in that var file.
func getVarNamesFromVarFile(varFile string) ([]string, error) {
	fileContents, err := ioutil.ReadFile(varFile)
	if err != nil {
		return nil, err
	}

	var variables map[string]interface{}
	if strings.HasSuffix(varFile, "json") {
		if err := json.Unmarshal(fileContents, &variables); err != nil {
			return nil, err
		}
	} else {
		if err := config.ParseAndDecodeVarFile(string(fileContents), varFile, &variables); err != nil {
			return nil, err
		}
	}

	out := []string{}
	for varName := range variables {
		out = append(out, varName)
	}
	return out, nil
}

// Returns the CLI flags defined on the provided arguments list that correspond to -var and -var-file. Returns two
// slices, one for `-var` args (the first one) and one for `-var-file` args (the second one).
func getVarFlagsFromArgList(argList []string) ([]string, []string, error) {
	vars := []string{}
	varFiles := []string{}

	for _, arg := range argList {
		// Use shlex to handle shell style quoting rules. This will reduce quoted args to remove quoting rules. For
		// example, the string:
		// -var="'"foo"'"='bar'
		// becomes:
		// -var='foo'=bar
		shlexedArgSlice, err := shlex.Split(arg)
		if err != nil {
			return vars, varFiles, err
		}
		// Since we expect each element in extra_args.arguments to correspond to a single arg for terraform, we join
		// back the shlex split slice even if it thinks there are multiple.
		shlexedArg := strings.Join(shlexedArgSlice, " ")

		if strings.HasPrefix(shlexedArg, "-var=") {
			// -var is passed in in the format -var=VARNAME=VALUE, so we split on '=' and take the middle value.
			splitArg := strings.Split(shlexedArg, "=")
			if len(splitArg) < 2 {
				return vars, varFiles, fmt.Errorf("Unexpected -var arg format in terraform.extra_arguments.arguments. Expected '-var=VARNAME=VALUE', got %s.", arg)
			}
			vars = append(vars, splitArg[1])
		}
		if strings.HasPrefix(shlexedArg, "-var-file=") {
			varFiles = append(varFiles, strings.TrimPrefix(shlexedArg, "-var-file="))
		}
	}

	return vars, varFiles, nil
}
