package redesign

import (
	tea "charm.land/bubbletea/v2"
)

// NewCopyFinishedMsgForTest constructs the private copyFinishedMsg so
// external tests can drive Model.Update through the copy-exit code path
// without spinning up a real bubbletea runtime.
func NewCopyFinishedMsgForTest(
	err error,
	workingDir string,
	required []string,
	optional []string,
	valuesWritten, valuesSkipped bool,
) tea.Msg {
	opt := make([]OptionalValue, 0, len(optional))
	for _, name := range optional {
		opt = append(opt, OptionalValue{Name: name})
	}

	return copyFinishedMsg{
		err: err,
		result: copyResult{
			workingDir:    workingDir,
			references:    ValuesReferences{Required: required, Optional: opt},
			valuesWritten: valuesWritten,
			valuesSkipped: valuesSkipped,
		},
	}
}

// NewScaffoldFinishedMsgForTest constructs the private scaffoldFinishedMsg
// for external tests.
func NewScaffoldFinishedMsgForTest(err error) tea.Msg {
	return scaffoldFinishedMsg{err: err}
}

// NewRendererErrMsgForTest constructs the private rendererErrMsg for
// external tests.
func NewRendererErrMsgForTest(err error) tea.Msg {
	return rendererErrMsg{err: err}
}

// MatchesTab reports whether the given entry belongs in the named tab. It
// wraps the package-private tabKind.matches for use in external tests.
func MatchesTab(tab tabKind, entry *ComponentEntry) bool {
	return tab.matches(entry)
}

// LoadingForTest exposes the streaming Model's loading flag so external
// tests can observe whether the (loading...) tab indicator should still
// be rendered.
func LoadingForTest(m Model) bool {
	return m.loading
}

// TagPillRenderForTest renders a single tag pill. selected toggles between
// the unselected and selected color variants.
func TagPillRenderForTest(tag string, selected bool) string {
	return tagPillStyle(tag, selected).Render(tag)
}

// KindForTagForTest exposes kindForTag for external tests.
func KindForTagForTest(tag string) (ComponentKind, bool) {
	return kindForTag(tag)
}

// RenderTagPillsForTest exposes renderTagPills for external tests.
func RenderTagPillsForTest(tags []string, maxWidth int, selected bool) string {
	return renderTagPills(tags, maxWidth, selected)
}

// RenderDetailTagPillsForTest exposes renderDetailTagPills for external tests.
func RenderDetailTagPillsForTest(tags []string) string {
	return renderDetailTagPills(tags)
}

// TagsMarkdownSectionForTest exposes tagsMarkdownSection for external tests.
func TagsMarkdownSectionForTest(tags []string) string {
	return tagsMarkdownSection(tags)
}

// EnvTagsListLayoutForTest exposes the temporary env-var name so tests can
// drive list-layout selection without hard-coding it.
const EnvTagsListLayoutForTest = envTagsListLayout

// EnvTagsDetailStyleForTest exposes the temporary env-var name so tests can
// drive detail-style selection without hard-coding it.
const EnvTagsDetailStyleForTest = envTagsDetailStyle

// ResolveTagsListLayoutMetaForTest is the layout value selected when the
// env map lacks an override.
var ResolveTagsListLayoutMetaForTest = func(env map[string]string) bool {
	return resolveTagsListLayout(env) == tagsListLayoutMeta
}

// ResolveTagsListLayoutRowForTest returns true when the row layout is active.
var ResolveTagsListLayoutRowForTest = func(env map[string]string) bool {
	return resolveTagsListLayout(env) == tagsListLayoutRow
}

// ResolveTagsDetailStylePillsForTest returns true when pills detail style is active.
var ResolveTagsDetailStylePillsForTest = func(env map[string]string) bool {
	return resolveTagsDetailStyle(env) == tagsDetailStylePills
}

// ResolveTagsDetailStyleSectionForTest returns true when section detail style is active.
var ResolveTagsDetailStyleSectionForTest = func(env map[string]string) bool {
	return resolveTagsDetailStyle(env) == tagsDetailStyleSection
}
