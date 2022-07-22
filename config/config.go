package config

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/mitchellh/mapstructure"
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

const foundInFile = "found_in_file"

const (
	MetadataTerraform                   = "terraform"
	MetadataTerraformBinary             = "terraform_binary"
	MetadataTerraformVersionConstraint  = "terraform_version_constraint"
	MetadataTerragruntVersionConstraint = "terragrunt_version_constraint"
	MetadataRemoteState                 = "remote_state"
	MetadataDependencies                = "dependencies"
	MetadataDependency                  = "dependency"
	MetadataDownloadDir                 = "download_dir"
	MetadataPreventDestroy              = "prevent_destroy"
	MetadataSkip                        = "skip"
	MetadataIamRole                     = "iam_role"
	MetadataIamAssumeRoleDuration       = "iam_assume_role_duration"
	MetadataIamAssumeRoleSessionName    = "iam_assume_role_session_name"
	MetadataInputs                      = "inputs"
	MetadataLocals                      = "locals"
	MetadataGenerateConfigs             = "generate"
	MetadataRetryableErrors             = "retryable_errors"
	MetadataRetryMaxAttempts            = "retry_max_attempts"
	MetadataRetrySleepIntervalSec       = "retry_sleep_interval_sec"
)

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
	IamAssumeRoleDuration       *int64
	IamAssumeRoleSessionName    string
	Inputs                      map[string]interface{}
	Locals                      map[string]interface{}
	TerragruntDependencies      []Dependency
	GenerateConfigs             map[string]codegen.GenerateConfig
	RetryableErrors             []string
	RetryMaxAttempts            *int
	RetrySleepIntervalSec       *int

	// Fields used for internal tracking
	// Indicates whether or not this is the result of a partial evaluation
	IsPartial bool

	// Map of processed includes
	ProcessedIncludes map[string]IncludeConfig

	// Map to store fields metadata
	FieldsMetadata map[string]map[string]interface{}
}

func (conf *TerragruntConfig) String() string {
	return fmt.Sprintf("TerragruntConfig{Terraform = %v, RemoteState = %v, Dependencies = %v, PreventDestroy = %v}", conf.Terraform, conf.RemoteState, conf.Dependencies, conf.PreventDestroy)
}

// GetIAMRoleOptions is a helper function that converts the Terragrunt config IAM role attributes to
// options.IAMRoleOptions struct.
func (conf *TerragruntConfig) GetIAMRoleOptions() options.IAMRoleOptions {
	configIAMRoleOptions := options.IAMRoleOptions{
		RoleARN:               conf.IamRole,
		AssumeRoleSessionName: conf.IamAssumeRoleSessionName,
	}
	if conf.IamAssumeRoleDuration != nil {
		configIAMRoleOptions.AssumeRoleDuration = *conf.IamAssumeRoleDuration
	}
	return configIAMRoleOptions
}

// terragruntConfigFile represents the configuration supported in a Terragrunt configuration file (i.e.
// terragrunt.hcl)
type terragruntConfigFile struct {
	Terraform                   *TerraformConfig `hcl:"terraform,block"`
	TerraformBinary             *string          `hcl:"terraform_binary,attr"`
	TerraformVersionConstraint  *string          `hcl:"terraform_version_constraint,attr"`
	TerragruntVersionConstraint *string          `hcl:"terragrunt_version_constraint,attr"`
	Inputs                      *cty.Value       `hcl:"inputs,attr"`

	// We allow users to configure remote state (backend) via blocks:
	//
	// remote_state {
	//   backend = "s3"
	//   config  = { ... }
	// }
	//
	// Or as attributes:
	//
	// remote_state = {
	//   backend = "s3"
	//   config  = { ... }
	// }
	RemoteState     *remoteStateConfigFile `hcl:"remote_state,block"`
	RemoteStateAttr *cty.Value             `hcl:"remote_state,optional"`

	Dependencies             *ModuleDependencies `hcl:"dependencies,block"`
	DownloadDir              *string             `hcl:"download_dir,attr"`
	PreventDestroy           *bool               `hcl:"prevent_destroy,attr"`
	Skip                     *bool               `hcl:"skip,attr"`
	IamRole                  *string             `hcl:"iam_role,attr"`
	IamAssumeRoleDuration    *int64              `hcl:"iam_assume_role_duration,attr"`
	IamAssumeRoleSessionName *string             `hcl:"iam_assume_role_session_name,attr"`
	TerragruntDependencies   []Dependency        `hcl:"dependency,block"`

	// We allow users to configure code generation via blocks:
	//
	// generate "example" {
	//   path     = "example.tf"
	//   contents = "example"
	// }
	//
	// Or via attributes:
	//
	// generate = {
	//   example = {
	//     path     = "example.tf"
	//     contents = "example"
	//   }
	// }
	GenerateAttrs  *cty.Value                `hcl:"generate,optional"`
	GenerateBlocks []terragruntGenerateBlock `hcl:"generate,block"`

	RetryableErrors       []string `hcl:"retryable_errors,optional"`
	RetryMaxAttempts      *int     `hcl:"retry_max_attempts,optional"`
	RetrySleepIntervalSec *int     `hcl:"retry_sleep_interval_sec,optional"`

	// This struct is used for validating and parsing the entire terragrunt config. Since locals and include are
	// evaluated in a completely separate cycle, it should not be evaluated here. Otherwise, we can't support self
	// referencing other elements in the same block.
	// We don't want to use the special Remain keyword here, as that would cause the checker to support parsing config
	// that have extraneous, unsupported blocks and attributes.
	Locals  *terragruntLocal          `hcl:"locals,block"`
	Include []terragruntIncludeIgnore `hcl:"include,block"`
}

