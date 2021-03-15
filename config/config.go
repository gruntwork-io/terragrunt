package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/sirupsen/logrus"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/util"
)

const DefaultTerragruntConfigPath = "terragrunt.hcl"
const DefaultTerragruntJsonConfigPath = "terragrunt.hcl.json"

// TerragruntConfig represents a parsed and expanded configuration
// NOTE: if any attributes are added, make sure to update terragruntConfigAsCty in config_as_cty.go
type TerragruntConfig struct {
	Terraform                   *TerraformConfig
	TerraformBinary             string
	TerraformVersionConstraint  string
	TerragruntVersionConstraint string
	RemoteState                 *remote.RemoteState
	Dependencies                *ModuleDependencies
	DownloadDir                 string
	PreventDestroy              *bool
	Skip                        bool
	IamRole                     string
	Inputs                      map[string]interface{}
	Locals                      map[string]interface{}
	TerragruntDependencies      []Dependency
	GenerateConfigs             map[string]codegen.GenerateConfig
	RetryableErrors             []string

	// Indicates whether or not this is the result of a partial evaluation
	IsPartial bool
}

func (conf *TerragruntConfig) String() string {
	return fmt.Sprintf("TerragruntConfig{Terraform = %v, RemoteState = %v, Dependencies = %v, PreventDestroy = %v}", conf.Terraform, conf.RemoteState, conf.Dependencies, conf.PreventDestroy)
}

// terragruntConfigFile represents the configuration supported in a Terragrunt configuration file (i.e.
// terragrunt.hcl)
type terragruntConfigFile struct {
	Terraform                   *TerraformConfig          `hcl:"terraform,block"`
	TerraformBinary             *string                   `hcl:"terraform_binary,attr"`
	TerraformVersionConstraint  *string                   `hcl:"terraform_version_constraint,attr"`
	TerragruntVersionConstraint *string                   `hcl:"terragrunt_version_constraint,attr"`
	Inputs                      *cty.Value                `hcl:"inputs,attr"`
	Include                     *IncludeConfig            `hcl:"include,block"`
	RemoteState                 *remoteStateConfigFile    `hcl:"remote_state,block"`
	Dependencies                *ModuleDependencies       `hcl:"dependencies,block"`
	DownloadDir                 *string                   `hcl:"download_dir,attr"`
	PreventDestroy              *bool                     `hcl:"prevent_destroy,attr"`
	Skip                        *bool                     `hcl:"skip,attr"`
	IamRole                     *string                   `hcl:"iam_role,attr"`
	TerragruntDependencies      []Dependency              `hcl:"dependency,block"`
	GenerateBlocks              []terragruntGenerateBlock `hcl:"generate,block"`
	RetryableErrors             []string                  `hcl:"retryable_errors,optional"`

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
	Backend                       string                     `hcl:"backend,attr"`
	DisableInit                   *bool                      `hcl:"disable_init,attr"`
	DisableDependencyOptimization *bool                      `hcl:"disable_dependency_optimization,attr"`
	Generate                      *remoteStateConfigGenerate `hcl:"generate,attr"`
	Config                        cty.Value                  `hcl:"config,attr"`
}

func (remoteState *remoteStateConfigFile) String() string {
	return fmt.Sprintf("remoteStateConfigFile{Backend = %v, Config = %v}", remoteState.Backend, remoteState.Config)
}

// Convert the parsed config file remote state struct to the internal representation struct of remote state
// configurations.
func (remoteState *remoteStateConfigFile) toConfig() (*remote.RemoteState, error) {
	remoteStateConfig, err := parseCtyValueToMap(remoteState.Config)
	if err != nil {
		return nil, err
	}

	config := &remote.RemoteState{}
	config.Backend = remoteState.Backend
	if remoteState.Generate != nil {
		config.Generate = &remote.RemoteStateGenerate{
			Path:     remoteState.Generate.Path,
			IfExists: remoteState.Generate.IfExists,
		}
	}
	config.Config = remoteStateConfig

	if remoteState.DisableInit != nil {
		config.DisableInit = *remoteState.DisableInit
	}
	if remoteState.DisableDependencyOptimization != nil {
		config.DisableDependencyOptimization = *remoteState.DisableDependencyOptimization
	}

	config.FillDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, err
}

