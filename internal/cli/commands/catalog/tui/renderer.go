package tui

import (
	"github.com/gruntwork-io/terragrunt/internal/md"
)

// markdownRenderer returns a renderer matching the current width and
// dark/light setting, reusing a cached one when both still match.
//
// The cache lives on the Model, which is passed by value, so callers must
// propagate the returned Model upward; otherwise the cache write is lost on
// the next copy.
func (m Model) markdownRenderer() (Model, *md.TerminalRenderer, error) {
	if m.mdRenderer != nil && m.mdRendererWidth == m.width && m.mdRendererDark == m.hasDarkBG {
		return m, m.mdRenderer, nil
	}

	wrap := m.width
	if !m.softWrap {
		wrap = md.NoWrapWidth
	}

	background := md.LightBackground
	if m.hasDarkBG {
		background = md.DarkBackground
	}

	r, err := md.NewTerminalRenderer(wrap, background)
	if err != nil {
		return m, nil, err
	}

	m.mdRenderer = r
	m.mdRendererWidth = m.width
	m.mdRendererDark = m.hasDarkBG

	return m, r, nil
}