// We use a struct designed to not parse the block, as locals and includes are parsed and decoded using a special
// routine that allows references to the other locals in the same block.
type terragruntLocal struct {
	Remain hcl.Body `hcl:",remain"`
}
type terragruntIncludeIgnore struct {
	Name   string   `hcl:"name,label"`
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
	Path             string  `hcl:"path,attr" mapstructure:"path"`
	IfExists         string  `hcl:"if_exists,attr" mapstructure:"if_exists"`
	CommentPrefix    *string `hcl:"comment_prefix,attr" mapstructure:"comment_prefix"`
	Contents         string  `hcl:"contents,attr" mapstructure:"contents"`
	DisableSignature *bool   `hcl:"disable_signature,attr" mapstructure:"disable_signature"`
}

// IncludeConfig represents the configuration settings for a parent Terragrunt configuration file that you can
// include into a child Terragrunt configuration file. You can have more than one include config.
type IncludeConfig struct {
	Name          string  `hcl:"name,label"`
	Path          string  `hcl:"path,attr"`
	Expose        *bool   `hcl:"expose,attr"`
	MergeStrategy *string `hcl:"merge_strategy,attr"`
}

func (cfg *IncludeConfig) String() string {
	return fmt.Sprintf("IncludeConfig{Path = %s, Expose = %v, MergeStrategy = %v}", cfg.Path, cfg.Expose, cfg.MergeStrategy)
}

func (cfg *IncludeConfig) GetExpose() bool {
	if cfg == nil || cfg.Expose == nil {
		return false
	}
	return *cfg.Expose
}

func (cfg *IncludeConfig) GetMergeStrategy() (MergeStrategyType, error) {
	if cfg.MergeStrategy == nil {
		return ShallowMerge, nil
	}

	strategy := *cfg.MergeStrategy
	switch strategy {
	case string(NoMerge):
		return NoMerge, nil
	case string(ShallowMerge):
		return ShallowMerge, nil
	case string(DeepMerge):
		return DeepMerge, nil
	case string(DeepMergeMapOnly):
		return DeepMergeMapOnly, nil
	default:
		return NoMerge, errors.WithStackTrace(InvalidMergeStrategyType(strategy))
	}
}

type MergeStrategyType string

const (
	NoMerge          MergeStrategyType = "no_merge"
	ShallowMerge     MergeStrategyType = "shallow"
	DeepMerge        MergeStrategyType = "deep"
	DeepMergeMapOnly MergeStrategyType = "deep_map_only"
)

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

type ErrorHook struct {
	Name       string   `hcl:"name,label" cty:"name"`
	Commands   []string `hcl:"commands,attr" cty:"commands"`
	Execute    []string `hcl:"execute,attr" cty:"execute"`
	OnErrors   []string `hcl:"on_errors,attr" cty:"on_errors"`
	WorkingDir *string  `hcl:"working_dir,attr" cty:"working_dir"`
}

