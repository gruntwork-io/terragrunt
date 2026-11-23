// Package config parses Terragrunt configuration files.
//
// A [Store] parses unit configs. It decodes each file into block structs of its own and converts them to a
// [TerragruntConfig]. A parse fetches no dependency outputs: it decodes the parts that read no `dependency` value,
// which [QueueBodies] names, and keeps the rest. [UnitConfig.ResolveRunConfig] fetches the outputs and decodes the
// parts that read them, which [RunBodies] names, including an exclude block that reads `dependency`. Every part
// evaluates once, and nothing a parse produced changes afterwards.
//
// The engine block is the one part of both: a parse decodes it with `dependency` unknown, as discovery needs it, and
// ResolveRunConfig decodes it again with the outputs.
//
// The Store shares a file's syntax tree across the units that include it, and shares a unit's parse with the callers
// that wait on it while it runs. Which parts of a caller's context a memoized stage depends on is the contract of the
// run digest in store_profile.go. A memoized result holds nothing of its caller's: each stage takes the settings and
// sinks of a [ParsingContext], and assembles a [TerragruntConfig] of the caller's own, which the merges change in
// place.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/hclhelper"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"errors"

	"github.com/go-viper/mapstructure/v2"
	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

const (
	// DefaultTerragruntConfigPath is the file name of a unit's Terragrunt config.
	DefaultTerragruntConfigPath = "terragrunt.hcl"
	// DefaultStackFile is the file name of a stack config.
	DefaultStackFile = "terragrunt.stack.hcl"
	// DefaultTerragruntJSONConfigPath is the file name of a unit's Terragrunt config written in HCL JSON syntax.
	DefaultTerragruntJSONConfigPath = "terragrunt.hcl.json"
	// DefaultAutoIncludeFile is the file name of the autoinclude file stack generation writes into a unit.
	DefaultAutoIncludeFile = inthclparse.AutoIncludeFile
	// DefaultAutoIncludeStackFile is the file name of the autoinclude file stack generation writes into a stack.
	DefaultAutoIncludeStackFile = inthclparse.AutoIncludeStackFile
	// RecommendedParentConfigName is the file name `terragrunt scaffold` gives the root config that units include.
	RecommendedParentConfigName = "root.hcl"

	// FoundInFile is the field metadata key whose value is the path of the config file that set the field.
	FoundInFile = "found_in_file"

	logMsgSeparator = "\n"

	// DefaultEngineType is the engine type used when the `engine` block sets no `type`.
	DefaultEngineType = "rpc"
	// MetadataTerraform names the `terraform` block.
	MetadataTerraform = "terraform"
	// MetadataTerraformBinary names the `terraform_binary` attribute.
	MetadataTerraformBinary = "terraform_binary"
	// MetadataTerraformVersionConstraint names the `terraform_version_constraint` attribute.
	MetadataTerraformVersionConstraint = "terraform_version_constraint"
	// MetadataTerragruntVersionConstraint names the `terragrunt_version_constraint` attribute.
	MetadataTerragruntVersionConstraint = "terragrunt_version_constraint"
	// MetadataRemoteState names the `remote_state` block.
	MetadataRemoteState = "remote_state"
	// MetadataDependencies names the `dependencies` block.
	MetadataDependencies = "dependencies"
	// MetadataDependency names the `dependency` block and the variable that exposes its outputs.
	MetadataDependency = "dependency"
	// MetadataDownloadDir names the `download_dir` attribute.
	MetadataDownloadDir = "download_dir"
	// MetadataPreventDestroy names the `prevent_destroy` attribute.
	MetadataPreventDestroy = "prevent_destroy"
	// MetadataIamRole names the `iam_role` attribute.
	MetadataIamRole = "iam_role"
	// MetadataIamAssumeRoleDuration names the `iam_assume_role_duration` attribute.
	MetadataIamAssumeRoleDuration = "iam_assume_role_duration"
	// MetadataIamAssumeRoleSessionName names the `iam_assume_role_session_name` attribute.
	MetadataIamAssumeRoleSessionName = "iam_assume_role_session_name"
	// MetadataIamWebIdentityToken names the `iam_web_identity_token` attribute.
	MetadataIamWebIdentityToken = "iam_web_identity_token"
	// MetadataInputs names the `inputs` attribute.
	MetadataInputs = "inputs"
	// MetadataVariables names the `variables` block.
	MetadataVariables = "variables"
	// MetadataLocals names the `locals` block.
	MetadataLocals = "locals"
	// MetadataLocal names the variable that exposes locals in expressions, as in `local.name`.
	MetadataLocal = "local"
	// MetadataCatalog names the `catalog` block.
	MetadataCatalog = "catalog"
	// MetadataEngine names the `engine` block.
	MetadataEngine = "engine"
	// MetadataGenerateConfigs names the `generate` block.
	MetadataGenerateConfigs = "generate"
	// MetadataInclude names the `include` block and the variable that exposes included configs.
	MetadataInclude = "include"
	// MetadataFeatureFlag names the `feature` block and the variable that exposes feature flags.
	MetadataFeatureFlag = "feature"
	// MetadataExclude names the `exclude` block.
	MetadataExclude = "exclude"
	// MetadataErrors names the `errors` block.
	MetadataErrors = "errors"
	// MetadataRetry names the `retry` block nested in `errors`.
	MetadataRetry = "retry"
	// MetadataIgnore names the `ignore` block nested in `errors`.
	MetadataIgnore = "ignore"
	// MetadataValues names the `values` variable.
	MetadataValues = "values"
	// MetadataStack names the `stack` block of a stack file.
	MetadataStack = "stack"
	// MetadataUnit names the `unit` block of a stack file.
	MetadataUnit = "unit"
)

var (
	// DefaultTerragruntConfigPaths lists the default config file names in lookup order. Order matters: if none of
	// the files are found, `GetDefaultConfigPath` returns the last element.
	DefaultTerragruntConfigPaths = []string{
		DefaultTerragruntJSONConfigPath,
		DefaultTerragruntConfigPath,
	}

	// DefaultGenerateBlockIfDisabledValueStr is the `if_disabled` value of a `generate` block that sets none.
	DefaultGenerateBlockIfDisabledValueStr = codegen.DisabledSkipStr
)

// DecodedBaseBlocks decoded base blocks struct
type DecodedBaseBlocks struct {
	// TrackInclude is the include blocks of the file.
	TrackInclude *TrackInclude
	// Locals is the evaluated locals block.
	Locals *cty.Value
	// FeatureFlags is the value of the `feature` variable: the file's flags merged with those of its includes.
	FeatureFlags *cty.Value
	// FeatureFlagBlocks is the feature blocks the file itself declares.
	FeatureFlagBlocks FeatureFlags
}

// IncludedFeatureFlags returns the feature blocks that the config include names declares, for the parse whose
// include blocks are trackInclude.
type IncludedFeatureFlags func(
	ctx context.Context,
	l log.Logger,
	trackInclude *TrackInclude,
	include *IncludeConfig,
) (FeatureFlags, error)

// IncludedDependencies returns the dependency blocks that the config include names declares.
type IncludedDependencies func(ctx context.Context, l log.Logger, include *IncludeConfig) (Dependencies, error)

// TerragruntConfig represents a parsed and expanded configuration
// NOTE: if any attributes are added, make sure to update terragruntConfigAsCty in config_as_cty.go
type TerragruntConfig struct {
	Locals                      map[string]any
	ProcessedIncludes           IncludeConfigsMap
	FieldsMetadata              map[string]map[string]any
	Terraform                   *TerraformConfig
	Errors                      *ErrorsConfig
	Outputs                     *OutputsConfig
	RemoteState                 *remotestate.RemoteState
	Dependencies                *ModuleDependencies
	Exclude                     *ExcludeConfig
	PreventDestroy              *bool
	GenerateConfigs             map[string]codegen.GenerateConfig
	IamAssumeRoleDuration       *int64
	Inputs                      map[string]any
	Variables                   []VariablesConfig
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
}

// GetRemoteState returns the config's `remote_state` block, or nil when the config has none.
//
// When the block exists, GetRemoteState also sets pctx.WorkingDir to the directory the unit's source downloads into.
func (cfg *TerragruntConfig) GetRemoteState(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
) (*remotestate.RemoteState, error) {
	if cfg.RemoteState == nil {
		l.Debug("Did not find remote `remote_state` block in the config")

		return nil, nil
	}

	markNotShareable(ctx)

	sourceURL, err := GetTerraformSourceURL(
		pctx.Source,
		pctx.SourceMap,
		pctx.OriginalTerragruntConfigPath,
		cfg,
	)
	if err != nil {
		return nil, err
	}

	if sourceURL != "" {
		walkWithSymlinks := pctx.Experiments.Evaluate(experiment.Symlinks)
		// Apply the rewrite so this working-dir computation agrees with the
		// downloader's; a plain https://www.googleapis.com/storage/... source
		// would otherwise resolve remote-state to a path the downloader never
		// writes to.
		canonicalSourceURL := tf.RewriteLegacyGCSPublicSource(
			ctx,
			l,
			sourceURL,
			pctx.StrictControls,
		)

		tfSource, err := tf.NewSource(
			l,
			v.FS,
			canonicalSourceURL,
			pctx.DownloadDir,
			pctx.WorkingDir,
			walkWithSymlinks,
		)
		if err != nil {
			return nil, err
		}

		pctx.WorkingDir = tfSource.WorkingDir
	}

	return cfg.RemoteState, nil
}

func (cfg *TerragruntConfig) String() string {
	return fmt.Sprintf(
		"TerragruntConfig{Terraform = %v, RemoteState = %v, Dependencies = %v, PreventDestroy = %v}",
		cfg.Terraform,
		cfg.RemoteState,
		cfg.Dependencies,
		cfg.PreventDestroy,
	)
}

// GetIAMRoleOptions is a helper function that converts the Terragrunt config IAM role attributes to
// iam.RoleOptions struct.
func (cfg *TerragruntConfig) GetIAMRoleOptions() iam.RoleOptions {
	configIAMRoleOptions := iam.RoleOptions{
		RoleARN:               cfg.IamRole,
		AssumeRoleSessionName: cfg.IamAssumeRoleSessionName,
		WebIdentityToken:      cfg.IamWebIdentityToken,
	}
	if cfg.IamAssumeRoleDuration != nil {
		configIAMRoleOptions.AssumeRoleDuration = *cfg.IamAssumeRoleDuration
	}

	return configIAMRoleOptions
}

