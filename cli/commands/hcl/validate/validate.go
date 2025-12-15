// Package validate-inputs collects all the terraform variables defined in the target module, and the terragrunt
// inputs that are configured, and compare the two to determine if there are any unused inputs or undefined required
// inputs.
package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"

	"github.com/google/shlex"
	"github.com/hashicorp/hcl/v2"
	"golang.org/x/exp/slices"

	"maps"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/view"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

const splitCount = 2

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if opts.HCLValidateInputs {
		if opts.HCLValidateShowConfigPath {
			return errors.Errorf("specifying both -%s and -%s is invalid", ShowConfigPathFlagName, InputsFlagName)
		}

		if opts.HCLValidateJSONOutput {
			return errors.Errorf("specifying both -%s and -%s is invalid", JSONFlagName, InputsFlagName)
		}

		return RunValidateInputs(ctx, l, opts)
	}

	if opts.HCLValidateStrict {
		return errors.Errorf("specifying -%s without -%s is invalid", StrictFlagName, InputsFlagName)
	}

	return RunValidate(ctx, l, opts)
}

func RunValidate(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	var diags diagnostic.Diagnostics

	// Diagnostics handler to collect validation errors
	diagnosticsHandler := hclparse.WithDiagnosticsHandler(func(file *hcl.File, hclDiags hcl.Diagnostics) (hcl.Diagnostics, error) {
		for _, hclDiag := range hclDiags {
			// Only report diagnostics that are actually in the file being parsed,
			// not errors from dependencies or other files
			if hclDiag.Subject != nil && file != nil {
				fileFilename := file.Body.MissingItemRange().Filename

				diagFilename := hclDiag.Subject.Filename
				if diagFilename != fileFilename {
					continue
				}
			}

			newDiag := diagnostic.NewDiagnostic(file, hclDiag)
			if !diags.Contains(newDiag) {
				diags = append(diags, newDiag)
			}
		}

		return nil, nil
	})

	opts.SkipOutput = true
	opts.NonInteractive = true

	// Create discovery with filter support if experiment enabled
	d, err := discovery.NewForHCLCommand(discovery.HCLCommandOptions{
		WorkingDir:    opts.WorkingDir,
		FilterQueries: opts.FilterQueries,
		Experiments:   opts.Experiments,
	})
	if err != nil {
		return processDiagnostics(l, opts, diags, errors.New(err))
	}

	if opts.Experiments.Evaluate(experiment.FilterFlag) {
		// We do worktree generation here instead of in the discovery constructor
		// so that we can defer cleanup in the same context.
		filters, parseErr := filter.ParseFilterQueries(opts.FilterQueries)
		if parseErr != nil {
			return fmt.Errorf("failed to parse filters: %w", parseErr)
		}

		gitFilters := filters.UniqueGitFilters()

		worktrees, parseErr := worktrees.NewWorktrees(ctx, l, opts.WorkingDir, gitFilters)
		if parseErr != nil {
			return errors.Errorf("failed to create worktrees: %w", parseErr)
		}

		defer func() {
			cleanupErr := worktrees.Cleanup(ctx, l)
			if cleanupErr != nil {
				l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
			}
		}()

		d = d.WithWorktrees(worktrees)
	}

	components, err := d.Discover(ctx, l, opts)
	if err != nil {
		return processDiagnostics(l, opts, diags, errors.New(err))
	}

	parseOptions := []hclparse.Option{diagnosticsHandler}

	parseErrs := []error{}

	for _, c := range components {
		parseOpts := opts.Clone()
		parseOpts.WorkingDir = c.Path()

		if _, ok := c.(*component.Stack); ok {
			stackFilePath := filepath.Join(c.Path(), config.DefaultStackFile)
			parseOpts.TerragruntConfigPath = stackFilePath

			values, err := config.ReadValues(ctx, l, parseOpts, c.Path())
			if err != nil {
				parseErrs = append(parseErrs, errors.New(err))
			}

			parser := config.NewParsingContext(ctx, l, parseOpts).WithParseOption(parseOptions)
			if values != nil {
				parser = parser.WithValues(values)
			}

			file, err := hclparse.NewParser(parser.ParserOptions...).ParseFromFile(stackFilePath)
			if err != nil {
				parseErrs = append(parseErrs, errors.New(err))
				continue
			}

			//nolint:contextcheck
			if _, err := config.ParseStackConfig(l, parser, parseOpts, file, values); err != nil {
				parseErrs = append(parseErrs, errors.New(err))
			}

			continue
		}

		// Determine which config filename to use for a full parse
		configFilename := config.DefaultTerragruntConfigPath
		if len(opts.TerragruntConfigPath) > 0 {
			configFilename = filepath.Base(opts.TerragruntConfigPath)
		}

		parseOpts.TerragruntConfigPath = filepath.Join(c.Path(), configFilename)

		if _, err := config.ReadTerragruntConfig(ctx, l, parseOpts, parseOptions); err != nil {
			parseErrs = append(parseErrs, errors.New(err))
		}
	}

	var combinedErr error
	if len(parseErrs) > 0 {
		combinedErr = errors.Join(parseErrs...)
	}

	return processDiagnostics(l, opts, diags, combinedErr)
}

