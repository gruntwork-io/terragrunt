package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	PublicKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----

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
iE6bHk4c9kBU
=TmYc
-----END PGP PUBLIC KEY BLOCK-----`
)

func verifyEngine(packageFile, checksumsFile, signatureFile string) error {

	checksums, err := os.ReadFile(checksumsFile)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	checksumsSignature, err := os.ReadFile(signatureFile)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// validate first checksum file signature
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(PublicKey))
	if err != nil {
		return errors.WithStackTrace(err)
	}
	_, err = openpgp.CheckDetachedSignature(keyring, bytes.NewReader(checksums), bytes.NewReader(checksumsSignature), nil)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// verify checksums
	// calculate checksum of package file
	packageChecksum, err := util.FileSHA256(packageFile)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	// match expected checksum
	expectedChecksum := util.MatchSha256Checksum(checksums, []byte(filepath.Base(packageFile)))
	if expectedChecksum == nil {
		return errors.Errorf("checksum list has no entry for %s", packageFile)
	}
	var expectedSHA256Sum [sha256.Size]byte
	if _, err := hex.Decode(expectedSHA256Sum[:], expectedChecksum); err != nil {
		return errors.WithStackTrace(err)
	}

	if !bytes.Equal(expectedSHA256Sum[:], packageChecksum) {
		return errors.Errorf("checksum list has unexpected SHA-256 hash %x (expected %x)", packageChecksum, expectedSHA256Sum)
	}

	return nil
}