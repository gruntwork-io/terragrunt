package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

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

	awsProviderPatchOverrides, err := parseMutliStringKeyValueArg(args, OPT_TERRAGRUNT_OVERRIDE_ATTR, nil)
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

	// Those correspond to logrus levels
	logLevel, err := parseStringArg(args, OPT_TERRAGRUNT_LOGLEVEL, util.DEFAULT_LOG_LEVEL.String())
	if err != nil {
		return nil, err
	}

	loggingLevel, err := logrus.ParseLevel(logLevel)
	if err != nil {
		util.GlobalFallbackLogEntry.Errorf(err.Error())
		return nil, err
	}

	opts, err := options.NewTerragruntOptions(filepath.ToSlash(terragruntConfigPath))
	if err != nil {
		return nil, err
	}

	debug := parseBooleanArg(args, OPT_TERRAGRUNT_DEBUG, false)
	if debug {
		opts.Debug = true
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
	opts.OriginalTerraformCommand = util.FirstArg(opts.TerraformCliArgs)
	opts.TerraformCommand = util.FirstArg(opts.TerraformCliArgs)
	opts.WorkingDir = filepath.ToSlash(workingDir)
	opts.DownloadDir = filepath.ToSlash(downloadDir)
	opts.LogLevel = loggingLevel
	opts.Logger = util.CreateLogEntry("", loggingLevel)
	opts.Logger.Logger.SetOutput(errWriter)
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
	opts.AwsProviderPatchOverrides = awsProviderPatchOverrides

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
					varFiles := arg.GetVarFiles(terragruntOptions.Logger)
					for _, file := range varFiles {
						out = append(out, fmt.Sprintf("-var-file=%s", file))
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

// isDeprecatedOption checks if provided option is deprecated, and returns its substitution with the check
// if option is not deprecated - we are returning same value
func isDeprecatedOption(optionName string) (string, bool) {
	newOption, deprecated := DEPRECATED_ARGUMENTS[optionName]
	if deprecated {
		return newOption, true
	}
	return optionName, false
}

// Find a boolean argument (e.g. --foo) of the given name in the given list of arguments. If it's present, return true.
// If it isn't, return defaultValue.
func parseBooleanArg(args []string, argName string, defaultValue bool) bool {
	for _, arg := range args {
		if arg == fmt.Sprintf("--%s", argName) {
			newOption, deprecated := isDeprecatedOption(argName)
			if deprecated {
				util.GlobalFallbackLogEntry.Warnf("Command line option %s is deprecated, please consider using %s", argName, newOption)
			}
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
			newOption, deprecated := isDeprecatedOption(argName)
			if deprecated {
				util.GlobalFallbackLogEntry.Warnf("Command line option %s is deprecated, please consider using %s", argName, newOption)
			}
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
			newOption, deprecated := isDeprecatedOption(argName)
			if deprecated {
				util.GlobalFallbackLogEntry.Warnf("Command line option %s is deprecated, please consider using %s", argName, newOption)
			}
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
			newOption, deprecated := isDeprecatedOption(argName)
			if deprecated {
				util.GlobalFallbackLogEntry.Warnf("Command line option %s is deprecated, please consider using %s", argName, newOption)
			}
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

// Find multiple key=vallue arguments of the same type (e.g. --foo "KEY_A=VALUE_A" --foo "KEY_B=VALUE_B") of the given name in the given list of arguments. If there are any present,
// return a map of all values. If there are any present, but one of them has no value, return an error. If there aren't any present, return defaultValue.
func parseMutliStringKeyValueArg(args []string, argName string, defaultValue map[string]string) (map[string]string, error) {
	asList, err := parseMultiStringArg(args, argName, nil)
	if err != nil {
		return nil, err
	}

	if asList == nil {
		return defaultValue, nil
	}

	asMap := map[string]string{}
	for _, arg := range asList {
		parts := strings.Split(arg, "=")
		if len(parts) != 2 {
			return nil, errors.WithStackTrace(InvalidKeyValue(arg))
		}

		key := parts[0]
		value := parts[1]

		asMap[key] = value
	}

	return asMap, nil
}

// Convert the given variables to a map of environment variables that will expose those variables to Terraform. The
// keys will be of the format TF_VAR_xxx and the values will be converted to JSON, which Terraform knows how to read
// natively.
func toTerraformEnvVars(vars map[string]interface{}) (map[string]string, error) {
	out := map[string]string{}

	for varName, varValue := range vars {
		envVarName := fmt.Sprintf("%s_%s", TFVarPrefix, varName)

		envVarValue, err := asTerraformEnvVarJsonValue(varValue)
		if err != nil {
			return nil, err
		}

		out[envVarName] = string(envVarValue)
	}

	return out, nil
}

// Convert the given value to a JSON value that can be passed to Terraform as an environment variable. For the most
// part, this converts the value directly to JSON using Go's built-in json.Marshal. However, we have special handling
// for strings, which with normal JSON conversion would be wrapped in quotes, but when passing them to Terraform via
// env vars, we need to NOT wrap them in quotes, so this method adds special handling for that case.
func asTerraformEnvVarJsonValue(value interface{}) (string, error) {
	switch val := value.(type) {
	case string:
		return val, nil
	default:
		envVarValue, err := json.Marshal(val)
		if err != nil {
			return "", errors.WithStackTrace(err)
		}
		return string(envVarValue), nil
	}
}

// Custom error types

type ArgMissingValue string

func (err ArgMissingValue) Error() string {
	return fmt.Sprintf("You must specify a value for the --%s option", string(err))
}

type InvalidKeyValue string

func (err InvalidKeyValue) Error() string {
	return fmt.Sprintf("Invalid key-value pair. Expected format KEY=VALUE, got %s.", string(err))
}
