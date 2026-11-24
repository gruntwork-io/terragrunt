package venv_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/venv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseCapabilitiesRefusesAnUnknownName pins that a misspelled capability
// fails to parse, so an operator is never left believing they granted it.
func TestParseCapabilitiesRefusesAnUnknownName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"exe", "network", "", "EXEC", "fs_rw"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := venv.ParseCapabilities([]string{name}, venv.RestrictCapabilities())
			require.ErrorIs(t, err, venv.ErrUnknownCapability)
		})
	}
}

// TestParseCapabilitiesRefusesANameTheCallerDoesNotAccept pins that the
// accepted list, not the package's own constants, decides what parses.
func TestParseCapabilitiesRefusesANameTheCallerDoesNotAccept(t *testing.T) {
	t.Parallel()

	_, err := venv.ParseCapabilities([]string{"sops"}, []venv.Capability{venv.CapabilityExec})
	require.ErrorIs(t, err, venv.ErrUnknownCapability)
}

// TestParseCapabilitiesAcceptsACallerCapability pins that a caller can grant a
// capability of its own beside the ones Restrict governs.
func TestParseCapabilitiesAcceptsACallerCapability(t *testing.T) {
	t.Parallel()

	const docs venv.Capability = "docs"

	granted, err := venv.ParseCapabilities([]string{"docs"}, append(venv.RestrictCapabilities(), docs))
	require.NoError(t, err)

	assert.True(t, granted.Has(docs))
}

// TestParseCapabilitiesIgnoresSurroundingSpace pins the tolerance a comma
// separated list needs, since --allow=exec, http reaches us with the space
// attached to the second value.
func TestParseCapabilitiesIgnoresSurroundingSpace(t *testing.T) {
	t.Parallel()

	granted, err := venv.ParseCapabilities([]string{" exec", "http "}, venv.RestrictCapabilities())
	require.NoError(t, err)

	assert.True(t, granted.Has(venv.CapabilityExec))
	assert.True(t, granted.Has(venv.CapabilityHTTP))
}

func TestParseCapabilitiesGrantsWhatWasNamed(t *testing.T) {
	t.Parallel()

	granted, err := venv.ParseCapabilities([]string{"exec", "sops"}, venv.RestrictCapabilities())
	require.NoError(t, err)

	assert.True(t, granted.Has(venv.CapabilityExec))
	assert.True(t, granted.Has(venv.CapabilitySops))
	assert.False(t, granted.Has(venv.CapabilityHTTP))
}

func TestParseCapabilitiesGrantsNothingByDefault(t *testing.T) {
	t.Parallel()

	granted, err := venv.ParseCapabilities(nil, venv.RestrictCapabilities())
	require.NoError(t, err)

	for _, c := range venv.RestrictCapabilities() {
		assert.False(t, granted.Has(c), string(c))
	}
}
