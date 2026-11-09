package getter_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/redact"
)

func TestChecksumResolver_Scheme(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https", getter.NewChecksumResolver("https", "sha256:"+hexOf64).Scheme())
}

// TestChecksumResolver_Probe: a literal keys the URL without any request; a file reference has no key.
func TestChecksumResolver_Probe(t *testing.T) {
	t.Parallel()

	const url = "https://example.com/mod.tgz?checksum=sha256:" + hexOf64

	testCases := []struct {
		wantErr  error
		name     string
		checksum string
		wantKey  string
	}{
		{
			name:     "typed literal",
			checksum: "sha256:" + hexOf64,
			wantKey:  cas.OpaqueKey("https", url, "checksum:sha256:"+hexOf64),
		},
		{
			name:     "uppercase hex keys like lowercase",
			checksum: "sha256:" + strings.ToUpper(hexOf64),
			wantKey:  cas.OpaqueKey("https", url, "checksum:sha256:"+hexOf64),
		},
		{
			name:     "bare literal",
			checksum: hexOf64,
			wantKey:  cas.OpaqueKey("https", url, "checksum:"+hexOf64),
		},
		{
			name:     "file reference",
			checksum: "file:https://example.com/SHA256SUMS",
			wantErr:  cas.ErrNoVersionMetadata,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			key, err := getter.NewChecksumResolver("https", tc.checksum).Probe(t.Context(), redact.NewURL(url))

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, key)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantKey, key)
		})
	}
}

// TestChecksumResolver_KeysSeparateChecksumsAndURLs: a different pin or a different URL must not share a stored tree.
func TestChecksumResolver_KeysSeparateChecksumsAndURLs(t *testing.T) {
	t.Parallel()

	const (
		url      = "https://example.com/mod.tgz?checksum=sha256:" + hexOf64
		checksum = "sha256:" + hexOf64
	)

	base, err := getter.NewChecksumResolver("https", checksum).Probe(t.Context(), redact.NewURL(url))
	require.NoError(t, err)

	other := "sha256:" + strings.Repeat("0", 64)
	otherKey, err := getter.NewChecksumResolver("https", other).Probe(t.Context(), redact.NewURL(url))
	require.NoError(t, err)
	assert.NotEqual(t, base, otherKey, "a different checksum names different content")

	rawURL := url + "&archive=false"
	rawKey, err := getter.NewChecksumResolver("https", checksum).Probe(t.Context(), redact.NewURL(rawURL))
	require.NoError(t, err)
	assert.NotEqual(t, base, rawKey, "the same bytes left unextracted form a different tree")
}
