package getproviders_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/getproviders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageAuthenticationResultSignedBy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		result     getproviders.PackageAuthenticationResult
		hashiCorp  bool
		anyParty   bool
		thirdParty bool
	}{
		{
			name:       "verified-checksum",
			result:     getproviders.VerifiedChecksum,
			hashiCorp:  false,
			anyParty:   false,
			thirdParty: false,
		},
		{
			name:       "official-provider",
			result:     getproviders.OfficialProvider,
			hashiCorp:  true,
			anyParty:   true,
			thirdParty: false,
		},
		{
			name:       "partner-provider",
			result:     getproviders.PartnerProvider,
			hashiCorp:  false,
			anyParty:   true,
			thirdParty: true,
		},
		{
			name:       "community-provider",
			result:     getproviders.CommunityProvider,
			hashiCorp:  false,
			anyParty:   true,
			thirdParty: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.hashiCorp, tc.result.SignedByHashiCorp())
			assert.Equal(t, tc.anyParty, tc.result.SignedByAnyParty())
			assert.Equal(t, tc.thirdParty, tc.result.ThirdPartySigned())
		})
	}
}

// passingChecksumAuth builds a matching-checksum authenticator whose document
// lists the wanted hash for the given filename, so Authenticate succeeds without
// touching the filesystem or any cryptography.
func passingChecksumAuth(
	filename string,
	want [sha256.Size]byte,
) getproviders.PackageAuthentication {
	document := fmt.Appendf(nil, "%x %s\n", want, filename)

	return getproviders.NewMatchingChecksumAuthentication(document, filename, want)
}

func TestPackageAuthenticationAllAuthenticate(t *testing.T) {
	t.Parallel()

	want := [sha256.Size]byte{0xde, 0xca, 0xde}

	t.Run("all-pass", func(t *testing.T) {
		t.Parallel()

		auth := getproviders.PackageAuthenticationAll(
			passingChecksumAuth("first.zip", want),
			passingChecksumAuth("second.zip", want),
		)

		result, err := auth.Authenticate("unused-path")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("first-fails", func(t *testing.T) {
		t.Parallel()

		// The document has no entry for the requested filename, so the first
		// check fails and PackageAuthenticationAll returns early with the error.
		failing := getproviders.NewMatchingChecksumAuthentication(
			fmt.Appendf(nil, "%x other.zip\n", want), "missing.zip", want,
		)

		auth := getproviders.PackageAuthenticationAll(
			failing,
			passingChecksumAuth("second.zip", want),
		)

		_, err := auth.Authenticate("unused-path")
		require.Error(t, err)
	})
}

func TestPackageAuthenticationAllAcceptableHashes(t *testing.T) {
	t.Parallel()

	const nullSHA256 = "53e30545ff8926a8e30ad30648991ca8b93b6fa496272cd23b26763c8ee84515"

	shasums := nullSHA256 + "  terraform-provider-null_3.1.0_linux_amd64.zip\n"

	t.Run("returns-hashes-from-last-capable-check", func(t *testing.T) {
		t.Parallel()

		signatureAuth := getproviders.NewSignatureAuthentication([]byte(shasums), nil, nil)

		auth := getproviders.PackageAuthenticationAll(
			passingChecksumAuth("first.zip", [sha256.Size]byte{0x01}),
			signatureAuth,
		)

		hashes, ok := auth.(getproviders.PackageAuthenticationHashes)
		require.True(t, ok)

		assert.Equal(
			t,
			[]getproviders.Hash{"zh:" + nullSHA256},
			hashes.AcceptableHashes(),
		)
	})

	t.Run("nil-when-no-check-provides-hashes", func(t *testing.T) {
		t.Parallel()

		auth := getproviders.PackageAuthenticationAll(
			passingChecksumAuth("first.zip", [sha256.Size]byte{0x01}),
		)

		hashes, ok := auth.(getproviders.PackageAuthenticationHashes)
		require.True(t, ok)

		assert.Nil(t, hashes.AcceptableHashes())
	})
}
