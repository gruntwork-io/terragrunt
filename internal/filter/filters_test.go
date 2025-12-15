package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilters_ParseFilterQueries(t *testing.T) {
	t.Parallel()

	t.Run("empty filter list", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{})
		require.NoError(t, err)
		assert.NotNil(t, filters)
		assert.Equal(t, "[]", filters.String())
	})

	t.Run("single valid filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*"})
		require.NoError(t, err)
		assert.NotNil(t, filters)
		assert.Equal(t, `["./apps/*"]`, filters.String())
	})

	t.Run("multiple valid filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "name=db", "!legacy"})
		require.NoError(t, err)
		assert.NotNil(t, filters)
		assert.Equal(t, `["./apps/*","name=db","!legacy"]`, filters.String())
	})

	t.Run("single invalid filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"invalid |"})
		require.Error(t, err)
		assert.NotNil(t, filters)
		assert.Contains(t, err.Error(), "filter 0")
		assert.Contains(t, err.Error(), "invalid |")
	})

	t.Run("mixed valid and invalid filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "name=", "!legacy"})
		require.Error(t, err)
		assert.NotNil(t, filters)
		// Should have 2 valid filters parsed
		assert.Contains(t, filters.String(), "./apps/*")
		assert.Contains(t, filters.String(), "!legacy")
		// Error should mention the invalid filter
		assert.Contains(t, err.Error(), "filter 1")
		assert.Contains(t, err.Error(), "name=")
	})

	t.Run("multiple invalid filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"foo |", "bar |", "!baz"})
		require.Error(t, err)
		assert.NotNil(t, filters)
		// Should have 1 valid filter
		assert.Equal(t, `["!baz"]`, filters.String())
		// Error should mention both invalid filters
		assert.Contains(t, err.Error(), "filter 0")
		assert.Contains(t, err.Error(), "filter 1")
	})
}

func TestFilters_Evaluate(t *testing.T) {
	t.Parallel()

	components := component.Components{
		component.NewUnit("./apps/app1"),
		component.NewUnit("./apps/app2"),
		component.NewUnit("./apps/legacy"),
		component.NewUnit("./libs/db"),
		component.NewUnit("./libs/api"),
	}

	for _, c := range components {
		c.SetDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		})
	}

	t.Run("empty filters returns all components", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, components, result)
	})

	t.Run("single positive filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, components)
		require.NoError(t, err)

		expected := component.Components{
			component.NewUnit("./apps/app1"),
			component.NewUnit("./apps/app2"),
			component.NewUnit("./apps/legacy"),
		}

		for _, c := range expected {
			c.SetDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: ".",
			})
		}

		assert.ElementsMatch(t, expected, result)
	})

	t.Run("union of multiple positive filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/app1", "name=db"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, components)
		require.NoError(t, err)

		expected := component.Components{
			component.NewUnit("./apps/app1"),
			component.NewUnit("./libs/db"),
		}

		for _, c := range expected {
			c.SetDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: ".",
			})
		}

		assert.ElementsMatch(t, expected, result)
	})

	t.Run("union with overlapping results (deduplication)", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "name=app1"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, components)
		require.NoError(t, err)

		expected := component.Components{
			component.NewUnit("./apps/app1"),
			component.NewUnit("./apps/app2"),
			component.NewUnit("./apps/legacy"),
		}

		for _, c := range expected {
			c.SetDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: ".",
			})
		}

		assert.ElementsMatch(t, expected, result)
		// Verify no duplicates - should have exactly 3 components
		assert.Len(t, result, 3)
	})

	t.Run("positive filters then negative filter removes results", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "!legacy"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, components)
		require.NoError(t, err)

		expected := component.Components{
			component.NewUnit("./apps/app1"),
			component.NewUnit("./apps/app2"),
		}

		for _, c := range expected {
			c.SetDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: ".",
			})
		}

		assert.ElementsMatch(t, expected, result)
	})

	t.Run("multiple negative filters applied in sequence", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "!legacy", "!app2"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, components)
		require.NoError(t, err)

		expected := component.Components{
			component.NewUnit("./apps/app1"),
		}

		for _, c := range expected {
			c.SetDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: ".",
			})
		}

		assert.ElementsMatch(t, expected, result)
	})

	t.Run("only negative filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"!legacy", "!db"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, components)
		require.NoError(t, err)

		expected := component.Components{
			component.NewUnit("./apps/app1"),
			component.NewUnit("./apps/app2"),
			component.NewUnit("./libs/api"),
		}

		for _, c := range expected {
			c.SetDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: ".",
			})
		}

		assert.ElementsMatch(t, expected, result)
	})

	t.Run("complex mix of positive and negative filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{
			"./apps/*",
			"./libs/*",
			"!legacy",
			"!api",
		})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, components)
		require.NoError(t, err)

		expected := component.Components{
			component.NewUnit("./apps/app1"),
			component.NewUnit("./apps/app2"),
			component.NewUnit("./libs/db"),
		}

		for _, c := range expected {
			c.SetDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: ".",
			})
		}

		assert.ElementsMatch(t, expected, result)
	})
}

