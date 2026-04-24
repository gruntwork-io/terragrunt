package redesign

import (
	"charm.land/glamour/v2"
)

// markdownRenderer returns a glamour renderer matching the current Model
// width and dark/light setting. It rebuilds only when the width or the
// background preference changes; otherwise the cached renderer is reused.
//
// Building a glamour.TermRenderer compiles a goldmark pipeline and loads the
// chroma theme registry. Doing that per click is the dominant source of
// latency when opening a component's README; caching keeps subsequent opens
// in the same terminal single-frame.
//
// The returned renderer is also stored back on m, so callers that receive a
// Model by value must propagate the returned Model up to their caller.
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
