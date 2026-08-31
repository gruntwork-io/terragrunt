package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStateOutputsJSONEncryptedState verifies that OpenTofu client-side state encryption is
// reported as its own error rather than as malformed state or as an absent outputs object.
// See https://opentofu.org/docs/language/state/encryption/ for the envelope this recognizes.
func TestStateOutputsJSONEncryptedState(t *testing.T) {
	t.Parallel()

	const location = "gs://state-bucket/environment/service/default.tfstate"

	tcs := []struct {
		name  string
		state string
	}{
		{
			name:  "envelope first",
			state: `{"encrypted_data":"Y2lwaGVydGV4dA==","encryption_version":"v0"}`,
		},
		{
			name:  "envelope after metadata",
			state: `{"serial":3,"lineage":"6d4c9f18","encryption_version":"v0","encrypted_data":"Y2lwaGVydGV4dA=="}`,
		},
		{
			name:  "envelope without a version",
			state: `{"encrypted_data":"Y2lwaGVydGV4dA=="}`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := stateOutputsJSON(strings.NewReader(tc.state), location)

			require.ErrorIs(t, err, ErrDependencyStateEncrypted)

			var encryptedErr DependencyStateEncryptedError
			require.ErrorAs(t, err, &encryptedErr)
			assert.Equal(t, location, encryptedErr.Location)
		})
	}
}

// TestStateOutputsJSONPlaintextState guards the encrypted-state check against firing on state
// that merely mentions encryption in an output name or value.
func TestStateOutputsJSONPlaintextState(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name  string
		state string
		want  string
	}{
		{
			name:  "output named after the envelope",
			state: `{"version":4,"outputs":{"encrypted_data":{"sensitive":false,"type":"string","value":"kept"}}}`,
			want:  `{"encrypted_data":{"sensitive":false,"type":"string","value":"kept"}}`,
		},
		{
			name:  "resource attribute named after the envelope",
			state: `{"version":4,"resources":[{"encrypted_data":"x"}],"outputs":{"value":{"type":"string","value":"kept"}}}`,
			want:  `{"value":{"type":"string","value":"kept"}}`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			outputs, err := stateOutputsJSON(strings.NewReader(tc.state), "gs://state-bucket/service.tfstate")

			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(outputs))
		})
	}
}
