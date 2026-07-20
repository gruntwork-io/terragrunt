package cas_test

import (
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStripGitURLParams pins the go-getter query-parameter parsing shared by
// the CAS git getter and stack source cloning. depth and ref must be lifted
// out of the URL: depth is a shallow-clone hint, not a native git URL
// parameter, so leaving it in makes git treat "?depth=1" as part of the
// repository name and reject the clone (regression #6512).
func TestStripGitURLParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		wantURL   string
		wantRef   string
		wantDepth int
	}{
		{
			name:      "depth and ref are both stripped",
			in:        "https://github.com/foo/bar.git?depth=1&ref=v5.21.0",
			wantURL:   "https://github.com/foo/bar.git",
			wantRef:   "v5.21.0",
			wantDepth: 1,
		},
		{
			name:      "depth alone is stripped and parsed",
			in:        "https://github.com/foo/bar.git?depth=3",
			wantURL:   "https://github.com/foo/bar.git",
			wantRef:   "",
			wantDepth: 3,
		},
		{
			name:      "ref alone leaves depth at zero",
			in:        "https://github.com/foo/bar.git?ref=main",
			wantURL:   "https://github.com/foo/bar.git",
			wantRef:   "main",
			wantDepth: 0,
		},
		{
			name:      "no query is unchanged",
			in:        "https://github.com/foo/bar.git",
			wantURL:   "https://github.com/foo/bar.git",
			wantRef:   "",
			wantDepth: 0,
		},
		{
			name:      "non-positive depth is ignored but still stripped",
			in:        "https://github.com/foo/bar.git?depth=abc&ref=v1",
			wantURL:   "https://github.com/foo/bar.git",
			wantRef:   "v1",
			wantDepth: 0,
		},
		{
			name:      "unrelated query parameters survive",
			in:        "https://github.com/foo/bar.git?depth=1&ref=v1&archive=false",
			wantURL:   "https://github.com/foo/bar.git?archive=false",
			wantRef:   "v1",
			wantDepth: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(tt.in)
			require.NoError(t, err)

			gotRef, gotDepth := cas.StripGitURLParams(u)
			assert.Equal(t, tt.wantRef, gotRef)
			assert.Equal(t, tt.wantDepth, gotDepth)
			// u is mutated in place: the returned URL must no longer carry
			// the go-getter parameters that break native git.
			assert.Equal(t, tt.wantURL, u.String())
		})
	}
}
