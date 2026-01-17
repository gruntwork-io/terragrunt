// Package runcfg provides configuration types for running terragrunt commands.
// This package exists to break import cycles between config and run packages.
// The `run` package should only import `runcfg`, never `pkg/config`.
//
// Breaking up the imports this way also allows us to ensure that we never do any config parsing in the `run` package,
// which is slow and needs to be handled carefully.
package runcfg

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// RunConfig contains all configuration data needed to execute terragrunt commands.
// This is the primary configuration struct passed to runner packages.
type RunConfig struct {
	// RemoteState contains remote state backend configuration
	RemoteState remotestate.RemoteState
	// ProcessedIncludes contains processed include configurations
	ProcessedIncludes map[string]IncludeConfig
	// GenerateConfigs contains code generation configurations
	GenerateConfigs map[string]codegen.GenerateConfig
	// Inputs contains input variables to pass to terraform
	Inputs map[string]any
	// Engine contains engine-specific settings
	Engine EngineConfig
	// DownloadDir is the custom download directory for terraform source
	DownloadDir string
	// TerragruntVersionConstraint specifies version constraints for terragrunt
	TerragruntVersionConstraint string
	// TerraformVersionConstraint specifies version constraints for terraform
	TerraformVersionConstraint string
	// TerraformBinary is the path to the terraform/tofu binary
	TerraformBinary string
	// IAMRole contains IAM role options for AWS authentication
	IAMRole options.IAMRoleOptions
	// Errors contains error handling configuration
	Errors ErrorsConfig
	// Dependencies contains paths to dependent modules
	Dependencies ModuleDependencies
	// Terraform contains terraform-specific settings
	Terraform TerraformConfig
	// Exclude contains exclusion rules
	Exclude ExcludeConfig
	// PreventDestroy prevents terraform destroy from running
	PreventDestroy bool
}

// TerraformConfig contains terraform-specific settings.
type TerraformConfig struct {
	// Source is the terraform source URL
	Source string

	// IncludeInCopy lists files to include when copying source
	IncludeInCopy []string

	// ExcludeFromCopy lists files to exclude when copying source
	ExcludeFromCopy []string

	// ExtraArgs contains extra terraform CLI arguments
	ExtraArgs []TerraformExtraArguments

	// BeforeHooks are hooks to run before terraform commands
	BeforeHooks []Hook

	// AfterHooks are hooks to run after terraform commands
	AfterHooks []Hook

	// ErrorHooks are hooks to run on terraform errors
	ErrorHooks []ErrorHook

	// NoCopyTerraformLockFile specifies whether to skip copying the lock file
	// Defaults to false (copy the lock file) when not set
	NoCopyTerraformLockFile bool
}

// Hook represents a lifecycle hook (before/after).
type Hook struct {
	// WorkingDir is the directory to run the hook in
	WorkingDir string
	// Name is the hook identifier
	Name string
	// Commands are terraform commands that trigger this hook
	Commands []string
	// Execute is the command to execute
	Execute []string
	// RunOnError specifies whether to run on error
	RunOnError bool
	// If is a condition for running the hook
	If bool
	// SuppressStdout suppresses stdout output
	SuppressStdout bool
}

// ErrorHook represents an error handling hook.
type ErrorHook struct {
	// WorkingDir is the directory to run the hook in
	WorkingDir string
	// Name is the hook identifier
	Name string
	// Commands are terraform commands that trigger this hook
	Commands []string
	// Execute is the command to execute
	Execute []string
	// OnErrors are error patterns that trigger this hook
	OnErrors []string
	// SuppressStdout suppresses stdout output
	SuppressStdout bool
}

// TerraformExtraArguments represents extra CLI arguments for terraform.
type TerraformExtraArguments struct {
	// Arguments are the extra CLI arguments
	Arguments []string
	// RequiredVarFiles are required variable files
	RequiredVarFiles []string
	// OptionalVarFiles are optional variable files
	OptionalVarFiles []string
	// EnvVars are environment variables to set
	EnvVars map[string]string
	// Name is the identifier for this set of arguments
	Name string
	// VarFiles contains the computed list of variable files (required + existing optional files)
	// This is computed during config translation.
	VarFiles []string
	// Commands are terraform commands these arguments apply to
	Commands []string
}

// ExcludeConfig contains exclusion rules.
type ExcludeConfig struct {
	// Actions are the actions to exclude
	Actions []string
	// If is the condition for exclusion
	If bool
	// NoRun specifies whether to skip running
	NoRun bool
	// ExcludeDependencies specifies whether to exclude dependencies
	ExcludeDependencies bool
}

// IsActionListed checks if the action is listed in the exclude block.
func (e *ExcludeConfig) IsActionListed(action string) bool {
	return IsActionListedInExclude(e.Actions, action)
}

// ShouldPreventRun returns true if execution should be prevented.
func (e *ExcludeConfig) ShouldPreventRun(command string) bool {
	return ShouldPreventRunBasedOnExclude(e.Actions, &e.NoRun, e.If, command)
}

// IncludeConfig represents an included configuration.
type IncludeConfig struct {
	// MergeStrategy specifies how to merge the include
	MergeStrategy string
	// Name is the include name/label
	Name string
	// Path is the path to the included config
	Path string
	// Expose specifies whether to expose the include
	Expose bool
}

// ModuleDependencies represents paths to dependent modules.
type ModuleDependencies struct {
	// Paths are the paths to dependent modules
	Paths []string
}

// EngineConfig represents engine-specific configuration.
type EngineConfig struct {
	// Version is the engine version
	Version string
	// Type is the engine type
	Type string
	// Meta contains engine metadata
	Meta *cty.Value
	// Source is the engine source URL
	Source string
	// Enable indicates whether the engine block was specified,
	// meaning that we should be using the engine.
	Enable bool
}

// ErrorsConfig represents the top-level errors configuration.
type ErrorsConfig struct {
	// Retry contains retry block configurations
	Retry []*RetryBlock
	// Ignore contains ignore block configurations
	Ignore []*IgnoreBlock
}

// RetryBlock represents a labeled retry block.
type RetryBlock struct {
	// Label is the name of the retry block
	Label string
	// RetryableErrors are error patterns that trigger retry
	RetryableErrors []string
	// MaxAttempts is the maximum number of retry attempts
	MaxAttempts int
	// SleepIntervalSec is the sleep interval between retries in seconds
	SleepIntervalSec int
}

// IgnoreBlock represents a labeled ignore block.
type IgnoreBlock struct {
	// Signals contains signal mappings
	Signals map[string]cty.Value
	// Label is the name of the ignore block
	Label string
	// Message is an optional message for ignored errors
	Message string
	// IgnorableErrors are error patterns that should be ignored
	IgnorableErrors []string
}
