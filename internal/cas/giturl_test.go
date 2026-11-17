package cas_test

import (
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStripGitURLParams pins the go-getter query-parameter handling shared by
// the CAS git getter and stack source cloning: ref and sshkey are lifted out and
// returned, depth is lifted out and discarded, and the caller's URL is left
// untouched.
func TestStripGitURLParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		in         string
		wantURL    string
		wantRef    string
		wantSSHKey string
	}{
		{
			name:    "depth and ref are both stripped",
			in:      "https://github.com/foo/bar.git?depth=1&ref=v5.21.0",
			wantURL: "https://github.com/foo/bar.git",
			wantRef: "v5.21.0",
		},
		{
			name:    "depth alone is stripped",
			in:      "https://github.com/foo/bar.git?depth=3",
			wantURL: "https://github.com/foo/bar.git",
			wantRef: "",
		},
		{
			name:    "ref alone is stripped",
			in:      "https://github.com/foo/bar.git?ref=main",
			wantURL: "https://github.com/foo/bar.git",
			wantRef: "main",
		},
		{
			name:    "no query is unchanged",
			in:      "https://github.com/foo/bar.git",
			wantURL: "https://github.com/foo/bar.git",
			wantRef: "",
		},
		{
			name:    "non-numeric depth is stripped too",
			in:      "https://github.com/foo/bar.git?depth=abc&ref=v1",
			wantURL: "https://github.com/foo/bar.git",
			wantRef: "v1",
		},
		{
			name:    "unrelated query parameters survive",
			in:      "https://github.com/foo/bar.git?depth=1&ref=v1&archive=false",
			wantURL: "https://github.com/foo/bar.git?archive=false",
			wantRef: "v1",
		},
		{
			name:       "sshkey alone is stripped",
			in:         "ssh://git@github.com/foo/bar.git?sshkey=fakekey0",
			wantURL:    "ssh://git@github.com/foo/bar.git",
			wantRef:    "",
			wantSSHKey: "fakekey0",
		},
		{
			name:       "sshkey is stripped with ref and depth",
			in:         "ssh://git@github.com/foo/bar.git?depth=1&ref=v1&sshkey=fakekey0",
			wantURL:    "ssh://git@github.com/foo/bar.git",
			wantRef:    "v1",
			wantSSHKey: "fakekey0",
		},
		{
			name:       "sshkey is stripped beside unrelated query parameters",
			in:         "ssh://git@github.com/foo/bar.git?archive=false&sshkey=fakekey0",
			wantURL:    "ssh://git@github.com/foo/bar.git?archive=false",
			wantRef:    "",
			wantSSHKey: "fakekey0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(tt.in)
			require.NoError(t, err)

			stripped, params := cas.StripGitURLParams(u)

			assert.Equal(t, tt.wantRef, params.Ref)
			assert.Equal(t, tt.wantSSHKey, params.SSHKey)
			assert.Equal(t, tt.wantURL, stripped.String())
			assert.Equal(t, tt.in, u.String(), "the input URL must not be mutated")
		})
	}
}
