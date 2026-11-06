package glob_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/glob"
)

func TestOpenTofuMatch(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{name: "double star crosses separators", pattern: "**/*.json", path: "a/b/c.json", want: true},
		{name: "star stays in a segment", pattern: "*.json", path: "a/c.json"},
		{name: "alternatives", pattern: "{a,b}/*.json", path: "b/c.json", want: true},
		{name: "backslash escapes", pattern: `a\*/*.json`, path: "a*/c.json", want: true},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.True(t, glob.ValidOpenTofuPattern(tc.pattern))

			got, err := glob.OpenTofuMatch(tc.pattern, tc.path)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestOpenTofuMatchRejectsAMalformedPattern(t *testing.T) {
	t.Parallel()

	assert.False(t, glob.ValidOpenTofuPattern("a/["))

	_, err := glob.OpenTofuMatch("a/[", "a/b")
	require.ErrorIs(t, err, glob.ErrBadPattern)
}
