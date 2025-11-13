// Package config provides functionality for parsing Terragrunt configuration files.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/writer"
	"github.com/gruntwork-io/terragrunt/tf"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/mitchellh/mapstructure"

	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"maps"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	DefaultTerragruntConfigPath     = "terragrunt.hcl"
	DefaultStackFile                = "terragrunt.stack.hcl"
	DefaultTerragruntJSONConfigPath = "terragrunt.hcl.json"
	RecommendedParentConfigName     = "root.hcl"

	FoundInFile = "found_in_file"

	iamRoleCacheName = "iamRoleCache"

	logMsgSeparator = "\n"

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
	MetadataDependentModules            = "dependent_modules"
	MetadataInclude                     = "include"
	MetadataFeatureFlag                 = "feature"
	MetadataExclude                     = "exclude"
	MetadataErrors                      = "errors"
	MetadataRetry                       = "retry"
	MetadataIgnore                      = "ignore"
	MetadataValues                      = "values"
	MetadataStack                       = "stack"
	MetadataUnit                        = "unit"
)

var (
	// Order matters, for example if none of the files are found `GetDefaultConfigPath` func returns the last element.
	DefaultTerragruntConfigPaths = []string{
		DefaultTerragruntJSONConfigPath,
		DefaultTerragruntConfigPath,
	}

	DefaultParserOptions = func(l log.Logger, opts *options.TerragruntOptions) []hclparse.Option {
		writer := writer.New(
			writer.WithLogger(l),
			writer.WithDefaultLevel(log.ErrorLevel),
			writer.WithMsgSeparator(logMsgSeparator),
		)

		parseOpts := []hclparse.Option{
			hclparse.WithDiagnosticsWriter(writer, l.Formatter().DisabledColors()),
			hclparse.WithLogger(l),
		}

		strictControl := opts.StrictControls.Find(controls.BareInclude)

		// If we can't find the strict control, we're probably in a test
		// where the option is being hand written. In that case,
		// we'll assume we're not in strict mode.
		if strictControl != nil {
			strictControl.SuppressWarning()

			if err := strictControl.Evaluate(context.Background()); err != nil {
				return parseOpts
			}
		}

		parseOpts = append(parseOpts, hclparse.WithFileUpdate(updateBareIncludeBlock))

		return parseOpts
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
	Locals                      map[string]any
	ProcessedIncludes           IncludeConfigsMap
	FieldsMetadata              map[string]map[string]any
	Terraform                   *TerraformConfig
	Errors                      *ErrorsConfig
	RemoteState                 *remotestate.RemoteState
	Dependencies                *ModuleDependencies
	Exclude                     *ExcludeConfig
	PreventDestroy              *bool
	GenerateConfigs             map[string]codegen.GenerateConfig
	IamAssumeRoleDuration       *int64
	Inputs                      map[string]any
	Engine                      *EngineConfig
	Catalog                     *CatalogConfig
	IamWebIdentityToken         string
	IamAssumeRoleSessionName    string
	IamRole                     string
	DownloadDir                 string
	TerragruntVersionConstraint string
	TerraformVersionConstraint  string
	TerraformBinary             string
	TerragruntDependencies      Dependencies
	FeatureFlags                FeatureFlags
	DependentModulesPath        []*string
	IsPartial                   bool
}

func (cfg *TerragruntConfig) GetRemoteState(l log.Logger, opts *options.TerragruntOptions) (*remotestate.RemoteState, error) {
	if cfg.RemoteState == nil {
		l.Debug("Did not find remote `remote_state` block in the config")

		return nil, nil
	}

	sourceURL, err := GetTerraformSourceURL(opts, cfg)
	if err != nil {
		return nil, err
	}

	if sourceURL != "" {
		walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)

		tfSource, err := tf.NewSource(l, sourceURL, opts.DownloadDir, opts.WorkingDir, walkWithSymlinks)
		if err != nil {
			return nil, err
		}

		opts.WorkingDir = tfSource.WorkingDir
	}

	return cfg.RemoteState, nil
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

// WriteTo writes the terragrunt config to a writer
func (cfg *TerragruntConfig) WriteTo(w io.Writer) (int64, error) {
	cfgAsCty, err := TerragruntConfigAsCty(cfg)
	if err != nil {
		return 0, err
	}

	f := hclwrite.NewFile()
	rootBody := f.Body()

	// Handle blocks first
	if len(cfg.Locals) > 0 {
		localsBlock := hclwrite.NewBlock("locals", nil)
		localsBody := localsBlock.Body()

		localsAsCty := cfgAsCty.GetAttr("locals")

		for k := range cfg.Locals {
			localsBody.SetAttributeValue(k, localsAsCty.GetAttr(k))
		}

		rootBody.AppendBlock(localsBlock)
	}

	if cfg.Terraform != nil {
		terraformBlock := hclwrite.NewBlock("terraform", nil)
		terraformBody := terraformBlock.Body()
		terraformAsCty := cfgAsCty.GetAttr("terraform")

		// Handle source
		if cfg.Terraform.Source != nil {
			terraformBody.SetAttributeValue("source", terraformAsCty.GetAttr("source"))
		}

		// Handle extra_arguments blocks
		if len(cfg.Terraform.ExtraArgs) > 0 {
			extraArgsAsCty := terraformAsCty.GetAttr("extra_arguments").AsValueMap()

			for _, arg := range cfg.Terraform.ExtraArgs {
				extraArgBlock := hclwrite.NewBlock("extra_arguments", []string{arg.Name})
				extraArgBody := extraArgBlock.Body()
				argCty := extraArgsAsCty[arg.Name]

				if arg.Commands != nil {
					extraArgBody.SetAttributeValue("commands", argCty.GetAttr("commands"))
				}

				if arg.Arguments != nil {
					extraArgBody.SetAttributeValue("arguments", argCty.GetAttr("arguments"))
				}

				if arg.RequiredVarFiles != nil {
					extraArgBody.SetAttributeValue("required_var_files", argCty.GetAttr("required_var_files"))
				}

				if arg.OptionalVarFiles != nil {
					extraArgBody.SetAttributeValue("optional_var_files", argCty.GetAttr("optional_var_files"))
				}

				if arg.EnvVars != nil {
					extraArgBody.SetAttributeValue("env_vars", argCty.GetAttr("env_vars"))
				}

				terraformBody.AppendBlock(extraArgBlock)
			}
		}

		// Handle hooks
		for _, beforeHook := range cfg.Terraform.BeforeHooks { //nolint:dupl
			beforeHookBlock := hclwrite.NewBlock("before_hook", []string{beforeHook.Name})
			beforeHookBody := beforeHookBlock.Body()

			beforeHookAsCty := terraformAsCty.GetAttr("before_hook").AsValueMap()[beforeHook.Name]

			if beforeHook.If != nil {
				beforeHookBody.SetAttributeValue("if", beforeHookAsCty.GetAttr("if"))
			}

			if beforeHook.RunOnError != nil {
				beforeHookBody.SetAttributeValue("run_on_error", beforeHookAsCty.GetAttr("run_on_error"))
			}

			beforeHookBody.SetAttributeValue("commands", beforeHookAsCty.GetAttr("commands"))
			beforeHookBody.SetAttributeValue("execute", beforeHookAsCty.GetAttr("execute"))

			if beforeHook.WorkingDir != nil {
				beforeHookBody.SetAttributeValue("working_dir", beforeHookAsCty.GetAttr("working_dir"))
			}

			terraformBody.AppendBlock(beforeHookBlock)
		}

		for _, afterHook := range cfg.Terraform.AfterHooks { //nolint:dupl
			afterHookBlock := hclwrite.NewBlock("after_hook", []string{afterHook.Name})
			afterHookBody := afterHookBlock.Body()

			afterHookAsCty := terraformAsCty.GetAttr("after_hook").AsValueMap()[afterHook.Name]

			if afterHook.If != nil {
				afterHookBody.SetAttributeValue("if", afterHookAsCty.GetAttr("if"))
			}

			if afterHook.RunOnError != nil {
				afterHookBody.SetAttributeValue("run_on_error", afterHookAsCty.GetAttr("run_on_error"))
			}

			afterHookBody.SetAttributeValue("commands", afterHookAsCty.GetAttr("commands"))
			afterHookBody.SetAttributeValue("execute", afterHookAsCty.GetAttr("execute"))

			if afterHook.WorkingDir != nil {
				afterHookBody.SetAttributeValue("working_dir", afterHookAsCty.GetAttr("working_dir"))
			}

			terraformBody.AppendBlock(afterHookBlock)
		}

		for _, errorHook := range cfg.Terraform.ErrorHooks {
			errorHookBlock := hclwrite.NewBlock("error_hook", []string{errorHook.Name})
			errorHookBody := errorHookBlock.Body()

			errorHookAsCty := terraformAsCty.GetAttr("error_hook").AsValueMap()[errorHook.Name]

			errorHookBody.SetAttributeValue("commands", errorHookAsCty.GetAttr("commands"))
			errorHookBody.SetAttributeValue("execute", errorHookAsCty.GetAttr("execute"))
			errorHookBody.SetAttributeValue("on_errors", errorHookAsCty.GetAttr("on_errors"))

			if errorHook.WorkingDir != nil {
				errorHookBody.SetAttributeValue("working_dir", errorHookAsCty.GetAttr("working_dir"))
			}

			terraformBody.AppendBlock(errorHookBlock)
		}

		rootBody.AppendBlock(terraformBlock)
	}

	if cfg.RemoteState != nil {
		remoteStateBlock := hclwrite.NewBlock("remote_state", nil)
		remoteStateBody := remoteStateBlock.Body()
		remoteStateAsCty := cfgAsCty.GetAttr("remote_state")

		remoteStateBody.SetAttributeValue("backend", remoteStateAsCty.GetAttr("backend"))

		if cfg.RemoteState.DisableInit {
			remoteStateBody.SetAttributeValue("disable_init", remoteStateAsCty.GetAttr("disable_init"))
		}

		if cfg.RemoteState.DisableDependencyOptimization {
			remoteStateBody.SetAttributeValue("disable_dependency_optimization", remoteStateAsCty.GetAttr("disable_dependency_optimization"))
		}

		if cfg.RemoteState.BackendConfig != nil {
			remoteStateBody.SetAttributeValue("config", remoteStateAsCty.GetAttr("config"))
		}

		rootBody.AppendBlock(remoteStateBlock)
	}

	if cfg.Dependencies != nil && len(cfg.Dependencies.Paths) > 0 {
		dependenciesBlock := hclwrite.NewBlock("dependencies", nil)
		dependenciesBody := dependenciesBlock.Body()

		dependenciesAsCty := cfgAsCty.GetAttr("dependencies")

		dependenciesBody.SetAttributeValue("paths", dependenciesAsCty.GetAttr("paths"))
		rootBody.AppendBlock(dependenciesBlock)
	}

	// Handle dependency blocks
	for _, dep := range cfg.TerragruntDependencies {
		depBlock := hclwrite.NewBlock("dependency", []string{dep.Name})
		depBody := depBlock.Body()
		depAsCty := cfgAsCty.GetAttr("dependency").GetAttr(dep.Name)
		depBody.SetAttributeValue("config_path", depAsCty.GetAttr("config_path"))

		if dep.Enabled != nil {
			depBody.SetAttributeValue("enabled", goboolToCty(*dep.Enabled))
		}

		if dep.SkipOutputs != nil {
			depBody.SetAttributeValue("skip_outputs", goboolToCty(*dep.SkipOutputs))
		}

		if dep.MockOutputs != nil {
			depBody.SetAttributeValue("mock_outputs", depAsCty.GetAttr("mock_outputs"))
		}

		if dep.MockOutputsAllowedTerraformCommands != nil {
			depBody.SetAttributeValue("mock_outputs_allowed_terraform_commands", depAsCty.GetAttr("mock_outputs_allowed_terraform_commands"))
		}

		if dep.MockOutputsMergeStrategyWithState != nil {
			depBody.SetAttributeValue("mock_outputs_merge_strategy_with_state", depAsCty.GetAttr("mock_outputs_merge_strategy_with_state"))
		}

		rootBody.AppendBlock(depBlock)
	}

	// Handle generate blocks
	for name, gen := range cfg.GenerateConfigs {
		genBlock := hclwrite.NewBlock("generate", []string{name})
		genBody := genBlock.Body()
		genBody.SetAttributeValue("path", gostringToCty(gen.Path))
		genBody.SetAttributeValue("if_exists", gostringToCty(gen.IfExistsStr))
		genBody.SetAttributeValue("if_disabled", gostringToCty(gen.IfDisabledStr))
		genBody.SetAttributeValue("contents", gostringToCty(gen.Contents))

		if gen.CommentPrefix != codegen.DefaultCommentPrefix {
			genBody.SetAttributeValue("comment_prefix", gostringToCty(gen.CommentPrefix))
		}

		if gen.DisableSignature {
			genBody.SetAttributeValue("disable_signature", goboolToCty(gen.DisableSignature))
		}

		if gen.Disable {
			genBody.SetAttributeValue("disable", goboolToCty(gen.Disable))
		}

		rootBody.AppendBlock(genBlock)
	}

	// Handle feature flags
	for _, flag := range cfg.FeatureFlags {
		flagBlock := hclwrite.NewBlock("feature", []string{flag.Name})
		flagBody := flagBlock.Body()
		flagAsCty := cfgAsCty.GetAttr("feature").GetAttr(flag.Name)

		if flag.Default != nil {
			flagBody.SetAttributeValue("default", flagAsCty.GetAttr("default"))
		}

		rootBody.AppendBlock(flagBlock)
	}

	// Handle engine block
	if cfg.Engine != nil {
		engineBlock := hclwrite.NewBlock("engine", nil)
		engineBody := engineBlock.Body()
		engineAsCty := cfgAsCty.GetAttr("engine")

		if cfg.Engine.Source != "" {
			engineBody.SetAttributeValue("source", engineAsCty.GetAttr("source"))
		}

		if cfg.Engine.Version != nil {
			engineBody.SetAttributeValue("version", engineAsCty.GetAttr("version"))
		}

		if cfg.Engine.Type != nil {
			engineBody.SetAttributeValue("type", engineAsCty.GetAttr("type"))
		}

		if cfg.Engine.Meta != nil {
			engineBody.SetAttributeValue("meta", engineAsCty.GetAttr("meta"))
		}

		rootBody.AppendBlock(engineBlock)
	}

	// Handle exclude block
	if cfg.Exclude != nil {
		excludeBlock := hclwrite.NewBlock("exclude", nil)
		excludeBody := excludeBlock.Body()
		excludeAsCty := cfgAsCty.GetAttr("exclude")

		if cfg.Exclude.ExcludeDependencies != nil {
			excludeBody.SetAttributeValue("exclude_dependencies", excludeAsCty.GetAttr("exclude_dependencies"))
		}

		if len(cfg.Exclude.Actions) > 0 {
			excludeBody.SetAttributeValue("actions", excludeAsCty.GetAttr("actions"))
		}

		if cfg.Exclude.NoRun != nil {
			excludeBody.SetAttributeValue("no_run", excludeAsCty.GetAttr("no_run"))
		}

		excludeBody.SetAttributeValue("if", excludeAsCty.GetAttr("if"))

		rootBody.AppendBlock(excludeBlock)
	}

	// Handle errors block
	if cfg.Errors != nil {
		errorsBlock := hclwrite.NewBlock("errors", nil)
		errorsBody := errorsBlock.Body()

		// Handle retry blocks
		if len(cfg.Errors.Retry) > 0 {
			for _, retryConfig := range cfg.Errors.Retry {
				retryBlock := hclwrite.NewBlock("retry", []string{retryConfig.Label})
				retryBody := retryBlock.Body()

				if retryConfig.MaxAttempts > 0 {
					retryBody.SetAttributeValue("max_attempts", cty.NumberIntVal(int64(retryConfig.MaxAttempts)))
				}

				if retryConfig.SleepIntervalSec > 0 {
					retryBody.SetAttributeValue("sleep_interval_sec", cty.NumberIntVal(int64(retryConfig.SleepIntervalSec)))
				}

				if len(retryConfig.RetryableErrors) > 0 {
					retryableErrors := make([]cty.Value, len(retryConfig.RetryableErrors))

					for i, err := range retryConfig.RetryableErrors {
						retryableErrors[i] = cty.StringVal(err)
					}

					retryBody.SetAttributeValue("retryable_errors", cty.ListVal(retryableErrors))
				}

				errorsBody.AppendBlock(retryBlock)
			}
		}

		// Handle ignore blocks
		if len(cfg.Errors.Ignore) > 0 {
			for _, ignoreConfig := range cfg.Errors.Ignore {
				ignoreBlock := hclwrite.NewBlock("ignore", []string{ignoreConfig.Label})
				ignoreBody := ignoreBlock.Body()

				if len(ignoreConfig.IgnorableErrors) > 0 {
					ignorableErrors := make([]cty.Value, len(ignoreConfig.IgnorableErrors))

					for i, err := range ignoreConfig.IgnorableErrors {
						ignorableErrors[i] = cty.StringVal(err)
					}

					ignoreBody.SetAttributeValue("ignorable_errors", cty.ListVal(ignorableErrors))
				}

				if ignoreConfig.Message != "" {
					ignoreBody.SetAttributeValue("message", cty.StringVal(ignoreConfig.Message))
				}

				if ignoreConfig.Signals != nil {
					ignoreBody.SetAttributeValue("signals", cty.MapVal(ignoreConfig.Signals))
				}

				errorsBody.AppendBlock(ignoreBlock)
			}
		}

		rootBody.AppendBlock(errorsBlock)
	}

	// Handle catalog block
	if cfg.Catalog != nil {
		catalogBlock := hclwrite.NewBlock("catalog", nil)
		catalogBody := catalogBlock.Body()
		catalogAsCty := cfgAsCty.GetAttr("catalog")

		if cfg.Catalog.DefaultTemplate != "" {
			catalogBody.SetAttributeValue("default_template", catalogAsCty.GetAttr("default_template"))
		}

		if len(cfg.Catalog.URLs) > 0 {
			catalogBody.SetAttributeValue("urls", catalogAsCty.GetAttr("urls"))
		}

		if cfg.Catalog.NoShell != nil {
			catalogBody.SetAttributeValue("no_shell", catalogAsCty.GetAttr("no_shell"))
		}

		if cfg.Catalog.NoHooks != nil {
			catalogBody.SetAttributeValue("no_hooks", catalogAsCty.GetAttr("no_hooks"))
		}

		rootBody.AppendBlock(catalogBlock)
	}

	// Handle attributes
	if cfg.TerraformBinary != "" {
		rootBody.SetAttributeValue("terraform_binary", cfgAsCty.GetAttr("terraform_binary"))
	}

	if cfg.TerraformVersionConstraint != "" {
		rootBody.SetAttributeValue("terraform_version_constraint", cfgAsCty.GetAttr("terraform_version_constraint"))
	}

	if cfg.TerragruntVersionConstraint != "" {
		rootBody.SetAttributeValue("terragrunt_version_constraint", cfgAsCty.GetAttr("terragrunt_version_constraint"))
	}

	if cfg.DownloadDir != "" {
		rootBody.SetAttributeValue("download_dir", cfgAsCty.GetAttr("download_dir"))
	}

	if cfg.PreventDestroy != nil {
		rootBody.SetAttributeValue("prevent_destroy", cfgAsCty.GetAttr("prevent_destroy"))
	}

	if cfg.IamRole != "" {
		rootBody.SetAttributeValue("iam_role", cfgAsCty.GetAttr("iam_role"))
	}

	if cfg.IamAssumeRoleDuration != nil {
		rootBody.SetAttributeValue("iam_assume_role_duration", cfgAsCty.GetAttr("iam_assume_role_duration"))
	}

	if cfg.IamAssumeRoleSessionName != "" {
		rootBody.SetAttributeValue("iam_assume_role_session_name", cfgAsCty.GetAttr("iam_assume_role_session_name"))
	}

	if len(cfg.Inputs) > 0 {
		rootBody.SetAttributeValue("inputs", cfgAsCty.GetAttr("inputs"))
	}

	return f.WriteTo(w)
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
	RemoteState     *remotestate.ConfigFile `hcl:"remote_state,block"`
	RemoteStateAttr *cty.Value              `hcl:"remote_state,optional"`

	Dependencies             *ModuleDependencies `hcl:"dependencies,block"`
	DownloadDir              *string             `hcl:"download_dir,attr"`
	PreventDestroy           *bool               `hcl:"prevent_destroy,attr"`
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
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:"name,label"`
}

// Struct used to parse generate blocks. This will later be converted to GenerateConfig structs so that we can go
// through the codegen routine.
type terragruntGenerateBlock struct {
	IfDisabled       *string `hcl:"if_disabled,attr" mapstructure:"if_disabled"`
	CommentPrefix    *string `hcl:"comment_prefix,attr" mapstructure:"comment_prefix"`
	DisableSignature *bool   `hcl:"disable_signature,attr" mapstructure:"disable_signature"`
	Disable          *bool   `hcl:"disable,attr" mapstructure:"disable"`
	Name             string  `hcl:",label" mapstructure:",omitempty"`
	Path             string  `hcl:"path,attr" mapstructure:"path"`
	IfExists         string  `hcl:"if_exists,attr" mapstructure:"if_exists"`
	Contents         string  `hcl:"contents,attr" mapstructure:"contents"`
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
	Expose        *bool   `hcl:"expose,attr"`
	MergeStrategy *string `hcl:"merge_strategy,attr"`
	Name          string  `hcl:"name,label"`
	Path          string  `hcl:"path,attr"`
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
	If             *bool    `hcl:"if,attr" cty:"if"`
	RunOnError     *bool    `hcl:"run_on_error,attr" cty:"run_on_error"`
	SuppressStdout *bool    `hcl:"suppress_stdout,attr" cty:"suppress_stdout"`
	WorkingDir     *string  `hcl:"working_dir,attr" cty:"working_dir"`
	Name           string   `hcl:"name,label" cty:"name"`
	Commands       []string `hcl:"commands,attr" cty:"commands"`
	Execute        []string `hcl:"execute,attr" cty:"execute"`
}

type ErrorHook struct {
	SuppressStdout *bool    `hcl:"suppress_stdout,attr" cty:"suppress_stdout"`
	WorkingDir     *string  `hcl:"working_dir,attr" cty:"working_dir"`
	Name           string   `hcl:"name,label" cty:"name"`
	Commands       []string `hcl:"commands,attr" cty:"commands"`
	Execute        []string `hcl:"execute,attr" cty:"execute"`
	OnErrors       []string `hcl:"on_errors,attr" cty:"on_errors"`
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
	Source *string `hcl:"source,attr"`

	// Ideally we can avoid the pointer to list slice, but if it is not a pointer, Terraform requires the attribute to
	// be defined and we want to make this optional.
	IncludeInCopy   *[]string `hcl:"include_in_copy,attr"`
	ExcludeFromCopy *[]string `hcl:"exclude_from_copy,attr"`

	CopyTerraformLockFile *bool                     `hcl:"copy_terraform_lock_file,attr"`
	ExtraArgs             []TerraformExtraArguments `hcl:"extra_arguments,block"`
	BeforeHooks           []Hook                    `hcl:"before_hook,block"`
	AfterHooks            []Hook                    `hcl:"after_hook,block"`
	ErrorHooks            []ErrorHook               `hcl:"error_hook,block"`
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
	Arguments        *[]string          `hcl:"arguments,attr" cty:"arguments"`
	RequiredVarFiles *[]string          `hcl:"required_var_files,attr" cty:"required_var_files"`
	OptionalVarFiles *[]string          `hcl:"optional_var_files,attr" cty:"optional_var_files"`
	EnvVars          *map[string]string `hcl:"env_vars,attr" cty:"env_vars"`
	Name             string             `hcl:"name,label" cty:"name"`
	Commands         []string           `hcl:"commands,attr" cty:"commands"`
}

func (args *TerraformExtraArguments) String() string {
	return fmt.Sprintf(
		"TerraformArguments{Name = %s, Arguments = %v, Commands = %v, EnvVars = %v}",
		args.Name,
		args.Arguments,
		args.Commands,
		args.EnvVars)
}

func (args *TerraformExtraArguments) GetVarFiles(l log.Logger) []string {
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
				l.Debugf("Skipping var-file %s as it does not exist", file)
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

	walkFunc := filepath.WalkDir

	if opts.Experiments.Evaluate(experiment.Symlinks) {
		walkFunc = util.WalkDirWithSymlinks
	}

	err := walkFunc(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
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
func ReadTerragruntConfig(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, parserOptions []hclparse.Option) (*TerragruntConfig, error) {
	l.Debugf("Reading Terragrunt config file at %s", terragruntOptions.TerragruntConfigPath)

	ctx = tf.ContextWithTerraformCommandHook(ctx, nil)
	parsingCtx := NewParsingContext(ctx, l, terragruntOptions).WithParseOption(parserOptions)

	// TODO: Remove lint ignore
	return ParseConfigFile(parsingCtx, l, terragruntOptions.TerragruntConfigPath, nil) //nolint:contextcheck
}

// ParseConfigFile parses the Terragrunt config file at the given path. If the include parameter is not nil, then treat this as a config
// included in some other config file when resolving relative paths.
func ParseConfigFile(ctx *ParsingContext, l log.Logger, configPath string, includeFromChild *IncludeConfig) (*TerragruntConfig, error) {
	var config *TerragruntConfig

	hclCache := cache.ContextCache[*hclparse.File](ctx, HclCacheContextKey)

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "parse_config_file", map[string]any{
		"config_path": configPath,
		"working_dir": ctx.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		childKey := "nil"
		if includeFromChild != nil {
			childKey = includeFromChild.String()
		}

		decodeListKey := "nil"
		if ctx.PartialParseDecodeList != nil {
			decodeListKey = fmt.Sprintf("%v", ctx.PartialParseDecodeList)
		}

		fileInfo, err := os.Stat(configPath)
		if err != nil {
			if os.IsNotExist(err) {
				return TerragruntConfigNotFoundError{Path: configPath}
			}

			return errors.Errorf("failed to get file info: %w", err)
		}

		cacheKey := fmt.Sprintf("%v-%v-%v-%v-%v",
			configPath,
			ctx.TerragruntOptions.WorkingDir,
			childKey,
			decodeListKey,
			fileInfo.ModTime().UnixMicro(),
		)

		var file *hclparse.File

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
		config, err = ParseConfig(ctx, l, file, includeFromChild) //nolint:contextcheck
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return config, err
	}

	return config, nil
}

func ParseConfigString(ctx *ParsingContext, l log.Logger, configPath string, configString string, includeFromChild *IncludeConfig) (*TerragruntConfig, error) {
	// Parse the HCL file into an AST body that can be decoded multiple times later without having to re-parse
	file, err := hclparse.NewParser(ctx.ParserOptions...).ParseFromString(configString, configPath)
	if err != nil {
		return nil, err
	}

	config, err := ParseConfig(ctx, l, file, includeFromChild)
	if err != nil {
		return config, err
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
func ParseConfig(
	ctx *ParsingContext,
	l log.Logger,
	file *hclparse.File,
	includeFromChild *IncludeConfig,
) (*TerragruntConfig, error) {
	errs := &errors.MultiError{}

	if err := DetectDeprecatedConfigurations(ctx, l, file); err != nil {
		return nil, err
	}

	ctx = ctx.WithTrackInclude(nil)

	// Initial evaluation of configuration to load flags like IamRole which will be used for final parsing
	// https://github.com/gruntwork-io/terragrunt/issues/667
	if err := setIAMRole(ctx, l, file, includeFromChild); err != nil {
		errs = errs.Append(err)
	}

	// read unit files and add to context
	unitValues, err := ReadValues(ctx.Context, l, ctx.TerragruntOptions, filepath.Dir(file.ConfigPath))
	if err != nil {
		return nil, err
	}

	ctx = ctx.WithValues(unitValues)

	// Decode just the Base blocks. See the function docs for DecodeBaseBlocks for more info on what base blocks are.
	baseBlocks, err := DecodeBaseBlocks(ctx, l, file, includeFromChild)
	if err != nil {
		errs = errs.Append(err)
	}

	if baseBlocks != nil {
		ctx = ctx.WithTrackInclude(baseBlocks.TrackInclude)
		ctx = ctx.WithFeatures(baseBlocks.FeatureFlags)
		ctx = ctx.WithLocals(baseBlocks.Locals)
	}

	if !ctx.SkipOutputsResolution && ctx.DecodedDependencies == nil {
		// Decode just the `dependency` blocks, retrieving the outputs from the target terragrunt config in the
		// process.
		retrievedOutputs, err := decodeAndRetrieveOutputs(ctx, l, file)
		if err != nil {
			errs = errs.Append(err)
		}

		ctx.DecodedDependencies = retrievedOutputs
	}

	evalContext, err := createTerragruntEvalContext(ctx, l, file.ConfigPath)
	if err != nil {
		errs = errs.Append(err)
	}

	// Decode the rest of the config, passing in this config's `include` block or the child's `include` block, whichever
	// is appropriate
	terragruntConfigFile, err := decodeAsTerragruntConfigFile(ctx, l, file, evalContext)
	if err != nil {
		errs = errs.Append(err)
	}

	if terragruntConfigFile == nil {
		return nil, errors.New(CouldNotResolveTerragruntConfigInFileError(file.ConfigPath))
	}

	config, err := convertToTerragruntConfig(ctx, file.ConfigPath, terragruntConfigFile)
	if err != nil {
		errs = errs.Append(err)
	}

	// If this file includes another, parse and merge it. Otherwise, just return this config.
	// If there have been errors during this parse, don't attempt to parse the included config.
	if ctx.TrackInclude != nil {
		mergedConfig, err := handleInclude(ctx, l, config, false)
		if err != nil {
			errs = errs.Append(err)
			return config, errs.ErrorOrNil()
		}

		// We should never get a nil config here, so if we do, return the config we've been able to parse so far
		// and return any errors that have occurred so far to avoid a nil pointer dereference below.
		if mergedConfig == nil {
			return config, errs.ErrorOrNil()
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

		return mergedConfig, errs.ErrorOrNil()
	}

	return config, errs.ErrorOrNil()
}

// DetectDeprecatedConfigurations detects if deprecated configurations are used in the given HCL file.
func DetectDeprecatedConfigurations(ctx *ParsingContext, l log.Logger, file *hclparse.File) error {
	if DetectInputsCtyUsage(file) {
		// Dependency inputs (dependency.foo.inputs.bar) are now blocked by default for performance.
		// This deprecated feature causes significant performance overhead due to recursive parsing.
		return errors.New("Reading inputs from dependencies is no longer supported. To acquire values from dependencies, use outputs (dependency.foo.outputs.bar) instead.")
	}

	if detectBareIncludeUsage(file) {
		allControls := ctx.TerragruntOptions.StrictControls

		bareInclude := allControls.Find(controls.BareInclude)
		if bareInclude == nil {
			return errors.New("failed to find control " + controls.BareInclude)
		}

		evalCtx := log.ContextWithLogger(ctx, l)
		if err := bareInclude.Evaluate(evalCtx); err != nil {
			return err
		}
	}

	return nil
}

// DetectInputsCtyUsage detects if an identifier matching dependency.foo.inputs.bar is used in the given HCL file.
//
// This is deprecated functionality, so we look for this to determine if we should throw an error or warning.
func DetectInputsCtyUsage(file *hclparse.File) bool {
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return false
	}

	for _, attr := range body.Attributes {
		for _, traversal := range attr.Expr.Variables() {
			const dependencyInputsIdentifierMinParts = 3

			if len(traversal) < dependencyInputsIdentifierMinParts {
				continue
			}

			root, ok := traversal[0].(hcl.TraverseRoot)
			if !ok || root.Name != MetadataDependency {
				continue
			}

			attrTraversal, ok := traversal[2].(hcl.TraverseAttr)
			if !ok || attrTraversal.Name != MetadataInputs {
				continue
			}

			return true
		}
	}

	return false
}

// detectBareIncludeUsage detects if an identifier matching include.foo is used in the given HCL file.
//
// This is deprecated functionality, so we look for this to determine if we should throw an error or warning.
func detectBareIncludeUsage(file *hclparse.File) bool {
	switch filepath.Ext(file.ConfigPath) {
	case ".json":
		var data map[string]any
		if err := json.Unmarshal(file.Bytes, &data); err != nil {
			// If JSON is invalid, it can't be a valid bare include structure.
			// The main parser will handle the invalid JSON error.
			return false
		}

		includeBlockUntyped, exists := data[MetadataInclude]
		if !exists {
			return false
		}

		switch includeBlockTyped := includeBlockUntyped.(type) {
		case map[string]any:
			// Delegate to the logic from include.go, which checks if the map
			// represents a bare include block (e.g., only known include attributes).
			return jsonIsIncludeBlock(includeBlockTyped)
		case []any:
			// A bare include in JSON array form must have exactly one element,
			// and that element must be an include block.
			if len(includeBlockTyped) == 1 {
				if firstElement, ok := includeBlockTyped[0].(map[string]any); ok {
					return jsonIsIncludeBlock(firstElement)
				}
			}

			return false
		default:
			return false
		}
	default:
		body, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			return false
		}

		for _, block := range body.Blocks {
			if block.Type == MetadataInclude && len(block.Labels) == 0 {
				return true
			}
		}

		return false
	}
}

// iamRoleCache - store for cached values of IAM roles
var iamRoleCache = cache.NewCache[options.IAMRoleOptions](iamRoleCacheName)

// setIAMRole - extract IAM role details from Terragrunt flags block
func setIAMRole(ctx *ParsingContext, l log.Logger, file *hclparse.File, includeFromChild *IncludeConfig) error {
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
			iamConfig, err := TerragruntConfigFromPartialConfig(ctx.WithDecodeList(TerragruntFlags), l, file, includeFromChild)
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

func decodeAsTerragruntConfigFile(ctx *ParsingContext, l log.Logger, file *hclparse.File, evalContext *hcl.EvalContext) (*terragruntConfigFile, error) {
	terragruntConfig := terragruntConfigFile{}

	if err := file.Decode(&terragruntConfig, evalContext); err != nil {
		var diagErr hcl.Diagnostics

		ok := errors.As(err, &diagErr)

		// in case of render-json command and inputs reference error, we update the inputs with default value
		if (!ok || !isRenderJSONCommand(ctx) || !isAttributeAccessError(diagErr)) &&
			(!ok || !isRenderCommand(ctx) || !isAttributeAccessError(diagErr)) {
			return &terragruntConfig, err
		}

		l.Warnf("Failed to decode inputs %v", diagErr)
	}

	if terragruntConfig.Inputs != nil {
		inputs, err := ctyhelper.UpdateUnknownCtyValValues(*terragruntConfig.Inputs)
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
	errs := &errors.MultiError{}

	if ctx.ConvertToTerragruntConfigFunc != nil {
		return ctx.ConvertToTerragruntConfigFunc(ctx, configPath, terragruntConfigFromFile)
	}

	terragruntConfig := &TerragruntConfig{
		IsPartial: false,
		// Initialize GenerateConfigs so we can append to it
		GenerateConfigs: map[string]codegen.GenerateConfig{},
	}

	defaultMetadata := map[string]any{FoundInFile: configPath}

	if terragruntConfigFromFile.RemoteState != nil {
		config, err := terragruntConfigFromFile.RemoteState.Config()
		if err != nil {
			errs = errs.Append(err)
		}

		terragruntConfig.RemoteState = remotestate.New(config)
		terragruntConfig.SetFieldMetadata(MetadataRemoteState, defaultMetadata)
	}

	if terragruntConfigFromFile.RemoteStateAttr != nil {
		remoteStateMap, err := ctyhelper.ParseCtyValueToMap(*terragruntConfigFromFile.RemoteStateAttr)
		if err != nil {
			return nil, err
		}

		var config *remotestate.Config
		if err := mapstructure.Decode(remoteStateMap, &config); err != nil {
			return nil, err
		}

		terragruntConfig.RemoteState = remotestate.New(config)
		terragruntConfig.SetFieldMetadata(MetadataRemoteState, defaultMetadata)
	}

	if err := terragruntConfigFromFile.Terraform.ValidateHooks(); err != nil {
		errs = errs.Append(err)
	}

	terragruntConfig.Terraform = terragruntConfigFromFile.Terraform
	if terragruntConfig.Terraform != nil { // since Terraform is nil each time avoid saving metadata when it is nil
		terragruntConfig.SetFieldMetadata(MetadataTerraform, defaultMetadata)
	}

	if err := validateDependencies(ctx, terragruntConfigFromFile.Dependencies); err != nil {
		errs = errs.Append(err)
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
		generateMap, err := ctyhelper.ParseCtyValueToMap(*terragruntConfigFromFile.GenerateAttrs)
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
		errs = errs.Append(err)
	}

	for _, block := range generateBlocks {
		// Validate that if_exists is provided (required attribute)
		if block.IfExists == "" {
			errs = errs.Append(errors.Errorf("generate block %q is missing required attribute \"if_exists\"", block.Name))
			continue
		}

		ifExists, err := codegen.GenerateConfigExistsFromString(block.IfExists)
		if err != nil {
			errs = errs.Append(errors.Errorf("generate block %q: %w", block.Name, err))
			continue
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
		inputs, err := ctyhelper.ParseCtyValueToMap(*terragruntConfigFromFile.Inputs)
		if err != nil {
			errs = errs.Append(err)
		}

		terragruntConfig.Inputs = inputs
		terragruntConfig.SetFieldMetadataMap(MetadataInputs, terragruntConfig.Inputs, defaultMetadata)
	}

	if ctx.Locals != nil && *ctx.Locals != cty.NilVal {
		localsParsed, err := ctyhelper.ParseCtyValueToMap(*ctx.Locals)
		if err != nil {
			return nil, err
		}

		// Only set Locals if there are actual values to avoid setting an empty map
		if len(localsParsed) > 0 {
			terragruntConfig.Locals = localsParsed
			terragruntConfig.SetFieldMetadataMap(MetadataLocals, localsParsed, defaultMetadata)
		}
	}

	return terragruntConfig, errs.ErrorOrNil()
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
		if os.IsNotExist(err) {
			return false, DependencyFileNotFoundError{Path: configPath}
		}

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
func (cfg *TerragruntConfig) SetFieldMetadataWithType(fieldType, fieldName string, m map[string]any) {
	if cfg.FieldsMetadata == nil {
		cfg.FieldsMetadata = map[string]map[string]any{}
	}

	field := fmt.Sprintf("%s-%s", fieldType, fieldName)

	metadata, found := cfg.FieldsMetadata[field]
	if !found {
		metadata = make(map[string]any)
	}

	maps.Copy(metadata, m)

	cfg.FieldsMetadata[field] = metadata
}

// SetFieldMetadata set metadata on the given field name.
func (cfg *TerragruntConfig) SetFieldMetadata(fieldName string, m map[string]any) {
	cfg.SetFieldMetadataWithType(fieldName, fieldName, m)
}

// SetFieldMetadataMap set metadata on fields from map keys.
// Example usage - setting metadata on all variables from inputs.
func (cfg *TerragruntConfig) SetFieldMetadataMap(field string, data map[string]any, metadata map[string]any) {
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
	meta := map[string]any{}

	if cfg.Engine.Meta != nil {
		parsedMeta, err := ctyhelper.ParseCtyValueToMap(*cfg.Engine.Meta)
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

		// Validate retry settings
		if retryBlock.MaxAttempts < 1 {
			return nil, errors.Errorf("cannot have less than 1 max retry in errors.retry %q, but you specified %d", retryBlock.Label, retryBlock.MaxAttempts)
		}

		if retryBlock.SleepIntervalSec < 0 {
			return nil, errors.Errorf("cannot sleep for less than 0 seconds in errors.retry %q, but you specified %d", retryBlock.Label, retryBlock.SleepIntervalSec)
		}

		compiledPatterns := make([]*options.ErrorsPattern, 0, len(retryBlock.RetryableErrors))

		for _, pattern := range retryBlock.RetryableErrors {
			value, err := errorsPattern(pattern)
			if err != nil {
				return nil, errors.Errorf("invalid retry pattern %q in block %q: %w",
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

		var signals map[string]any

		if ignoreBlock.Signals != nil {
			value, err := ConvertValuesMapToCtyVal(ignoreBlock.Signals)
			if err != nil {
				return nil, err
			}

			signals, err = ctyhelper.ParseCtyValueToMap(value)
			if err != nil {
				return nil, err
			}
		}

		compiledPatterns := make([]*options.ErrorsPattern, 0, len(ignoreBlock.IgnorableErrors))

		for _, pattern := range ignoreBlock.IgnorableErrors {
			value, err := errorsPattern(pattern)
			if err != nil {
				return nil, errors.Errorf("invalid ignore pattern %q in block %q: %w",
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

// ParseRemoteState reads the Terragrunt config file from its default location
// and parses and returns the `remote_state` block.
func ParseRemoteState(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (*remotestate.RemoteState, error) {
	cfg, err := ReadTerragruntConfig(ctx, l, opts, DefaultParserOptions(l, opts))
	if err != nil {
		return nil, err
	}

	return cfg.GetRemoteState(l, opts)
}
