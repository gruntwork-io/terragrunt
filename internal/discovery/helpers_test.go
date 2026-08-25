package discovery_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
)

func TestRelPathOrAbs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		base   string
		target string
		want   string
	}{
		{
			name:   "both empty",
			base:   "",
			target: "",
			want:   ".",
		},
		{
			name:   "empty target against absolute base falls back to target",
			base:   "/a",
			target: "",
			want:   "",
		},
		{
			name:   "empty base against absolute target falls back to target",
			base:   "",
			target: "/a",
			want:   "/a",
		},
		{
			name:   "child of base",
			base:   "/a",
			target: "/a/b",
			want:   "b",
		},
		{
			name:   "same path",
			base:   "/a",
			target: "/a",
			want:   ".",
		},
		{
			name:   "up traversal",
			base:   "/a/b",
			target: "/a",
			want:   "..",
		},
		{
			name:   "sibling of base",
			base:   "/a/b",
			target: "/a/c",
			want:   "../c",
		},
		{
			name:   "absolute base with relative target falls back to target",
			base:   "/a",
			target: "b",
			want:   "b",
		},
		{
			name:   "relative base with absolute target falls back to target",
			base:   "a",
			target: "/b",
			want:   "/b",
		},
		{
			name:   "both relative, sibling",
			base:   "a/b",
			target: "a/c",
			want:   "../c",
		},
		{
			name:   "both relative, no shared ancestor",
			base:   "a",
			target: "b",
			want:   "../b",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()

			got := discovery.RelPathOrAbs(l, tc.base, tc.target, "test")
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestRelPathForComponentPicksTheBase pins which directory a component's paths
// are reported against. A component discovered in a git-filter worktree
// carries its own working dir, and reporting it against the caller's would
// emit a path that walks out of the worktree.
func TestRelPathForComponentPicksTheBase(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		discoveryWorkingDir string
		name                string
		target              string
		want                string
	}{
		{
			name:   "no discovery working dir falls back",
			target: "/estate/a",
			want:   "a",
		},
		{
			name:                "discovery working dir wins",
			discoveryWorkingDir: "/worktree",
			target:              "/worktree/a",
			want:                "a",
		},
		{
			name:                "fallback still applies against an empty discovery working dir",
			discoveryWorkingDir: "",
			target:              "/estate/nested/a",
			want:                filepath.Join("nested", "a"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := component.NewUnit(tc.target)
			c.SetDiscoveryContext(&component.DiscoveryContext{WorkingDir: tc.discoveryWorkingDir})

			got := discovery.RelPathForComponent(
				logger.CreateLogger(),
				c,
				"/estate",
				tc.target,
				"test",
			)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestRelPathForComponentToleratesNoDiscoveryContext pins that a component
// carrying no discovery context reports against the fallback rather than
// panicking.
func TestRelPathForComponentToleratesNoDiscoveryContext(t *testing.T) {
	t.Parallel()

	c := component.NewUnit("/estate/a")
	c.SetDiscoveryContext(nil)

	got := discovery.RelPathForComponent(logger.CreateLogger(), c, "/estate", "/estate/a", "test")
	assert.Equal(t, "a", got)
}
