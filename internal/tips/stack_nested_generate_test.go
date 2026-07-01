package tips_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/function"
)

func TestSuggestRecursiveStackFilter(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "./my-stack/** | type=stack", tips.SuggestRecursiveStackFilter("./my-stack"))
}

func emptyStackFuncFactory() inthclparse.StackFuncFactory {
	return func(string) (map[string]function.Function, error) {
		return map[string]function.Function{}, nil
	}
}

func writeFile(t *testing.T, fs vfs.FS, path, content string) {
	t.Helper()

	require.NoError(t, fs.MkdirAll(filepath.Dir(path), 0o755)) //nolint:mnd

	f, err := fs.Create(path)
	require.NoError(t, err)
	_, err = f.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

type fileSpec struct {
	path    string
	content string
}

func TestGiveStackNestedGenerateTip(t *testing.T) {
	t.Parallel()

	const (
		workingDir        = "/work"
		stackOfStack      = "stack \"child\" {\n  source = \"x\"\n  path = \"child\"\n}\n"
		stackOfUnit       = "unit \"u\" {\n  source = \"x\"\n  path = \"u\"\n}\n"
		stackOfGrandchild = "stack \"grandchild\" {\n  source = \"x\"\n  path = \"grandchild\"\n}\n"
		stackMalformed    = "stack \"child\" {\n"
	)

	stackDir := filepath.Join(workingDir, "my-stack")
	parentStack := filepath.Join(stackDir, "terragrunt.stack.hcl")
	nestedStackDir := filepath.Join(stackDir, ".terragrunt-stack", "child")
	nestedStack := filepath.Join(nestedStackDir, "terragrunt.stack.hcl")
	nestedUnit := filepath.Join(nestedStackDir, ".terragrunt-stack", "u", "terragrunt.hcl")
	flatUnit := filepath.Join(stackDir, ".terragrunt-stack", "u", "terragrunt.hcl")

	nestedNotGenerated := []fileSpec{{parentStack, stackOfStack}, {nestedStack, stackOfUnit}}

	tcs := []struct {
		name        string
		filter      string
		files       []fileSpec
		disableTip  bool
		expectShown bool
	}{
		{
			name:        "nested stacks not generated shows tip",
			filter:      "./my-stack | type=stack",
			files:       nestedNotGenerated,
			expectShown: true,
		},
		{
			name:        "nested stacks recursively generated shows no tip",
			filter:      "./my-stack | type=stack",
			files:       append([]fileSpec{{nestedUnit, ""}}, nestedNotGenerated...),
			expectShown: false,
		},
		{
			name:        "nested stack with only sub-stacks not generated shows tip",
			filter:      "./my-stack | type=stack",
			files:       []fileSpec{{parentStack, stackOfStack}, {nestedStack, stackOfGrandchild}},
			expectShown: true,
		},
		{
			name:        "flat stack with generated units shows no tip",
			filter:      "./my-stack | type=stack",
			files:       []fileSpec{{parentStack, stackOfUnit}, {flatUnit, ""}},
			expectShown: false,
		},
		{
			name:        "unparseable parent stack shows no tip",
			filter:      "./my-stack | type=stack",
			files:       []fileSpec{{parentStack, stackMalformed}},
			expectShown: false,
		},
		{
			name:        "unparseable nested stack shows no tip",
			filter:      "./my-stack | type=stack",
			files:       []fileSpec{{parentStack, stackOfStack}, {nestedStack, stackMalformed}},
			expectShown: false,
		},
		{
			name:        "glob filter shows no tip",
			filter:      "./my-stack/** | type=stack",
			files:       nestedNotGenerated,
			expectShown: false,
		},
		{
			name:        "filter not restricted to stacks shows no tip",
			filter:      "./my-stack",
			files:       nestedNotGenerated,
			expectShown: false,
		},
		{
			name:        "tip disabled",
			filter:      "./my-stack | type=stack",
			files:       nestedNotGenerated,
			disableTip:  true,
			expectShown: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()
			for _, f := range tc.files {
				writeFile(t, fs, f.path, f.content)
			}

			parsed, err := filter.Parse(tc.filter)
			require.NoError(t, err)

			allTips := tips.NewTips()
			if tc.disableTip {
				require.NoError(t, allTips.DisableTip(tips.StackNestedStacksNotGenerated))
			}

			l, output := newTestLogger()

			tips.GiveStackNestedGenerateTip(l, fs, emptyStackFuncFactory(), workingDir, filter.Filters{parsed}, allTips)

			if tc.expectShown {
				assert.Contains(t, output.String(), tips.StackNestedStacksNotGenerated)
				assert.Contains(t, output.String(), "./my-stack | type=stack")
				assert.Contains(t, output.String(), "./my-stack/** | type=stack")

				return
			}

			assert.NotContains(t, output.String(), tips.StackNestedStacksNotGenerated)
		})
	}
}

func TestGiveStackNestedGenerateTipNoOp(t *testing.T) {
	t.Parallel()

	const workingDir = "/work"

	parsed, err := filter.Parse("./my-stack | type=stack")
	require.NoError(t, err)

	filters := filter.Filters{parsed}

	tcs := []struct {
		name     string
		funcsFor inthclparse.StackFuncFactory
		allTips  tips.Tips
		filters  filter.Filters
	}{
		{
			name:     "no filters",
			funcsFor: emptyStackFuncFactory(),
			allTips:  tips.NewTips(),
			filters:  filter.Filters{},
		},
		{
			name:     "nil tips collection",
			funcsFor: emptyStackFuncFactory(),
			allTips:  nil,
			filters:  filters,
		},
		{
			name:     "nil func factory",
			funcsFor: nil,
			allTips:  tips.NewTips(),
			filters:  filters,
		},
		{
			name:     "tip absent from collection",
			funcsFor: emptyStackFuncFactory(),
			allTips:  tips.Tips{},
			filters:  filters,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l, output := newTestLogger()

			tips.GiveStackNestedGenerateTip(l, vfs.NewMemMapFS(), tc.funcsFor, workingDir, tc.filters, tc.allTips)

			assert.Empty(t, output.String())
		})
	}
}
