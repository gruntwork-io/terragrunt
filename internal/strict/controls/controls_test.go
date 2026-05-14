package controls_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/stretchr/testify/assert"
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
