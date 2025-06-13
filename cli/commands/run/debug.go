package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

const TerragruntTFVarsFile = "terragrunt-debug.tfvars.json"

const defaultPermissions = int(0600)

// WriteTerragruntDebugFile will create a tfvars file that can be used to invoke the terraform module in the same way
// that terragrunt invokes the module, so that you can debug issues with the terragrunt config.
func WriteTerragruntDebugFile(l log.Logger, opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
	l.Infof(
		"Debug mode requested: generating debug file %s in working dir %s",
		TerragruntTFVarsFile,
		opts.WorkingDir,
	)

	required, optional, err := tf.ModuleVariables(opts.WorkingDir)
	if err != nil {
		return err
	}

	variables := append(required, optional...)

	l.Debugf("The following variables were detected in the terraform module:")
	l.Debugf("%v", variables)

	fileContents, err := terragruntDebugFileContents(l, opts, cfg, variables)
	if err != nil {
		return err
	}

	configFolder := filepath.Dir(opts.TerragruntConfigPath)

	fileName := filepath.Join(configFolder, TerragruntTFVarsFile)
	if err := os.WriteFile(fileName, fileContents, os.FileMode(defaultPermissions)); err != nil {
		return errors.New(err)
	}

	l.Debugf("Variables passed to terraform are located in \"%s\"", fileName)
	l.Debugf("Run this command to replicate how terraform was invoked:")
	l.Debugf(
		"\tterraform -chdir=\"%s\" %s -var-file=\"%s\" ",
		opts.WorkingDir,
		strings.Join(opts.TerraformCliArgs, " "),
		fileName,
	)

	return nil
}

// terragruntDebugFileContents will return a tfvars file in json format of all the terragrunt rendered variables values
// that should be set to invoke the terraform module in the same way as terragrunt. Note that this will only include the
// values of variables that are actually defined in the module.
func terragruntDebugFileContents(
	l log.Logger,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
	moduleVariables []string,
) ([]byte, error) {
	envVars := map[string]string{}
	if opts.Env != nil {
		envVars = opts.Env
	}

	jsonValuesByKey := make(map[string]any)

	for varName, varValue := range cfg.Inputs {
		nameAsEnvVar := fmt.Sprintf(tf.EnvNameTFVarFmt, varName)
		_, varIsInEnv := envVars[nameAsEnvVar]
		varIsDefined := util.ListContainsElement(moduleVariables, varName)

		// Only add to the file if the explicit env var does NOT exist and the variable is defined in the module.
		// We must do this in order to avoid overriding the env var when the user follows up with a direct invocation to
		// terraform using this file (due to the order in which terraform resolves config sources).
		switch {
		case !varIsInEnv && varIsDefined:
			jsonValuesByKey[varName] = varValue
		case varIsInEnv:
			l.Debugf(
				"WARN: The variable %s was omitted from the debug file because the env var %s is already set.",
				varName, nameAsEnvVar,
			)
		case !varIsDefined:
			l.Debugf(
				"WARN: The variable %s was omitted because it is not defined in the terraform module.",
				varName,
			)
		}
	}

	jsonContent, err := json.MarshalIndent(jsonValuesByKey, "", "  ")
	if err != nil {
		return nil, errors.New(err)
	}

	return jsonContent, nil
}
