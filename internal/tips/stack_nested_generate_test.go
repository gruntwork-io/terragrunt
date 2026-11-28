package tips_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// errStatFault stands in for a Stat failure that is not "file does not exist".
var errStatFault = errors.New("stat fault")

// missingAsErrorFS wraps a vfs.FS and reports a missing path as a hard Stat
// error instead of a clean "not found". It lets tests drive the FileExists
// error branches that an ordinary in-memory FS never reaches.
type missingAsErrorFS struct {
	vfs.FS
	err error
}

func (f *missingAsErrorFS) Stat(name string) (fs.FileInfo, error) {
	info, err := f.FS.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, f.err
	}

	return info, err
}

func TestSuggestRecursiveStackFilter(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "./my-stack/** | type=stack", tips.SuggestRecursiveStackFilter("./my-stack"))
}

func emptyStackFuncFactory() inthclparse.StackFuncFactory {
	return func(string) (map[string]function.Function, error) {
		return map[string]function.Function{}, nil
	}
}

func emptyStackVarFactory() inthclparse.StackVarFactory {
	return func(string) (map[string]cty.Value, error) {
		return nil, nil
	}
}

func writeFile(t *testing.T, fsys vfs.FS, path, content string) {
	t.Helper()

	require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0o755))

	f, err := fsys.Create(path)
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

			tips.GiveStackNestedGenerateTip(
				t.Context(),
				l,
				fs,
				&inthclparse.StackDirArgs{FuncsFor: emptyStackFuncFactory(), VarsFor: emptyStackVarFactory()},
				workingDir,
				filter.Filters{parsed},
				allTips,
			)

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
		name    string
		allTips tips.Tips
		filters filter.Filters
	}{
		{
			name:    "no filters",
			allTips: tips.NewTips(),
			filters: filter.Filters{},
		},
		{
			name:    "tip absent from collection",
			allTips: tips.Tips{},
			filters: filters,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l, output := newTestLogger()

			tips.GiveStackNestedGenerateTip(
				t.Context(),
				l,
				vfs.NewMemMapFS(),
				&inthclparse.StackDirArgs{FuncsFor: emptyStackFuncFactory(), VarsFor: emptyStackVarFactory()},
				workingDir,
				tc.filters,
				tc.allTips,
			)

			assert.Empty(t, output.String())
		})
	}
}

func TestGiveStackNestedGenerateTipPanicsOnNilArgument(t *testing.T) {
	t.Parallel()

	parsed, err := filter.Parse("./my-stack | type=stack")
	require.NoError(t, err)

	tcs := []struct {
		args    *inthclparse.StackDirArgs
		name    string
		want    string
		allTips tips.Tips
	}{
		{
			name:    "nil tips collection",
			args:    &inthclparse.StackDirArgs{FuncsFor: emptyStackFuncFactory(), VarsFor: emptyStackVarFactory()},
			allTips: nil,
			want:    "tips.GiveStackNestedGenerateTip: allTips is nil",
		},
		{
			name:    "nil args",
			args:    nil,
			allTips: tips.NewTips(),
			want:    "tips.GiveStackNestedGenerateTip: args is nil",
		},
		{
			name:    "nil func factory",
			args:    &inthclparse.StackDirArgs{VarsFor: emptyStackVarFactory()},
			allTips: tips.NewTips(),
			want:    "tips.GiveStackNestedGenerateTip: args.FuncsFor is nil",
		},
		{
			name:    "nil var factory",
			args:    &inthclparse.StackDirArgs{FuncsFor: emptyStackFuncFactory()},
			allTips: tips.NewTips(),
			want:    "tips.GiveStackNestedGenerateTip: args.VarsFor is nil",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l, _ := newTestLogger()

			require.PanicsWithValue(t, tc.want, func() {
				tips.GiveStackNestedGenerateTip(
					t.Context(),
					l,
					vfs.NewMemMapFS(),
					tc.args,
					"/work",
					filter.Filters{parsed},
					tc.allTips,
				)
			})
		})
	}
}

