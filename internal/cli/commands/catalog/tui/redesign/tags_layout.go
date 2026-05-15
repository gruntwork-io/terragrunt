package redesign

import (
	"strings"
)

// tagsListLayout selects how tag pills appear in the catalog list view.
type tagsListLayout int

const (
	// tagsListLayoutMeta appends tag pills to the existing metadata row,
	// truncating with `…` when they don't fit beside the source URL.
	tagsListLayoutMeta tagsListLayout = iota
	// tagsListLayoutRow renders tag pills on their own fourth line below
	// the metadata row, bumping each list item from 3 to 4 lines tall.
	tagsListLayoutRow
)

// tagsDetailStyle selects how tags are rendered in the pager (detail) view.
type tagsDetailStyle int

const (
	// tagsDetailStylePills prepends a row of colored pills above the
	// rendered README body.
	tagsDetailStylePills tagsDetailStyle = iota
	// tagsDetailStyleSection appends a `## Tags` markdown section so
	// glamour renders the tags inline with the rest of the doc.
	tagsDetailStyleSection
)

// envTagsListLayout is a temporary, undocumented environment variable used
// during development to A/B the two list-view tag layouts. Do NOT rely on
// it: it can be removed or have its name changed at any time without notice
// and is not part of Terragrunt's user-facing configuration surface.
const envTagsListLayout = "TG_TMP_CATALOG_TAGS_LIST"

// envTagsDetailStyle is a temporary, undocumented environment variable used
// during development to A/B the two pager-view tag styles. Do NOT rely on
// it: it can be removed or have its name changed at any time without notice
// and is not part of Terragrunt's user-facing configuration surface.
const envTagsDetailStyle = "TG_TMP_CATALOG_TAGS_DETAIL"

// resolveTagsListLayout reads envTagsListLayout from the venv-mediated env
// map and returns the selected layout. Unknown values fall back silently to
// the default (meta).
func resolveTagsListLayout(env map[string]string) tagsListLayout {
	switch strings.ToLower(strings.TrimSpace(env[envTagsListLayout])) {
	case "row":
		return tagsListLayoutRow
	default:
		return tagsListLayoutMeta
	}
}

// resolveTagsDetailStyle reads envTagsDetailStyle from the venv-mediated env
// map and returns the selected style. Unknown values fall back silently to
// the default (pills).
func resolveTagsDetailStyle(env map[string]string) tagsDetailStyle {
	switch strings.ToLower(strings.TrimSpace(env[envTagsDetailStyle])) {
	case "section":
		return tagsDetailStyleSection
	default:
		return tagsDetailStylePills
	}
}
