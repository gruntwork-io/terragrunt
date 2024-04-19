package provider

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/pkg/errors"

	"github.com/ProtonMail/go-crypto/openpgp"
	openpgpArmor "github.com/ProtonMail/go-crypto/openpgp/armor"
	openpgpErrors "github.com/ProtonMail/go-crypto/openpgp/errors"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

const (
	verifiedChecksum PackageAuthenticationResult = iota
	officialProvider
	partnerProvider
	communityProvider
)

// PackageAuthenticationResult is returned from a PackageAuthentication implementation which implements Stringer.
type PackageAuthenticationResult int

func NewPackageAuthenticationResult(res PackageAuthenticationResult) *PackageAuthenticationResult {
	return &res
}

func (result *PackageAuthenticationResult) String() string {
	if result == nil {
		return "unauthenticated"
	}

	return []string{
		"verified checksum",
		"signed by HashiCorp",
		"signed by a HashiCorp partner",
		"self-signed",
	}[*result]
}

// SignedByHashiCorp returns whether the package was authenticated as signed by HashiCorp.
func (result PackageAuthenticationResult) SignedByHashiCorp() bool {
	return result == officialProvider
}

// SignedByAnyParty returns whether the package was authenticated as signed by either HashiCorp or by a third-party.
func (result PackageAuthenticationResult) SignedByAnyParty() bool {
	return result == officialProvider || result == partnerProvider || result == communityProvider
}

// ThirdPartySigned returns whether the package was authenticated as signed by a party other than HashiCorp.
func (result PackageAuthenticationResult) ThirdPartySigned() bool {
	return result == partnerProvider || result == communityProvider
}

// SigningKey represents a key used to sign packages from a registry, along with an optional trust signature from the registry operator. These are both in ASCII armored OpenPGP format.
type SigningKey struct {
	ASCIIArmor     string `json:"ascii_armor"`
	TrustSignature string `json:"trust_signature"`
}

// PackageAuthentication implementation is responsible for authenticating that a package is what its distributor intended to distribute and that it has not been tampered with.
type PackageAuthentication interface {
	// Authenticate takes the path  of a package and returns a PackageAuthenticationResult, or an error if the authentication checks fail.
	Authenticate(path string) (*PackageAuthenticationResult, error)
}

// PackageAuthenticationHashes is an optional interface implemented by PackageAuthentication implementations that are able to return a set of hashes they would consider valid
// if a given path referred to a package that matched that hash string.
type PackageAuthenticationHashes interface {
	PackageAuthentication

	// AcceptableHashes returns a set of hashes that this authenticator considers to be valid for the current package or, where possible, equivalent packages on other platforms.
	AcceptableHashes() []Hash
}

type packageAuthenticationAll []PackageAuthentication

// PackageAuthenticationAll combines several authentications together into a single check value, which passes only if all of the given ones pass.
func PackageAuthenticationAll(checks ...PackageAuthentication) PackageAuthentication {
	return packageAuthenticationAll(checks)
}

func (checks packageAuthenticationAll) Authenticate(path string) (*PackageAuthenticationResult, error) {
	var authResult *PackageAuthenticationResult

	for _, check := range checks {
		var err error
		authResult, err = check.Authenticate(path)
		if err != nil {
			return authResult, err
		}
	}
	return authResult, nil
}

func (checks packageAuthenticationAll) AcceptableHashes() []Hash {
	for i := len(checks) - 1; i >= 0; i-- {
		check, ok := checks[i].(PackageAuthenticationHashes)
		if !ok {
			continue
		}
		allHashes := check.AcceptableHashes()
		if len(allHashes) > 0 {
			return allHashes
		}
	}
	return nil
}

type archiveHashAuthentication struct {
	WantSHA256Sum [sha256.Size]byte
}

// NewArchiveChecksumAuthentication returns a PackageAuthentication implementation that checks that the original distribution archive matches the given hash.
func NewArchiveChecksumAuthentication(wantSHA256Sum [sha256.Size]byte) PackageAuthentication {
	return archiveHashAuthentication{wantSHA256Sum}
}

func (a archiveHashAuthentication) Authenticate(path string) (*PackageAuthenticationResult, error) {
	gotHash, err := PackageHashLegacyZipSHA(path)
	if err != nil {
		return nil, errors.Errorf("failed to compute checksum for %s: %s", path, err)
	}
	wantHash := HashLegacyZipSHAFromSHA(a.WantSHA256Sum)
	if gotHash != wantHash {
		return nil, errors.Errorf("archive has incorrect checksum %s (expected %s)", gotHash, wantHash)
	}

	return NewPackageAuthenticationResult(verifiedChecksum), nil
}

func (a archiveHashAuthentication) AcceptableHashes() []Hash {
	return []Hash{HashLegacyZipSHAFromSHA(a.WantSHA256Sum)}
}

type matchingChecksumAuthentication struct {
	Document      []byte
	Filename      string
	WantSHA256Sum [sha256.Size]byte
}

// NewMatchingChecksumAuthentication returns a PackageAuthentication implementation that scans a registry-provided SHA256SUMS document for a specified filename,
// and compares the SHA256 hash against the expected hash
func NewMatchingChecksumAuthentication(document []byte, filename string, wantSHA256Sum [sha256.Size]byte) PackageAuthentication {
	return matchingChecksumAuthentication{
		Document:      document,
		Filename:      filename,
		WantSHA256Sum: wantSHA256Sum,
	}
}

