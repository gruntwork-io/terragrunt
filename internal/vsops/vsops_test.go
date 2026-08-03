package vsops_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatForPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path     string
		expected string
	}{
		{path: "secrets.yaml", expected: "yaml"},
		{path: "secrets.yml", expected: "yaml"},
		{path: "secrets.json", expected: "json"},
		{path: "secrets.env", expected: "dotenv"},
		{path: "secrets.ini", expected: "ini"},
		{path: "secrets.txt", expected: "binary"},
		{path: "secrets", expected: "binary"},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, vsops.FormatForPath(tc.path))
		})
	}
}

func TestMemDecrypterDispatchesToHandler(t *testing.T) {
	t.Parallel()

	d := vsops.NewMemDecrypter(func(path, format string) ([]byte, error) {
		assert.Equal(t, "secrets.env", path)
		assert.Equal(t, "dotenv", format)

		return []byte("value=cleartext"), nil
	})

	data, err := d.DecryptFile("secrets.env", "dotenv")
	require.NoError(t, err)
	assert.Equal(t, "value=cleartext", string(data))
}

func TestMemDecrypterReturnsHandlerError(t *testing.T) {
	t.Parallel()

	handlerErr := errors.New("no data key")

	d := vsops.NewMemDecrypter(func(string, string) ([]byte, error) {
		return nil, handlerErr
	})

	data, err := d.DecryptFile("secrets.json", "json")
	require.ErrorIs(t, err, handlerErr)
	assert.Nil(t, data)
}

func TestNewMemDecrypterNilHandlerPanics(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		vsops.NewMemDecrypter(nil)
	})
}

func TestOSDecrypterMissingFile(t *testing.T) {
	t.Parallel()

	d := vsops.NewOSDecrypter()

	_, err := d.DecryptFile(filepath.Join(t.TempDir(), "missing.json"), "json")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestOSDecrypterFileWithoutSopsMetadata(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "plain.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"value":"not encrypted"}`), 0600))

	d := vsops.NewOSDecrypter()

	_, err := d.DecryptFile(path, "json")
	require.Error(t, err)
}
