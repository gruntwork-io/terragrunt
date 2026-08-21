// Package vsops provides a virtual SOPS decryption abstraction for testing and production use.
// It wraps the getsops/sops library to provide a consistent, injectable interface for
// decrypting SOPS-encrypted files.
package vsops

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/gruntwork-io/terragrunt/internal/locks"
)

// ErrEnvRestore is the panic value the OS decrypter raises when it cannot put
// a variable back after a decrypt. Restore only touches names the publishing
// half already set successfully, so a failure here means the environment is
// left holding another unit's credentials, which is not a state to continue a
// run in.
var ErrEnvRestore = errors.New("vsops: cannot restore the process environment after decrypting")

// ErrEnvNil is the panic value [Decrypter.DecryptFile] raises when its env
// argument is nil. Production callers take the map from a [venv.Venv], which
// guarantees a non-nil Env, so a nil points at a caller bug rather than a
// runtime condition.
var ErrEnvNil = errors.New("vsops: DecryptFile env must not be nil")

// Decrypter is the SOPS decryption interface used throughout the codebase.
// It provides an abstraction over the real sops library and in-memory
// decryption.
type Decrypter interface {
	// DecryptFile decrypts the SOPS-encrypted file at path, parsing its
	// content according to format, and returns the cleartext data. It panics
	// with [ErrEnvNil] on a nil env.
	DecryptFile(env map[string]string, path, format string) ([]byte, error)
}

// Handler processes a single decryption request for the in-memory backend and
// returns the cleartext. It is invoked synchronously by [Decrypter.DecryptFile].
type Handler func(env map[string]string, path, format string) ([]byte, error)

// FormatForPath returns the sops format name implied by the file extension of
// path: "yaml", "json", "dotenv", or "ini", falling back to "binary" for
// unrecognized extensions.
func FormatForPath(path string) string {
	return formatNames[formats.FormatForPath(path)]
}

// NewOSDecrypter returns a [Decrypter] backed by the real sops library. It
// reads the encrypted file from the OS filesystem and resolves data keys
// through the key services named in the file's sops metadata.
//
// Those key services read credentials from the process environment, and sops'
// only stable API, decrypt.File, takes no configuration through which they
// could be supplied instead. Reaching past the venv is unavoidable here, so it
// happens here rather than at a call site: DecryptFile publishes env into the
// process environment for the length of one decrypt and restores what it
// displaced.
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

func (osDecrypter) DecryptFile(env map[string]string, path, format string) ([]byte, error) {
	requireEnv(env)

	defer PublishEnv(env)()

	data, err := decrypt.File(path, format)
	if err != nil {
		if groupErrs := dataKeyGroupErrors(err); len(groupErrs) > 0 {
			return nil, errors.Join(groupErrs...)
		}

		return nil, err
	}

	return data, nil
}

type memDecrypter struct {
	handler Handler
}

func (d memDecrypter) DecryptFile(env map[string]string, path, format string) ([]byte, error) {
	requireEnv(env)

	return d.handler(env, path, format)
}

func requireEnv(env map[string]string) {
	if env == nil {
		panic(ErrEnvNil)
	}
}

// PublishEnv mirrors env into the process environment and returns the function
// that puts the environment back. It holds [locks.EnvLock] until that function
// runs, so no two callers hold the environment at once. [NewOSDecrypter] is
// the caller that needs it.
//
// It publishes blanks too. Credentials apply as a set, and an auth provider
// returning static keys reports an empty AWS_SESSION_TOKEN. Skipping that
// blank would pair the unit's key with the session token the process started
// with, an identity that belongs to neither.
func PublishEnv(env map[string]string) func() {
	locks.EnvLock.Lock()

	type displaced struct {
		value string
		set   bool
	}

	prior := make(map[string]displaced, len(env))

	for k, v := range env {
		old, ok := os.LookupEnv(k)
		if ok && old == v {
			continue
		}

		if err := os.Setenv(k, v); err != nil {
			continue
		}

		prior[k] = displaced{value: old, set: ok}
	}

	return func() {
		defer locks.EnvLock.Unlock()

		for k, d := range prior {
			if err := restore(k, d.value, d.set); err != nil {
				panic(fmt.Errorf("%w: %q: %w", ErrEnvRestore, k, err))
			}
		}
	}
}

// restore puts one variable back to the value it held before the decrypt, or
// removes it when the process did not carry it. FuzzOSDecrypterRestoresEnv
// exercises the names that make its error return fire.
func restore(name, value string, set bool) error {
	if !set {
		return os.Unsetenv(name)
	}

	return os.Setenv(name, value)
}

// dataKeyGroupErrors returns the per-key-group failures hidden in sops'
// getDataKeyError, whose own message doesn't explain why each key group
// failed. The sops library doesn't export the type or its fields, so the
// field walk is reflective and may break on future sops versions. A nil
// result means there is nothing to extract: err isn't a getDataKeyError,
// its shape changed, or no group recorded a failure (successful groups
// leave nil entries in GroupResults).
func dataKeyGroupErrors(err error) []error {
	errValue := reflect.ValueOf(err)
	if errValue.Kind() == reflect.Pointer {
		errValue = errValue.Elem()
	}

	if errValue.Type().Name() != "getDataKeyError" {
		return nil
	}

	field := errValue.FieldByName("GroupResults")
	if !field.IsValid() || field.Type() != reflect.TypeFor[[]error]() {
		return nil
	}

	var groupErrs []error

	for _, groupErr := range field.Interface().([]error) {
		if groupErr != nil {
			groupErrs = append(groupErrs, groupErr)
		}
	}

	return groupErrs
}
