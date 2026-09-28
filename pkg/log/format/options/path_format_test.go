package options_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelativePatherReplaceAbsPaths(t *testing.T) {
	t.Parallel()

	base := venvtest.Root("/work/app")
	parent := filepath.Dir(base)
	child := filepath.Join(base, "unit")
	sibling := filepath.Join(parent, "db")

	testCases := []struct {
		name string
		in   string
		want string
	}{
		{name: "BaseDir", in: base, want: "."},
		{name: "Child", in: child, want: "." + string(filepath.Separator) + "unit"},
		{name: "Sibling", in: sibling, want: filepath.Join("..", "db")},
		{name: "Quoted", in: `"` + base + `"`, want: `"."`},
		{name: "InSentence", in: "running in " + base + " now", want: "running in . now"},
		{name: "AdjacentPaths", in: base + " " + base, want: ". ."},
		{name: "LongerDirName", in: base + "x", want: filepath.Join("..", "appx")},
		{name: "PrecededByWordChar", in: "x" + base, want: "x" + base},
		{name: "NoPath", in: "nothing to replace", want: "nothing to replace"},
	}

	pather, err := options.NewRelativePather(base)
	require.NoError(t, err)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, pather.ReplaceAbsPaths(tc.in))
		})
	}
}

// TestRelativePatherInvalidUTF8BaseDir pins that a base directory whose name
// is not valid UTF-8, which a working directory on Linux can be, still has its
// paths replaced.
func TestRelativePatherInvalidUTF8BaseDir(t *testing.T) {
	t.Parallel()

	base := venvtest.Root("/work/\xb2")

	pather, err := options.NewRelativePather(base)
	require.NoError(t, err)

	assert.Equal(t, "in .", pather.ReplaceAbsPaths("in "+base))
}
