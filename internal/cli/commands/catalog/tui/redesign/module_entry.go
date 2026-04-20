package redesign

import "github.com/gruntwork-io/terragrunt/internal/services/catalog/module"

// ModuleEntry wraps a *module.Module with display-only metadata
// that the redesign TUI needs but that doesn't belong on the shared type.
type ModuleEntry struct {
	Module   *module.Module
	Version  string // resolved semver tag e.g. "v1.2.3", or "" if unknown
	Source   string // clean repo URL e.g. "github.com/gruntwork-io/repo"
	ItemType string // "module" for now
}

// NewModuleEntry creates a ModuleEntry with ItemType set to "module".
// Use WithVersion and WithSource to attach optional metadata.
func NewModuleEntry(mod *module.Module) *ModuleEntry {
	return &ModuleEntry{
		Module:   mod,
		ItemType: "module",
	}
}

// WithVersion returns the entry with its version set.
func (e *ModuleEntry) WithVersion(version string) *ModuleEntry {
	e.Version = version

	return e
}

// WithSource returns the entry with its source set.
func (e *ModuleEntry) WithSource(source string) *ModuleEntry {
	e.Source = source

	return e
}

// FilterValue implements list.Item by delegating to the inner Module.
func (e *ModuleEntry) FilterValue() string { return e.Module.FilterValue() }

// Title implements list.DefaultItem by delegating to the inner Module.
func (e *ModuleEntry) Title() string { return e.Module.Title() }

// Description implements list.DefaultItem by delegating to the inner Module.
func (e *ModuleEntry) Description() string { return e.Module.Description() }
