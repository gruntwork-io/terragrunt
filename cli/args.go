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

// ParseTerragruntOptions Parse command line options that are passed in for Terragrunt
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
	workingDir, err := parseStringArg(args, optWorkingDir, defaultWorkingDir)
	if err != nil {
		return nil, err
	}

	downloadDirRaw, err := parseStringArg(args, optDownloadDir, os.Getenv("TERRAGRUNT_DOWNLOAD"))
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

	terragruntConfigPath, err := parseStringArg(args, optTerragruntConfig, os.Getenv("TERRAGRUNT_CONFIG"))
	if err != nil {
		return nil, err
	}
	if terragruntConfigPath == "" {
		terragruntConfigPath = config.GetDefaultConfigPath(workingDir)
	}

	terragruntHclFilePath, err := parseStringArg(args, optTerragruntHCLFmt, "")
	if err != nil {
		return nil, err
	}

	awsProviderPatchOverrides, err := parseMutliStringKeyValueArg(args, optTerragruntOverrideAttr, nil)
	if err != nil {
		return nil, err
	}

	terraformPath, err := parseStringArg(args, optTerragruntTFPath, os.Getenv("TERRAGRUNT_TFPATH"))
	if err != nil {
		return nil, err
	}
	if terraformPath == "" {
		terraformPath = options.TERRAFORM_DEFAULT_PATH
	}

	terraformSource, err := parseStringArg(args, optTerragruntSource, os.Getenv("TERRAGRUNT_SOURCE"))
	if err != nil {
		return nil, err
	}

	terraformSourceMapEnvVar, err := parseMultiStringKeyValueEnvVar("TERRAGRUNT_SOURCE_MAP")
	if err != nil {
		return nil, err
	}
	terraformSourceMap, err := parseMutliStringKeyValueArg(args, optTerragruntSourceMap, terraformSourceMapEnvVar)
	if err != nil {
		return nil, err
	}

	sourceUpdate := parseBooleanArg(args, optTerragruntSourceUpdate, os.Getenv("TERRAGRUNT_SOURCE_UPDATE") == "true" || os.Getenv("TERRAGRUNT_SOURCE_UPDATE") == "1")

	ignoreDependencyErrors := parseBooleanArg(args, optTerragruntIgnoreDependencyErrors, false)

	ignoreDependencyOrder := parseBooleanArg(args, optTerragruntIgnoreDependencyOrder, false)

	ignoreExternalDependencies := parseBooleanArg(args, optTerragruntIgnoreExternalDependencies, false)

	includeExternalDependencies := parseBooleanArg(args, optTerragruntIncludeExternalDependencies, os.Getenv("TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES") == "true")

	excludeDirs, err := parseMultiStringArg(args, optTerragruntExcludeDir, []string{})
	if err != nil {
		return nil, err
	}

	includeDirs, err := parseMultiStringArg(args, optTerragruntIncludeDir, []string{})
	if err != nil {
		return nil, err
	}

	strictInclude := parseBooleanArg(args, optTerragruntStrictInclude, false)

	modulesThatInclude, err := parseMultiStringArg(args, optTerragruntModulesThatInclude, []string{})
	if err != nil {
		return nil, err
	}

	// Those correspond to logrus levels
	logLevel, err := parseStringArg(args, optTerragruntLogLevel, util.GetDefaultLogLevel().String())
	if err != nil {
		return nil, err
	}

	loggingLevel, err := logrus.ParseLevel(logLevel)
	if err != nil {
		util.GlobalFallbackLogEntry.Errorf(err.Error())
		return nil, err
	}

	validateStrictMode := parseBooleanArg(args, optTerragruntStrictValidate, false)

	opts, err := options.NewTerragruntOptions(filepath.ToSlash(terragruntConfigPath))
	if err != nil {
		return nil, err
	}

	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath

	debug := parseBooleanArg(args, optTerragruntDebug, os.Getenv("TERRAGRUNT_DEBUG") == "true" || os.Getenv("TERRAGRUNT_DEBUG") == "1")
	if debug {
		opts.Debug = true
	}

	opts.RunAllAutoApprove = !parseBooleanArg(args, optTerragruntNoAutoApprove, os.Getenv("TERRAGRUNT_AUTO_APPROVE") == "false")

	var parallelism int
	if !opts.RunAllAutoApprove {
		// When running in no-auto-approve mode, set parallelism to 1 so that interactive prompts work.
		parallelism = 1
	} else {
		envValue, envProvided := os.LookupEnv("TERRAGRUNT_PARALLELISM")
		parsedParallelism, err := parseIntArg(args, optTerragruntParallelism, envValue, envProvided, options.DEFAULT_PARALLELISM)
		if err != nil {
			return nil, err
		}
		parallelism = parsedParallelism
	}
	opts.Parallelism = parallelism

	iamRoleOpts, err := parseIAMRoleOptions(args)
	if err != nil {
		return nil, err
	}
	// We don't need to check for nil, because parseIAMRoleOptions always returns a valid pointer when no error is
	// returned.
	opts.OriginalIAMRoleOptions = *iamRoleOpts
	opts.IAMRoleOptions = *iamRoleOpts

	opts.TerraformPath = filepath.ToSlash(terraformPath)
	opts.AutoInit = !parseBooleanArg(args, optTerragruntNoAutoInit, os.Getenv("TERRAGRUNT_AUTO_INIT") == "false")
	opts.AutoRetry = !parseBooleanArg(args, optTerragruntNoAutoRetry, os.Getenv("TERRAGRUNT_AUTO_RETRY") == "false")
	opts.NonInteractive = parseBooleanArg(args, optNonInteractive, os.Getenv("TF_INPUT") == "false" || os.Getenv("TF_INPUT") == "0")
	opts.TerraformCliArgs = filterTerragruntArgs(args)
	opts.OriginalTerraformCommand = util.FirstArg(opts.TerraformCliArgs)
	opts.TerraformCommand = util.FirstArg(opts.TerraformCliArgs)
	opts.WorkingDir = filepath.ToSlash(workingDir)
	opts.DownloadDir = filepath.ToSlash(downloadDir)
	opts.LogLevel = loggingLevel
	opts.ValidateStrict = validateStrictMode
	opts.Logger = util.CreateLogEntry("", loggingLevel)
	opts.Logger.Logger.SetOutput(errWriter)
	opts.RunTerragrunt = RunTerragrunt
	opts.Source = terraformSource
	opts.SourceMap = terraformSourceMap
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
	opts.ExcludeDirs = excludeDirs
	opts.IncludeDirs = includeDirs
	opts.ModulesThatInclude = modulesThatInclude
	opts.StrictInclude = strictInclude
	opts.Check = parseBooleanArg(args, optTerragruntCheck, os.Getenv("TERRAGRUNT_CHECK") == "true")
	opts.HclFile = filepath.ToSlash(terragruntHclFilePath)
	opts.AwsProviderPatchOverrides = awsProviderPatchOverrides
	opts.FetchDependencyOutputFromState = parseBooleanArg(args, optTerragruntFetchDependencyOutputFromState, os.Getenv("TERRAGRUNT_FETCH_DEPENDENCY_OUTPUT_FROM_STATE") == "true")
	opts.JSONOut, err = parseStringArg(args, optTerragruntJSONOut, "terragrunt_rendered.json")
	if err != nil {
		return nil, err
	}

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
		if util.ListContainsElement(MULTI_MODULE_COMMANDS, arg) {
			// Skip multi-module commands entirely
			continue
		}

		argWithoutPrefix := strings.TrimPrefix(arg, "--")
		if util.ListContainsElement(allTerragruntStringOpts, argWithoutPrefix) {
			// String flags that directly match the arg have the argument and the value, so skip both
			i = i + 1
			continue
		}
		if util.ListContainsElement(allTerragruntBooleanOpts, argWithoutPrefix) {
			// Just skip the boolean flag
			continue
		}

		// Handle the case where the terragrunt arg is passed in as --OPTION_KEY=VALUE
		if strings.Contains(argWithoutPrefix, "=") {
			argWithoutValue := strings.Split(argWithoutPrefix, "=")[0]
			if util.ListContainsElement(allTerragruntStringOpts, argWithoutValue) {
				// String args encoded as --OPTION_KEY=VALUE only need to skip the current arg
				continue
			}
		}

		out = append(out, arg)
	}
	return out
}

