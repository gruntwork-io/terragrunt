package redesign

import "strings"

// ComponentEntry wraps a *Component with display-only metadata that the
// redesign TUI needs but that doesn't belong on the shared type.
type ComponentEntry struct {
	Component *Component
	Version   string // resolved semver tag e.g. "v1.2.3", or "" if unknown
	Source    string // clean repo URL e.g. "github.com/gruntwork-io/repo"

	tagsCache    []string
	tagsResolved bool
}

// NewComponentEntry creates a ComponentEntry for the given Component.
// Use WithVersion and WithSource to attach optional metadata.
func NewComponentEntry(c *Component) *ComponentEntry {
	return &ComponentEntry{Component: c}
}

// WithVersion returns the entry with its version set.
func (e *ComponentEntry) WithVersion(version string) *ComponentEntry {
	e.Version = version

	return e
}

// WithSource returns the entry with its source set.
func (e *ComponentEntry) WithSource(source string) *ComponentEntry {
	e.Source = source

	return e
}

// Kind returns the underlying component's kind.
func (e *ComponentEntry) Kind() ComponentKind { return e.Component.Kind }

// FilterValue implements list.Item by delegating to the inner Component.
func (e *ComponentEntry) FilterValue() string { return e.Component.FilterValue() }

// Title implements list.DefaultItem by delegating to the inner Component.
func (e *ComponentEntry) Title() string { return e.Component.Title() }

// Description implements list.DefaultItem by delegating to the inner Component.
func (e *ComponentEntry) Description() string { return e.Component.Description() }

// Tags returns the component's tags, cached on first call so the list
// delegate doesn't re-parse the README on every render.
func (e *ComponentEntry) Tags() []string {
	if !e.tagsResolved {
		e.tagsCache = e.Component.Tags()
		e.tagsResolved = true
	}

	return e.tagsCache
}

// HasTagForKind reports whether any of the entry's tags case-insensitively
// equals the canonical String() for kind. Singular only; no plural matching.
func (e *ComponentEntry) HasTagForKind(kind ComponentKind) bool {
	target := kind.String()

	for _, t := range e.Tags() {
		if strings.EqualFold(t, target) {
			return true
		}
	}

	return false
}
