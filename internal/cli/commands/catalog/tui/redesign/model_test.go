package redesign_test

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeComponents(t)
		require.GreaterOrEqual(t, len(components), 2, "need at least 2 components")

		componentCh := make(chan *redesign.ComponentEntry, len(components))
		m := redesign.NewModelStreaming(l, opts, components[len(components)-1], componentCh)
		close(componentCh)

		msgs := make([]tea.Msg, 0, len(components))
		for i := len(components) - 2; i >= 0; i-- {
			msgs = append(msgs, redesign.ComponentMsg(components[i]))
		}

		msgs = append(msgs, tea.KeyPressMsg{Code: 'q', Text: "q"})

		finalModel := driveModel(t, m, 120, 40, msgs).(redesign.Model)

		assert.Equal(t, redesign.ListState, finalModel.State)
		items := finalModel.List().Items()
		assert.Len(t, items, len(components), "all components should be in the list")

		for i := 1; i < len(items); i++ {
			prev := strings.ToLower(items[i-1].(*redesign.ComponentEntry).Title())
			curr := strings.ToLower(items[i].(*redesign.ComponentEntry).Title())
			assert.LessOrEqual(t, prev, curr, "components should be in alphabetical order: %q should come before %q", prev, curr)
		}
	})
}

// makeMixedComponents returns a module entry followed by a template entry
// for tests that need both kinds.
func makeMixedComponents(t *testing.T) []*redesign.ComponentEntry {
	t.Helper()

	return []*redesign.ComponentEntry{
		redesign.NewComponentEntry(redesign.NewComponentForTest(
			redesign.ComponentKindModule,
			"github.com/gruntwork-io/test-repo",
			"modules/aws-vpc",
			"# AWS VPC",
		)).WithSource("github.com/gruntwork-io/test-repo"),
		redesign.NewComponentEntry(redesign.NewComponentForTest(
			redesign.ComponentKindTemplate,
			"github.com/gruntwork-io/test-repo",
			"templates/service",
			"# Service Template",
		)).WithSource("github.com/gruntwork-io/test-repo"),
	}
}

// TestModelTabsFilterByKind verifies that each tab shows only components
// of its kind, while the All tab shows everything.
func TestModelTabsFilterByKind(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeMixedComponents(t)

		componentCh := make(chan *redesign.ComponentEntry, len(components))
		m := redesign.NewModelStreaming(l, opts, components[0], componentCh)
		close(componentCh)

		// Cycle: All -> Templates (first tab after All in the current order).
		msgs := []tea.Msg{
			redesign.ComponentMsg(components[1]),
			tea.KeyPressMsg{Code: tea.KeyTab},
			tea.KeyPressMsg{Code: 'q', Text: "q"},
		}

		finalModel := driveModel(t, m, 120, 40, msgs).(redesign.Model)

		assert.Equal(t, redesign.TabTemplates, finalModel.ActiveTab(), "tab key should cycle to Templates")

		templatesItems := finalModel.List().Items()
		require.Len(t, templatesItems, 1, "Templates tab should contain only the one template")
		assert.Equal(t, redesign.ComponentKindTemplate, templatesItems[0].(*redesign.ComponentEntry).Kind())
	})
}

// TestModelTabShiftTabCycles verifies that shift+tab cycles tabs in
// reverse order.
func TestModelTabShiftTabCycles(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeMixedComponents(t)

		componentCh := make(chan *redesign.ComponentEntry, len(components))
		m := redesign.NewModelStreaming(l, opts, components[0], componentCh)
		close(componentCh)

		// Starts on All. Shift+Tab wraps to the last tab (Stacks).
		msgs := []tea.Msg{
			redesign.ComponentMsg(components[1]),
			tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift},
			tea.KeyPressMsg{Code: 'q', Text: "q"},
		}

		finalModel := driveModel(t, m, 120, 40, msgs).(redesign.Model)

		assert.Equal(t, redesign.TabModules, finalModel.ActiveTab(), "shift+tab from All should wrap to the last tab")
	})
}

// TestModelCopyActionTransitionsToScaffoldState asserts that pressing the
// scaffold key on a copyable component transitions the Model to
// ScaffoldState, which is what dispatches the copy action.
//
// The copy itself is exercised end-to-end in copy_test.go; here we only
// verify the wire-up, because tea.Exec (used by the copy dispatch) only runs
// the underlying ExecCommand inside a real bubbletea runtime.
func TestModelCopyActionTransitionsToScaffoldState(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		repoDir := helpers.TmpDirWOSymlinks(t)

		unitBody := `locals { region = values.region }` + "\n"
		writeFile(t, filepath.Join(repoDir, "vpc", "terragrunt.hcl"), unitBody)

		repo := newFakeRepo(t, repoDir)

		components, err := redesign.NewComponentDiscovery().Discover(repo)
		require.NoError(t, err)
		require.Len(t, components, 1)
		require.Equal(t, redesign.ComponentKindUnit, components[0].Kind)

		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		opts.WorkingDir = t.TempDir()

		entry := redesign.NewComponentEntry(components[0]).WithSource("github.com/gruntwork-io/fake-repo")

		componentCh := make(chan *redesign.ComponentEntry)
		close(componentCh)

		m := redesign.NewModelStreaming(logger.CreateLogger(), opts, entry, componentCh)

		msgs := []tea.Msg{tea.KeyPressMsg{Code: 's', Text: "s"}}

		finalModel := driveModel(t, m, 120, 40, msgs).(redesign.Model)

		assert.Equal(t, redesign.ScaffoldState, finalModel.State,
			"pressing 's' on a unit should transition to ScaffoldState")
	})
}

// TestModelStreamingDeduplicates verifies that sending the same component
// twice does not result in a duplicate entry in the list.
func TestModelStreamingDeduplicates(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeComponents(t)
		require.NotEmpty(t, components)

		componentCh := make(chan *redesign.ComponentEntry, len(components))
		m := redesign.NewModelStreaming(l, opts, components[0], componentCh)
		close(componentCh)

		msgs := []tea.Msg{
			redesign.ComponentMsg(components[0]),
			tea.KeyPressMsg{Code: 'q', Text: "q"},
		}

		finalModel := driveModel(t, m, 120, 40, msgs).(redesign.Model)

		assert.Equal(t, redesign.ListState, finalModel.State)
		assert.Len(t, finalModel.List().Items(), 1, "duplicate component should not appear twice")
	})
}
