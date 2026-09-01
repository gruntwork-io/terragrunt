package portal_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const previewPortalBaseURL = "https://preview.portal.example.com"

// tokenLifetime is what the portal issues, so a saved token is unexpired for
// the whole of a test that does not deliberately outlive it.
const tokenLifetime = 30 * 24 * time.Hour

func issuedToken(orgID, orgName string) *portal.Token {
	return &portal.Token{
		AccessToken: portal.Secret("fake-access-token-" + orgID),
		TokenType:   "Bearer",
		Scope:       portal.ScopeCatalogRead,
		Org:         portal.Org{ID: orgID, Name: orgName},
		Account:     portal.Account{Email: "someone@example.com"},
		ExpiresIn:   tokenLifetime,
	}
}

func saveToken(t *testing.T, v *venv.Venv, baseURL string, token *portal.Token) {
	t.Helper()

	require.NoError(t, portal.SaveToken(logger.CreateLogger(), v, baseURL, token))
}

func loadTokens(t *testing.T, v *venv.Venv, baseURL string) map[string]portal.StoredToken {
	t.Helper()

	tokens, err := portal.LoadTokens(logger.CreateLogger(), v, baseURL)
	require.NoError(t, err)

	return tokens
}

func storePath(t *testing.T, v *venv.Venv) string {
	t.Helper()

	path, err := portal.TokenStorePath(v)
	require.NoError(t, err)

	return path
}

func readStoreFile(t *testing.T, v *venv.Venv) string {
	t.Helper()

	data, err := vfs.ReadFile(v.FS, storePath(t, v))
	require.NoError(t, err)

	return string(data)
}

// lockingFS re-exposes the optional lock interface that wrapping a filesystem
// in a struct otherwise hides, so a decorated filesystem is still one a save
// can serialize on.
type lockingFS struct {
	vfs.FS
}

func (fsys lockingFS) Lock(name string) (vfs.Unlocker, error) {
	return vfs.Lock(fsys.FS, name)
}

func (fsys lockingFS) TryLock(name string) (vfs.Unlocker, bool, error) {
	return vfs.TryLock(fsys.FS, name)
}

func TestSaveTokenRoundTrip(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v, portalBaseURL, issuedToken("org_fake", "Acme"))

	tokens := loadTokens(t, v, portalBaseURL)
	require.Len(t, tokens, 1)

	got := tokens["org_fake"]
	assert.Equal(t, "fake-access-token-org_fake", got.AccessToken.Reveal())
	assert.Equal(t, "Bearer", got.TokenType)
	assert.Equal(t, portal.ScopeCatalogRead, got.Scope)
	assert.Equal(t, portal.Org{ID: "org_fake", Name: "Acme"}, got.Org)
	assert.Equal(t, portal.Account{Email: "someone@example.com"}, got.Account)
	assert.True(t, got.ExpiresAt.After(time.Now()), "a token just saved has not run out")
}

// TestSaveTokenKeepsTheTokensForOtherOrgs pins that signing in to a second org
// leaves the first one signed in. The two credentials reach different data, so
// one cannot stand in for the other.
func TestSaveTokenKeepsTheTokensForOtherOrgs(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v, portalBaseURL, issuedToken("org_one", "Acme"))
	saveToken(t, v, portalBaseURL, issuedToken("org_two", "Initech"))

	tokens := loadTokens(t, v, portalBaseURL)
	require.Len(t, tokens, 2)
	assert.Equal(t, "fake-access-token-org_one", tokens["org_one"].AccessToken.Reveal())
	assert.Equal(t, "fake-access-token-org_two", tokens["org_two"].AccessToken.Reveal())
	assert.Equal(t, "Initech", tokens["org_two"].Org.Name)
}

