package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/config"
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
		assert.Contains(t, err.Error(), "failed to match path pattern")
	})
}

func TestEvaluate_GraphExpression(t *testing.T) {
	t.Parallel()

	// Create a component graph: vpc -> db -> app

	tests := []struct {
		expr     *filter.GraphExpression
		setup    func() []component.Component
		name     string
		expected []string
	}{
		{
			name: "dependency traversal - app...",
			expr: &filter.GraphExpression{
				Target:              &filter.AttributeExpression{Key: "name", Value: "app"},
				IncludeDependencies: true,
				IncludeDependents:   false,
				ExcludeTarget:       false,
			},
			expected: []string{"./app", "./db", "./vpc"},
			setup: func() []component.Component {
				vpcCtx := &component.DiscoveryContext{}
				vpcCtx.SuggestOrigin(component.OriginGraphDiscovery)
				vpc := component.NewUnit("./vpc").WithDiscoveryContext(vpcCtx)

				dbCtx := &component.DiscoveryContext{}
				dbCtx.SuggestOrigin(component.OriginGraphDiscovery)
				db := component.NewUnit("./db").WithDiscoveryContext(dbCtx)

				app := component.NewUnit("./app")

				app.AddDependency(db)
				db.AddDependency(vpc)

				return []component.Component{vpc, db, app}
			},
		},
		{
			name: "dependent traversal - ...vpc",
			expr: &filter.GraphExpression{
				Target:              &filter.AttributeExpression{Key: "name", Value: "vpc"},
				IncludeDependencies: false,
				IncludeDependents:   true,
				ExcludeTarget:       false,
			},
			expected: []string{"./vpc", "./db", "./app"},
			setup: func() []component.Component {
				vpc := component.NewUnit("./vpc")

				dbCtx := &component.DiscoveryContext{}
				dbCtx.SuggestOrigin(component.OriginGraphDiscovery)
				db := component.NewUnit("./db").WithDiscoveryContext(dbCtx)

				appCtx := &component.DiscoveryContext{}
				appCtx.SuggestOrigin(component.OriginGraphDiscovery)
				app := component.NewUnit("./app").WithDiscoveryContext(appCtx)

				app.AddDependency(db)
				db.AddDependency(vpc)

				return []component.Component{vpc, db, app}
			},
		},
		{
			name: "both directions - ...db...",
			expr: &filter.GraphExpression{
				Target:              &filter.AttributeExpression{Key: "name", Value: "db"},
				IncludeDependencies: true,
				IncludeDependents:   true,
				ExcludeTarget:       false,
			},
			expected: []string{"./db", "./vpc", "./app"},
			setup: func() []component.Component {
				vpcCtx := &component.DiscoveryContext{}
				vpcCtx.SuggestOrigin(component.OriginGraphDiscovery)
				vpc := component.NewUnit("./vpc").WithDiscoveryContext(vpcCtx)

				db := component.NewUnit("./db")

				appCtx := &component.DiscoveryContext{}
				appCtx.SuggestOrigin(component.OriginGraphDiscovery)
				app := component.NewUnit("./app").WithDiscoveryContext(appCtx)

				app.AddDependency(db)
				db.AddDependency(vpc)

				return []component.Component{vpc, db, app}
			},
		},
		{
			name: "exclude target - ^app...",
			expr: &filter.GraphExpression{
				Target:              &filter.AttributeExpression{Key: "name", Value: "app"},
				IncludeDependencies: true,
				IncludeDependents:   false,
				ExcludeTarget:       true,
			},
			expected: []string{"./db", "./vpc"},
			setup: func() []component.Component {
				vpcCtx := &component.DiscoveryContext{}
				vpcCtx.SuggestOrigin(component.OriginGraphDiscovery)
				vpc := component.NewUnit("./vpc").WithDiscoveryContext(vpcCtx)

				dbCtx := &component.DiscoveryContext{}
				dbCtx.SuggestOrigin(component.OriginGraphDiscovery)
				db := component.NewUnit("./db").WithDiscoveryContext(dbCtx)

				app := component.NewUnit("./app")

				app.AddDependency(db)
				db.AddDependency(vpc)

				return []component.Component{vpc, db, app}
			},
		},
		{
			name: "exclude target with dependents - ...^db...",
			expr: &filter.GraphExpression{
				Target:              &filter.AttributeExpression{Key: "name", Value: "db"},
				IncludeDependencies: true,
				IncludeDependents:   true,
				ExcludeTarget:       true,
			},
			expected: []string{"./vpc", "./app"},
			setup: func() []component.Component {
				vpcCtx := &component.DiscoveryContext{}
				vpcCtx.SuggestOrigin(component.OriginGraphDiscovery)
				vpc := component.NewUnit("./vpc").WithDiscoveryContext(vpcCtx)

				db := component.NewUnit("./db")

				appCtx := &component.DiscoveryContext{}
				appCtx.SuggestOrigin(component.OriginGraphDiscovery)
				app := component.NewUnit("./app").WithDiscoveryContext(appCtx)

				app.AddDependency(db)
				db.AddDependency(vpc)

				return []component.Component{vpc, db, app}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			components := tt.setup()

			expected := make([]component.Component, 0, len(tt.expected))

			expectedMap := make(map[string]bool)
			for _, path := range tt.expected {
				expectedMap[path] = true
			}

			for _, c := range components {
				if expectedMap[c.Path()] {
					expected = append(expected, c)
				}
			}

			l := log.New()
			result, err := filter.Evaluate(l, tt.expr, components)
			require.NoError(t, err)
			assert.ElementsMatch(t, expected, result)
		})
	}
}

