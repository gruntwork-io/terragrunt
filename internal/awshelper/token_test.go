package awshelper_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oversizedToken stands in for the web identity tokens an OIDC provider issues.
// Only the length matters: a real one runs past what any platform accepts as a
// filename, which is what sends the stat down a different path than a miss.
func oversizedToken() string {
	return "not-a-real-web-identity-token-" + strings.Repeat("x", 700)
}

func TestFetchTokenReadsRawToken(t *testing.T) {
	t.Parallel()

	token := oversizedToken()

	// The OS filesystem rather than a memory one, which reports a name this
	// long as merely absent and would pass whether or not the length is
	// handled.
	got, err := awshelper.TokenFetcher{FS: vfs.NewOSFS(), Token: token}.FetchToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, token, string(got))
}

func TestFetchTokenReadsShortRawToken(t *testing.T) {
	t.Parallel()

	got, err := awshelper.TokenFetcher{FS: vfs.NewMemMapFS(), Token: "short-token"}.FetchToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "short-token", string(got))
}

func TestFetchTokenReadsTokenFile(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/tokens/web-identity", []byte("token-from-disk"), 0o600))

	got, err := awshelper.TokenFetcher{FS: fs, Token: "/tokens/web-identity"}.FetchToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "token-from-disk", string(got))
}

func TestFetchTokenSurfacesStatFailure(t *testing.T) {
	t.Parallel()

	fs := &statErrorFS{FS: vfs.NewMemMapFS(), err: os.ErrPermission}

	_, err := awshelper.TokenFetcher{FS: fs, Token: "/tokens/web-identity"}.FetchToken(t.Context())
	require.Error(t, err)
	require.ErrorIs(t, err, os.ErrPermission)
	assert.False(t, vfs.IsNameTooLong(err))
}

// statErrorFS fails every stat, standing in for a path the caller genuinely
// meant that the filesystem refuses to report on.
type statErrorFS struct {
	vfs.FS
	err error
}

func (fsys *statErrorFS) Stat(string) (os.FileInfo, error) {
	return nil, fsys.err
}
