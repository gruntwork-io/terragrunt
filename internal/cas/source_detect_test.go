package cas_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectRemoteSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "github shorthand",
			src:  "github.com/gruntwork-io/terragrunt-scale-catalog",
			want: "git::https://github.com/gruntwork-io/terragrunt-scale-catalog.git",
		},
		{
			// Callers strip //subdir via getter.SourceDirSubdir before this,
			// so the realistic input keeps the query but not the subdir.
			name: "github shorthand with ref",
			src:  "github.com/gruntwork-io/terragrunt-scale-catalog?ref=v1.11.0",
			want: "git::https://github.com/gruntwork-io/terragrunt-scale-catalog.git?ref=v1.11.0",
		},
		{
			name: "gitlab shorthand",
			src:  "gitlab.com/team/repo",
			want: "git::https://gitlab.com/team/repo.git",
		},
		{
			name: "scp-style ssh url",
			src:  "git@github.com:gruntwork-io/terragrunt.git",
			want: "git::ssh://git@github.com/gruntwork-io/terragrunt.git",
		},
		{
			name: "explicit git:: prefix is preserved",
			src:  "git::https://github.com/gruntwork-io/terragrunt.git?ref=main",
			want: "git::https://github.com/gruntwork-io/terragrunt.git?ref=main",
		},
		{
			// No detector matches plain .git URLs without a known host;
			// callers that want CAS for these must prefix `git::` themselves.
			name: "plain https .git url is returned unchanged",
			src:  "https://example.com/team/repo.git",
			want: "https://example.com/team/repo.git",
		},
		{
			name: "unrecognised plain http url is returned unchanged",
			src:  "http://127.0.0.1:7654/foo",
			want: "http://127.0.0.1:7654/foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := cas.DetectRemoteSource(tt.src)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectRemoteSourceRewritesProduceGitPrefixForKnownHosts(t *testing.T) {
	t.Parallel()

	// Regression guard: any known-host shorthand must come back with `git::`
	// so the central git store sees a URL git can consume.
	knownHostShorthands := []string{
		"github.com/gruntwork-io/terragrunt",
		"gitlab.com/team/repo",
	}

	for _, src := range knownHostShorthands {
		got, err := cas.DetectRemoteSource(src)
		require.NoError(t, err, "src=%s", src)
		require.True(t, strings.HasPrefix(got, "git::"), "src=%s rewritten to %q which lacks git:: prefix", src, got)
	}
}
