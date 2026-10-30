package options_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
)

func TestPathLabels(t *testing.T) {
	t.Parallel()

	base := venvtest.Root("/work/live")
	outside := venvtest.Root("/other")

	stacked := func(elems ...string) string {
		path := base
		for _, elem := range elems {
			path = filepath.Join(path, ".terragrunt-stack", elem)
		}

		return path
	}

	tests := []struct {
		expected map[string]string
		name     string
		paths    []string
	}{
		{
			name:  "unique_names_keep_the_last_element",
			paths: []string{stacked("openebs", "mayastor-diskpools"), filepath.Join(base, "vpc")},
			expected: map[string]string{
				stacked("openebs", "mayastor-diskpools"): "mayastor-diskpools",
				filepath.Join(base, "vpc"):               "vpc",
			},
		},
		{
			name:  "same_names_add_parents_with_stack_dirs",
			paths: []string{stacked("app-a", "db"), stacked("app-b", "db"), stacked("app-a", "vpc")},
			expected: map[string]string{
				stacked("app-a", "db"):  filepath.Join("app-a", ".terragrunt-stack", "db"),
				stacked("app-b", "db"):  filepath.Join("app-b", ".terragrunt-stack", "db"),
				stacked("app-a", "vpc"): "vpc",
			},
		},
		{
			name:  "parents_are_added_until_unique",
			paths: []string{filepath.Join(base, "eu", "app", "db"), filepath.Join(base, "us", "app", "db")},
			expected: map[string]string{
				filepath.Join(base, "eu", "app", "db"): filepath.Join("eu", "app", "db"),
				filepath.Join(base, "us", "app", "db"): filepath.Join("us", "app", "db"),
			},
		},
		{
			name:  "name_ending_another_falls_back_to_full_path",
			paths: []string{filepath.Join(base, "db"), filepath.Join(base, "app", "db")},
			expected: map[string]string{
				filepath.Join(base, "db"):        "db",
				filepath.Join(base, "app", "db"): filepath.Join("app", "db"),
			},
		},
		{
			name:  "stack_dir_tells_units_apart",
			paths: []string{stacked("app", "db"), filepath.Join(base, "app", "db")},
			expected: map[string]string{
				stacked("app", "db"):             filepath.Join(".terragrunt-stack", "db"),
				filepath.Join(base, "app", "db"): filepath.Join("app", "db"),
			},
		},
		{
			name:  "relative_paths_are_resolved_against_base_dir",
			paths: []string{filepath.Join("app-a", "db"), filepath.Join(base, "app-b", "db")},
			expected: map[string]string{
				filepath.Join(base, "app-a", "db"): filepath.Join("app-a", "db"),
				filepath.Join(base, "app-b", "db"): filepath.Join("app-b", "db"),
			},
		},
		{
			name:  "paths_outside_base_dir_use_their_own_parents",
			paths: []string{filepath.Join(outside, "db"), filepath.Join(base, "db")},
			expected: map[string]string{
				filepath.Join(outside, "db"): filepath.Join(filepath.Base(outside), "db"),
				filepath.Join(base, "db"):    "db",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, options.PathLabels(base, tc.paths))
		})
	}
}