// DependencyPaths returns the paths cfg depends on: the config_path of each enabled dependency block, then each path
// in the dependencies block, without duplicates. A relative path is relative to the directory of the unit.
//
// It also returns the name of each enabled dependency block whose config_path is not a non-empty known string, in
// declaration order. A partial parse can leave config_path unknown, and such a block has no path to return.
func (cfg *TerragruntConfig) DependencyPaths() (paths, unresolved []string) {
	seen := make(map[string]struct{}, len(cfg.TerragruntDependencies))

	add := func(path string) {
		if _, ok := seen[path]; ok {
			return
		}

		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	for i := range cfg.TerragruntDependencies {
		dep := &cfg.TerragruntDependencies[i]
		if !dep.isEnabled() {
			continue
		}

		if !IsValidConfigPath(dep.ConfigPath) {
			unresolved = append(unresolved, dep.Name)
			continue
		}

		add(dep.ConfigPath.AsString())
	}

	if cfg.Dependencies != nil {
		for _, path := range cfg.Dependencies.Paths {
			add(path)
		}
	}

	return paths, unresolved
}

// WriteTo writes the terragrunt config to a writer
func (cfg *TerragruntConfig) WriteTo(w io.Writer) (int64, error) {
	cfgAsCty, err := TerragruntConfigAsCty(cfg)
	if err != nil {
		return 0, err
	}

	f := hclwrite.NewFile()
	rootBody := f.Body()

	if len(cfg.Locals) > 0 {
		rootBody.AppendBlock(localsBlock(cfg.Locals, cfgAsCty.GetAttr("locals")))
	}

	if cfg.Terraform != nil {
		rootBody.AppendBlock(terraformBlock(cfg.Terraform, cfgAsCty.GetAttr("terraform")))
	}

	if cfg.RemoteState != nil {
		rootBody.AppendBlock(remoteStateBlock(cfg.RemoteState, cfgAsCty.GetAttr("remote_state")))
	}

	if cfg.Dependencies != nil && len(cfg.Dependencies.Paths) > 0 {
		rootBody.AppendBlock(dependenciesBlock(cfgAsCty.GetAttr("dependencies")))
	}

	if err := appendDependencyBlocks(rootBody, cfg.TerragruntDependencies); err != nil {
		return 0, err
	}

	for name, gen := range cfg.GenerateConfigs {
		rootBody.AppendBlock(generateBlock(name, &gen))
	}

	for _, flag := range cfg.FeatureFlags {
		rootBody.AppendBlock(featureBlock(flag, cfgAsCty.GetAttr("feature").GetAttr(flag.Name)))
	}

	if cfg.Engine != nil {
		rootBody.AppendBlock(engineBlock(cfg.Engine, cfgAsCty.GetAttr("engine")))
	}

	if cfg.Exclude != nil {
		rootBody.AppendBlock(excludeBlock(cfg.Exclude, cfgAsCty.GetAttr("exclude")))
	}

	if cfg.Errors != nil {
		rootBody.AppendBlock(errorsBlock(cfg.Errors))
	}

	if cfg.Outputs != nil {
		rootBody.AppendBlock(outputsBlock(cfg.Outputs))
	}

	if cfg.Catalog != nil {
		rootBody.AppendBlock(catalogBlock(cfg.Catalog, cfgAsCty.GetAttr("catalog")))
	}

	if len(cfg.Variables) > 0 {
		for it := cfgAsCty.GetAttr(MetadataVariables).ElementIterator(); it.Next(); {
			_, block := it.Element()
			rootBody.AppendBlock(variablesBlock(block))
		}
	}

	appendRootAttributes(rootBody, cfg, cfgAsCty)

	return f.WriteTo(w)
}

// localsBlock renders the evaluated locals.
func localsBlock(locals map[string]any, asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock("locals", nil)
	body := block.Body()

	for name := range locals {
		body.SetAttributeRaw(name, hclhelper.TokensForValue(asCty.GetAttr(name)))
	}

	return block
}

// terraformBlock renders the terraform block along with the extra arguments and hooks
// nested in it.
func terraformBlock(terraform *TerraformConfig, asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock("terraform", nil)
	body := block.Body()

	if terraform.Source != nil {
		body.SetAttributeValue("source", asCty.GetAttr("source"))
	}

	if terraform.UpdateSourceWithCAS != nil {
		body.SetAttributeValue("update_source_with_cas", asCty.GetAttr("update_source_with_cas"))
	}

	if terraform.Mutable != nil {
		body.SetAttributeValue("mutable", asCty.GetAttr("mutable"))
	}

	if len(terraform.ExtraArgs) > 0 {
		extraArgsAsCty := asCty.GetAttr("extra_arguments").AsValueMap()

		for i := range terraform.ExtraArgs {
			arg := &terraform.ExtraArgs[i]
			body.AppendBlock(extraArgumentsBlock(arg, extraArgsAsCty[arg.Name]))
		}
	}

	for i := range terraform.BeforeHooks {
		hook := &terraform.BeforeHooks[i]
		body.AppendBlock(
			hookBlock("before_hook", hook, asCty.GetAttr("before_hook").AsValueMap()[hook.Name]),
		)
	}

	for i := range terraform.AfterHooks {
		hook := &terraform.AfterHooks[i]
		body.AppendBlock(
			hookBlock("after_hook", hook, asCty.GetAttr("after_hook").AsValueMap()[hook.Name]),
		)
	}

	for i := range terraform.ErrorHooks {
		hook := &terraform.ErrorHooks[i]
		body.AppendBlock(errorHookBlock(hook, asCty.GetAttr("error_hook").AsValueMap()[hook.Name]))
	}

	return block
}

// extraArgumentsBlock renders one extra_arguments block of a terraform block.
func extraArgumentsBlock(arg *TerraformExtraArguments, asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock("extra_arguments", []string{arg.Name})
	body := block.Body()

	if arg.Commands != nil {
		body.SetAttributeValue("commands", asCty.GetAttr("commands"))
	}

	if arg.Arguments != nil {
		body.SetAttributeValue("arguments", asCty.GetAttr("arguments"))
	}

	if arg.RequiredVarFiles != nil {
		body.SetAttributeValue("required_var_files", asCty.GetAttr("required_var_files"))
	}

	if arg.OptionalVarFiles != nil {
		body.SetAttributeValue("optional_var_files", asCty.GetAttr("optional_var_files"))
	}

	if arg.EnvVars != nil {
		body.SetAttributeRaw("env_vars", hclhelper.TokensForValue(asCty.GetAttr("env_vars")))
	}

	return block
}

// hookBlock renders one before_hook or after_hook block, which share a shape.
func hookBlock(blockType string, hook *Hook, asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock(blockType, []string{hook.Name})
	body := block.Body()

	if hook.If != nil {
		body.SetAttributeValue("if", asCty.GetAttr("if"))
	}

	if hook.RunOnError != nil {
		body.SetAttributeValue("run_on_error", asCty.GetAttr("run_on_error"))
	}

	body.SetAttributeValue("commands", asCty.GetAttr("commands"))
	body.SetAttributeValue("execute", asCty.GetAttr("execute"))

	if hook.WorkingDir != nil {
		body.SetAttributeValue("working_dir", asCty.GetAttr("working_dir"))
	}

	return block
}

// errorHookBlock renders one error_hook block. It runs on the errors it names rather than
// on a command, so it carries neither the if nor the run_on_error the other hooks do.
func errorHookBlock(hook *ErrorHook, asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock("error_hook", []string{hook.Name})
	body := block.Body()

	body.SetAttributeValue("commands", asCty.GetAttr("commands"))
	body.SetAttributeValue("execute", asCty.GetAttr("execute"))
	body.SetAttributeValue("on_errors", asCty.GetAttr("on_errors"))

	if hook.WorkingDir != nil {
		body.SetAttributeValue("working_dir", asCty.GetAttr("working_dir"))
	}

	return block
}

// remoteStateBlock renders the remote_state block.
func remoteStateBlock(state *remotestate.RemoteState, asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock("remote_state", nil)
	body := block.Body()

	body.SetAttributeValue("backend", asCty.GetAttr("backend"))

	if state.DisableInit {
		body.SetAttributeValue("disable_init", asCty.GetAttr("disable_init"))
	}

	if state.DisableDependencyOptimization {
		body.SetAttributeValue(
			"disable_dependency_optimization",
			asCty.GetAttr("disable_dependency_optimization"),
		)
	}

	if state.BackendConfig != nil {
		body.SetAttributeRaw("config", hclhelper.TokensForValue(asCty.GetAttr("config")))
	}

	return block
}

// dependenciesBlock renders the dependencies block listing bare paths.
func dependenciesBlock(asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock("dependencies", nil)
	block.Body().SetAttributeValue("paths", asCty.GetAttr("paths"))

	return block
}

// appendDependencyBlocks renders the dependency blocks, previewing the ones that expanded.
func appendDependencyBlocks(body *hclwrite.Body, deps Dependencies) error {
	for _, group := range groupExpandedDependencies(deps) {
		if group.source == nil {
			for _, dep := range group.deps {
				depBlock, err := dependencyBlock(dep)
				if err != nil {
					return err
				}

				body.AppendBlock(depBlock)
			}

			continue
		}

		if err := appendExpansionPreview(body, group); err != nil {
			return err
		}
	}

	return nil
}

// generateBlock renders one generate block.
func generateBlock(name string, gen *codegen.GenerateConfig) *hclwrite.Block {
	block := hclwrite.NewBlock("generate", []string{name})
	body := block.Body()

	body.SetAttributeValue("path", gostringToCty(gen.Path))
	body.SetAttributeValue("if_exists", gostringToCty(gen.IfExistsStr))
	body.SetAttributeValue("if_disabled", gostringToCty(gen.IfDisabledStr))
	body.SetAttributeValue("contents", gostringToCty(gen.Contents))

	if gen.CommentPrefix != codegen.DefaultCommentPrefix {
		body.SetAttributeValue("comment_prefix", gostringToCty(gen.CommentPrefix))
	}

	if gen.DisableSignature {
		body.SetAttributeValue("disable_signature", goboolToCty(gen.DisableSignature))
	}

	if gen.Disable {
		body.SetAttributeValue("disable", goboolToCty(gen.Disable))
	}

	if gen.HclFmt != nil {
		body.SetAttributeValue("hcl_fmt", goboolToCty(*gen.HclFmt))
	}

	if gen.Mutable != nil {
		body.SetAttributeValue("mutable", goboolToCty(*gen.Mutable))
	}

	return block
}

// featureBlock renders one feature block.
func featureBlock(flag *FeatureFlag, asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock("feature", []string{flag.Name})

	if flag.Default != nil {
		block.Body().SetAttributeRaw("default", hclhelper.TokensForValue(asCty.GetAttr("default")))
	}

	return block
}

// engineBlock renders the engine block.
func engineBlock(engine *EngineConfig, asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock("engine", nil)
	body := block.Body()

	if engine.Source != "" {
		body.SetAttributeValue("source", asCty.GetAttr("source"))
	}

	if engine.Version != nil {
		body.SetAttributeValue("version", asCty.GetAttr("version"))
	}

	if engine.Type != nil {
		body.SetAttributeValue("type", asCty.GetAttr("type"))
	}

	if engine.Meta != nil {
		body.SetAttributeRaw("meta", hclhelper.TokensForValue(asCty.GetAttr("meta")))
	}

	return block
}

// excludeBlock renders the exclude block.
func excludeBlock(exclude *ExcludeConfig, asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock("exclude", nil)
	body := block.Body()

	if exclude.ExcludeDependencies != nil {
		body.SetAttributeValue("exclude_dependencies", asCty.GetAttr("exclude_dependencies"))
	}

	if len(exclude.Actions) > 0 {
		body.SetAttributeValue("actions", asCty.GetAttr("actions"))
	}

	if exclude.NoRun != nil {
		body.SetAttributeValue("no_run", asCty.GetAttr("no_run"))
	}

	body.SetAttributeValue("if", asCty.GetAttr("if"))

	return block
}

// errorsBlock renders the errors block along with the retry and ignore blocks nested in
// it. It reads the config directly, which is where the retry and ignore values live.
func errorsBlock(errs *ErrorsConfig) *hclwrite.Block {
	block := hclwrite.NewBlock("errors", nil)
	body := block.Body()

	for _, retry := range errs.Retry {
		body.AppendBlock(retryBlock(retry))
	}

	for _, ignore := range errs.Ignore {
		body.AppendBlock(ignoreBlock(ignore))
	}

	return block
}

// stringListVal renders values as a cty list of strings, empty when values has none.
func stringListVal(values []string) cty.Value {
	if len(values) == 0 {
		return cty.ListValEmpty(cty.String)
	}

	rendered := make([]cty.Value, len(values))
	for i, value := range values {
		rendered[i] = cty.StringVal(value)
	}

	return cty.ListVal(rendered)
}

// retryBlock renders one retry block of an errors block.
func retryBlock(retry *RetryBlock) *hclwrite.Block {
	block := hclwrite.NewBlock("retry", []string{retry.Label})
	body := block.Body()

	if retry.MaxAttempts > 0 {
		body.SetAttributeValue("max_attempts", cty.NumberIntVal(int64(retry.MaxAttempts)))
	}

	if retry.SleepIntervalSec > 0 {
		body.SetAttributeValue(
			"sleep_interval_sec",
			cty.NumberIntVal(int64(retry.SleepIntervalSec)),
		)
	}

	if len(retry.RetryableErrors) > 0 {
		body.SetAttributeValue("retryable_errors", stringListVal(retry.RetryableErrors))
	}

	return block
}

// ignoreBlock renders one ignore block of an errors block.
func ignoreBlock(ignore *IgnoreBlock) *hclwrite.Block {
	block := hclwrite.NewBlock("ignore", []string{ignore.Label})
	body := block.Body()

	if len(ignore.IgnorableErrors) > 0 {
		body.SetAttributeValue("ignorable_errors", stringListVal(ignore.IgnorableErrors))
	}

	if ignore.Message != "" {
		body.SetAttributeValue("message", cty.StringVal(ignore.Message))
	}

	if ignore.Signals != nil {
		body.SetAttributeRaw("signals", hclhelper.TokensForValue(cty.MapVal(ignore.Signals)))
	}

	return block
}

// outputsBlock renders the outputs block along with the mock_strategy and output blocks nested in it.
func outputsBlock(outputs *OutputsConfig) *hclwrite.Block {
	block := hclwrite.NewBlock(MetadataOutputs, nil)
	body := block.Body()

	if strategy := outputs.MockStrategy; strategy != nil {
		strategyBlock := hclwrite.NewBlock(MetadataMockStrategy, nil)

		if strategy.AllowedCommands != nil {
			strategyBlock.Body().SetAttributeValue("allowed_commands", stringListVal(*strategy.AllowedCommands))
		}

		if strategy.MergeWithState != nil {
			strategyBlock.Body().SetAttributeValue("merge_with_state", cty.StringVal(string(*strategy.MergeWithState)))
		}

		body.AppendBlock(strategyBlock)
	}

	for _, output := range outputs.Outputs {
		outputBlock := hclwrite.NewBlock(MetadataOutput, []string{output.Name})

		if output.MockValue != nil {
			mockValue, _ := output.MockValue.UnmarkDeep()
			outputBlock.Body().SetAttributeRaw("mock_value", hclhelper.TokensForValue(mockValue))
		}

		body.AppendBlock(outputBlock)
	}

	return block
}

// catalogBlock renders the catalog block.
func catalogBlock(catalog *CatalogConfig, asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock("catalog", nil)
	body := block.Body()

	if catalog.DefaultTemplate != "" {
		body.SetAttributeValue("default_template", asCty.GetAttr("default_template"))
	}

	if len(catalog.URLs) > 0 {
		body.SetAttributeValue("urls", asCty.GetAttr("urls"))
	}

	if catalog.NoShell != nil {
		body.SetAttributeValue("no_shell", asCty.GetAttr("no_shell"))
	}

	if catalog.NoHooks != nil {
		body.SetAttributeValue("no_hooks", asCty.GetAttr("no_hooks"))
	}

	return block
}

// appendRootAttributes renders the attributes set at the top level of the config, which
// follow its blocks in the rendered output.
func appendRootAttributes(body *hclwrite.Body, cfg *TerragruntConfig, asCty cty.Value) {
	if cfg.TerraformBinary != "" {
		body.SetAttributeValue("terraform_binary", asCty.GetAttr("terraform_binary"))
	}

	if cfg.TerraformVersionConstraint != "" {
		body.SetAttributeValue(
			"terraform_version_constraint",
			asCty.GetAttr("terraform_version_constraint"),
		)
	}

	if cfg.TerragruntVersionConstraint != "" {
		body.SetAttributeValue(
			"terragrunt_version_constraint",
			asCty.GetAttr("terragrunt_version_constraint"),
		)
	}

	if cfg.DownloadDir != "" {
		body.SetAttributeValue("download_dir", asCty.GetAttr("download_dir"))
	}

	if cfg.PreventDestroy != nil {
		body.SetAttributeValue("prevent_destroy", asCty.GetAttr("prevent_destroy"))
	}

	if cfg.IamRole != "" {
		body.SetAttributeValue("iam_role", asCty.GetAttr("iam_role"))
	}

	if cfg.IamAssumeRoleDuration != nil {
		body.SetAttributeValue(
			"iam_assume_role_duration",
			asCty.GetAttr("iam_assume_role_duration"),
		)
	}

	if cfg.IamAssumeRoleSessionName != "" {
		body.SetAttributeValue(
			"iam_assume_role_session_name",
			asCty.GetAttr("iam_assume_role_session_name"),
		)
	}

	if len(cfg.Inputs) > 0 {
		body.SetAttributeRaw("inputs", hclhelper.TokensForValue(asCty.GetAttr("inputs")))
	}
}

// dependencyGroup is the decoded dependencies one written block produced. source is nil
// for a block that declared no expansion, and for one decoded outside the expanding
// decoder, which leaves deps holding a single dependency.
type dependencyGroup struct {
	source *hclparse.SourceBlock
	deps   []*Dependency
}

// groupExpandedDependencies gathers the elements of each expanded block back under the
// block that produced them, in the order the elements were decoded. Dependencies that
// share no source block each get a group of their own.
func groupExpandedDependencies(deps Dependencies) []dependencyGroup {
	groups := make([]dependencyGroup, 0, len(deps))
	byRange := map[hcl.Range]int{}

	for i := range deps {
		dep := &deps[i]

		if dep.Expansion == nil || dep.Expansion.Source == nil {
			groups = append(groups, dependencyGroup{deps: []*Dependency{dep}})
			continue
		}

		source := dep.Expansion.Source

		if at, ok := byRange[source.Range]; ok {
			groups[at].deps = append(groups[at].deps, dep)
			continue
		}

		byRange[source.Range] = len(groups)
		groups = append(groups, dependencyGroup{source: source, deps: []*Dependency{dep}})
	}

	return groups
}

// appendExpansionPreview writes an expanded block as it was written, followed by the
// elements it expanded into, commented out and with every reference resolved.
//
// The elements stay comments because they all repeat the block's label, which
// [validateUniqueDependencies] warns about and, under its strict control, rejects.
// Rendering them as configuration would produce a file that reads back as one dependency.
func appendExpansionPreview(body *hclwrite.Body, group dependencyGroup) error {
	text, err := group.source.Body()
	if err != nil {
		return err
	}

	source, diags := hclwrite.ParseConfig(
		[]byte(text),
		group.source.Range.Filename,
		group.source.Range.Start,
	)
	if diags.HasErrors() {
		return diags
	}

	for _, block := range source.Body().Blocks() {
		body.AppendBlock(block)
	}

	lines, err := expansionPreviewLines(group.deps)
	if err != nil {
		return err
	}

	// The quoted source ends at its closing brace, so the first newline ends that line
	// and the second sets the preview off from the block it describes.
	body.AppendNewline()
	body.AppendNewline()
	body.AppendUnstructuredTokens(commentTokens(lines))
	body.AppendNewline()

	return nil
}

// expansionPreviewLines renders the elements of one expanded block, blank line separated,
// as the lines of the preview comment.
func expansionPreviewLines(deps []*Dependency) ([]string, error) {
	lines := []string{"Expands to:", ""}

	for i, dep := range deps {
		if i > 0 {
			lines = append(lines, "")
		}

		block, err := dependencyBlock(dep)
		if err != nil {
			return nil, err
		}

		rendered := hclwrite.NewEmptyFile()
		rendered.Body().AppendBlock(block)

		lines = append(
			lines,
			strings.Split(strings.TrimRight(string(rendered.Bytes()), "\n"), "\n")...)
	}

	return lines, nil
}

// commentTokens renders lines as consecutive comment lines, blank ones included so a
// comment can be paragraphed.
func commentTokens(lines []string) hclwrite.Tokens {
	tokens := make(hclwrite.Tokens, 0, len(lines))

	for _, line := range lines {
		text := "#\n"
		if line != "" {
			text = "# " + line + "\n"
		}

		tokens = append(tokens, &hclwrite.Token{
			Type:  hclsyntax.TokenComment,
			Bytes: []byte(text),
		})
	}

	return tokens
}

// dependencyBlock renders one decoded dependency, with the references in its body already
// resolved to the values this instance decoded against.
func dependencyBlock(dep *Dependency) (*hclwrite.Block, error) {
	depAsCty, err := GoTypeToCty(*dep)
	if err != nil {
		return nil, err
	}

	depBlock := hclwrite.NewBlock("dependency", []string{dep.Name})
	depBody := depBlock.Body()

	depBody.SetAttributeValue("config_path", depAsCty.GetAttr("config_path"))

	if dep.Enabled != nil {
		depBody.SetAttributeValue("enabled", goboolToCty(*dep.Enabled))
	}

	if dep.SkipOutputs != nil {
		depBody.SetAttributeValue("skip_outputs", goboolToCty(*dep.SkipOutputs))
	}

	if dep.MockOutputs != nil {
		depBody.SetAttributeRaw("mock_outputs", hclhelper.TokensForValue(depAsCty.GetAttr("mock_outputs")))
	}

	if dep.MockOutputsAllowedTerraformCommands != nil {
		depBody.SetAttributeValue(
			"mock_outputs_allowed_terraform_commands",
			depAsCty.GetAttr("mock_outputs_allowed_terraform_commands"),
		)
	}

	if dep.MockOutputsMergeStrategyWithState != nil {
		depBody.SetAttributeValue(
			"mock_outputs_merge_strategy_with_state",
			depAsCty.GetAttr("mock_outputs_merge_strategy_with_state"),
		)
	}

	return depBlock, nil
}

// TerragruntConfigFile represents the configuration supported in a Terragrunt configuration file (i.e.
// terragrunt.hcl)
type TerragruntConfigFile struct {
	Catalog                     *CatalogConfig   `hcl:"catalog,block"`
	Engine                      *EngineConfig    `hcl:"engine,block"`
	Terraform                   *TerraformConfig `hcl:"terraform,block"`
	TerraformBinary             *string          `hcl:"terraform_binary,attr"`
	TerraformVersionConstraint  *string          `hcl:"terraform_version_constraint,attr"`
	TerragruntVersionConstraint *string          `hcl:"terragrunt_version_constraint,attr"`
	Inputs                      *cty.Value       `hcl:"inputs,attr"`
	Variables                   []VariablesBlock `hcl:"variables,block"`

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
	DependencyBlocks         []dependencyHeader  `hcl:"dependency,block"`
	TerragruntDependencies   []Dependency
	FeatureFlags             []*FeatureFlag `hcl:"feature,block"`
	Exclude                  *ExcludeConfig `hcl:"exclude,block"`
	Errors                   *ErrorsConfig  `hcl:"errors,block"`
	Outputs                  *OutputsConfig `hcl:"outputs,block"`

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
	GenerateBlocks []TerragruntGenerateBlock `hcl:"generate,block"`

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

// dependencyHeader keeps `dependency` in the config file's schema without evaluating the
// block body.
type dependencyHeader struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:",label"`
}

type terragruntIncludeIgnore struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:"name,label"`
}

// TerragruntGenerateBlock is a generate block as decoded from HCL. [ConvertToTerragruntConfig] converts it to a
// [codegen.GenerateConfig] for the codegen routine.
type TerragruntGenerateBlock struct {
	IfDisabled       *string `hcl:"if_disabled,attr"       mapstructure:"if_disabled"`
	CommentPrefix    *string `hcl:"comment_prefix,attr"    mapstructure:"comment_prefix"`
	DisableSignature *bool   `hcl:"disable_signature,attr" mapstructure:"disable_signature"`
	Disable          *bool   `hcl:"disable,attr"           mapstructure:"disable"`
	HclFmt           *bool   `hcl:"hcl_fmt,attr"           mapstructure:"hcl_fmt"`
	Mutable          *bool   `hcl:"mutable,attr"           mapstructure:"mutable"`
	Name             string  `hcl:",label"                 mapstructure:",omitempty"`
	Path             string  `hcl:"path,attr"              mapstructure:"path"`
	IfExists         string  `hcl:"if_exists,attr"         mapstructure:"if_exists"`
	Contents         string  `hcl:"contents,attr"          mapstructure:"contents"`
}

// IncludeConfigsMap maps `include` block labels to their configs.
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

// IncludeConfigs lists `include` blocks in the order they appear in a config.
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

	exposeStr := nilText
	if include.Expose != nil {
		exposeStr = strconv.FormatBool(*include.Expose)
	}

	mergeStrategyStr := nilText
	if include.MergeStrategy != nil {
		mergeStrategyStr = fmt.Sprintf("%v", include.MergeStrategy)
	}

	return fmt.Sprintf(
		"IncludeConfig{Path = %v, Expose = %v, MergeStrategy = %v}",
		include.Path,
		exposeStr,
		mergeStrategyStr,
	)
}

