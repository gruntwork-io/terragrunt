package tf_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasModuleCalls(t *testing.T) {
	t.Parallel()

	const moduleBlock = `
module "example" {
  source = "./example"
}
`

	testCases := []struct {
		files    map[string]string
		name     string
		expected bool
	}{
		{
			name:     "no files",
			files:    map[string]string{},
			expected: false,
		},
		{
			name:     "module block in a .tf file",
			files:    map[string]string{"main.tf": moduleBlock},
			expected: true,
		},
		{
			name:     "module block in a .tofu file",
			files:    map[string]string{"main.tofu": moduleBlock},
			expected: true,
		},
		{
			name:     "module block in a .tf.json file",
			files:    map[string]string{"main.tf.json": `{"module": {"example": {"source": "./example"}}}`},
			expected: true,
		},
		{
			name:     "module block in a .tofu.json file",
			files:    map[string]string{"main.tofu.json": `{"module": {"example": {"source": "./example"}}}`},
			expected: true,
		},
		{
			name: "module block beside other blocks",
			files: map[string]string{
				"versions.tf": `terraform { required_version = ">= 1.0" }`,
				"main.tf":     `resource "null_resource" "this" {}` + moduleBlock,
			},
			expected: true,
		},
		{
			// Regression test for https://github.com/gruntwork-io/terragrunt/issues/7220
			name: "module block only in a subdirectory",
			files: map[string]string{
				"main.tf":                `resource "null_resource" "this" {}`,
				"examples/basic/main.tf": `module "example" { source = "../.." }`,
			},
			expected: false,
		},
		{
			name: "commented-out module blocks",
			files: map[string]string{
				"main.tf": `
# module "hash" {
#   source = "./example"
# }

// module "slashes" {
//   source = "./example"
// }

/*
module "block" {
  source = "./example"
}
*/

resource "null_resource" "this" {}
`,
			},
			expected: false,
		},
		{
			name: "module text inside a string",
			files: map[string]string{
				"main.tf": `
resource "null_resource" "this" {
  triggers = {
    note = "module \"example\" is called from examples/"
  }
}
`,
			},
			expected: false,
		},
		{
			name: "module text inside a heredoc",
			files: map[string]string{
				"main.tf": `
locals {
  readme = <<-EOT
    module "example" {
      source = "../.."
    }
  EOT
}
`,
			},
			expected: false,
		},
		{
			name: "module key nested in a .tf.json resource",
			files: map[string]string{
				"main.tf.json": `{"resource": {"null_resource": {"this": {"triggers": {"module": "example"}}}}}`,
			},
			expected: false,
		},
		{
			name: "files that are not module sources",
			files: map[string]string{
				"terragrunt.hcl":            moduleBlock,
				"terragrunt.tfvars.json":    `{"module": {"example": {}}}`,
				"README.md":                 moduleBlock,
				"main.tf":                   `resource "null_resource" "this" {}`,
				".terragrunt-init-required": "",
			},
			expected: false,
		},
		{
			name:     "unparseable .tf file",
			files:    map[string]string{"main.tf": `module "broken" {`},
			expected: true,
		},
		{
			name:     "unparseable .tf.json file",
			files:    map[string]string{"main.tf.json": `{"module": `},
			expected: true,
		},
		{
			name:     "module block without a label",
			files:    map[string]string{"main.tf": `module { source = "./example" }`},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			modulePath := "/module"

			require.NoError(t, fsys.MkdirAll(modulePath, 0755))

			for filename, content := range tc.files {
				path := filepath.Join(modulePath, filename)
				require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0755))
				require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0644))
			}

			hasModuleCalls, err := tf.HasModuleCalls(fsys, modulePath)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, hasModuleCalls)
		})
	}
}

func TestHasModuleCallsMissingDirectory(t *testing.T) {
	t.Parallel()

	_, err := tf.HasModuleCalls(vfs.NewMemMapFS(), "/missing")
	require.Error(t, err)
}
