package getproviders_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/getproviders"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePackageDir(t *testing.T, content string) string {
	t.Helper()

	dir := helpers.TmpDirWOSymlinks(t)

	const ownerWriteGlobalReadPerms = 0644
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terraform-provider-test"), []byte(content), ownerWriteGlobalReadPerms))

	return dir
}

func TestPackageHashV1(t *testing.T) {
	t.Parallel()

	dir := writePackageDir(t, "package contents")

	hash, err := getproviders.PackageHashV1(dir)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash.String(), "h1:"), "expected h1: scheme, got %q", hash)

	// The content hash must be deterministic for identical directory contents.
	again, err := getproviders.PackageHashV1(dir)
	require.NoError(t, err)
	assert.Equal(t, hash, again)

	// Different contents must yield a different hash.
	other, err := getproviders.PackageHashV1(writePackageDir(t, "different contents"))
	require.NoError(t, err)
	assert.NotEqual(t, hash, other)
}

func TestPackageHashV1NotADirectory(t *testing.T) {
	t.Parallel()

	dir := helpers.TmpDirWOSymlinks(t)
	filePath := filepath.Join(dir, "not-a-dir")

	const ownerWriteGlobalReadPerms = 0644
	require.NoError(t, os.WriteFile(filePath, []byte("x"), ownerWriteGlobalReadPerms))

	hash, err := getproviders.PackageHashV1(filePath)
	require.Error(t, err)
	assert.Empty(t, hash.String())
}

func TestPackageHashV1MissingPath(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(helpers.TmpDirWOSymlinks(t), "does-not-exist")

	hash, err := getproviders.PackageHashV1(missing)
	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.Empty(t, hash.String())
}