// GetExpose reports whether the `include` block sets `expose = true`. It returns false for a nil include.
func (include *IncludeConfig) GetExpose() bool {
	if include == nil || include.Expose == nil {
		return false
	}

	return *include.Expose
}

// GetMergeStrategy returns the `merge_strategy` of the `include` block, or [ShallowMerge] when it sets none.
//
// Returns [InvalidMergeStrategyTypeError] when `merge_strategy` names no known strategy.
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
		return NoMerge, InvalidMergeStrategyTypeError(strategy)
	}
}

// MergeStrategyType is a `merge_strategy` value, which decides how an included config or a dependency's mock
// outputs merge into the config that declares them.
type MergeStrategyType string

const (
	// NoMerge skips the merge.
	NoMerge MergeStrategyType = "no_merge"
	// ShallowMerge merges only top-level keys. A key set on both sides takes one side's value whole.
	ShallowMerge MergeStrategyType = "shallow"
	// DeepMerge concatenates lists and merges maps recursively. Mock outputs reject it.
	DeepMerge MergeStrategyType = "deep"
	// DeepMergeMapOnly merges maps recursively and replaces lists whole. Only mock outputs accept it.
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
		if !slices.Contains(deps.Paths, path) {
			deps.Paths = append(deps.Paths, path)
		}
	}
}

