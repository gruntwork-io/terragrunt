//nolint:testpackage // needs access to unexported bytesDiff
package format

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytesDiff(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		before        string
		after         string
		path          string
		wantHunkLines []string
		wantHeader    []string
		wantEmpty     bool
	}{
		"identical inputs produce no diff": {
			before:    "a = 1\n",
			after:     "a = 1\n",
			path:      "x.hcl",
			wantEmpty: true,
		},
		"single-line change produces unified diff": {
			before: "a = 1\n",
			after:  "a = 2\n",
			path:   "x.hcl",
			wantHunkLines: []string{
				"@@ -1,1 +1,1 @@",
				"-a = 1",
				"+a = 2",
			},
			wantHeader: []string{
				"--- old/x.hcl",
				"+++ new/x.hcl",
			},
		},
		"path is prefixed with old/ and new/ labels": {
			before: "a\n",
			after:  "b\n",
			path:   "nested/dir/file.hcl",
			wantHeader: []string{
				"--- old/nested/dir/file.hcl",
				"+++ new/nested/dir/file.hcl",
			},
		},
		"missing trailing newline is flagged": {
			before: "a = 1\n",
			after:  "a = 2",
			path:   "x.hcl",
			wantHunkLines: []string{
				"\\ No newline at end of file",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := string(bytesDiff([]byte(tc.before), []byte(tc.after), tc.path))

			if tc.wantEmpty {
				assert.Empty(t, got, "expected no diff output for identical inputs")
				return
			}

			assert.NotEmpty(t, got, "expected non-empty diff output")

			// The diff labels embed tc.path with the OS separator; compare in forward-slash space.
			gotSlash := filepath.ToSlash(got)

			for _, want := range tc.wantHeader {
				assert.Contains(t, gotSlash, want, "diff missing header line")
			}

			for _, want := range tc.wantHunkLines {
				assert.Contains(t, gotSlash, want, "diff missing hunk line")
			}
		})
	}
}
