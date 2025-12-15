package filter_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testComponents = []component.Component{
	component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: ".",
	}),
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
	component.NewUnit("./services/web").WithDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: ".",
	}),
	component.NewUnit("./services/worker").WithDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: ".",
	}),
}

func TestFilter_ParseAndEvaluate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		filterString string
		expected     component.Components
		expectError  bool
	}{
		{
			name:         "simple name filter",
			filterString: "app1",
			expected: component.Components{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:         "attribute filter",
			filterString: "name=db",
			expected: component.Components{
				component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:         "path filter with wildcard",
			filterString: "./apps/*",
			expected: component.Components{
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
			name:         "negated filter",
			filterString: "!legacy",
			expected: component.Components{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./libs/api").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./services/web").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./services/worker").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:         "intersection of path and name",
			filterString: "./apps/* | app1",
			expected: component.Components{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:         "intersection with negation",
			filterString: "./apps/* | !legacy",
			expected: component.Components{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:         "chained intersections",
			filterString: "./apps/* | !legacy | app1",
			expected: component.Components{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:         "recursive wildcard",
			filterString: "./services/**",
			expected: component.Components{
				component.NewUnit("./services/web").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./services/worker").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:         "parse error - empty",
			filterString: "",
			expected:     nil,
			expectError:  true,
		},
		{
			name:         "parse error - invalid syntax",
			filterString: "foo |",
			expected:     nil,
			expectError:  true,
		},
		{
			name:         "parse error - incomplete expression",
			filterString: "name=",
			expected:     nil,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter, err := filter.Parse(tt.filterString)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, filter)

				return
			}

			require.NoError(t, err)

			require.NotNil(t, filter)

			logger := log.New()
			result, err := filter.Evaluate(logger, testComponents)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expected, result)

			// Verify String() returns original query
			assert.Equal(t, tt.filterString, filter.String())
		})
	}
}

func TestFilter_Apply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		filterString string
		components   component.Components
		expected     component.Components
		expectError  bool
	}{
		{
			name:         "apply with simple filter",
			filterString: "app1",
			components:   testComponents,
			expected: component.Components{
				component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:         "apply with path filter",
			filterString: "./libs/*",
			components:   testComponents,
			expected: component.Components{
				component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
				component.NewUnit("./libs/api").WithDiscoveryContext(&component.DiscoveryContext{
					WorkingDir: ".",
				}),
			},
		},
		{
			name:         "apply with empty components",
			filterString: "anything",
			components:   component.Components{},
			expected:     component.Components{},
		},
		{
			name:         "apply with parse error",
			filterString: "!",
			components:   testComponents,
			expected:     nil,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := log.New()
			result, err := filter.Apply(l, tt.filterString, tt.components)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, result)

				return
			}

			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestFilter_Expression(t *testing.T) {
	t.Parallel()

	filterString := "name=foo"
	f, err := filter.Parse(filterString)
	require.NoError(t, err)

	expr := f.Expression()
	assert.NotNil(t, expr)

	// Verify it's the correct type
	attrFilter, ok := expr.(*filter.AttributeExpression)
	assert.True(t, ok)
	assert.Equal(t, "name", attrFilter.Key)
	assert.Equal(t, "foo", attrFilter.Value)
}

func TestFilter_RealWorldScenarios(t *testing.T) {
	t.Parallel()

	repoComponents := []component.Component{
		component.NewUnit("./infrastructure/networking/vpc"),
		component.NewUnit("./infrastructure/networking/subnets"),
		component.NewUnit("./infrastructure/networking/security-groups"),
		component.NewUnit("./infrastructure/compute/app-server"),
		component.NewUnit("./infrastructure/compute/db-server"),
		component.NewUnit("./apps/frontend"),
		component.NewUnit("./apps/backend"),
		component.NewUnit("./apps/api"),
		component.NewUnit("./test/test-app"),
	}

	for _, c := range repoComponents {
		c.SetDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		})
	}

	tests := []struct {
		name         string
		filterString string
		description  string
		expected     []string
	}{
		{
			name:         "all networking infrastructure",
			filterString: "./infrastructure/networking/*",
			description:  "Select all networking-related units",
			expected:     []string{"vpc", "subnets", "security-groups"},
		},
		{
			name:         "apps excluding test-app",
			filterString: "./apps/* | !test-app",
			description:  "Select all apps except test-app",
			expected:     []string{"frontend", "backend", "api"},
		},
		{
			name:         "compute infrastructure excluding db-server",
			filterString: "./infrastructure/compute/* | !db-server",
			description:  "Select compute infrastructure except db-server",
			expected:     []string{"app-server"},
		},
		{
			name:         "everything in infrastructure",
			filterString: "./infrastructure/**",
			description:  "Select all infrastructure units recursively",
			expected:     []string{"vpc", "subnets", "security-groups", "app-server", "db-server"},
		},
		{
			name:         "exclude specific unit",
			filterString: "!test-app",
			description:  "Exclude test-app from all units",
			expected:     []string{"vpc", "subnets", "security-groups", "app-server", "db-server", "frontend", "backend", "api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := log.New()
			result, err := filter.Apply(l, tt.filterString, repoComponents)
			require.NoError(t, err)

			var resultNames []string
			for _, c := range result {
				resultNames = append(resultNames, filepath.Base(c.Path()))
			}

			assert.ElementsMatch(t, tt.expected, resultNames, tt.description)
		})
	}
}

func TestFilter_EdgeCasesAndErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("filter with no matches", func(t *testing.T) {
		t.Parallel()

		l := log.New()
		result, err := filter.Apply(l, "nonexistent", testComponents)
		require.NoError(t, err)

		assert.Empty(t, result)
	})

	t.Run("multiple parse and evaluate calls", func(t *testing.T) {
		t.Parallel()

		filter, err := filter.Parse("app1")
		require.NoError(t, err)

		l := log.New()

		result1, err := filter.Evaluate(l, testComponents)
		require.NoError(t, err)

		result2, err := filter.Evaluate(l, testComponents)
		require.NoError(t, err)

		assert.Equal(t, result1, result2)
	})

	t.Run("whitespace handling", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			filterString string
		}{
			{"./apps/* |   !legacy"},
			{"  ./apps/*  |  !legacy  "},
			{"./apps/* | !legacy"},
		}

		expected := component.Components{
			component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: ".",
			}),
			component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: ".",
			}),
		}

		for _, tt := range tests {
			l := log.New()
			result, err := filter.Apply(l, tt.filterString, testComponents)
			require.NoError(t, err)

			assert.ElementsMatch(t, expected, result)
		}
	})
}
