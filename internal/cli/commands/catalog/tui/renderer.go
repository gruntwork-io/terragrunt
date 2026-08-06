package tui

import (
	"charm.land/glamour/v2"

	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
)

// glamourDocumentMargin matches glamour's standard document.margin so
// prepended content aligns with the rendered body.
const glamourDocumentMargin = 2

// noWrapColumnWidth is the wrap column glamour gets when soft-wrap is off.
// Glamour's word-wrap can't be disabled outright (`0` collapses the
// document), so a very wide value preserves the author's line breaks and
// lets the viewport letterbox anything that overruns the terminal.
const noWrapColumnWidth = 1 << 14

// markdownRenderer returns a renderer matching the current width and
// dark/light setting, reusing a cached one when both still match.
//
// The cache lives on the Model, which is passed by value, so callers must
// propagate the returned Model upward; otherwise the cache write is lost on
// the next copy.
func (m Model) markdownRenderer() (Model, *glamour.TermRenderer, error) {
	if m.mdRenderer != nil && m.mdRendererWidth == m.width && m.mdRendererDark == m.hasDarkBG {
		return m, m.mdRenderer, nil
	}

	wrap := m.width
	if !m.softWrap {
		wrap = noWrapColumnWidth
	}

	r, err := viewtui.NewMarkdownRenderer(wrap, m.hasDarkBG)
	if err != nil {
		return m, nil, err
	}

	m.mdRenderer = r
	m.mdRendererWidth = m.width
	m.mdRendererDark = m.hasDarkBG

	return m, r, nil
}
