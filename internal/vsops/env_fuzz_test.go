package vsops_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/stretchr/testify/require"
)

// setenv wraps os.Setenv so the fuzz target can seed a variable without
// t.Setenv, which a fuzz target may not call.
func setenv(name, value string) error {
	return os.Setenv(name, value)
}

// FuzzOSDecrypterRestoresEnv validates the assumption the OS decrypter's
// restore path asserts on: whatever name and value a caller's environment
// carries, a decrypt either leaves the process environment untouched or puts
// back exactly what it displaced.
//
// The interesting inputs are the names os.Setenv rejects (empty, holding a
// NUL, holding an "=" on Unix), since those are the ones that could be
// published without being restorable. A rejected name must never reach the
// restore set, and the panic it would raise there is what this proves absent.
func FuzzOSDecrypterRestoresEnv(f *testing.F) {
	f.Add("SOPS_FUZZ_KEY", "prior", "published", true)
	f.Add("SOPS_FUZZ_KEY", "", "published", false)
	f.Add("SOPS_FUZZ_KEY", "prior", "", true)
	f.Add("", "prior", "published", false)
	f.Add("SOPS=FUZZ", "prior", "published", false)
	f.Add("SOPS\x00FUZZ", "prior", "published", false)
	f.Add("SOPS_FUZZ_KEY", "same", "same", true)

	path := filepath.Join(f.TempDir(), "secret.json")
	require.NoError(f, os.WriteFile(path, []byte(`{"value":1}`), 0o600))

	d := vsops.NewOSDecrypter()

	f.Fuzz(func(t *testing.T, name, prior, published string, hasPrior bool) {
		// A name the process cannot hold in the first place says nothing about
		// restore, and setting it here would fail for reasons of its own.
		if hasPrior && setenv(name, prior) != nil {
			t.Skip("the OS rejects this name, so there is no prior value to displace")
		}

		wantValue, wantSet := os.LookupEnv(name)

		_, err := d.DecryptFile(map[string]string{name: published}, path, "json")
		require.Error(t, err)

		gotValue, gotSet := os.LookupEnv(name)
		require.Equal(t, wantSet, gotSet, "presence of %q changed across the decrypt", name)
		require.Equal(t, wantValue, gotValue, "value of %q changed across the decrypt", name)

		if hasPrior {
			require.NoError(t, os.Unsetenv(name))
		}
	})
}
