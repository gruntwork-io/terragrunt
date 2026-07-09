package tui

import (
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Warning is a warn-or-worse log entry captured while a TUI owns the screen.
// Models receive it as a message and surface it as a toast.
type Warning struct {
	Message string
}

// ToastExpired dismisses the toast with the given ID. Each toast schedules
// its own expiry when pushed.
type ToastExpired struct {
	ID int
}

// toast is a single on-screen notification, identified so its scheduled
// expiry can dismiss it.
type toast struct {
	message string
	id      int
}

const (
	// toastTTL is how long a toast stays on screen before it expires.
	toastTTL = 5 * time.Second

	// maxToasts caps how many toasts are shown at once; pushing past the cap
	// drops the oldest.
	maxToasts = 3

	// toastMaxWidth is a toast's maximum total width, border included.
	toastMaxWidth = 48

	// toastMaxLines caps a toast message's wrapped lines, so one long warning
	// can't cover the screen.
	toastMaxLines = 3

	// toastMarginX insets the toast stack from the terminal's right edge.
	toastMarginX = 2
	// toastMarginY insets the toast stack from the terminal's top edge.
	toastMarginY = 1

	// toastFrameWidth is the horizontal space a toast's border and padding
	// take, leaving the rest of its width for the message.
	toastFrameWidth = 4

	// WarnChannelBuffer is the buffer for channels carrying [Warning]s from a
	// [WarnHook] to a model. The hook drops entries rather than block when
	// the model falls behind draining it.
	WarnChannelBuffer = 32
)

// warningColor is the yellow used for warning toasts.
const warningColor = "#E6DB74"

var toastStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color(warningColor)).
	Foreground(lipgloss.Color(warningColor)).
	Padding(0, 1)

// ToastStack is the stack of floating warning notifications a TUI composites
// over its view, so warnings surface without shifting the layout or being
// written over the alt screen. The zero value is an empty stack.
type ToastStack struct {
	toasts []toast
	lastID int
}

// Push adds a toast with the given message, dropping the oldest once the
// stack is full. The returned command schedules the toast's expiry.
func (s *ToastStack) Push(message string) tea.Cmd {
	s.lastID++
	s.toasts = append(s.toasts, toast{id: s.lastID, message: message})

	if len(s.toasts) > maxToasts {
		s.toasts = s.toasts[len(s.toasts)-maxToasts:]
	}

	id := s.lastID

	return tea.Tick(toastTTL, func(time.Time) tea.Msg {
		return ToastExpired{ID: id}
	})
}

// Drop removes the toast with the given ID; expiry of an already-dropped
// toast is a no-op.
func (s *ToastStack) Drop(id int) {
	s.toasts = slices.DeleteFunc(s.toasts, func(t toast) bool {
		return t.id == id
	})
}

// Overlay composites the active toasts over content, floating in the
// top-right corner of a terminal of the given size. It returns content
// unchanged when there are no toasts or the terminal is too narrow to fit a
// toast's frame.
func (s ToastStack) Overlay(content string, width, height int) string {
	if len(s.toasts) == 0 {
		return content
	}

	stack := s.render(width)
	if stack == "" {
		return content
	}

	x := max(width-lipgloss.Width(stack)-toastMarginX, 0)

	return lipgloss.NewCompositor(
		lipgloss.NewLayer(content),
		lipgloss.NewLayer(stack).X(x).Y(toastMarginY).Z(1),
	).Render()
}

// render renders the active toasts as bordered boxes stacked oldest to
// newest, right edges aligned.
func (s ToastStack) render(width int) string {
	innerWidth := min(toastMaxWidth, width-toastMarginX*2) - toastFrameWidth
	if innerWidth <= 0 {
		return ""
	}

	boxes := make([]string, 0, len(s.toasts))

	for _, t := range s.toasts {
		wrapped := lipgloss.NewStyle().Width(innerWidth).Render("⚠ " + t.message)
		boxes = append(boxes, toastStyle.Render(ClipToPane(wrapped, innerWidth, toastMaxLines)))
	}

	return lipgloss.JoinVertical(lipgloss.Right, boxes...)
}

// ListenForWarnings returns a command that blocks on ch and delivers the next
// warning as a message. Warnings keep coming, so the [Warning] handler should
// re-arm this command after each one. A nil channel returns nil, and a closed
// channel delivers nothing, stopping the re-arming.
func ListenForWarnings(ch <-chan Warning) tea.Cmd {
	if ch == nil {
		return nil
	}

	return func() tea.Msg {
		w, ok := <-ch
		if !ok {
			return nil
		}

		return w
	}
}

// ClipToPane trims rendered content to a pane interior: at most height lines,
// each truncated to width columns with ANSI styling preserved. This keeps long
// lines and tall content from overrunning a pane's frame.
func ClipToPane(s string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}

	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
	}

	return strings.Join(lines, "\n")
}