func (conf *Hook) String() string {
	return fmt.Sprintf("Hook{Name = %s, Commands = %v}", conf.Name, len(conf.Commands))
}
func (conf *ErrorHook) String() string {
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
	ErrorHooks  []ErrorHook               `hcl:"error_hook,block"`

	// Ideally we can avoid the pointer to list slice, but if it is not a pointer, Terraform requires the attribute to
	// be defined and we want to make this optional.
	IncludeInCopy *[]string `hcl:"include_in_copy,attr"`
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

func (conf *TerraformConfig) GetErrorHooks() []ErrorHook {
	if conf == nil {
		return nil
	}

	return conf.ErrorHooks
}

func (conf *TerraformConfig) ValidateHooks() error {
	beforeAndAfterHooks := append(conf.GetBeforeHooks(), conf.GetAfterHooks()...)

	for _, curHook := range beforeAndAfterHooks {
		if len(curHook.Execute) < 1 || curHook.Execute[0] == "" {
			return InvalidArgError(fmt.Sprintf("Error with hook %s. Need at least one non-empty argument in 'execute'.", curHook.Name))
		}
	}

	for _, curHook := range conf.GetErrorHooks() {
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
func GetTerraformSourceUrl(terragruntOptions *options.TerragruntOptions, terragruntConfig *TerragruntConfig) (string, error) {
	if terragruntOptions.Source != "" {
		return terragruntOptions.Source, nil
	} else if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.Source != nil {
		return adjustSourceWithMap(terragruntOptions.SourceMap, *terragruntConfig.Terraform.Source, terragruntOptions.OriginalTerragruntConfigPath)
	} else {
		return "", nil
	}
}

// adjustSourceWithMap implements the --terragrunt-source-map feature. This function will check if the URL portion of a
// terraform source matches any entry in the provided source map and if it does, replace it with the configured source
// in the map. Note that this only performs literal matches with the URL portion.
//
// Example:
// Suppose terragrunt is called with:
//
//   --terragrunt-source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=/path/to/local-modules
//
// and the terraform source is:
//
//   git::ssh://git@github.com/gruntwork-io/i-dont-exist.git//fixture-source-map/modules/app?ref=master
//
// This function will take that source and transform it to:
//
//   /path/to/local-modules/fixture-source-map/modules/app
//
func adjustSourceWithMap(sourceMap map[string]string, source string, modulePath string) (string, error) {
	// Skip logic if source map is not configured
	if len(sourceMap) == 0 {
		return source, nil
	}

	// use go-getter to split the module source string into a valid URL and subdirectory (if // is present)
	moduleUrl, moduleSubdir := getter.SourceDirSubdir(source)

	// if both URL and subdir are missing, something went terribly wrong
	if moduleUrl == "" && moduleSubdir == "" {
		return "", errors.WithStackTrace(InvalidSourceUrlWithMap{ModulePath: modulePath, ModuleSourceUrl: source})
	}

	// If module URL is missing, return the source as is as it will not match anything in the map.
	if moduleUrl == "" {
		return source, nil
	}

	// Before looking up in sourceMap, make sure to drop any query parameters.
	moduleUrlParsed, err := url.Parse(moduleUrl)
	if err != nil {
		return source, err
	}
	moduleUrlParsed.RawQuery = ""
	moduleUrlQuery := moduleUrlParsed.String()

	// Check if there is an entry to replace the URL portion in the map. Return the source as is if there is no entry in
	// the map.
	sourcePath, hasKey := sourceMap[moduleUrlQuery]
	if hasKey == false {
		return source, nil
	}

	// Since there is a source mapping, replace the module URL portion with the entry in the map, and join with the
	// subdir.
	// If subdir is missing, check if we can obtain a valid module name from the URL portion.
	if moduleSubdir == "" {
		moduleSubdirFromUrl, err := getModulePathFromSourceUrl(moduleUrl)
		if err != nil {
			return moduleSubdirFromUrl, err
		}
		moduleSubdir = moduleSubdirFromUrl
	}
	return util.JoinTerraformModulePath(sourcePath, moduleSubdir), nil

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
	return ParseConfigFile(terragruntOptions.TerragruntConfigPath, terragruntOptions, nil, nil)
}

// Parse the Terragrunt config file at the given path. If the include parameter is not nil, then treat this as a config
// included in some other config file when resolving relative paths.
func ParseConfigFile(filename string, terragruntOptions *options.TerragruntOptions, include *IncludeConfig, dependencyOutputs *cty.Value) (*TerragruntConfig, error) {
	configString, err := util.ReadFileAsString(filename)
	if err != nil {
		return nil, err
	}

	config, err := ParseConfigString(configString, terragruntOptions, include, filename, dependencyOutputs)
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
//    Note that this step is skipped if we already retrieved all the dependencies (which is the case when parsing
//    included config files). This is determined by the dependencyOutputs input parameter.
//    Allowed References:
//      - locals
// 4. Parse everything else. At this point, all the necessary building blocks for parsing the rest of the config are
//    available, so parse the rest of the config.
//    Allowed References:
//      - locals
//      - dependency
// 5. Merge the included config with the parsed config. Note that all the config data is mergable except for `locals`
//    blocks, which are only scoped to be available within the defining config.
func ParseConfigString(
	configString string,
	terragruntOptions *options.TerragruntOptions,
	includeFromChild *IncludeConfig,
	filename string,
	dependencyOutputs *cty.Value,
) (*TerragruntConfig, error) {
	// Parse the HCL string into an AST body that can be decoded multiple times later without having to re-parse
	parser := hclparse.NewParser()
	file, err := parseHcl(parser, configString, filename)
	if err != nil {
		return nil, err
	}

	// Initial evaluation of configuration to load flags like IamRole which will be used for final parsing
	// https://github.com/gruntwork-io/terragrunt/issues/667
	if err := setIAMRole(configString, terragruntOptions, includeFromChild, filename); err != nil {
		return nil, err
	}

	// Decode just the Base blocks. See the function docs for DecodeBaseBlocks for more info on what base blocks are.
	localsAsCty, trackInclude, err := DecodeBaseBlocks(terragruntOptions, parser, file, filename, includeFromChild, nil)
	if err != nil {
		return nil, err
	}

	// Initialize evaluation context extensions from base blocks.
	contextExtensions := EvalContextExtensions{
		Locals:              localsAsCty,
		TrackInclude:        trackInclude,
		DecodedDependencies: dependencyOutputs,
	}

	if dependencyOutputs == nil {
		// Decode just the `dependency` blocks, retrieving the outputs from the target terragrunt config in the
		// process.
		retrievedOutputs, err := decodeAndRetrieveOutputs(file, filename, terragruntOptions, trackInclude, contextExtensions)
		if err != nil {
			return nil, err
		}
		contextExtensions.DecodedDependencies = retrievedOutputs
	}

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
	if trackInclude != nil {
		mergedConfig, err := handleInclude(config, trackInclude, terragruntOptions, contextExtensions.DecodedDependencies)
		if err != nil {
			return nil, err
		}
		// Saving processed includes into configuration, direct assignment since nested includes aren't supported
		mergedConfig.ProcessedIncludes = trackInclude.CurrentMap
		// Make sure the top level information that is not automatically merged in is captured on the merged config to
		// ensure the proper representation of the config is captured.
		// - Locals are deliberately not merged in so that they remain local in scope. Here, we directly set it to the
		//   original locals for the current config being handled, as that is the locals list that is in scope for this
		//   config.
		mergedConfig.Locals = config.Locals

		return mergedConfig, nil
	}
	return config, nil
}

// iamRoleCache - store for cached values of IAM roles
var iamRoleCache = NewIAMRoleOptionsCache()

// setIAMRole - extract IAM role details from Terragrunt flags block
func setIAMRole(configString string, terragruntOptions *options.TerragruntOptions, includeFromChild *IncludeConfig, filename string) error {
	// as key is considered HCL code and include configuration
	var key = fmt.Sprintf("%v-%v", configString, includeFromChild)
	var config, found = iamRoleCache.Get(key)
	if !found {
		iamConfig, err := PartialParseConfigString(configString, terragruntOptions, includeFromChild, filename, []PartialDecodeSectionType{TerragruntFlags})
		if err != nil {
			return err
		}
		config = iamConfig.GetIAMRoleOptions()
		iamRoleCache.Put(key, config)
	}
	// We merge the OriginalIAMRoleOptions into the one from the config, because the CLI passed IAMRoleOptions has
	// precedence.
	terragruntOptions.IAMRoleOptions = options.MergeIAMRoleOptions(
		config,
		terragruntOptions.OriginalIAMRoleOptions,
	)
	return nil
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

// Returns the index of the ErrorHook with the given name,
// or -1 if no Hook have the given name.
// TODO: Figure out more DRY way to do this
func getIndexOfErrorHookWithName(hooks []ErrorHook, name string) int {
	for i, hook := range hooks {
		if hook.Name == name {
			return i
		}
	}
	return -1
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

	defaultMetadata := map[string]interface{}{foundInFile: configPath}
	if terragruntConfigFromFile.RemoteState != nil {
		remoteState, err := terragruntConfigFromFile.RemoteState.toConfig()
		if err != nil {
			return nil, err
		}
		terragruntConfig.RemoteState = remoteState
		terragruntConfig.SetFieldMetadata(MetadataRemoteState, defaultMetadata)
	}

	if terragruntConfigFromFile.RemoteStateAttr != nil {
		remoteStateMap, err := parseCtyValueToMap(*terragruntConfigFromFile.RemoteStateAttr)
		if err != nil {
			return nil, err
		}

		var remoteState *remote.RemoteState
		if err := mapstructure.Decode(remoteStateMap, &remoteState); err != nil {
			return nil, err
		}

		terragruntConfig.RemoteState = remoteState
		terragruntConfig.SetFieldMetadata(MetadataRemoteState, defaultMetadata)
	}

	if err := terragruntConfigFromFile.Terraform.ValidateHooks(); err != nil {
		return nil, err
	}

	terragruntConfig.Terraform = terragruntConfigFromFile.Terraform
	if terragruntConfig.Terraform != nil { // since Terraform is nil each time avoid saving metadata when it is nil
		terragruntConfig.SetFieldMetadata(MetadataTerraform, defaultMetadata)
	}

	if err := validateDependencies(terragruntOptions, terragruntConfigFromFile.Dependencies); err != nil {
		return nil, err
	}
	terragruntConfig.Dependencies = terragruntConfigFromFile.Dependencies
	if terragruntConfig.Dependencies != nil {
		for _, item := range terragruntConfig.Dependencies.Paths {
			terragruntConfig.SetFieldMetadataWithType(MetadataDependencies, item, defaultMetadata)
		}
	}

	terragruntConfig.TerragruntDependencies = terragruntConfigFromFile.TerragruntDependencies
	for _, dep := range terragruntConfig.TerragruntDependencies {
		terragruntConfig.SetFieldMetadataWithType(MetadataDependency, dep.Name, defaultMetadata)
	}

	if terragruntConfigFromFile.TerraformBinary != nil {
		terragruntConfig.TerraformBinary = *terragruntConfigFromFile.TerraformBinary
		terragruntConfig.SetFieldMetadata(MetadataTerraformBinary, defaultMetadata)
	}

	if terragruntConfigFromFile.RetryableErrors != nil {
		terragruntConfig.RetryableErrors = terragruntConfigFromFile.RetryableErrors
		terragruntConfig.SetFieldMetadata(MetadataRetryableErrors, defaultMetadata)
	}

	if terragruntConfigFromFile.RetryMaxAttempts != nil {
		terragruntConfig.RetryMaxAttempts = terragruntConfigFromFile.RetryMaxAttempts
		terragruntConfig.SetFieldMetadata(MetadataRetryMaxAttempts, defaultMetadata)
	}

	if terragruntConfigFromFile.RetrySleepIntervalSec != nil {
		terragruntConfig.RetrySleepIntervalSec = terragruntConfigFromFile.RetrySleepIntervalSec
		terragruntConfig.SetFieldMetadata(MetadataRetrySleepIntervalSec, defaultMetadata)
	}

	if terragruntConfigFromFile.DownloadDir != nil {
		terragruntConfig.DownloadDir = *terragruntConfigFromFile.DownloadDir
		terragruntConfig.SetFieldMetadata(MetadataDownloadDir, defaultMetadata)
	}

	if terragruntConfigFromFile.TerraformVersionConstraint != nil {
		terragruntConfig.TerraformVersionConstraint = *terragruntConfigFromFile.TerraformVersionConstraint
		terragruntConfig.SetFieldMetadata(MetadataTerraformVersionConstraint, defaultMetadata)
	}

	if terragruntConfigFromFile.TerragruntVersionConstraint != nil {
		terragruntConfig.TerragruntVersionConstraint = *terragruntConfigFromFile.TerragruntVersionConstraint
		terragruntConfig.SetFieldMetadata(MetadataTerragruntVersionConstraint, defaultMetadata)
	}

	if terragruntConfigFromFile.PreventDestroy != nil {
		terragruntConfig.PreventDestroy = terragruntConfigFromFile.PreventDestroy
		terragruntConfig.SetFieldMetadata(MetadataPreventDestroy, defaultMetadata)
	}

	if terragruntConfigFromFile.Skip != nil {
		terragruntConfig.Skip = *terragruntConfigFromFile.Skip
		terragruntConfig.SetFieldMetadata(MetadataSkip, defaultMetadata)
	}

	if terragruntConfigFromFile.IamRole != nil {
		terragruntConfig.IamRole = *terragruntConfigFromFile.IamRole
		terragruntConfig.SetFieldMetadata(MetadataIamRole, defaultMetadata)
	}

	if terragruntConfigFromFile.IamAssumeRoleDuration != nil {
		terragruntConfig.IamAssumeRoleDuration = terragruntConfigFromFile.IamAssumeRoleDuration
		terragruntConfig.SetFieldMetadata(MetadataIamAssumeRoleDuration, defaultMetadata)
	}

	if terragruntConfigFromFile.IamAssumeRoleSessionName != nil {
		terragruntConfig.IamAssumeRoleSessionName = *terragruntConfigFromFile.IamAssumeRoleSessionName
		terragruntConfig.SetFieldMetadata(MetadataIamAssumeRoleSessionName, defaultMetadata)
	}

	generateBlocks := []terragruntGenerateBlock{}
	generateBlocks = append(generateBlocks, terragruntConfigFromFile.GenerateBlocks...)

	if terragruntConfigFromFile.GenerateAttrs != nil {
		generateMap, err := parseCtyValueToMap(*terragruntConfigFromFile.GenerateAttrs)
		if err != nil {
			return nil, err
		}

		for name, block := range generateMap {
			var generateBlock terragruntGenerateBlock
			if err := mapstructure.Decode(block, &generateBlock); err != nil {
				return nil, err
			}
			generateBlock.Name = name
			generateBlocks = append(generateBlocks, generateBlock)
		}
	}
	if err := validateGenerateBlocks(&generateBlocks); err != nil {
		return nil, err
	}
	for _, block := range generateBlocks {
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
		terragruntConfig.SetFieldMetadataWithType(MetadataGenerateConfigs, block.Name, defaultMetadata)
	}

	if terragruntConfigFromFile.Inputs != nil {
		inputs, err := parseCtyValueToMap(*terragruntConfigFromFile.Inputs)
		if err != nil {
			return nil, err
		}

		terragruntConfig.Inputs = inputs
		terragruntConfig.SetFieldMetadataMap(MetadataInputs, terragruntConfig.Inputs, defaultMetadata)
	}

	if contextExtensions.Locals != nil && *contextExtensions.Locals != cty.NilVal {
		localsParsed, err := parseCtyValueToMap(*contextExtensions.Locals)
		if err != nil {
			return nil, err
		}
		terragruntConfig.Locals = localsParsed
		terragruntConfig.SetFieldMetadataMap(MetadataLocals, localsParsed, defaultMetadata)
	}

	return terragruntConfig, nil
}

// Iterate over dependencies paths and check if directories exists, return error with all missing dependencies
func validateDependencies(terragruntOptions *options.TerragruntOptions, dependencies *ModuleDependencies) error {
	var missingDependencies []string
	if dependencies == nil {
		return nil
	}
	for _, dependencyPath := range dependencies.Paths {
		fullPath := filepath.FromSlash(dependencyPath)
		if !filepath.IsAbs(fullPath) {
			fullPath = path.Join(terragruntOptions.WorkingDir, fullPath)
		}
		if !util.IsDir(fullPath) {
			missingDependencies = append(missingDependencies, fmt.Sprintf("%s (%s)", dependencyPath, fullPath))
		}
	}
	if len(missingDependencies) > 0 {
		return DependencyDirNotFound{missingDependencies}
	}

	return nil
}

// Iterate over generate blocks and detect duplicate names, return error with list of duplicated names
func validateGenerateBlocks(blocks *[]terragruntGenerateBlock) error {
	var blockNames = map[string]bool{}
	var duplicatedGenerateBlockNames []string

	for _, block := range *blocks {
		_, found := blockNames[block.Name]
		if found {
			duplicatedGenerateBlockNames = append(duplicatedGenerateBlockNames, block.Name)
			continue
		}
		blockNames[block.Name] = true
	}
	if len(duplicatedGenerateBlockNames) != 0 {
		return DuplicatedGenerateBlocks{duplicatedGenerateBlockNames}
	}
	return nil
}

// configFileHasDependencyBlock statically checks the terrragrunt config file at the given path and checks if it has any
// dependency or dependencies blocks defined. Note that this does not do any decoding of the blocks, as it is only meant
// to check for block presence.
func configFileHasDependencyBlock(configPath string, terragruntOptions *options.TerragruntOptions) (bool, error) {
	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	// We use hclwrite to parse the config instead of the normal parser because the normal parser doesn't give us an AST
	// that we can walk and scan, and requires structured data to map against. This makes the parsing strict, so to
	// avoid weird parsing errors due to missing dependency data, we do a structural scan here.
	hclFile, diags := hclwrite.ParseConfig(configBytes, configPath, hcl.InitialPos)
	if diags.HasErrors() {
		return false, errors.WithStackTrace(diags)
	}
	for _, block := range hclFile.Body().Blocks() {
		if block.Type() == "dependency" || block.Type() == "dependencies" {
			return true, nil
		}
	}
	return false, nil
}

// SetFieldMetadataWithType set metadata on the given field name grouped by type.
// Example usage - setting metadata on different dependencies, locals, inputs.
func (conf *TerragruntConfig) SetFieldMetadataWithType(fieldType, fieldName string, m map[string]interface{}) {
	if conf.FieldsMetadata == nil {
		conf.FieldsMetadata = map[string]map[string]interface{}{}
	}

	field := fmt.Sprintf("%s-%s", fieldType, fieldName)

	metadata, found := conf.FieldsMetadata[field]
	if !found {
		metadata = make(map[string]interface{})
	}
	for key, value := range m {
		metadata[key] = value
	}
	conf.FieldsMetadata[field] = metadata
}

// SetFieldMetadata set metadata on the given field name.
func (conf *TerragruntConfig) SetFieldMetadata(fieldName string, m map[string]interface{}) {
	conf.SetFieldMetadataWithType(fieldName, fieldName, m)
}

// SetFieldMetadataMap set metadata on fields from map keys.
// Example usage - setting metadata on all variables from inputs.
func (conf *TerragruntConfig) SetFieldMetadataMap(field string, data map[string]interface{}, metadata map[string]interface{}) {
	for name, _ := range data {
		conf.SetFieldMetadataWithType(field, name, metadata)
	}
}

// GetFieldMetadata return field metadata by field name.
func (conf *TerragruntConfig) GetFieldMetadata(fieldName string) (map[string]string, bool) {
	return conf.GetMapFieldMetadata(fieldName, fieldName)
}

// GetMapFieldMetadata return field metadata by field type and name.
func (conf *TerragruntConfig) GetMapFieldMetadata(fieldType, fieldName string) (map[string]string, bool) {
	if conf.FieldsMetadata == nil {
		return nil, false
	}
	field := fmt.Sprintf("%s-%s", fieldType, fieldName)

	value, found := conf.FieldsMetadata[field]
	if !found {
		return nil, false
	}
	var result = make(map[string]string)
	for key, value := range value {
		result[key] = fmt.Sprintf("%v", value)
	}

	return result, found
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

type InvalidMergeStrategyType string

func (err InvalidMergeStrategyType) Error() string {
	return fmt.Sprintf(
		"Include merge strategy %s is unknown. Valid strategies are: %s, %s, %s, %s",
		string(err),
		NoMerge,
		ShallowMerge,
		DeepMerge,
		DeepMergeMapOnly,
	)
}

type DependencyDirNotFound struct {
	Dir []string
}

func (err DependencyDirNotFound) Error() string {
	return fmt.Sprintf(
		"Found paths in the 'dependencies' block that do not exist: %v", err.Dir,
	)
}

type DuplicatedGenerateBlocks struct {
	BlockName []string
}

func (err DuplicatedGenerateBlocks) Error() string {
	return fmt.Sprintf(
		"Detected generate blocks with the same name: %v", err.BlockName,
	)
}
