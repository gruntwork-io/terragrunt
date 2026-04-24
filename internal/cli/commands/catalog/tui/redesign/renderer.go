package redesign

import (
	"charm.land/glamour/v2"
)

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

	style := "dark"
	if !m.hasDarkBG {
		style = "light"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(m.width),
	)
	if err != nil {
		return m, nil, err
	}

	m.mdRenderer = r
	m.mdRendererWidth = m.width
	m.mdRendererDark = m.hasDarkBG

	return m, r, nil
}
