package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/urfave/cli"
)

const TerragruntTFVarsFileName = "terragrunt-generated.auto.tfvars.json"

// Parse command line options that are passed in for Terragrunt
func ParseTerragruntOptions(cliContext *cli.Context) (*options.TerragruntOptions, error) {
	terragruntOptions, err := parseTerragruntOptionsFromArgs(cliContext.App.Version, cliContext.Args(), cliContext.App.Writer, cliContext.App.ErrWriter)
	if err != nil {
		return nil, err
	}
	return terragruntOptions, nil
}

// TODO: replace the urfave CLI library with something else.
//
// EXPLANATION: The normal way to parse flags with the urfave CLI library would be to define the flags in the
// CreateTerragruntCLI method and to read the values of those flags using cliContext.String(...),
// cliContext.Bool(...), etc. Unfortunately, this does not work here due to a limitation in the urfave
// CLI library: if the user passes in any "command" whatsoever, (e.g. the "apply" in "terragrunt apply"), then
// any flags that come after it are not parsed (e.g. the "--foo" is not parsed in "terragrunt apply --foo").
// Therefore, we have to parse options ourselves, which is infuriating. For more details on this limitation,
// see: https://github.com/urfave/cli/issues/533. For now, our workaround is to dumbly loop over the arguments
// and look for the ones we need, but in the future, we should change to a different CLI library to avoid this
// limitation.
func parseTerragruntOptionsFromArgs(terragruntVersion string, args []string, writer, errWriter io.Writer) (*options.TerragruntOptions, error) {
	defaultWorkingDir := os.Getenv("TERRAGRUNT_WORKING_DIR")
	if defaultWorkingDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		defaultWorkingDir = currentDir
	}
	workingDir, err := parseStringArg(args, OPT_WORKING_DIR, defaultWorkingDir)
	if err != nil {
		return nil, err
	}

	downloadDirRaw, err := parseStringArg(args, OPT_DOWNLOAD_DIR, os.Getenv("TERRAGRUNT_DOWNLOAD"))
	if err != nil {
		return nil, err
	}
	if downloadDirRaw == "" {
		downloadDirRaw = util.JoinPath(workingDir, options.TerragruntCacheDir)
	}
	downloadDir, err := filepath.Abs(downloadDirRaw)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	terragruntConfigPath, err := parseStringArg(args, OPT_TERRAGRUNT_CONFIG, os.Getenv("TERRAGRUNT_CONFIG"))
	if err != nil {
		return nil, err
	}
	if terragruntConfigPath == "" {
		terragruntConfigPath = config.GetDefaultConfigPath(workingDir)
	}

	terragruntHclFilePath, err := parseStringArg(args, OPT_TERRAGRUNT_HCLFMT_FILE, "")
	if err != nil {
		return nil, err
	}

	terraformPath, err := parseStringArg(args, OPT_TERRAGRUNT_TFPATH, os.Getenv("TERRAGRUNT_TFPATH"))
	if err != nil {
		return nil, err
	}
	if terraformPath == "" {
		terraformPath = options.TERRAFORM_DEFAULT_PATH
	}

	terraformSource, err := parseStringArg(args, OPT_TERRAGRUNT_SOURCE, os.Getenv("TERRAGRUNT_SOURCE"))
	if err != nil {
		return nil, err
	}

	sourceUpdate := parseBooleanArg(args, OPT_TERRAGRUNT_SOURCE_UPDATE, os.Getenv("TERRAGRUNT_SOURCE_UPDATE") == "true" || os.Getenv("TERRAGRUNT_SOURCE_UPDATE") == "1")

	ignoreDependencyErrors := parseBooleanArg(args, OPT_TERRAGRUNT_IGNORE_DEPENDENCY_ERRORS, false)

	ignoreDependencyOrder := parseBooleanArg(args, OPT_TERRAGRUNT_IGNORE_DEPENDENCY_ORDER, false)

	ignoreExternalDependencies := parseBooleanArg(args, OPT_TERRAGRUNT_IGNORE_EXTERNAL_DEPENDENCIES, false)

	includeExternalDependencies := parseBooleanArg(args, OPT_TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES, false)

	iamRole, err := parseStringArg(args, OPT_TERRAGRUNT_IAM_ROLE, os.Getenv("TERRAGRUNT_IAM_ROLE"))
	if err != nil {
		return nil, err
	}

	excludeDirs, err := parseMultiStringArg(args, OPT_TERRAGRUNT_EXCLUDE_DIR, []string{})
	if err != nil {
		return nil, err
	}

	includeDirs, err := parseMultiStringArg(args, OPT_TERRAGRUNT_INCLUDE_DIR, []string{})
	if err != nil {
		return nil, err
	}

	strictInclude := parseBooleanArg(args, OPT_TERRAGRUNT_STRICT_INCLUDE, false)

	opts, err := options.NewTerragruntOptions(filepath.ToSlash(terragruntConfigPath))
	if err != nil {
		return nil, err
	}

	envValue, envProvided := os.LookupEnv("TERRAGRUNT_PARALLELISM")
	parallelism, err := parseIntArg(args, OPT_TERRAGRUNT_PARALLELISM, envValue, envProvided, options.DEFAULT_PARALLELISM)
	if err != nil {
		return nil, err
	}

	opts.TerraformPath = filepath.ToSlash(terraformPath)
	opts.AutoInit = !parseBooleanArg(args, OPT_TERRAGRUNT_NO_AUTO_INIT, os.Getenv("TERRAGRUNT_AUTO_INIT") == "false")
	opts.AutoRetry = !parseBooleanArg(args, OPT_TERRAGRUNT_NO_AUTO_RETRY, os.Getenv("TERRAGRUNT_AUTO_RETRY") == "false")
	opts.NonInteractive = parseBooleanArg(args, OPT_NON_INTERACTIVE, os.Getenv("TF_INPUT") == "false" || os.Getenv("TF_INPUT") == "0")
	opts.TerraformCliArgs = filterTerragruntArgs(args)
	opts.TerraformCommand = util.FirstArg(opts.TerraformCliArgs)
	opts.WorkingDir = filepath.ToSlash(workingDir)
	opts.DownloadDir = filepath.ToSlash(downloadDir)
	opts.Logger = util.CreateLoggerWithWriter(errWriter, "")
	opts.RunTerragrunt = RunTerragrunt
	opts.Source = terraformSource
	opts.SourceUpdate = sourceUpdate
	opts.TerragruntVersion, err = version.NewVersion(terragruntVersion)
	if err != nil {
		// Malformed Terragrunt version; set the version to 0.0
		opts.TerragruntVersion, err = version.NewVersion("0.0")
		if err != nil {
			return nil, err
		}
	}
	opts.IgnoreDependencyErrors = ignoreDependencyErrors
	opts.IgnoreDependencyOrder = ignoreDependencyOrder
	opts.IgnoreExternalDependencies = ignoreExternalDependencies
	opts.IncludeExternalDependencies = includeExternalDependencies
	opts.Writer = writer
	opts.ErrWriter = errWriter
	opts.Env = parseEnvironmentVariables(os.Environ())
	opts.IamRole = iamRole
	opts.ExcludeDirs = excludeDirs
	opts.IncludeDirs = includeDirs
	opts.StrictInclude = strictInclude
	opts.Parallelism = parallelism
	opts.Check = parseBooleanArg(args, OPT_TERRAGRUNT_CHECK, os.Getenv("TERRAGRUNT_CHECK") == "true")
	opts.HclFile = filepath.ToSlash(terragruntHclFilePath)

	return opts, nil
}

