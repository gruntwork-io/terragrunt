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

// TestLockFileConstraint pins the `constraints` value tofu writes to a lock
// file for a provider's `version` argument.
func TestLockFileConstraint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "pessimistic with two parts", in: "~> 3.0", want: "~> 3.0"},
		{name: "pessimistic with one part", in: "~> 3", want: "~> 3.0"},
		{name: "pessimistic with three parts", in: "~> 3.2.0", want: "~> 3.2.0"},
		{name: "lower bound without a space", in: ">=3.0", want: ">= 3.0.0"},
		{name: "exact with equals", in: "= 3.2.3", want: "3.2.3"},
		{name: "range with the upper bound first", in: "< 4.0, >= 3.0", want: ">= 3.0.0, < 4.0.0"},
		{
			name: "three terms out of order",
			in:   "!= 3.2.0, < 4, ~> 3.0",
			want: "~> 3.0, != 3.2.0, < 4.0.0",
		},
		{name: "repeated term", in: ">= 3.0, >= 3.0.0", want: ">= 3.0.0"},
		{
			name: "pessimistic precisions sharing a version",
			in:   "~> 3.2, ~> 3.2.0",
			want: "~> 3.2.0, ~> 3.2",
		},
		{
			name: "numeric build metadata",
			in:   ">= 3.0.0+10, >= 3.0.0+2",
			want: ">= 3.0.0+2, >= 3.0.0+10",
		},
		{
			name: "build metadata with fewer identifiers",
			in:   ">= 3.0.0+a.1, >= 3.0.0+b",
			want: ">= 3.0.0+b, >= 3.0.0+a.1",
		},
		{
			name: "prerelease with fewer identifiers",
			in:   ">= 3.0.0-alpha.1, >= 3.0.0-beta",
			want: ">= 3.0.0-beta, >= 3.0.0-alpha.1",
		},
		{
			name: "release after its prerelease",
			in:   ">= 3.0.0+1, >= 3.0.0-beta+2",
			want: ">= 3.0.0-beta+2, >= 3.0.0+1",
		},
		{name: "dash with no prerelease after it", in: ">= 3.0-", want: ">= 3.0.0"},
		{name: "lower bound and pessimistic", in: ">= 3.0, ~> 3.1", want: ">= 3.0.0, ~> 3.1"},
		{name: "two pessimistic precisions", in: "~> 3.1, ~> 3.1.4", want: "~> 3.1, ~> 3.1.4"},
		{name: "repeated pessimistic with one and two parts", in: "~> 3, ~> 3.0", want: "~> 3.0"},
		{name: "repeated exact with and without equals", in: "= 3.2.3, 3.2.3", want: "3.2.3"},
		{
			name: "every operator sharing a version",
			in:   "!= 3.2.0, < 3.2.0, <= 3.2.0, ~> 3.2, ~> 3.2.0, 3.2.0, >= 3.2.0, > 3.2.0",
			want: "> 3.2.0, >= 3.2.0, 3.2.0, ~> 3.2.0, ~> 3.2, <= 3.2.0, < 3.2.0, != 3.2.0",
		},
		{name: "prerelease before its release", in: ">= 3.0.0, != 3.0.0-beta1", want: "!= 3.0.0-beta1, >= 3.0.0"},
		{name: "numeric build metadata in order", in: ">= 3.0.0+2, >= 3.0.0+10", want: ">= 3.0.0+2, >= 3.0.0+10"},
		{name: "numeric build metadata of one length", in: ">= 3.0.0+21, >= 3.0.0+12", want: ">= 3.0.0+12, >= 3.0.0+21"},
		{name: "numeric build metadata before text", in: ">= 3.0.0+1a, >= 3.0.0+2", want: ">= 3.0.0+2, >= 3.0.0+1a"},
		{name: "text build metadata", in: ">= 3.0.0+build10, >= 3.0.0+build2", want: ">= 3.0.0+build10, >= 3.0.0+build2"},
		{
			name: "build metadata identifier before the last decides over the count",
			in:   ">= 3.0.0+b.1, >= 3.0.0+a.x.y",
			want: ">= 3.0.0+a.x.y, >= 3.0.0+b.1",
		},
		{
			name: "build metadata with a shared first identifier",
			in:   ">= 3.0.0+a.b.c, >= 3.0.0+a.z",
			want: ">= 3.0.0+a.z, >= 3.0.0+a.b.c",
		},
		{
			name: "build metadata differing in the second identifier",
			in:   ">= 3.0.0+build.10, >= 3.0.0+build.2",
			want: ">= 3.0.0+build.2, >= 3.0.0+build.10",
		},
		{name: "no build metadata before build metadata", in: ">= 3.0.0+build, >= 3.0.0", want: ">= 3.0.0, >= 3.0.0+build"},
		{name: "build metadata before the operator", in: "> 3.0.0+2, != 3.0.0+1", want: "!= 3.0.0+1, > 3.0.0+2"},
		{name: "version number before build metadata", in: ">= 3.0.1+1, >= 3.0.0+2", want: ">= 3.0.0+2, >= 3.0.1+1"},
		{name: "repeated term with build metadata", in: ">= 3.0+2, >= 3.0.0+2", want: ">= 3.0.0+2"},
		{name: "pessimistic with two parts and build metadata", in: "~> 3.1+10, ~> 3.1+2", want: "~> 3.1+2, ~> 3.1+10"},
		{name: "numeric prerelease", in: ">= 3.0.0-10, >= 3.0.0-2", want: ">= 3.0.0-2, >= 3.0.0-10"},
		{name: "numeric prerelease before text", in: ">= 3.0.0-alpha, >= 3.0.0-1", want: ">= 3.0.0-1, >= 3.0.0-alpha"},
		{name: "text prerelease", in: ">= 3.0.0-rc2, >= 3.0.0-rc10", want: ">= 3.0.0-rc10, >= 3.0.0-rc2"},
		{
			name: "prerelease identifier before the last decides over the count",
			in:   ">= 3.0.0-beta.1, >= 3.0.0-alpha.x.y",
			want: ">= 3.0.0-alpha.x.y, >= 3.0.0-beta.1",
		},
		{
			name: "prerelease differing in the second identifier",
			in:   ">= 3.0.0-alpha.10, >= 3.0.0-alpha.2",
			want: ">= 3.0.0-alpha.2, >= 3.0.0-alpha.10",
		},
		{name: "prerelease before build metadata", in: ">= 3.0.0-beta+1, >= 3.0.0-alpha+2", want: ">= 3.0.0-alpha+2, >= 3.0.0-beta+1"},
		{name: "pessimistic with a dash and no prerelease after it", in: "~> 3.0-", want: "~> 3.0"},
		{name: "prerelease of a dash", in: ">= 3.0--", want: ">= 3.0.0--"},
		{name: "prerelease starting with a dash", in: ">= 3.0.0--a", want: ">= 3.0.0--a"},
		{name: "prerelease starting with a dot", in: ">= 3.0-.a", want: ">= 3.0.0-.a"},
		{name: "dash with no prerelease sorts as a release", in: ">= 3.0-, >= 3.0.0-beta", want: ">= 3.0.0-beta, >= 3.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := semver.LockFileConstraint(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestLockFileConstraintRejects pins the provider version arguments tofu
// rejects, including ones only one of its two parsers rejects.
func TestLockFileConstraintRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "not a constraint", in: "not a constraint"},
		{name: "reversed operator", in: "=> 2.2"},
		{name: "plus with no build metadata after it", in: ">= 3.0+"},
		{name: "letters after the version", in: ">= 3.0a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := semver.LockFileConstraint(tt.in)
			require.Error(t, err)
		})
	}
}

func TestMustParsePanicsOnGarbage(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { semver.MustParse("latest") })
}