func TestEvaluate_GraphExpression_ComplexGraph(t *testing.T) {
	t.Parallel()

	// Create a more complex graph:
	// vpc -> [db, cache] -> app

	t.Run("dependency traversal from app finds all dependencies", func(t *testing.T) {
		t.Parallel()

		vpcCtx := &component.DiscoveryContext{}
		vpcCtx.SuggestOrigin(component.OriginGraphDiscovery)
		vpc := component.NewUnit("./vpc").WithDiscoveryContext(vpcCtx)

		dbCtx := &component.DiscoveryContext{}
		dbCtx.SuggestOrigin(component.OriginGraphDiscovery)
		db := component.NewUnit("./db").WithDiscoveryContext(dbCtx)

		cacheCtx := &component.DiscoveryContext{}
		cacheCtx.SuggestOrigin(component.OriginGraphDiscovery)
		cache := component.NewUnit("./cache").WithDiscoveryContext(cacheCtx)

		app := component.NewUnit("./app")

		app.AddDependency(db)
		app.AddDependency(cache)
		db.AddDependency(vpc)
		cache.AddDependency(vpc)

		components := []component.Component{vpc, db, cache, app}

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

		vpc := component.NewUnit("./vpc")

		dbCtx := &component.DiscoveryContext{}
		dbCtx.SuggestOrigin(component.OriginGraphDiscovery)
		db := component.NewUnit("./db").WithDiscoveryContext(dbCtx)

		cacheCtx := &component.DiscoveryContext{}
		cacheCtx.SuggestOrigin(component.OriginGraphDiscovery)
		cache := component.NewUnit("./cache").WithDiscoveryContext(cacheCtx)

		appCtx := &component.DiscoveryContext{}
		appCtx.SuggestOrigin(component.OriginGraphDiscovery)
		app := component.NewUnit("./app").WithDiscoveryContext(appCtx)

		app.AddDependency(db)
		app.AddDependency(cache)
		db.AddDependency(vpc)
		cache.AddDependency(vpc)

		components := []component.Component{vpc, db, cache, app}

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

	bCtx := &component.DiscoveryContext{}
	bCtx.SuggestOrigin(component.OriginGraphDiscovery)
	b := component.NewUnit("./b").WithDiscoveryContext(bCtx)

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

	vpcCtx := &component.DiscoveryContext{
		WorkingDir: ".",
	}
	vpcCtx.SuggestOrigin(component.OriginGraphDiscovery)
	vpc := component.NewUnit("./vpc").WithDiscoveryContext(vpcCtx)

	dbCtx := &component.DiscoveryContext{
		WorkingDir: ".",
	}
	dbCtx.SuggestOrigin(component.OriginGraphDiscovery)
	db := component.NewUnit("./db").WithDiscoveryContext(dbCtx)

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

func TestEvaluate_GraphExpression_DepthLimited(t *testing.T) {
	t.Parallel()

	// Create a component graph: a -> b -> c -> d
	a := component.NewUnit("./a")
	b := component.NewUnit("./b")
	c := component.NewUnit("./c")
	d := component.NewUnit("./d")

	// Set up dependencies: d depends on c, c depends on b, b depends on a
	d.AddDependency(c)
	c.AddDependency(b)
	b.AddDependency(a)

	components := []component.Component{a, b, c, d}

	t.Run("depth 1 dependency traversal from d", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "d"},
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       false,
			DependencyDepth:     1,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, []component.Component{d, c}, result)
	})

	t.Run("depth 2 dependency traversal from d", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "d"},
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       false,
			DependencyDepth:     2,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, []component.Component{d, c, b}, result)
	})

	t.Run("depth 1 dependent traversal from a", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "a"},
			IncludeDependencies: false,
			IncludeDependents:   true,
			ExcludeTarget:       false,
			DependentDepth:      1,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, []component.Component{a, b}, result)
	})

	t.Run("depth 2 dependent traversal from a", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "a"},
			IncludeDependencies: false,
			IncludeDependents:   true,
			ExcludeTarget:       false,
			DependentDepth:      2,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, []component.Component{a, b, c}, result)
	})

	t.Run("unlimited depth (0) traverses all", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              &filter.AttributeExpression{Key: "name", Value: "d"},
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       false,
			DependencyDepth:     0,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)
		assert.ElementsMatch(t, []component.Component{d, c, b, a}, result)
	})
}