// logIfDeprecatedOption checks if provided option is deprecated, and logs a warning message if it is.
func logIfDeprecatedOption(optionName string) {
	newOption, deprecated := deprecatedArguments[optionName]
	if deprecated {
		util.GlobalFallbackLogEntry.Warnf("Command line option %s is deprecated, please consider using %s", optionName, newOption)
	}
}

// parseIAMRoleOptions parses the Terragrunt CLI args and converts them to the IAMRoleOptions struct. Note that this
// will always return the struct, even if none of the args were passed in. This is to ensure that we can correctly
// handle the case where the assume role parameters were passed in via CLI, but not the role ARN.
func parseIAMRoleOptions(args []string) (*options.IAMRoleOptions, error) {
	iamRole, err := parseStringArg(args, optTerragruntIAMRole, os.Getenv("TERRAGRUNT_IAM_ROLE"))
	if err != nil {
		return nil, err
	}

	envValue, envProvided := os.LookupEnv("TERRAGRUNT_IAM_ASSUME_ROLE_DURATION")
	iamAssumeRoleDuration, err := parseIntArg(args, optTerragruntIAMAssumeRoleDuration, envValue, envProvided, 0)
	if err != nil {
		return nil, err
	}

	defaultIamAssumeRoleSessionName := os.Getenv("TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME")
	iamAssumeRoleSessionName, err := parseStringArg(args, optTerragruntIAMAssumeRoleSessionName, defaultIamAssumeRoleSessionName)
	if err != nil {
		return nil, err
	}

	optsOut := &options.IAMRoleOptions{
		RoleARN:               iamRole,
		AssumeRoleDuration:    int64(iamAssumeRoleDuration),
		AssumeRoleSessionName: iamAssumeRoleSessionName,
	}
	return optsOut, nil
}

