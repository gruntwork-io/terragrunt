package form

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
)

// ErrCancelled reports that the user dismissed the form without submitting
// it. Match with errors.Is to tell an abandoned form from a failed one.
var ErrCancelled = errors.New("form cancelled")

// Run draws the form on the terminal and returns the values the user filled
// in, keyed by field name. Fields the user left empty are absent, so the
// caller's own default applies to them. It returns [ErrCancelled] when the
// user dismisses the form.
//
// It is how a command reaches the form the catalog user interface embeds in
// its own screen, so the two ask for the same values the same way.
func Run(ctx context.Context, title string, fields []Field) (map[string]string, error) {
	final, err := tea.NewProgram(NewRunner(title, fields), tea.WithContext(ctx)).Run()
	if err != nil {
		return nil, err
	}

	runner, ok := final.(*Runner)
	if !ok || runner.Cancelled() {
		return nil, ErrCancelled
	}

	return runner.Values(), nil
}

// Runner is the whole program when the form is run on its own: it owns the
// terminal the catalog user interface would otherwise own, and ends the
// program on the messages that interface would handle by changing screens.
type Runner struct {
	form      *Model
	values    map[string]string
	cancelled bool
}

// NewRunner returns a form that runs as a program of its own.
func NewRunner(title string, fields []Field) *Runner {
	return &Runner{form: NewModel(title, fields)}
}

// Values returns what the user submitted, and nil until they do.
func (r *Runner) Values() map[string]string {
	return r.values
}

// Cancelled reports whether the user dismissed the form.
func (r *Runner) Cancelled() bool {
	return r.cancelled
}

// Init implements [tea.Model].
func (r *Runner) Init() tea.Cmd {
	return nil
}

// Update implements [tea.Model].
func (r *Runner) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.form.SetSize(msg.Width, msg.Height)

		return r, nil

	case SubmitMsg:
		r.values = msg.Values

		return r, tea.Quit

	case CancelMsg:
		r.cancelled = true

		return r, tea.Quit
	}

	form, cmd := r.form.Update(msg)
	r.form = form

	return r, cmd
}

// View implements [tea.Model]. The form takes the alternate screen, as it
// does inside the catalog user interface, so the terminal the user was
// looking at is restored once they are done with it.
func (r *Runner) View() tea.View {
	v := tea.NewView(r.form.View())
	v.AltScreen = true

	return v
}
