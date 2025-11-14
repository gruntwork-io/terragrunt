package filter_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestrictToStacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expr     filter.Expression
		name     string
		expected bool
	}{
		{
			name:     "path filter",
			expr:     filter.NewPathFilter("./apps/*"),
			expected: false,
		},
		{
			name:     "attribute filter restricted to stacks",
			expr:     filter.NewAttributeExpression("type", "stack"),
			expected: true,
		},
		{
			name:     "attribute filter not restricted to stacks",
			expr:     filter.NewAttributeExpression("name", "foo"),
			expected: false,
		},
		{
			name:     "prefix expression restricted to stacks",
			expr:     filter.NewPrefixExpression("!", filter.NewAttributeExpression("type", "unit")),
			expected: true,
		},
		{
			name:     "prefix expression not restricted to stacks",
			expr:     filter.NewPrefixExpression("!", filter.NewAttributeExpression("name", "foo")),
			expected: false,
		},
		{
			name:     "infix expression restricted to stacks",
			expr:     filter.NewInfixExpression(filter.NewAttributeExpression("type", "stack"), "|", filter.NewAttributeExpression("external", "true")),
			expected: true,
		},
		{
			name:     "infix expression also restricted to stacks",
			expr:     filter.NewInfixExpression(filter.NewAttributeExpression("external", "true"), "|", filter.NewAttributeExpression("type", "stack")),
			expected: true,
		},
		{
			name:     "infix expression not restricted to stacks",
			expr:     filter.NewInfixExpression(filter.NewAttributeExpression("name", "foo"), "|", filter.NewAttributeExpression("external", "true")),
			expected: false,
		},
		{
			name:     "graph expression",
			expr:     filter.NewGraphExpression(filter.NewAttributeExpression("name", "foo"), true, false, false),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.expr.IsRestrictedToStacks())
		})
	}
}

func TestGitExpression_Expand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		diffs              *git.Diffs
		name               string
		expectedToPaths    []string
		expectedToReadings []string
		expectedFrom       int
		expectedTo         int
	}{
		{
			name: "removed terragrunt.hcl files create from filters",
			diffs: &git.Diffs{
				Removed: []string{
					"app1/terragrunt.hcl",
					"app2/terragrunt.hcl",
				},
			},
			expectedFrom:       2,
			expectedTo:         0,
			expectedToPaths:    []string{},
			expectedToReadings: []string{},
		},
		{
			name: "added terragrunt.hcl files create to filters",
			diffs: &git.Diffs{
				Added: []string{
					"app1/terragrunt.hcl",
					"app2/terragrunt.hcl",
				},
			},
			expectedFrom:       0,
			expectedTo:         2,
			expectedToPaths:    []string{"app1", "app2"},
			expectedToReadings: []string{},
		},
		{
			name: "changed terragrunt.hcl files create to filters",
			diffs: &git.Diffs{
				Changed: []string{
					"app1/terragrunt.hcl",
					"app2/terragrunt.hcl",
				},
			},
			expectedFrom:       0,
			expectedTo:         2,
			expectedToPaths:    []string{"app1", "app2"},
			expectedToReadings: []string{},
		},
		{
			name: "changed non-terragrunt.hcl files create reading filters",
			diffs: &git.Diffs{
				Changed: []string{
					"app1/main.tf",
					"app1/variables.tf",
					"app2/data.tf",
				},
			},
			expectedFrom:       0,
			expectedTo:         3,
			expectedToPaths:    []string{},
			expectedToReadings: []string{"app1/main.tf", "app1/variables.tf", "app2/data.tf"},
		},
		{
			name: "changed stack files are skipped",
			diffs: &git.Diffs{
				Changed: []string{
					"stack/terragrunt.stack.hcl",
				},
			},
			expectedFrom:       0,
			expectedTo:         0,
			expectedToPaths:    []string{},
			expectedToReadings: []string{},
		},
		{
			name: "mixed file types create appropriate filters",
			diffs: &git.Diffs{
				Removed: []string{
					"app-removed/terragrunt.hcl",
				},
				Added: []string{
					"app-added/terragrunt.hcl",
				},
				Changed: []string{
					"app-modified/terragrunt.hcl",
					"app-modified/main.tf",
					"stack/terragrunt.stack.hcl",
					"other/file.hcl",
				},
			},
			expectedFrom:       1,
			expectedTo:         4,
			expectedToPaths:    []string{"app-added", "app-modified"},
			expectedToReadings: []string{"app-modified/main.tf", "other/file.hcl"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gitExpr := filter.NewGitExpression("main", "HEAD")
			fromFilters, toFilters := gitExpr.Expand(tt.diffs)

			// Verify from filters count
			assert.Len(t, fromFilters, tt.expectedFrom, "From filters count should match")

			// Verify to filters count
			assert.Len(t, toFilters, tt.expectedTo, "To filters count should match")

			// Verify from filters are path filters with correct paths
			for i, f := range fromFilters {
				pathExpr, ok := f.Expression().(*filter.PathExpression)
				require.True(t, ok, "From filter %d should be a PathExpression", i)
				expectedPath := filepath.Dir(tt.diffs.Removed[i])
				assert.Equal(t, expectedPath, pathExpr.Value, "From filter %d should have correct path", i)
			}

			// Verify to filters
			toPaths := []string{}
			toReadings := []string{}

			for _, f := range toFilters {
				switch expr := f.Expression().(type) {
				case *filter.PathExpression:
					toPaths = append(toPaths, expr.Value)
				case *filter.AttributeExpression:
					if expr.Key == "reading" {
						toReadings = append(toReadings, expr.Value)
					}
				}
			}

			// Verify path filters
			assert.ElementsMatch(t, tt.expectedToPaths, toPaths, "To path filters should match")

			// Verify reading filters
			assert.ElementsMatch(t, tt.expectedToReadings, toReadings, "To reading filters should match")
		})
	}
}

