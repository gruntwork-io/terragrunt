package redesign

import (
	"os"
	"strings"
)

// TagsListLayout selects how tag pills appear in the catalog list view.
type TagsListLayout int

const (
	// TagsListLayoutMeta appends tag pills to the existing metadata row,
	// truncating with `…` when they don't fit beside the source URL.
	TagsListLayoutMeta TagsListLayout = iota
	// TagsListLayoutRow renders tag pills on their own fourth line below
	// the metadata row, bumping each list item from 3 to 4 lines tall.
	TagsListLayoutRow
)

// TagsDetailStyle selects how tags are rendered in the pager (detail) view.
type TagsDetailStyle int

const (
	// TagsDetailStylePills prepends a row of colored pills above the
	// rendered README body.
	TagsDetailStylePills TagsDetailStyle = iota
	// TagsDetailStyleSection appends a `## Tags` markdown section so
	// glamour renders the tags inline with the rest of the doc.
	TagsDetailStyleSection
)

// EnvTagsListLayout is a temporary, undocumented environment variable used
// during development to A/B the two list-view tag layouts. Do NOT rely on
// it: it can be removed or have its name changed at any time without notice
// and is not part of Terragrunt's user-facing configuration surface.
const EnvTagsListLayout = "TG_TMP_CATALOG_TAGS_LIST"

// EnvTagsDetailStyle is a temporary, undocumented environment variable used
// during development to A/B the two pager-view tag styles. Do NOT rely on
// it: it can be removed or have its name changed at any time without notice
// and is not part of Terragrunt's user-facing configuration surface.
const EnvTagsDetailStyle = "TG_TMP_CATALOG_TAGS_DETAIL"

// ResolveTagsListLayout reads EnvTagsListLayout and returns the selected
// layout. Unknown values fall back silently to the default (meta).
func ResolveTagsListLayout() TagsListLayout {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvTagsListLayout))) {
	case "row":
		return TagsListLayoutRow
	default:
		return TagsListLayoutMeta
	}
}

// ResolveTagsDetailStyle reads EnvTagsDetailStyle and returns the selected
// style. Unknown values fall back silently to the default (pills).
func ResolveTagsDetailStyle() TagsDetailStyle {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvTagsDetailStyle))) {
	case "section":
		return TagsDetailStyleSection
	default:
		return TagsDetailStylePills
	}
}
