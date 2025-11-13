package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluate_PathFilter(t *testing.T) {
	t.Parallel()

	components := []component.Component{
		component.NewUnit("./apps/app1"),
		component.NewUnit("./apps/app2"),
		component.NewUnit("./apps/legacy"),
		component.NewUnit("./libs/db"),
		component.NewUnit("./libs/api"),
		component.NewUnit("./apps/subdir/nested"),
	}

	for _, c := range components {
		c.SetDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		})
	}

	tests := []struct {
		name     string
		filter   *filter.PathExpression
		expected []component.Component
	}{
		{
			name:   "exact path match",
			filter: &filter.PathExpression{Value: "./apps/app1"},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:   "glob with single wildcard",
			filter: &filter.PathExpression{Value: "./apps/*"},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/legacy").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:   "glob with single wildcard and partial match",
			filter: &filter.PathExpression{Value: "./apps/app*"},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:   "glob with recursive wildcard",
			filter: &filter.PathExpression{Value: "./apps/**"},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/legacy").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/subdir/nested").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:     "no matches",
			filter:   &filter.PathExpression{Value: "./nonexistent/*"},
			expected: []component.Component{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := log.New()
			result, err := filter.Evaluate(l, tt.filter, components)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter(t *testing.T) {
	t.Parallel()

	components := []component.Component{
		component.NewUnit("./apps/app"),
		component.NewUnit("./libs/app"),
		component.NewUnit("./libs/db"),
		component.NewUnit("./libs/api"),
		component.NewStack("./libs/api"),
	}

	tests := []struct {
		name     string
		filter   *filter.AttributeExpression
		expected []component.Component
	}{
		{
			name:   "name filter single match",
			filter: &filter.AttributeExpression{Key: "name", Value: "db"},
			expected: []component.Component{
				component.NewUnit("./libs/db"),
			},
		},
		{
			name:   "name filter multiple matches",
			filter: &filter.AttributeExpression{Key: "name", Value: "app"},
			expected: []component.Component{
				component.NewUnit("./apps/app"),
				component.NewUnit("./libs/app"),
			},
		},
		{
			name:     "name filter no matches",
			filter:   &filter.AttributeExpression{Key: "name", Value: "nonexistent"},
			expected: []component.Component{},
		},
		{
			name:   "type filter unit",
			filter: &filter.AttributeExpression{Key: "type", Value: "unit"},
			expected: []component.Component{
				component.NewUnit("./apps/app"),
				component.NewUnit("./libs/app"),
				component.NewUnit("./libs/db"),
				component.NewUnit("./libs/api"),
			},
		},
		{
			name:   "type filter stack",
			filter: &filter.AttributeExpression{Key: "type", Value: "stack"},
			expected: []component.Component{
				component.NewStack("./libs/api"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := log.New()
			result, err := filter.Evaluate(l, tt.filter, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter_InvalidKey(t *testing.T) {
	t.Parallel()

	components := []component.Component{
		component.NewUnit("./apps/app"),
	}

	attrFilter := &filter.AttributeExpression{Key: "invalid", Value: "foo"}
	l := log.New()
	result, err := filter.Evaluate(l, attrFilter, components)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unknown attribute key")
}

func TestEvaluate_AttributeFilter_Reading(t *testing.T) {
	t.Parallel()

	components := []component.Component{
		component.NewUnit("./apps/app1").WithReading("shared.hcl", "shared.tfvars"),
		component.NewUnit("./apps/app2").WithReading("shared.hcl", "common/variables.hcl"),
		component.NewUnit("./apps/app3").WithReading("config.yaml", "settings.json"),
		component.NewUnit("./libs/db").WithReading("database.hcl"),
		component.NewUnit("./libs/api").WithReading(),
		component.NewUnit("./apps/app4").WithReading("shared.hcl", "shared.tfvars", "extra.hcl"),
	}

	tests := []struct {
		name     string
		filter   *filter.AttributeExpression
		expected []component.Component
	}{
		{
			name:   "exact file path match - single match",
			filter: &filter.AttributeExpression{Key: "reading", Value: "database.hcl"},
			expected: []component.Component{
				component.NewUnit("./libs/db").WithReading("database.hcl"),
			},
		},
		{
			name:   "exact file path match - multiple matches",
			filter: &filter.AttributeExpression{Key: "reading", Value: "shared.hcl"},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithReading("shared.hcl", "shared.tfvars"),
				component.NewUnit("./apps/app2").WithReading("shared.hcl", "common/variables.hcl"),
				component.NewUnit("./apps/app4").WithReading("shared.hcl", "shared.tfvars", "extra.hcl"),
			},
		},
		{
			name:     "exact file path match - no matches",
			filter:   &filter.AttributeExpression{Key: "reading", Value: "nonexistent.hcl"},
			expected: []component.Component{},
		},
		{
			name:   "glob pattern with single wildcard - *.hcl",
			filter: &filter.AttributeExpression{Key: "reading", Value: "*.hcl"},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithReading("shared.hcl", "shared.tfvars"),
				component.NewUnit("./apps/app2").WithReading("shared.hcl", "common/variables.hcl"),
				component.NewUnit("./libs/db").WithReading("database.hcl"),
				component.NewUnit("./apps/app4").WithReading("shared.hcl", "shared.tfvars", "extra.hcl"),
			},
		},
		{
			name:   "glob pattern with prefix - shared*",
			filter: &filter.AttributeExpression{Key: "reading", Value: "shared*"},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithReading("shared.hcl", "shared.tfvars"),
				component.NewUnit("./apps/app2").WithReading("shared.hcl", "common/variables.hcl"),
				component.NewUnit("./apps/app4").WithReading("shared.hcl", "shared.tfvars", "extra.hcl"),
			},
		},
		{
			name:   "glob pattern with double wildcard - **/variables.hcl",
			filter: &filter.AttributeExpression{Key: "reading", Value: "**/variables.hcl"},
			expected: []component.Component{
				component.NewUnit("./apps/app2").WithReading("shared.hcl", "common/variables.hcl"),
			},
		},
		{
			name:     "empty Reading slice - no matches",
			filter:   &filter.AttributeExpression{Key: "reading", Value: "*.hcl"},
			expected: []component.Component{},
		},
		{
			name:   "glob pattern with question mark - config.???l",
			filter: &filter.AttributeExpression{Key: "reading", Value: "config.???l"},
			expected: []component.Component{
				component.NewUnit("./apps/app3").WithReading("config.yaml", "settings.json"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var testComponents []component.Component
			if tt.name == "empty Reading slice - no matches" {
				testComponents = []component.Component{
					component.NewUnit("./libs/api").WithReading(),
				}
			} else {
				testComponents = components
			}

			l := log.New()
			result, err := filter.Evaluate(l, tt.filter, testComponents)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter_Source(t *testing.T) {
	t.Parallel()

	components := []component.Component{
		component.NewUnit("./apps/app1").WithConfig(
			&config.TerragruntConfig{
				Terraform: &config.TerraformConfig{
					Source: helpers.PointerTo("github.com/acme/foo"),
				},
			},
		),
		component.NewUnit("./apps/app2").WithConfig(
			&config.TerragruntConfig{
				Terraform: &config.TerraformConfig{
					Source: helpers.PointerTo("git::git@github.com:acme/bar?ref=v1.0.0"),
				},
			},
		),
	}

	tests := []struct {
		name     string
		filter   *filter.AttributeExpression
		expected []component.Component
	}{
		{
			name:   "glob pattern with single wildcard - github.com/acme/*",
			filter: &filter.AttributeExpression{Key: "source", Value: "github.com/acme/*"},
			expected: []component.Component{
				components[0],
			},
		},
		{
			name:   "glob pattern with double wildcard - git::git@github.com:acme/**",
			filter: &filter.AttributeExpression{Key: "source", Value: "git::git@github.com:acme/**"},
			expected: []component.Component{
				components[1],
			},
		},
		{
			name:   "glob pattern with double wildcard - **github.com**",
			filter: &filter.AttributeExpression{Key: "source", Value: "**github.com**"},
			expected: []component.Component{
				components[0],
				components[1],
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := log.New()
			result, err := filter.Evaluate(l, tt.filter, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_AttributeFilter_Reading_ComponentAddedOnlyOnce(t *testing.T) {
	t.Parallel()

	components := []component.Component{
		component.NewUnit("./apps/app1").WithReading("shared.hcl", "shared.tfvars", "shared.yaml"),
	}

	// This glob should match multiple files in the Reading slice, but component should only be added once
	attrFilter := &filter.AttributeExpression{Key: "reading", Value: "shared*"}
	l := log.New()
	result, err := filter.Evaluate(l, attrFilter, components)
	require.NoError(t, err)

	// Should only have one component even though three files matched
	assert.Len(t, result, 1)
	assert.Equal(t, "./apps/app1", result[0].Path())
}

func TestEvaluate_PrefixExpression(t *testing.T) {
	t.Parallel()

	components := []component.Component{
		component.NewUnit("./apps/app1"),
		component.NewUnit("./apps/app2"),
		component.NewUnit("./apps/legacy"),
		component.NewUnit("./libs/db"),
	}

	for _, c := range components {
		c.SetDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		})
	}

	tests := []struct {
		name     string
		expr     *filter.PrefixExpression
		expected []component.Component
	}{
		{
			name: "exclude by name",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.AttributeExpression{Key: "name", Value: "legacy"},
			},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name: "exclude by path",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.PathExpression{Value: "./apps/legacy"},
			},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name: "exclude by glob",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.PathExpression{Value: "./apps/*"},
			},
			expected: []component.Component{
				component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name: "exclude all (double negation effect)",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.AttributeExpression{Key: "type", Value: "unit"},
			},
			expected: []component.Component{},
		},
		{
			name: "exclude nothing",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right:    &filter.AttributeExpression{Key: "name", Value: "nonexistent"},
			},
			expected: components,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := log.New()
			result, err := filter.Evaluate(l, tt.expr, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_InfixExpression(t *testing.T) {
	t.Parallel()

	components := []component.Component{
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

	tests := []struct {
		name     string
		expr     *filter.InfixExpression
		expected []component.Component
	}{
		{
			name: "intersection of path and name",
			expr: &filter.InfixExpression{
				Left:     &filter.PathExpression{Value: "./apps/*"},
				Operator: "|",
				Right:    &filter.AttributeExpression{Key: "name", Value: "app1"},
			},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name: "intersection with no overlap",
			expr: &filter.InfixExpression{
				Left:     &filter.PathExpression{Value: "./apps/*"},
				Operator: "|",
				Right:    &filter.AttributeExpression{Key: "name", Value: "db"},
			},
			expected: []component.Component{},
		},
		{
			name: "intersection of exact path and name",
			expr: &filter.InfixExpression{
				Left:     &filter.PathExpression{Value: "./apps/app1"},
				Operator: "|",
				Right:    &filter.AttributeExpression{Key: "name", Value: "app1"},
			},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name: "intersection of empty results",
			expr: &filter.InfixExpression{
				Left:     &filter.AttributeExpression{Key: "name", Value: "nonexistent1"},
				Operator: "|",
				Right:    &filter.AttributeExpression{Key: "name", Value: "app1"},
			},
			expected: []component.Component{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := log.New()
			result, err := filter.Evaluate(l, tt.expr, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_ComplexExpressions(t *testing.T) {
	t.Parallel()

	components := []component.Component{
		component.NewUnit("./apps/app1"),
		component.NewUnit("./apps/app2"),
		component.NewUnit("./apps/legacy"),
		component.NewUnit("./libs/db"),
		component.NewUnit("./libs/api"),
		component.NewUnit("./special/unit"),
	}

	for _, c := range components {
		c.SetDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		})
	}

	tests := []struct {
		name     string
		expr     filter.Expression
		expected []component.Component
	}{
		{
			name: "intersection with negation (refinement)",
			expr: &filter.InfixExpression{
				Left:     &filter.PathExpression{Value: "./apps/*"},
				Operator: "|",
				Right: &filter.PrefixExpression{
					Operator: "!",
					Right:    &filter.AttributeExpression{Key: "name", Value: "legacy"},
				},
			},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name: "negated intersection",
			expr: &filter.PrefixExpression{
				Operator: "!",
				Right: &filter.InfixExpression{
					Left:     &filter.PathExpression{Value: "./apps/*"},
					Operator: "|",
					Right:    &filter.AttributeExpression{Key: "name", Value: "app1"},
				},
			},
			expected: []component.Component{
				component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/legacy").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./libs/api").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./special/unit").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name: "chained intersections (multiple refinements)",
			expr: &filter.InfixExpression{
				Left: &filter.InfixExpression{
					Left:     &filter.PathExpression{Value: "./apps/*"},
					Operator: "|",
					Right:    &filter.PrefixExpression{Operator: "!", Right: &filter.AttributeExpression{Key: "name", Value: "legacy"}},
				},
				Operator: "|",
				Right:    &filter.AttributeExpression{Key: "name", Value: "app1"},
			},
			expected: []component.Component{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := log.New()
			result, err := filter.Evaluate(l, tt.expr, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil expression", func(t *testing.T) {
		t.Parallel()

		components := []component.Component{component.NewUnit("./app")}
		l := log.New()
		result, err := filter.Evaluate(l, nil, components)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "expression is nil")
	})

	t.Run("empty components list", func(t *testing.T) {
		t.Parallel()

		expr := &filter.AttributeExpression{Key: "name", Value: "foo"}
		l := log.New()
		result, err := filter.Evaluate(l, expr, []component.Component{})

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("invalid glob pattern", func(t *testing.T) {
		t.Parallel()

		components := []component.Component{component.NewUnit("./app")}
		expr := &filter.PathExpression{Value: "[invalid-glob"}
		l := log.New()
		result, err := filter.Evaluate(l, expr, components)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to compile glob pattern")
	})
}

func TestEvaluate_GraphExpression(t *testing.T) {
	t.Parallel()

	// Create a component graph: vpc -> db -> app
	vpc := component.NewUnit("./vpc")
	db := component.NewUnit("./db")
	app := component.NewUnit("./app")

	// Set up dependencies: app depends on db, db depends on vpc
	app.AddDependency(db)
	db.AddDependency(vpc)

	components := []component.Component{vpc, db, app}

	tests := []struct {
		name     string
		expr     *filter.GraphExpression
		expected []component.Component
	}{
		{
			name: "dependency traversal - app...",
			expr: &filter.GraphExpression{
				Target:              &filter.AttributeExpression{Key: "name", Value: "app"},
				IncludeDependencies: true,
				IncludeDependents:   false,
				ExcludeTarget:       false,
			},
			expected: []component.Component{app, db, vpc},
		},
		{
			name: "dependent traversal - ...vpc",
			expr: &filter.GraphExpression{
				Target:              &filter.AttributeExpression{Key: "name", Value: "vpc"},
				IncludeDependencies: false,
				IncludeDependents:   true,
				ExcludeTarget:       false,
			},
			expected: []component.Component{vpc, db, app},
		},
		{
			name: "both directions - ...db...",
			expr: &filter.GraphExpression{
				Target:              &filter.AttributeExpression{Key: "name", Value: "db"},
				IncludeDependencies: true,
				IncludeDependents:   true,
				ExcludeTarget:       false,
			},
			expected: []component.Component{db, vpc, app},
		},
		{
			name: "exclude target - ^app...",
			expr: &filter.GraphExpression{
				Target:              &filter.AttributeExpression{Key: "name", Value: "app"},
				IncludeDependencies: true,
				IncludeDependents:   false,
				ExcludeTarget:       true,
			},
			expected: []component.Component{db, vpc},
		},
		{
			name: "exclude target with dependents - ...^db...",
			expr: &filter.GraphExpression{
				Target:              &filter.AttributeExpression{Key: "name", Value: "db"},
				IncludeDependencies: true,
				IncludeDependents:   true,
				ExcludeTarget:       true,
			},
			expected: []component.Component{vpc, app},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := log.New()
			result, err := filter.Evaluate(l, tt.expr, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEvaluate_GraphExpression_ComplexGraph(t *testing.T) {
	t.Parallel()

	// Create a more complex graph:
	// vpc -> [db, cache] -> app
	vpc := component.NewUnit("./vpc")
	db := component.NewUnit("./db")
	cache := component.NewUnit("./cache")
	app := component.NewUnit("./app")

	app.AddDependency(db)
	app.AddDependency(cache)
	db.AddDependency(vpc)
	cache.AddDependency(vpc)

	components := []component.Component{vpc, db, cache, app}

	t.Run("dependency traversal from app finds all dependencies", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "app"},
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, []component.Component{app, db, cache, vpc}, result)
	})

	t.Run("dependent traversal from vpc finds all dependents", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "vpc"},
			IncludeDependencies: false,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, []component.Component{vpc, db, cache, app}, result)
	})
}

func TestEvaluate_GraphExpression_EmptyResults(t *testing.T) {
	t.Parallel()

	components := []component.Component{
		component.NewUnit("./app"),
		component.NewUnit("./db"),
	}

	t.Run("target doesn't match any component", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "nonexistent"},
			IncludeDependencies: true,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestEvaluate_GraphExpression_NoDependencies(t *testing.T) {
	t.Parallel()

	// Components with no dependencies or dependents
	isolated := component.NewUnit("./isolated")
	another := component.NewUnit("./another")

	components := []component.Component{isolated, another}

	t.Run("component with no dependencies", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "isolated"},
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, []component.Component{isolated}, result)
	})

	t.Run("component with no dependents", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "isolated"},
			IncludeDependencies: false,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, []component.Component{isolated}, result)
	})
}

func TestEvaluate_GraphExpression_CircularDependencies(t *testing.T) {
	t.Parallel()

	// Create a circular dependency: a -> b -> a
	// The traversal should not infinite loop
	a := component.NewUnit("./a")
	b := component.NewUnit("./b")

	a.AddDependency(b)
	b.AddDependency(a)

	components := []component.Component{a, b}

	t.Run("circular dependency - dependency traversal stops at cycle", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "a"},
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		// Should include both a and b, but not loop infinitely
		assert.ElementsMatch(t, []component.Component{a, b}, result)
		assert.Len(t, result, 2)
	})

	t.Run("circular dependency - dependent traversal stops at cycle", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "a"},
			IncludeDependencies: false,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		// Should include both a and b, but not loop infinitely
		assert.ElementsMatch(t, []component.Component{a, b}, result)
		assert.Len(t, result, 2)
	})
}

func TestEvaluate_GraphExpression_WithPathFilter(t *testing.T) {
	t.Parallel()

	vpc := component.NewUnit("./vpc").WithDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: ".",
	})
	db := component.NewUnit("./db").WithDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: ".",
	})
	app := component.NewUnit("./app").WithDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: ".",
	})

	app.AddDependency(db)
	db.AddDependency(vpc)

	components := []component.Component{vpc, db, app}

	t.Run("graph expression with path filter target", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.PathExpression{Value: "./app"},
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, []component.Component{app, db, vpc}, result)
	})
}

func TestEvaluate_GitFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fromRef   string
		toRef     string
		setup     func() []component.Component
		expected  []component.Component
		wantError bool
	}{
		{
			name:    "components without DiscoveryContext are filtered out",
			fromRef: "main",
			toRef:   "HEAD",
			setup: func() []component.Component {
				return []component.Component{
					component.NewUnit("./apps/app1"),
					component.NewUnit("./apps/app2"),
					component.NewUnit("./libs/db"),
				}
			},
			expected: []component.Component{},
		},
		{
			name:    "components with empty Ref are filtered out",
			fromRef: "main",
			toRef:   "HEAD",
			setup: func() []component.Component {
				app1 := component.NewUnit("./apps/app1")
				app1.SetDiscoveryContext(&component.DiscoveryContext{Ref: ""})
				app2 := component.NewUnit("./apps/app2")
				app2.SetDiscoveryContext(&component.DiscoveryContext{Ref: ""})
				return []component.Component{app1, app2}
			},
			expected: []component.Component{},
		},
		{
			name:    "components with Ref matching FromRef are included",
			fromRef: "main",
			toRef:   "HEAD",
			setup: func() []component.Component {
				app1 := component.NewUnit("./apps/app1")
				app1.SetDiscoveryContext(&component.DiscoveryContext{Ref: "main"})
				app2 := component.NewUnit("./apps/app2")
				app2.SetDiscoveryContext(&component.DiscoveryContext{Ref: "feature"})
				db := component.NewUnit("./libs/db")
				db.SetDiscoveryContext(&component.DiscoveryContext{Ref: "main"})
				return []component.Component{app1, app2, db}
			},
			expected: []component.Component{
				component.NewUnit("./apps/app1"),
				component.NewUnit("./libs/db"),
			},
		},
		{
			name:    "components with Ref matching ToRef are included",
			fromRef: "main",
			toRef:   "HEAD",
			setup: func() []component.Component {
				app1 := component.NewUnit("./apps/app1")
				app1.SetDiscoveryContext(&component.DiscoveryContext{Ref: "HEAD"})
				app2 := component.NewUnit("./apps/app2")
				app2.SetDiscoveryContext(&component.DiscoveryContext{Ref: "feature"})
				db := component.NewUnit("./libs/db")
				db.SetDiscoveryContext(&component.DiscoveryContext{Ref: "HEAD"})
				return []component.Component{app1, app2, db}
			},
			expected: []component.Component{
				component.NewUnit("./apps/app1"),
				component.NewUnit("./libs/db"),
			},
		},
		{
			name:    "components with Ref matching either FromRef or ToRef are included",
			fromRef: "main",
			toRef:   "HEAD",
			setup: func() []component.Component {
				app1 := component.NewUnit("./apps/app1")
				app1.SetDiscoveryContext(&component.DiscoveryContext{Ref: "main"})
				app2 := component.NewUnit("./apps/app2")
				app2.SetDiscoveryContext(&component.DiscoveryContext{Ref: "HEAD"})
				db := component.NewUnit("./libs/db")
				db.SetDiscoveryContext(&component.DiscoveryContext{Ref: "feature"})
				api := component.NewUnit("./libs/api")
				api.SetDiscoveryContext(&component.DiscoveryContext{Ref: "main"})
				return []component.Component{app1, app2, db, api}
			},
			expected: []component.Component{
				component.NewUnit("./apps/app1"),
				component.NewUnit("./apps/app2"),
				component.NewUnit("./libs/api"),
			},
		},
		{
			name:    "components with Ref not matching either are filtered out",
			fromRef: "main",
			toRef:   "HEAD",
			setup: func() []component.Component {
				app1 := component.NewUnit("./apps/app1")
				app1.SetDiscoveryContext(&component.DiscoveryContext{Ref: "feature"})
				app2 := component.NewUnit("./apps/app2")
				app2.SetDiscoveryContext(&component.DiscoveryContext{Ref: "develop"})
				db := component.NewUnit("./libs/db")
				db.SetDiscoveryContext(&component.DiscoveryContext{Ref: "release"})
				return []component.Component{app1, app2, db}
			},
			expected: []component.Component{},
		},
		{
			name:    "mixed components with and without DiscoveryContext",
			fromRef: "main",
			toRef:   "HEAD",
			setup: func() []component.Component {
				app1 := component.NewUnit("./apps/app1")
				app1.SetDiscoveryContext(&component.DiscoveryContext{Ref: "main"})
				app2 := component.NewUnit("./apps/app2")
				// No DiscoveryContext set
				db := component.NewUnit("./libs/db")
				db.SetDiscoveryContext(&component.DiscoveryContext{Ref: "HEAD"})
				api := component.NewUnit("./libs/api")
				api.SetDiscoveryContext(&component.DiscoveryContext{Ref: ""})
				return []component.Component{app1, app2, db, api}
			},
			expected: []component.Component{
				component.NewUnit("./apps/app1"),
				component.NewUnit("./libs/db"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gitFilter := filter.NewGitExpression(tt.fromRef, tt.toRef)
			components := tt.setup()

			l := log.New()
			result, err := filter.Evaluate(l, gitFilter, components)

			if tt.wantError {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)

				resultPaths := make([]string, len(result))
				for i, c := range result {
					resultPaths[i] = c.Path()
				}

				expectedPaths := make([]string, len(tt.expected))
				for i, c := range tt.expected {
					expectedPaths[i] = c.Path()
				}

				assert.ElementsMatch(t, expectedPaths, resultPaths)
			}
		})
	}
}

func TestEvaluate_GitFilterString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filter   *filter.GitExpression
		expected string
	}{
		{
			name:     "two references",
			filter:   filter.NewGitExpression("main", "HEAD"),
			expected: "[main...HEAD]",
		},
		{
			name:     "commit SHA references",
			filter:   filter.NewGitExpression("abc123", "def456"),
			expected: "[abc123...def456]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.filter.String())
		})
	}
}

func TestGitFilter_RequiresDiscovery(t *testing.T) {
	t.Parallel()

	gitFilter := filter.NewGitExpression("main", "HEAD")

	expr, requires := gitFilter.RequiresDiscovery()
	assert.True(t, requires)
	assert.Equal(t, gitFilter, expr)
}

func TestGitFilter_RequiresParse(t *testing.T) {
	t.Parallel()

	gitFilter := filter.NewGitExpression("main", "HEAD")

	expr, requires := gitFilter.RequiresParse()
	assert.False(t, requires)
	assert.Nil(t, expr)
}
