package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/hashicorp/hcl2/gohcl"
	"github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/hcl2/hclparse"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/util"
)

const DefaultTerragruntConfigPath = "terragrunt.hcl"

// TerragruntConfig represents a parsed and expanded configuration
type TerragruntConfig struct {
	Terraform                  *TerraformConfig
	TerraformBinary            string
	TerraformVersionConstraint string
	RemoteState                *remote.RemoteState
	Dependencies               *ModuleDependencies
	PreventDestroy             bool
	Skip                       bool
	IamRole                    string
	Inputs                     map[string]interface{}
	Locals                     map[string]interface{}
	TerragruntDependencies     []Dependency

	// Indicates whether or not this is the result of a partial evaluation
	IsPartial bool
}

func (conf *TerragruntConfig) String() string {
	return fmt.Sprintf("TerragruntConfig{Terraform = %v, RemoteState = %v, Dependencies = %v, PreventDestroy = %v}", conf.Terraform, conf.RemoteState, conf.Dependencies, conf.PreventDestroy)
}

// terragruntConfigFile represents the configuration supported in a Terragrunt configuration file (i.e.
// terragrunt.hcl)
type terragruntConfigFile struct {
	Terraform                  *TerraformConfig       `hcl:"terraform,block"`
	TerraformBinary            *string                `hcl:"terraform_binary,attr"`
	TerraformVersionConstraint *string                `hcl:"terraform_version_constraint,attr"`
	Inputs                     *cty.Value             `hcl:"inputs,attr"`
	Include                    *IncludeConfig         `hcl:"include,block"`
	RemoteState                *remoteStateConfigFile `hcl:"remote_state,block"`
	Dependencies               *ModuleDependencies    `hcl:"dependencies,block"`
	PreventDestroy             *bool                  `hcl:"prevent_destroy,attr"`
	Skip                       *bool                  `hcl:"skip,attr"`
	IamRole                    *string                `hcl:"iam_role,attr"`
	TerragruntDependencies     []Dependency           `hcl:"dependency,block"`

	// This struct is used for validating and parsing the entire terragrunt config. Since locals are evaluated in a
	// completely separate cycle, it should not be evaluated here. Otherwise, we can't support self referencing other
	// elements in the same block.
	Locals *terragruntLocal `hcl:"locals,block"`
}

// We use a struct designed to not parse the block, as locals are parsed and decoded using a special routine that allows
// references to the other locals in the same block.
type terragruntLocal struct {
	Remain hcl.Body `hcl:",remain"`
}

// Configuration for Terraform remote state as parsed from a terragrunt.hcl config file
type remoteStateConfigFile struct {
	Backend     string    `hcl:"backend,attr"`
	DisableInit *bool     `hcl:"disable_init,attr"`
	Config      cty.Value `hcl:"config,attr"`
}

func (remoteState *remoteStateConfigFile) String() string {
	return fmt.Sprintf("remoteStateConfigFile{Backend = %v, Config = %v}", remoteState.Backend, remoteState.Config)
}

// IncludeConfig represents the configuration settings for a parent Terragrunt configuration file that you can
// "include" in a child Terragrunt configuration file
type IncludeConfig struct {
	Path string `hcl:"path,attr"`
}

func (cfg *IncludeConfig) String() string {
	return fmt.Sprintf("IncludeConfig{Path = %s}", cfg.Path)
}

// ModuleDependencies represents the paths to other Terraform modules that must be applied before the current module
// can be applied
type ModuleDependencies struct {
	Paths []string `hcl:"paths,attr"`
}

// Merge appends the paths in the proided ModuleDependencies object into this ModuleDependencies object.
func (deps *ModuleDependencies) Merge(source *ModuleDependencies) {
	if source == nil {
		return
	}

	for _, path := range source.Paths {
		if !util.ListContainsElement(deps.Paths, path) {
			deps.Paths = append(deps.Paths, path)
		}
	}
}

func (deps *ModuleDependencies) String() string {
	return fmt.Sprintf("ModuleDependencies{Paths = %v}", deps.Paths)
}

