package list_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUIExperimentError(t *testing.T) {
	t.Parallel()

	err := list.NewTUIExperimentError()

	var target *list.TUIExperimentError
	require.ErrorAs(t, err, &target)
	require.ErrorIs(t, err, &list.TUIExperimentError{})
}

// TestLsTUIExperimentGate pins the decision the list command's Before hook
// makes: --tui is rejected unless the ls-tui experiment is enabled. It also
// confirms the experiment is registered (EnableExperiment errors otherwise).
func TestLsTUIExperimentGate(t *testing.T) {
	t.Parallel()

	exps := experiment.NewExperiments()
	assert.False(t, exps.Evaluate(experiment.LsTUI))

	require.NoError(t, exps.EnableExperiment(experiment.LsTUI))
	assert.True(t, exps.Evaluate(experiment.LsTUI))
}
