package tips_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiveMissingDependencyConfigTip(t *testing.T) {
	t.Parallel()

	missing := config.MissingDependencyConfigError{UnitPath: "/repo/app", DependencyPath: "/repo/cache"}

	tcs := []struct {
		err         error
		name        string
		disableTip  bool
		expectShown bool
	}{
		{
			name:        "discovery error",
			err:         missing,
			expectShown: true,
		},
		{
			name:        "wrapped discovery error",
			err:         fmt.Errorf("discovering units: %w", missing),
			expectShown: true,
		},
		{
			name:        "joined discovery error",
			err:         errors.Join(errors.New("other failure"), missing),
			expectShown: true,
		},
		{
			name: "dependency output error",
			err: fmt.Errorf(
				"resolving dependency %q outputs: %w",
				"cache",
				config.DependencyConfigNotFound{Path: "/repo/cache"},
			),
			expectShown: true,
		},
		{
			name:        "config not found outside a dependency",
			err:         config.TerragruntConfigNotFoundError{Path: "/repo/app/terragrunt.hcl"},
			expectShown: false,
		},
		{
			name:        "unrelated error",
			err:         errors.New("boom"),
			expectShown: false,
		},
		{
			name:        "no error",
			expectShown: false,
		},
		{
			name:        "tip disabled",
			err:         missing,
			disableTip:  true,
			expectShown: false,
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

			if tc.expectShown {
				assert.Contains(t, output.String(), tips.MissingDependencyConfig)
				assert.Contains(t, output.String(), "hcl validate --check-dependencies")

				return
			}

			assert.NotContains(t, output.String(), tips.MissingDependencyConfig)
		})
	}
}

func TestGiveMissingDependencyConfigTipShownOnce(t *testing.T) {
	t.Parallel()

	allTips := tips.NewTips()
	l, output := newTestLogger()
	err := config.MissingDependencyConfigError{UnitPath: "/repo/app", DependencyPath: "/repo/cache"}

	tips.GiveMissingDependencyConfigTip(l, err, allTips)
	tips.GiveMissingDependencyConfigTip(l, err, allTips)

	assert.Equal(t, 1, strings.Count(output.String(), tips.MissingDependencyConfig))
}

func TestGiveMissingDependencyConfigTipWithoutTips(t *testing.T) {
	t.Parallel()

	l, output := newTestLogger()
	err := config.MissingDependencyConfigError{UnitPath: "/repo/app", DependencyPath: "/repo/cache"}

	require.NotPanics(t, func() {
		tips.GiveMissingDependencyConfigTip(l, err, nil)
	})

	assert.Empty(t, output.String())
}