type remoteStateConfigGenerate struct {
	// We use cty instead of hcl, since we are using this type to convert an attr and not a block.
	Path     string `cty:"path"`
	IfExists string `cty:"if_exists"`
}

// Struct used to parse generate blocks. This will later be converted to GenerateConfig structs so that we can go
// through the codegen routine.
type terragruntGenerateBlock struct {
	Name             string  `hcl:",label"`
	Path             string  `hcl:"path,attr"`
	IfExists         string  `hcl:"if_exists,attr"`
	CommentPrefix    *string `hcl:"comment_prefix,attr"`
	Contents         string  `hcl:"contents,attr"`
	DisableSignature *bool   `hcl:"disable_signature,attr"`
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
	Paths []string `hcl:"paths,attr" cty:"paths"`
}

// Merge appends the paths in the provided ModuleDependencies object into this ModuleDependencies object.
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
	Name       string   `hcl:"name,label" cty:"name"`
	Commands   []string `hcl:"commands,attr" cty:"commands"`
	Execute    []string `hcl:"execute,attr" cty:"execute"`
	RunOnError *bool    `hcl:"run_on_error,attr" cty:"run_on_error"`
	WorkingDir *string  `hcl:"working_dir,attr" cty:"working_dir"`
}

func (conf *Hook) String() string {
	return fmt.Sprintf("Hook{Name = %s, Commands = %v}", conf.Name, len(conf.Commands))
}

// TerraformConfig specifies where to find the Terraform configuration files
// NOTE: If any attributes or blocks are added here, be sure to add it to ctyTerraformConfig in config_as_cty.go as
// well.
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
	Name             string             `hcl:"name,label" cty:"name"`
	Arguments        *[]string          `hcl:"arguments,attr" cty:"arguments"`
	RequiredVarFiles *[]string          `hcl:"required_var_files,attr" cty:"required_var_files"`
	OptionalVarFiles *[]string          `hcl:"optional_var_files,attr" cty:"optional_var_files"`
	Commands         []string           `hcl:"commands,attr" cty:"commands"`
	EnvVars          *map[string]string `hcl:"env_vars,attr" cty:"env_vars"`
}

func (conf *TerraformExtraArguments) String() string {
	return fmt.Sprintf(
		"TerraformArguments{Name = %s, Arguments = %v, Commands = %v, EnvVars = %v}",
		conf.Name,
		conf.Arguments,
		conf.Commands,
		conf.EnvVars)
}

func (conf *TerraformExtraArguments) GetVarFiles(logger *logrus.Entry) []string {
	varFiles := []string{}

	// Include all specified RequiredVarFiles.
	if conf.RequiredVarFiles != nil {
		for _, file := range util.RemoveDuplicatesFromListKeepLast(*conf.RequiredVarFiles) {
			varFiles = append(varFiles, file)
		}
	}

	// If OptionalVarFiles is specified, check for each file if it exists and if so, include in the var
	// files list. Note that it is possible that many files resolve to the same path, so we remove
	// duplicates.
	if conf.OptionalVarFiles != nil {
		for _, file := range util.RemoveDuplicatesFromListKeepLast(*conf.OptionalVarFiles) {
			if util.FileExists(file) {
				varFiles = append(varFiles, file)
			} else {
				logger.Debugf("Skipping var-file %s as it does not exist", file)
			}
		}
	}

	return varFiles
}

// There are two ways a user can tell Terragrunt that it needs to download Terraform configurations from a specific
// URL: via a command-line option or via an entry in the Terragrunt configuration. If the user used one of these, this
// method returns the source URL or an empty string if there is no source url
func GetTerraformSourceUrl(terragruntOptions *options.TerragruntOptions, terragruntConfig *TerragruntConfig) string {
	if terragruntOptions.Source != "" {
		return terragruntOptions.Source
	} else if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.Source != nil {
		return *terragruntConfig.Terraform.Source
	} else {
		return ""
	}
}

