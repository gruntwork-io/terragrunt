package run

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModulesNeedInit(t *testing.T) {
	t.Parallel()

	const (
		cacheDir    = "/cache"
		moduleBlock = "module \"example\" {\n  source = \"./example\"\n}\n"
	)

	testCases := []struct {
		files    map[string]string
		name     string
		dirs     []string
		expected bool
	}{
		{
			name:     "module block in a root .tf file",
			files:    map[string]string{"main.tf": moduleBlock},
			expected: true,
		},
		{
			name:     "module block in a root .tofu file",
			files:    map[string]string{"main.tofu": moduleBlock},
			expected: true,
		},
		{
			// Refs: #7220.
			name: "module block only in a subdirectory",
			files: map[string]string{
				"main.tf":                `resource "null_resource" "this" {}`,
				"examples/basic/main.tf": moduleBlock,
			},
			expected: false,
		},
		{
			name:     "modules already installed",
			files:    map[string]string{"main.tf": moduleBlock},
			dirs:     []string{".terraform/modules"},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			require.NoError(t, fsys.MkdirAll(cacheDir, 0755))

			for _, dir := range tc.dirs {
				require.NoError(t, fsys.MkdirAll(filepath.Join(cacheDir, dir), 0755))
			}

			for name, contents := range tc.files {
				path := filepath.Join(cacheDir, name)
				require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0755))
				require.NoError(t, vfs.WriteFile(fsys, path, []byte(contents), 0644))
			}

			needsInit, err := modulesNeedInit(fsys, &Options{CacheDir: cacheDir}, map[string]string{})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, needsInit)
		})
	}
}
