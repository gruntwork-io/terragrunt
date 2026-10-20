package runner_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
)

func TestUnitPathLabels(t *testing.T) {
	t.Parallel()

	wd := venvtest.Root("/work/live")

	path := func(elems ...string) string {
		return filepath.Join(append([]string{wd}, elems...)...)
	}

	testCases := []struct {
		expected map[string]string
		name     string
		paths    []string
	}{
		{
			name: "unique last elements",
			paths: []string{
				path(".terragrunt-stack", "openebs", ".terragrunt-stack", "mayastor-diskpools"),
				path("vpc"),
			},
			expected: map[string]string{
				path(".terragrunt-stack", "openebs", ".terragrunt-stack", "mayastor-diskpools"): "mayastor-diskpools",
				path("vpc"): "vpc",
			},
		},
		{
			name: "colliding last elements get parents without stack dirs",
			paths: []string{
				path(".terragrunt-stack", "openebs", ".terragrunt-stack", "mayastor-diskpools"),
				path(".terragrunt-stack", "app-a", ".terragrunt-stack", "db"),
				path(".terragrunt-stack", "app-b", ".terragrunt-stack", "db"),
			},
			expected: map[string]string{
				path(".terragrunt-stack", "openebs", ".terragrunt-stack", "mayastor-diskpools"): "mayastor-diskpools",
				path(".terragrunt-stack", "app-a", ".terragrunt-stack", "db"):                   filepath.Join("app-a", "db"),
				path(".terragrunt-stack", "app-b", ".terragrunt-stack", "db"):                   filepath.Join("app-b", "db"),
			},
		},
		{
			name: "only as many parents as needed",
			paths: []string{
				path("prod", "eu", "app", "db"),
				path("prod", "us", "app", "db"),
				path("prod", "us", "cache"),
			},
			expected: map[string]string{
				path("prod", "eu", "app", "db"): filepath.Join("eu", "app", "db"),
				path("prod", "us", "app", "db"): filepath.Join("us", "app", "db"),
				path("prod", "us", "cache"):     "cache",
			},
		},
		{
			name:  "tail end of another path falls back to the relative path",
			paths: []string{path("db"), path("app", "db")},
			expected: map[string]string{
				path("db"):        "db",
				path("app", "db"): filepath.Join("app", "db"),
			},
		},
		{
			name:  "paths differing only by stack dirs fall back to the relative path",
			paths: []string{path("app", "db"), path("app", ".terragrunt-stack", "db")},
			expected: map[string]string{
				path("app", "db"):                      filepath.Join("app", "db"),
				path("app", ".terragrunt-stack", "db"): filepath.Join("app", ".terragrunt-stack", "db"),
			},
		},
		{
			name:  "working dir and paths outside of it get no label",
			paths: []string{wd, filepath.Join(filepath.Dir(wd), "other", "db"), path("app", "db")},
			expected: map[string]string{
				path("app", "db"): "db",
			},
		},
		{
			name:  "duplicate paths count once",
			paths: []string{path("app", "db"), path("app", "db") + string(filepath.Separator)},
			expected: map[string]string{
				path("app", "db"): "db",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, runner.UnitPathLabels(wd, tc.paths))
		})
	}
}
