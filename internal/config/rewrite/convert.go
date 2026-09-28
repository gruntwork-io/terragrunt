package config

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
)

// queueFile builds a new [pkgconfig.TerragruntConfigFile] from the tagged fields of c and its queue parts q, with
// the run parts left out. Every call allocates new values, since the conversion and include merges mutate what the
// file points to.
func (c *UnitConfig) queueFile(q *QueueBodies) *pkgconfig.TerragruntConfigFile {
	return &pkgconfig.TerragruntConfigFile{
		Catalog:                     c.Catalog.v1(),
		Terraform:                   c.Terraform.v1(),
		TerraformBinary:             clonePtr(c.TerraformBinary),
		TerraformVersionConstraint:  clonePtr(c.TerraformVersionConstraint),
		TerragruntVersionConstraint: clonePtr(c.TerragruntVersionConstraint),
		Dependencies:                c.Dependencies.v1(),
		DownloadDir:                 clonePtr(c.DownloadDir),
		PreventDestroy:              clonePtr(c.PreventDestroy),
		IamRole:                     clonePtr(c.IamRole),
		IamAssumeRoleDuration:       clonePtr(c.IamAssumeRoleDuration),
		IamAssumeRoleSessionName:    clonePtr(c.IamAssumeRoleSessionName),
		IamWebIdentityToken:         clonePtr(c.IamWebIdentityToken),
		FeatureFlags:                convertAll(c.FeatureFlags, (*FeatureFlagHCL).v1),
		Exclude:                     c.Exclude.v1(q.Exclude),
		Errors:                      c.Errors.v1(q),
	}
}

// fill adds the run parts r of c to file, which queueFile built from c.
func (r *RunBodies) fill(c *UnitConfig, file *pkgconfig.TerragruntConfigFile) {
	file.Inputs = r.Inputs
	file.GenerateAttrs = r.GenerateAttrs
	file.RemoteStateAttr = r.RemoteStateAttr
	file.RemoteState = c.RemoteState.v1(r.RemoteState)
	file.Engine = c.Engine.v1(r.Engine)
	file.GenerateBlocks = convertEach(c.GenerateBlocks, r.Generate, (*GenerateHCL).v1)

	if c.Terraform == nil {
		return
	}

	file.Terraform.ExtraArgs = convertEach(c.Terraform.ExtraArgs, r.ExtraArgs, (*ExtraArgumentsHCL).v1)
	file.Terraform.BeforeHooks = convertEach(c.Terraform.BeforeHooks, r.BeforeHooks, (*HookHCL).v1)
	file.Terraform.AfterHooks = convertEach(c.Terraform.AfterHooks, r.AfterHooks, (*HookHCL).v1)
	file.Terraform.ErrorHooks = convertEach(c.Terraform.ErrorHooks, r.ErrorHooks, (*ErrorHookHCL).v1)
}

func (c *CatalogHCL) v1() *pkgconfig.CatalogConfig {
	if c == nil {
		return nil
	}

	return &pkgconfig.CatalogConfig{
		NoShell:         clonePtr(c.NoShell),
		NoHooks:         clonePtr(c.NoHooks),
		DefaultTemplate: c.DefaultTemplate,
		URLs:            slices.Clone(c.URLs),
	}
}

func (e *EngineHCL) v1(body *EngineBody) *pkgconfig.EngineConfig {
	if e == nil {
		return nil
	}

	return &pkgconfig.EngineConfig{
		Version: clonePtr(e.Version),
		Type:    clonePtr(e.Type),
		Meta:    body.Meta,
		Source:  e.Source,
	}
}

// v1 converts the tagged fields of t. The nested blocks are run parts that [RunBodies.fill] adds.
func (t *TerraformHCL) v1() *pkgconfig.TerraformConfig {
	if t == nil {
		return nil
	}

	return &pkgconfig.TerraformConfig{
		Source:                clonePtr(t.Source),
		Version:               clonePtr(t.Version),
		UpdateSourceWithCAS:   clonePtr(t.UpdateSourceWithCAS),
		Mutable:               clonePtr(t.Mutable),
		IncludeInCopy:         cloneSlicePtr(t.IncludeInCopy),
		ExcludeFromCopy:       cloneSlicePtr(t.ExcludeFromCopy),
		CopyTerraformLockFile: clonePtr(t.CopyTerraformLockFile),
	}
}

