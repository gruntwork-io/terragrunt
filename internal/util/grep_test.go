package util_test

import (
	"regexp"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrepFilesWithSuffix(t *testing.T) {
	t.Parallel()

	backendRegex := regexp.MustCompile(`(?m)"backend":[[:space:]]*{[[:space:]]*"s3"`)

	tests := []struct {
		files  map[string]string
		regex  *regexp.Regexp
		name   string
		root   string
		suffix string
		want   bool
	}{
		{
			name: "matches file at root",
			files: map[string]string{
				"/mod/backend.tf.json": `{"terraform": {"backend": {"s3": {}}}}`,
			},
			root:   "/mod",
			suffix: ".tf.json",
			regex:  backendRegex,
			want:   true,
		},
		{
			name: "matches nested file",
			files: map[string]string{
				"/mod/readme.md":                "hello",
				"/mod/sub/deep/backend.tf.json": `{"terraform": {"backend": {"s3": {}}}}`,
			},
			root:   "/mod",
			suffix: ".tf.json",
			regex:  backendRegex,
			want:   true,
		},
		{
			name: "no match when suffix differs",
			files: map[string]string{
				// .tf, not .tf.json, must be skipped.
				"/mod/backend.tf": `terraform { backend "s3" {} }`,
			},
			root:   "/mod",
			suffix: ".tf.json",
			regex:  backendRegex,
			want:   false,
		},
		{
			name: "no match when regex does not hit",
			files: map[string]string{
				"/mod/main.tf.json":   `{"resource": {}}`,
				"/mod/output.tf.json": `{"output": {}}`,
			},
			root:   "/mod",
			suffix: ".tf.json",
			regex:  backendRegex,
			want:   false,
		},
		{
			name: "ignores files outside root",
			files: map[string]string{
				"/other/backend.tf.json": `{"terraform": {"backend": {"s3": {}}}}`,
				"/mod/main.tf.json":      `{}`,
			},
			root:   "/mod",
			suffix: ".tf.json",
			regex:  backendRegex,
			want:   false,
		},
		{
			name:   "missing root returns false without error",
			files:  map[string]string{},
			root:   "/does-not-exist",
			suffix: ".tf.json",
			regex:  backendRegex,
			want:   false,
		},
		{
			name: "empty suffix matches any file",
			files: map[string]string{
				"/mod/notes": "has backend s3 annotation",
			},
			root:   "/mod",
			suffix: "",
			regex:  regexp.MustCompile(`backend s3`),
			want:   true,
		},
		{
			name: "compound suffix is honored",
			files: map[string]string{
				// .json alone should not be enough when suffix is .tf.json.
				"/mod/config.json": `{"terraform": {"backend": {"s3": {}}}}`,
			},
			root:   "/mod",
			suffix: ".tf.json",
			regex:  backendRegex,
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			for path, contents := range tc.files {
				require.NoError(t, vfs.WriteFile(fsys, path, []byte(contents), 0o644))
			}

			got, err := util.GrepFilesWithSuffix(fsys, tc.regex, tc.root, tc.suffix)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGrepFilesWithSuffix_SurvivesUnmatchedSiblings(t *testing.T) {
	t.Parallel()

	// Exercises the case where a matching file sits alongside many non-matching
	// siblings. Proving that the walk actually short-circuits would require a
	// counting FS wrapper; this test only asserts that the function still
	// returns the match without being derailed by the surrounding files.
	fsys := vfs.NewMemMapFS()

	regex := regexp.MustCompile(`needle`)

	require.NoError(t, vfs.WriteFile(fsys, "/mod/a.tf.json", []byte(`needle`), 0o644))

	for _, name := range []string{"/mod/b.tf.json", "/mod/c.tf.json", "/mod/sub/d.tf.json"} {
		require.NoError(t, vfs.WriteFile(fsys, name, []byte(`no match`), 0o644))
	}

	got, err := util.GrepFilesWithSuffix(fsys, regex, "/mod", ".tf.json")
	require.NoError(t, err)
	assert.True(t, got)
}
