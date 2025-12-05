package module

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerraformSourcePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		cloneURL  string
		moduleDir string
		expected  string
	}{
		{
			name:      "root module without ref",
			cloneURL:  "git::https://github.com/org/repo.git",
			moduleDir: "",
			expected:  "git::https://github.com/org/repo.git",
		},
		{
			name:      "root module with ref",
			cloneURL:  "git::https://github.com/org/repo.git?ref=v1.0.0",
			moduleDir: "",
			expected:  "git::https://github.com/org/repo.git?ref=v1.0.0",
		},
		{
			name:      "submodule without ref",
			cloneURL:  "git::https://github.com/org/repo.git",
			moduleDir: "modules/foo",
			expected:  "git::https://github.com/org/repo.git//modules/foo",
		},
		{
			name:      "submodule with ref",
			cloneURL:  "git::https://github.com/org/repo.git?ref=v1.0.0",
			moduleDir: "modules/foo",
			expected:  "git::https://github.com/org/repo.git//modules/foo?ref=v1.0.0",
		},
		{
			name:      "ssh url with ref",
			cloneURL:  "git::ssh://git@github.com/org/repo.git?ref=v1.0.0",
			moduleDir: "modules/bar",
			expected:  "git::ssh://git@github.com/org/repo.git//modules/bar?ref=v1.0.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &Module{
				cloneURL:  tc.cloneURL,
				moduleDir: tc.moduleDir,
			}
			assert.Equal(t, tc.expected, m.TerraformSourcePath())
		})
	}
}
