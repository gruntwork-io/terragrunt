package discovery

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateNoCoexistence_NoConflict(t *testing.T) {
	t.Parallel()

	results := []DiscoveryResult{
		{Component: component.NewUnit("/a")},
		{Component: component.NewStack("/b")},
		{Component: component.NewUnit("/c")},
	}

	err := validateNoCoexistence(results)
	require.NoError(t, err)
}

func TestValidateNoCoexistence_SameKindDuplicate(t *testing.T) {
	t.Parallel()

	results := []DiscoveryResult{
		{Component: component.NewUnit("/a")},
		{Component: component.NewUnit("/a")},
	}

	err := validateNoCoexistence(results)
	require.NoError(t, err, "same kind at same path should not error")
}

func TestValidateNoCoexistence_UnitAndStackConflict(t *testing.T) {
	t.Parallel()

	results := []DiscoveryResult{
		{Component: component.NewUnit("/app")},
		{Component: component.NewStack("/app")},
	}

	err := validateNoCoexistence(results)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/app")
	assert.Contains(t, err.Error(), "a directory must be either a unit or a stack, not both")
}

func TestValidateNoCoexistence_Empty(t *testing.T) {
	t.Parallel()

	err := validateNoCoexistence(nil)
	require.NoError(t, err)
}
