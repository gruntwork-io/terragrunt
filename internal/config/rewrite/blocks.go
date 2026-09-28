package config

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
)

// CatalogHCL is a catalog block.
type CatalogHCL struct {
	NoShell         *bool    `hcl:"no_shell,optional"`
	NoHooks         *bool    `hcl:"no_hooks,optional"`
	DefaultTemplate string   `hcl:"default_template,optional"`
	URLs            []string `hcl:"urls,attr"`
}

// EngineHCL is an engine block.
type EngineHCL struct {
	Remain  hcl.Body `hcl:",remain"`
	Version *string  `hcl:"version,attr"`
	Type    *string  `hcl:"type,attr"`
	Source  string   `hcl:"source,attr"`
}

// EngineBody is decoded from [EngineHCL.Remain].
type EngineBody struct {
	Meta *cty.Value `hcl:"meta,attr"`
}

// TerraformHCL is a terraform block.
type TerraformHCL struct {
	Source                *string             `hcl:"source,attr"`
	Version               *string             `hcl:"version,attr"`
	UpdateSourceWithCAS   *bool               `hcl:"update_source_with_cas,attr"`
	Mutable               *bool               `hcl:"mutable,attr"`
	IncludeInCopy         *[]string           `hcl:"include_in_copy,attr"`
	ExcludeFromCopy       *[]string           `hcl:"exclude_from_copy,attr"`
	CopyTerraformLockFile *bool               `hcl:"copy_terraform_lock_file,attr"`
	ExtraArgs             []ExtraArgumentsHCL `hcl:"extra_arguments,block"`
	BeforeHooks           []HookHCL           `hcl:"before_hook,block"`
	AfterHooks            []HookHCL           `hcl:"after_hook,block"`
	ErrorHooks            []ErrorHookHCL      `hcl:"error_hook,block"`
}

// ExtraArgumentsHCL is an extra_arguments block.
type ExtraArgumentsHCL struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:"name,label"`
}

// ExtraArgumentsBody is decoded from [ExtraArgumentsHCL.Remain].
type ExtraArgumentsBody struct {
	Arguments        *[]string          `hcl:"arguments,attr"`
	RequiredVarFiles *[]string          `hcl:"required_var_files,attr"`
	OptionalVarFiles *[]string          `hcl:"optional_var_files,attr"`
	EnvVars          *map[string]string `hcl:"env_vars,attr"`
	Commands         []string           `hcl:"commands,attr"`
}

// HookHCL is a before_hook or after_hook block.
type HookHCL struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:"name,label"`
}

// HookBody is decoded from [HookHCL.Remain].
type HookBody struct {
	If             *bool    `hcl:"if,attr"`
	RunOnError     *bool    `hcl:"run_on_error,attr"`
	SuppressStdout *bool    `hcl:"suppress_stdout,attr"`
	WorkingDir     *string  `hcl:"working_dir,attr"`
	Commands       []string `hcl:"commands,attr"`
	Execute        []string `hcl:"execute,attr"`
}

// ErrorHookHCL is an error_hook block.
type ErrorHookHCL struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:"name,label"`
}

// ErrorHookBody is decoded from [ErrorHookHCL.Remain].
type ErrorHookBody struct {
	SuppressStdout *bool    `hcl:"suppress_stdout,attr"`
	WorkingDir     *string  `hcl:"working_dir,attr"`
	Commands       []string `hcl:"commands,attr"`
	Execute        []string `hcl:"execute,attr"`
	OnErrors       []string `hcl:"on_errors,attr"`
}

// RemoteStateHCL is a remote_state block.
type RemoteStateHCL struct {
	Remain                        hcl.Body                        `hcl:",remain"`
	DisableInit                   *bool                           `hcl:"disable_init,attr"`
	DisableDependencyOptimization *bool                           `hcl:"disable_dependency_optimization,attr"`
	Generate                      *remotestate.ConfigFileGenerate `hcl:"generate,attr"`
	BackendName                   string                          `hcl:"backend,attr"`
}

// RemoteStateBody is decoded from [RemoteStateHCL.Remain].
type RemoteStateBody struct {
	BackendConfig cty.Value  `hcl:"config,attr"`
	Encryption    *cty.Value `hcl:"encryption,attr"`
}

// ModuleDependenciesHCL is a dependencies block.
type ModuleDependenciesHCL struct {
	Paths []string `hcl:"paths,attr"`
}

// ExcludeHCL is an exclude block.
type ExcludeHCL struct {
	Remain hcl.Body `hcl:",remain"`
}

// ExcludeBody is decoded from [ExcludeHCL.Remain].
type ExcludeBody struct {
	ExcludeDependencies *bool    `hcl:"exclude_dependencies,attr"`
	NoRun               *bool    `hcl:"no_run,attr"`
	Actions             []string `hcl:"actions,attr"`
	If                  bool     `hcl:"if,attr"`
}

// ErrorsHCL is an errors block.
type ErrorsHCL struct {
	Retry  []RetryHCL  `hcl:"retry,block"`
	Ignore []IgnoreHCL `hcl:"ignore,block"`
}

// RetryHCL is a retry block.
type RetryHCL struct {
	Remain hcl.Body `hcl:",remain"`
	Label  string   `hcl:"name,label"`
}

// RetryBody is decoded from [RetryHCL.Remain].
type RetryBody struct {
	RetryableErrors  []string `hcl:"retryable_errors,attr"`
	MaxAttempts      int      `hcl:"max_attempts,attr"`
	SleepIntervalSec int      `hcl:"sleep_interval_sec,attr"`
}

// IgnoreHCL is an ignore block.
type IgnoreHCL struct {
	Remain hcl.Body `hcl:",remain"`
	Label  string   `hcl:"name,label"`
}

// IgnoreBody is decoded from [IgnoreHCL.Remain].
type IgnoreBody struct {
	Signals         map[string]cty.Value `hcl:"signals,optional"`
	Message         string               `hcl:"message,optional"`
	IgnorableErrors []string             `hcl:"ignorable_errors,attr"`
}

// FeatureFlagHCL is a feature block.
type FeatureFlagHCL struct {
	Default *cty.Value `hcl:"default,attr"`
	Name    string     `hcl:",label"`
}

// GenerateHCL is a generate block.
type GenerateHCL struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:",label"`
}

// GenerateBody is decoded from [GenerateHCL.Remain].
type GenerateBody struct {
	IfDisabled       *string `hcl:"if_disabled,attr"`
	CommentPrefix    *string `hcl:"comment_prefix,attr"`
	DisableSignature *bool   `hcl:"disable_signature,attr"`
	Disable          *bool   `hcl:"disable,attr"`
	HclFmt           *bool   `hcl:"hcl_fmt,attr"`
	Mutable          *bool   `hcl:"mutable,attr"`
	Path             string  `hcl:"path,attr"`
	IfExists         string  `hcl:"if_exists,attr"`
	Contents         string  `hcl:"contents,attr"`
}

// DependencyHCL is a dependency block. pkg/config decodes its body into [UnitConfig.DependencyConfigs].
type DependencyHCL struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:",label"`
}

// IncludeHCL is an include block. pkg/config decodes its body into [UnitConfig.Includes].
type IncludeHCL struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:"name,label"`
}

// LocalsHCL is a locals block. pkg/config evaluates its body into [UnitConfig.Locals].
type LocalsHCL struct {
	Remain hcl.Body `hcl:",remain"`
}