func processDiagnostics(l log.Logger, opts *options.TerragruntOptions, diags diagnostic.Diagnostics, callErr error) error {
	if len(diags) == 0 {
		return callErr
	}

	sort.Slice(diags, func(i, j int) bool {
		var a, b string

		if diags[i].Range != nil {
			a = diags[i].Range.Filename
		}

		if diags[j].Range != nil {
			b = diags[j].Range.Filename
		}

		return a < b
	})

	if err := writeDiagnostics(l, opts, diags); err != nil {
		return err
	}

	diagError := errors.Errorf("%d HCL validation error(s) found", len(diags))

	// If diagnostics exist and no other error was returned,
	// return a synthetic error to mark validation as failed and
	// ensure a non-zero exit code from Terragrunt.
	if callErr == nil {
		return diagError
	}

	return errors.Join(callErr, diagError)
}

func writeDiagnostics(l log.Logger, opts *options.TerragruntOptions, diags diagnostic.Diagnostics) error {
	render := view.NewHumanRender(l.Formatter().DisabledColors())
	if opts.HCLValidateJSONOutput {
		render = view.NewJSONRender()
	}

	writer := view.NewWriter(opts.Writer, render)

	if opts.HCLValidateShowConfigPath {
		return writer.ShowConfigPath(diags)
	}

	return writer.Diagnostics(diags)
}

func RunValidateInputs(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	target := run.NewTarget(run.TargetPointGenerateConfig, runValidateInputs)

	return run.RunWithTarget(ctx, l, opts, report.NewReport(), target)
}

