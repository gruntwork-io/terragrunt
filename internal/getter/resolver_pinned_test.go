package getter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestResolverPinned pins which URLs each resolver reports as naming
// content that cannot change upstream. A pinned source keeps its recorded
// probe for a day; everything else is re-probed unless the caller sets a
// mutable TTL, so a URL wrongly called pinned serves a stale answer.
func TestResolverPinned(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	tests := []struct {
		resolver getter.SourceResolver
		name     string
		url      string
		want     bool
	}{
		{
			name:     "s3 object version",
			resolver: getter.NewS3Resolver(v),
			url:      "https://bucket.s3.us-west-2.amazonaws.com/key.tgz?version=abc123",
			want:     true,
		},
		{
			name:     "s3 without a version",
			resolver: getter.NewS3Resolver(v),
			url:      "https://bucket.s3.us-west-2.amazonaws.com/key.tgz",
			want:     false,
		},
		{
			name:     "oci manifest digest",
			resolver: getter.NewOCIResolver(logger.CreateLogger(), nil),
			url:      "oci://registry.example.com/mod?digest=sha256:" + hexOf64,
			want:     true,
		},
		{
			name:     "oci tag",
			resolver: getter.NewOCIResolver(logger.CreateLogger(), nil),
			url:      "oci://registry.example.com/mod?tag=latest",
			want:     false,
		},
		{
			name:     "tfr exact version",
			resolver: getter.NewTFRResolver(),
			url:      "tfr://registry.opentofu.org/org/mod/aws?version=1.2.3",
			want:     true,
		},
		{
			name:     "tfr version constraint",
			resolver: getter.NewTFRResolver(),
			url:      "tfr://registry.opentofu.org/org/mod/aws?version=%3E%3D1.2.0",
			want:     false,
		},
		{
			name:     "tfr without a version",
			resolver: getter.NewTFRResolver(),
			url:      "tfr://registry.opentofu.org/org/mod/aws",
			want:     false,
		},
		{
			name:     "hg changeset node",
			resolver: getter.NewHgResolver(vexec.NewOSExec()),
			url:      "https://hg.example.com/repo?rev=" + hexOf40,
			want:     true,
		},
		{
			name:     "hg branch name",
			resolver: getter.NewHgResolver(vexec.NewOSExec()),
			url:      "https://hg.example.com/repo?rev=default",
			want:     false,
		},
		{
			name:     "gcs object",
			resolver: getter.NewGCSResolver(v),
			url:      "gs://bucket/object.zip",
			want:     false,
		},
		{
			name:     "http with a checksum",
			resolver: getter.NewHTTPSResolver(),
			url:      "https://example.com/mod.tgz?checksum=sha256:" + hexOf64,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.resolver.Pinned(tt.url))
		})
	}
}

const (
	hexOf40 = "0123456789abcdef0123456789abcdef01234567"
	hexOf64 = hexOf40 + "89abcdef0123456789abcdef"
)
