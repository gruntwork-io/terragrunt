package tf_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleOutputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files    map[string]string
		name     string
		expected []string
	}{
		{
			name:     "no files",
			files:    map[string]string{},
			expected: []string{},
		},
		{
			name: "outputs across hcl and json files are sorted and deduplicated",
			files: map[string]string{
				"outputs.tf": `
output "vpc_id" {
  value = "vpc"
}

output "subnet_ids" {
  value = []
}
`,
				"more.tf.json": `{"output": {"vpc_id": {"value": "again"}, "arn": {"value": "arn"}}}`,
				"main.tf":      `variable "region" {}`,
			},
			expected: []string{"arn", "subnet_ids", "vpc_id"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			moduleDir := "/module"

			require.NoError(t, fsys.MkdirAll(moduleDir, 0o755))

			for name, content := range tc.files {
				require.NoError(t, vfs.WriteFile(fsys, filepath.Join(moduleDir, name), []byte(content), 0o644))
			}

			outputs, err := tf.ModuleOutputs(fsys, moduleDir)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, outputs)
		})
	}
}

func TestModuleOutputsUnparseableModule(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	moduleDir := "/module"

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(moduleDir, "main.tf"), []byte(`output "broken" {`), 0o644))

	_, err := tf.ModuleOutputs(fsys, moduleDir)
	require.Error(t, err)
}