// Hook specifies terraform commands (apply/plan) and array of os commands to execute
type Hook struct {
	Name       string   `hcl:"name,label"`
	Commands   []string `hcl:"commands,attr"`
	Execute    []string `hcl:"execute,attr"`
	RunOnError *bool    `hcl:"run_on_error,attr"`
}

func (conf *Hook) String() string {
	return fmt.Sprintf("Hook{Name = %s, Commands = %v}", conf.Name, len(conf.Commands))
}

// TerraformConfig specifies where to find the Terraform configuration files
type TerraformConfig struct {
	ExtraArgs   []TerraformExtraArguments `hcl:"extra_arguments,block"`
	Source      *string                   `hcl:"source,attr"`
	BeforeHooks []Hook                    `hcl:"before_hook,block"`
	AfterHooks  []Hook                    `hcl:"after_hook,block"`
}

func (conf *TerraformConfig) String() string {
	return fmt.Sprintf("TerraformConfig{Source = %v}", conf.Source)
}

func (conf *TerraformConfig) GetBeforeHooks() []Hook {
	if conf == nil {
		return nil
	}

	return conf.BeforeHooks
}

func (conf *TerraformConfig) GetAfterHooks() []Hook {
	if conf == nil {
		return nil
	}

	return conf.AfterHooks
}

func (conf *TerraformConfig) ValidateHooks() error {
	allHooks := append(conf.GetBeforeHooks(), conf.GetAfterHooks()...)

	for _, curHook := range allHooks {
		if len(curHook.Execute) < 1 || curHook.Execute[0] == "" {
			return InvalidArgError(fmt.Sprintf("Error with hook %s. Need at least one non-empty argument in 'execute'.", curHook.Name))
		}
	}

	return nil
}

// TerraformExtraArguments sets a list of arguments to pass to Terraform if command fits any in the `Commands` list
type TerraformExtraArguments struct {
	Name             string             `hcl:"name,label"`
	Arguments        *[]string          `hcl:"arguments,attr"`
	RequiredVarFiles *[]string          `hcl:"required_var_files,attr"`
	OptionalVarFiles *[]string          `hcl:"optional_var_files,attr"`
	Commands         []string           `hcl:"commands,attr"`
	EnvVars          *map[string]string `hcl:"env_vars,attr"`
}

func (conf *TerraformExtraArguments) String() string {
	return fmt.Sprintf(
		"TerraformArguments{Name = %s, Arguments = %v, Commands = %v, EnvVars = %v}",
		conf.Name,
		conf.Arguments,
		conf.Commands,
		conf.EnvVars)
}

// Return the default path to use for the Terragrunt configuration file in the given directory
func DefaultConfigPath(workingDir string) string {
	return util.JoinPath(workingDir, DefaultTerragruntConfigPath)
}

// Returns a list of all Terragrunt config files in the given path or any subfolder of the path. A file is a Terragrunt
// config file if it has a name as returned by the DefaultConfigPath method
func FindConfigFilesInPath(rootPath string, terragruntOptions *options.TerragruntOptions) ([]string, error) {
	configFiles := []string{}

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		isTerragruntModule, err := containsTerragruntModule(path, info, terragruntOptions)
		if err != nil {
			return err
		}

		if isTerragruntModule {
			configFiles = append(configFiles, DefaultConfigPath(path))
		}

		return nil
	})

	return configFiles, err
}

// Returns true if the given path with the given FileInfo contains a Terragrunt module and false otherwise. A path
// contains a Terragrunt module if it contains a Terragrunt configuration file (terragrunt.hcl) and is not a cache
// or download dir.
func containsTerragruntModule(path string, info os.FileInfo, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if !info.IsDir() {
		return false, nil
	}

	// Skip the Terragrunt cache dir
	if strings.Contains(path, options.TerragruntCacheDir) {
		return false, nil
	}

	canonicalPath, err := util.CanonicalPath(path, "")
	if err != nil {
		return false, err
	}

	canonicalDownloadPath, err := util.CanonicalPath(terragruntOptions.DownloadDir, "")
	if err != nil {
		return false, err
	}

	// Skip any custom download dir specified by the user
	if strings.Contains(canonicalPath, canonicalDownloadPath) {
		return false, err
	}

	return util.FileExists(DefaultConfigPath(path)), nil
}

