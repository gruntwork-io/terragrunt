package tips_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiveMissingDependencyConfigTip(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name       string
		disableTip bool
	}{
		{name: "tip enabled"},
		{name: "tip disabled", disableTip: true},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			allTips := tips.NewTips()
			if tc.disableTip {
				require.NoError(t, allTips.DisableTip(tips.MissingDependencyConfig))
			}

			l, output := newTestLogger()

			tips.GiveMissingDependencyConfigTip(l, allTips, "live/dep", "HEAD~1")

			if tc.disableTip {
				assert.NotContains(t, output.String(), tips.MissingDependencyConfig)

				return
			}

			assert.Contains(t, output.String(), "TIP ("+tips.MissingDependencyConfig+")")
			assert.Contains(t, output.String(), "Deleted unit: live/dep (exists at HEAD~1).")
		})
	}
}

func TestGiveMissingDependencyConfigTipNilTips(t *testing.T) {
	t.Parallel()

	l, output := newTestLogger()

	tips.GiveMissingDependencyConfigTip(l, nil, "live/dep", "HEAD~1")

	assert.Empty(t, output.String())
}
