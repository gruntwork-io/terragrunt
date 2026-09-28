package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
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
	case DiscoveryResult:
		m.disc.apply(msg, m.root)
		m.ensurePreview()

		if msg.Err != nil {
			// The full error is logged at debug by the browse command once the
			// browser exits; the toast just flags that the tree may be incomplete.
			return m, m.toasts.Push("discovery failed; showing partial results")
		}

		return m, nil
	case viewtui.Warning:
		return m, tea.Batch(m.toasts.Push(msg.Message), viewtui.ListenForWarnings(m.warnCh))
	case viewtui.ToastExpired:
		m.toasts.Drop(msg.ID)

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

	// gg chord: the first g arms the jump, the second performs it, and any
	// other key in between disarms it.
	gPending := m.gPending
	m.gPending = false

	switch {
	case m.lastQuery != "" && msg.Code == tea.KeyEscape:
		// Escape clears a committed search before it quits the browser.
		m.lastQuery = ""
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Search):
		cmd := m.startSearch()

		return m, cmd
	case key.Matches(msg, m.keys.Top) && gPending:
		m.moveCursor(-len(m.current.children))
	case key.Matches(msg, m.keys.Top):
		m.gPending = true
	case key.Matches(msg, m.keys.Home):
		m.moveCursor(-len(m.current.children))
	case key.Matches(msg, m.keys.Bottom):
		m.moveCursor(len(m.current.children))
	case key.Matches(msg, m.keys.PageUp):
		m.moveCursor(-m.pageSize())
	case key.Matches(msg, m.keys.PageDown):
		m.moveCursor(m.pageSize())
	case key.Matches(msg, m.keys.Up):
		m.moveCursor(-1)
	case key.Matches(msg, m.keys.Down):
		m.moveCursor(1)
	case key.Matches(msg, m.keys.Ascend):
		m.ascend()
	case key.Matches(msg, m.keys.Descend):
		m.descend()
	case key.Matches(msg, m.keys.NextMatch):
		m.nextMatch(searchForward)
	case key.Matches(msg, m.keys.PrevMatch):
		m.nextMatch(searchBackward)
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

	m.loadDir(target)

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

	sel.preview = m.renderFilePreview(sel, width)
	sel.previewWidth = width
	sel.previewDark = m.hasDarkBG
	sel.previewReady = true
}

// ensureVisibleOthers loads filesystem entries for the directories whose
// contents are about to be rendered: the current directory and its parent.
func (m *Model) ensureVisibleOthers() {
	m.loadDir(m.current)

	if m.current.parent != nil {
		m.loadDir(m.current.parent)
	}
}