// Read the Terragrunt config file from its default location
func ReadTerragruntConfig(terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	terragruntOptions.Logger.Printf("Reading Terragrunt config file at %s", terragruntOptions.TerragruntConfigPath)
	return ParseConfigFile(terragruntOptions.TerragruntConfigPath, terragruntOptions, nil)
}

// Parse the Terragrunt config file at the given path. If the include parameter is not nil, then treat this as a config
// included in some other config file when resolving relative paths.
func ParseConfigFile(filename string, terragruntOptions *options.TerragruntOptions, include *IncludeConfig) (*TerragruntConfig, error) {
	configString, err := util.ReadFileAsString(filename)
	if err != nil {
		return nil, err
	}

	config, err := ParseConfigString(configString, terragruntOptions, include, filename)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// Parse the Terragrunt config contained in the given string and merge it with the given include config (if any). Note
// that the config parsing consists of multiple stages so as to allow referencing of data resulting from parsing
// previous config. The parsing order is:
// 1. Parse locals. Since locals are parsed first, you can only reference other locals in the locals block and it is not
//    merged from a config imported with an include block.
//    Allowed References:
//      - locals
// 2. Parse include. Include is parsed next and is used to import another config. All the config in the include block is
//    then merged into the current TerragruntConfig.
//    Allowed References:
//      - locals
// 3. Parse dependency blocks. This includes running `terragrunt output` to fetch the output data from another
//    terragrunt config, so that it is accessible within the config. See PartialParseConfigString for a way to parse the
//    blocks but avoid decoding.
//    Allowed References:
//      - locals
// 4. Parse everything else. At this point, all the necessary building blocks for parsing the rest of the config are
//    available, so parse the rest of the config.
//    Allowed References:
//      - locals
//      - dependency
// 5. Merge the included config with the parsed config. Note that all the config data is mergable except for `locals`
//    blocks, which are only scoped to be available within the defining config.
func ParseConfigString(configString string, terragruntOptions *options.TerragruntOptions, includeFromChild *IncludeConfig, filename string) (*TerragruntConfig, error) {
	// Parse the HCL string into an AST body that can be decoded multiple times later without having to re-parse
	parser := hclparse.NewParser()
	file, err := parseHcl(parser, configString, filename)
	if err != nil {
		return nil, err
	}

	// Decode just the Base blocks. See the function docs for DecodeBaseBlocks for more info on what base blocks are.
	localsAsCty, terragruntInclude, includeForDecode, err := DecodeBaseBlocks(terragruntOptions, parser, file, filename, includeFromChild)
	if err != nil {
		return nil, err
	}

	// Initialize evaluation context extensions from base blocks.
	contextExtensions := EvalContextExtensions{
		Locals:  localsAsCty,
		Include: includeForDecode,
	}

	// Decode just the `dependency` blocks, retrieving the outputs from the target terragrunt config in the
	// process.
	retrievedOutputs, err := decodeAndRetrieveOutputs(file, filename, terragruntOptions, contextExtensions)
	if err != nil {
		return nil, err
	}
	contextExtensions.DecodedDependencies = retrievedOutputs

	// Decode the rest of the config, passing in this config's `include` block or the child's `include` block, whichever
	// is appropriate
	terragruntConfigFile, err := decodeAsTerragruntConfigFile(file, filename, terragruntOptions, contextExtensions)
	if err != nil {
		return nil, err
	}
	if terragruntConfigFile == nil {
		return nil, errors.WithStackTrace(CouldNotResolveTerragruntConfigInFile(filename))
	}

	config, err := convertToTerragruntConfig(terragruntConfigFile, filename, terragruntOptions)
	if err != nil {
		return nil, err
	}

	// If this file includes another, parse and merge it.  Otherwise just return this config.
	if terragruntInclude.Include != nil {
		includedConfig, err := parseIncludedConfig(terragruntInclude.Include, terragruntOptions)
		if err != nil {
			return nil, err
		}
		return mergeConfigWithIncludedConfig(config, includedConfig, terragruntOptions)
	} else {
		return config, nil
	}
}

func getIncludedConfigForDecode(
	parsedTerragruntInclude *terragruntInclude,
	terragruntOptions *options.TerragruntOptions,
	includeFromChild *IncludeConfig,
) (*IncludeConfig, error) {
	if parsedTerragruntInclude.Include != nil && includeFromChild != nil {
		return nil, errors.WithStackTrace(TooManyLevelsOfInheritance{
			ConfigPath:             terragruntOptions.TerragruntConfigPath,
			FirstLevelIncludePath:  includeFromChild.Path,
			SecondLevelIncludePath: parsedTerragruntInclude.Include.Path,
		})
	} else if parsedTerragruntInclude.Include != nil {
		return parsedTerragruntInclude.Include, nil
	} else if includeFromChild != nil {
		return includeFromChild, nil
	}
	return nil, nil
}

func decodeAsTerragruntConfigFile(
	file *hcl.File,
	filename string,
	terragruntOptions *options.TerragruntOptions,
	extensions EvalContextExtensions,
) (*terragruntConfigFile, error) {
	terragruntConfig := terragruntConfigFile{}
	err := decodeHcl(file, filename, &terragruntConfig, terragruntOptions, extensions)
	if err != nil {
		return nil, err
	}
	return &terragruntConfig, nil
}

// decodeHcl uses the HCL2 parser to decode the parsed HCL into the struct specified by out.
func decodeHcl(
	file *hcl.File,
	filename string,
	out interface{},
	terragruntOptions *options.TerragruntOptions,
	extensions EvalContextExtensions,
) (err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfig{RecoveredValue: recovered, ConfigFile: filename})
		}
	}()

	evalContext := CreateTerragruntEvalContext(filename, terragruntOptions, extensions)

	decodeDiagnostics := gohcl.DecodeBody(file.Body, evalContext, out)
	if decodeDiagnostics != nil && decodeDiagnostics.HasErrors() {
		return decodeDiagnostics
	}

	return nil
}

