// Package config provides functionality for parsing Terragrunt configuration files.
package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/writer"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/mitchellh/mapstructure"

	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	DefaultTerragruntConfigPath     = "terragrunt.hcl"
	DefaultTerragruntJSONConfigPath = "terragrunt.hcl.json"
	FoundInFile                     = "found_in_file"

	iamRoleCacheName = "iamRoleCache"

	DefaultEngineType                   = "rpc"
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
	MetadataIamWebIdentityToken         = "iam_web_identity_token"
	MetadataInputs                      = "inputs"
	MetadataLocals                      = "locals"
	MetadataLocal                       = "local"
	MetadataCatalog                     = "catalog"
	MetadataEngine                      = "engine"
	MetadataGenerateConfigs             = "generate"
	MetadataRetryableErrors             = "retryable_errors"
	MetadataRetryMaxAttempts            = "retry_max_attempts"
	MetadataRetrySleepIntervalSec       = "retry_sleep_interval_sec"
	MetadataDependentModules            = "dependent_modules"
	MetadataInclude                     = "include"
	MetadataFeatureFlag                 = "feature"
	MetadataExclude                     = "exclude"
	MetadataErrors                      = "errors"
	MetadataRetry                       = "retry"
	MetadataIgnore                      = "ignore"
)

var (
	// Order matters, for example if none of the files are found `GetDefaultConfigPath` func returns the last element.
	DefaultTerragruntConfigPaths = []string{
		DefaultTerragruntJSONConfigPath,
		DefaultTerragruntConfigPath,
	}

	DefaultParserOptions = func(opts *options.TerragruntOptions) []hclparse.Option {
		writer := writer.New(writer.WithLogger(opts.Logger), writer.WithDefaultLevel(log.ErrorLevel))

		return []hclparse.Option{
			hclparse.WithDiagnosticsWriter(writer, opts.DisableLogColors),
			hclparse.WithFileUpdate(updateBareIncludeBlock),
			hclparse.WithLogger(opts.Logger),
		}
	}

	DefaultGenerateBlockIfDisabledValueStr = codegen.DisabledSkipStr
)

// DecodedBaseBlocks decoded base blocks struct
type DecodedBaseBlocks struct {
	TrackInclude *TrackInclude
	Locals       *cty.Value
	FeatureFlags *cty.Value
}

// TerragruntConfig represents a parsed and expanded configuration
// NOTE: if any attributes are added, make sure to update terragruntConfigAsCty in config_as_cty.go
type TerragruntConfig struct {
	Catalog                     *CatalogConfig
	Terraform                   *TerraformConfig
	TerraformBinary             string
	TerraformVersionConstraint  string
	TerragruntVersionConstraint string
	RemoteState                 *remote.RemoteState
	Dependencies                *ModuleDependencies
	DownloadDir                 string
	PreventDestroy              *bool
	Skip                        *bool
	IamRole                     string
	IamAssumeRoleDuration       *int64
	IamAssumeRoleSessionName    string
	IamWebIdentityToken         string
	Inputs                      map[string]interface{}
	Locals                      map[string]interface{}
	TerragruntDependencies      Dependencies
	GenerateConfigs             map[string]codegen.GenerateConfig
	RetryableErrors             []string
	RetryMaxAttempts            *int
	RetrySleepIntervalSec       *int
	Engine                      *EngineConfig
	FeatureFlags                FeatureFlags
	Exclude                     *ExcludeConfig
	Errors                      *ErrorsConfig

	// Fields used for internal tracking
	// Indicates whether this is the result of a partial evaluation
	IsPartial bool

	// Map of processed includes
	ProcessedIncludes IncludeConfigsMap

	// Map to store fields metadata
	FieldsMetadata map[string]map[string]interface{}

	// List of dependent modules
	DependentModulesPath []*string
}

func (cfg *TerragruntConfig) String() string {
	return fmt.Sprintf("TerragruntConfig{Terraform = %v, RemoteState = %v, Dependencies = %v, PreventDestroy = %v}", cfg.Terraform, cfg.RemoteState, cfg.Dependencies, cfg.PreventDestroy)
}

// GetIAMRoleOptions is a helper function that converts the Terragrunt config IAM role attributes to
// options.IAMRoleOptions struct.
func (cfg *TerragruntConfig) GetIAMRoleOptions() options.IAMRoleOptions {
	configIAMRoleOptions := options.IAMRoleOptions{
		RoleARN:               cfg.IamRole,
		AssumeRoleSessionName: cfg.IamAssumeRoleSessionName,
		WebIdentityToken:      cfg.IamWebIdentityToken,
	}
	if cfg.IamAssumeRoleDuration != nil {
		configIAMRoleOptions.AssumeRoleDuration = *cfg.IamAssumeRoleDuration
	}

	return configIAMRoleOptions
}

