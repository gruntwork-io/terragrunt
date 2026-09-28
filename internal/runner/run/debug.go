package run

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const TerragruntTFVarsFile = "terragrunt-debug.tfvars.json"

const defaultPermissions = int(0600)

// WriteTerragruntDebugFile will create a tfvars file that can be used to invoke the tofu/terraform module in the same way
// that terragrunt invokes the module, so that users can debug issues with the terragrunt config.
func WriteTerragruntDebugFile(
	l log.Logger,
	v *venv.Venv,
	opts *Options,
	cfg *runcfg.RunConfig,
) error {
	l.Infof(
		"Debug mode requested: generating debug file %s in working dir %s",
		TerragruntTFVarsFile,
		opts.CacheDir,
	)

	declared, err := tf.ModuleVariables(v.FS, opts.CacheDir)
	if err != nil {
		return err
	}

	variables := slices.Sorted(maps.Keys(declared))

	tofuImpl := "tofu"
	if opts.TofuImplementation != "" {
		tofuImpl = string(opts.TofuImplementation)
	}

	l.Debugf("The following variables were detected in the %s module:", tofuImpl)
	l.Debugf("%v", variables)

	configFolder := filepath.Dir(opts.TerragruntConfigPath)

	fileName := filepath.Join(configFolder, TerragruntTFVarsFile)
	if err := vfs.StreamFileAtomic(
		v.FS,
		fileName,
		os.FileMode(defaultPermissions),
		func(w io.Writer) error {
			return writeDebugVars(w, l, v.Env, cfg, variables)
		},
	); err != nil {
		return err
	}

	l.Debugf("Variables passed to %s are located in \"%s\"", tofuImpl, fileName)
	l.Debugf("Run this command to replicate how %s was invoked:", tofuImpl)
	l.Debugf(
		"\t%s -chdir=\"%s\" %s -var-file=\"%s\" ",
		tofuImpl,
		opts.CacheDir,
		strings.Join(opts.TerraformCliArgs.Slice(), " "),
		fileName,
	)

	return nil
}

// terragruntDebugFileContents will return a tfvars file in json format of all the terragrunt rendered variables values
// that should be set to invoke the tofu/terraform module in the same way as terragrunt. Note that this will only include the
// values of variables that are actually defined in the module.
func writeDebugVars(
	w io.Writer,
	l log.Logger,
	env map[string]string,
	cfg *runcfg.RunConfig,
	moduleVariables []string,
) error {
	envVars := map[string]string{}
	if env != nil {
		envVars = env
	}

	jsonValuesByKey := make(map[string]any)

	for varName, varValue := range cfg.Inputs {
		nameAsEnvVar := fmt.Sprintf(tf.EnvNameTFVarFmt, varName)
		_, varIsInEnv := envVars[nameAsEnvVar]
		varIsDefined := slices.Contains(moduleVariables, varName)

		// Only add to the file if the explicit env var does NOT exist and the variable is defined in the module.
		// We must do this in order to avoid overriding the env var when the user follows up with a direct invocation to
		// tofu/terraform using this file (due to the order in which tofu/terraform resolves config sources).
		switch {
		case !varIsInEnv && varIsDefined:
			jsonValuesByKey[varName] = varValue
		case varIsInEnv:
			l.Debugf(
				"WARN: The variable %s was omitted from the debug file because the env var %s is already set.",
				varName,
				nameAsEnvVar,
			)
		case !varIsDefined:
			l.Debugf(
				"WARN: The variable %s was omitted because it is not defined in the OpenTofu/Terraform module.",
				varName,
			)
		}
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	return encoder.Encode(jsonValuesByKey)
}
