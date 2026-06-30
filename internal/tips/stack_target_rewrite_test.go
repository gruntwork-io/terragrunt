package tips_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSuggestStackTargetRewriteIsRestrictedToStacks pins the contract for
// SuggestStackTargetRewrite. For each input filter shape, the suggested
// rewrite must parse cleanly and produce a filter where
// IsRestrictedToStacks() returns true. Stack generate's filter semantics
// (see filter.Filters.RestrictToStacks) depend on that property, so if it
// ever drifts the tip would give users misleading advice.
func TestSuggestStackTargetRewriteIsRestrictedToStacks(t *testing.T) {
	t.Parallel()

	cases := []string{
		"./envs/prod",
		"./envs/prod/",
		"/abs/path/to/stack",
		"!./envs/prod",
		"./envs/prod | name=foo",
		"name=foo | ./envs/prod",
		"./envs/prod | reading=shared.hcl",
		"!./envs/prod | name=foo",
		"./envs/prod...",
		"...{./envs/prod}",
	}

	for _, original := range cases {
		t.Run(original, func(t *testing.T) {
			t.Parallel()

			rewrite := tips.SuggestStackTargetRewrite(original)

			parsed, err := filter.Parse(rewrite)
			require.NoError(t, err, "rewrite must parse: %q", rewrite)

			assert.True(
				t,
				parsed.Expression().IsRestrictedToStacks(),
				"rewrite %q must be restricted to stacks", rewrite,
			)
		})
	}
}
