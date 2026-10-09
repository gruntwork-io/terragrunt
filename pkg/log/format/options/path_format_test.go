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

func TestPathFormatOptionFormat(t *testing.T) {
	t.Parallel()

	base := venvtest.Root("/work/app")
	unit := filepath.Join(base, "unit")
	sibling := filepath.Join(filepath.Dir(base), "db")
	file := filepath.Join(unit, "main.tf")

	pather, err := options.NewRelativePather(base)
	require.NoError(t, err)

	withPather := &options.Data{RelativePather: pather, BaseDir: base}
	withoutPather := &options.Data{BaseDir: base}

	tests := []struct {
		data     *options.Data
		name     string
		value    string
		expected string
		format   options.PathFormatValue
	}{
		{
			name:     "relative",
			format:   options.RelativePath,
			data:     withPather,
			value:    "in " + unit,
			expected: "in ." + string(filepath.Separator) + "unit",
		},
		{
			name:     "relative_without_pather_is_passthrough",
			format:   options.RelativePath,
			data:     withoutPather,
			value:    unit,
			expected: unit,
		},
		{
			name:     "short_relative_base_dir_is_empty",
			format:   options.ShortRelativePath,
			data:     withPather,
			value:    base,
			expected: "",
		},
		{
			name:     "short_relative_drops_current_dir_prefix",
			format:   options.ShortRelativePath,
			data:     withPather,
			value:    unit,
			expected: "unit",
		},
		{
			name:     "short_relative_keeps_parent_prefix",
			format:   options.ShortRelativePath,
			data:     withPather,
			value:    sibling,
			expected: filepath.Join("..", "db"),
		},
		{
			name:     "short_relative_without_pather_is_passthrough",
			format:   options.ShortRelativePath,
			data:     withoutPather,
			value:    unit,
			expected: unit,
		},
		{
			name:     "short_base_dir_is_empty",
			format:   options.ShortPath,
			data:     withoutPather,
			value:    base,
			expected: "",
		},
		{
			name:     "short_other_path_is_passthrough",
			format:   options.ShortPath,
			data:     withoutPather,
			value:    unit,
			expected: unit,
		},
		{
			name:     "filename",
			format:   options.FilenamePath,
			data:     withoutPather,
			value:    file,
			expected: "main.tf",
		},
		{
			name:     "directory",
			format:   options.DirectoryPath,
			data:     withoutPather,
			value:    file,
			expected: unit,
		},
		{
			name:     "none_is_passthrough",
			format:   options.NonePath,
			data:     withoutPather,
			value:    file,
			expected: file,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.PathFormat(tc.format)
			assert.Equal(t, options.PathFormatOptionName, opt.Name())

			out, err := opt.Format(tc.data, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}

// TestPathFormatAllowedValues checks a placeholder rejects path formats outside its allowed list.
func TestPathFormatAllowedValues(t *testing.T) {
	t.Parallel()

	opt := options.PathFormat(options.NonePath, options.RelativePath, options.FilenamePath)

	require.NoError(t, opt.ParseValue("filename"))

	out, err := opt.Format(&options.Data{}, filepath.Join("a", "b.tf"))
	require.NoError(t, err)
	assert.Equal(t, "b.tf", out)

	require.EqualError(t, opt.ParseValue("dir"), "available values: filename,relative")
}
