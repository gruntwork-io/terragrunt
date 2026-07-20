package getter_test

import (
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitCloneURL pins the URL normalization CASGetter applies before
// handing the URL to git. The github-shorthand and SCP rows are the ones
// that previously regressed: when the v2 GitDetector reattaches "git::" to
// its detection result, dropping it before invoking git is what prevents
// "git: 'remote-git' is not a git command" from the underlying clone.
func TestGitCloneURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "github shorthand result strips git:: and keeps https",
			in:   "git::https://github.com/gruntwork-io/terragrunt.git",
			want: "https://github.com/gruntwork-io/terragrunt.git",
		},
		{
			name: "scp result strips git:: and converts ssh:// to scp form",
			in:   "git::ssh://git@github.com/gruntwork-io/terragrunt.git",
			want: "git@github.com:gruntwork-io/terragrunt.git",
		},
		{
			name: "bare ssh url converts to scp form",
			in:   "ssh://git@github.com/gruntwork-io/terragrunt.git",
			want: "git@github.com:gruntwork-io/terragrunt.git",
		},
		{
			name: "https url passes through unchanged",
			in:   "https://github.com/foo/bar.git",
			want: "https://github.com/foo/bar.git",
		},
		{
			name: "ssh url with custom port keeps url form",
			in:   "ssh://git@github.com:2222/gruntwork-io/terragrunt.git",
			want: "ssh://git@github.com:2222/gruntwork-io/terragrunt.git",
		},
		{
			name: "git:: ssh url with custom port keeps url form",
			in:   "git::ssh://git@github.com:2222/gruntwork-io/terragrunt.git",
			want: "ssh://git@github.com:2222/gruntwork-io/terragrunt.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, getter.GitCloneURL(tt.in))
		})
	}
}

// TestGitURLParams pins the go-getter query-parameter parsing CAS applies
// before invoking git. depth and ref must be lifted out of the URL: depth
// is a shallow-clone hint, not a native git URL parameter, so leaving it in
// makes git treat "?depth=1" as part of the repository name and reject the
// clone (regression #6512). ref selects the revision to check out.
func TestGitURLParams(t *testing.T) {
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

			gotURL, gotRef, gotDepth := getter.GitURLParams(u)
			assert.Equal(t, tt.wantURL, gotURL)
			assert.Equal(t, tt.wantRef, gotRef)
			assert.Equal(t, tt.wantDepth, gotDepth)
		})
	}
}