// parseHcl uses the HCL2 parser to parse the given string into an HCL file body.
func parseHcl(parser *hclparse.Parser, hcl string, filename string) (file *hcl.File, err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfig{RecoveredValue: recovered, ConfigFile: filename})
		}
	}()

	file, parseDiagnostics := parser.ParseHCL([]byte(hcl), filename)
	if parseDiagnostics != nil && parseDiagnostics.HasErrors() {
		return nil, parseDiagnostics
	}

	return file, nil
}

// Merge the given config with an included config. Anything specified in the current config will override the contents
// of the included config. If the included config is nil, just return the current config.
func mergeConfigWithIncludedConfig(config *TerragruntConfig, includedConfig *TerragruntConfig, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	if config.RemoteState != nil {
		includedConfig.RemoteState = config.RemoteState
	}
	if config.PreventDestroy {
		includedConfig.PreventDestroy = config.PreventDestroy
	}

	// Skip has to be set specifically in each file that should be skipped
	includedConfig.Skip = config.Skip

	if config.Terraform != nil {
		if includedConfig.Terraform == nil {
			includedConfig.Terraform = config.Terraform
		} else {
			if config.Terraform.Source != nil {
				includedConfig.Terraform.Source = config.Terraform.Source
			}
			mergeExtraArgs(terragruntOptions, config.Terraform.ExtraArgs, &includedConfig.Terraform.ExtraArgs)

			mergeHooks(terragruntOptions, config.Terraform.BeforeHooks, &includedConfig.Terraform.BeforeHooks)
			mergeHooks(terragruntOptions, config.Terraform.AfterHooks, &includedConfig.Terraform.AfterHooks)
		}
	}

	if config.Dependencies != nil {
		includedConfig.Dependencies = config.Dependencies
	}

	if config.IamRole != "" {
		includedConfig.IamRole = config.IamRole
	}

	if config.TerraformVersionConstraint != "" {
		includedConfig.TerraformVersionConstraint = config.TerraformVersionConstraint
	}

	if config.TerraformBinary != "" {
		includedConfig.TerraformBinary = config.TerraformBinary
	}

	if config.Inputs != nil {
		includedConfig.Inputs = mergeInputs(config.Inputs, includedConfig.Inputs)
	}

	return includedConfig, nil
}

