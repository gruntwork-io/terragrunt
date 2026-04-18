package redesign_test

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runModel starts a tea.Program with the given model, sends messages via
// the interact callback, and returns the final model once the program exits.
func runModel(t *testing.T, m redesign.Model, width, height int, interact func(p *tea.Program)) redesign.Model { //nolint:gocritic
	t.Helper()

	var out bytes.Buffer

	pr, pw, err := os.Pipe()
	require.NoError(t, err)

	defer pr.Close()
	defer pw.Close()

	p := tea.NewProgram(m,
		tea.WithInput(pr),
		tea.WithOutput(&out),
		tea.WithWindowSize(width, height),
		tea.WithColorProfile(colorprofile.TrueColor),
	)

	done := make(chan tea.Model, 1)

	go func() {
		finalModel, err := p.Run()
		assert.NoError(t, err)

		done <- finalModel
	}()

	time.Sleep(50 * time.Millisecond)

	interact(p)

	select {
	case fm := <-done:
		return fm.(redesign.Model)
	case <-time.After(10 * time.Second):
		p.Kill()
		t.Fatal("program did not exit within timeout")

		return redesign.Model{}
	}
}

// makeComponents builds a deterministic list of ComponentEntry values for
// testing. Each entry has a distinct Dir so Title() returns the directory
// basename and sort order is predictable.
func makeComponents(t *testing.T) []*redesign.ComponentEntry {
	t.Helper()

	return []*redesign.ComponentEntry{
		redesign.NewComponentEntry(redesign.NewComponentForTest(
			redesign.ComponentKindModule,
			"github.com/gruntwork-io/test-repo-1",
			"modules/aws-vpc",
			"# AWS VPC Module\nThis module creates a VPC in AWS.",
		)).WithSource("github.com/gruntwork-io/test-repo-1"),
		redesign.NewComponentEntry(redesign.NewComponentForTest(
			redesign.ComponentKindModule,
			"github.com/gruntwork-io/test-repo-2",
			"modules/eks-cluster",
			"# AWS EKS Module\nThis module creates an EKS cluster.",
		)).WithSource("github.com/gruntwork-io/test-repo-2"),
	}
}

// TestModelStreamingInsertsSorted verifies that components sent via componentMsg
// are inserted in alphabetical order in the list.
func TestModelStreamingInsertsSorted(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	components := makeComponents(t)
	require.GreaterOrEqual(t, len(components), 2, "need at least 2 components")

	// Start with the last component alphabetically
	componentCh := make(chan *redesign.ComponentEntry, len(components))
	m := redesign.NewModelStreaming(l, opts, components[len(components)-1], componentCh)

	finalModel := runModel(t, m, 120, 40, func(p *tea.Program) {
		// Send the remaining components in reverse order
		for i := len(components) - 2; i >= 0; i-- {
			p.Send(redesign.ComponentMsg(components[i]))
			time.Sleep(50 * time.Millisecond)
		}

		time.Sleep(100 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	assert.Equal(t, redesign.ListState, finalModel.State)
	items := finalModel.List.Items()
	assert.Len(t, items, len(components), "all components should be in the list")

	for i := 1; i < len(items); i++ {
		prev := strings.ToLower(items[i-1].(*redesign.ComponentEntry).Title())
		curr := strings.ToLower(items[i].(*redesign.ComponentEntry).Title())
		assert.LessOrEqual(t, prev, curr, "components should be in alphabetical order: %q should come before %q", prev, curr)
	}
}

// TestModelStreamingDeduplicates verifies that sending the same component
// twice does not result in a duplicate entry in the list.
func TestModelStreamingDeduplicates(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	components := makeComponents(t)
	require.NotEmpty(t, components)

	componentCh := make(chan *redesign.ComponentEntry, len(components))
	m := redesign.NewModelStreaming(l, opts, components[0], componentCh)

	finalModel := runModel(t, m, 120, 40, func(p *tea.Program) {
		p.Send(redesign.ComponentMsg(components[0]))
		time.Sleep(100 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	assert.Equal(t, redesign.ListState, finalModel.State)
	assert.Len(t, finalModel.List.Items(), 1, "duplicate component should not appear twice")
}
