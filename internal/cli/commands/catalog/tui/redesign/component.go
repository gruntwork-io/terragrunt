package redesign

import (
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
)

const (
	defaultDescription   = "(no description found)"
	maxDescriptionLength = 200
)

// ComponentKind classifies a Component. Modules and templates are scaffoldable
// artifacts; units and stacks are terragrunt configurations that the user
// copies into their working directory.
type ComponentKind int

const (
	// ComponentKindModule is a directory containing .tf files.
	ComponentKindModule ComponentKind = iota
	// ComponentKindTemplate is a directory containing a `.boilerplate/`
	// subdirectory or a top-level `boilerplate.yml`.
	ComponentKindTemplate
	// ComponentKindUnit is a directory containing a `terragrunt.hcl` file.
	ComponentKindUnit
	// ComponentKindStack is a directory containing a `terragrunt.stack.hcl`
	// file.
	ComponentKindStack
)

// String returns the user-visible kind label.
func (k ComponentKind) String() string {
	switch k {
	case ComponentKindTemplate:
		return "template"
	case ComponentKindUnit:
		return "unit"
	case ComponentKindStack:
		return "stack"
	case ComponentKindModule:
		return "module"
	default:
		return "module"
	}
}

// IsCopyable reports whether a component of this kind is installed by copying
// its directory tree into the working directory rather than by scaffolding.
func (k ComponentKind) IsCopyable() bool {
	return k == ComponentKindUnit || k == ComponentKindStack
}

// Component is the redesign-owned representation of a scaffoldable directory
// discovered inside a cloned repository. It is intentionally independent of
// services/catalog/module.Module so the redesign path can evolve without
// coupling to the legacy catalog discovery pipeline.
type Component struct {
	// Doc is always non-nil; use NewComponentDoc/FindComponentDoc.
	Doc *ComponentDoc

	// Repo is the cloned repository this component lives in. It comes from
	// services/catalog/module.NewRepo, which is the only part of the legacy
	// catalog pipeline the redesign reuses (generic clone/git plumbing).
	Repo *module.Repo

	// Dir is the slash-relative path from the repo root. Empty string means
	// the repo root itself is the component.
	Dir string

	cloneURL string
	repoPath string
	url      string

	Kind ComponentKind
}

// Components is a slice of *Component for ergonomic return types.
type Components []*Component

// Title returns the component's display title. It prefers the doc title
// (README front-matter or first heading) and falls back to the directory
// basename.
func (c *Component) Title() string {
	if c.Doc != nil {
		if title := c.Doc.Title(); title != "" {
			return strings.TrimSpace(title)
		}
	}

	if c.Dir == "" {
		return filepath.Base(c.repoPath)
	}

	return filepath.Base(c.Dir)
}

// Description returns a short description for the list view.
func (c *Component) Description() string {
	if c.Doc != nil {
		if desc := c.Doc.Description(maxDescriptionLength); desc != "" {
			return desc
		}
	}

	return defaultDescription
}

// FilterValue is what the list fuzzy-matches against when the user filters.
func (c *Component) FilterValue() string { return c.Title() }

// Tags returns the component's README front-matter tags, in authoring order.
// Returns nil when the component has no Doc or declares no tags.
func (c *Component) Tags() []string {
	if c.Doc == nil {
		return nil
	}

	return c.Doc.Tags()
}

// URL returns the browser-friendly source URL for the component, or an empty
// string if one could not be derived.
func (c *Component) URL() string { return c.url }

// TerraformSourcePath returns the go-getter-style source string
// (baseURL//dir?query) used when scaffolding a unit from this component.
func (c *Component) TerraformSourcePath() string {
	if c.Dir == "" {
		return c.cloneURL
	}

	base, query, _ := strings.Cut(c.cloneURL, "?")

	result := base + "//" + c.Dir
	if query != "" {
		result += "?" + query
	}

	return result
}

// IsMarkDown reports whether the component's README (if any) is Markdown,
// which determines whether we render it through glamour.
func (c *Component) IsMarkDown() bool {
	if c.Doc == nil {
		return false
	}

	return c.Doc.IsMarkDown()
}

// Content returns the component's README content, optionally with tags
// stripped for plain-text rendering.
func (c *Component) Content(stripTags bool) string {
	if c.Doc == nil {
		return ""
	}

	return c.Doc.Content(stripTags)
}

// NewComponentForTest constructs a Component for use in unit tests without
// requiring a cloned repository on disk.
func NewComponentForTest(kind ComponentKind, cloneURL, dir, readme string) *Component {
	var doc *ComponentDoc
	if readme != "" {
		doc = NewComponentDoc(readme, mdExt)
	} else {
		doc = &ComponentDoc{}
	}

	return &Component{
		Doc:      doc,
		Kind:     kind,
		Dir:      dir,
		cloneURL: cloneURL,
	}
}
