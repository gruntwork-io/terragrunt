package controls_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDeprecatedReplacedCommand(t *testing.T) {
	t.Parallel()

	ctrl := controls.NewDeprecatedReplacedCommand("old", "new")

	assert.Equal(t, "old", ctrl.Name)
	assert.Equal(t, "replaced with: new", ctrl.Description)
	require.Error(t, ctrl.Error)
	assert.NotEmpty(t, ctrl.Warning)
}

func TestNewDeprecatedCommand(t *testing.T) {
	t.Parallel()

	ctrl := controls.NewDeprecatedCommand("legacy")

	assert.Equal(t, "legacy", ctrl.Name)
	assert.Equal(t, "no replaced command", ctrl.Description)
	require.Error(t, ctrl.Error)
	assert.NotEmpty(t, ctrl.Warning)
}
