package filter_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger creates a logger for tests with colors disabled.
func testLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}

func TestFilters_ParseFilterQueries(t *testing.T) {
	t.Parallel()

	t.Run("empty filter list", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{})
		require.NoError(t, err)
		assert.NotNil(t, filters)
		assert.Equal(t, "[]", filters.String())
	})

	t.Run("single valid filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*"})
		require.NoError(t, err)
		assert.NotNil(t, filters)
		assert.Equal(t, `["./apps/*"]`, filters.String())
	})

	t.Run("multiple valid filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"./apps/*", "name=db", "!legacy"},
		)
		require.NoError(t, err)
		assert.NotNil(t, filters)
		assert.Equal(t, `["./apps/*","name=db","!legacy"]`, filters.String())
	})

	t.Run("single invalid filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"invalid |"})
		require.Error(t, err)
		assert.NotNil(t, filters)
		// Rich diagnostic format
		assert.Contains(t, err.Error(), "--filter")
		assert.Contains(t, err.Error(), "invalid |")
	})

	t.Run("mixed valid and invalid filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"./apps/*", "name=", "!legacy"},
		)
		require.Error(t, err)
		assert.NotNil(t, filters)
		// Should have 2 valid filters parsed
		assert.Contains(t, filters.String(), "./apps/*")
		assert.Contains(t, filters.String(), "!legacy")
		// Error should mention the invalid filter with rich diagnostic format
		assert.Contains(t, err.Error(), "--filter[1]")
		assert.Contains(t, err.Error(), "name=")
	})

	t.Run("multiple invalid filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"foo |", "bar |", "!baz"})
		require.Error(t, err)
		assert.NotNil(t, filters)
		// Should have 1 valid filter
		assert.Equal(t, `["!baz"]`, filters.String())
		// Error should mention both invalid filters with rich diagnostic format
		assert.Contains(t, err.Error(), "--filter 'foo |'")
		assert.Contains(t, err.Error(), "--filter[1]")
	})

	t.Run("filter in parent directory", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"../apps/*"})
		require.NoError(t, err)
		assert.NotNil(t, filters)
		assert.Equal(t, `["../apps/*"]`, filters.String())
	})

	t.Run("name filter with slash", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"app/1"})
		require.NoError(t, err)
		assert.NotNil(t, filters)
		assert.Equal(t, `["app/1"]`, filters.String())
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

		filters, err := filter.ParseFilterQueries(testLogger(), []string{})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, filter.EvaluationContext{}, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, components, result)
	})

	t.Run("single positive filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, filter.EvaluationContext{}, components)
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

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/app1", "name=db"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, filter.EvaluationContext{}, components)
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

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*", "name=app1"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, filter.EvaluationContext{}, components)
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

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*", "!legacy"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, filter.EvaluationContext{}, components)
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

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"./apps/*", "!legacy", "!app2"},
		)
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, filter.EvaluationContext{}, components)
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

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"!legacy", "!db"})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, filter.EvaluationContext{}, components)
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

		filters, err := filter.ParseFilterQueries(testLogger(), []string{
			"./apps/*",
			"./libs/*",
			"!legacy",
			"!api",
		})
		require.NoError(t, err)

		l := log.New()
		result, err := filters.Evaluate(l, filter.EvaluationContext{}, components)
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

// TestFilters_EvaluateCompoundNegation pins compound filters to the left-to-right intersection
// documented for "|", so that a leading negation narrows the filter instead of turning the whole
// query into a bare exclusion that lets unrelated components through.
func TestFilters_EvaluateCompoundNegation(t *testing.T) {
	t.Parallel()

	newComponents := func() component.Components {
		components := component.Components{
			component.NewUnit("./apps/app1"),
			component.NewUnit("./apps/app2"),
			component.NewUnit("./apps/legacy"),
			component.NewUnit("./libs/db"),
			component.NewUnit("./libs/api"),
		}

		for _, c := range components {
			c.SetDiscoveryContext(&component.DiscoveryContext{WorkingDir: "."})
		}

		return components
	}

	tests := []struct {
		name     string
		filters  []string
		expected []string
	}{
		{
			name:     "negation on the left still narrows to the right operand",
			filters:  []string{"!name=app1 | ./apps/*"},
			expected: []string{"./apps/app2", "./apps/legacy"},
		},
		{
			name:     "operands that share no components select nothing",
			filters:  []string{"!./apps/* | name=app1"},
			expected: []string{},
		},
		{
			name:     "negation on the right",
			filters:  []string{"./apps/* | !name=legacy"},
			expected: []string{"./apps/app1", "./apps/app2"},
		},
		{
			name:     "negation on both operands",
			filters:  []string{"!name=app1 | !name=app2"},
			expected: []string{"./apps/legacy", "./libs/db", "./libs/api"},
		},
		{
			name:     "three operands chained left to right",
			filters:  []string{"!name=app1 | ./apps/* | !name=legacy"},
			expected: []string{"./apps/app2"},
		},
		{
			name:     "compound filter unions with another compound filter",
			filters:  []string{"!name=app1 | ./apps/*", "!name=db | ./libs/*"},
			expected: []string{"./apps/app2", "./apps/legacy", "./libs/api"},
		},
		{
			name:     "bare negation subtracts from a compound filter's selection",
			filters:  []string{"!name=app1 | ./apps/*", "!name=legacy"},
			expected: []string{"./apps/app2"},
		},
		{
			name:     "bare negation alone still starts from every component",
			filters:  []string{"!name=app1"},
			expected: []string{"./apps/app2", "./apps/legacy", "./libs/db", "./libs/api"},
		},
		{
			name:     "chained negations subtract from another filter's selection",
			filters:  []string{"./apps/*", "!name=legacy | !name=app2"},
			expected: []string{"./apps/app1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := testLogger()

			filters, err := filter.ParseFilterQueries(l, tt.filters)
			require.NoError(t, err)

			result, err := filters.Evaluate(l, filter.EvaluationContext{}, newComponents())
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expected, paths(result))

			if len(tt.filters) != 1 {
				return
			}

			// A single filter has no union or subtraction to arrange, so Filters.Evaluate owes
			// the same answer as walking the filter's own AST.
			direct, err := filters[0].Evaluate(l, filter.EvaluationContext{}, newComponents())
			require.NoError(t, err)

			assert.ElementsMatch(t, paths(direct), paths(result))
		})
	}
}

// TestFilters_EvaluateNegationsAreOrderIndependent pins that two filters that only exclude reach
// the same result whichever order they arrive in. A negated graph expression can only find the
// dependents to remove while its target is still in the set, so measuring each negation against
// the leftovers of the last one would let the order of the flags change the answer.
func TestFilters_EvaluateNegationsAreOrderIndependent(t *testing.T) {
	t.Parallel()

	orders := [][]string{
		{"!name=db", "!...name=db"},
		{"!...name=db", "!name=db"},
	}

	for _, order := range orders {
		t.Run(strings.Join(order, " then "), func(t *testing.T) {
			t.Parallel()

			l := testLogger()

			db := component.NewUnit("./libs/db")
			app := component.NewUnit("./apps/app")
			app.AddDependency(db)

			components := component.Components{db, app}
			for _, c := range components {
				c.SetDiscoveryContext(&component.DiscoveryContext{WorkingDir: "."})
			}

			filters, err := filter.ParseFilterQueries(l, order)
			require.NoError(t, err)

			result, err := filters.Evaluate(l, filter.EvaluationContext{}, components)
			require.NoError(t, err)

			assert.Empty(t, paths(result), "excluding db and its dependents leaves nothing")
		})
	}
}

func paths(components component.Components) []string {
	result := make([]string, 0, len(components))
	for _, c := range components {
		result = append(result, c.Path())
	}

	return result
}

func TestFilters_HasPositiveFilter(t *testing.T) {
	t.Parallel()

	t.Run("empty filters - has positive filter is false", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{})
		require.NoError(t, err)
		assert.False(t, filters.HasPositiveFilter())
	})

	t.Run("single positive filter - has positive filter is true", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*"})
		require.NoError(t, err)
		assert.True(t, filters.HasPositiveFilter())
	})

	t.Run("single negative filter - has positive filter is false", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"!legacy"})
		require.NoError(t, err)
		assert.False(t, filters.HasPositiveFilter())
	})

	t.Run("multiple negative filters - has positive filter is false", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"!legacy", "!test"})
		require.NoError(t, err)
		assert.False(t, filters.HasPositiveFilter())
	})

	t.Run("multiple positive filters - has positive filter is true", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*", "./libs/*"})
		require.NoError(t, err)
		assert.True(t, filters.HasPositiveFilter())
	})

	t.Run("mixed positive and negative - has positive filter is true", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*", "!legacy"})
		require.NoError(t, err)
		assert.True(t, filters.HasPositiveFilter())
	})

	t.Run("negative then positive - has positive filter is true", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"!legacy", "./apps/*"})
		require.NoError(t, err)
		assert.True(t, filters.HasPositiveFilter())
	})
}

func TestFilters_String(t *testing.T) {
	t.Parallel()

	t.Run("empty filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{})
		require.NoError(t, err)
		assert.Equal(t, "[]", filters.String())
	})

	t.Run("single filter", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*"})
		require.NoError(t, err)
		assert.Equal(t, `["./apps/*"]`, filters.String())
	})

	t.Run("multiple filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"./apps/*", "name=db", "!legacy"},
		)
		require.NoError(t, err)
		assert.Equal(t, `["./apps/*","name=db","!legacy"]`, filters.String())
	})
}

func TestFilters_RequiresDependencyDiscovery(t *testing.T) {
	t.Parallel()

	t.Run("no graph expressions - empty result", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*", "name=db"})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		assert.Empty(t, targets)
	})

	t.Run("single dependency graph expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"app..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 1)

		// Verify the target is the correct expression
		assert.Equal(t, mustAttr(t, "name", "app"), targets[0])
	})

	t.Run("multiple dependency graph expressions", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"app...", "db..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 2)

		assert.Equal(t, mustAttr(t, "name", "app"), targets[0])
		assert.Equal(t, mustAttr(t, "name", "db"), targets[1])
	})

	t.Run("dependent-only graph expression - no dependency discovery", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...app"})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		assert.Empty(t, targets)
	})

	t.Run("both directions graph expression - includes dependency discovery", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...app..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 1)
		assert.Equal(t, mustAttr(t, "name", "app"), targets[0])
	})

	t.Run("nested graph expressions in infix", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"app... | db..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 2)
	})

	t.Run("graph expression in prefix expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"!app..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 1)
	})

	t.Run("mixed graph and non-graph expressions", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"app...", "./apps/*"})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 1)
		assert.Equal(t, mustAttr(t, "name", "app"), targets[0])
	})
}

func TestFilters_RequiresDependentDiscovery(t *testing.T) {
	t.Parallel()

	t.Run("no graph expressions - empty result", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*", "name=db"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		assert.Empty(t, targets)
	})

	t.Run("single dependent graph expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...app"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 1)

		assert.Equal(t, mustAttr(t, "name", "app"), targets[0])
	})

	t.Run("multiple dependent graph expressions", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...app", "...db"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 2)

		assert.Equal(t, mustAttr(t, "name", "app"), targets[0])
		assert.Equal(t, mustAttr(t, "name", "db"), targets[1])
	})

	t.Run("dependency-only graph expression - no dependent discovery", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"app..."})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		assert.Empty(t, targets)
	})

	t.Run("both directions graph expression - includes dependent discovery", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...app..."})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 1)
		assert.Equal(t, mustAttr(t, "name", "app"), targets[0])
	})

	t.Run("nested graph expressions in infix", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...app | ...db"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 2)
	})

	t.Run("graph expression in prefix expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"!...app"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 1)
	})

	t.Run("mixed graph and non-graph expressions", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...app", "./apps/*"})
		require.NoError(t, err)

		targets := filters.DependentGraphExpressions()
		require.Len(t, targets, 1)
		assert.Equal(t, mustAttr(t, "name", "app"), targets[0])
	})
}

func TestFilters_RestrictToStacks(t *testing.T) {
	t.Parallel()

	t.Run("empty filters - empty result", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{})
		require.NoError(t, err)

		restricted := filters.RestrictToStacks()
		assert.Empty(t, restricted)
	})

	t.Run("single filter - restricted to stacks", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"type=stack"})
		require.NoError(t, err)

		restricted := filters.RestrictToStacks()
		require.Len(t, restricted, 1)
	})

	t.Run("multiple filters - one of them restricted to stacks", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"type=stack", "name=app"})
		require.NoError(t, err)

		restricted := filters.RestrictToStacks()
		require.Len(t, restricted, 1)
	})

	t.Run("multiple filters - none of them restricted to stacks", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"name=app", "type=unit"})
		require.NoError(t, err)

		restricted := filters.RestrictToStacks()
		require.Empty(t, restricted)
	})
}

func TestFilters_PartitionReadingFilters(t *testing.T) {
	t.Parallel()

	newReading := func(t *testing.T, value string) *filter.Filter {
		t.Helper()

		expr, err := filter.NewAttributeExpression(filter.AttributeReading, value)
		require.NoError(t, err)

		return filter.NewFilter(expr, expr.String())
	}

	newPath := func(t *testing.T, value string) *filter.Filter {
		t.Helper()

		expr, err := filter.NewPathFilter(value)
		require.NoError(t, err)

		return filter.NewFilter(expr, expr.String())
	}

	newName := func(t *testing.T, value string) *filter.Filter {
		t.Helper()

		expr, err := filter.NewAttributeExpression(filter.AttributeName, value)
		require.NoError(t, err)

		return filter.NewFilter(expr, expr.String())
	}

	t.Run("empty filters", func(t *testing.T) {
		t.Parallel()

		reading, other := filter.Filters{}.PartitionReadingFilters()
		assert.Empty(t, reading)
		assert.Empty(t, other)
	})

	t.Run("only reading filters", func(t *testing.T) {
		t.Parallel()

		filters := filter.Filters{newReading(t, "config/item-a.yml")}

		reading, other := filters.PartitionReadingFilters()
		assert.Len(t, reading, 1)
		assert.Empty(t, other)
	})

	t.Run("only non-reading filters", func(t *testing.T) {
		t.Parallel()

		filters := filter.Filters{newPath(t, "app")}

		reading, other := filters.PartitionReadingFilters()
		assert.Empty(t, reading)
		assert.Len(t, other, 1)
	})

	t.Run("mixed filters preserve order within groups", func(t *testing.T) {
		t.Parallel()

		filters := filter.Filters{
			newPath(t, "removed-unit"),
			newReading(t, "config/item-a.yml"),
			newName(t, "keep"),
			newReading(t, "config/item-b.yml"),
			newPath(t, "another-removed"),
		}

		reading, other := filters.PartitionReadingFilters()

		readingStrs := make([]string, 0, len(reading))
		for _, f := range reading {
			readingStrs = append(readingStrs, f.String())
		}

		otherStrs := make([]string, 0, len(other))
		for _, f := range other {
			otherStrs = append(otherStrs, f.String())
		}

		// Only reading-attribute filters land in the reading group; a non-reading attribute
		// filter (name=keep) must stay with the path filters in the other group.
		assert.Equal(
			t,
			[]string{"reading=config/item-a.yml", "reading=config/item-b.yml"},
			readingStrs,
		)
		assert.Equal(t, []string{"removed-unit", "name=keep", "another-removed"}, otherStrs)
	})
}

func TestFilters_ExcludingGitFilters(t *testing.T) {
	t.Parallel()

	t.Run("empty filters", func(t *testing.T) {
		t.Parallel()

		filters := filter.Filters{}
		result := filters.ExcludingGitFilters()
		assert.Empty(t, result)
	})

	t.Run("only git expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"[HEAD~1...HEAD]"})
		require.NoError(t, err)

		result := filters.ExcludingGitFilters()
		assert.Empty(t, result)
	})

	t.Run("no git expressions", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./live/**", "!./iac/**"})
		require.NoError(t, err)
		require.Len(t, filters, 2)

		result := filters.ExcludingGitFilters()
		assert.Len(t, result, 2)
	})

	t.Run("mixed git and non-git", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"[HEAD~1...HEAD]", "!./iac/**", "type=unit"},
		)
		require.NoError(t, err)
		require.Len(t, filters, 3)

		result := filters.ExcludingGitFilters()
		assert.Len(t, result, 2)

		// Verify the remaining filters are the non-git ones
		resultStrs := make([]string, 0, len(result))
		for _, f := range result {
			resultStrs = append(resultStrs, f.String())
		}

		assert.Contains(t, resultStrs, "!./iac/**")
		assert.Contains(t, resultStrs, "type=unit")
	})

	t.Run("infix containing git expression is excluded", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"[HEAD~1...HEAD] | !./iac/**"},
		)
		require.NoError(t, err)
		require.Len(t, filters, 1)

		result := filters.ExcludingGitFilters()
		assert.Empty(t, result, "infix filter containing a git expression should be excluded")
	})

	t.Run("does not modify original", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"[HEAD~1...HEAD]", "./live/**"},
		)
		require.NoError(t, err)
		require.Len(t, filters, 2)

		result := filters.ExcludingGitFilters()
		assert.Len(t, result, 1)
		assert.Len(t, filters, 2, "original should not be modified")
	})
}

func TestFilters_RequiresGitReferences(t *testing.T) {
	t.Parallel()

	t.Run("no Git filters - empty result", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"./apps/*", "name=db"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		assert.Empty(t, refs)
	})

	t.Run("single Git filter with one reference", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"[main]"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("single Git filter with two references", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"[main...HEAD]"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("multiple Git filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"[main...HEAD]", "[feature-branch]"},
		)
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 3)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD", "feature-branch"})
	})

	t.Run("Git filters with deduplication", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"[main...HEAD]", "[HEAD...main]"},
		)
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2) // main and HEAD, no duplicates
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("Git filter combined with other filters", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"[main...HEAD]", "./apps/*", "name=db"},
		)
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("Git filter with negation", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"![main...HEAD]"})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("Git filter with intersection", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(
			testLogger(),
			[]string{"[main...HEAD] | ./apps/*"},
		)
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})

	t.Run("Git filter nested in graph expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"[main...HEAD]..."})
		require.NoError(t, err)

		refs := filters.UniqueGitFilters().UniqueGitRefs()
		require.Len(t, refs, 2)
		assert.ElementsMatch(t, refs, []string{"main", "HEAD"})
	})
}

// TestFilters_GitExpressionAsGraphTarget tests that Filters correctly extracts
// GitExpression targets from GraphExpressions for dependency/dependent discovery.
func TestFilters_GitExpressionAsGraphTarget(t *testing.T) {
	t.Parallel()

	t.Run(
		"DependencyGraphExpressions extracts GitExpression target - dependencies only",
		func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(testLogger(), []string{"[main...HEAD]..."})
			require.NoError(t, err)

			targets := filters.DependencyGraphExpressions()
			require.Len(t, targets, 1, "Should have one dependency target expression")

			gitExpr, ok := targets[0].(*filter.GitExpression)
			require.True(t, ok, "Target should be a GitExpression, got %T", targets[0])
			assert.Equal(t, "main", gitExpr.FromRef)
			assert.Equal(t, "HEAD", gitExpr.ToRef)
		},
	)

	t.Run(
		"DependentGraphExpressions extracts GitExpression target - dependents only",
		func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(testLogger(), []string{"...[main...HEAD]"})
			require.NoError(t, err)

			targets := filters.DependentGraphExpressions()
			require.Len(t, targets, 1, "Should have one dependent target expression")

			gitExpr, ok := targets[0].(*filter.GitExpression)
			require.True(t, ok, "Target should be a GitExpression, got %T", targets[0])
			assert.Equal(t, "main", gitExpr.FromRef)
			assert.Equal(t, "HEAD", gitExpr.ToRef)
		},
	)

	t.Run(
		"Both graph expressions extract GitExpression target - issue #5307 pattern",
		func(t *testing.T) {
			t.Parallel()

			filters, err := filter.ParseFilterQueries(testLogger(), []string{"...[main...HEAD]..."})
			require.NoError(t, err)

			depTargets := filters.DependencyGraphExpressions()
			require.Len(t, depTargets, 1, "Should have one dependency target expression")

			depGitExpr, ok := depTargets[0].(*filter.GitExpression)
			require.True(t, ok, "Dependency target should be a GitExpression")
			assert.Equal(t, "main", depGitExpr.FromRef)
			assert.Equal(t, "HEAD", depGitExpr.ToRef)

			depentTargets := filters.DependentGraphExpressions()
			require.Len(t, depentTargets, 1, "Should have one dependent target expression")

			depentGitExpr, ok := depentTargets[0].(*filter.GitExpression)
			require.True(t, ok, "Dependent target should be a GitExpression")
			assert.Equal(t, "main", depentGitExpr.FromRef)
			assert.Equal(t, "HEAD", depentGitExpr.ToRef)
		},
	)

	t.Run("UniqueGitFilters extracts GitExpression from all graph positions", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...[main...HEAD]..."})
		require.NoError(t, err)

		gitFilters := filters.UniqueGitFilters()
		require.Len(t, gitFilters, 1, "Should have one unique git filter")
		assert.Equal(t, "main", gitFilters[0].FromRef)
		assert.Equal(t, "HEAD", gitFilters[0].ToRef)
	})

	t.Run("Multiple git expressions in graph - unique extraction", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{
			"[main...HEAD]...",
			"...[feature...develop]",
		})
		require.NoError(t, err)

		depTargets := filters.DependencyGraphExpressions()
		require.Len(t, depTargets, 1, "First filter has dependencies")

		depentTargets := filters.DependentGraphExpressions()
		require.Len(t, depentTargets, 1, "Second filter has dependents")

		gitFilters := filters.UniqueGitFilters()
		require.Len(t, gitFilters, 2, "Should have two unique git filters")
	})

	t.Run("Exclude target with GitExpression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"^[main...HEAD]..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 1)

		gitExpr, ok := targets[0].(*filter.GitExpression)
		require.True(t, ok)
		assert.Equal(t, "main", gitExpr.FromRef)
	})

	t.Run("Single git ref with graph expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"[main]..."})
		require.NoError(t, err)

		targets := filters.DependencyGraphExpressions()
		require.Len(t, targets, 1)

		gitExpr, ok := targets[0].(*filter.GitExpression)
		require.True(t, ok)
		assert.Equal(t, "main", gitExpr.FromRef)
		assert.Equal(t, "HEAD", gitExpr.ToRef)
	})

	t.Run("Git expression with commit SHA in graph", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...[abc123...def456]..."})
		require.NoError(t, err)

		depTargets := filters.DependencyGraphExpressions()
		require.Len(t, depTargets, 1)

		gitExpr, ok := depTargets[0].(*filter.GitExpression)
		require.True(t, ok)
		assert.Equal(t, "abc123", gitExpr.FromRef)
		assert.Equal(t, "def456", gitExpr.ToRef)
	})

	t.Run("RequiresParse returns true for git-graph expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...[main...HEAD]..."})
		require.NoError(t, err)

		expr, requires := filters.RequiresParse()
		assert.True(t, requires, "Git-graph expression should require parsing")
		assert.NotNil(t, expr)
	})

	t.Run("HasPositiveFilter returns true for git-graph expression", func(t *testing.T) {
		t.Parallel()

		filters, err := filter.ParseFilterQueries(testLogger(), []string{"...[main...HEAD]..."})
		require.NoError(t, err)

		assert.True(t, filters.HasPositiveFilter(), "Git-graph expression is a positive filter")
	})
}
