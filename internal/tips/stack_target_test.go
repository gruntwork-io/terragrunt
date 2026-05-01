package tips_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiveStackTargetTip(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name        string
		filters     []string
		stackDirs   []string
		unitDirs    []string
		disableTip  bool
		expectShown bool
	}{
		{
			name:        "literal path matches stack",
			filters:     []string{"./envs/prod"},
			stackDirs:   []string{"envs/prod"},
			expectShown: true,
		},
		{
			name:        "literal path matches unit not stack",
			filters:     []string{"./units/vpc"},
			unitDirs:    []string{"units/vpc"},
			expectShown: false,
		},
		{
			name:        "filter already restricted to stacks",
			filters:     []string{"./envs/prod | type=stack"},
			stackDirs:   []string{"envs/prod"},
			expectShown: false,
		},
		{
			name:        "negated path matches stack",
			filters:     []string{"!./envs/prod"},
			stackDirs:   []string{"envs/prod"},
			expectShown: true,
		},
		{
			name:        "glob path is skipped",
			filters:     []string{"./envs/**"},
			stackDirs:   []string{"envs/prod"},
			expectShown: false,
		},
		{
			name:        "no filters",
			filters:     nil,
			stackDirs:   []string{"envs/prod"},
			expectShown: false,
		},
		{
			name:        "tip disabled",
			filters:     []string{"./envs/prod"},
			stackDirs:   []string{"envs/prod"},
			disableTip:  true,
			expectShown: false,
		},
		{
			name:        "second filter is offender",
			filters:     []string{"name=foo", "./envs/prod"},
			stackDirs:   []string{"envs/prod"},
			expectShown: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workingDir := "/work"
			fs := vfs.NewMemMapFS()

			for _, dir := range tc.stackDirs {
				abs := filepath.Join(workingDir, dir)
				require.NoError(t, fs.MkdirAll(abs, 0o755)) //nolint:mnd
				f, err := fs.Create(filepath.Join(abs, config.DefaultStackFile))
				require.NoError(t, err)
				require.NoError(t, f.Close())
			}

			for _, dir := range tc.unitDirs {
				abs := filepath.Join(workingDir, dir)
				require.NoError(t, fs.MkdirAll(abs, 0o755)) //nolint:mnd
				f, err := fs.Create(filepath.Join(abs, "terragrunt.hcl"))
				require.NoError(t, err)
				require.NoError(t, f.Close())
			}

			parsed := make(filter.Filters, 0, len(tc.filters))

			for _, q := range tc.filters {
				p, err := filter.Parse(q)
				require.NoError(t, err)

				parsed = append(parsed, p)
			}

			allTips := tips.NewTips()
			if tc.disableTip {
				require.NoError(t, allTips.DisableTip(tips.StackTargetMissingTypeStack))
			}

			l, output := newTestLogger()

			tips.GiveStackTargetTip(l, fs, workingDir, parsed, allTips)

			if tc.expectShown {
				assert.Contains(t, output.String(), tips.StackTargetMissingTypeStack)
				assert.Contains(t, output.String(), "type=stack")
			} else {
				assert.NotContains(t, output.String(), tips.StackTargetMissingTypeStack)
			}
		})
	}
}

func TestGiveStackTargetTipFiresOnceAcrossCalls(t *testing.T) {
	t.Parallel()

	workingDir := "/work"
	fs := vfs.NewMemMapFS()

	abs := filepath.Join(workingDir, "envs/prod")
	require.NoError(t, fs.MkdirAll(abs, 0o755)) //nolint:mnd
	f, err := fs.Create(filepath.Join(abs, config.DefaultStackFile))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	parsed, err := filter.Parse("./envs/prod")
	require.NoError(t, err)

	allTips := tips.NewTips()

	l, output := newTestLogger()

	tips.GiveStackTargetTip(l, fs, workingDir, filter.Filters{parsed}, allTips)
	tips.GiveStackTargetTip(l, fs, workingDir, filter.Filters{parsed}, allTips)

	assert.Equal(t, 1, strings.Count(output.String(), tips.StackTargetMissingTypeStack))
}