func filterTerraformExtraArgs(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []string {
	out := []string{}
	cmd := util.FirstArg(terragruntOptions.TerraformCliArgs)

	for _, arg := range terragruntConfig.Terraform.ExtraArgs {
		for _, arg_cmd := range arg.Commands {
			if cmd == arg_cmd {
				lastArg := util.LastArg(terragruntOptions.TerraformCliArgs)
				skipVars := (cmd == "apply" || cmd == "destroy") && util.IsFile(lastArg)

				// The following is a fix for GH-493.
				// If the first argument is "apply" and the second argument is a file (plan),
				// we don't add any -var-file to the command.
				if arg.Arguments != nil {
					if skipVars {
						// If we have to skip vars, we need to iterate over all elements of array...
						for _, a := range *arg.Arguments {
							if !strings.HasPrefix(a, "-var") {
								out = append(out, a)
							}
						}
					} else {
						// ... Otherwise, let's add all the arguments
						out = append(out, *arg.Arguments...)
					}
				}

				if !skipVars {
					// If RequiredVarFiles is specified, add -var-file=<file> for each specified files
					if arg.RequiredVarFiles != nil {
						for _, file := range util.RemoveDuplicatesFromListKeepLast(*arg.RequiredVarFiles) {
							out = append(out, fmt.Sprintf("-var-file=%s", file))
						}
					}

					// If OptionalVarFiles is specified, check for each file if it exists and if so, add -var-file=<file>
					// It is possible that many files resolve to the same path, so we remove duplicates.
					if arg.OptionalVarFiles != nil {
						for _, file := range util.RemoveDuplicatesFromListKeepLast(*arg.OptionalVarFiles) {
							if util.FileExists(file) {
								out = append(out, fmt.Sprintf("-var-file=%s", file))
							} else {
								terragruntOptions.Logger.Printf("Skipping var-file %s as it does not exist", file)
							}
						}
					}
				}
			}
		}
	}

	return out
}

func filterTerraformEnvVarsFromExtraArgs(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) map[string]string {
	out := map[string]string{}
	cmd := util.FirstArg(terragruntOptions.TerraformCliArgs)

	for _, arg := range terragruntConfig.Terraform.ExtraArgs {
		if arg.EnvVars == nil {
			continue
		}
		for _, argcmd := range arg.Commands {
			if cmd == argcmd {
				for k, v := range *arg.EnvVars {
					out[k] = v
				}
			}
		}
	}

	return out
}

func parseEnvironmentVariables(environment []string) map[string]string {
	environmentMap := make(map[string]string)

	for i := 0; i < len(environment); i++ {
		variableSplit := strings.SplitN(environment[i], "=", 2)

		if len(variableSplit) == 2 {
			environmentMap[strings.TrimSpace(variableSplit[0])] = variableSplit[1]
		}
	}

	return environmentMap
}

// Return a copy of the given args with all Terragrunt-specific args removed
func filterTerragruntArgs(args []string) []string {
	out := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		argWithoutPrefix := strings.TrimPrefix(arg, "--")

		if util.ListContainsElement(MULTI_MODULE_COMMANDS, arg) {
			// Skip multi-module commands entirely
			continue
		}

		if util.ListContainsElement(ALL_TERRAGRUNT_STRING_OPTS, argWithoutPrefix) {
			// String flags have the argument and the value, so skip both
			i = i + 1
			continue
		}
		if util.ListContainsElement(ALL_TERRAGRUNT_BOOLEAN_OPTS, argWithoutPrefix) {
			// Just skip the boolean flag
			continue
		}

		out = append(out, arg)
	}
	return out
}

