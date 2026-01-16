// Package runcfg provides configuration types for running terragrunt commands.
// This package exists to break import cycles between config and run packages.
// The `run` package should only import `runcfg`, never `pkg/config`.
//
// Breaking up the imports this way also allows us to ensure that we never do any config parsing in the `run` package,
// which is slow and needs to be handled carefully.
package runcfg

import (
	"slices"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// RunConfig contains all configuration data needed to execute terragrunt commands.
// This is the primary configuration struct passed to runner packages.
type RunConfig struct {
	// PreventDestroy prevents terraform destroy from running
	PreventDestroy *bool
	// RemoteState contains remote state backend configuration
	RemoteState *remotestate.RemoteState
	// Exclude contains exclusion rules
	Exclude *ExcludeConfig
	// GenerateConfigs contains code generation configurations
	GenerateConfigs map[string]codegen.GenerateConfig
	// Inputs contains input variables to pass to terraform
	Inputs map[string]any
	// Dependencies contains paths to dependent modules
	Dependencies *ModuleDependencies
	// Terraform contains terraform-specific settings
	Terraform *TerraformConfig
	// Engine contains engine-specific settings
	Engine *EngineConfig
	// Errors contains error handling configuration
	Errors *ErrorsConfig
	// ProcessedIncludes contains processed include configurations
	ProcessedIncludes map[string]IncludeConfig
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
}

// TerraformConfig contains terraform-specific settings.
type TerraformConfig struct {
	// Source is the terraform source URL
	Source *string

	// IncludeInCopy lists files to include when copying source
	IncludeInCopy *[]string

	// ExcludeFromCopy lists files to exclude when copying source
	ExcludeFromCopy *[]string

	// CopyTerraformLockFile specifies whether to copy the lock file
	CopyTerraformLockFile *bool

	// ExtraArgs contains extra terraform CLI arguments
	ExtraArgs []TerraformExtraArguments

	// BeforeHooks are hooks to run before terraform commands
	BeforeHooks []Hook

	// AfterHooks are hooks to run after terraform commands
	AfterHooks []Hook

	// ErrorHooks are hooks to run on terraform errors
	ErrorHooks []ErrorHook
}

// GetBeforeHooks returns the before hooks, or nil if TerraformConfig is nil.
func (cfg *TerraformConfig) GetBeforeHooks() []Hook {
	if cfg == nil {
		return nil
	}

	return cfg.BeforeHooks
}

// GetAfterHooks returns the after hooks, or nil if TerraformConfig is nil.
func (cfg *TerraformConfig) GetAfterHooks() []Hook {
	if cfg == nil {
		return nil
	}

	return cfg.AfterHooks
}

// GetErrorHooks returns the error hooks, or nil if TerraformConfig is nil.
func (cfg *TerraformConfig) GetErrorHooks() []ErrorHook {
	if cfg == nil {
		return nil
	}

	return cfg.ErrorHooks
}

// Hook represents a lifecycle hook (before/after).
type Hook struct {
	// WorkingDir is the directory to run the hook in
	WorkingDir *string
	// RunOnError specifies whether to run on error
	RunOnError *bool
	// If is a condition for running the hook
	If *bool
	// SuppressStdout suppresses stdout output
	SuppressStdout *bool
	// Name is the hook identifier
	Name string
	// Commands are terraform commands that trigger this hook
	Commands []string
	// Execute is the command to execute
	Execute []string
}

// ErrorHook represents an error handling hook.
type ErrorHook struct {
	// WorkingDir is the directory to run the hook in
	WorkingDir *string
	// SuppressStdout suppresses stdout output
	SuppressStdout *bool
	// Name is the hook identifier
	Name string
	// Commands are terraform commands that trigger this hook
	Commands []string
	// Execute is the command to execute
	Execute []string
	// OnErrors are error patterns that trigger this hook
	OnErrors []string
}

// TerraformExtraArguments represents extra CLI arguments for terraform.
type TerraformExtraArguments struct {
	// Arguments are the extra CLI arguments
	Arguments *[]string
	// RequiredVarFiles are required variable files
	RequiredVarFiles *[]string
	// OptionalVarFiles are optional variable files
	OptionalVarFiles *[]string
	// EnvVars are environment variables to set
	EnvVars *map[string]string
	// Name is the identifier for this set of arguments
	Name string
	// Commands are terraform commands these arguments apply to
	Commands []string
}

// GetVarFiles returns a list of variable files, including required and optional files.
func (args *TerraformExtraArguments) GetVarFiles(l log.Logger) []string {
	var varFiles []string

	// Include all specified RequiredVarFiles.
	if args.RequiredVarFiles != nil {
		varFiles = append(varFiles, util.RemoveDuplicatesKeepLast(*args.RequiredVarFiles)...)
	}

	// If OptionalVarFiles is specified, check for each file if it exists and if so, include in the var
	// files list. Note that it is possible that many files resolve to the same path, so we remove
	// duplicates.
	if args.OptionalVarFiles != nil {
		for _, file := range util.RemoveDuplicatesKeepLast(*args.OptionalVarFiles) {
			if util.FileExists(file) {
				varFiles = append(varFiles, file)
			} else {
				l.Debugf("Skipping var-file %s as it does not exist", file)
			}
		}
	}

	return varFiles
}

// ExcludeConfig contains exclusion rules.
type ExcludeConfig struct {
	// ExcludeDependencies specifies whether to exclude dependencies
	ExcludeDependencies *bool
	// NoRun specifies whether to skip running
	NoRun *bool
	// Actions are the actions to exclude
	Actions []string
	// If is the condition for exclusion
	If bool
}

// IsActionListed checks if the action is listed in the exclude block.
func (e *ExcludeConfig) IsActionListed(action string) bool {
	if e == nil || len(e.Actions) == 0 {
		return false
	}

	const (
		allActions              = "all"
		allExcludeOutputActions = "all_except_output"
		tgOutput                = "output"
	)

	actionLower := strings.ToLower(action)

	for _, checkAction := range e.Actions {
		if checkAction == allActions {
			return true
		}

		if checkAction == allExcludeOutputActions && actionLower != tgOutput {
			return true
		}

		if strings.ToLower(checkAction) == actionLower {
			return true
		}
	}

	return false
}

// ShouldPreventRun returns true if execution should be prevented.
func (e *ExcludeConfig) ShouldPreventRun(command string) bool {
	if e == nil {
		return false
	}

	if !e.If {
		return false
	}

	if e.NoRun != nil && *e.NoRun {
		return e.IsActionListed(command)
	}

	return slices.Contains(e.Actions, command)
}

// IncludeConfig represents an included configuration.
type IncludeConfig struct {
	// Expose specifies whether to expose the include
	Expose *bool
	// MergeStrategy specifies how to merge the include
	MergeStrategy *string
	// Name is the include name/label
	Name string
	// Path is the path to the included config
	Path string
}

// ModuleDependencies represents paths to dependent modules.
type ModuleDependencies struct {
	// Paths are the paths to dependent modules
	Paths []string
}

// EngineConfig represents engine-specific configuration.
type EngineConfig struct {
	// Version is the engine version
	Version *string
	// Type is the engine type
	Type *string
	// Meta contains engine metadata
	Meta *cty.Value
	// Source is the engine source URL
	Source string
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