func runValidateInputs(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
	required, optional, err := tf.ModuleVariables(opts.WorkingDir)
	if err != nil {
		return err
	}

	allVars := append(required, optional...)

	allInputs, err := getDefinedTerragruntInputs(l, opts, cfg)
	if err != nil {
		return err
	}

	// Unused variables are those that are passed in by terragrunt, but are not defined in terraform.
	unusedVars := []string{}

	for _, varName := range allInputs {
		if !slices.Contains(allVars, varName) {
			unusedVars = append(unusedVars, varName)
		}
	}

	// Missing variables are those that are required by the terraform config, but not defined in terragrunt.
	missingVars := []string{}

	for _, varName := range required {
		if !slices.Contains(allInputs, varName) {
			missingVars = append(missingVars, varName)
		}
	}

	// Now print out all the information
	if len(unusedVars) > 0 {
		l.Warn("The following inputs passed in by terragrunt are unused:\n")

		for _, varName := range unusedVars {
			l.Warnf("\t- %s", varName)
		}

		l.Warn("")
	} else {
		l.Info("All variables passed in by terragrunt are in use.")
		l.Debug(fmt.Sprintf("Strict mode enabled: %t", opts.HCLValidateStrict))
	}

	if len(missingVars) > 0 {
		l.Error("The following required inputs are missing:\n")

		for _, varName := range missingVars {
			l.Errorf("\t- %s", varName)
		}

		l.Error("")
	} else {
		l.Info("All required inputs are passed in by terragrunt")
		l.Debug(fmt.Sprintf("Strict mode enabled: %t", opts.HCLValidateStrict))
	}

	// Return an error when there are misaligned inputs. Terragrunt strict mode defaults to false. When it is false,
	// an error will only be returned if required inputs are missing. When strict mode is true, an error will be
	// returned if required inputs are missing OR if any unused variables are passed
	if len(missingVars) > 0 || len(unusedVars) > 0 && opts.HCLValidateStrict {
		return errors.New("terragrunt configuration has inputs that are not defined in the OpenTofu/Terraform module. This is not allowed when strict mode is enabled")
	} else if len(unusedVars) > 0 {
		l.Warn("Terragrunt configuration has misaligned inputs, but running in relaxed mode so ignoring.")
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
func getDefinedTerragruntInputs(l log.Logger, opts *options.TerragruntOptions, cfg *config.TerragruntConfig) ([]string, error) {
	envVarTFVars := getTerraformInputNamesFromEnvVar(opts, cfg)
	inputsTFVars := getTerraformInputNamesFromConfig(cfg)

	varFileTFVars, err := getTerraformInputNamesFromVarFiles(l, opts, cfg)
	if err != nil {
		return nil, err
	}

	cliArgsTFVars, err := getTerraformInputNamesFromCLIArgs(l, opts, cfg)
	if err != nil {
		return nil, err
	}

	autoVarFileTFVars, err := getTerraformInputNamesFromAutomaticVarFiles(l, opts)
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
func getTerraformInputNamesFromEnvVar(opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []string {
	envVars := opts.Env

	// Make sure to check if there are configured env vars in the parsed terragrunt config.
	if terragruntConfig.Terraform != nil {
		for _, arg := range terragruntConfig.Terraform.ExtraArgs {
			if arg.EnvVars != nil {
				maps.Copy(envVars, *arg.EnvVars)
			}
		}
	}

	var (
		out         = []string{}
		tfVarPrefix = fmt.Sprintf(tf.EnvNameTFVarFmt, "")
	)

	for envName := range envVars {
		if after, ok := strings.CutPrefix(envName, tfVarPrefix); ok {
			inputName := after
			out = append(out, inputName)
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
func getTerraformInputNamesFromVarFiles(l log.Logger, opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) ([]string, error) {
	if terragruntConfig.Terraform == nil {
		return nil, nil
	}

	varFiles := []string{}
	for _, arg := range terragruntConfig.Terraform.ExtraArgs {
		varFiles = append(varFiles, arg.GetVarFiles(l)...)
	}

	return getVarNamesFromVarFiles(l, opts, varFiles)
}

// getTerraformInputNamesFromCLIArgs will return the list of names of variables configured by -var and -var-file CLI
// args that are passed in via the configured arguments attribute in the extra_arguments block of the given terragrunt
// config and those that are directly passed in via the CLI.
func getTerraformInputNamesFromCLIArgs(l log.Logger, opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) ([]string, error) {
	inputNames, varFiles, err := GetVarFlagsFromArgList(opts.TerraformCliArgs)
	if err != nil {
		return inputNames, err
	}

	if terragruntConfig.Terraform != nil {
		for _, arg := range terragruntConfig.Terraform.ExtraArgs {
			if arg.Arguments != nil {
				vars, rawVarFiles, getArgsErr := GetVarFlagsFromArgList(*arg.Arguments)
				if getArgsErr != nil {
					return inputNames, getArgsErr
				}

				inputNames = append(inputNames, vars...)
				varFiles = append(varFiles, rawVarFiles...)
			}
		}
	}

	fileVars, err := getVarNamesFromVarFiles(l, opts, varFiles)
	if err != nil {
		return inputNames, err
	}

	inputNames = append(inputNames, fileVars...)

	return inputNames, nil
}

// getTerraformInputNamesFromAutomaticVarFiles returns all the variables names
func getTerraformInputNamesFromAutomaticVarFiles(l log.Logger, opts *options.TerragruntOptions) ([]string, error) {
	base := opts.WorkingDir
	automaticVarFiles := []string{}

	tfTFVarsFile := filepath.Join(base, "terraform.tfvars")
	if util.FileExists(tfTFVarsFile) {
		automaticVarFiles = append(automaticVarFiles, tfTFVarsFile)
	}

	tfTFVarsJSONFile := filepath.Join(base, "terraform.tfvars.json")
	if util.FileExists(tfTFVarsJSONFile) {
		automaticVarFiles = append(automaticVarFiles, tfTFVarsJSONFile)
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

	return getVarNamesFromVarFiles(l, opts, automaticVarFiles)
}

// getVarNamesFromVarFiles will parse all the given var files and returns a list of names of variables that are
// configured in all of them combined together.
func getVarNamesFromVarFiles(l log.Logger, opts *options.TerragruntOptions, varFiles []string) ([]string, error) {
	inputNames := []string{}

	for _, varFile := range varFiles {
		fileVars, err := getVarNamesFromVarFile(l, opts, varFile)
		if err != nil {
			return inputNames, err
		}

		inputNames = append(inputNames, fileVars...)
	}

	return inputNames, nil
}

// getVarNamesFromVarFile will parse the given terraform var file and return a list of names of variables that are
// configured in that var file.
func getVarNamesFromVarFile(l log.Logger, opts *options.TerragruntOptions, varFile string) ([]string, error) {
	fileContents, err := os.ReadFile(varFile)
	if err != nil {
		return nil, err
	}

	var variables map[string]any
	if strings.HasSuffix(varFile, "json") {
		if err := json.Unmarshal(fileContents, &variables); err != nil {
			return nil, err
		}
	} else {
		if err := config.ParseAndDecodeVarFile(l, opts, varFile, fileContents, &variables); err != nil {
			return nil, err
		}
	}

	out := []string{}
	for varName := range variables {
		out = append(out, varName)
	}

	return out, nil
}

// GetVarFlagsFromArgList returns the CLI flags defined on the provided arguments list that correspond to -var and -var-file.
// Returns two slices, one for `-var` args (the first one) and one for `-var-file` args (the second one).
func GetVarFlagsFromArgList(argList []string) ([]string, []string, error) {
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
			if len(splitArg) < splitCount {
				return vars, varFiles, fmt.Errorf("unexpected -var arg format in terraform.extra_arguments.arguments. Expected '-var=VARNAME=VALUE', got %s", arg)
			}

			vars = append(vars, splitArg[1])
		}

		if after, ok := strings.CutPrefix(shlexedArg, "-var-file="); ok {
			varFiles = append(varFiles, after)
		}
	}

	return vars, varFiles, nil
}