// Find a boolean argument (e.g. --foo) of the given name in the given list of arguments. If it's present, return true.
// If it isn't, return defaultValue.
func parseBooleanArg(args []string, argName string, defaultValue bool) bool {
	for _, arg := range args {
		if arg == fmt.Sprintf("--%s", argName) {
			return true
		}
	}
	return defaultValue
}

// Find a string argument (e.g. --foo "VALUE") of the given name in the given list of arguments. If it's present,
// return its value. If it is present, but has no value, return an error. If it isn't present, return defaultValue.
func parseStringArg(args []string, argName string, defaultValue string) (string, error) {
	for i, arg := range args {
		if arg == fmt.Sprintf("--%s", argName) {
			if (i + 1) < len(args) {
				return args[i+1], nil
			} else {
				return "", errors.WithStackTrace(ArgMissingValue(argName))
			}
		}
	}
	return defaultValue, nil
}

// Find a int argument (e.g. --foo 1) of the given name in the given list of arguments. If it's present,
// return its value. If it is present, but has no value, return an error. If it isn't present, return envValue if provided. If not provided, return defaultValue.
func parseIntArg(args []string, argName string, envValue string, envProvided bool, defaultValue int) (int, error) {
	for i, arg := range args {
		if arg == fmt.Sprintf("--%s", argName) {
			if (i + 1) < len(args) {
				return strconv.Atoi(args[i+1])
			} else {
				return 0, errors.WithStackTrace(ArgMissingValue(argName))
			}
		}
	}
	if envProvided {
		return strconv.Atoi(envValue)
	} else {
		return defaultValue, nil
	}
}

