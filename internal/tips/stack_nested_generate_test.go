package tips_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuggestRecursiveStackFilter(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "./my-stack/** | type=stack", tips.SuggestRecursiveStackFilter("./my-stack"))
}

func writeEmptyFile(t *testing.T, fs vfs.FS, path string) {
	t.Helper()

	require.NoError(t, fs.MkdirAll(filepath.Dir(path), 0o755)) //nolint:mnd

	f, err := fs.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestGiveStackNestedGenerateTip(t *testing.T) {
	t.Parallel()

	const workingDir = "/work"

	gen := filepath.Join(workingDir, "my-stack", config.StackDir)
	nestedStack := filepath.Join(gen, "child", config.DefaultStackFile)
	nestedExpansion := filepath.Join(gen, "child", config.StackDir, "unit", "terragrunt.hcl")
	flatUnit := filepath.Join(gen, "unit", "terragrunt.hcl")

	tcs := []struct {
		name        string
		filter      string
		files       []string
		disableTip  bool
		expectShown bool
	}{
		{
			name:        "literal filter with ungenerated nested stack shows tip",
			filter:      "./my-stack | type=stack",
			files:       []string{nestedStack},
			expectShown: true,
		},
		{
			name:        "nested stack already generated shows no tip",
			filter:      "./my-stack | type=stack",
			files:       []string{nestedStack, nestedExpansion},
			expectShown: false,
		},
		{
			name:        "flat stack with no nested stacks shows no tip",
			filter:      "./my-stack | type=stack",
			files:       []string{flatUnit},
			expectShown: false,
		},
		{
			name:        "glob filter shows no tip",
			filter:      "./my-stack/** | type=stack",
			files:       []string{nestedStack},
			expectShown: false,
		},
		{
			name:        "filter not restricted to stacks shows no tip",
			filter:      "./my-stack",
			files:       []string{nestedStack},
			expectShown: false,
		},
		{
			name:        "tip disabled",
			filter:      "./my-stack | type=stack",
			files:       []string{nestedStack},
			disableTip:  true,
			expectShown: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()
			for _, f := range tc.files {
				writeEmptyFile(t, fs, f)
			}

			parsed, err := filter.Parse(tc.filter)
			require.NoError(t, err)

			allTips := tips.NewTips()
			if tc.disableTip {
				require.NoError(t, allTips.DisableTip(tips.StackNestedStacksNotGenerated))
			}

			l, output := newTestLogger()

			tips.GiveStackNestedGenerateTip(l, fs, workingDir, filter.Filters{parsed}, allTips)

			if tc.expectShown {
				assert.Contains(t, output.String(), tips.StackNestedStacksNotGenerated)
				assert.Contains(t, output.String(), "./my-stack/** | type=stack")
			} else {
				assert.NotContains(t, output.String(), tips.StackNestedStacksNotGenerated)
			}
		})
	}
}
