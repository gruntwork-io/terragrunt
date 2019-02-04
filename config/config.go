package config

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/gruntwork-io/terragrunt/errors"
    "github.com/gruntwork-io/terragrunt/options"
    "github.com/gruntwork-io/terragrunt/remote"
    "github.com/gruntwork-io/terragrunt/util"
    "github.com/hashicorp/hcl"
)

const DefaultTerragruntConfigPath = "terraform.tfvars"
const OldTerragruntConfigPath = ".terragrunt"

// TerragruntConfig represents a parsed and expanded configuration
type TerragruntConfig struct {
    Terraform      *TerraformConfig
    RemoteState    *remote.RemoteState
    Dependencies   *ModuleDependencies
    PreventDestroy bool
    IamRole        string
}

func (conf *TerragruntConfig) String() string {
    return fmt.Sprintf("TerragruntConfig{Terraform = %v, RemoteState = %v, Dependencies = %v, PreventDestroy = %v}", conf.Terraform, conf.RemoteState, conf.Dependencies, conf.PreventDestroy)
}

// terragruntConfigFile represents the configuration supported in a Terragrunt configuration file (i.e.
// terraform.tfvars or .terragrunt)
type terragruntConfigFile struct {
    Terraform      *TerraformConfig    `hcl:"terraform,omitempty"`
    Include        *IncludeConfig      `hcl:"include,omitempty"`
    Lock           *LockConfig         `hcl:"lock,omitempty"`
    RemoteState    *remote.RemoteState `hcl:"remote_state,omitempty"`
    Dependencies   *ModuleDependencies `hcl:"dependencies,omitempty"`
    PreventDestroy bool                `hcl:"prevent_destroy,omitempty"`
    IamRole        string              `hcl:"iam_role"`
}

// Older versions of Terraform did not support locking, so Terragrunt offered locking as a feature. As of version 0.9.0,
// Terraform supports locking natively, so this feature was removed from Terragrunt. However, we keep around the
// LockConfig so we can log a warning for Terragrunt users who are still trying to use it.
type LockConfig map[interface{}]interface{}

// tfvarsFileWithTerragruntConfig represents a .tfvars file that contains a terragrunt = { ... } block
type tfvarsFileWithTerragruntConfig struct {
    Terragrunt *terragruntConfigFile `hcl:"terragrunt,omitempty"`
}

// IncludeConfig represents the configuration settings for a parent Terragrunt configuration file that you can
// "include" in a child Terragrunt configuration file
type IncludeConfig struct {
    Path string `hcl:"path"`
}

// ModuleDependencies represents the paths to other Terraform modules that must be applied before the current module
// can be applied
type ModuleDependencies struct {
    Paths []string `hcl:"paths"`
}

func (deps *ModuleDependencies) String() string {
    return fmt.Sprintf("ModuleDependencies{Paths = %v}", deps.Paths)
}

type LoadEnvironmentVariables struct {
    Execute   []string `hcl:"execute,omitempty"`
    Format	  string   `hcl:"fomat,omitempty"`
    Transient bool     `hcl:"transient,omitempty"`
    Overwrite bool     `hcl:"transient,omitempty"`
}

// Hook specifies terraform commands (apply/plan) and array of os commands to execute
type Hook struct {
    Name        string   				  `hcl:",key"`
    Commands    []string 				  `hcl:"commands,omitempty"`
    Execute     []string 				  `hcl:"execute,omitempty"`
    LoadEnvVars *LoadEnvironmentVariables `hcl:"load_env_vars,omitempty"`
    RunOnError  bool     				  `hcl:"run_on_error,omitempty"`
}

func (conf *Hook) String() string {
    return fmt.Sprintf("Hook{Name = %s, Commands = %v}", conf.Name, len(conf.Commands))
}

// TerraformConfig specifies where to find the Terraform configuration files
type TerraformConfig struct {
    ExtraArgs   []TerraformExtraArguments `hcl:"extra_arguments"`
    Source      string                    `hcl:"source"`
    BeforeHooks []Hook                    `hcl:"before_hook"`
    AfterHooks  []Hook                    `hcl:"after_hook"`
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
        if curHook.LoadEnvVars != nil  {
            if len(curHook.LoadEnvVars.Execute) < 1 || curHook.LoadEnvVars.Execute[0] == "" {
                return InvalidArgError(fmt.Sprintf("Error with hook %s. Need at least one non-empty argument in 'execute' for 'load_env_vars'.", curHook.Name))
            }
        }
    }

    return nil
}

