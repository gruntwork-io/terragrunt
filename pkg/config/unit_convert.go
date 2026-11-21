package config

import (
	"maps"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"

	"github.com/zclconf/go-cty/cty"
)

// queueFile builds a new [TerragruntConfigFile] from the tagged fields of c and its queue parts q, with
// the run parts left out. Every call allocates new values, since the conversion and include merges mutate what the
// file points to.
func (c *UnitConfig) queueFile(q *QueueBodies) *TerragruntConfigFile {
	return &TerragruntConfigFile{
		Catalog:                     c.Catalog.convert(),
		Terraform:                   c.Terraform.convert(),
		TerraformBinary:             clonePtr(c.TerraformBinary),
		TerraformVersionConstraint:  clonePtr(c.TerraformVersionConstraint),
		TerragruntVersionConstraint: clonePtr(c.TerragruntVersionConstraint),
		Dependencies:                c.Dependencies.convert(),
		DownloadDir:                 clonePtr(c.DownloadDir),
		PreventDestroy:              clonePtr(c.PreventDestroy),
		IamRole:                     clonePtr(c.IamRole),
		IamAssumeRoleDuration:       clonePtr(c.IamAssumeRoleDuration),
		IamAssumeRoleSessionName:    clonePtr(c.IamAssumeRoleSessionName),
		IamWebIdentityToken:         clonePtr(c.IamWebIdentityToken),
		FeatureFlags:                convertAll(c.FeatureFlags, (*FeatureFlagHCL).convert),
		Exclude:                     c.Exclude.convert(q.Exclude),
		Engine:                      c.Engine.convert(q.Engine),
		Errors:                      c.Errors.convert(q),
		Outputs:                     c.Outputs.convert(q.MockValues),
	}
}

// fill adds the run parts r of c to file, which queueFile built from c.
func (r *RunBodies) fill(c *UnitConfig, file *TerragruntConfigFile) {
	if r.Exclude != nil {
		file.Exclude = c.Exclude.convert(r.Exclude)
	}

	c.fillRunAttributes(r, file)

	file.Inputs = r.Inputs
	file.GenerateAttrs = r.GenerateAttrs
	file.RemoteStateAttr = r.RemoteStateAttr
	file.RemoteState = c.RemoteState.convert(r.RemoteState)
	file.Engine = c.Engine.convert(r.Engine)
	file.GenerateBlocks = convertEach(c.GenerateBlocks, r.Generate, (*GenerateHCL).convert)
	file.Variables = convertEach(c.VariablesBlocks, r.Variables, (*VariablesHCL).convert)
	file.Outputs = c.Outputs.convert(r.MockValues)

	if c.Terraform == nil {
		return
	}

	file.Terraform.ExtraArgs = convertEach(c.Terraform.ExtraArgs, r.ExtraArgs, (*ExtraArgumentsHCL).convert)
	file.Terraform.BeforeHooks = convertEach(c.Terraform.BeforeHooks, r.BeforeHooks, (*HookHCL).convert)
	file.Terraform.AfterHooks = convertEach(c.Terraform.AfterHooks, r.AfterHooks, (*HookHCL).convert)
	file.Terraform.ErrorHooks = convertEach(c.Terraform.ErrorHooks, r.ErrorHooks, (*ErrorHookHCL).convert)
}

func (c *CatalogHCL) convert() *CatalogConfig {
	if c == nil {
		return nil
	}

	return &CatalogConfig{
		NoShell:         clonePtr(c.NoShell),
		NoHooks:         clonePtr(c.NoHooks),
		DefaultTemplate: c.DefaultTemplate,
		URLs:            slices.Clone(c.URLs),
	}
}

func (e *EngineHCL) convert(body *EngineBody) *EngineConfig {
	if e == nil {
		return nil
	}

	return &EngineConfig{
		Version: clonePtr(e.Version),
		Type:    clonePtr(e.Type),
		Meta:    clonePtr(body.Meta),
		Source:  e.Source,
	}
}

// v1 converts the tagged fields of t. The nested blocks are run parts that [RunBodies.fill] adds.
func (t *TerraformHCL) convert() *TerraformConfig {
	if t == nil {
		return nil
	}

	return &TerraformConfig{
		Source:                clonePtr(t.Source),
		Version:               clonePtr(t.Version),
		UpdateSourceWithCAS:   clonePtr(t.UpdateSourceWithCAS),
		Mutable:               clonePtr(t.Mutable),
		IncludeInCopy:         cloneSlicePtr(t.IncludeInCopy),
		ExcludeFromCopy:       cloneSlicePtr(t.ExcludeFromCopy),
		CopyTerraformLockFile: clonePtr(t.CopyTerraformLockFile),
		NoDotTerragruntCache:  clonePtr(t.NoDotTerragruntCache),
	}
}

func (a *ExtraArgumentsHCL) convert(body *ExtraArgumentsBody) TerraformExtraArguments {
	return TerraformExtraArguments{
		Arguments:        body.Arguments,
		RequiredVarFiles: body.RequiredVarFiles,
		OptionalVarFiles: body.OptionalVarFiles,
		EnvVars:          body.EnvVars,
		Name:             a.Name,
		Commands:         body.Commands,
	}
}

func (h *HookHCL) convert(body *HookBody) Hook {
	return Hook{
		If:             body.If,
		RunOnError:     body.RunOnError,
		SuppressStdout: body.SuppressStdout,
		WorkingDir:     body.WorkingDir,
		Name:           h.Name,
		Commands:       body.Commands,
		Execute:        body.Execute,
	}
}