// TestSaveTokenKeepsBothCredentialsWithRacing pins that logins finishing at the
// same moment all end up in the store. Each one reads the whole file, adds its
// own org and writes the file back, so the last write would otherwise be the
// only one that survived.
func TestSaveTokenKeepsBothCredentialsWithRacing(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	orgIDs := []string{"org_one", "org_two", "org_three", "org_four", "org_five", "org_six"}

	var group errgroup.Group

	for _, orgID := range orgIDs {
		group.Go(func() error {
			return portal.SaveToken(logger.CreateLogger(), v, portalBaseURL, issuedToken(orgID, "Acme"))
		})
	}

	require.NoError(t, group.Wait())

	assert.Len(t, loadTokens(t, v, portalBaseURL), len(orgIDs))
}

// TestSaveTokenKeepsTheTokensForOtherPortals pins that a login against a preview
// deployment does not sign the user out of production. A credential is only
// good at the portal that issued it.
func TestSaveTokenKeepsTheTokensForOtherPortals(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v, portalBaseURL, issuedToken("org_fake", "Acme"))
	saveToken(t, v, previewPortalBaseURL, issuedToken("org_fake", "Acme"))

	production := loadTokens(t, v, portalBaseURL)
	require.Len(t, production, 1)

	preview := loadTokens(t, v, previewPortalBaseURL)
	require.Len(t, preview, 1)

	assert.Empty(t, loadTokens(t, v, "https://third.portal.example.com"))
}

// TestLoadTokensKeepsTheSchemesApart pins that a token issued over https is not
// handed back for a plaintext address naming the same machine. Handing it back
// would put the credential in an Authorization header travelling in the clear.
func TestLoadTokensKeepsTheSchemesApart(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v, "https://portal.example.com", issuedToken("org_fake", "Acme"))

	assert.Empty(t, loadTokens(t, v, "http://portal.example.com"))
	assert.Len(t, loadTokens(t, v, "https://portal.example.com"), 1)
}

// TestLoadTokensReadsOnePortalWrittenSeveralWays pins that the port a scheme
// implies, a host in another case, and a path after the host all reach the
// entry the login wrote, rather than reporting the user as logged out.
func TestLoadTokensReadsOnePortalWrittenSeveralWays(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v, "https://portal.example.com", issuedToken("org_fake", "Acme"))

	for _, baseURL := range []string{
		"https://portal.example.com:443",
		"https://PORTAL.example.com",
		"https://portal.example.com/api/v1",
	} {
		assert.Len(t, loadTokens(t, v, baseURL), 1, baseURL)
	}

	assert.Empty(t, loadTokens(t, v, "https://portal.example.com:8443"), "another port is another portal")
}

// TestSaveTokenRefreshesTheOrgName pins that logging in again reads the org's
// name afresh, so an org renamed in the portal keeps the entry it had rather
// than gaining a second one under the new name.
func TestSaveTokenRefreshesTheOrgName(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v, portalBaseURL, issuedToken("org_fake", "Acme"))
	saveToken(t, v, portalBaseURL, issuedToken("org_fake", "Acme Holdings"))

	tokens := loadTokens(t, v, portalBaseURL)
	require.Len(t, tokens, 1)
	assert.Equal(t, "Acme Holdings", tokens["org_fake"].Org.Name)
}

// TestLoadTokensLeavesOutAnExpiredToken pins that a token past its lifetime
// reads as no token at all. The portal issues no way to renew one, so the only
// answer is another login.
func TestLoadTokensLeavesOutAnExpiredToken(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		v := venvtest.New()

		saveToken(t, v, portalBaseURL, issuedToken("org_fake", "Acme"))
		require.Len(t, loadTokens(t, v, portalBaseURL), 1)

		time.Sleep(tokenLifetime + time.Hour)

		assert.Empty(t, loadTokens(t, v, portalBaseURL))
	})
}

