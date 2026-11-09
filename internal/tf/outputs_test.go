package tf_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleOutputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files    map[string]string
		name     string
		impl     tfimpl.Type
		expected []string
	}{
		{
			name:     "no files",
			files:    map[string]string{},
			impl:     tfimpl.OpenTofu,
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
			impl:     tfimpl.OpenTofu,
			expected: []string{"arn", "subnet_ids", "vpc_id"},
		},
		{
			name: "opentofu leaves out a .tf file beside its .tofu twin",
			files: map[string]string{
				"main.tf":   `output "retired" { value = "retired" }`,
				"main.tofu": `output "current" { value = "current" }`,
				"other.tf":  `output "kept" { value = "kept" }`,
			},
			impl:     tfimpl.OpenTofu,
			expected: []string{"current", "kept"},
		},
		{
			name: "opentofu leaves out a .tf.json file beside its .tofu.json twin",
			files: map[string]string{
				"main.tf.json":   `{"output": {"retired": {"value": "retired"}}}`,
				"main.tofu.json": `{"output": {"current": {"value": "current"}}}`,
			},
			impl:     tfimpl.OpenTofu,
			expected: []string{"current"},
		},
		{
			name: "terraform leaves out every .tofu file",
			files: map[string]string{
				"main.tf":         `output "retired" { value = "retired" }`,
				"main.tofu":       `output "current" { value = "current" }`,
				"extra.tofu.json": `{"output": {"json_only": {"value": "json"}}}`,
			},
			impl:     tfimpl.Terraform,
			expected: []string{"retired"},
		},
		{
			name: "an unknown implementation reads every file",
			files: map[string]string{
				"main.tf":   `output "retired" { value = "retired" }`,
				"main.tofu": `output "current" { value = "current" }`,
			},
			impl:     tfimpl.Unknown,
			expected: []string{"current", "retired"},
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

			outputs, err := tf.ModuleOutputs(fsys, moduleDir, tc.impl)
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

	_, err := tf.ModuleOutputs(fsys, moduleDir, tfimpl.OpenTofu)
	require.Error(t, err)
}
