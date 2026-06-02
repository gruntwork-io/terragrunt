package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		// The first size event is the earliest point we render, so load the
		// other entries for the initially visible columns. ensureVisibleOthers
		// is idempotent, so later resizes cost nothing. The preview is width-
		// dependent, so re-render it for the new size.
		m.ensureVisibleOthers()
		m.ensurePreview()

		return m, nil
	case tea.BackgroundColorMsg:
		m.hasDarkBG = msg.IsDark()
		m.ensurePreview()

		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// handleKey applies a navigation key to the model. While the search input is
// open, keys are routed to it instead.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.searching {
		return m.handleSearchKey(msg)
	}

	switch {
	case m.lastQuery != "" && msg.Code == tea.KeyEscape:
		// Escape clears a committed search before it quits the browser.
		m.lastQuery = ""
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Search):
		cmd := m.startSearch()

		return m, cmd
	case key.Matches(msg, m.keys.Up):
		m.moveCursor(-1)
	case key.Matches(msg, m.keys.Down):
		m.moveCursor(1)
	case key.Matches(msg, m.keys.Ascend):
		m.ascend()
	case key.Matches(msg, m.keys.Descend):
		m.descend()
	case key.Matches(msg, m.keys.NextMatch):
		m.nextMatch(1)
	case key.Matches(msg, m.keys.PrevMatch):
		m.nextMatch(-1)
	}

	m.ensurePreview()

	return m, nil
}

// handleSearchKey feeds keys to the open search input. Enter commits the search,
// escape cancels it, and ctrl+c still quits; everything else edits the query and
// jumps the cursor to the first match.
func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Code == 'c' && msg.Mod == tea.ModCtrl:
		return m, tea.Quit
	case msg.Code == tea.KeyEnter:
		m.commitSearch()
		m.ensurePreview()

		return m, nil
	case msg.Code == tea.KeyEscape:
		m.cancelSearch()
		m.ensurePreview()

		return m, nil
	}

	input, cmd := m.searchInput.Update(msg)
	m.searchInput = input
	m.applySearch()
	m.ensurePreview()

	return m, cmd
}

// ascend moves to the current directory's parent, loading the entries needed by
// the newly visible columns.
func (m *Model) ascend() {
	if m.current.parent == nil {
		return
	}

	m.current = m.current.parent
	m.lastQuery = ""
	m.ensureVisibleOthers()
}

// descend enters the highlighted child when it has contents. Plain directories,
// units, and stacks are all real directories on disk, so you can descend into a
// unit or stack to reach its config files. Only plain files are leaves; an
// empty directory has nothing to enter.
func (m *Model) descend() {
	target := m.Selected()
	if target == nil || target.kind == KindFile {
		return
	}

	loadOthers(m.fs, target)

	if len(target.children) == 0 {
		return
	}

	m.current = target
	m.lastQuery = ""
	m.ensureVisibleOthers()
}

// ensurePreview renders the highlighted file's syntax-highlighted preview and
// caches it on the node. It re-renders only when the pane width or the terminal
// background changed since the cached render, and is a no-op for anything that
// isn't a file.
func (m *Model) ensurePreview() {
	sel := m.Selected()
	if sel == nil || sel.kind != KindFile {
		return
	}

	width, _ := m.previewArea()
	if width <= 0 {
		return
	}

	if sel.previewReady && sel.previewWidth == width && sel.previewDark == m.hasDarkBG {
		return
	}

	sel.preview = renderFilePreview(m.fs, sel, width, m.shouldColor, m.hasDarkBG)
	sel.previewWidth = width
	sel.previewDark = m.hasDarkBG
	sel.previewReady = true
}

// ensureVisibleOthers loads filesystem entries for the directories whose
// contents are about to be rendered: the current directory and its parent.
func (m *Model) ensureVisibleOthers() {
	loadOthers(m.fs, m.current)

	if m.current.parent != nil {
		loadOthers(m.fs, m.current.parent)
	}
}
