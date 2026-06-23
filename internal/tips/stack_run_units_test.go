package tips_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuggestStackUnitsFilter(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		relDir string
		want   string
	}{
		{relDir: "stacks/first", want: "./stacks/first/** | type=unit"},
		{relDir: ".", want: "./** | type=unit"},
		{relDir: "", want: "./** | type=unit"},
	}

	for _, tc := range tcs {
		t.Run(tc.relDir, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tips.SuggestStackUnitsFilter(tc.relDir))
		})
	}
}

func TestGiveStackRunUnitsTip(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name        string
		stackDirs   []string
		disableTip  bool
		expectShown bool
	}{
		{
			name:        "matched stacks shows the tip",
			stackDirs:   []string{"stacks/first"},
			expectShown: true,
		},
		{
			name:        "no matched stacks shows nothing",
			stackDirs:   nil,
			expectShown: false,
		},
		{
			name:        "tip disabled",
			stackDirs:   []string{"stacks/first"},
			disableTip:  true,
			expectShown: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			allTips := tips.NewTips()
			if tc.disableTip {
				require.NoError(t, allTips.DisableTip(tips.StackRunFilterMatchedStacks))
			}

			l, output := newTestLogger()

			tips.GiveStackRunUnitsTip(l, allTips, tc.stackDirs)

			if tc.expectShown {
				assert.Contains(t, output.String(), tips.StackRunFilterMatchedStacks)
				assert.Contains(t, output.String(), "./stacks/first/** | type=unit")
			} else {
				assert.NotContains(t, output.String(), tips.StackRunFilterMatchedStacks)
			}
		})
	}
}