func TestFilters_HasPositiveFilter(t *testing.T) {
	t.Parallel()

	t.Run("empty filters - has positive filter is false", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{})
		require.NoError(t, err)
		assert.False(t, filters.HasPositiveFilter())
	})

	t.Run("single positive filter - has positive filter is true", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*"})
		require.NoError(t, err)
		assert.True(t, filters.HasPositiveFilter())
	})

	t.Run("single negative filter - has positive filter is false", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"!legacy"})
		require.NoError(t, err)
		assert.False(t, filters.HasPositiveFilter())
	})

	t.Run("multiple negative filters - has positive filter is false", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"!legacy", "!test"})
		require.NoError(t, err)
		assert.False(t, filters.HasPositiveFilter())
	})

	t.Run("multiple positive filters - has positive filter is true", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "./libs/*"})
		require.NoError(t, err)
		assert.True(t, filters.HasPositiveFilter())
	})

	t.Run("mixed positive and negative - has positive filter is true", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "!legacy"})
		require.NoError(t, err)
		assert.True(t, filters.HasPositiveFilter())
	})

	t.Run("negative then positive - has positive filter is true", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"!legacy", "./apps/*"})
		require.NoError(t, err)
		assert.True(t, filters.HasPositiveFilter())
	})
}

func TestFilters_String(t *testing.T) {
	t.Parallel()

	t.Run("empty filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{})
		require.NoError(t, err)
		assert.Equal(t, "[]", filters.String())
	})

	t.Run("single filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*"})
		require.NoError(t, err)
		assert.Equal(t, `["./apps/*"]`, filters.String())
	})

	t.Run("multiple filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "name=db", "!legacy"})
		require.NoError(t, err)
		assert.Equal(t, `["./apps/*","name=db","!legacy"]`, filters.String())
	})
}

func TestFilters_RequiresDependencyDiscovery(t *testing.T) {
	t.Parallel()

	t.Run("no graph expressions - empty result", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "name=db"})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		assert.Empty(t, targets)
	})

	t.Run("single dependency graph expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"app..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 1)

		// Verify the target is the correct expression
		expectedTarget := &filter.AttributeExpression{Key: "name", Value: "app"}
		assert.Equal(t, expectedTarget, targets[0])
	})

	t.Run("multiple dependency graph expressions", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"app...", "db..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 2)

		assert.Equal(t, &filter.AttributeExpression{Key: "name", Value: "app"}, targets[0])
		assert.Equal(t, &filter.AttributeExpression{Key: "name", Value: "db"}, targets[1])
	})

	t.Run("dependent-only graph expression - no dependency discovery", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"...app"})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		assert.Empty(t, targets)
	})

	t.Run("both directions graph expression - includes dependency discovery", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"...app..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 1)
		assert.Equal(t, &filter.AttributeExpression{Key: "name", Value: "app"}, targets[0])
	})

	t.Run("nested graph expressions in infix", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"app... | db..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 2)
	})

	t.Run("graph expression in prefix expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"!app..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 1)
	})

	t.Run("mixed graph and non-graph expressions", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"app...", "./apps/*"})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 1)
		assert.Equal(t, &filter.AttributeExpression{Key: "name", Value: "app"}, targets[0])
	})
}

func TestFilters_RequiresDependentDiscovery(t *testing.T) {
	t.Parallel()

	t.Run("no graph expressions - empty result", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "name=db"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		assert.Empty(t, targets)
	})

	t.Run("single dependent graph expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"...app"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 1)

		assert.Equal(t, &filter.AttributeExpression{Key: "name", Value: "app"}, targets[0])
	})

	t.Run("multiple dependent graph expressions", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"...app", "...db"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 2)

		assert.Equal(t, &filter.AttributeExpression{Key: "name", Value: "app"}, targets[0])
		assert.Equal(t, &filter.AttributeExpression{Key: "name", Value: "db"}, targets[1])
	})

	t.Run("dependency-only graph expression - no dependent discovery", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"app..."})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		assert.Empty(t, targets)
	})

	t.Run("both directions graph expression - includes dependent discovery", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"...app..."})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 1)
		assert.Equal(t, &filter.AttributeExpression{Key: "name", Value: "app"}, targets[0])
	})

	t.Run("nested graph expressions in infix", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"...app | ...db"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 2)
	})

	t.Run("graph expression in prefix expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"!...app"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 1)
	})

	t.Run("mixed graph and non-graph expressions", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"...app", "./apps/*"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 1)
		assert.Equal(t, &filter.AttributeExpression{Key: "name", Value: "app"}, targets[0])
	})
}

func TestFilters_RestrictToStacks(t *testing.T) {
	t.Parallel()

	t.Run("empty filters - empty result", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{})
		require.NoError(t, err)

		restricted := filters.RestrictToStacks()
		assert.Empty(t, restricted)
	})

	t.Run("single filter - restricted to stacks", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"type=stack"})
		require.NoError(t, err)

		restricted := filters.RestrictToStacks()
		require.Len(t, restricted, 1)
	})

	t.Run("multiple filters - one of them restricted to stacks", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"type=stack", "name=app"})
		require.NoError(t, err)

		restricted := filters.RestrictToStacks()
		require.Len(t, restricted, 1)
	})

	t.Run("multiple filters - none of them restricted to stacks", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"name=app", "type=unit"})
		require.NoError(t, err)

		restricted := filters.RestrictToStacks()
		require.Empty(t, restricted)
	})
}

func TestFilters_RequiresGitReferences(t *testing.T) {
	t.Parallel()

	t.Run("no Git filters - empty result", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"./apps/*", "name=db"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		assert.Empty(t, refs)
	})

	t.Run("single Git filter with one reference", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"[main]"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("single Git filter with two references", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"[main...HEAD]"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("multiple Git filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"[main...HEAD]", "[feature-branch]"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 3)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD", "feature-branch"})
	})

	t.Run("Git filters with deduplication", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"[main...HEAD]", "[HEAD...main]"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2) // main and HEAD, no duplicates
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("Git filter combined with other filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"[main...HEAD]", "./apps/*", "name=db"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("Git filter with negation", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"![main...HEAD]"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("Git filter with intersection", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"[main...HEAD] | ./apps/*"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("Git filter nested in graph expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries([]string{"[main...HEAD]..."})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})
}
