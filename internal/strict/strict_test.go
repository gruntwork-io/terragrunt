package strict_test

// Add some basic tests that confirm that by default, a warning is emitted when strict mode is disabled,
// and an error is emitted when a specific control is enabled.
// Make sure to test both when the specific control is enabled, and when the global strict mode is enabled.

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/stretchr/testify/require"
)

func TestStrictControl(t *testing.T) {
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
			expectedErr:      strict.ErrStrictPlanAll,
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
			expectedErr:      strict.ErrStrictPlanAll,
		},
		{
			name:             "control disabled, strict mode enabled",
			enableControl:    false,
			enableStrictMode: true,
			expectedErr:      strict.ErrStrictPlanAll,
		},
	}

	for _, tt := range tests { //nolint:varnamelen
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TG_STRICT_MODE", "false")
			t.Setenv("TG_STRICT_PLAN_ALL", "false")

			if tt.enableControl {
				t.Setenv("TG_STRICT_PLAN_ALL", "true")
			}

			if tt.enableStrictMode {
				t.Setenv("TG_STRICT_MODE", "true")
			}

			planAll, ok := strict.GetStrictControl(strict.PlanAll)
			require.True(t, ok, "control not found")

			warning, err := planAll.Evaluate()
			require.Equal(t, tt.expectedErr, err)

			if tt.enableControl || tt.enableStrictMode {
				require.Empty(t, warning)
			} else {
				require.NotEmpty(t, warning)
			}
		})
	}
}
