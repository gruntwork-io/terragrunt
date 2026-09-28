package form_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/view/tui/form"
)

func testFields() []form.Field {
	return []form.Field{
		{Name: "name", TypeStr: "string", Required: true},
		{Name: "region", TypeStr: "string", Initial: `"us-east-1"`},
	}
}

func TestRunnerSubmitCarriesValues(t *testing.T) {
	t.Parallel()

	runner := form.NewRunner("vpc", testFields())

	assert.Nil(t, runner.Values(), "nothing is collected until the form is submitted")
	assert.False(t, runner.Cancelled())

	model, cmd := runner.Update(form.SubmitMsg{Values: map[string]string{"name": `"prod"`}})

	assert.Same(t, runner, model)
	assert.Equal(t, map[string]string{"name": `"prod"`}, runner.Values())
	assert.False(t, runner.Cancelled())
	assertQuits(t, cmd)
}

func TestRunnerCancel(t *testing.T) {
	t.Parallel()

	runner := form.NewRunner("vpc", testFields())

	model, cmd := runner.Update(form.CancelMsg{})

	assert.Same(t, runner, model)
	assert.True(t, runner.Cancelled())
	assert.Nil(t, runner.Values())
	assertQuits(t, cmd)
}

// TestRunnerResizesTheForm covers the message a program receives before its
// first frame: without it the form would render at a zero size.
func TestRunnerResizesTheForm(t *testing.T) {
	t.Parallel()

	runner := form.NewRunner("vpc", testFields())

	_, cmd := runner.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	assert.Nil(t, cmd)

	assert.Contains(t, runner.View().Content, "vpc", "the form renders once sized")
}

// TestRunnerForwardsKeysToTheForm checks the pass-through path: a key the
// runner does not act on reaches the form and moves its cursor.
func TestRunnerForwardsKeysToTheForm(t *testing.T) {
	t.Parallel()

	runner := form.NewRunner("vpc", testFields())
	runner.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	rendered := runner.View().Content
	require.Contains(t, rendered, "name")

	runner.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})

	assert.NotEqual(t, rendered, runner.View().Content, "the cursor moved")
}

func assertQuits(t *testing.T, cmd tea.Cmd) {
	t.Helper()

	require.NotNil(t, cmd, "the program must end")
	assert.IsType(t, tea.QuitMsg{}, cmd())
}