// getOptionArgIfMatched returns the value to use for the option key, if the arg matches the option. This will handle
// options passed in as --OPTION_KEY VALUE or --OPTION_KEY=VALUE. The second boolean return value indicates if the arg
// matches the option, returning false if it does not. This will error if it expects a value in the next arg, but the
// current arg is the end of the input.
func getOptionArgIfMatched(arg string, nextArg *string, optionName string) (string, bool, error) {
	optionOnly := fmt.Sprintf("--%s", optionName)
	if arg == optionOnly {
		logIfDeprecatedOption(optionName)
		if nextArg == nil {
			return "", true, errors.WithStackTrace(ArgMissingValue(optionName))
		}
		return *nextArg, true, nil
	} else if strings.HasPrefix(arg, optionOnly+"=") {
		logIfDeprecatedOption(optionName)
		return strings.TrimPrefix(arg, fmt.Sprintf("--%s=", optionName)), true, nil
	}
	return "", false, nil
}

// Find a boolean argument (e.g. --foo) of the given name in the given list of arguments. If it's present, return true.
// If it isn't, return defaultValue.
func parseBooleanArg(args []string, argName string, defaultValue bool) bool {
	for _, arg := range args {
		if arg == fmt.Sprintf("--%s", argName) {
			logIfDeprecatedOption(argName)
			return true
		}
	}
	return defaultValue
}

// Find a string argument (e.g. --foo "VALUE") of the given name in the given list of arguments. If it's present,
// return its value. If it is present, but has no value, return an error. If it isn't present, return defaultValue.
func parseStringArg(args []string, argName string, defaultValue string) (string, error) {
	for i, arg := range args {
		var nextArg *string
		if (i + 1) < len(args) {
			nextArg = &args[i+1]
		}
		val, hasVal, err := getOptionArgIfMatched(arg, nextArg, argName)
		if err != nil {
			return "", err
		} else if hasVal {
			return val, nil
		}
	}
	return defaultValue, nil
}

// Find a int argument (e.g. --foo 1) of the given name in the given list of arguments. If it's present,
// return its value. If it is present, but has no value, return an error. If it isn't present, return envValue if provided. If not provided, return defaultValue.
func parseIntArg(args []string, argName string, envValue string, envProvided bool, defaultValue int) (int, error) {
	for i, arg := range args {
		var nextArg *string
		if (i + 1) < len(args) {
			nextArg = &args[i+1]
		}
		val, hasVal, err := getOptionArgIfMatched(arg, nextArg, argName)
		if err != nil {
			return 0, err
		} else if hasVal {
			return strconv.Atoi(val)
		}
	}
	if envProvided {
		return strconv.Atoi(envValue)
	}
	return defaultValue, nil
}

// Find multiple string arguments of the same type (e.g. --foo "VALUE_A" --foo "VALUE_B") of the given name in the given list of arguments. If there are any present,
// return a list of all values. If there are any present, but one of them has no value, return an error. If there aren't any present, return defaultValue.
func parseMultiStringArg(args []string, argName string, defaultValue []string) ([]string, error) {
	stringArgs := []string{}

	for i, arg := range args {
		var nextArg *string
		if (i + 1) < len(args) {
			nextArg = &args[i+1]
		}
		val, hasVal, err := getOptionArgIfMatched(arg, nextArg, argName)
		if err != nil {
			return nil, err
		} else if hasVal {
			stringArgs = append(stringArgs, val)
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
	return util.KeyValuePairStringListToMap(asList)
}

// Parses an environment variable that is encoded as a comma separated kv pair (e.g.,
// `key1=value1,key2=value2,key3=value3`) and converts it to a map. Returns empty map if the environnment variable is
// not set, and error if the environment variable is not encoded as a comma separated kv pair.
func parseMultiStringKeyValueEnvVar(envVarName string) (map[string]string, error) {
	rawEnvVarVal := os.Getenv(envVarName)
	if rawEnvVarVal == "" {
		return map[string]string{}, nil
	}
	mappingsAsList := strings.Split(rawEnvVarVal, ",")
	return util.KeyValuePairStringListToMap(mappingsAsList)
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
