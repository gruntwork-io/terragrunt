package config

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
)

// UnitBody is decoded from [UnitConfig.Remain].
type UnitBody struct {
	// Inputs is the inputs attribute.
	Inputs *cty.Value `hcl:"inputs,attr"`
	// GenerateAttrs is the generate attribute.
	GenerateAttrs *cty.Value `hcl:"generate,optional"`
	// RemoteStateAttr is [UnitConfig.RemoteStateAttr] evaluated.
	RemoteStateAttr *cty.Value
	// Generate is the generate blocks, in source order.
	Generate []Generate `hcl:"generate,block"`
}

// Catalog is a catalog block.
type Catalog struct {
	NoShell         *bool    `hcl:"no_shell,optional"`
	NoHooks         *bool    `hcl:"no_hooks,optional"`
	DefaultTemplate string   `hcl:"default_template,optional"`
	URLs            []string `hcl:"urls,attr"`
}

// Engine is an engine block.
type Engine struct {
	Remain  hcl.Body `hcl:",remain"`
	Version *string  `hcl:"version,attr"`
	Type    *string  `hcl:"type,attr"`
	Body    EngineBody
	Source  string `hcl:"source,attr"`
}

// EngineBody is decoded from [Engine.Remain].
type EngineBody struct {
	Meta *cty.Value `hcl:"meta,attr"`
}

// Terraform is a terraform block.
type Terraform struct {
	Source                *string          `hcl:"source,attr"`
	Version               *string          `hcl:"version,attr"`
	UpdateSourceWithCAS   *bool            `hcl:"update_source_with_cas,attr"`
	Mutable               *bool            `hcl:"mutable,attr"`
	IncludeInCopy         *[]string        `hcl:"include_in_copy,attr"`
	ExcludeFromCopy       *[]string        `hcl:"exclude_from_copy,attr"`
	CopyTerraformLockFile *bool            `hcl:"copy_terraform_lock_file,attr"`
	ExtraArgs             []ExtraArguments `hcl:"extra_arguments,block"`
	BeforeHooks           []Hook           `hcl:"before_hook,block"`
	AfterHooks            []Hook           `hcl:"after_hook,block"`
	ErrorHooks            []ErrorHook      `hcl:"error_hook,block"`
}

// ExtraArguments is an extra_arguments block.
type ExtraArguments struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:"name,label"`
	Body   ExtraArgumentsBody
}

// ExtraArgumentsBody is decoded from [ExtraArguments.Remain].
type ExtraArgumentsBody struct {
	Arguments        *[]string          `hcl:"arguments,attr"`
	RequiredVarFiles *[]string          `hcl:"required_var_files,attr"`
	OptionalVarFiles *[]string          `hcl:"optional_var_files,attr"`
	EnvVars          *map[string]string `hcl:"env_vars,attr"`
	Commands         []string           `hcl:"commands,attr"`
}

// Hook is a before_hook or after_hook block.
type Hook struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:"name,label"`
	Body   HookBody
}

// HookBody is decoded from [Hook.Remain].
type HookBody struct {
	If             *bool    `hcl:"if,attr"`
	RunOnError     *bool    `hcl:"run_on_error,attr"`
	SuppressStdout *bool    `hcl:"suppress_stdout,attr"`
	WorkingDir     *string  `hcl:"working_dir,attr"`
	Commands       []string `hcl:"commands,attr"`
	Execute        []string `hcl:"execute,attr"`
}

// ErrorHook is an error_hook block.
type ErrorHook struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:"name,label"`
	Body   ErrorHookBody
}

// ErrorHookBody is decoded from [ErrorHook.Remain].
type ErrorHookBody struct {
	SuppressStdout *bool    `hcl:"suppress_stdout,attr"`
	WorkingDir     *string  `hcl:"working_dir,attr"`
	Commands       []string `hcl:"commands,attr"`
	Execute        []string `hcl:"execute,attr"`
	OnErrors       []string `hcl:"on_errors,attr"`
}

// RemoteState is a remote_state block.
type RemoteState struct {
	Remain                        hcl.Body                        `hcl:",remain"`
	DisableInit                   *bool                           `hcl:"disable_init,attr"`
	DisableDependencyOptimization *bool                           `hcl:"disable_dependency_optimization,attr"`
	Generate                      *remotestate.ConfigFileGenerate `hcl:"generate,attr"`
	Body                          RemoteStateBody
	BackendName                   string `hcl:"backend,attr"`
}

// RemoteStateBody is decoded from [RemoteState.Remain].
type RemoteStateBody struct {
	BackendConfig cty.Value  `hcl:"config,attr"`
	Encryption    *cty.Value `hcl:"encryption,attr"`
}

// ModuleDependencies is a dependencies block.
type ModuleDependencies struct {
	Paths []string `hcl:"paths,attr"`
}

// Exclude is an exclude block.
type Exclude struct {
	ExcludeDependencies *bool    `hcl:"exclude_dependencies,attr"`
	NoRun               *bool    `hcl:"no_run,attr"`
	Actions             []string `hcl:"actions,attr"`
	If                  bool     `hcl:"if,attr"`
}

// Errors is an errors block.
type Errors struct {
	Retry  []Retry  `hcl:"retry,block"`
	Ignore []Ignore `hcl:"ignore,block"`
}

// Retry is a retry block.
type Retry struct {
	Remain hcl.Body `hcl:",remain"`
	Label  string   `hcl:"name,label"`
	Body   RetryBody
}

// RetryBody is decoded from [Retry.Remain].
type RetryBody struct {
	RetryableErrors  []string `hcl:"retryable_errors,attr"`
	MaxAttempts      int      `hcl:"max_attempts,attr"`
	SleepIntervalSec int      `hcl:"sleep_interval_sec,attr"`
}

// Ignore is an ignore block.
type Ignore struct {
	Remain hcl.Body `hcl:",remain"`
	Label  string   `hcl:"name,label"`
	Body   IgnoreBody
}

// IgnoreBody is decoded from [Ignore.Remain].
type IgnoreBody struct {
	Signals         map[string]cty.Value `hcl:"signals,optional"`
	Message         string               `hcl:"message,optional"`
	IgnorableErrors []string             `hcl:"ignorable_errors,attr"`
}

// FeatureFlag is a feature block.
type FeatureFlag struct {
	Default *cty.Value `hcl:"default,attr"`
	Name    string     `hcl:",label"`
}

// Generate is a generate block.
type Generate struct {
	IfDisabled       *string `hcl:"if_disabled,attr"`
	CommentPrefix    *string `hcl:"comment_prefix,attr"`
	DisableSignature *bool   `hcl:"disable_signature,attr"`
	Disable          *bool   `hcl:"disable,attr"`
	HclFmt           *bool   `hcl:"hcl_fmt,attr"`
	Mutable          *bool   `hcl:"mutable,attr"`
	Name             string  `hcl:",label"`
	Path             string  `hcl:"path,attr"`
	IfExists         string  `hcl:"if_exists,attr"`
	Contents         string  `hcl:"contents,attr"`
}

// Dependency is a dependency block. pkg/config decodes its body into [UnitConfig.DependencyConfigs].
type Dependency struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:",label"`
}

// Include is an include block. pkg/config decodes its body into [UnitConfig.Includes].
type Include struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:"name,label"`
}

// Locals is a locals block. pkg/config evaluates its body into [UnitConfig.Locals].
type Locals struct {
	Remain hcl.Body `hcl:",remain"`
}