func TestEvaluate_GraphExpression_DepthLimited_MultipleTargets(t *testing.T) {
	t.Parallel()

	// Graph structure:
	//   targetA (2 hops from shared) --> intermediate --> shared --> deep1 --> deep2
	//   targetB (1 hop from shared) --> shared
	//
	// With depth=2:
	//   - From targetA: can reach intermediate, shared (2 hops)
	//   - From targetB: can reach shared, deep1 (2 hops)
	//   - Result should include deep1 even though targetA reaches shared first with less remaining depth

	ctx := &component.DiscoveryContext{WorkingDir: "."}

	targetA := component.NewUnit("./targetA").WithDiscoveryContext(ctx)
	targetB := component.NewUnit("./targetB").WithDiscoveryContext(ctx)
	intermediate := component.NewUnit("./intermediate").WithDiscoveryContext(ctx)
	shared := component.NewUnit("./shared").WithDiscoveryContext(ctx)
	deep1 := component.NewUnit("./deep1").WithDiscoveryContext(ctx)
	deep2 := component.NewUnit("./deep2").WithDiscoveryContext(ctx)

	// Set up dependencies
	targetA.AddDependency(intermediate)
	intermediate.AddDependency(shared)
	targetB.AddDependency(shared)
	shared.AddDependency(deep1)
	deep1.AddDependency(deep2)

	components := []component.Component{targetA, targetB, intermediate, shared, deep1, deep2}

	t.Run("multiple targets with shared dependency at different distances", func(t *testing.T) {
		t.Parallel()

		// Match both targetA and targetB using glob
		expr := &filter.GraphExpression{
			Target:              &filter.PathExpression{Value: "./target*"},
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       false,
			DependencyDepth:     2,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		// Should include: targetA, targetB, intermediate (1 hop from A), shared (2 hops from A, 1 from B), deep1 (2 hops from B)
		// Should NOT include: deep2 (3 hops from B, too deep)
		assert.ElementsMatch(t, []component.Component{targetA, targetB, intermediate, shared, deep1}, result)
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

// TestEvaluate_GraphExpressionWithGitExpressionTarget tests evaluating GraphExpressions
// where the target is a GitExpression.
//
// e.g.
// `... [main...commit] ...`
func TestEvaluate_GraphExpressionWithGitExpressionTarget(t *testing.T) {
	t.Parallel()

	t.Run("dependencies of git-changed component", func(t *testing.T) {
		t.Parallel()

		vpc := component.NewUnit("./vpc")
		db := component.NewUnit("./db")
		app := component.NewUnit("./app")

		app.AddDependency(db)
		db.AddDependency(vpc)

		discoveryCtx := &component.DiscoveryContext{Ref: "HEAD"}
		discoveryCtx.SuggestOrigin(component.OriginWorktreeDiscovery)

		db.SetDiscoveryContext(discoveryCtx)

		graphDiscoveryCtx := &component.DiscoveryContext{}
		graphDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)

		vpc.SetDiscoveryContext(graphDiscoveryCtx)
		app.SetDiscoveryContext(graphDiscoveryCtx)

		components := []component.Component{vpc, db, app}

		expr := &filter.GraphExpression{
			Target:              filter.NewGitExpression("main", "HEAD"),
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		resultPaths := make([]string, len(result))
		for i, c := range result {
			resultPaths[i] = c.Path()
		}

		assert.ElementsMatch(
			t,
			[]string{"./db", "./vpc"},
			resultPaths,
			"Should include db (git-matched) and vpc (its dependency)",
		)
	})

	t.Run("dependents of git-changed component", func(t *testing.T) {
		t.Parallel()

		vpc := component.NewUnit("./vpc")
		db := component.NewUnit("./db")
		app := component.NewUnit("./app")

		app.AddDependency(db)
		db.AddDependency(vpc)

		discoveryCtx := &component.DiscoveryContext{Ref: "HEAD"}
		discoveryCtx.SuggestOrigin(component.OriginWorktreeDiscovery)

		db.SetDiscoveryContext(discoveryCtx)

		graphDiscoveryCtx := &component.DiscoveryContext{}
		graphDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)

		vpc.SetDiscoveryContext(graphDiscoveryCtx)
		app.SetDiscoveryContext(graphDiscoveryCtx)

		components := []component.Component{vpc, db, app}

		expr := &filter.GraphExpression{
			Target:              filter.NewGitExpression("main", "HEAD"),
			IncludeDependencies: false,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		resultPaths := make([]string, len(result))
		for i, c := range result {
			resultPaths[i] = c.Path()
		}

		assert.ElementsMatch(
			t,
			[]string{"./db", "./app"},
			resultPaths,
			"Should include db (git-matched) and app (its dependent)",
		)
	})

	t.Run("both directions of git-changed component", func(t *testing.T) {
		t.Parallel()

		vpc := component.NewUnit("./vpc")
		db := component.NewUnit("./db")
		app := component.NewUnit("./app")

		app.AddDependency(db)
		db.AddDependency(vpc)

		discoveryCtx := &component.DiscoveryContext{Ref: "HEAD"}
		discoveryCtx.SuggestOrigin(component.OriginWorktreeDiscovery)

		db.SetDiscoveryContext(discoveryCtx)

		graphDiscoveryCtx := &component.DiscoveryContext{}
		graphDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)

		vpc.SetDiscoveryContext(graphDiscoveryCtx)
		app.SetDiscoveryContext(graphDiscoveryCtx)

		components := []component.Component{vpc, db, app}

		expr := &filter.GraphExpression{
			Target:              filter.NewGitExpression("main", "HEAD"),
			IncludeDependencies: true,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		resultPaths := make([]string, len(result))
		for i, c := range result {
			resultPaths[i] = c.Path()
		}

		assert.ElementsMatch(
			t,
			[]string{"./vpc", "./db", "./app"},
			resultPaths,
			"Should include db (git-matched), vpc (dependency), and app (dependent)",
		)
	})

	t.Run("no components match git filter - returns empty", func(t *testing.T) {
		t.Parallel()

		vpc := component.NewUnit("./vpc")
		db := component.NewUnit("./db")
		app := component.NewUnit("./app")

		app.AddDependency(db)
		db.AddDependency(vpc)

		graphDiscoveryCtx := &component.DiscoveryContext{}
		graphDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)

		vpc.SetDiscoveryContext(graphDiscoveryCtx)
		app.SetDiscoveryContext(graphDiscoveryCtx)

		components := []component.Component{vpc, db, app}

		expr := &filter.GraphExpression{
			Target:              filter.NewGitExpression("main", "HEAD"),
			IncludeDependencies: true,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		assert.Empty(t, result, "Should return empty when no components match git filter")
	})

	t.Run("multiple git-changed components with shared dependencies", func(t *testing.T) {
		t.Parallel()

		vpc := component.NewUnit("./vpc")
		db := component.NewUnit("./db")
		cache := component.NewUnit("./cache")
		app := component.NewUnit("./app")

		app.AddDependency(db)
		app.AddDependency(cache)
		db.AddDependency(vpc)
		cache.AddDependency(vpc)

		discoveryCtx := &component.DiscoveryContext{Ref: "HEAD"}
		discoveryCtx.SuggestOrigin(component.OriginWorktreeDiscovery)

		db.SetDiscoveryContext(discoveryCtx)

		discoveryCtx = &component.DiscoveryContext{Ref: "HEAD"}
		discoveryCtx.SuggestOrigin(component.OriginWorktreeDiscovery)

		cache.SetDiscoveryContext(discoveryCtx)

		graphDiscoveryCtx := &component.DiscoveryContext{}
		graphDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)

		vpc.SetDiscoveryContext(graphDiscoveryCtx)
		app.SetDiscoveryContext(graphDiscoveryCtx)

		components := []component.Component{vpc, db, cache, app}

		expr := &filter.GraphExpression{
			Target:              filter.NewGitExpression("main", "HEAD"),
			IncludeDependencies: true,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		resultPaths := make([]string, len(result))
		for i, c := range result {
			resultPaths[i] = c.Path()
		}

		assert.ElementsMatch(
			t,
			[]string{"./vpc", "./db", "./cache", "./app"},
			resultPaths,
			"Should include all components connected to git-changed components",
		)
	})

	t.Run("exclude target with git expression", func(t *testing.T) {
		t.Parallel()

		vpc := component.NewUnit("./vpc")
		db := component.NewUnit("./db")
		app := component.NewUnit("./app")

		app.AddDependency(db)
		db.AddDependency(vpc)

		discoveryCtx := &component.DiscoveryContext{Ref: "HEAD"}
		discoveryCtx.SuggestOrigin(component.OriginWorktreeDiscovery)

		db.SetDiscoveryContext(discoveryCtx)

		graphDiscoveryCtx := &component.DiscoveryContext{}
		graphDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)

		vpc.SetDiscoveryContext(graphDiscoveryCtx)
		app.SetDiscoveryContext(graphDiscoveryCtx)

		components := []component.Component{vpc, db, app}

		expr := &filter.GraphExpression{
			Target:              filter.NewGitExpression("main", "HEAD"),
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       true,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		resultPaths := make([]string, len(result))
		for i, c := range result {
			resultPaths[i] = c.Path()
		}

		assert.ElementsMatch(
			t,
			[]string{"./vpc"},
			resultPaths,
			"Should include only vpc (dependency), excluding db (target)",
		)
	})

	t.Run("git-changed component with Ref matching FromRef", func(t *testing.T) {
		t.Parallel()

		vpc := component.NewUnit("./vpc")
		db := component.NewUnit("./db")
		app := component.NewUnit("./app")

		app.AddDependency(db)
		db.AddDependency(vpc)

		discoveryCtx := &component.DiscoveryContext{Ref: "main"}
		discoveryCtx.SuggestOrigin(component.OriginWorktreeDiscovery)

		db.SetDiscoveryContext(discoveryCtx)

		graphDiscoveryCtx := &component.DiscoveryContext{}
		graphDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)

		vpc.SetDiscoveryContext(graphDiscoveryCtx)
		app.SetDiscoveryContext(graphDiscoveryCtx)

		components := []component.Component{vpc, db, app}

		expr := &filter.GraphExpression{
			Target:              filter.NewGitExpression("main", "HEAD"),
			IncludeDependencies: true,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		resultPaths := make([]string, len(result))
		for i, c := range result {
			resultPaths[i] = c.Path()
		}

		assert.ElementsMatch(
			t,
			[]string{"./vpc", "./db", "./app"},
			resultPaths,
			"Should include components when Ref matches FromRef",
		)
	})
}

// TestEvaluate_GraphExpressionWithGitTarget_DependencyChain tests that dependency
// traversal works correctly through a chain when the starting component is git-matched.
func TestEvaluate_GraphExpressionWithGitTarget_DependencyChain(t *testing.T) {
	t.Parallel()

	// Create a longer chain: a -> b -> c -> d -> e
	a := component.NewUnit("./a")
	b := component.NewUnit("./b")
	c := component.NewUnit("./c")
	d := component.NewUnit("./d")
	e := component.NewUnit("./e")

	a.AddDependency(b)
	b.AddDependency(c)
	c.AddDependency(d)
	d.AddDependency(e)

	aDiscoveryCtx := &component.DiscoveryContext{}
	aDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)
	a.SetDiscoveryContext(aDiscoveryCtx)

	bDiscoveryCtx := &component.DiscoveryContext{}
	bDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)
	b.SetDiscoveryContext(bDiscoveryCtx)

	cDiscoveryCtx := &component.DiscoveryContext{Ref: "HEAD"}
	cDiscoveryCtx.SuggestOrigin(component.OriginWorktreeDiscovery)
	c.SetDiscoveryContext(cDiscoveryCtx)

	dDiscoveryCtx := &component.DiscoveryContext{}
	dDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)
	d.SetDiscoveryContext(dDiscoveryCtx)

	eDiscoveryCtx := &component.DiscoveryContext{}
	eDiscoveryCtx.SuggestOrigin(component.OriginGraphDiscovery)
	e.SetDiscoveryContext(eDiscoveryCtx)

	components := []component.Component{a, b, c, d, e}

	t.Run("dependencies traverse the full chain", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              filter.NewGitExpression("main", "HEAD"),
			IncludeDependencies: true,
			IncludeDependents:   false,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		resultPaths := make([]string, len(result))
		for i, comp := range result {
			resultPaths[i] = comp.Path()
		}

		assert.ElementsMatch(t, []string{"./c", "./d", "./e"}, resultPaths)
	})

	t.Run("dependents traverse the full chain", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              filter.NewGitExpression("main", "HEAD"),
			IncludeDependencies: false,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		resultPaths := make([]string, len(result))
		for i, comp := range result {
			resultPaths[i] = comp.Path()
		}

		t.Logf("Result paths: %v", resultPaths)

		assert.ElementsMatch(t, []string{"./a", "./b", "./c"}, resultPaths)
	})

	t.Run("both directions traverse the full graph", func(t *testing.T) {
		t.Parallel()

		expr := &filter.GraphExpression{
			Target:              filter.NewGitExpression("main", "HEAD"),
			IncludeDependencies: true,
			IncludeDependents:   true,
			ExcludeTarget:       false,
		}

		l := log.New()
		result, err := filter.Evaluate(l, expr, components)
		require.NoError(t, err)

		resultPaths := make([]string, len(result))
		for i, comp := range result {
			resultPaths[i] = comp.Path()
		}

		t.Logf("Result paths: %v", resultPaths)

		assert.ElementsMatch(
			t,
			[]string{"./a", "./b", "./c", "./d", "./e"},
			resultPaths,
		)
	})
}