// Merge the hooks (before_hook and after_hook).
//
// If a child's hook (before_hook or after_hook) has the same name a parent's hook,
// then the child's hook will be selected (and the parent's ignored)
// If a child's hook has a different name from all of the parent's hooks,
// then the child's hook will be added to the end of the parent's.
// Therefore, the child with the same name overrides the parent
func mergeHooks(terragruntOptions *options.TerragruntOptions, childHooks []Hook, parentHooks *[]Hook) {
	result := *parentHooks
	for _, child := range childHooks {
		parentHookWithSameName := getIndexOfHookWithName(result, child.Name)
		if parentHookWithSameName != -1 {
			// If the parent contains a hook with the same name as the child,
			// then override the parent's hook with the child's.
			terragruntOptions.Logger.Printf("hook '%v' from child overriding parent", child.Name)
			result[parentHookWithSameName] = child
		} else {
			// If the parent does not contain a hook with the same name as the child
			// then add the child to the end.
			result = append(result, child)
		}
	}
	*parentHooks = result
}

// Returns the index of the Hook with the given name,
// or -1 if no Hook have the given name.
func getIndexOfHookWithName(hooks []Hook, name string) int {
	for i, hook := range hooks {
		if hook.Name == name {
			return i
		}
	}
	return -1
}

// Merge the extra arguments.
//
// If a child's extra_arguments has the same name a parent's extra_arguments,
// then the child's extra_arguments will be selected (and the parent's ignored)
// If a child's extra_arguments has a different name from all of the parent's extra_arguments,
// then the child's extra_arguments will be added to the end  of the parents.
// Therefore, terragrunt will put the child extra_arguments after the parent's
// extra_arguments on the terraform cli.
// Therefore, if .tfvar files from both the parent and child contain a variable
// with the same name, the value from the child will win.
func mergeExtraArgs(terragruntOptions *options.TerragruntOptions, childExtraArgs []TerraformExtraArguments, parentExtraArgs *[]TerraformExtraArguments) {
	result := *parentExtraArgs
	for _, child := range childExtraArgs {
		parentExtraArgsWithSameName := getIndexOfExtraArgsWithName(result, child.Name)
		if parentExtraArgsWithSameName != -1 {
			// If the parent contains an extra_arguments with the same name as the child,
			// then override the parent's extra_arguments with the child's.
			terragruntOptions.Logger.Printf("extra_arguments '%v' from child overriding parent", child.Name)
			result[parentExtraArgsWithSameName] = child
		} else {
			// If the parent does not contain an extra_arguments with the same name as the child
			// then add the child to the end.
			// This ensures the child extra_arguments are added to the command line after the parent extra_arguments.
			result = append(result, child)
		}
	}
	*parentExtraArgs = result
}

// Returns the index of the extraArgs with the given name,
// or -1 if no extraArgs have the given name.
func getIndexOfExtraArgsWithName(extraArgs []TerraformExtraArguments, name string) int {
	for i, extra := range extraArgs {
		if extra.Name == name {
			return i
		}
	}
	return -1
}

// Parse the config of the given include, if one is specified
func parseIncludedConfig(includedConfig *IncludeConfig, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	if includedConfig.Path == "" {
		return nil, errors.WithStackTrace(IncludedConfigMissingPath(terragruntOptions.TerragruntConfigPath))
	}

	includePath := includedConfig.Path

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(filepath.Dir(terragruntOptions.TerragruntConfigPath), includePath)
	}

	return ParseConfigFile(includePath, terragruntOptions, includedConfig)
}

func mergeInputs(childInputs map[string]interface{}, parentInputs map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}

	for key, value := range parentInputs {
		out[key] = value
	}

	for key, value := range childInputs {
		out[key] = value
	}

	return out
}

