package getter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/stretchr/testify/assert"
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