// TerraformExtraArguments sets a list of arguments to pass to Terraform if command fits any in the `Commands` list
type TerraformExtraArguments struct {
    Name             string            `hcl:",key"`
    Arguments        []string          `hcl:"arguments,omitempty"`
    RequiredVarFiles []string          `hcl:"required_var_files,omitempty"`
    OptionalVarFiles []string          `hcl:"optional_var_files,omitempty"`
    Commands         []string          `hcl:"commands,omitempty"`
    EnvVars          map[string]string `hcl:"env_vars,omitempty"`
}

func (conf *TerraformExtraArguments) String() string {
    return fmt.Sprintf(
        "TerraformArguments{Name = %s, Arguments = %v, Commands = %v, EnvVars = %v}",
        conf.Name,
        conf.Arguments,
        conf.Commands,
        conf.EnvVars)
}

// Return the default path to use for the Terragrunt configuration file. The reason this is a method rather than a
// constant is that older versions of Terragrunt stored configuration in a different file. This method returns the
// path to the old configuration format if such a file exists and the new format otherwise.
func DefaultConfigPath(workingDir string) string {
    path := util.JoinPath(workingDir, OldTerragruntConfigPath)
    if util.FileExists(path) {
        return path
    }
    return util.JoinPath(workingDir, DefaultTerragruntConfigPath)
}

// Returns a list of all Terragrunt config files in the given path or any subfolder of the path. A file is a Terragrunt
// config file if it has a name as returned by the DefaultConfigPath method and contains Terragrunt config contents
// as returned by the IsTerragruntConfigFile method.
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
// contains a Terragrunt module if it contains a Terragrunt configuration file (terraform.tfvars) and is not a cache
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

    return IsTerragruntConfigFile(DefaultConfigPath(path))
}

// Returns true if the given path corresponds to file that could be a Terragrunt config file. A file could be a
// Terragrunt config file if:
//
// 1. The file exists
// 2. It is a .terragrunt file, which is the old Terragrunt-specific file format
// 3. The file contains HCL contents with a terragrunt = { ... } block
func IsTerragruntConfigFile(path string) (bool, error) {
    if !util.FileExists(path) {
        return false, nil
    }

    if isOldTerragruntConfig(path) {
        return true, nil
    }

    return isNewTerragruntConfig(path)
}

// Returns true if the given path points to an old Terragrunt config file
func isOldTerragruntConfig(path string) bool {
    return strings.HasSuffix(path, OldTerragruntConfigPath)
}

// Retrusn true if the given path points to a new (current) Terragrunt config file
func isNewTerragruntConfig(path string) (bool, error) {
    configContents, err := util.ReadFileAsString(path)
    if err != nil {
        return false, err
    }

    containsBlock, err := containsTerragruntBlock(configContents)
    if err != nil {
        return false, errors.WithStackTrace(ErrorParsingTerragruntConfig{ConfigPath: path, Underlying: err})
    }

    return containsBlock, nil
}

// Returns true if the given string contains valid HCL with a terragrunt = { ... } block
func containsTerragruntBlock(configString string) (bool, error) {
    terragruntConfig := &tfvarsFileWithTerragruntConfig{}
    if err := hcl.Decode(terragruntConfig, configString); err != nil {
        return false, errors.WithStackTrace(err)
    }
    return terragruntConfig.Terragrunt != nil, nil
}

// Read the Terragrunt config file from its default location
func ReadTerragruntConfig(terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
    terragruntOptions.Logger.Printf("Reading Terragrunt config file at %s", terragruntOptions.TerragruntConfigPath)
    return ParseConfigFile(terragruntOptions.TerragruntConfigPath, terragruntOptions, nil)
}

// Parse the Terragrunt config file at the given path. If the include parameter is not nil, then treat this as a config
// included in some other config file when resolving relative paths.
func ParseConfigFile(configPath string, terragruntOptions *options.TerragruntOptions, include *IncludeConfig) (*TerragruntConfig, error) {
    if isOldTerragruntConfig(configPath) {
        terragruntOptions.Logger.Printf("DEPRECATION WARNING: Found deprecated config file format %s. This old config format will not be supported in the future. Please move your config files into a %s file.", configPath, DefaultTerragruntConfigPath)
    }

    configString, err := util.ReadFileAsString(configPath)
    if err != nil {
        return nil, err
    }

    config, err := parseConfigString(configString, terragruntOptions, include, configPath)
    if err != nil {
        return nil, err
    }

    return config, nil
}