// Return the default hcl path to use for the Terragrunt configuration file in the given directory
func DefaultConfigPath(workingDir string) string {
	return util.JoinPath(workingDir, DefaultTerragruntConfigPath)
}

// Return the default path to use for the Terragrunt Json configuration file in the given directory
func DefaultJsonConfigPath(workingDir string) string {
	return util.JoinPath(workingDir, DefaultTerragruntJsonConfigPath)
}

// Return the default path to use for the Terragrunt configuration that exists within the path giving preference to `terragrunt.hcl`
func GetDefaultConfigPath(workingDir string) string {
	if util.FileNotExists(DefaultConfigPath(workingDir)) && util.FileExists(DefaultJsonConfigPath(workingDir)) {
		return DefaultJsonConfigPath(workingDir)
	}

	return DefaultConfigPath(workingDir)
}

// Returns a list of all Terragrunt config files in the given path or any subfolder of the path. A file is a Terragrunt
// config file if it has a name as returned by the DefaultConfigPath method
func FindConfigFilesInPath(rootPath string, terragruntOptions *options.TerragruntOptions) ([]string, error) {
	configFiles := []string{}

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the Terragrunt cache dir entirely
		if info.IsDir() && info.Name() == options.TerragruntCacheDir {
			return filepath.SkipDir
		}

		isTerragruntModule, err := containsTerragruntModule(path, info, terragruntOptions)
		if err != nil {
			return err
		}

		if isTerragruntModule {
			configFiles = append(configFiles, GetDefaultConfigPath(path))
		}

		return nil
	})

	return configFiles, err
}

