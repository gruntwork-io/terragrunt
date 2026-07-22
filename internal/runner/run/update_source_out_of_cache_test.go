package run_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestRewriteRelativePathsOutOfCache(t *testing.T) {
	t.Parallel()

	// The unit dir sits at /unit; workingDir mirrors the real cache layout,
	// three directory levels (.terragrunt-cache/<hash>/<hash>) below it. An
	// escaping path therefore gains a "../../../" prefix, so "../modules/foo"
	// becomes "../../../../modules/foo".
	const (
		unitDir    = "/unit"
		workingDir = "/unit/.terragrunt-cache/hash1/hash2"
	)

	testCases := []struct {
		name     string
		relPath  string
		input    string
		expected string
	}{
		{
			name:    "module source escaping the unit is rewritten",
			relPath: "main.tf",
			input: `module "foo" {
  source = "../modules/foo"
}
`,
			expected: `module "foo" {
  source = "../../../../modules/foo"
}
`,
		},
		{
			name:    "in-unit reference is left untouched",
			relPath: "main.tf",
			input: `module "foo" {
  source = "./modules/foo"
}
`,
			expected: `module "foo" {
  source = "./modules/foo"
}
`,
		},
		{
			name:    "remote source is left untouched",
			relPath: "main.tf",
			input: `module "foo" {
  source = "git::https://example.com/foo.git"
}
`,
			expected: `module "foo" {
  source = "git::https://example.com/foo.git"
}
`,
		},
		{
			name:    "path passed to a function is rewritten",
			relPath: "main.tf",
			input: `locals {
  x = file("../templates/x.tpl")
}
`,
			expected: `locals {
  x = file("../../../../templates/x.tpl")
}
`,
		},
		{
			name:    "interpolated string is left untouched",
			relPath: "main.tf",
			input: `locals {
  x = "${path.module}/../x"
}
`,
			expected: `locals {
  x = "${path.module}/../x"
}
`,
		},
		{
			name:    "tofu extension is rewritten",
			relPath: "main.tofu",
			input: `module "foo" {
  source = "../foo"
}
`,
			expected: `module "foo" {
  source = "../../../../foo"
}
`,
		},
		{
			// A "../" from a subdir that resolves back inside the unit must not
			// be rewritten; one that climbs above the unit root must be.
			name:    "subdir reference is judged by where it resolves",
			relPath: filepath.Join("sub", "main.tf"),
			input: `module "a" {
  source = "../sibling"
}
module "b" {
  source = "../../escape"
}
`,
			expected: `module "a" {
  source = "../sibling"
}
module "b" {
  source = "../../../../../escape"
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			file := filepath.Join(workingDir, tc.relPath)
			require.NoError(t, vfs.WriteFile(fsys, file, []byte(tc.input), 0o644))

			require.NoError(
				t,
				run.RewriteRelativePathsOutOfCache(
					logger.CreateLogger(),
					fsys,
					unitDir,
					workingDir,
				),
			)

			got, err := vfs.ReadFile(fsys, file)
			require.NoError(t, err)
			require.Equal(t, tc.expected, string(got))
		})
	}
}

func TestRewriteRelativePathsOutOfCacheSkipsJSON(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	file := "/unit/.terragrunt-cache/hash1/hash2/main.tf.json"
	content := `{"module":{"foo":{"source":"../modules/foo"}}}`
	require.NoError(t, vfs.WriteFile(fsys, file, []byte(content), 0o644))

	require.NoError(
		t,
		run.RewriteRelativePathsOutOfCache(
			logger.CreateLogger(),
			fsys,
			"/unit",
			"/unit/.terragrunt-cache/hash1/hash2",
		),
	)

	got, err := vfs.ReadFile(fsys, file)
	require.NoError(t, err)
	require.Equal(t, content, string(got))
}

func TestRewriteRelativePathsOutOfCacheLeavesInvalidFileUnchanged(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	file := "/unit/.terragrunt-cache/hash1/hash2/main.tf"
	content := `module "foo" {
  source =
}
`
	require.NoError(t, vfs.WriteFile(fsys, file, []byte(content), 0o644))

	require.NoError(
		t,
		run.RewriteRelativePathsOutOfCache(
			logger.CreateLogger(),
			fsys,
			"/unit",
			"/unit/.terragrunt-cache/hash1/hash2",
		),
	)

	got, err := vfs.ReadFile(fsys, file)
	require.NoError(t, err)
	require.Equal(t, content, string(got))
}
