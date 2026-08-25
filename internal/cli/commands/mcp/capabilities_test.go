package mcp_test

import (
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseCapabilitiesRefusesAnUnknownName pins that a typo fails rather than
// granting nothing quietly, which would leave an operator believing they had
// opened something up.
func TestParseCapabilitiesRefusesAnUnknownName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"exe", "network", "", "EXEC", "fs_rw"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := tgmcp.ParseCapabilities([]string{name})
			require.ErrorIs(t, err, tgmcp.ErrUnknownCapability)
		})
	}
}

// TestParseCapabilitiesIgnoresSurroundingSpace pins the tolerance a comma
// separated list needs, since --allow=exec, http reaches us with the space
// attached to the second value.
func TestParseCapabilitiesIgnoresSurroundingSpace(t *testing.T) {
	t.Parallel()

	granted, err := tgmcp.ParseCapabilities([]string{" exec", "http "})
	require.NoError(t, err)

	assert.True(t, granted.Has(tgmcp.CapabilityExec))
	assert.True(t, granted.Has(tgmcp.CapabilityHTTP))
}

func TestParseCapabilitiesGrantsWhatWasNamed(t *testing.T) {
	t.Parallel()

	granted, err := tgmcp.ParseCapabilities([]string{"exec", "sops"})
	require.NoError(t, err)

	assert.True(t, granted.Has(tgmcp.CapabilityExec))
	assert.True(t, granted.Has(tgmcp.CapabilitySops))
	assert.False(t, granted.Has(tgmcp.CapabilityHTTP))
}

func TestParseCapabilitiesGrantsNothingByDefault(t *testing.T) {
	t.Parallel()

	granted, err := tgmcp.ParseCapabilities(nil)
	require.NoError(t, err)

	for _, c := range []tgmcp.Capability{
		tgmcp.CapabilityExec, tgmcp.CapabilityHTTP, tgmcp.CapabilitySops,
	} {
		assert.False(t, granted.Has(c), string(c))
	}
}