// Parse the Terragrunt config contained in the given string.
func parseConfigString(configString string, terragruntOptions *options.TerragruntOptions, include *IncludeConfig, configPath string) (*TerragruntConfig, error) {
    resolvedConfigString, err := ResolveTerragruntConfigString(configString, include, terragruntOptions)
    if err != nil {
        return nil, err
    }

    terragruntConfigFile, err := parseConfigStringAsTerragruntConfigFile(resolvedConfigString, configPath)
    if err != nil {
        return nil, err
    }
    if terragruntConfigFile == nil {
        return nil, errors.WithStackTrace(CouldNotResolveTerragruntConfigInFile(configPath))
    }

    config, err := convertToTerragruntConfig(terragruntConfigFile, terragruntOptions)
    if err != nil {
        return nil, err
    }

    if include != nil && terragruntConfigFile.Include != nil {
        return nil, errors.WithStackTrace(TooManyLevelsOfInheritance{
            ConfigPath:             terragruntOptions.TerragruntConfigPath,
            FirstLevelIncludePath:  include.Path,
            SecondLevelIncludePath: terragruntConfigFile.Include.Path,
        })
    }

    includedConfig, err := parseIncludedConfig(terragruntConfigFile.Include, terragruntOptions)
    if err != nil {
        return nil, err
    }

    return mergeConfigWithIncludedConfig(config, includedConfig, terragruntOptions)
}

// Parse the given config string, read from the given config file, as a terragruntConfigFile struct. This method solely
// converts the HCL syntax in the string to the terragruntConfigFile struct; it does not process any interpolations.
func parseConfigStringAsTerragruntConfigFile(configString string, configPath string) (*terragruntConfigFile, error) {
    if isOldTerragruntConfig(configPath) {
        terragruntConfig := &terragruntConfigFile{}
        if err := hcl.Decode(terragruntConfig, configString); err != nil {
            return nil, errors.WithStackTrace(err)
        }
        return terragruntConfig, nil
    } else {
        tfvarsConfig := &tfvarsFileWithTerragruntConfig{}
        if err := hcl.Decode(tfvarsConfig, configString); err != nil {
            return nil, errors.WithStackTrace(err)
        }

        return tfvarsConfig.Terragrunt, nil
    }
}

// Merge the given config with an included config. Anything specified in the current config will override the contents
// of the included config. If the included config is nil, just return the current config.
func mergeConfigWithIncludedConfig(config *TerragruntConfig, includedConfig *TerragruntConfig, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
    if includedConfig == nil {
        return config, nil
    }

    if config.RemoteState != nil {
        includedConfig.RemoteState = config.RemoteState
    }
    if config.PreventDestroy {
        includedConfig.PreventDestroy = config.PreventDestroy
    }

    if config.Terraform != nil {
        if includedConfig.Terraform == nil {
            includedConfig.Terraform = config.Terraform
        } else {
            if config.Terraform.Source != "" {
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
    if includedConfig == nil {
        return nil, nil
    }
    if includedConfig.Path == "" {
        return nil, errors.WithStackTrace(IncludedConfigMissingPath(terragruntOptions.TerragruntConfigPath))
    }

    resolvedIncludePath, err := ResolveTerragruntConfigString(includedConfig.Path, nil, terragruntOptions)
    if err != nil {
        return nil, err
    }

    if !filepath.IsAbs(resolvedIncludePath) {
        resolvedIncludePath = util.JoinPath(filepath.Dir(terragruntOptions.TerragruntConfigPath), resolvedIncludePath)
    }

    return ParseConfigFile(resolvedIncludePath, terragruntOptions, includedConfig)
}

// Convert the contents of a fully resolved Terragrunt configuration to a TerragruntConfig object
func convertToTerragruntConfig(terragruntConfigFromFile *terragruntConfigFile, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
    terragruntConfig := &TerragruntConfig{}

    if terragruntConfigFromFile.Lock != nil {
        terragruntOptions.Logger.Printf("WARNING: Found a lock configuration in the Terraform configuration at %s. Terraform added native support for locking as of version 0.9.0, so this feature has been removed from Terragrunt and will have no effect. See your Terraform backend docs for how to configure locking: https://www.terraform.io/docs/backends/types/index.html.", terragruntOptions.TerragruntConfigPath)
    }

    if terragruntConfigFromFile.RemoteState != nil {
        terragruntConfigFromFile.RemoteState.FillDefaults()
        if err := terragruntConfigFromFile.RemoteState.Validate(); err != nil {
            return nil, err
        }

        terragruntConfig.RemoteState = terragruntConfigFromFile.RemoteState
    }

    if err := terragruntConfigFromFile.Terraform.ValidateHooks(); err != nil {
        return nil, err
    }

    terragruntConfig.Terraform = terragruntConfigFromFile.Terraform
    terragruntConfig.Dependencies = terragruntConfigFromFile.Dependencies
    terragruntConfig.PreventDestroy = terragruntConfigFromFile.PreventDestroy
    terragruntConfig.IamRole = terragruntConfigFromFile.IamRole

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