// terragruntConfigFile represents the configuration supported in a Terragrunt configuration file (i.e.
// terragrunt.hcl)
type terragruntConfigFile struct {
	Catalog                     *CatalogConfig   `hcl:"catalog,block"`
	Engine                      *EngineConfig    `hcl:"engine,block"`
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
	IamWebIdentityToken      *string             `hcl:"iam_web_identity_token,attr"`
	TerragruntDependencies   []Dependency        `hcl:"dependency,block"`
	FeatureFlags             []*FeatureFlag      `hcl:"feature,block"`
	Exclude                  *ExcludeConfig      `hcl:"exclude,block"`
	Errors                   *ErrorsConfig       `hcl:"errors,block"`

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
	remoteStateConfig, err := ParseCtyValueToMap(remoteState.Config)
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
	Name             string  `hcl:",label" mapstructure:",omitempty"`
	Path             string  `hcl:"path,attr" mapstructure:"path"`
	IfExists         string  `hcl:"if_exists,attr" mapstructure:"if_exists"`
	IfDisabled       *string `hcl:"if_disabled,attr" mapstructure:"if_disabled"`
	CommentPrefix    *string `hcl:"comment_prefix,attr" mapstructure:"comment_prefix"`
	Contents         string  `hcl:"contents,attr" mapstructure:"contents"`
	DisableSignature *bool   `hcl:"disable_signature,attr" mapstructure:"disable_signature"`
	Disable          *bool   `hcl:"disable,attr" mapstructure:"disable"`
}

type IncludeConfigsMap map[string]IncludeConfig

// ContainsPath returns true if the given path is contained in at least one configuration.
func (cfgs IncludeConfigsMap) ContainsPath(path string) bool {
	for _, cfg := range cfgs {
		if cfg.Path == path {
			return true
		}
	}

	return false
}

type IncludeConfigs []IncludeConfig

// IncludeConfig represents the configuration settings for a parent Terragrunt configuration file that you can
// include into a child Terragrunt configuration file. You can have more than one include config.
type IncludeConfig struct {
	Name          string  `hcl:"name,label"`
	Path          string  `hcl:"path,attr"`
	Expose        *bool   `hcl:"expose,attr"`
	MergeStrategy *string `hcl:"merge_strategy,attr"`
}

func (include *IncludeConfig) String() string {
	if include == nil {
		return "IncludeConfig{nil}"
	}

	exposeStr := "nil"
	if include.Expose != nil {
		exposeStr = strconv.FormatBool(*include.Expose)
	}

	mergeStrategyStr := "nil"
	if include.MergeStrategy != nil {
		mergeStrategyStr = fmt.Sprintf("%v", include.MergeStrategy)
	}

	return fmt.Sprintf("IncludeConfig{Path = %v, Expose = %v, MergeStrategy = %v}", include.Path, exposeStr, mergeStrategyStr)
}

func (include *IncludeConfig) GetExpose() bool {
	if include == nil || include.Expose == nil {
		return false
	}

	return *include.Expose
}

func (include *IncludeConfig) GetMergeStrategy() (MergeStrategyType, error) {
	if include.MergeStrategy == nil {
		return ShallowMerge, nil
	}

	strategy := *include.MergeStrategy
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
		return NoMerge, errors.New(InvalidMergeStrategyTypeError(strategy))
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
	Name           string   `hcl:"name,label" cty:"name"`
	Commands       []string `hcl:"commands,attr" cty:"commands"`
	Execute        []string `hcl:"execute,attr" cty:"execute"`
	RunOnError     *bool    `hcl:"run_on_error,attr" cty:"run_on_error"`
	SuppressStdout *bool    `hcl:"suppress_stdout,attr" cty:"suppress_stdout"`
	WorkingDir     *string  `hcl:"working_dir,attr" cty:"working_dir"`
}

type ErrorHook struct {
	Name           string   `hcl:"name,label" cty:"name"`
	Commands       []string `hcl:"commands,attr" cty:"commands"`
	Execute        []string `hcl:"execute,attr" cty:"execute"`
	OnErrors       []string `hcl:"on_errors,attr" cty:"on_errors"`
	SuppressStdout *bool    `hcl:"suppress_stdout,attr" cty:"suppress_stdout"`
	WorkingDir     *string  `hcl:"working_dir,attr" cty:"working_dir"`
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
	IncludeInCopy   *[]string `hcl:"include_in_copy,attr"`
	ExcludeFromCopy *[]string `hcl:"exclude_from_copy,attr"`

	CopyTerraformLockFile *bool `hcl:"copy_terraform_lock_file,attr"`
}

func (cfg *TerraformConfig) String() string {
	return fmt.Sprintf("TerraformConfig{Source = %v}", cfg.Source)
}

func (cfg *TerraformConfig) GetBeforeHooks() []Hook {
	if cfg == nil {
		return nil
	}

	return cfg.BeforeHooks
}

func (cfg *TerraformConfig) GetAfterHooks() []Hook {
	if cfg == nil {
		return nil
	}

	return cfg.AfterHooks
}

