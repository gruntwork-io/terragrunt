package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/urfave/cli"
)

// Parse command line options that are passed in for Terragrunt
func ParseTerragruntOptions(cliContext *cli.Context) (*options.TerragruntOptions, error) {
	terragruntOptions, err := parseTerragruntOptionsFromArgs(cliContext.Args(), cliContext.App.Writer, cliContext.App.ErrWriter)
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
func parseTerragruntOptionsFromArgs(args []string, writer, errWriter io.Writer) (*options.TerragruntOptions, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	workingDir, err := parseStringArg(args, OPT_WORKING_DIR, currentDir)
	if err != nil {
		return nil, err
	}

	terragruntConfigPath, err := parseStringArg(args, OPT_TERRAGRUNT_CONFIG, os.Getenv("TERRAGRUNT_CONFIG"))
	if err != nil {
		return nil, err
	}
	if terragruntConfigPath == "" {
		terragruntConfigPath = config.DefaultConfigPath(workingDir)
	}

	terraformPath, err := parseStringArg(args, OPT_TERRAGRUNT_TFPATH, os.Getenv("TERRAGRUNT_TFPATH"))
	if err != nil {
		return nil, err
	}
	if terraformPath == "" {
		terraformPath = "terraform"
	}

	terraformSource, err := parseStringArg(args, OPT_TERRAGRUNT_SOURCE, os.Getenv("TERRAGRUNT_SOURCE"))
	if err != nil {
		return nil, err
	}

	sourceUpdate := parseBooleanArg(args, OPT_TERRAGRUNT_SOURCE_UPDATE, false)

	ignoreDependencyErrors := parseBooleanArg(args, OPT_TERRAGRUNT_IGNORE_DEPENDENCY_ERRORS, false)

	iamRole, err := parseStringArg(args, OPT_TERRAGRUNT_IAM_ROLE, os.Getenv("TERRAGRUNT_IAM_ROLE"))
	if err != nil {
		return nil, err
	}

	opts, err := options.NewTerragruntOptions(filepath.ToSlash(terragruntConfigPath))
	if err != nil {
		return nil, err
	}

	opts.TerraformPath = filepath.ToSlash(terraformPath)
	opts.AutoInit = !parseBooleanArg(args, OPT_TERRAGRUNT_NO_AUTO_INIT, os.Getenv("TERRAGRUNT_AUTO_INIT") == "false")
	opts.NonInteractive = parseBooleanArg(args, OPT_NON_INTERACTIVE, os.Getenv("TF_INPUT") == "false" || os.Getenv("TF_INPUT") == 0)
	opts.TerraformCliArgs = filterTerragruntArgs(args)
	opts.WorkingDir = filepath.ToSlash(workingDir)
	opts.Logger = util.CreateLoggerWithWriter(errWriter, "")
	opts.RunTerragrunt = runTerragrunt
	opts.Source = terraformSource
	opts.SourceUpdate = sourceUpdate
	opts.IgnoreDependencyErrors = ignoreDependencyErrors
	opts.Writer = writer
	opts.ErrWriter = errWriter
	opts.Env = parseEnvironmentVariables(os.Environ())
	opts.IamRole = iamRole

	return opts, nil
}

func filterTerraformExtraArgs(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []string {
	out := []string{}
	cmd := firstArg(terragruntOptions.TerraformCliArgs)

	for _, arg := range terragruntConfig.Terraform.ExtraArgs {
		for _, arg_cmd := range arg.Commands {
			if cmd == arg_cmd {
				out = append(out, arg.Arguments...)

				// If RequiredVarFiles is specified, add -var-file=<file> for each specified files
				for _, file := range util.RemoveDuplicatesFromListKeepLast(arg.RequiredVarFiles) {
					out = append(out, fmt.Sprintf("-var-file=%s", file))
				}

				// If OptionalVarFiles is specified, check for each file if it exists and if so, add -var-file=<file>
				// It is possible that many files resolve to the same path, so we remove duplicates.
				for _, file := range util.RemoveDuplicatesFromListKeepLast(arg.OptionalVarFiles) {
					if util.FileExists(file) {
						out = append(out, fmt.Sprintf("-var-file=%s", file))
					} else {
						terragruntOptions.Logger.Printf("Skipping var-file %s as it does not exist", file)
					}
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

// A convenience method that returns the first item (0th index) in the given list or an empty string if this is an
// empty list
func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// A convenience method that returns the second item (1st index) in the given list or an empty string if this is a
// list that has less than 2 items in it
func secondArg(args []string) string {
	if len(args) > 1 {
		return args[1]
	}
	return ""
}

// Custom error types

type ArgMissingValue string

func (err ArgMissingValue) Error() string {
	return fmt.Sprintf("You must specify a value for the --%s option", string(err))
}
