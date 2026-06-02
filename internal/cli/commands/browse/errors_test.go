package browse_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBrowseExperimentGate pins the decision the browse command's Before hook
// makes: the command is rejected unless the browse experiment is enabled. It
// also confirms the experiment is registered (EnableExperiment errors otherwise).
func TestBrowseExperimentGate(t *testing.T) {
	t.Parallel()

	exps := experiment.NewExperiments()
	assert.False(t, exps.Evaluate(experiment.BrowseTUI))

	require.NoError(t, exps.EnableExperiment(experiment.BrowseTUI))
	assert.True(t, exps.Evaluate(experiment.BrowseTUI))
}