func TestGiveStackNestedGenerateTipSkipsOnStatError(t *testing.T) {
	t.Parallel()

	const (
		workingDir   = "/work"
		stackOfStack = "stack \"child\" {\n  source = \"x\"\n  path = \"child\"\n}\n"
		stackOfUnit  = "unit \"u\" {\n  source = \"x\"\n  path = \"u\"\n}\n"
	)

	stackDir := filepath.Join(workingDir, "my-stack")

	base := vfs.NewMemMapFS()
	writeFile(t, base, filepath.Join(stackDir, "terragrunt.stack.hcl"), stackOfStack)
	writeFile(
		t,
		base,
		filepath.Join(stackDir, ".terragrunt-stack", "child", "terragrunt.stack.hcl"),
		stackOfUnit,
	)

	// The nested unit's generated path does not exist; the wrapper turns that
	// missing-path Stat into an error, exercising anyPathMissing's error branch.
	fs := &missingAsErrorFS{FS: base, err: errStatFault}

	parsed, err := filter.Parse("./my-stack | type=stack")
	require.NoError(t, err)

	l, output := newTestLogger()

	tips.GiveStackNestedGenerateTip(
		t.Context(),
		l,
		fs,
		&inthclparse.StackDirArgs{FuncsFor: emptyStackFuncFactory(), VarsFor: emptyStackVarFactory()},
		workingDir,
		filter.Filters{parsed},
		tips.NewTips(),
	)

	assert.NotContains(t, output.String(), tips.StackNestedStacksNotGenerated)
}

func TestGiveStackNestedGenerateTipEvaluatesWithFactoryVariables(t *testing.T) {
	t.Parallel()

	const (
		workingDir   = "/work"
		stackOfStack = "locals {\n  dir = path.stack\n}\n\nstack \"child\" {\n  source = \"x\"\n  path = \"child\"\n}\n"
		stackOfUnit  = "unit \"u\" {\n  source = \"x\"\n  path = \"u\"\n}\n"
	)

	stackDir := filepath.Join(workingDir, "my-stack")

	fsys := vfs.NewMemMapFS()
	writeFile(t, fsys, filepath.Join(stackDir, "terragrunt.stack.hcl"), stackOfStack)
	writeFile(
		t,
		fsys,
		filepath.Join(stackDir, ".terragrunt-stack", "child", "terragrunt.stack.hcl"),
		stackOfUnit,
	)

	parsed, err := filter.Parse("./my-stack | type=stack")
	require.NoError(t, err)

	l, output := newTestLogger()

	tips.GiveStackNestedGenerateTip(
		t.Context(),
		l,
		fsys,
		&inthclparse.StackDirArgs{
			FuncsFor: emptyStackFuncFactory(),
			VarsFor: func(dir string) (map[string]cty.Value, error) {
				return map[string]cty.Value{
					"path": cty.ObjectVal(map[string]cty.Value{"stack": cty.StringVal(dir)}),
				}, nil
			},
		},
		workingDir,
		filter.Filters{parsed},
		tips.NewTips(),
	)

	assert.Contains(t, output.String(), tips.StackNestedStacksNotGenerated)
}

func TestGiveStackNestedGenerateTipReadsCommandsFromFactoryVariables(t *testing.T) {
	t.Parallel()

	const (
		workingDir   = "/work"
		stackOfStack = "locals {\n  name = commands.accept_vars[0]\n}\n\n" +
			"stack \"child\" {\n  source = \"x\"\n  path = local.name\n}\n"
		stackOfUnit = "unit \"u\" {\n  source = \"x\"\n  path = \"u\"\n}\n"
	)

	stackDir := filepath.Join(workingDir, "my-stack")

	fsys := vfs.NewMemMapFS()
	writeFile(t, fsys, filepath.Join(stackDir, "terragrunt.stack.hcl"), stackOfStack)
	writeFile(
		t,
		fsys,
		filepath.Join(stackDir, ".terragrunt-stack", "plan", "terragrunt.stack.hcl"),
		stackOfUnit,
	)

	parsed, err := filter.Parse("./my-stack | type=stack")
	require.NoError(t, err)

	l, _ := newTestLogger()
	allTips := tips.NewTips()

	tips.GiveStackNestedGenerateTip(
		t.Context(),
		l,
		fsys,
		&inthclparse.StackDirArgs{
			FuncsFor: emptyStackFuncFactory(),
			VarsFor: func(string) (map[string]cty.Value, error) {
				return map[string]cty.Value{
					"commands": cty.ObjectVal(map[string]cty.Value{
						"accept_vars": cty.ListVal([]cty.Value{cty.StringVal("plan")}),
					}),
				}, nil
			},
		},
		workingDir,
		filter.Filters{parsed},
		allTips,
	)

	tip := allTips.Find(tips.StackNestedStacksNotGenerated)
	require.NotNil(t, tip)

	shown := true

	tip.OnceShow.Do(func() { shown = false })

	require.True(t, shown)
}