// Convert the contents of a fully resolved Terragrunt configuration to a TerragruntConfig object
func convertToTerragruntConfig(terragruntConfigFromFile *terragruntConfigFile, configPath string, terragruntOptions *options.TerragruntOptions) (cfg *TerragruntConfig, err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfig{RecoveredValue: recovered, ConfigFile: configPath})
		}
	}()

	terragruntConfig := &TerragruntConfig{IsPartial: false}

	if terragruntConfigFromFile.RemoteState != nil {
		remoteStateConfig, err := parseCtyValueToMap(terragruntConfigFromFile.RemoteState.Config)
		if err != nil {
			return nil, err
		}

		remoteState := &remote.RemoteState{}
		remoteState.Backend = terragruntConfigFromFile.RemoteState.Backend
		remoteState.Config = remoteStateConfig

		if terragruntConfigFromFile.RemoteState.DisableInit != nil {
			remoteState.DisableInit = *terragruntConfigFromFile.RemoteState.DisableInit
		}

		remoteState.FillDefaults()
		if err := remoteState.Validate(); err != nil {
			return nil, err
		}

		terragruntConfig.RemoteState = remoteState
	}

	if err := terragruntConfigFromFile.Terraform.ValidateHooks(); err != nil {
		return nil, err
	}

	terragruntConfig.Terraform = terragruntConfigFromFile.Terraform
	terragruntConfig.Dependencies = terragruntConfigFromFile.Dependencies
	terragruntConfig.TerragruntDependencies = terragruntConfigFromFile.TerragruntDependencies

	if terragruntConfigFromFile.TerraformBinary != nil {
		terragruntConfig.TerraformBinary = *terragruntConfigFromFile.TerraformBinary
	}
	if terragruntConfigFromFile.TerraformVersionConstraint != nil {
		terragruntConfig.TerraformVersionConstraint = *terragruntConfigFromFile.TerraformVersionConstraint
	}

	if terragruntConfigFromFile.PreventDestroy != nil {
		terragruntConfig.PreventDestroy = *terragruntConfigFromFile.PreventDestroy
	}

	if terragruntConfigFromFile.Skip != nil {
		terragruntConfig.Skip = *terragruntConfigFromFile.Skip
	}

	if terragruntConfigFromFile.IamRole != nil {
		terragruntConfig.IamRole = *terragruntConfigFromFile.IamRole
	}

	if terragruntConfigFromFile.Inputs != nil {
		inputs, err := parseCtyValueToMap(*terragruntConfigFromFile.Inputs)
		if err != nil {
			return nil, err
		}

		terragruntConfig.Inputs = inputs
	}

	return terragruntConfig, nil
}

// Custom error types

type InvalidArgError string

func (e InvalidArgError) Error() string {
	return string(e)
}

type IncludedConfigMissingPath string

func (err IncludedConfigMissingPath) Error() string {
	return fmt.Sprintf("The include configuration in %s must specify a 'path' parameter", string(err))
}

type TooManyLevelsOfInheritance struct {
	ConfigPath             string
	FirstLevelIncludePath  string
	SecondLevelIncludePath string
}

func (err TooManyLevelsOfInheritance) Error() string {
	return fmt.Sprintf("%s includes %s, which itself includes %s. Only one level of includes is allowed.", err.ConfigPath, err.FirstLevelIncludePath, err.SecondLevelIncludePath)
}

type CouldNotResolveTerragruntConfigInFile string

func (err CouldNotResolveTerragruntConfigInFile) Error() string {
	return fmt.Sprintf("Could not find Terragrunt configuration settings in %s", string(err))
}

type ErrorParsingTerragruntConfig struct {
	ConfigPath string
	Underlying error
}

func (err ErrorParsingTerragruntConfig) Error() string {
	return fmt.Sprintf("Error parsing Terragrunt config at %s: %v", err.ConfigPath, err.Underlying)
}

type PanicWhileParsingConfig struct {
	ConfigFile     string
	RecoveredValue interface{}
}

func (err PanicWhileParsingConfig) Error() string {
	return fmt.Sprintf("Recovering panic while parsing '%s'. Got error of type '%v': %v", err.ConfigFile, reflect.TypeOf(err.RecoveredValue), err.RecoveredValue)
}

type InvalidBackendConfigType struct {
	ExpectedType string
	ActualType   string
}

func (err InvalidBackendConfigType) Error() string {
	return fmt.Sprintf("Expected backend config to be of type '%s' but got '%s'.", err.ExpectedType, err.ActualType)
}