func (deps *ModuleDependencies) String() string {
	return fmt.Sprintf("ModuleDependencies{Paths = %v}", deps.Paths)
}

// Hook specifies terraform commands (apply/plan) and array of os commands to execute
type Hook struct {
	If             *bool    `hcl:"if,attr"              cty:"if"`
	RunOnError     *bool    `hcl:"run_on_error,attr"    cty:"run_on_error"`
	SuppressStdout *bool    `hcl:"suppress_stdout,attr" cty:"suppress_stdout"`
	WorkingDir     *string  `hcl:"working_dir,attr"     cty:"working_dir"`
	Name           string   `hcl:"name,label"           cty:"name"`
	Commands       []string `hcl:"commands,attr"        cty:"commands"`
	Execute        []string `hcl:"execute,attr"         cty:"execute"`
}

// ErrorHook is an `error_hook` block: a command to run when the error message of a failed run matches one of the
// OnErrors regular expressions.
type ErrorHook struct {
	SuppressStdout *bool    `hcl:"suppress_stdout,attr" cty:"suppress_stdout"`
	WorkingDir     *string  `hcl:"working_dir,attr"     cty:"working_dir"`
	Name           string   `hcl:"name,label"           cty:"name"`
	Commands       []string `hcl:"commands,attr"        cty:"commands"`
	Execute        []string `hcl:"execute,attr"         cty:"execute"`
	OnErrors       []string `hcl:"on_errors,attr"       cty:"on_errors"`
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
	Source              *string `hcl:"source,attr"`
	Version             *string `hcl:"version,attr"`
	UpdateSourceWithCAS *bool   `hcl:"update_source_with_cas,attr"`
	Mutable             *bool   `hcl:"mutable,attr"`

	// Ideally we can avoid the pointer to list slice, but if it is not a pointer, Terraform requires the attribute to
	// be defined and we want to make this optional.
	IncludeInCopy   *[]string `hcl:"include_in_copy,attr"`
	ExcludeFromCopy *[]string `hcl:"exclude_from_copy,attr"`

	CopyTerraformLockFile *bool                     `hcl:"copy_terraform_lock_file,attr"`
	NoDotTerragruntCache  *bool                     `hcl:"no_dot_terragrunt_cache,attr"`
	ExtraArgs             []TerraformExtraArguments `hcl:"extra_arguments,block"`
	BeforeHooks           []Hook                    `hcl:"before_hook,block"`
	AfterHooks            []Hook                    `hcl:"after_hook,block"`
	ErrorHooks            []ErrorHook               `hcl:"error_hook,block"`
}

func (cfg *TerraformConfig) String() string {
	return fmt.Sprintf("TerraformConfig{Source = %v}", cfg.Source)
}

// GetBeforeHooks returns the `before_hook` blocks, or nil for a nil config.
func (cfg *TerraformConfig) GetBeforeHooks() []Hook {
	if cfg == nil {
		return nil
	}

	return cfg.BeforeHooks
}

// GetAfterHooks returns the `after_hook` blocks, or nil for a nil config.
func (cfg *TerraformConfig) GetAfterHooks() []Hook {
	if cfg == nil {
		return nil
	}

	return cfg.AfterHooks
}

// GetErrorHooks returns the `error_hook` blocks, or nil for a nil config.
func (cfg *TerraformConfig) GetErrorHooks() []ErrorHook {
	if cfg == nil {
		return nil
	}

	return cfg.ErrorHooks
}

// ValidateHooks checks that every hook has a non-empty first `execute` argument.
//
// Returns [InvalidArgError] naming the first hook that does not.
func (cfg *TerraformConfig) ValidateHooks() error {
	beforeAndAfterHooks := append(cfg.GetBeforeHooks(), cfg.GetAfterHooks()...)

	for _, curHook := range beforeAndAfterHooks {
		if len(curHook.Execute) < 1 || curHook.Execute[0] == "" {
			return InvalidArgError(
				fmt.Sprintf(
					"Error with hook %s. Need at least one non-empty argument in 'execute'.",
					curHook.Name,
				),
			)
		}
	}

	for _, curHook := range cfg.GetErrorHooks() {
		if len(curHook.Execute) < 1 || curHook.Execute[0] == "" {
			return InvalidArgError(
				fmt.Sprintf(
					"Error with hook %s. Need at least one non-empty argument in 'execute'.",
					curHook.Name,
				),
			)
		}
	}

	return nil
}

// ValidateVersion checks the optional version attribute. A version constraint only has
// meaning for a tfr:// registry source and must not duplicate a constraint already
// pinned inline via ?version= on that source. version and the source it constrains can
// come from different files via include, so this must run on the merged config rather
// than per file.
func (cfg *TerraformConfig) ValidateVersion(cfgPath string) error {
	if cfg == nil || cfg.Version == nil {
		return nil
	}

	var source string
	if cfg.Source != nil {
		source = *cfg.Source
	}

	sourceURL, err := url.Parse(source)
	if err != nil || sourceURL.Scheme != "tfr" {
		return VersionAttributeNonRegistrySourceError{ConfigPath: cfgPath}
	}

	if sourceURL.Query().Has("version") {
		return VersionAttributeSourceConstraintConflictError{ConfigPath: cfgPath}
	}

	return nil
}

// TerraformExtraArguments sets a list of arguments to pass to Terraform if command fits any in the `Commands` list
type TerraformExtraArguments struct {
	Arguments        *[]string          `hcl:"arguments,attr"          cty:"arguments"`
	RequiredVarFiles *[]string          `hcl:"required_var_files,attr" cty:"required_var_files"`
	OptionalVarFiles *[]string          `hcl:"optional_var_files,attr" cty:"optional_var_files"`
	EnvVars          *map[string]string `hcl:"env_vars,attr"           cty:"env_vars"`
	Name             string             `hcl:"name,label"              cty:"name"`
	Commands         []string           `hcl:"commands,attr"           cty:"commands"`
}

