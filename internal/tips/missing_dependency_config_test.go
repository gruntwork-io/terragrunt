package tips_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiveMissingDependencyConfigTip(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		err        error
		name       string
		disableTip bool
		wantShown  bool
	}{
		{
			name:      "dependency config not found during discovery",
			err:       errors.Join(config.TerragruntConfigNotFoundError{Path: "/repo/dep/terragrunt.hcl"}),
			wantShown: true,
		},
		{
			name: "dependency outputs of a missing unit",
			err: fmt.Errorf(
				"resolving dependency %q outputs: %w",
				"dep",
				config.DependencyConfigNotFound{Path: "/repo/dep"},
			),
			wantShown: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("boom"),
		},
		{
			name: "no error",
		},
		{
			name:       "tip disabled",
			err:        config.DependencyConfigNotFound{Path: "/repo/dep"},
			disableTip: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			allTips := tips.NewTips()
			if tc.disableTip {
				require.NoError(t, allTips.DisableTip(tips.MissingDependencyConfig))
			}

			l, output := newTestLogger()

			tips.GiveMissingDependencyConfigTip(l, tc.err, allTips)

			if tc.wantShown {
				assert.Contains(t, output.String(), tips.MissingDependencyConfig)

				return
			}

			assert.NotContains(t, output.String(), tips.MissingDependencyConfig)
		})
	}
}