func (h *ErrorHookHCL) convert(body *ErrorHookBody) ErrorHook {
	return ErrorHook{
		SuppressStdout: body.SuppressStdout,
		WorkingDir:     body.WorkingDir,
		Name:           h.Name,
		Commands:       body.Commands,
		Execute:        body.Execute,
		OnErrors:       body.OnErrors,
	}
}

func (r *RemoteStateHCL) convert(body *RemoteStateBody) *remotestate.ConfigFile {
	if r == nil {
		return nil
	}

	return &remotestate.ConfigFile{
		BackendConfig:                 body.BackendConfig,
		DisableInit:                   clonePtr(r.DisableInit),
		DisableDependencyOptimization: clonePtr(r.DisableDependencyOptimization),
		Generate:                      clonePtr(r.Generate),
		Encryption:                    body.Encryption,
		BackendName:                   r.BackendName,
	}
}

func (d *ModuleDependenciesHCL) convert() *ModuleDependencies {
	if d == nil {
		return nil
	}

	return &ModuleDependencies{Paths: slices.Clone(d.Paths)}
}

// v1 converts e with its decoded body, returning nil when the file has no exclude block or body is nil because the
// block reads `dependency` and [RunBodies.fill] converts it instead.
func (e *ExcludeHCL) convert(body *ExcludeBody) *ExcludeConfig {
	if e == nil || body == nil {
		return nil
	}

	return &ExcludeConfig{
		ExcludeDependencies: clonePtr(body.ExcludeDependencies),
		NoRun:               clonePtr(body.NoRun),
		Actions:             slices.Clone(body.Actions),
		If:                  body.If,
	}
}

func (e *ErrorsHCL) convert(q *QueueBodies) *ErrorsConfig {
	if e == nil {
		return nil
	}

	return &ErrorsConfig{
		Retry:  convertEach(e.Retry, q.Retry, (*RetryHCL).convert),
		Ignore: convertEach(e.Ignore, q.Ignore, (*IgnoreHCL).convert),
	}
}

func (r *RetryHCL) convert(body *RetryBody) *RetryBlock {
	return &RetryBlock{
		Label:            r.Label,
		RetryableErrors:  slices.Clone(body.RetryableErrors),
		MaxAttempts:      body.MaxAttempts,
		SleepIntervalSec: body.SleepIntervalSec,
	}
}

func (i *IgnoreHCL) convert(body *IgnoreBody) *IgnoreBlock {
	return &IgnoreBlock{
		Signals:         maps.Clone(body.Signals),
		Label:           i.Label,
		Message:         body.Message,
		IgnorableErrors: slices.Clone(body.IgnorableErrors),
	}
}

func (o *OutputsHCL) convert(mockValues []*cty.Value) *OutputsConfig {
	if o == nil {
		return nil
	}

	return &OutputsConfig{
		MockStrategy: o.MockStrategy.convert(),
		Outputs:      convertEach(o.Output, mockValues, (*OutputHCL).convert),
	}
}

func (s *MockStrategyHCL) convert() *OutputsMockStrategy {
	if s == nil {
		return nil
	}

	return &OutputsMockStrategy{
		AllowedCommands: cloneSlicePtr(s.AllowedCommands),
		MergeWithState:  clonePtr(s.MergeWithState),
	}
}

func (d *OutputHCL) convert(mockValue **cty.Value) *OutputDeclaration {
	return &OutputDeclaration{
		MockValue: clonePtr(*mockValue),
		Name:      d.Name,
	}
}

func (f *FeatureFlagHCL) convert() *FeatureFlag {
	return &FeatureFlag{
		Default: clonePtr(f.Default),
		Name:    f.Name,
	}
}

func (g *GenerateHCL) convert(body *GenerateBody) TerragruntGenerateBlock {
	return TerragruntGenerateBlock{
		IfDisabled:       body.IfDisabled,
		CommentPrefix:    body.CommentPrefix,
		DisableSignature: body.DisableSignature,
		Disable:          body.Disable,
		HclFmt:           body.HclFmt,
		Mutable:          body.Mutable,
		Name:             g.Name,
		Path:             body.Path,
		IfExists:         body.IfExists,
		Contents:         body.Contents,
	}
}

func (*VariablesHCL) convert(body *VariablesBody) VariablesBlock {
	return VariablesBlock{
		Kind:     clonePtr(body.Kind),
		Nullable: clonePtr(body.Nullable),
		Validate: clonePtr(body.Validate),
		Inputs:   body.Inputs,
	}
}

// convertAll converts each item with v1, returning nil for no items.
func convertAll[T, U any](items []T, v1 func(*T) U) []U {
	if len(items) == 0 {
		return nil
	}

	converted := make([]U, len(items))
	for i := range items {
		converted[i] = v1(&items[i])
	}

	return converted
}

// convertEach converts each item with the body at its index, returning nil for no items.
func convertEach[T, B, U any](items []T, bodies []B, v1 func(*T, *B) U) []U {
	if len(items) == 0 {
		return nil
	}

	converted := make([]U, len(items))
	for i := range items {
		converted[i] = v1(&items[i], &bodies[i])
	}

	return converted
}

// clonePtr returns a pointer to a copy of *p, or nil.
func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}

	return new(*p)
}

// cloneSlicePtr returns a pointer to a copy of *p, or nil.
func cloneSlicePtr[T any](p *[]T) *[]T {
	if p == nil {
		return nil
	}

	return new(slices.Clone(*p))
}
