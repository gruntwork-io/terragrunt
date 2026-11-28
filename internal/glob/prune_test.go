package glob_test

import (
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/glob"
)

func TestPrunerAdmits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		admits  []string
		prunes  []string
	}{
		{
			name:    "literal path",
			pattern: "apps/match",
			admits:  []string{".", "apps", "apps/match"},
			prunes:  []string{"infra", "apps/ignore", "apps/match/nested"},
		},
		{
			name:    "single-segment wildcard",
			pattern: "apps/*",
			admits:  []string{".", "apps", "apps/one", "apps/two"},
			prunes:  []string{"infra", "apps/one/nested"},
		},
		{
			name:    "recursive root",
			pattern: "apps/**",
			admits:  []string{".", "apps", "apps/one", "apps/one/nested/deep"},
			prunes:  []string{"infra", "infra/apps"},
		},
		{
			name:    "wildcard before a globstar",
			pattern: "env/*/app/**",
			admits:  []string{"env", "env/dev", "env/dev/app", "env/dev/app/x/y"},
			prunes:  []string{"env/dev/db", "env/dev/db/x", "other"},
		},
		{
			name:    "the top of the walk",
			pattern: ".",
			admits:  []string{"."},
			prunes:  []string{"apps"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pruner := glob.NewPruner(glob.Roots(tt.pattern))

			for _, dir := range tt.admits {
				assert.True(t, pruner.Admits(dir), "admits %q", dir)
			}

			for _, dir := range tt.prunes {
				assert.False(t, pruner.Admits(dir), "prunes %q", dir)
			}
		})
	}
}

func TestPrunerAdmitsBelow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		below   []string
		leaves  []string
	}{
		{
			name:    "literal path",
			pattern: "apps/match",
			below:   []string{".", "apps"},
			leaves:  []string{"apps/match"},
		},
		{
			name:    "single-segment wildcard",
			pattern: "apps/*",
			below:   []string{".", "apps"},
			leaves:  []string{"apps/one", "apps/two"},
		},
		{
			name:    "recursive root",
			pattern: "apps/**",
			below:   []string{".", "apps", "apps/one", "apps/one/nested"},
		},
		{
			name:    "the top of the walk",
			pattern: ".",
			leaves:  []string{"."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pruner := glob.NewPruner(glob.Roots(tt.pattern))

			for _, dir := range tt.below {
				assert.True(t, pruner.AdmitsBelow(dir), "admits below %q", dir)
			}

			for _, dir := range tt.leaves {
				assert.True(t, pruner.Admits(dir), "admits %q", dir)
				assert.False(t, pruner.AdmitsBelow(dir), "admits nothing below %q", dir)
			}
		})
	}
}

func FuzzPrunerAdmitsEveryAncestorOfAMatch(f *testing.F) {
	f.Add("apps/*/x", "apps/one/x")
	f.Add("env/*/app/**", "env/dev/app/x")
	f.Add("{a,b/c}/**", "b/c/d")
	f.Add("a?/b", "ab/b")

	f.Fuzz(func(t *testing.T, pattern, candidate string) {
		if !gobwasHandles(pattern, candidate) || !isWalkInput(pattern, candidate) || !oracleMatch(pattern, candidate) {
			return
		}

		pruner := glob.NewPruner(glob.Roots(pattern))

		require.True(t, pruner.Admits("."), "%q matches %q but the top is pruned", pattern, candidate)

		if candidate == "." {
			return
		}

		require.True(t, pruner.AdmitsBelow("."), "%q matches %q but nothing below the top is admitted", pattern, candidate)

		dir := ""
		for segment := range strings.SplitSeq(candidate, "/") {
			dir = path.Join(dir, segment)
			require.Truef(t, pruner.Admits(dir), "%q matches %q but %q is pruned", pattern, candidate, dir)

			if dir != candidate {
				require.Truef(t, pruner.AdmitsBelow(dir), "%q matches %q but nothing below %q is admitted", pattern, candidate, dir)
			}
		}
	})
}

// isWalkInput reports whether pattern and candidate take the form a walk hands
// a pruner: a relative pattern, and a clean path relative to the walk's top.
func isWalkInput(pattern, candidate string) bool {
	return !strings.HasPrefix(pattern, "/") && candidate != "" && path.Clean(candidate) == candidate &&
		!path.IsAbs(candidate) && !strings.HasPrefix(candidate, "..")
}
