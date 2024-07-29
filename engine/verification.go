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

// verifyFile verifies the checksums file and the signature file of the passed file
func verifyFile(checkedFile, checksumsFile, signatureFile string) error {
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
	packageChecksum, err := util.FileSHA256(checkedFile)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	// match expected checksum
	expectedChecksum := util.MatchSha256Checksum(checksums, []byte(filepath.Base(checkedFile)))
	if expectedChecksum == nil {
		return errors.Errorf("checksum list has no entry for %s", checkedFile)
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