// TestSaveTokenTakesAnExpiredTokenOffDisk pins that a credential the CLI has
// stopped handing back is removed rather than carried forward into the next
// save. The store is plaintext, so a backup or a synced configuration directory
// would otherwise pick up every token the user has ever been issued.
func TestSaveTokenTakesAnExpiredTokenOffDisk(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		v := venvtest.New()

		saveToken(t, v, portalBaseURL, issuedToken("org_old", "Acme"))
		saveToken(t, v, previewPortalBaseURL, issuedToken("org_old", "Acme"))

		time.Sleep(tokenLifetime + time.Hour)

		saveToken(t, v, portalBaseURL, issuedToken("org_new", "Initech"))

		contents := readStoreFile(t, v)
		assert.NotContains(t, contents, "fake-access-token-org_old")
		assert.NotContains(t, contents, "org_old")
		assert.NotContains(t, contents, "preview.portal.example.com", "a portal left holding nothing goes too")
		assert.Contains(t, contents, "fake-access-token-org_new", "the live credential is still there to read back")
	})
}

func TestSaveTokenWritesAnOwnerOnlyStore(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v, portalBaseURL, issuedToken("org_fake", "Acme"))

	path := storePath(t, v)

	info, err := v.FS.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the token store must be owner-only")

	dir, err := v.FS.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dir.Mode().Perm(), "the directory holding it must be owner-only")
}

// wideCreateFS opens every new file readable by anyone, standing in for a
// filesystem, or a scratch-file helper, that does not create one owner-only.
type wideCreateFS struct {
	lockingFS
}

func (fsys *wideCreateFS) OpenFile(name string, flag int, _ os.FileMode) (vfs.File, error) {
	return fsys.FS.OpenFile(name, flag, 0o666)
}

// TestSaveTokenClosesAScratchFileCreatedWide pins that the store's mode is set
// rather than inherited from whatever created the scratch file the save renames
// into place.
func TestSaveTokenClosesAScratchFileCreatedWide(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v.WithFS(&wideCreateFS{lockingFS: lockingFS{FS: v.FS}}), portalBaseURL, issuedToken("org_fake", "Acme"))

	info, err := v.FS.Stat(storePath(t, v))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// TestSaveTokenTightensAStoreThatWasAlreadyThere pins that the owner-only mode
// is enforced rather than requested. A store left behind by an earlier release,
// or by a user who copied their config directory around, is closed on the next
// save, and what it held survives.
func TestSaveTokenTightensAStoreThatWasAlreadyThere(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v, portalBaseURL, issuedToken("org_one", "Acme"))

	path := storePath(t, v)
	require.NoError(t, v.FS.Chmod(path, 0o644))
	require.NoError(t, v.FS.Chmod(filepath.Dir(path), 0o755))

	saveToken(t, v, portalBaseURL, issuedToken("org_two", "Initech"))

	info, err := v.FS.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a store left readable by others must be closed")

	dir, err := v.FS.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dir.Mode().Perm(), "a directory left listable by others must be closed")

	assert.Len(t, loadTokens(t, v, portalBaseURL), 2, "tightening the store must not lose what it held")
}

// TestSaveTokenWritesAnOwnerOnlyStoreOnDisk repeats the permission check against
// the real filesystem, whose rename has to carry the mode set on the scratch
// file over to the destination for the in-memory result to mean anything.
func TestSaveTokenWritesAnOwnerOnlyStoreOnDisk(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Windows file modes carry no owner-only bit to assert")
	}

	configDir := t.TempDir()
	v := venvtest.NewWithOSFS().WithUserConfigDir(func() (string, error) { return configDir, nil })

	saveToken(t, v, portalBaseURL, issuedToken("org_fake", "Acme"))

	path := storePath(t, v)
	require.NoError(t, v.FS.Chmod(path, 0o644))

	saveToken(t, v, portalBaseURL, issuedToken("org_fake", "Acme"))

	info, err := v.FS.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	dir, err := v.FS.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dir.Mode().Perm())
}

