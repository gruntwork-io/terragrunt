// Package vsops provides a virtual SOPS decryption abstraction for testing and production use.
// It wraps the getsops/sops library to provide a consistent, injectable interface for
// decrypting SOPS-encrypted files.
package vsops

import (
	"errors"
	"reflect"

	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
)

// Decrypter is the SOPS decryption interface used throughout the codebase.
// It provides an abstraction over the real sops library and in-memory
// decryption.
type Decrypter interface {
	// DecryptFile decrypts the SOPS-encrypted file at path, parsing its
	// content according to format, and returns the cleartext data.
	DecryptFile(path, format string) ([]byte, error)
}

// Handler processes a single decryption request for the in-memory backend and
// returns the cleartext. It is invoked synchronously by [Decrypter.DecryptFile].
type Handler func(path, format string) ([]byte, error)

// FormatForPath returns the sops format name implied by the file extension of
// path: "yaml", "json", "dotenv", or "ini", falling back to "binary" for
// unrecognized extensions.
func FormatForPath(path string) string {
	return formatNames[formats.FormatForPath(path)]
}

// NewOSDecrypter returns a [Decrypter] backed by the real sops library. It reads
// the encrypted file from the OS filesystem and resolves data keys through the
// key services named in the file's sops metadata, which draw credentials from
// the process environment.
func NewOSDecrypter() Decrypter {
	return osDecrypter{}
}

// NewMemDecrypter returns a [Decrypter] whose [Decrypter.DecryptFile] calls
// are dispatched to h instead of the sops library. It is intended for tests:
// h decides how each request should behave.
//
// h must not be nil.
func NewMemDecrypter(h Handler) Decrypter {
	if h == nil {
		panic("vsops: NewMemDecrypter requires a non-nil Handler")
	}

	return memDecrypter{handler: h}
}

var formatNames = map[formats.Format]string{
	formats.Binary: "binary",
	formats.Dotenv: "dotenv",
	formats.Ini:    "ini",
	formats.Json:   "json",
	formats.Yaml:   "yaml",
}

type osDecrypter struct{}

func (osDecrypter) DecryptFile(path, format string) ([]byte, error) {
	data, err := decrypt.File(path, format)
	if err != nil {
		return nil, extractGroupErrors(err)
	}

	return data, nil
}

type memDecrypter struct {
	handler Handler
}

func (d memDecrypter) DecryptFile(path, format string) ([]byte, error) {
	return d.handler(path, format)
}

// extractGroupErrors pulls the per-group errors out of sops' getDataKeyError via reflection.
// The sops library doesn't export these, so the field walk may break on future sops versions.
func extractGroupErrors(err error) error {
	var errs []error

	errValue := reflect.ValueOf(err)
	if errValue.Kind() == reflect.Pointer {
		errValue = errValue.Elem()
	}

	if errValue.Type().Name() == "getDataKeyError" {
		groupResultsField := errValue.FieldByName("GroupResults")
		if groupResultsField.IsValid() && groupResultsField.Kind() == reflect.Slice {
			for i := range groupResultsField.Len() {
				groupErr := groupResultsField.Index(i)
				if groupErr.CanInterface() {
					if resultErr, ok := groupErr.Interface().(error); ok {
						errs = append(errs, resultErr)
					}
				}
			}
		}
	}

	if len(errs) == 0 {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
