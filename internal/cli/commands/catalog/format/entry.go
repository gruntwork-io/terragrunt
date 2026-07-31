package format

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
)

// Entry is the serialized form of a discovered catalog component. Its shape
// is published as a JSON schema at
// https://docs.terragrunt.com/schemas/catalog/v1/schema.json, so field names
// and meanings are part of what consumers can rely on.
type Entry struct {
	Kind            string   `json:"kind"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Source          string   `json:"source"`
	Dir             string   `json:"dir,omitempty"`
	Version         string   `json:"version,omitempty"`
	URL             string   `json:"url,omitempty"`
	ComponentSource string   `json:"component_source"`
	Doc             string   `json:"doc,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Copyable        bool     `json:"copyable,omitempty"`
}

// NewEntry converts a discovered component into its serialized form.
func NewEntry(e *tui.ComponentEntry) *Entry {
	c := e.Component

	return &Entry{
		Kind:            c.Kind.String(),
		Title:           c.Title(),
		Description:     c.Description(),
		Source:          e.Source,
		Dir:             c.Dir,
		Version:         e.Version,
		URL:             c.URL(),
		ComponentSource: c.TerraformSourcePath(),
		Doc:             c.Content(false),
		Tags:            e.Tags(),
		Copyable:        c.Kind.IsCopyable(),
	}
}
