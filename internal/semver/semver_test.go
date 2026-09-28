package semver_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/semver"
)

// TestParseAcceptsToolWrittenVersions pins the lenient reading callers rely
// on for strings a tool wrote, where a two-part version or an unseparated
// pre-release is common.
func TestParseAcceptsToolWrittenVersions(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"1.9", "v1.9.0", "1.9.0beta", "1.9.0-rc.1", "1", "1.2.3.4"} {
		v, err := semver.Parse(s)
		require.NoError(t, err, s)
		assert.NotNil(t, v)
	}

	for _, s := range []string{"latest", ">= 1.2", "", "release-1.2.3"} {
		_, err := semver.Parse(s)
		require.Error(t, err, s)
	}
}

// TestIsExact pins which strings name one specific release. A partial
// version is a moving target, so treating it as fixed would serve a stale
// answer once a maintainer moves it.
func TestIsExact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want bool
	}{
		{in: "v1.2.3", want: true},
		{in: "1.2.3", want: true},
		{in: "v1.2.3-rc.1", want: true},
		{in: "v1.2.3+build.7", want: true},
		{in: "v1.2.3-rc.1+build.7", want: true},
		{in: "v1.2", want: false},
		{in: "v1", want: false},
		{in: "1.2.3.4", want: false},
		{in: "01.2.3", want: false},
		{in: "latest", want: false},
		{in: "release-1.2.3", want: false},
		{in: ">= 1.2.3", want: false},
		{in: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, semver.IsExact(tt.in))
		})
	}
}

// TestParseIsLenientWhereIsExactIsNot pins the split between the two
// readings, which is the reason both are exposed.
func TestParseIsLenientWhereIsExactIsNot(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"v1", "v1.2", "01.2.3", "1.2.3.4"} {
		_, err := semver.Parse(s)
		require.NoError(t, err, s)
		assert.False(t, semver.IsExact(s), s)
	}
}

func TestParseConstraint(t *testing.T) {
	t.Parallel()

	c, err := semver.ParseConstraint(">= 1.2, < 2.0")
	require.NoError(t, err)

	assert.True(t, c.Check(semver.MustParse("1.5.0")))
	assert.False(t, c.Check(semver.MustParse("2.1.0")))

	_, err = semver.ParseConstraint("not a constraint")
	require.Error(t, err)
}

// TestCheckConstraint pins what the semver_check HCL function accepts,
// which is user-facing: it reads its version argument more strictly than
// [semver.Parse] does.
func TestCheckConstraint(t *testing.T) {
	t.Parallel()

	ok, err := semver.CheckConstraint("1.5.0", ">= 1.2, < 2.0")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = semver.CheckConstraint("2.1.0", ">= 1.2, < 2.0")
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = semver.CheckConstraint("1.2.3beta", ">= 1.2")
	require.Error(t, err, "an unseparated pre-release is rejected here but not by Parse")

	_, err = semver.CheckConstraint("1.2.3", "not a constraint")
	require.Error(t, err)
}

func TestMustParsePanicsOnGarbage(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { semver.MustParse("latest") })
}