func TestGitExpression_Expand_AttributeReadingFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		diffs            *git.Diffs
		expectedReadings []string
	}{
		{
			name: "changed .tf file creates reading filter",
			diffs: &git.Diffs{
				Changed: []string{
					"app/main.tf",
				},
			},
			expectedReadings: []string{"app/main.tf"},
		},
		{
			name: "changed .hcl file (not terragrunt.hcl) creates reading filter",
			diffs: &git.Diffs{
				Changed: []string{
					"app/config.hcl",
				},
			},
			expectedReadings: []string{"app/config.hcl"},
		},
		{
			name: "changed file in subdirectory creates reading filter with correct path",
			diffs: &git.Diffs{
				Changed: []string{
					"app/modules/database/main.tf",
				},
			},
			expectedReadings: []string{"app/modules/database/main.tf"},
		},
		{
			name: "multiple changed files create multiple reading filters",
			diffs: &git.Diffs{
				Changed: []string{
					"app1/main.tf",
					"app1/variables.tf",
					"app2/data.tf",
					"app2/outputs.tf",
				},
			},
			expectedReadings: []string{
				"app1/main.tf",
				"app1/variables.tf",
				"app2/data.tf",
				"app2/outputs.tf",
			},
		},
		{
			name: "mixed terragrunt.hcl and other files",
			diffs: &git.Diffs{
				Changed: []string{
					"app/terragrunt.hcl",
					"app/main.tf",
					"app/variables.tf",
				},
			},
			expectedReadings: []string{
				"app/main.tf",
				"app/variables.tf",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gitExpr := filter.NewGitExpression("main", "HEAD")
			_, toFilters := gitExpr.Expand(tt.diffs)

			// Extract reading filters
			readings := []string{}

			for _, f := range toFilters {
				if attrExpr, ok := f.Expression().(*filter.AttributeExpression); ok {
					if attrExpr.Key == "reading" {
						readings = append(readings, attrExpr.Value)
					}
				}
			}

			// Verify reading filters match expected
			assert.ElementsMatch(t, tt.expectedReadings, readings, "Reading filters should match expected paths")

			// Verify each reading filter is properly constructed
			for _, expectedReading := range tt.expectedReadings {
				found := false

				for _, f := range toFilters {
					if attrExpr, ok := f.Expression().(*filter.AttributeExpression); ok {
						if attrExpr.Key == "reading" && attrExpr.Value == expectedReading {
							found = true

							assert.Equal(t, "reading", attrExpr.Key, "Filter should have reading key")
							assert.Equal(t, expectedReading, attrExpr.Value, "Filter should have correct file path")

							break
						}
					}
				}

				assert.True(t, found, "Expected reading filter for %s should be present", expectedReading)
			}
		})
	}
}