func (m matchingChecksumAuthentication) Authenticate(location string) (*PackageAuthenticationResult, error) {
	// Find the checksum in the list with matching filename. The document is in the form "0123456789abcdef filename.zip".
	filename := []byte(m.Filename)
	var checksum []byte
	for _, line := range bytes.Split(m.Document, []byte("\n")) {
		parts := bytes.Fields(line)
		if len(parts) > 1 && bytes.Equal(parts[1], filename) {
			checksum = parts[0]
			break
		}
	}
	if checksum == nil {
		return nil, errors.Errorf("checksum list has no SHA-256 hash for %q", m.Filename)
	}

	// Decode the ASCII checksum into a byte array for comparison.
	var gotSHA256Sum [sha256.Size]byte
	if _, err := hex.Decode(gotSHA256Sum[:], checksum); err != nil {
		return nil, errors.Errorf("checksum list has invalid SHA256 hash %q: %s", string(checksum), err)
	}

	// If the checksums don't match, authentication fails.
	if !bytes.Equal(gotSHA256Sum[:], m.WantSHA256Sum[:]) {
		return nil, errors.Errorf("checksum list has unexpected SHA-256 hash %x (expected %x)", gotSHA256Sum, m.WantSHA256Sum[:])
	}

	return nil, nil
}

type signatureAuthentication struct {
	Document  []byte
	Signature []byte
	Keys      []SigningKey
}

// NewSignatureAuthentication returns a PackageAuthentication implementation that verifies the cryptographic signature for a package against any of the provided keys.
func NewSignatureAuthentication(document, signature []byte, keys []SigningKey) PackageAuthentication {
	return signatureAuthentication{
		Document:  document,
		Signature: signature,
		Keys:      keys,
	}
}

func (s signatureAuthentication) Authenticate(location string) (*PackageAuthenticationResult, error) {
	// Find the key that signed the checksum file. This can fail if there is no valid signature for any of the provided keys.
	signingKey, err := s.findSigningKey()
	if err != nil {
		return nil, err
	}

	// Verify the signature using the HashiCorp public key. If this succeeds, this is an official provider.
	hashicorpKeyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(HashicorpPublicKey))
	if err != nil {
		return nil, errors.Errorf("error creating HashiCorp keyring: %s", err)
	}

	if err := s.checkDetachedSignature(hashicorpKeyring, bytes.NewReader(s.Document), bytes.NewReader(s.Signature), nil); err == nil {
		return NewPackageAuthenticationResult(officialProvider), nil
	}

	// If the signing key has a trust signature, attempt to verify it with the HashiCorp partners public key.
	if signingKey.TrustSignature != "" {
		hashicorpPartnersKeyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(HashicorpPartnersKey))
		if err != nil {
			return nil, errors.Errorf("error creating HashiCorp Partners keyring: %s", err)
		}

		authorKey, err := openpgpArmor.Decode(strings.NewReader(signingKey.ASCIIArmor))
		if err != nil {
			return nil, errors.Errorf("error decoding signing key: %s", err)
		}

		trustSignature, err := openpgpArmor.Decode(strings.NewReader(signingKey.TrustSignature))
		if err != nil {
			return nil, errors.Errorf("error decoding trust signature: %s", err)
		}

		if err := s.checkDetachedSignature(hashicorpPartnersKeyring, authorKey.Body, trustSignature.Body, nil); err != nil {
			return nil, errors.Errorf("error verifying trust signature: %s", err)
		}

		return NewPackageAuthenticationResult(partnerProvider), nil
	}

	// We have a valid signature, but it's not from the HashiCorp key, and it also isn't a trusted partner. This is a community provider.
	return NewPackageAuthenticationResult(communityProvider), nil
}

func (s signatureAuthentication) checkDetachedSignature(keyring openpgp.KeyRing, signed, signature io.Reader, config *packet.Config) error {
	entity, err := openpgp.CheckDetachedSignature(keyring, signed, signature, config)

	if err == openpgpErrors.ErrKeyExpired {
		for id := range entity.Identities {
			log.Warnf("expired openpgp key from %s\n", id)
		}
		err = nil
	}
	return err
}

func (s signatureAuthentication) AcceptableHashes() []Hash {
	var hashes []Hash

	sc := bufio.NewScanner(bytes.NewReader(s.Document))
	for sc.Scan() {
		parts := bytes.Fields(sc.Bytes())
		if len(parts) != 0 && len(parts) < 2 {
			// Doesn't look like a valid sums file line, so we'll assume this whole thing isn't a checksums file.
			return nil
		}

		// If this is a checksums file then the first part should be a hex-encoded SHA256 hash, so it should be 64 characters long and contain only hex digits.
		hashStr := parts[0]
		hashLen := 64
		if len(hashStr) != hashLen {
			return nil // doesn't look like a checksums file
		}

		var gotSHA256Sum [sha256.Size]byte
		if _, err := hex.Decode(gotSHA256Sum[:], hashStr); err != nil {
			return nil // doesn't look like a checksums file
		}

		hashes = append(hashes, HashLegacyZipSHAFromSHA(gotSHA256Sum))
	}

	return hashes
}

// findSigningKey attempts to verify the signature using each of the keys returned by the registry. If a valid signature is found, it returns the signing key.
func (s signatureAuthentication) findSigningKey() (*SigningKey, error) {
	for _, key := range s.Keys {
		keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(key.ASCIIArmor))
		if err != nil {
			return nil, errors.Errorf("error decoding signing key: %s", err)
		}

		if err := s.checkDetachedSignature(keyring, bytes.NewReader(s.Document), bytes.NewReader(s.Signature), nil); err != nil {
			// If the signature issuer does not match the the key, keep trying the rest of the provided keys.
			if err == openpgpErrors.ErrUnknownIssuer {
				continue
			}

			// Any other signature error is terminal.
			return nil, errors.Errorf("error checking signature: %s", err)
		}

		return &key, nil
	}

	// If none of the provided keys issued the signature, this package is unsigned. This is currently a terminal authentication error.
	return nil, errors.Errorf("authentication signature from unknown issuer")
}
