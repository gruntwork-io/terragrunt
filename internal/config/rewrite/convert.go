package config

import (
	"maps"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
)

// v1File builds a new [pkgconfig.TerragruntConfigFile] from c. Every call allocates new values, since the conversion
// and include merges mutate what the file points to.
func (c *UnitConfig) v1File() *pkgconfig.TerragruntConfigFile {
	return &pkgconfig.TerragruntConfigFile{
		Catalog:                     c.Catalog.v1(),
		Engine:                      c.Engine.v1(),
		Terraform:                   c.Terraform.v1(),
		TerraformBinary:             clonePtr(c.TerraformBinary),
		TerraformVersionConstraint:  clonePtr(c.TerraformVersionConstraint),
		TerragruntVersionConstraint: clonePtr(c.TerragruntVersionConstraint),
		Inputs:                      clonePtr(c.Body.Inputs),
		RemoteState:                 c.RemoteState.v1(),
		RemoteStateAttr:             clonePtr(c.Body.RemoteStateAttr),
		Dependencies:                c.Dependencies.v1(),
		DownloadDir:                 clonePtr(c.DownloadDir),
		PreventDestroy:              clonePtr(c.PreventDestroy),
		IamRole:                     clonePtr(c.IamRole),
		IamAssumeRoleDuration:       clonePtr(c.IamAssumeRoleDuration),
		IamAssumeRoleSessionName:    clonePtr(c.IamAssumeRoleSessionName),
		IamWebIdentityToken:         clonePtr(c.IamWebIdentityToken),
		FeatureFlags:                convertAll(c.FeatureFlags, (*FeatureFlag).v1),
		Exclude:                     c.Exclude.v1(),
		Errors:                      c.Errors.v1(),
		GenerateAttrs:               clonePtr(c.Body.GenerateAttrs),
		GenerateBlocks:              convertAll(c.Body.Generate, (*Generate).v1),
	}
}

func (c *Catalog) v1() *pkgconfig.CatalogConfig {
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

func (e *Engine) v1() *pkgconfig.EngineConfig {
	if e == nil {
		return nil
	}

	return &pkgconfig.EngineConfig{
		Version: clonePtr(e.Version),
		Type:    clonePtr(e.Type),
		Meta:    clonePtr(e.Body.Meta),
		Source:  e.Source,
	}
}

func (t *Terraform) v1() *pkgconfig.TerraformConfig {
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
		ExtraArgs:             convertAll(t.ExtraArgs, (*ExtraArguments).v1),
		BeforeHooks:           convertAll(t.BeforeHooks, (*Hook).v1),
		AfterHooks:            convertAll(t.AfterHooks, (*Hook).v1),
		ErrorHooks:            convertAll(t.ErrorHooks, (*ErrorHook).v1),
	}
}

func (a *ExtraArguments) v1() pkgconfig.TerraformExtraArguments {
	var envVars *map[string]string
	if a.Body.EnvVars != nil {
		envVars = new(maps.Clone(*a.Body.EnvVars))
	}

	return pkgconfig.TerraformExtraArguments{
		Arguments:        cloneSlicePtr(a.Body.Arguments),
		RequiredVarFiles: cloneSlicePtr(a.Body.RequiredVarFiles),
		OptionalVarFiles: cloneSlicePtr(a.Body.OptionalVarFiles),
		EnvVars:          envVars,
		Name:             a.Name,
		Commands:         slices.Clone(a.Body.Commands),
	}
}

func (h *Hook) v1() pkgconfig.Hook {
	return pkgconfig.Hook{
		If:             clonePtr(h.Body.If),
		RunOnError:     clonePtr(h.Body.RunOnError),
		SuppressStdout: clonePtr(h.Body.SuppressStdout),
		WorkingDir:     clonePtr(h.Body.WorkingDir),
		Name:           h.Name,
		Commands:       slices.Clone(h.Body.Commands),
		Execute:        slices.Clone(h.Body.Execute),
	}
}

func (h *ErrorHook) v1() pkgconfig.ErrorHook {
	return pkgconfig.ErrorHook{
		SuppressStdout: clonePtr(h.Body.SuppressStdout),
		WorkingDir:     clonePtr(h.Body.WorkingDir),
		Name:           h.Name,
		Commands:       slices.Clone(h.Body.Commands),
		Execute:        slices.Clone(h.Body.Execute),
		OnErrors:       slices.Clone(h.Body.OnErrors),
	}
}

func (r *RemoteState) v1() *remotestate.ConfigFile {
	if r == nil {
		return nil
	}

	return &remotestate.ConfigFile{
		BackendConfig:                 r.Body.BackendConfig,
		DisableInit:                   clonePtr(r.DisableInit),
		DisableDependencyOptimization: clonePtr(r.DisableDependencyOptimization),
		Generate:                      clonePtr(r.Generate),
		Encryption:                    clonePtr(r.Body.Encryption),
		BackendName:                   r.BackendName,
	}
}

func (d *ModuleDependencies) v1() *pkgconfig.ModuleDependencies {
	if d == nil {
		return nil
	}

	return &pkgconfig.ModuleDependencies{Paths: slices.Clone(d.Paths)}
}

func (e *Exclude) v1() *pkgconfig.ExcludeConfig {
	if e == nil {
		return nil
	}

	return &pkgconfig.ExcludeConfig{
		ExcludeDependencies: clonePtr(e.ExcludeDependencies),
		NoRun:               clonePtr(e.NoRun),
		Actions:             slices.Clone(e.Actions),
		If:                  e.If,
	}
}

func (e *Errors) v1() *pkgconfig.ErrorsConfig {
	if e == nil {
		return nil
	}

	return &pkgconfig.ErrorsConfig{
		Retry:  convertAll(e.Retry, (*Retry).v1),
		Ignore: convertAll(e.Ignore, (*Ignore).v1),
	}
}

func (r *Retry) v1() *pkgconfig.RetryBlock {
	return &pkgconfig.RetryBlock{
		Label:            r.Label,
		RetryableErrors:  slices.Clone(r.Body.RetryableErrors),
		MaxAttempts:      r.Body.MaxAttempts,
		SleepIntervalSec: r.Body.SleepIntervalSec,
	}
}

func (i *Ignore) v1() *pkgconfig.IgnoreBlock {
	return &pkgconfig.IgnoreBlock{
		Signals:         maps.Clone(i.Body.Signals),
		Label:           i.Label,
		Message:         i.Body.Message,
		IgnorableErrors: slices.Clone(i.Body.IgnorableErrors),
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

func (f *FeatureFlag) v1() *pkgconfig.FeatureFlag {
	return &pkgconfig.FeatureFlag{
		Default: clonePtr(f.Default),
		Name:    f.Name,
	}
}

func (g *Generate) v1() pkgconfig.TerragruntGenerateBlock {
	return pkgconfig.TerragruntGenerateBlock{
		IfDisabled:       clonePtr(g.IfDisabled),
		CommentPrefix:    clonePtr(g.CommentPrefix),
		DisableSignature: clonePtr(g.DisableSignature),
		Disable:          clonePtr(g.Disable),
		HclFmt:           clonePtr(g.HclFmt),
		Mutable:          clonePtr(g.Mutable),
		Name:             g.Name,
		Path:             g.Path,
		IfExists:         g.IfExists,
		Contents:         g.Contents,
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
