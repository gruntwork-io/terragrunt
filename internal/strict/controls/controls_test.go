package controls_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLegacyGCSPublicPrefixControlIsRegistered pins the Warning field on the
// legacy-gcs-public-prefix control. Emission via the framework's sync.Once
// dedup requires the field to be populated.
func TestLegacyGCSPublicPrefixControlIsRegistered(t *testing.T) {
	t.Parallel()

	ctrl := controls.New().Find(controls.LegacyGCSPublicPrefix)

	if assert.NotNil(t, ctrl, "legacy-gcs-public-prefix must be registered") {
		assert.Equal(t, controls.LegacyGCSDeprecationWarning, ctrl.(*controls.Control).Warning)
	}
}

// TestIsFastCopyEnabled pins the predicate against an absent control, a
// present-but-disabled control, and an enabled control.
func TestIsFastCopyEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ctrl *controls.Control
		name string
		want bool
	}{
		{
			name: "control absent",
			ctrl: nil,
			want: false,
		},
		{
			name: "control present but disabled",
			ctrl: &controls.Control{Name: controls.FastCopy, Enabled: false},
			want: false,
		},
		{
			name: "control enabled",
			ctrl: &controls.Control{Name: controls.FastCopy, Enabled: true},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			set := strict.Controls{}
			if tc.ctrl != nil {
				set = append(set, tc.ctrl)
			}

			assert.Equal(t, tc.want, controls.IsFastCopyEnabled(set))
		})
	}
}

func TestControlString(t *testing.T) {
	t.Parallel()

	ctrl := &controls.Control{Name: "legacy-logs"}
	assert.Equal(t, "legacy-logs", ctrl.String())
}

func TestControlGetDescription(t *testing.T) {
	t.Parallel()

	ctrl := &controls.Control{
		Name:        "deprecated-flags",
		Description: "prevents deprecated flags",
	}

	assert.Equal(t, "prevents deprecated flags", ctrl.GetDescription())
}

func TestControlGetStatus(t *testing.T) {
	t.Parallel()

	ctrl := &controls.Control{Name: "x", Status: strict.CompletedStatus}
	assert.Equal(t, strict.CompletedStatus, ctrl.GetStatus())
}

func TestControlEnable(t *testing.T) {
	t.Parallel()

	ctrl := &controls.Control{Name: "deprecated-flags"}
	ctrl.Enable()
	assert.True(t, ctrl.GetEnabled())
}

func TestControlGetSubcontrols(t *testing.T) {
	t.Parallel()

	sub := &controls.Control{Name: "sub"}
	ctrl := &controls.Control{Name: "parent", Subcontrols: strict.Controls{sub}}

	assert.Equal(t, strict.Controls{sub}, ctrl.GetSubcontrols())
}

func TestControlAddSubcontrols(t *testing.T) {
	t.Parallel()

	t.Run("allocates when Subcontrols is nil", func(t *testing.T) {
		t.Parallel()

		ctrl := &controls.Control{Name: "parent"}
		sub := &controls.Control{Name: "sub"}

		ctrl.AddSubcontrols(sub)
		assert.Equal(t, strict.Controls{sub}, ctrl.GetSubcontrols())
	})

	t.Run("appends to existing Subcontrols", func(t *testing.T) {
		t.Parallel()

		existing := &controls.Control{Name: "existing"}
		ctrl := &controls.Control{
			Name:        "parent",
			Subcontrols: strict.Controls{existing},
		}
		extra := &controls.Control{Name: "extra"}

		ctrl.AddSubcontrols(extra)
		assert.Equal(t, strict.Controls{existing, extra}, ctrl.GetSubcontrols())
	})
}

func TestControlSuppressWarning(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := log.ContextWithLogger(t.Context(), l)

	ctrl := &controls.Control{Name: "deprecated", Warning: "warn"}
	ctrl.SuppressWarning()

	require.NoError(t, ctrl.Evaluate(ctx))
	require.NoError(t, ctrl.Evaluate(ctx))
}

func TestControlEvaluate(t *testing.T) {
	t.Parallel()

	bootErr := errors.New("boom")

	t.Run("context error short-circuits", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		ctrl := &controls.Control{Name: "ctrl"}
		err := ctrl.Evaluate(ctx)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("nil receiver returns nil", func(t *testing.T) {
		t.Parallel()

		var ctrl *controls.Control
		require.NoError(t, ctrl.Evaluate(t.Context()))
	})

	t.Run("enabled active with error returns error", func(t *testing.T) {
		t.Parallel()

		ctrl := &controls.Control{
			Name:    "active",
			Status:  strict.ActiveStatus,
			Error:   bootErr,
			Enabled: true,
		}

		err := ctrl.Evaluate(t.Context())
		require.ErrorIs(t, err, bootErr)
	})

	t.Run("enabled completed returns nil", func(t *testing.T) {
		t.Parallel()

		ctrl := &controls.Control{
			Name:    "completed",
			Status:  strict.CompletedStatus,
			Error:   bootErr,
			Enabled: true,
		}

		require.NoError(t, ctrl.Evaluate(t.Context()))
	})

	t.Run("enabled active with nil error returns nil", func(t *testing.T) {
		t.Parallel()

		ctrl := &controls.Control{
			Name:    "active-no-error",
			Status:  strict.ActiveStatus,
			Enabled: true,
		}

		require.NoError(t, ctrl.Evaluate(t.Context()))
	})

	t.Run("disabled with logger and warning emits and returns nil", func(t *testing.T) {
		t.Parallel()

		l := logger.CreateLogger()
		ctx := log.ContextWithLogger(t.Context(), l)

		ctrl := &controls.Control{Name: "warn", Warning: "warn message"}
		require.NoError(t, ctrl.Evaluate(ctx))
	})

	t.Run("disabled with no logger returns nil", func(t *testing.T) {
		t.Parallel()

		ctrl := &controls.Control{Name: "warn", Warning: "warn message"}
		require.NoError(t, ctrl.Evaluate(t.Context()))
	})

	t.Run("disabled with subcontrol error propagates", func(t *testing.T) {
		t.Parallel()

		sub := &controls.Control{
			Name:    "sub",
			Status:  strict.ActiveStatus,
			Error:   bootErr,
			Enabled: true,
		}
		parent := &controls.Control{
			Name:        "parent",
			Subcontrols: strict.Controls{sub},
		}

		err := parent.Evaluate(t.Context())
		require.ErrorIs(t, err, bootErr)
	})
}
