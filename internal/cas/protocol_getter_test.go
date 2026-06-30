package cas_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCASRef(t *testing.T) {
	t.Parallel()

	const (
		sha1Hash   = "f39ea0ebf891c9954c89d07b73b487ff938ef08b"
		sha256Hash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	)

	tests := []struct {
		wantErr error
		name    string
		ref     string
		want    string
	}{
		{
			name: "valid sha1 ref",
			ref:  "sha1:" + sha1Hash,
			want: sha1Hash,
		},
		{
			name:    "missing sha1 prefix",
			ref:     sha1Hash,
			wantErr: cas.ErrCASRefMissingPrefix,
		},
		{
			name:    "empty hash",
			ref:     "sha1:",
			wantErr: cas.ErrCASRefEmptyHash,
		},
		{
			name: "valid sha256 ref",
			ref:  "sha256:" + sha256Hash,
			want: sha256Hash,
		},
		{
			name:    "unknown algorithm",
			ref:     "md5:abc123",
			wantErr: cas.ErrCASRefMissingPrefix,
		},
		{
			name:    "single character sha1 hash",
			ref:     "sha1:a",
			wantErr: cas.ErrCASRefInvalidHash,
		},
		{
			name:    "sha1 hash too short",
			ref:     "sha1:abc123def456",
			wantErr: cas.ErrCASRefInvalidHash,
		},
		{
			name:    "sha1 hash with non-hex characters",
			ref:     "sha1:zz9ea0ebf891c9954c89d07b73b487ff938ef08b",
			wantErr: cas.ErrCASRefInvalidHash,
		},
		{
			name:    "sha1 hash with uppercase hex",
			ref:     "sha1:F39EA0EBF891C9954C89D07B73B487FF938EF08B",
			wantErr: cas.ErrCASRefInvalidHash,
		},
		{
			name:    "sha256 hash too short",
			ref:     "sha256:" + sha1Hash,
			wantErr: cas.ErrCASRefInvalidHash,
		},
		{
			name:    "sha1 hash with subdir tail",
			ref:     "sha1:" + sha1Hash + "//modules/vpc",
			wantErr: cas.ErrCASRefInvalidHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := cas.ParseCASRef(tt.ref)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				var wrapped *cas.WrappedError

				require.ErrorAs(t, err, &wrapped)
				assert.Equal(t, tt.ref, wrapped.Context)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCASProtocolGetterGet_MalformedHash is a regression test: a malformed
// hash in a cas:: source used to reach the store's partitioning logic and
// panic with a slice-bounds error. It must instead fail cleanly with
// [cas.ErrCASRefInvalidHash].
func TestCASProtocolGetterGet_MalformedHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "single character hash",
			src:  "cas::sha1:a",
		},
		{
			name: "non-hex hash",
			src:  "cas::sha1:zz9ea0ebf891c9954c89d07b73b487ff938ef08b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, err := cas.New(cas.WithStorePath(t.TempDir()))
			require.NoError(t, err)

			v, err := cas.OSVenv()
			require.NoError(t, err)

			l := logger.CreateLogger()
			g := getter.NewCASProtocolGetter(l, c, v)

			req := &getter.Request{
				Src: tt.src,
				Dst: t.TempDir(),
			}

			err = g.Get(t.Context(), req)
			require.ErrorIs(t, err, cas.ErrCASRefInvalidHash)
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