// Returns true if the given path with the given FileInfo contains a Terragrunt module and false otherwise. A path
// contains a Terragrunt module if it contains a Terragrunt configuration file (terragrunt.hcl, terragrunt.hcl.json)
// and is not a cache, data, or download dir.
func containsTerragruntModule(path string, info os.FileInfo, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if !info.IsDir() {
		return false, nil
	}

	// Skip the Terragrunt cache dir
	if util.ContainsPath(path, options.TerragruntCacheDir) {
		return false, nil
	}

	// Skip the Terraform data dir
	dataDir := terragruntOptions.TerraformDataDir()
	if filepath.IsAbs(dataDir) {
		if util.HasPathPrefix(path, dataDir) {
			return false, nil
		}
	} else {
		if util.ContainsPath(path, dataDir) {
			return false, nil
		}
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

	return util.FileExists(GetDefaultConfigPath(path)), nil
}

// Read the Terragrunt config file from its default location
func ReadTerragruntConfig(terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	terragruntOptions.Logger.Debugf("Reading Terragrunt config file at %s", terragruntOptions.TerragruntConfigPath)
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
// 1. Parse include. Include is parsed first and is used to import another config. All the config in the include block is
//    then merged into the current TerragruntConfig, except for locals (by design). Note that since the include block is
//    parsed first, you cannot reference locals in the include block config.
// 2. Parse locals. Since locals are parsed next, you can only reference other locals in the locals block. Although it
//    is possible to merge locals from a config imported with an include block, we do not do that here to avoid
//    complicated referencing issues. Please refer to the globals proposal for an alternative that allows merging from
//    included config: https://github.com/gruntwork-io/terragrunt/issues/814
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

	config, err := convertToTerragruntConfig(terragruntConfigFile, filename, terragruntOptions, contextExtensions)
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

// Merge the given config with an included config. Anything specified in the current config will override the contents
// of the included config. If the included config is nil, just return the current config.
func mergeConfigWithIncludedConfig(config *TerragruntConfig, includedConfig *TerragruntConfig, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	if config.RemoteState != nil {
		includedConfig.RemoteState = config.RemoteState
	}

	if config.PreventDestroy != nil {
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

	if config.DownloadDir != "" {
		includedConfig.DownloadDir = config.DownloadDir
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

	if config.RetryableErrors != nil {
		includedConfig.RetryableErrors = config.RetryableErrors
	}

	if config.TerragruntVersionConstraint != "" {
		includedConfig.TerragruntVersionConstraint = config.TerragruntVersionConstraint
	}

	// Merge the generate configs. This is a shallow merge. Meaning, if the child has the same name generate block, then the
	// child's generate block will override the parent's block.
	for key, val := range config.GenerateConfigs {
		includedConfig.GenerateConfigs[key] = val
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
			terragruntOptions.Logger.Debugf("hook '%v' from child overriding parent", child.Name)
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
			terragruntOptions.Logger.Debugf("extra_arguments '%v' from child overriding parent", child.Name)
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
func convertToTerragruntConfig(
	terragruntConfigFromFile *terragruntConfigFile,
	configPath string,
	terragruntOptions *options.TerragruntOptions,
	contextExtensions EvalContextExtensions,
) (cfg *TerragruntConfig, err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfig{RecoveredValue: recovered, ConfigFile: configPath})
		}
	}()

	terragruntConfig := &TerragruntConfig{
		IsPartial: false,
		// Initialize GenerateConfigs so we can append to it
		GenerateConfigs: map[string]codegen.GenerateConfig{},
	}

	if terragruntConfigFromFile.RemoteState != nil {
		remoteState, err := terragruntConfigFromFile.RemoteState.toConfig()
		if err != nil {
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

	if terragruntConfigFromFile.RetryableErrors != nil {
		terragruntConfig.RetryableErrors = terragruntConfigFromFile.RetryableErrors
	}

	if terragruntConfigFromFile.DownloadDir != nil {
		terragruntConfig.DownloadDir = *terragruntConfigFromFile.DownloadDir
	}

	if terragruntConfigFromFile.TerraformVersionConstraint != nil {
		terragruntConfig.TerraformVersionConstraint = *terragruntConfigFromFile.TerraformVersionConstraint
	}

	if terragruntConfigFromFile.TerragruntVersionConstraint != nil {
		terragruntConfig.TerragruntVersionConstraint = *terragruntConfigFromFile.TerragruntVersionConstraint
	}

	if terragruntConfigFromFile.PreventDestroy != nil {
		terragruntConfig.PreventDestroy = terragruntConfigFromFile.PreventDestroy
	}

	if terragruntConfigFromFile.Skip != nil {
		terragruntConfig.Skip = *terragruntConfigFromFile.Skip
	}

	if terragruntConfigFromFile.IamRole != nil {
		terragruntConfig.IamRole = *terragruntConfigFromFile.IamRole
	}

	for _, block := range terragruntConfigFromFile.GenerateBlocks {
		ifExists, err := codegen.GenerateConfigExistsFromString(block.IfExists)
		if err != nil {
			return nil, err
		}
		genConfig := codegen.GenerateConfig{
			Path:        block.Path,
			IfExists:    ifExists,
			IfExistsStr: block.IfExists,
			Contents:    block.Contents,
		}
		if block.CommentPrefix == nil {
			genConfig.CommentPrefix = codegen.DefaultCommentPrefix
		} else {
			genConfig.CommentPrefix = *block.CommentPrefix
		}
		if block.DisableSignature == nil {
			genConfig.DisableSignature = false
		} else {
			genConfig.DisableSignature = *block.DisableSignature
		}
		terragruntConfig.GenerateConfigs[block.Name] = genConfig
	}

	if terragruntConfigFromFile.Inputs != nil {
		inputs, err := parseCtyValueToMap(*terragruntConfigFromFile.Inputs)
		if err != nil {
			return nil, err
		}

		terragruntConfig.Inputs = inputs
	}

	if contextExtensions.Locals != nil && *contextExtensions.Locals != cty.NilVal {
		localsParsed, err := parseCtyValueToMap(*contextExtensions.Locals)
		if err != nil {
			return nil, err
		}
		terragruntConfig.Locals = localsParsed
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
