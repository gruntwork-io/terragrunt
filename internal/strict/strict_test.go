package strict_test

// Add some basic tests that confirm that by default, a warning is emitted when strict mode is disabled,
// and an error is emitted when a specific control is enabled.
// Make sure to test both when the specific control is enabled, and when the global strict mode is enabled.

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictControl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		enableControl    bool
		enableStrictMode bool
		expectedErr      error
	}{
		{
			name:             "control enabled",
			enableControl:    true,
			enableStrictMode: false,
			expectedErr:      strict.StrictControls[strict.PlanAll].Error,
		},
		{
			name:             "control disabled",
			enableControl:    false,
			enableStrictMode: false,
			expectedErr:      nil,
		},
		{
			name:             "control enabled, strict mode enabled",
			enableControl:    true,
			enableStrictMode: true,
			expectedErr:      strict.StrictControls[strict.PlanAll].Error,
		},
		{
			name:             "control disabled, strict mode enabled",
			enableControl:    false,
			enableStrictMode: true,
			expectedErr:      strict.StrictControls[strict.PlanAll].Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.TerragruntOptions{}

			if tt.enableControl {
				opts.StrictMode = true
			}

			if tt.enableStrictMode {
				opts.StrictControls = []string{strict.PlanAll}
			}

			planAll, ok := strict.GetStrictControl(strict.PlanAll)
			require.True(t, ok, "control not found")

			// We intentionally ignore whether the control has already been triggered.
			warning, _, err := planAll.Evaluate(&opts)

			if tt.enableControl || tt.enableStrictMode {
				assert.Empty(t, warning)
				require.Error(t, err)
				require.Equal(t, tt.expectedErr, err)
			} else {
				assert.NotEmpty(t, warning)
				require.NoError(t, err)
			}
		})
	}
}
