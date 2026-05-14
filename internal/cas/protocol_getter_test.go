package cas_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCASRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ref     string
		want    string
		wantErr bool
	}{
		{
			name: "valid sha1 ref",
			ref:  "sha1:abc123def456",
			want: "abc123def456",
		},
		{
			name:    "missing sha1 prefix",
			ref:     "abc123def456",
			wantErr: true,
		},
		{
			name:    "empty hash",
			ref:     "sha1:",
			wantErr: true,
		},
		{
			name: "valid sha256 ref",
			ref:  "sha256:abc123def456",
			want: "abc123def456",
		},
		{
			name:    "unknown algorithm",
			ref:     "md5:abc123",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := cas.ParseCASRef(tt.ref)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatCASRef(t *testing.T) {
	t.Parallel()

	// Short hash → detected as SHA-1
	assert.Equal(t, "cas::sha1:abc123", cas.FormatCASRef("abc123"))

	// 40-char hash → SHA-1
	sha1Hash := "f39ea0ebf891c9954c89d07b73b487ff938ef08b"
	assert.Equal(t, "cas::sha1:"+sha1Hash, cas.FormatCASRef(sha1Hash))

	// 64-char hash → SHA-256
	sha256Hash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	assert.Equal(t, "cas::sha256:"+sha256Hash, cas.FormatCASRef(sha256Hash))
}

func TestFormatCASRefWithSubdir(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "cas::sha1:abc123//modules/vpc", cas.FormatCASRefWithSubdir("abc123", "modules/vpc"))
}

func TestDetectHashAlgorithm(t *testing.T) {
	t.Parallel()

	assert.Equal(t, cas.HashSHA1, cas.DetectHashAlgorithm("abc123"))
	assert.Equal(t, cas.HashSHA1, cas.DetectHashAlgorithm("f39ea0ebf891c9954c89d07b73b487ff938ef08b"))

	sha256Hash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	assert.Equal(t, cas.HashSHA256, cas.DetectHashAlgorithm(sha256Hash))
}

func TestCASProtocolGetterDetect(t *testing.T) {
	t.Parallel()

	g := &getter.CASProtocolGetter{}

	tests := []struct {
		name     string
		src      string
		forced   string
		detected bool
	}{
		{
			name:     "cas protocol",
			src:      "cas::sha1:abc123",
			detected: true,
		},
		{
			name:     "cas protocol with subdir",
			src:      "cas::sha1:abc123//modules/vpc",
			detected: true,
		},
		{
			name:     "go-getter strips cas:: before Detect",
			src:      "sha1:f39ea0ebf891c9954c89d07b73b487ff938ef08b",
			forced:   "cas",
			detected: true,
		},
		{
			name:     "forced cas with invalid ref",
			src:      "bogus:xyz",
			forced:   "cas",
			detected: false,
		},
		{
			name:     "git URL",
			src:      "git::https://github.com/foo/bar.git",
			detected: false,
		},
		{
			name:     "local path",
			src:      "../modules/vpc",
			detected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &getter.Request{Src: tt.src, Forced: tt.forced}
			detected, err := g.Detect(req)
			require.NoError(t, err)
			assert.Equal(t, tt.detected, detected)

			if detected {
				assert.Equal(t, "cas", req.Forced)
			}
		})
	}
}
