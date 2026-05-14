package discovery_test

import (
	"testing"

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