// Find multiple string arguments of the same type (e.g. --foo "VALUE_A" --foo "VALUE_B") of the given name in the given list of arguments. If there are any present,
// return a list of all values. If there are any present, but one of them has no value, return an error. If there aren't any present, return defaultValue.
func parseMultiStringArg(args []string, argName string, defaultValue []string) ([]string, error) {
	stringArgs := []string{}

	for i, arg := range args {
		if arg == fmt.Sprintf("--%s", argName) {
			if (i + 1) < len(args) {
				stringArgs = append(stringArgs, args[i+1])
			} else {
				return nil, errors.WithStackTrace(ArgMissingValue(argName))
			}
		}
	}
	if len(stringArgs) == 0 {
		return defaultValue, nil
	}

	return stringArgs, nil
}

// writeTFVarsFile will create a tfvars file that can be used to invoke the terraform module with the inputs generated
// in terragrunt.
func writeTFVarsFile(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	terragruntOptions.Logger.Printf("Generating tfvars file %s in working dir (%s)", TerragruntTFVarsFileName, terragruntOptions.WorkingDir)

	variables, err := terraformModuleVariables(terragruntOptions)
	if err != nil {
		return err
	}
	util.Debugf(terragruntOptions.Logger, "The following variables were detected in the terraform module:")
	util.Debugf(terragruntOptions.Logger, "%v", variables)

	fileContents, err := terragruntTFVarsFileContents(terragruntOptions, terragruntConfig, variables)
	if err != nil {
		return err
	}

	fileName := filepath.Join(terragruntOptions.WorkingDir, TerragruntTFVarsFileName)

	// If the file already exists, log a warning indicating that we will overwrite it.
	if util.FileExists(fileName) {
		terragruntOptions.Logger.Printf(
			"WARNING: File with name \"%s\" already exists in terraform working directory. This file will be replaced with terragrunt generated vars",
			TerragruntTFVarsFileName,
		)
	}

	if err := ioutil.WriteFile(fileName, fileContents, os.FileMode(int(0600))); err != nil {
		return errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Printf("Successfully generated tfvars file to pass to terraform (%s)", fileName)
	return nil
}

// terragruntTFVarsFileContents will return a tfvars file in json format of all the terragrunt rendered variables values
// that should be set to invoke the terraform module in the same way as terragrunt. Note that this will only include the
// values of variables that are actually defined in the module.
func terragruntTFVarsFileContents(
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
		nameAsEnvVar := fmt.Sprintf("TF_VAR_%s", varName)
		_, varIsInEnv := envVars[nameAsEnvVar]
		varIsDefined := util.ListContainsElement(moduleVariables, varName)

		// Only add to the file if the explicit env var does NOT exist and the variable is defined in the module.
		// We must do this in order to avoid overriding the env var when the user follows up with a direct invocation to
		// terraform using this file (due to the order in which terraform resolves config sources).
		if !varIsInEnv && varIsDefined {
			jsonValuesByKey[varName] = varValue
		} else if varIsInEnv {
			util.Debugf(
				terragruntOptions.Logger,
				"WARN: The variable %s was omitted from the debug file because the env var %s is already set.",
				varName, nameAsEnvVar,
			)
		} else if !varIsDefined {
			util.Debugf(
				terragruntOptions.Logger,
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
// account all the generated sources.
func terraformModuleVariables(terragruntOptions *options.TerragruntOptions) ([]string, error) {
	modulePath := terragruntOptions.WorkingDir
	module, diags := tfconfig.LoadModule(modulePath)
	if diags.HasErrors() {
		return nil, errors.WithStackTrace(diags)
	}

	variables := []string{}
	for _, variable := range module.Variables {
		variables = append(variables, variable.Name)
	}
	return variables, nil
}

// Custom error types

type ArgMissingValue string

func (err ArgMissingValue) Error() string {
	return fmt.Sprintf("You must specify a value for the --%s option", string(err))
}