// TestLoadTokensIgnoresAStoreItCannotRead pins that a file the CLI cannot make
// sense of is treated as no tokens rather than as a failure. Nothing in it can
// be recovered, and another login costs the user less than hunting down a file
// every command refuses to run without.
func TestLoadTokensIgnoresAStoreItCannotRead(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		contents string
	}{
		{name: "not JSON", contents: "this is not a token store"},
		{name: "truncated", contents: `{"portals":{"https://portal.example.com":{"org_fake":{"access_tok`},
		{name: "empty", contents: ""},
		{name: "wrong shape", contents: `{"portals":["org_fake"]}`},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New()
			path := storePath(t, v)

			require.NoError(t, vfs.EnsureDirectory(v.FS, filepath.Dir(path)))
			require.NoError(t, vfs.WriteFile(v.FS, path, []byte(tt.contents), 0o600))

			assert.Empty(t, loadTokens(t, v, portalBaseURL))

			saveToken(t, v, portalBaseURL, issuedToken("org_fake", "Acme"))
			assert.Len(t, loadTokens(t, v, portalBaseURL), 1, "the next login must write over it")
		})
	}
}

// errRenameFailed stands in for whatever stops a save from finishing.
var errRenameFailed = errors.New("rename refused")

// renameFailingFS is a filesystem that will not publish a scratch file, which is
// the last step of a save and the only one that changes what a reader sees.
type renameFailingFS struct {
	lockingFS
}

func (fsys *renameFailingFS) Rename(_, _ string) error {
	return errRenameFailed
}

// TestSaveTokenLeavesTheStoreIntactWhenTheWriteFails pins that a save that dies
// partway costs the user nothing: the credentials already on disk still work,
// and no half-written copy of the new one is left behind.
func TestSaveTokenLeavesTheStoreIntactWhenTheWriteFails(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v, portalBaseURL, issuedToken("org_one", "Acme"))

	blocked := v.WithFS(&renameFailingFS{lockingFS: lockingFS{FS: v.FS}})
	err := portal.SaveToken(logger.CreateLogger(), blocked, portalBaseURL, issuedToken("org_two", "Initech"))
	require.ErrorIs(t, err, errRenameFailed)

	tokens := loadTokens(t, v, portalBaseURL)
	require.Len(t, tokens, 1)
	assert.Equal(t, "fake-access-token-org_one", tokens["org_one"].AccessToken.Reveal())

	assert.Equal(t, []string{"tokens.json"}, storeDirEntries(t, v), "a scratch file holds the credential too")
}

func storeDirEntries(t *testing.T, v *venv.Venv) []string {
	t.Helper()

	dir, err := v.FS.Open(filepath.Dir(storePath(t, v)))
	require.NoError(t, err)

	defer func() {
		require.NoError(t, dir.Close())
	}()

	names, err := dir.Readdirnames(-1)
	require.NoError(t, err)

	return names
}

func TestSaveTokenRejectsUnusableBaseURL(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	tc := []struct {
		want    error
		name    string
		baseURL string
	}{
		{name: "unparseable", baseURL: "://portal.example.com", want: portal.ErrUnusablePortalURL},
		{name: "no scheme", baseURL: "portal.example.com", want: portal.ErrNoPortalHost},
		{name: "not addressable", baseURL: "ftp://portal.example.com", want: portal.ErrPortalSchemeUnsupported},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := portal.SaveToken(logger.CreateLogger(), v, tt.baseURL, issuedToken("org_fake", "Acme"))
			require.ErrorIs(t, err, tt.want)
			require.ErrorIs(t, err, portal.ErrUnusablePortalURL)

			_, err = portal.LoadTokens(logger.CreateLogger(), v, tt.baseURL)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

// TestStoredTokenDoesNotReachFormattedOutput pins that a credential read back
// off disk is as guarded as the one that came off the wire, so a struct dump
// added while chasing a parse failure cannot carry it.
func TestStoredTokenDoesNotReachFormattedOutput(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	saveToken(t, v, portalBaseURL, issuedToken("org_fake", "Acme"))

	stored := loadTokens(t, v, portalBaseURL)["org_fake"]

	assert.NotContains(t, fmt.Sprintf("%v", stored), "fake-access-token")
	assert.NotContains(t, fmt.Sprintf("%+v", stored), "fake-access-token")

	// %#v does not consult String, so GoString is what keeps a struct dump from
	// carrying the credential.
	assert.NotContains(t, fmt.Sprintf("%#v", stored), "fake-access-token")
}