func (cfg *TerraformConfig) GetErrorHooks() []ErrorHook {
	if cfg == nil {
		return nil
	}

	return cfg.ErrorHooks
}

func (cfg *TerraformConfig) ValidateHooks() error {
	beforeAndAfterHooks := append(cfg.GetBeforeHooks(), cfg.GetAfterHooks()...)

	for _, curHook := range beforeAndAfterHooks {
		if len(curHook.Execute) < 1 || curHook.Execute[0] == "" {
			return InvalidArgError(fmt.Sprintf("Error with hook %s. Need at least one non-empty argument in 'execute'.", curHook.Name))
		}
	}

	for _, curHook := range cfg.GetErrorHooks() {
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

func (args *TerraformExtraArguments) String() string {
	return fmt.Sprintf(
		"TerraformArguments{Name = %s, Arguments = %v, Commands = %v, EnvVars = %v}",
		args.Name,
		args.Arguments,
		args.Commands,
		args.EnvVars)
}

func (args *TerraformExtraArguments) GetVarFiles(logger log.Logger) []string {
	var varFiles []string

	// Include all specified RequiredVarFiles.
	if args.RequiredVarFiles != nil {
		varFiles = append(varFiles, util.RemoveDuplicatesFromListKeepLast(*args.RequiredVarFiles)...)
	}

	// If OptionalVarFiles is specified, check for each file if it exists and if so, include in the var
	// files list. Note that it is possible that many files resolve to the same path, so we remove
	// duplicates.
	if args.OptionalVarFiles != nil {
		for _, file := range util.RemoveDuplicatesFromListKeepLast(*args.OptionalVarFiles) {
			if util.FileExists(file) {
				varFiles = append(varFiles, file)
			} else {
				logger.Debugf("Skipping var-file %s as it does not exist", file)
			}
		}
	}

	return varFiles
}

// GetTerraformSourceURL returns the source URL for OpenTofu/Terraform configuration.
//
// There are two ways a user can tell Terragrunt that it needs to download Terraform configurations from a specific
// URL: via a command-line option or via an entry in the Terragrunt configuration. If the user used one of these, this
// method returns the source URL or an empty string if there is no source url
func GetTerraformSourceURL(terragruntOptions *options.TerragruntOptions, terragruntConfig *TerragruntConfig) (string, error) {
	switch {
	case terragruntOptions.Source != "":
		return terragruntOptions.Source, nil
	case terragruntConfig.Terraform != nil && terragruntConfig.Terraform.Source != nil:
		return adjustSourceWithMap(terragruntOptions.SourceMap, *terragruntConfig.Terraform.Source, terragruntOptions.OriginalTerragruntConfigPath)
	default:
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
//	--terragrunt-source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=/path/to/local-modules
//
// and the terraform source is:
//
//	git::ssh://git@github.com/gruntwork-io/i-dont-exist.git//fixtures/source-map/modules/app?ref=master
//
// This function will take that source and transform it to:
//
//	/path/to/local-modules/source-map/modules/app
func adjustSourceWithMap(sourceMap map[string]string, source string, modulePath string) (string, error) {
	// Skip logic if source map is not configured
	if len(sourceMap) == 0 {
		return source, nil
	}

	// use go-getter to split the module source string into a valid URL and subdirectory (if // is present)
	moduleURL, moduleSubdir := getter.SourceDirSubdir(source)

	// if both URL and subdir are missing, something went terribly wrong
	if moduleURL == "" && moduleSubdir == "" {
		return "", errors.New(InvalidSourceURLWithMapError{ModulePath: modulePath, ModuleSourceURL: source})
	}

	// If module URL is missing, return the source as is as it will not match anything in the map.
	if moduleURL == "" {
		return source, nil
	}

	// Before looking up in sourceMap, make sure to drop any query parameters.
	moduleURLParsed, err := url.Parse(moduleURL)
	if err != nil {
		return source, err
	}

	moduleURLParsed.RawQuery = ""
	moduleURLQuery := moduleURLParsed.String()

	// Check if there is an entry to replace the URL portion in the map. Return the source as is if there is no entry in
	// the map.
	sourcePath, hasKey := sourceMap[moduleURLQuery]
	if !hasKey {
		return source, nil
	}

	// Since there is a source mapping, replace the module URL portion with the entry in the map, and join with the
	// subdir.
	// If subdir is missing, check if we can obtain a valid module name from the URL portion.
	if moduleSubdir == "" {
		moduleSubdirFromURL, err := getModulePathFromSourceURL(moduleURL)
		if err != nil {
			return moduleSubdirFromURL, err
		}

		moduleSubdir = moduleSubdirFromURL
	}

	return util.JoinTerraformModulePath(sourcePath, moduleSubdir), nil
}

// GetDefaultConfigPath returns the default path to use for the Terragrunt configuration
// that exists within the path giving preference to `terragrunt.hcl`
func GetDefaultConfigPath(workingDir string) string {
	// check if a configuration file was passed as `workingDir`.
	if !files.IsDir(workingDir) && files.FileExists(workingDir) {
		return workingDir
	}

	var configPath string

	for _, configPath = range DefaultTerragruntConfigPaths {
		if !filepath.IsAbs(configPath) {
			configPath = util.JoinPath(workingDir, configPath)
		}

		if files.FileExists(configPath) {
			break
		}
	}

	return configPath
}

// FindConfigFilesInPath returns a list of all Terragrunt config files in the given path or any subfolder of the path. A file is a Terragrunt
// config file if it has a name as returned by the DefaultConfigPath method
func FindConfigFilesInPath(rootPath string, opts *options.TerragruntOptions) ([]string, error) {
	configFiles := []string{}

	experiment := opts.Experiments[experiment.Symlinks]

	walkFunc := filepath.Walk
	if experiment.Evaluate(opts.ExperimentMode) {
		walkFunc = util.WalkWithSymlinks
	}

	err := walkFunc(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			return nil
		}

		if ok, err := isTerragruntModuleDir(path, opts); err != nil {
			return err
		} else if !ok {
			return filepath.SkipDir
		}

		for _, configFile := range append(DefaultTerragruntConfigPaths, filepath.Base(opts.TerragruntConfigPath)) {
			if !filepath.IsAbs(configFile) {
				configFile = util.JoinPath(path, configFile)
			}

			if !util.IsDir(configFile) && util.FileExists(configFile) {
				configFiles = append(configFiles, configFile)
				break
			}
		}

		return nil
	})

	return configFiles, err
}

// isTerragruntModuleDir returns true if the given path contains a Terragrunt module and false otherwise. The path
// can not contain a cache, data, or download dir.
func isTerragruntModuleDir(path string, terragruntOptions *options.TerragruntOptions) (bool, error) {
	// Skip the Terragrunt cache dir
	if util.ContainsPath(path, util.TerragruntCacheDir) {
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
		return false, nil
	}

	return true, nil
}

// ReadTerragruntConfig reads the Terragrunt config file from its default location
func ReadTerragruntConfig(ctx context.Context, terragruntOptions *options.TerragruntOptions, parserOptions []hclparse.Option) (*TerragruntConfig, error) {
	terragruntOptions.Logger.Debugf("Reading Terragrunt config file at %s", terragruntOptions.TerragruntConfigPath)

	ctx = shell.ContextWithTerraformCommandHook(ctx, nil)
	parcingCtx := NewParsingContext(ctx, terragruntOptions).WithParseOption(parserOptions)

	// TODO: Remove lint ignore
	return ParseConfigFile(parcingCtx, terragruntOptions.TerragruntConfigPath, nil) //nolint:contextcheck
}

// ParseConfigFile parses the Terragrunt config file at the given path. If the include parameter is not nil, then treat this as a config
// included in some other config file when resolving relative paths.
func ParseConfigFile(ctx *ParsingContext, configPath string, includeFromChild *IncludeConfig) (*TerragruntConfig, error) {
	var config *TerragruntConfig

	hclCache := cache.ContextCache[*hclparse.File](ctx, HclCacheContextKey)

	err := telemetry.Telemetry(ctx, ctx.TerragruntOptions, "parse_config_file", map[string]interface{}{
		"config_path": configPath,
		"working_dir": ctx.TerragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		childKey := "nil"
		if includeFromChild != nil {
			childKey = includeFromChild.String()
		}

		decodeListKey := "nil"
		if ctx.PartialParseDecodeList != nil {
			decodeListKey = fmt.Sprintf("%v", ctx.PartialParseDecodeList)
		}

		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		fileInfo, err := os.Stat(configPath)
		if err != nil {
			return err
		}

		var (
			file     *hclparse.File
			cacheKey = fmt.Sprintf("parse-config-%v-%v-%v-%v-%v-%v", configPath, childKey, decodeListKey, ctx.TerragruntOptions.WorkingDir, dir, fileInfo.ModTime().UnixMicro())
		)

		// TODO: Remove lint ignore
		if cacheConfig, found := hclCache.Get(ctx, cacheKey); found { //nolint:contextcheck
			file = cacheConfig
		} else {
			// Parse the HCL file into an AST body that can be decoded multiple times later without having to re-parse
			file, err = hclparse.NewParser(ctx.ParserOptions...).ParseFromFile(configPath)
			if err != nil {
				return err
			}
			// TODO: Remove lint ignore
			hclCache.Put(ctx, cacheKey, file) //nolint:contextcheck
		}

		// TODO: Remove lint ignore
		config, err = ParseConfig(ctx, file, includeFromChild) //nolint:contextcheck
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return config, nil
}

func ParseConfigString(ctx *ParsingContext, configPath string, configString string, includeFromChild *IncludeConfig) (*TerragruntConfig, error) {
	// Parse the HCL file into an AST body that can be decoded multiple times later without having to re-parse
	file, err := hclparse.NewParser(ctx.ParserOptions...).ParseFromString(configString, configPath)
	if err != nil {
		return nil, err
	}

	config, err := ParseConfig(ctx, file, includeFromChild)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// ParseConfig parses the Terragrunt config contained in the given hcl file and merge it with the given include config (if any). Note
// that the config parsing consists of multiple stages so as to allow referencing of data resulting from parsing
// previous config. The parsing order is:
//  1. Parse include. Include is parsed first and is used to import another config. All the config in the include block is
//     then merged into the current TerragruntConfig, except for locals (by design). Note that since the include block is
//     parsed first, you cannot reference locals in the include block config.
//  2. Parse locals. Since locals are parsed next, you can only reference other locals in the locals block. Although it
//     is possible to merge locals from a config imported with an include block, we do not do that here to avoid
//     complicated referencing issues. Please refer to the globals proposal for an alternative that allows merging from
//     included config: https://github.com/gruntwork-io/terragrunt/issues/814
//     Allowed References:
//     - locals
//  3. Parse dependency blocks. This includes running `terragrunt output` to fetch the output data from another
//     terragrunt config, so that it is accessible within the config. See PartialParseConfigString for a way to parse the
//     blocks but avoid decoding.
//     Note that this step is skipped if we already retrieved all the dependencies (which is the case when parsing
//     included config files). This is determined by the dependencyOutputs input parameter.
//     Allowed References:
//     - locals
//  4. Parse everything else. At this point, all the necessary building blocks for parsing the rest of the config are
//     available, so parse the rest of the config.
//     Allowed References:
//     - locals
//     - dependency
//  5. Merge the included config with the parsed config. Note that all the config data is mergeable except for `locals`
//     blocks, which are only scoped to be available within the defining config.
func ParseConfig(ctx *ParsingContext, file *hclparse.File, includeFromChild *IncludeConfig) (*TerragruntConfig, error) {
	ctx = ctx.WithTrackInclude(nil)

	// Initial evaluation of configuration to load flags like IamRole which will be used for final parsing
	// https://github.com/gruntwork-io/terragrunt/issues/667
	if err := setIAMRole(ctx, file, includeFromChild); err != nil {
		return nil, err
	}

	// Decode just the Base blocks. See the function docs for DecodeBaseBlocks for more info on what base blocks are.
	baseBlocks, err := DecodeBaseBlocks(ctx, file, includeFromChild)
	if err != nil {
		return nil, err
	}

	ctx = ctx.WithTrackInclude(baseBlocks.TrackInclude)
	ctx = ctx.WithFeatures(baseBlocks.FeatureFlags)
	ctx = ctx.WithLocals(baseBlocks.Locals)

	if ctx.DecodedDependencies == nil {
		// Decode just the `dependency` blocks, retrieving the outputs from the target terragrunt config in the
		// process.
		retrievedOutputs, err := decodeAndRetrieveOutputs(ctx, file)
		if err != nil {
			return nil, err
		}

		ctx.DecodedDependencies = retrievedOutputs
	}

	evalContext, err := createTerragruntEvalContext(ctx, file.ConfigPath)
	if err != nil {
		return nil, err
	}

	// Decode the rest of the config, passing in this config's `include` block or the child's `include` block, whichever
	// is appropriate
	terragruntConfigFile, err := decodeAsTerragruntConfigFile(ctx, file, evalContext)
	if err != nil {
		return nil, err
	}

	if terragruntConfigFile == nil {
		return nil, errors.New(CouldNotResolveTerragruntConfigInFileError(file.ConfigPath))
	}

	config, err := convertToTerragruntConfig(ctx, file.ConfigPath, terragruntConfigFile)
	if err != nil {
		return nil, err
	}

	// If this file includes another, parse and merge it. Otherwise, just return this config.
	if ctx.TrackInclude != nil {
		mergedConfig, err := handleInclude(ctx, config, false)
		if err != nil {
			return nil, err
		}
		// Saving processed includes into configuration, direct assignment since nested includes aren't supported
		mergedConfig.ProcessedIncludes = ctx.TrackInclude.CurrentMap
		// Make sure the top level information that is not automatically merged in is captured on the merged config to
		// ensure the proper representation of the config is captured.
		// - Locals are deliberately not merged in so that they remain local in scope. Here, we directly set it to the
		//   original locals for the current config being handled, as that is the locals list that is in scope for this
		//   config.
		mergedConfig.Locals = config.Locals
		mergedConfig.Exclude = config.Exclude

		return mergedConfig, nil
	}

	return config, nil
}

// iamRoleCache - store for cached values of IAM roles
var iamRoleCache = cache.NewCache[options.IAMRoleOptions](iamRoleCacheName)

// setIAMRole - extract IAM role details from Terragrunt flags block
func setIAMRole(ctx *ParsingContext, file *hclparse.File, includeFromChild *IncludeConfig) error {
	// Prefer the IAM Role CLI args if they were passed otherwise lazily evaluate the IamRoleOptions using the config.
	if ctx.TerragruntOptions.OriginalIAMRoleOptions.RoleARN != "" {
		ctx.TerragruntOptions.IAMRoleOptions = ctx.TerragruntOptions.OriginalIAMRoleOptions
	} else {
		// as key is considered HCL code and include configuration
		var (
			key           = fmt.Sprintf("%v-%v", file.Content(), includeFromChild)
			config, found = iamRoleCache.Get(ctx, key)
		)

		if !found {
			iamConfig, err := TerragruntConfigFromPartialConfig(ctx.WithDecodeList(TerragruntFlags), file, includeFromChild)
			if err != nil {
				return err
			}

			config = iamConfig.GetIAMRoleOptions()
			iamRoleCache.Put(ctx, key, config)
		}
		// We merge the OriginalIAMRoleOptions into the one from the config, because the CLI passed IAMRoleOptions has
		// precedence.
		ctx.TerragruntOptions.IAMRoleOptions = options.MergeIAMRoleOptions(
			config,
			ctx.TerragruntOptions.OriginalIAMRoleOptions,
		)
	}

	return nil
}

func decodeAsTerragruntConfigFile(ctx *ParsingContext, file *hclparse.File, evalContext *hcl.EvalContext) (*terragruntConfigFile, error) {
	terragruntConfig := terragruntConfigFile{}

	if err := file.Decode(&terragruntConfig, evalContext); err != nil {
		var diagErr hcl.Diagnostics
		// diagErr, ok := errors.Unwrap(err).(hcl.Diagnostics)
		ok := errors.As(err, &diagErr)

		// in case of render-json command and inputs reference error, we update the inputs with default value
		if !ok || !isRenderJSONCommand(ctx) || !isAttributeAccessError(diagErr) {
			return nil, err
		}

		ctx.TerragruntOptions.Logger.Warnf("Failed to decode inputs %v", diagErr)
	}

	if terragruntConfig.Inputs != nil {
		inputs, err := UpdateUnknownCtyValValues(*terragruntConfig.Inputs)
		if err != nil {
			return nil, err
		}

		terragruntConfig.Inputs = &inputs
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

// isAttributeAccessError returns true if the given diagnostics indicate an error accessing an attribute
func isAttributeAccessError(diagnostics hcl.Diagnostics) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == hcl.DiagError && strings.Contains(diagnostic.Summary, "Unsupported attribute") {
			return true
		}
	}

	return false
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
func convertToTerragruntConfig(ctx *ParsingContext, configPath string, terragruntConfigFromFile *terragruntConfigFile) (cfg *TerragruntConfig, err error) {
	if ctx.ConvertToTerragruntConfigFunc != nil {
		return ctx.ConvertToTerragruntConfigFunc(ctx, configPath, terragruntConfigFromFile)
	}

	terragruntConfig := &TerragruntConfig{
		IsPartial: false,
		// Initialize GenerateConfigs so we can append to it
		GenerateConfigs: map[string]codegen.GenerateConfig{},
	}

	defaultMetadata := map[string]interface{}{FoundInFile: configPath}

	if terragruntConfigFromFile.RemoteState != nil {
		remoteState, err := terragruntConfigFromFile.RemoteState.toConfig()
		if err != nil {
			return nil, err
		}

		terragruntConfig.RemoteState = remoteState
		terragruntConfig.SetFieldMetadata(MetadataRemoteState, defaultMetadata)
	}

	if terragruntConfigFromFile.RemoteStateAttr != nil {
		remoteStateMap, err := ParseCtyValueToMap(*terragruntConfigFromFile.RemoteStateAttr)
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

	if err := validateDependencies(ctx, terragruntConfigFromFile.Dependencies); err != nil {
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
		terragruntConfig.Skip = terragruntConfigFromFile.Skip
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

	if terragruntConfigFromFile.IamWebIdentityToken != nil {
		terragruntConfig.IamWebIdentityToken = *terragruntConfigFromFile.IamWebIdentityToken
		terragruntConfig.SetFieldMetadata(MetadataIamWebIdentityToken, defaultMetadata)
	}

	if terragruntConfigFromFile.Engine != nil {
		terragruntConfig.Engine = terragruntConfigFromFile.Engine
		terragruntConfig.SetFieldMetadata(MetadataEngine, defaultMetadata)
	}

	if terragruntConfigFromFile.FeatureFlags != nil {
		terragruntConfig.FeatureFlags = terragruntConfigFromFile.FeatureFlags
		for _, flag := range terragruntConfig.FeatureFlags {
			terragruntConfig.SetFieldMetadataWithType(MetadataFeatureFlag, flag.Name, defaultMetadata)
		}
	}

	if terragruntConfigFromFile.Exclude != nil {
		terragruntConfig.Exclude = terragruntConfigFromFile.Exclude
		terragruntConfig.SetFieldMetadata(MetadataExclude, defaultMetadata)
	}

	if terragruntConfigFromFile.Errors != nil {
		terragruntConfig.Errors = terragruntConfigFromFile.Errors
		terragruntConfig.SetFieldMetadata(MetadataErrors, defaultMetadata)
	}

	generateBlocks := []terragruntGenerateBlock{}
	generateBlocks = append(generateBlocks, terragruntConfigFromFile.GenerateBlocks...)

	if terragruntConfigFromFile.GenerateAttrs != nil {
		generateMap, err := ParseCtyValueToMap(*terragruntConfigFromFile.GenerateAttrs)
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

		if block.IfDisabled == nil {
			block.IfDisabled = &DefaultGenerateBlockIfDisabledValueStr
		}

		ifDisabled, err := codegen.GenerateConfigDisabledFromString(*block.IfDisabled)
		if err != nil {
			return nil, err
		}

		genConfig := codegen.GenerateConfig{
			Path:          block.Path,
			IfExists:      ifExists,
			IfExistsStr:   block.IfExists,
			IfDisabled:    ifDisabled,
			IfDisabledStr: *block.IfDisabled,
			Contents:      block.Contents,
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

		if block.Disable == nil {
			genConfig.Disable = false
		} else {
			genConfig.Disable = *block.Disable
		}

		terragruntConfig.GenerateConfigs[block.Name] = genConfig
		terragruntConfig.SetFieldMetadataWithType(MetadataGenerateConfigs, block.Name, defaultMetadata)
	}

	if terragruntConfigFromFile.Inputs != nil {
		inputs, err := ParseCtyValueToMap(*terragruntConfigFromFile.Inputs)
		if err != nil {
			return nil, err
		}

		terragruntConfig.Inputs = inputs
		terragruntConfig.SetFieldMetadataMap(MetadataInputs, terragruntConfig.Inputs, defaultMetadata)
	}

	if ctx.Locals != nil && *ctx.Locals != cty.NilVal {
		localsParsed, err := ParseCtyValueToMap(*ctx.Locals)
		if err != nil {
			return nil, err
		}

		terragruntConfig.Locals = localsParsed
		terragruntConfig.SetFieldMetadataMap(MetadataLocals, localsParsed, defaultMetadata)
	}

	return terragruntConfig, nil
}

// Iterate over dependencies paths and check if directories exists, return error with all missing dependencies
func validateDependencies(ctx *ParsingContext, dependencies *ModuleDependencies) error {
	var missingDependencies []string

	if dependencies == nil {
		return nil
	}

	for _, dependencyPath := range dependencies.Paths {
		fullPath := filepath.FromSlash(dependencyPath)
		if !filepath.IsAbs(fullPath) {
			fullPath = path.Join(ctx.TerragruntOptions.WorkingDir, fullPath)
		}

		if !util.IsDir(fullPath) {
			missingDependencies = append(missingDependencies, fmt.Sprintf("%s (%s)", dependencyPath, fullPath))
		}
	}

	if len(missingDependencies) > 0 {
		return DependencyDirNotFoundError{missingDependencies}
	}

	return nil
}

// Iterate over generate blocks and detect duplicate names, return error with list of duplicated names
func validateGenerateBlocks(blocks *[]terragruntGenerateBlock) error {
	var (
		blockNames                   = map[string]bool{}
		duplicatedGenerateBlockNames []string
	)

	for _, block := range *blocks {
		_, found := blockNames[block.Name]
		if found {
			duplicatedGenerateBlockNames = append(duplicatedGenerateBlockNames, block.Name)
			continue
		}

		blockNames[block.Name] = true
	}

	if len(duplicatedGenerateBlockNames) != 0 {
		return DuplicatedGenerateBlocksError{duplicatedGenerateBlockNames}
	}

	return nil
}

// configFileHasDependencyBlock statically checks the terrragrunt config file at the given path and checks if it has any
// dependency or dependencies blocks defined. Note that this does not do any decoding of the blocks, as it is only meant
// to check for block presence.
func configFileHasDependencyBlock(configPath string) (bool, error) {
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return false, errors.New(err)
	}

	// We use hclwrite to parse the config instead of the normal parser because the normal parser doesn't give us an AST
	// that we can walk and scan, and requires structured data to map against. This makes the parsing strict, so to
	// avoid weird parsing errors due to missing dependency data, we do a structural scan here.
	hclFile, diags := hclwrite.ParseConfig(configBytes, configPath, hcl.InitialPos)
	if diags.HasErrors() {
		return false, errors.New(diags)
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
func (cfg *TerragruntConfig) SetFieldMetadataWithType(fieldType, fieldName string, m map[string]interface{}) {
	if cfg.FieldsMetadata == nil {
		cfg.FieldsMetadata = map[string]map[string]interface{}{}
	}

	field := fmt.Sprintf("%s-%s", fieldType, fieldName)

	metadata, found := cfg.FieldsMetadata[field]
	if !found {
		metadata = make(map[string]interface{})
	}

	for key, value := range m {
		metadata[key] = value
	}

	cfg.FieldsMetadata[field] = metadata
}

// SetFieldMetadata set metadata on the given field name.
func (cfg *TerragruntConfig) SetFieldMetadata(fieldName string, m map[string]interface{}) {
	cfg.SetFieldMetadataWithType(fieldName, fieldName, m)
}

// SetFieldMetadataMap set metadata on fields from map keys.
// Example usage - setting metadata on all variables from inputs.
func (cfg *TerragruntConfig) SetFieldMetadataMap(field string, data map[string]interface{}, metadata map[string]interface{}) {
	for name := range data {
		cfg.SetFieldMetadataWithType(field, name, metadata)
	}
}

// GetFieldMetadata return field metadata by field name.
func (cfg *TerragruntConfig) GetFieldMetadata(fieldName string) (map[string]string, bool) {
	return cfg.GetMapFieldMetadata(fieldName, fieldName)
}

// GetMapFieldMetadata return field metadata by field type and name.
func (cfg *TerragruntConfig) GetMapFieldMetadata(fieldType, fieldName string) (map[string]string, bool) {
	if cfg.FieldsMetadata == nil {
		return nil, false
	}

	field := fmt.Sprintf("%s-%s", fieldType, fieldName)

	value, found := cfg.FieldsMetadata[field]
	if !found {
		return nil, false
	}

	result := make(map[string]string)
	for key, value := range value {
		result[key] = fmt.Sprintf("%v", value)
	}

	return result, found
}

// EngineOptions fetch engine options
func (cfg *TerragruntConfig) EngineOptions() (*options.EngineOptions, error) {
	if cfg.Engine == nil {
		return nil, nil
	}
	// in case of Meta is null, set empty meta
	meta := map[string]interface{}{}

	if cfg.Engine.Meta != nil {
		parsedMeta, err := ParseCtyValueToMap(*cfg.Engine.Meta)
		if err != nil {
			return nil, err
		}

		meta = parsedMeta
	}

	var version, engineType string
	if cfg.Engine.Version != nil {
		version = *cfg.Engine.Version
	}

	if cfg.Engine.Type != nil {
		engineType = *cfg.Engine.Type
	}
	// if type is null of empty, set to "rpc"
	if len(engineType) == 0 {
		engineType = DefaultEngineType
	}

	return &options.EngineOptions{
		Source:  cfg.Engine.Source,
		Version: version,
		Type:    engineType,
		Meta:    meta,
	}, nil
}

// ErrorsConfig fetch errors configuration for options package
func (cfg *TerragruntConfig) ErrorsConfig() (*options.ErrorsConfig, error) {
	if cfg.Errors == nil {
		return nil, nil
	}

	result := &options.ErrorsConfig{
		Retry:  make(map[string]*options.RetryConfig),
		Ignore: make(map[string]*options.IgnoreConfig),
	}

	for _, retryBlock := range cfg.Errors.Retry {
		if retryBlock == nil {
			continue
		}

		compiledPatterns := make([]*options.ErrorsPattern, 0, len(retryBlock.RetryableErrors))

		for _, pattern := range retryBlock.RetryableErrors {
			value, err := errorsPattern(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid retry pattern %q in block %q: %w",
					pattern, retryBlock.Label, err)
			}

			compiledPatterns = append(compiledPatterns, value)
		}

		result.Retry[retryBlock.Label] = &options.RetryConfig{
			Name:             retryBlock.Label,
			RetryableErrors:  compiledPatterns,
			MaxAttempts:      retryBlock.MaxAttempts,
			SleepIntervalSec: retryBlock.SleepIntervalSec,
		}
	}

	for _, ignoreBlock := range cfg.Errors.Ignore {
		if ignoreBlock == nil {
			continue
		}

		var signals map[string]interface{}

		if ignoreBlock.Signals != nil {
			value, err := convertValuesMapToCtyVal(ignoreBlock.Signals)
			if err != nil {
				return nil, err
			}

			signals, err = ParseCtyValueToMap(value)
			if err != nil {
				return nil, err
			}
		}

		compiledPatterns := make([]*options.ErrorsPattern, 0, len(ignoreBlock.IgnorableErrors))

		for _, pattern := range ignoreBlock.IgnorableErrors {
			value, err := errorsPattern(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid ignore pattern %q in block %q: %w",
					pattern, ignoreBlock.Label, err)
			}

			compiledPatterns = append(compiledPatterns, value)
		}

		result.Ignore[ignoreBlock.Label] = &options.IgnoreConfig{
			Name:            ignoreBlock.Label,
			IgnorableErrors: compiledPatterns,
			Message:         ignoreBlock.Message,
			Signals:         signals,
		}
	}

	return result, nil
}

// Build ErrorsPattern from string
func errorsPattern(pattern string) (*options.ErrorsPattern, error) {
	isNegative := false
	p := pattern

	if len(p) > 0 && p[0] == '!' {
		isNegative = true
		p = p[1:]
	}

	compiled, err := regexp.Compile(p)

	if err != nil {
		return nil, err
	}

	return &options.ErrorsPattern{
		Pattern:  compiled,
		Negative: isNegative,
	}, nil
}