func (a *ExtraArgumentsHCL) v1(body *ExtraArgumentsBody) pkgconfig.TerraformExtraArguments {
	return pkgconfig.TerraformExtraArguments{
		Arguments:        body.Arguments,
		RequiredVarFiles: body.RequiredVarFiles,
		OptionalVarFiles: body.OptionalVarFiles,
		EnvVars:          body.EnvVars,
		Name:             a.Name,
		Commands:         body.Commands,
	}
}

func (h *HookHCL) v1(body *HookBody) pkgconfig.Hook {
	return pkgconfig.Hook{
		If:             body.If,
		RunOnError:     body.RunOnError,
		SuppressStdout: body.SuppressStdout,
		WorkingDir:     body.WorkingDir,
		Name:           h.Name,
		Commands:       body.Commands,
		Execute:        body.Execute,
	}
}

func (h *ErrorHookHCL) v1(body *ErrorHookBody) pkgconfig.ErrorHook {
	return pkgconfig.ErrorHook{
		SuppressStdout: body.SuppressStdout,
		WorkingDir:     body.WorkingDir,
		Name:           h.Name,
		Commands:       body.Commands,
		Execute:        body.Execute,
		OnErrors:       body.OnErrors,
	}
}

func (r *RemoteStateHCL) v1(body *RemoteStateBody) *remotestate.ConfigFile {
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

func (d *ModuleDependenciesHCL) v1() *pkgconfig.ModuleDependencies {
	if d == nil {
		return nil
	}

	return &pkgconfig.ModuleDependencies{Paths: slices.Clone(d.Paths)}
}

func (e *ExcludeHCL) v1(body *ExcludeBody) *pkgconfig.ExcludeConfig {
	if e == nil {
		return nil
	}

	return &pkgconfig.ExcludeConfig{
		ExcludeDependencies: body.ExcludeDependencies,
		NoRun:               body.NoRun,
		Actions:             body.Actions,
		If:                  body.If,
	}
}

func (e *ErrorsHCL) v1(q *QueueBodies) *pkgconfig.ErrorsConfig {
	if e == nil {
		return nil
	}

	return &pkgconfig.ErrorsConfig{
		Retry:  convertEach(e.Retry, q.Retry, (*RetryHCL).v1),
		Ignore: convertEach(e.Ignore, q.Ignore, (*IgnoreHCL).v1),
	}
}

func (r *RetryHCL) v1(body *RetryBody) *pkgconfig.RetryBlock {
	return &pkgconfig.RetryBlock{
		Label:            r.Label,
		RetryableErrors:  body.RetryableErrors,
		MaxAttempts:      body.MaxAttempts,
		SleepIntervalSec: body.SleepIntervalSec,
	}
}

func (i *IgnoreHCL) v1(body *IgnoreBody) *pkgconfig.IgnoreBlock {
	return &pkgconfig.IgnoreBlock{
		Signals:         body.Signals,
		Label:           i.Label,
		Message:         body.Message,
		IgnorableErrors: body.IgnorableErrors,
	}
}

func (i *Includes) v1() *pkgconfig.TrackInclude {
	if i == nil {
		return nil
	}

	trackInclude := &pkgconfig.TrackInclude{
		CurrentMap:  i.ByName(),
		Original:    i.IncludedVia,
		CurrentList: i.List,
	}

	if i.AutoIncludePath != "" {
		trackInclude.AutoIncludeOverride = &pkgconfig.IncludeConfig{Path: i.AutoIncludePath}
	}

	return trackInclude
}

func (f *FeatureFlagHCL) v1() *pkgconfig.FeatureFlag {
	return &pkgconfig.FeatureFlag{
		Default: clonePtr(f.Default),
		Name:    f.Name,
	}
}

func (g *GenerateHCL) v1(body *GenerateBody) pkgconfig.TerragruntGenerateBlock {
	return pkgconfig.TerragruntGenerateBlock{
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
