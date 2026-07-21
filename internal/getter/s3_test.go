package getter_test

import (
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultClientCanonicalizesS3SourceURLs pins that `s3::https://`
// sources work in every AWS S3 endpoint form, including the
// virtual-hosted style (`<bucket>.s3.<region>.amazonaws.com`) AWS
// documents as canonical. The upstream go-getter/s3/v2 getter only
// parses path-style hostnames (`s3.amazonaws.com`,
// `s3-<region>.amazonaws.com`), so whichever getter claims the request
// must canonicalize req.Src at Detect time: Client.Get re-parses
// req.Src after detection into the URL the fetch uses.
//
// The test mirrors the outer client's getter-selection loop instead of
// calling Client.Get so no network I/O happens.
func TestDefaultClientCanonicalizesS3SourceURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		wantHost string
		wantPath string
	}{
		{
			name:     "modern virtual-host style",
			src:      "s3::https://my-bucket.s3.us-west-2.amazonaws.com/terraform/modules/myapp.zip",
			wantHost: "s3-us-west-2.amazonaws.com",
			wantPath: "/my-bucket/terraform/modules/myapp.zip",
		},
		{
			name:     "legacy regional virtual-host style",
			src:      "s3::https://my-bucket.s3-us-west-2.amazonaws.com/terraform/modules/myapp.zip",
			wantHost: "s3-us-west-2.amazonaws.com",
			wantPath: "/my-bucket/terraform/modules/myapp.zip",
		},
		{
			name:     "global virtual-host style",
			src:      "s3::https://my-bucket.s3.amazonaws.com/terraform/modules/myapp.zip",
			wantHost: "s3.amazonaws.com",
			wantPath: "/my-bucket/terraform/modules/myapp.zip",
		},
		{
			name:     "modern path-style",
			src:      "s3::https://s3.us-west-2.amazonaws.com/my-bucket/terraform/modules/myapp.zip",
			wantHost: "s3-us-west-2.amazonaws.com",
			wantPath: "/my-bucket/terraform/modules/myapp.zip",
		},
		{
			name:     "legacy regional path-style stays unchanged",
			src:      "s3::https://s3-us-west-2.amazonaws.com/my-bucket/terraform/modules/myapp.zip",
			wantHost: "s3-us-west-2.amazonaws.com",
			wantPath: "/my-bucket/terraform/modules/myapp.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := getter.NewClient()
			req := &gogetter.Request{Src: tt.src, GetMode: getter.ModeAny}

			claimed := false

			for _, g := range client.Getters {
				ok, err := gogetter.Detect(req, g)
				require.NoError(t, err)

				if ok {
					claimed = true
					break
				}
			}

			require.True(t, claimed, "a getter must claim %s", tt.src)
			assert.Equal(t, getter.SchemeS3, req.Forced)

			u, err := url.Parse(req.Src)
			require.NoError(t, err)
			assert.Equal(
				t,
				tt.wantHost,
				u.Host,
				"Detect must canonicalize to a path-style host the bare go-getter/s3 v2 getter accepts",
			)
			assert.Equal(t, tt.wantPath, u.Path)
		})
	}
}
