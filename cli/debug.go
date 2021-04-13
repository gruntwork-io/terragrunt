package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const TerragruntTFVarsFile = "terragrunt-debug.tfvars.json"

// writeTerragruntDebugFile will create a tfvars file that can be used to invoke the terraform module in the same way
// that terragrunt invokes the module, so that you can debug issues with the terragrunt config.
func writeTerragruntDebugFile(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	terragruntOptions.Logger.Infof(
		"Debug mode requested: generating debug file %s in working dir %s",
		TerragruntTFVarsFile,
		terragruntOptions.WorkingDir,
	)

	required, optional, err := terraformModuleVariables(terragruntOptions)
	if err != nil {
		return err
	}
	variables := append(required, optional...)

	terragruntOptions.Logger.Debugf("The following variables were detected in the terraform module:")
	terragruntOptions.Logger.Debugf("%v", variables)

	fileContents, err := terragruntDebugFileContents(terragruntOptions, terragruntConfig, variables)
	if err != nil {
		return err
	}

	configFolder := filepath.Dir(terragruntOptions.TerragruntConfigPath)
	fileName := filepath.Join(configFolder, TerragruntTFVarsFile)
	if err := ioutil.WriteFile(fileName, fileContents, os.FileMode(int(0600))); err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Variables passed to terraform are located in \"%s\"", fileName)
	terragruntOptions.Logger.Debugf("Run this command to replicate how terraform was invoked:")
	terragruntOptions.Logger.Debugf(
		"\tterraform %s -var-file=\"%s\" \"%s\"",
		strings.Join(terragruntOptions.TerraformCliArgs, " "),
		fileName,
		terragruntOptions.WorkingDir,
	)
	return nil
}

// terragruntDebugFileContents will return a tfvars file in json format of all the terragrunt rendered variables values
// that should be set to invoke the terraform module in the same way as terragrunt. Note that this will only include the
// values of variables that are actually defined in the module.
func terragruntDebugFileContents(
	terragruntOptions *options.TerragruntOptions,
	terragruntConfig *config.TerragruntConfig,
	moduleVariables []string,
) ([]byte, error) {
	envVars := map[string]string{}
	if terragruntOptions.Env != nil {
		envVars = terragruntOptions.Env
	}

	jsonValuesByKey := make(map[string]interface{})
	for varName, varValue := range terragruntConfig.Inputs {
		nameAsEnvVar := fmt.Sprintf("%s_%s", TFVarPrefix, varName)
		_, varIsInEnv := envVars[nameAsEnvVar]
		varIsDefined := util.ListContainsElement(moduleVariables, varName)

		// Only add to the file if the explicit env var does NOT exist and the variable is defined in the module.
		// We must do this in order to avoid overriding the env var when the user follows up with a direct invocation to
		// terraform using this file (due to the order in which terraform resolves config sources).
		if !varIsInEnv && varIsDefined {
			jsonValuesByKey[varName] = varValue
		} else if varIsInEnv {
			terragruntOptions.Logger.Debugf(
				"WARN: The variable %s was omitted from the debug file because the env var %s is already set.",
				varName, nameAsEnvVar,
			)
		} else if !varIsDefined {
			terragruntOptions.Logger.Debugf(
				"WARN: The variable %s was omitted because it is not defined in the terraform module.",
				varName,
			)
		}
	}
	jsonContent, err := json.MarshalIndent(jsonValuesByKey, "", "  ")
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return jsonContent, nil
}

// terraformModuleVariables will return all the variables defined in the downloaded terraform modules, taking into
// account all the generated sources. This function will return the required and optional variables separately.
func terraformModuleVariables(terragruntOptions *options.TerragruntOptions) ([]string, []string, error) {
	modulePath := terragruntOptions.WorkingDir
	module, diags := tfconfig.LoadModule(modulePath)
	if diags.HasErrors() {
		return nil, nil, errors.WithStackTrace(diags)
	}

	required := []string{}
	optional := []string{}
	for _, variable := range module.Variables {
		if variable.Required {
			required = append(required, variable.Name)
		} else {
			optional = append(optional, variable.Name)
		}
	}
	return required, optional, nil
}