func (args *TerraformExtraArguments) String() string {
	return fmt.Sprintf(
		"TerraformArguments{Name = %s, Arguments = %v, Commands = %v, EnvVars = %v}",
		args.Name,
		args.Arguments,
		args.Commands,
		args.EnvVars)
}

// GetVarFiles returns every `required_var_files` entry followed by each `optional_var_files` entry that exists in
// fsys. Each list drops repeated paths, keeping the last occurrence.
func (args *TerraformExtraArguments) GetVarFiles(l log.Logger, fsys vfs.FS) []string {
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
			if vfs.Exists(fsys, file) {
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
// method returns the source URL. If neither is specified, returns "." to indicate the current directory should be
// used as the source, ensuring a .terragrunt-cache directory is always created for consistency.
func GetTerraformSourceURL(
	source string,
	sourceMap map[string]string,
	originalConfigPath string,
	cfg *TerragruntConfig,
) (string, error) {
	switch {
	case source != "":
		return source, nil
	case cfg.Terraform != nil && cfg.Terraform.Source != nil:
		return adjustSourceWithMap(
			sourceMap,
			*cfg.Terraform.Source,
			originalConfigPath,
		)
	default:
		return ".", nil
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
func adjustSourceWithMap(
	sourceMap map[string]string,
	source string,
	modulePath string,
) (string, error) {
	// Skip logic if source map is not configured
	if len(sourceMap) == 0 {
		return source, nil
	}

	// Split the module source string into a valid URL and subdirectory (if // is present).
	moduleURL, moduleSubdir := getter.SourceDirSubdir(source)

	// if both URL and subdir are missing, something went terribly wrong
	if moduleURL == "" && moduleSubdir == "" {
		return "", InvalidSourceURLWithMapError{ModulePath: modulePath, ModuleSourceURL: source}
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

// GetDefaultConfigPath returns the path of the Terragrunt configuration file in workingDir, preferring
// terragrunt.hcl.json over terragrunt.hcl. It returns workingDir itself when that names a file, and the
// terragrunt.hcl path when workingDir holds neither.
func GetDefaultConfigPath(fsys vfs.FS, workingDir string) string {
	// check if a configuration file was passed as `workingDir`.
	if vfs.IsFile(fsys, workingDir) {
		return workingDir
	}

	var cfgPath string

	for _, cfgPath = range DefaultTerragruntConfigPaths {
		if !filepath.IsAbs(cfgPath) {
			cfgPath = filepath.Join(workingDir, cfgPath)
		}

		if vfs.Exists(fsys, cfgPath) {
			break
		}
	}

	return cfgPath
}

// AmbiguousConfigCheck says whether resolving a unit directory fails when the directory holds both default unit
// configuration files.
type AmbiguousConfigCheck int

const (
	// RejectAmbiguousConfig fails with an [AmbiguousConfigError] on a directory holding both files.
	RejectAmbiguousConfig AmbiguousConfigCheck = iota

	// AllowAmbiguousConfig resolves a directory holding both files as [GetDefaultConfigPath] does, to
	// terragrunt.hcl.json. It applies when `--config` names the config file.
	AllowAmbiguousConfig
)

// RejectAmbiguousUnitConfig returns an [AmbiguousConfigError] when pctx rejects ambiguous configs and the directory
// of its config file holds both default unit configuration files.
func RejectAmbiguousUnitConfig(fsys vfs.FS, pctx *ParsingContext) error {
	return RejectAmbiguousDefaultConfig(fsys, filepath.Dir(pctx.TerragruntConfigPath), pctx.AmbiguousConfigCheck)
}

// RejectAmbiguousDefaultConfig returns an [AmbiguousConfigError] when check is [RejectAmbiguousConfig] and dir is a
// directory holding both default unit configuration files.
func RejectAmbiguousDefaultConfig(fsys vfs.FS, dir string, check AmbiguousConfigCheck) error {
	if check == AllowAmbiguousConfig {
		return nil
	}

	info, err := fsys.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	if !info.IsDir() {
		return nil
	}

	for _, name := range DefaultTerragruntConfigPaths {
		exists, err := vfs.FileExists(fsys, filepath.Join(dir, name))
		if err != nil {
			return err
		}

		if !exists {
			return nil
		}
	}

	return AmbiguousConfigError{
		ComponentPath: dir,
		ConfigFileA:   DefaultTerragruntConfigPath,
		ConfigFileB:   DefaultTerragruntJSONConfigPath,
	}
}

// FindConfigFilesInPath returns a list of all Terragrunt config files in the given path or any subfolder of the path.
//
// Parameters:
//   - fsys: the filesystem to walk
//   - rootPath: the root directory to search
//   - experiments: experiment flags (for symlink support)
//   - cfgPath: the terragrunt config path (to detect non-default config filenames)
//   - env: environment variables (to resolve TF_DATA_DIR)
//   - downloadDir: the terragrunt download directory to skip
func FindConfigFilesInPath(
	fsys vfs.FS,
	rootPath string,
	experiments experiment.Experiments,
	cfgPath string,
	env map[string]string,
	downloadDir string,
) ([]string, error) {
	configFiles := []string{}

	walkFunc := vfs.WalkDir

	if experiments.Evaluate(experiment.Symlinks) {
		walkFunc = vfs.WalkDirWithSymlinks
	}

	tfDataDir := tf.DefaultTFDataDir
	if d, ok := env["TF_DATA_DIR"]; ok {
		tfDataDir = d
	}

	err := walkFunc(fsys, rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		if !isTerragruntModuleDir(path, tfDataDir, downloadDir) {
			return filepath.SkipDir
		}

		for _, configFile := range append(DefaultTerragruntConfigPaths, filepath.Base(cfgPath)) {
			if !filepath.IsAbs(configFile) {
				configFile = filepath.Join(path, configFile)
			}

			if info, statErr := fsys.Stat(configFile); statErr == nil && !info.IsDir() {
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
func isTerragruntModuleDir(path string, tfDataDir string, downloadDir string) bool {
	// Skip the Terragrunt cache dir
	if util.ContainsPath(path, util.TerragruntCacheDir) {
		return false
	}

	// Skip the Terraform data dir
	if filepath.IsAbs(tfDataDir) {
		if util.HasPathPrefix(path, tfDataDir) {
			return false
		}
	} else {
		if util.ContainsPath(path, tfDataDir) {
			return false
		}
	}

	// Skip any custom download dir specified by the user
	if strings.Contains(filepath.Clean(path), filepath.Clean(downloadDir)) {
		return false
	}

	return true
}

// DetectDeprecatedConfigurations detects if deprecated configurations are used in the given HCL file.
func DetectDeprecatedConfigurations(
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	file *hclparse.File,
) error {
	if DetectInputsCtyUsage(file) {
		// Dependency inputs (dependency.foo.inputs.bar) are now blocked by default for performance.
		// This deprecated feature causes significant performance overhead due to recursive parsing.
		return errors.New(
			"Reading inputs from dependencies is no longer supported. To acquire values from dependencies, use outputs (dependency.foo.outputs.bar) instead.",
		)
	}

	if detectBareIncludeUsage(file) {
		allControls := pctx.StrictControls

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

			rest := traversal[2:]
			if _, indexed := rest[0].(hcl.TraverseIndex); indexed {
				rest = rest[1:]
			}

			if len(rest) == 0 {
				continue
			}

			attrTraversal, ok := rest[0].(hcl.TraverseAttr)
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

// iamRoleSchema names the attributes a full parse reads the IAM role options of a config from.
var iamRoleSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{{Name: MetadataIamRole}, {Name: MetadataIamWebIdentityToken}},
}

// remoteStateSchemas name the remote_state block and the remote_state attribute, which a config declares one of.
var remoteStateSchemas = []*hcl.BodySchema{
	{Blocks: []hcl.BlockHeaderSchema{{Type: MetadataRemoteState}}},
	{Attributes: []hcl.AttributeSchema{{Name: MetadataRemoteState}}},
}

// mayMergeDeclaration reports whether file, a file one of its include blocks merges, or its sibling autoinclude can
// declare an attribute or block of one of schemas. It reports true when [mergedIncludePaths] can't evaluate the
// include blocks of file. An included file that fails to parse declares none, since a partial parse that decodes the
// declaration fails on it.
func mayMergeDeclaration(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
	schemas ...*hcl.BodySchema,
) bool {
	if declares(file.Body, schemas) {
		return true
	}

	paths, ok := mergedIncludePaths(ctx, l, v, s, pctx, file)
	if !ok {
		return true
	}

	return anyDeclares(ctx, l, v, s, pctx, withSiblingAutoInclude(v, pctx, file.ConfigPath, paths), schemas)
}

// anyDeclares reports whether a config at one of paths declares an attribute or block of one of schemas. A config
// that fails to parse declares none.
func anyDeclares(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	paths []string,
	schemas []*hcl.BodySchema,
) bool {
	if len(paths) == 0 {
		return false
	}

	pctx = pctx.WithDiagnosticsDiscarded()

	return slices.ContainsFunc(paths, func(path string) bool {
		body, err := configBody(ctx, l, v, s, pctx, path)

		return err == nil && declares(body, schemas)
	})
}

// configBody returns the body of the config at path, parsed through s with a parser from pctx.
func configBody(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	path string,
) (hcl.Body, error) {
	info, err := v.FS.Stat(path)
	if err != nil {
		return nil, err
	}

	file, err := s.ParseUnitAST(ctx, v.FS, path, info, pctx.NewParser(l, v))
	if err != nil {
		return nil, err
	}

	return file.Body, nil
}

// withSiblingAutoInclude returns paths with the sibling autoinclude of the config at cfgPath appended when a parse
// with pctx merges it.
func withSiblingAutoInclude(v *venv.Venv, pctx *ParsingContext, cfgPath string, paths []string) []string {
	if autoIncludePath, ok := siblingAutoIncludePath(pctx, cfgPath); ok && vfs.Exists(v.FS, autoIncludePath) {
		return append(paths, autoIncludePath)
	}

	return paths
}

// mergedIncludePaths returns the absolute paths of the files the include blocks of file merge, evaluated with pctx and
// the values of file's directory, as a partial parse of file evaluates them. An include block with an invalid
// merge_strategy merges nothing.
//
// It evaluates only pure functions, so the evaluation has no effect a partial parse doesn't repeat. It returns false
// without evaluating anything when file is HCL JSON, or when its include blocks, or the values they read, call any
// other function. It returns false when an include block fails to evaluate.
func mergedIncludePaths(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
) ([]string, bool) {
	body, native := file.Body.(*hclsyntax.Body)
	if !native {
		return nil, false
	}

	includeBlocks := slices.DeleteFunc(slices.Clone(body.Blocks), func(block *hclsyntax.Block) bool {
		return !isIncludeBlock(block)
	})
	if len(includeBlocks) == 0 {
		return nil, true
	}

	if slices.ContainsFunc(includeBlocks, func(block *hclsyntax.Block) bool { return !callsOnlyPureFunctions(block) }) {
		return nil, false
	}

	pctx = pctx.WithTrackInclude(nil).WithDiagnosticsDiscarded()

	readsValues := slices.ContainsFunc(includeBlocks, func(block *hclsyntax.Block) bool {
		return referencesVariable(block, MetadataValues)
	})

	if readsValues {
		values, ok := pureValues(ctx, l, v, s, pctx, filepath.Dir(file.ConfigPath))
		if !ok {
			return nil, false
		}

		pctx = pctx.WithValues(values)
	}

	evalCtx := evalContextWithoutIncludes(ctx, l, v, s, pctx, file.ConfigPath, file.FunctionCalls())

	includes, decoded := nativeIncludes(body, evalCtx)
	if !decoded {
		return nil, false
	}

	paths := make([]string, 0, len(includes)+1)

	for _, include := range includes {
		if strategy, err := include.GetMergeStrategy(); err != nil || strategy == NoMerge {
			continue
		}

		path := include.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(file.ConfigPath), path)
		}

		paths = append(paths, path)
	}

	return paths, true
}

// pureValues returns the values in dir that a parse of a config in dir reads. It returns false without reading them
// when the values file calls a function [callsOnlyPureFunctions] rejects, and false when reading them fails.
func pureValues(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	dir string,
) (*cty.Value, bool) {
	valuesPath := filepath.Join(dir, valuesFile)

	if vfs.Exists(v.FS, valuesPath) {
		body, err := configBody(ctx, l, v, s, pctx, valuesPath)
		if err != nil {
			return nil, false
		}

		native, ok := body.(*hclsyntax.Body)
		if !ok || !callsOnlyPureFunctions(native) {
			return nil, false
		}
	}

	values, err := ReadValues(ctx, l, v, s, pctx, dir)
	if err != nil {
		return nil, false
	}

	return values, true
}

// pureFunctions returns the names of the functions whose calls have no effect beyond their value and read nothing
// but the filesystem's directory structure and the environment.
var pureFunctions = sync.OnceValue(func() map[string]struct{} {
	names := map[string]struct{}{}

	for name := range openTofuFunctions().Functions {
		if !slices.Contains(perCallFunctionNames, name) {
			names[name] = struct{}{}
		}
	}

	for name := range sharedFunctions().Functions {
		names[name] = struct{}{}
	}

	for _, name := range []string{
		FuncNameFindInParentFolders,
		FuncNameGetTerragruntDir,
		FuncNameGetParentTerragruntDir,
		FuncNameGetOriginalTerragruntDir,
		FuncNamePathRelativeToInclude,
		FuncNamePathRelativeFromInclude,
		FuncNameGetEnv,
		FuncNameGetPlatform,
		FuncNameGetTerraformCommand,
		FuncNameGetTerraformCLIArgs,
	} {
		names[name] = struct{}{}
	}

	return names
})

// callsOnlyPureFunctions reports whether every function node calls is one of [pureFunctions].
func callsOnlyPureFunctions(node hclsyntax.Node) bool {
	pure := true

	walkNodes(node, func(node hclsyntax.Node) {
		if call, ok := node.(*hclsyntax.FunctionCallExpr); ok {
			_, isPure := pureFunctions()[call.Name]
			pure = pure && isPure
		}
	})

	return pure
}

// referencesVariable reports whether an expression in node references the variable name.
func referencesVariable(node hclsyntax.Node, name string) bool {
	references := false

	walkNodes(node, func(node hclsyntax.Node) {
		if traversal, ok := node.(*hclsyntax.ScopeTraversalExpr); ok && traversal.Traversal.RootName() == name {
			references = true
		}
	})

	return references
}

// walkNodes calls visit with node and every node below it.
func walkNodes(node hclsyntax.Node, visit func(node hclsyntax.Node)) {
	diags := hclsyntax.VisitAll(node, func(node hclsyntax.Node) hcl.Diagnostics {
		visit(node)

		return nil
	})
	if diags.HasErrors() {
		panic("walkNodes: a visitor that reports nothing reported " + diags.Error())
	}
}

// nativeIncludes returns the path and merge_strategy of each include block of body, evaluated with evalCtx. It returns
// false when one fails to evaluate to a known string.
func nativeIncludes(body *hclsyntax.Body, evalCtx *hcl.EvalContext) (IncludeConfigs, bool) {
	var includes IncludeConfigs

	for _, block := range body.Blocks {
		if !isIncludeBlock(block) {
			continue
		}

		path, ok := stringAttribute(block.Body, "path", evalCtx)
		if !ok {
			return nil, false
		}

		include := IncludeConfig{Path: path}

		if _, declared := block.Body.Attributes["merge_strategy"]; declared {
			strategy, ok := stringAttribute(block.Body, "merge_strategy", evalCtx)
			if !ok {
				return nil, false
			}

			include.MergeStrategy = &strategy
		}

		includes = append(includes, include)
	}

	return includes, true
}

// isIncludeBlock reports whether block is an include block.
func isIncludeBlock(block *hclsyntax.Block) bool {
	return block.Type == MetadataInclude
}

// stringAttribute returns the value of the attribute name of body as a string, and false when body has no such
// attribute or it does not evaluate with evalCtx to a known string.
func stringAttribute(body *hclsyntax.Body, name string, evalCtx *hcl.EvalContext) (string, bool) {
	attr, ok := body.Attributes[name]
	if !ok {
		return "", false
	}

	value, diags := attr.Expr.Value(evalCtx)
	if diags.HasErrors() || value.IsNull() || !value.IsKnown() || !value.Type().Equals(cty.String) {
		return "", false
	}

	return value.AsString(), true
}

// declares reports whether body declares an attribute or block of one of schemas, or declares one of their names in a
// way that fails to decode.
func declares(body hcl.Body, schemas []*hcl.BodySchema) bool {
	native, isNative := body.(*hclsyntax.Body)

	return slices.ContainsFunc(schemas, func(schema *hcl.BodySchema) bool {
		if isNative {
			return declaresNatively(native, schema)
		}

		content, _, diags := body.PartialContent(schema)

		return diags.HasErrors() || len(content.Attributes) > 0 || len(content.Blocks) > 0
	})
}

// declaresNatively reports whether body has an attribute or block named like an attribute or block of schema.
func declaresNatively(body *hclsyntax.Body, schema *hcl.BodySchema) bool {
	named := func(name string) bool {
		_, isAttribute := body.Attributes[name]

		return isAttribute || slices.ContainsFunc(body.Blocks, func(block *hclsyntax.Block) bool {
			return block.Type == name
		})
	}

	return slices.ContainsFunc(schema.Attributes, func(attr hcl.AttributeSchema) bool { return named(attr.Name) }) ||
		slices.ContainsFunc(schema.Blocks, func(block hcl.BlockHeaderSchema) bool { return named(block.Type) })
}

// declaresRemoteState reports whether the config at cfgPath, a file it merges, or its sibling autoinclude declares a
// remote_state block or attribute. includes is the includes a partial parse of the config with pctx processed. A
// config that fails to parse declares none.
func declaresRemoteState(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	cfgPath string,
	includes map[string]IncludeConfig,
) bool {
	paths := []string{cfgPath}

	for _, include := range includes {
		if strategy, err := include.GetMergeStrategy(); err == nil && strategy != NoMerge {
			paths = append(paths, include.Path)
		}
	}

	return anyDeclares(ctx, l, v, s, pctx, withSiblingAutoInclude(v, pctx, cfgPath, paths), remoteStateSchemas)
}

// FileIAMRoleOptions returns the IAM role options a parse with pctx evaluates a file with when the file's config sets
// fileOptions. A role ARN passed on the CLI wins outright, and otherwise the CLI options merge on top of fileOptions.
func FileIAMRoleOptions(pctx *ParsingContext, fileOptions iam.RoleOptions) iam.RoleOptions {
	if pctx.OriginalIAMRoleOptions.RoleARN != "" {
		return pctx.OriginalIAMRoleOptions
	}

	return iam.MergeRoleOptions(fileOptions, pctx.OriginalIAMRoleOptions)
}

// CompleteTerragruntConfigFile finishes decoding file into cfgFile once its body has been decoded with evalContext.
// decodeErr is the error from that body decode, or nil. It then decodes the dependency blocks with sibling
// autoinclude overrides applied and replaces unknown values in the inputs.
//
// Returns cfgFile with decodeErr when the body decode failed, unless every diagnostic is an attribute access error
// and a sibling autoinclude or a render command will replace the affected values.
func CompleteTerragruntConfigFile(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
	evalContext *hcl.EvalContext,
	cfgFile *TerragruntConfigFile,
	decodeErr error,
) (*TerragruntConfigFile, error) {
	if decodeErr != nil {
		var diagErr hcl.Diagnostics

		ok := errors.As(decodeErr, &diagErr)

		// Suppress attribute access errors when a sibling autoinclude will merge on top, or during render-json/render
		// commands; the autoinclude merge replaces the affected inputs.
		canSuppress := ok && isAttributeAccessError(diagErr) &&
			(isRenderJSONCommand(pctx) || isRenderCommand(pctx) || hasSiblingAutoInclude(pctx))

		if !canSuppress {
			return cfgFile, decodeErr
		}

		l.Debugf("Deferred attribute access error to autoinclude merge: %v", diagErr)
	}

	dependencies, err := DecodeDependencyBlocks(
		ctx,
		l,
		v,
		s,
		pctx,
		file,
		evalContext,
	)
	if err != nil {
		return cfgFile, err
	}

	cfgFile.TerragruntDependencies = dependencies

	if cfgFile.Inputs != nil {
		inputs, err := ctyhelper.UpdateUnknownCtyValValues(*cfgFile.Inputs)
		if err != nil {
			return nil, err
		}

		cfgFile.Inputs = &inputs
	}

	for i := range cfgFile.Variables {
		inputs, err := ctyhelper.UpdateUnknownCtyValValues(cfgFile.Variables[i].Inputs)
		if err != nil {
			return nil, err
		}

		cfgFile.Variables[i].Inputs = inputs
	}

	return cfgFile, nil
}

// ValidateDependencies checks that each path of a dependencies block is a directory, resolving a relative path
// against the working dir of pctx. It checks nothing in a catalog-only parse, which keeps no dependencies.
//
// Returns [DependencyDirNotFoundError] naming every path that is not a directory.
func ValidateDependencies(v *venv.Venv, pctx *ParsingContext, dependencies *ModuleDependencies) error {
	if dependencies == nil || pctx.catalogOnly {
		return nil
	}

	var missingDependencies []string

	for _, dependencyPath := range dependencies.Paths {
		fullPath := filepath.FromSlash(dependencyPath)
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(pctx.WorkingDir, fullPath)
		}

		if !vfs.IsDir(v.FS, fullPath) {
			missingDependencies = append(
				missingDependencies,
				fmt.Sprintf("%s (%s)", dependencyPath, fullPath),
			)
		}
	}

	if len(missingDependencies) > 0 {
		return DependencyDirNotFoundError{missingDependencies}
	}

	return nil
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
		if diagnostic.Severity == hcl.DiagError &&
			strings.Contains(diagnostic.Summary, "Unsupported attribute") {
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

// remoteStateFromAttr decodes a `remote_state` written as an attribute rather than a block.
// JSON configs produce the attribute form.
func remoteStateFromAttr(attr cty.Value) (*remotestate.RemoteState, error) {
	remoteStateMap, err := ctyhelper.ParseCtyValueToMap(attr)
	if err != nil {
		return nil, err
	}

	var config *remotestate.Config

	if err := mapstructure.WeakDecode(remoteStateMap, &config); err != nil {
		return nil, err
	}

	return remotestate.New(config), nil
}

// ConvertToTerragruntConfig converts the contents of a fully resolved Terragrunt configuration to a
// [TerragruntConfig]. For a context built by [ReadCatalogConfig], it converts only the catalog block.
func ConvertToTerragruntConfig(
	v *venv.Venv,
	pctx *ParsingContext,
	cfgPath string,
	cfgFromFile *TerragruntConfigFile,
) (cfg *TerragruntConfig, err error) {
	var errs []error

	if pctx.catalogOnly {
		return convertToTerragruntCatalogConfig(v, pctx, cfgPath, cfgFromFile)
	}

	cfg = &TerragruntConfig{
		// Initialize GenerateConfigs so we can append to it
		GenerateConfigs: map[string]codegen.GenerateConfig{},
	}

	defaultMetadata := map[string]any{FoundInFile: cfgPath}

	if cfgFromFile.RemoteState != nil {
		config, err := cfgFromFile.RemoteState.Config()
		if err != nil {
			errs = append(errs, err)
		}

		cfg.RemoteState = remotestate.New(config)
		cfg.SetFieldMetadata(MetadataRemoteState, defaultMetadata)
	}

	if cfgFromFile.RemoteStateAttr != nil {
		remoteState, err := remoteStateFromAttr(*cfgFromFile.RemoteStateAttr)
		if err != nil {
			return nil, err
		}

		cfg.RemoteState = remoteState
		cfg.SetFieldMetadata(MetadataRemoteState, defaultMetadata)
	}

	if err := cfgFromFile.Terraform.ValidateHooks(); err != nil {
		errs = append(errs, err)
	}

	cfg.Terraform = cfgFromFile.Terraform
	if cfg.Terraform != nil { // since Terraform is nil each time avoid saving metadata when it is nil
		cfg.SetFieldMetadata(MetadataTerraform, defaultMetadata)

		if cfg.Terraform.Source != nil {
			if err := markLocalModuleSourceAsRead(v, pctx, *cfg.Terraform.Source); err != nil {
				errs = append(errs, err)
			}
		}
	}

	cfg.Dependencies = cfgFromFile.Dependencies
	if cfg.Dependencies != nil {
		for _, item := range cfg.Dependencies.Paths {
			cfg.SetFieldMetadataWithType(MetadataDependencies, item, defaultMetadata)
		}
	}

	cfg.TerragruntDependencies = cfgFromFile.TerragruntDependencies
	for _, dep := range cfg.TerragruntDependencies {
		cfg.SetFieldMetadataWithType(MetadataDependency, dep.Name, defaultMetadata)
	}

	if cfgFromFile.TerraformBinary != nil {
		cfg.TerraformBinary = *cfgFromFile.TerraformBinary
		cfg.SetFieldMetadata(MetadataTerraformBinary, defaultMetadata)
	}

	if cfgFromFile.DownloadDir != nil {
		cfg.DownloadDir = *cfgFromFile.DownloadDir
		cfg.SetFieldMetadata(MetadataDownloadDir, defaultMetadata)
	}

	if cfgFromFile.TerraformVersionConstraint != nil {
		cfg.TerraformVersionConstraint = *cfgFromFile.TerraformVersionConstraint
		cfg.SetFieldMetadata(MetadataTerraformVersionConstraint, defaultMetadata)
	}

	if cfgFromFile.TerragruntVersionConstraint != nil {
		cfg.TerragruntVersionConstraint = *cfgFromFile.TerragruntVersionConstraint
		cfg.SetFieldMetadata(MetadataTerragruntVersionConstraint, defaultMetadata)
	}

	if cfgFromFile.PreventDestroy != nil {
		cfg.PreventDestroy = cfgFromFile.PreventDestroy
		cfg.SetFieldMetadata(MetadataPreventDestroy, defaultMetadata)
	}

	if cfgFromFile.IamRole != nil {
		cfg.IamRole = *cfgFromFile.IamRole
		cfg.SetFieldMetadata(MetadataIamRole, defaultMetadata)
	}

	if cfgFromFile.IamAssumeRoleDuration != nil {
		cfg.IamAssumeRoleDuration = cfgFromFile.IamAssumeRoleDuration
		cfg.SetFieldMetadata(MetadataIamAssumeRoleDuration, defaultMetadata)
	}

	if cfgFromFile.IamAssumeRoleSessionName != nil {
		cfg.IamAssumeRoleSessionName = *cfgFromFile.IamAssumeRoleSessionName
		cfg.SetFieldMetadata(MetadataIamAssumeRoleSessionName, defaultMetadata)
	}

	if cfgFromFile.IamWebIdentityToken != nil {
		cfg.IamWebIdentityToken = *cfgFromFile.IamWebIdentityToken
		cfg.SetFieldMetadata(MetadataIamWebIdentityToken, defaultMetadata)
	}

	if cfgFromFile.Engine != nil {
		cfg.Engine = cfgFromFile.Engine
		cfg.SetFieldMetadata(MetadataEngine, defaultMetadata)
	}

	if cfgFromFile.FeatureFlags != nil {
		cfg.FeatureFlags = cfgFromFile.FeatureFlags
		for _, flag := range cfg.FeatureFlags {
			cfg.SetFieldMetadataWithType(
				MetadataFeatureFlag,
				flag.Name,
				defaultMetadata,
			)
		}
	}

	if cfgFromFile.Exclude != nil {
		cfg.Exclude = cfgFromFile.Exclude
		cfg.SetFieldMetadata(MetadataExclude, defaultMetadata)
	}

	if cfgFromFile.Errors != nil {
		cfg.Errors = cfgFromFile.Errors.lastPerLabel()
		cfg.SetFieldMetadata(MetadataErrors, defaultMetadata)
	}

	if cfgFromFile.Outputs != nil {
		if err := cfgFromFile.Outputs.Validate(pctx.Experiments, cfgPath); err != nil {
			errs = append(errs, err)
		}

		cfg.Outputs = cfgFromFile.Outputs
		cfg.SetFieldMetadata(MetadataOutputs, defaultMetadata)
	}

	generateBlocks := []TerragruntGenerateBlock{}
	generateBlocks = append(generateBlocks, cfgFromFile.GenerateBlocks...)

	if cfgFromFile.GenerateAttrs != nil {
		generateMap, err := ctyhelper.ParseCtyValueToMap(*cfgFromFile.GenerateAttrs)
		if err != nil {
			return nil, err
		}

		generateBlocks = slices.Grow(generateBlocks, len(generateMap))

		for name, block := range generateMap {
			var generateBlock TerragruntGenerateBlock
			if err := mapstructure.WeakDecode(block, &generateBlock); err != nil {
				return nil, err
			}

			generateBlock.Name = name
			generateBlocks = append(generateBlocks, generateBlock)
		}
	}

	if err := validateGenerateBlocks(&generateBlocks); err != nil {
		errs = append(errs, err)
	}

	for _, block := range generateBlocks {
		// Validate that if_exists is provided (required attribute)
		if block.IfExists == "" {
			errs = append(
				errs,
				fmt.Errorf(
					"generate block %q is missing required attribute \"if_exists\"",
					block.Name,
				),
			)

			continue
		}

		ifExists, err := codegen.GenerateConfigExistsFromString(block.IfExists)
		if err != nil {
			errs = append(errs, InvalidGenerateBlockError{BlockName: block.Name, Err: err})
			continue
		}

		if block.IfDisabled == nil {
			block.IfDisabled = &DefaultGenerateBlockIfDisabledValueStr
		}

		ifDisabled, err := codegen.GenerateConfigDisabledFromString(*block.IfDisabled)
		if err != nil {
			errs = append(errs, InvalidGenerateBlockError{BlockName: block.Name, Err: err})
			continue
		}

		genConfig := codegen.GenerateConfig{
			HclFmt:        block.HclFmt,
			Mutable:       block.Mutable,
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

		cfg.GenerateConfigs[block.Name] = genConfig
		cfg.SetFieldMetadataWithType(
			MetadataGenerateConfigs,
			block.Name,
			defaultMetadata,
		)
	}

	if cfgFromFile.Inputs != nil {
		inputs, err := ctyhelper.ParseCtyValueToMap(*cfgFromFile.Inputs)
		if err != nil {
			errs = append(errs, err)
		}

		cfg.Inputs = inputs
		cfg.SetFieldMetadataMap(
			MetadataInputs,
			cfg.Inputs,
			defaultMetadata,
		)
	}

	variables, err := convertVariablesBlocks(pctx, cfgPath, cfgFromFile.Variables)
	if err != nil {
		errs = append(errs, err)
	}

	cfg.Variables = variables

	if pctx.Locals != nil && *pctx.Locals != cty.NilVal {
		localsParsed, err := ctyhelper.ParseCtyValueToMap(*pctx.Locals)
		if err != nil {
			return nil, err
		}

		// Only set Locals if there are actual values to avoid setting an empty map
		if len(localsParsed) > 0 {
			cfg.Locals = localsParsed
			cfg.SetFieldMetadataMap(MetadataLocals, localsParsed, defaultMetadata)
		}
	}

	return cfg, errors.Join(errs...)
}

// moduleSourceReadExtensions lists file extensions within a local terraform module
// whose changes should propagate to consuming units via reading-based filters.
var moduleSourceReadExtensions = map[string]struct{}{
	".tf":        {},
	".tf.json":   {},
	".hcl":       {},
	".tofu":      {},
	".tofu.json": {},
}

// markLocalModuleSourceAsRead walks the local terraform module pointed to by
// rawSource and marks its configuration files as read in pctx.FilesRead. Sources
// that are not local or do not parse are skipped, since a genuinely broken source
// will surface during download, and so are entries the walk cannot read. It
// returns the error that stops the walk itself.
//
// A relative source resolves against pctx.WorkingDir. For a unit that is the
// unit's directory, where a run resolves the source, even when an included
// config in another directory declares it. For a file loaded with
// read_terragrunt_config it is that file's directory.
//
// A pctx that keeps no record of its reads gets no walk. The walk feeds nothing
// but that record, and is the most expensive thing a parse does for it, so this
// is where the cost of tracking goes when nobody is asking.
func markLocalModuleSourceAsRead(v *venv.Venv, pctx *ParsingContext, rawSource string) error {
	if !pctx.FilesRead.Tracking() {
		return nil
	}

	moduleDir, ok := localModuleDir(pctx.WorkingDir, rawSource)
	if !ok || !pctx.FilesRead.MarkDirIfNew(moduleDir) {
		return nil
	}

	walkFunc := vfs.WalkDir
	if pctx.Experiments.Evaluate(experiment.Symlinks) {
		walkFunc = vfs.WalkDirWithSymlinks
	}

	err := walkFunc(v.FS, moduleDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Skip unreadable entries rather than aborting the whole walk.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}

			return nil
		}

		if d.IsDir() {
			return util.SkipDirIfIgnorable(d.Name())
		}

		ext := moduleSourceFileExtension(d.Name())
		if _, ok := moduleSourceReadExtensions[ext]; !ok {
			return nil
		}

		pctx.FilesRead.Add(path)

		return nil
	})
	if err != nil {
		return fmt.Errorf("marking files of module source %s as read: %w", moduleDir, err)
	}

	return nil
}

// localModuleDir returns the absolute directory rawSource names, resolving a
// relative source against workingDir. It reports false for a source that is not
// local or does not parse.
func localModuleDir(workingDir, rawSource string) (string, bool) {
	sourceWithoutSubdir, subdir := getter.SourceDirSubdir(rawSource)

	sourceURL, err := tf.ToSourceURL(sourceWithoutSubdir, workingDir)
	if err != nil || !tf.IsLocalSource(sourceURL) {
		return "", false
	}

	moduleDir := filepath.Clean(sourceURL.Path)
	if subdir != "" {
		moduleDir = filepath.Clean(filepath.Join(moduleDir, subdir))
	}

	if !filepath.IsAbs(moduleDir) {
		moduleDir = filepath.Clean(filepath.Join(workingDir, moduleDir))
	}

	return moduleDir, true
}

// moduleSourceFileExtension returns the extension used for matching module source
// files, honoring compound extensions like ".tf.json" and ".tofu.json".
func moduleSourceFileExtension(name string) string {
	for _, compound := range []string{".tf.json", ".tofu.json"} {
		if strings.HasSuffix(name, compound) {
			return compound
		}
	}

	return filepath.Ext(name)
}

// Iterate over generate blocks and detect duplicate names, return error with list of duplicated names
func validateGenerateBlocks(blocks *[]TerragruntGenerateBlock) error {
	var (
		blockNames                   = map[string]struct{}{}
		duplicatedGenerateBlockNames []string
	)

	for _, block := range *blocks {
		_, found := blockNames[block.Name]
		if found {
			duplicatedGenerateBlockNames = append(duplicatedGenerateBlockNames, block.Name)
			continue
		}

		blockNames[block.Name] = struct{}{}
	}

	if len(duplicatedGenerateBlockNames) != 0 {
		return DuplicatedGenerateBlocksError{duplicatedGenerateBlockNames}
	}

	return nil
}

// hasDependencyBlockIn reports whether content, the contents of the config at cfgPath, declares a dependency or
// dependencies block.
func hasDependencyBlockIn(content []byte, cfgPath string) (bool, error) {
	// We use hclwrite to parse the config instead of the normal parser because the normal parser doesn't give us an AST
	// that we can walk and scan, and requires structured data to map against. This makes the parsing strict, so to
	// avoid weird parsing errors due to missing dependency data, we do a structural scan here.
	hclFile, diags := hclwrite.ParseConfig(content, cfgPath, hcl.InitialPos)
	if diags.HasErrors() {
		return false, diags
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
func (cfg *TerragruntConfig) SetFieldMetadataWithType(
	fieldType, fieldName string,
	m map[string]any,
) {
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
func (cfg *TerragruntConfig) SetFieldMetadataMap(
	field string,
	data map[string]any,
	metadata map[string]any,
) {
	for name := range data {
		cfg.SetFieldMetadataWithType(field, name, metadata)
	}
}

// GetFieldMetadata return field metadata by field name.
func (cfg *TerragruntConfig) GetFieldMetadata(fieldName string) (map[string]string, bool) {
	return cfg.GetMapFieldMetadata(fieldName, fieldName)
}

// GetMapFieldMetadata return field metadata by field type and name.
func (cfg *TerragruntConfig) GetMapFieldMetadata(
	fieldType, fieldName string,
) (map[string]string, bool) {
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
func (cfg *TerragruntConfig) EngineOptions() (*engine.EngineConfig, error) {
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

	return &engine.EngineConfig{
		Source:  cfg.Engine.Source,
		Version: version,
		Type:    engineType,
		Meta:    meta,
	}, nil
}

// ErrorsConfig fetch errors configuration for options package
func (cfg *TerragruntConfig) ErrorsConfig() (*errorconfig.Config, error) {
	if cfg.Errors == nil {
		return nil, nil
	}

	result := &errorconfig.Config{}

	for _, retryBlock := range cfg.Errors.Retry {
		if retryBlock == nil {
			continue
		}

		// Validate retry settings
		if retryBlock.MaxAttempts < 1 {
			return nil, fmt.Errorf(
				"cannot have less than 1 max retry in errors.retry %q, but you specified %d",
				retryBlock.Label,
				retryBlock.MaxAttempts,
			)
		}

		if retryBlock.SleepIntervalSec < 0 {
			return nil, fmt.Errorf(
				"cannot sleep for less than 0 seconds in errors.retry %q, but you specified %d",
				retryBlock.Label,
				retryBlock.SleepIntervalSec,
			)
		}

		compiledPatterns := make([]*errorconfig.Pattern, 0, len(retryBlock.RetryableErrors))

		for _, pattern := range retryBlock.RetryableErrors {
			value, err := errorsPattern(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid retry pattern %q in block %q: %w",
					pattern, retryBlock.Label, err)
			}

			compiledPatterns = append(compiledPatterns, value)
		}

		result.Retry = append(result.Retry, &errorconfig.RetryConfig{
			Name:             retryBlock.Label,
			RetryableErrors:  compiledPatterns,
			MaxAttempts:      retryBlock.MaxAttempts,
			SleepIntervalSec: retryBlock.SleepIntervalSec,
		})
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

		compiledPatterns := make([]*errorconfig.Pattern, 0, len(ignoreBlock.IgnorableErrors))

		for _, pattern := range ignoreBlock.IgnorableErrors {
			value, err := errorsPattern(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid ignore pattern %q in block %q: %w",
					pattern, ignoreBlock.Label, err)
			}

			compiledPatterns = append(compiledPatterns, value)
		}

		result.Ignore = append(result.Ignore, &errorconfig.IgnoreConfig{
			Name:            ignoreBlock.Label,
			IgnorableErrors: compiledPatterns,
			Message:         ignoreBlock.Message,
			Signals:         signals,
		})
	}

	return result, nil
}

// Build ErrorsPattern from string
func errorsPattern(pattern string) (*errorconfig.Pattern, error) {
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

	return &errorconfig.Pattern{
		Pattern:  compiled,
		Negative: isNegative,
	}, nil
}

// ParseRemoteState reads the Terragrunt config file from its default location
// and parses and returns the `remote_state` block.
// The caller provides a fully populated ParsingContext (typically via configbridge.NewParsingContext).
func ParseRemoteState(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
) (*remotestate.RemoteState, error) {
	cfg, err := readBackendConfig(ctx, l, v, s, pctx)
	if err != nil {
		l.Debugf(
			"Decoding only the backend blocks of %s failed (%v), reading the whole config instead",
			util.RelPathForLog(
				pctx.RootWorkingDir,
				pctx.TerragruntConfigPath,
				pctx.LogShowAbsPaths,
			),
			err,
		)

		cfg, err = s.ReadTerragruntConfig(ctx, l, v, pctx)
		if err != nil {
			return nil, err
		}
	}

	return cfg.GetRemoteState(ctx, l, v, pctx)
}

// readBackendConfig reads the config and decodes the two things a backend needs: the
// `remote_state` block, and the `terraform` block's `source`, which decides the directory the
// backend operates in. Keeping the decode this narrow lets a backend command run against a unit
// whose dependencies have never been applied.
//
// It fails when `remote_state` itself reads dependency outputs, since nothing here defines the
// `dependency` variable. [ParseRemoteState] handles that by reading the whole config.
func readBackendConfig(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
) (*TerragruntConfig, error) {
	// The whole-config read decides whether this config is valid, so a failure here must not
	// print diagnostics for a command that goes on to succeed.
	quietCtx := pctx.WithDiagnosticsSuppressed()

	iamRoleOptions := pctx.OriginalIAMRoleOptions

	// A whole-config read resolves `iam_role` before decoding anything, so that functions such as
	// get_aws_account_id() called from `remote_state` run under the assumed role.
	if iamRoleOptions.RoleARN == "" {
		flags, err := s.ResolvePartialConfig(
			ctx,
			l,
			v,
			quietCtx.WithDecodeList(TerragruntFlags),
			pctx.TerragruntConfigPath,
		)
		if err != nil {
			return nil, err
		}

		iamRoleOptions = iam.MergeRoleOptions(
			flags.GetIAMRoleOptions(),
			pctx.OriginalIAMRoleOptions,
		)
	}

	backendCtx := quietCtx.WithDecodeList(RemoteStateBlock, TerraformSource)
	backendCtx.IAMRoleOptions = iamRoleOptions

	return s.ResolvePartialConfig(ctx, l, v, backendCtx, pctx.TerragruntConfigPath)
}

// siblingAutoIncludePath returns the path of the sibling terragrunt.autoinclude.hcl beside cfgPath
// and whether it is in scope: the context is not already parsing a file pulled in by an autoinclude
// merge (SkipAutoIncludeMerge, which would recurse and let a pulled-in file fold its own sibling
// autoinclude), and cfgPath is not itself an autoinclude file. The registration that records the
// override on TrackInclude and the partial-parse cache key both route through here. Existence is left
// to the caller so the cache-key path can distinguish absent from present.
func siblingAutoIncludePath(pctx *ParsingContext, cfgPath string) (string, bool) {
	if pctx.SkipAutoIncludeMerge {
		return "", false
	}

	configBase := filepath.Base(cfgPath)
	if configBase == DefaultAutoIncludeFile || configBase == DefaultAutoIncludeStackFile {
		return "", false
	}

	return filepath.Join(filepath.Dir(cfgPath), DefaultAutoIncludeFile), true
}

// hasSiblingAutoInclude reports whether a sibling autoinclude is registered for this parse.
func hasSiblingAutoInclude(pctx *ParsingContext) bool {
	return pctx.includes != nil && pctx.includes.AutoIncludePath != ""
}
