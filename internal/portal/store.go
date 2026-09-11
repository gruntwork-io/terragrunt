package portal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	tokenStoreDirName  = "terragrunt"
	tokenStoreFileName = "tokens.json"

	// tokenStoreLockSuffix names the file the read-change-write in [SaveToken]
	// is serialized on. It sits beside the store rather than on it, so the lock
	// outlives the rename that publishes a new store.
	tokenStoreLockSuffix = ".lock"

	tokenStoreFileMode os.FileMode = 0o600
	tokenStoreDirMode  os.FileMode = 0o700
)

// StoredToken is a credential an earlier login left on disk, together with what
// it reaches and when it stops working. [Secret] keeps the credential out of
// terminal output and log lines.
type StoredToken struct {
	ExpiresAt   time.Time
	AccessToken Secret
	TokenType   string
	Scope       string
	Org         Org
	Account     Account
}

// tokenStore is the on-disk shape. A credential is filed under the portal it
// came from and then the org's id, so signing in to a second org, or to a
// preview portal alongside production, adds an entry instead of replacing one.
type tokenStore struct {
	Portals map[string]map[string]storedToken `json:"portals"`
}

// storedToken is the on-disk form of a credential. [Secret] redacts terminal
// and log output, not JSON, so the credential is written in the clear and the
// file's mode is what keeps it private.
type storedToken struct {
	ExpiresAt    time.Time `json:"expires_at"`
	AccessToken  Secret    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
	OrgName      string    `json:"org_name"`
	AccountEmail string    `json:"account_email"`
}

// TokenStorePath reports the file the CLI keeps portal credentials in.
func TokenStorePath(v *venv.Venv) (string, error) {
	v.RequireUserConfigDir()

	configDir, err := v.Platform.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating the user configuration directory: %w", err)
	}

	return filepath.Join(configDir, tokenStoreDirName, tokenStoreFileName), nil
}

// SaveToken records token as the credential for its org at the portal serving
// baseURL, so a later command reaches that org without a browser. Orgs are
// keyed by id, so a second save replaces the entry rather than adding one under
// the org's new name.
//
// The write leaves the store readable only by its owner and drops any
// credential that has expired.
func SaveToken(l log.Logger, v *venv.Venv, baseURL string, token *Token) error {
	v.RequireFS()

	key, err := portalKey(baseURL)
	if err != nil {
		return err
	}

	path, err := TokenStorePath(v)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := vfs.EnsureDirectory(v.FS, dir); err != nil {
		return fmt.Errorf("creating the directory for the portal token store: %w", err)
	}

	// EnsureDirectory returns early on a directory that already exists, without
	// touching its mode.
	if err := v.FS.Chmod(dir, tokenStoreDirMode); err != nil {
		return fmt.Errorf("restricting the directory holding the portal token store: %w", err)
	}

	// Two logins can be approved at once, and each rewrites the whole store.
	unlock, err := vfs.Lock(v.FS, path+tokenStoreLockSuffix)
	if err != nil {
		return fmt.Errorf("locking the portal token store: %w", err)
	}

	defer func() {
		if err := unlock.Unlock(); err != nil {
			l.Warnf("Could not release the lock on the portal token store: %v", err)
		}
	}()

	store, err := readTokenStore(l, v.FS, path)
	if err != nil {
		return err
	}

	now := time.Now()

	store.dropExpired(now)
	store.record(key, token, now)

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the portal token store: %w", err)
	}

	return writeFileAtomic(v.FS, path, data, tokenStoreFileMode)
}

// LoadTokens returns the credentials held for the portal serving baseURL, keyed
// by org id. An expired credential is left out.
func LoadTokens(l log.Logger, v *venv.Venv, baseURL string) (map[string]StoredToken, error) {
	v.RequireFS()

	key, err := portalKey(baseURL)
	if err != nil {
		return nil, err
	}

	path, err := TokenStorePath(v)
	if err != nil {
		return nil, err
	}

	store, err := readTokenStore(l, v.FS, path)
	if err != nil {
		return nil, err
	}

	return store.unexpired(key, time.Now()), nil
}

// unexpired returns the credentials the portal named by key still has, keyed by
// org id.
func (s *tokenStore) unexpired(key string, now time.Time) map[string]StoredToken {
	orgs := s.Portals[key]
	tokens := make(map[string]StoredToken, len(orgs))

	for orgID, entry := range orgs {
		if !entry.ExpiresAt.After(now) {
			continue
		}

		tokens[orgID] = StoredToken{
			ExpiresAt:   entry.ExpiresAt,
			AccessToken: entry.AccessToken,
			TokenType:   entry.TokenType,
			Scope:       entry.Scope,
			Org:         Org{ID: orgID, Name: entry.OrgName},
			Account:     Account{Email: entry.AccountEmail},
		}
	}

	return tokens
}

// record files token under the portal named by key, creating the maps on the
// way down when the store has none yet.
func (s *tokenStore) record(key string, token *Token, now time.Time) {
	if s.Portals == nil {
		s.Portals = map[string]map[string]storedToken{}
	}

	if s.Portals[key] == nil {
		s.Portals[key] = map[string]storedToken{}
	}

	s.Portals[key][token.Org.ID] = storedToken{
		ExpiresAt:    now.Add(token.ExpiresIn).UTC(),
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		Scope:        token.Scope,
		OrgName:      token.Org.Name,
		AccountEmail: token.Account.Email,
	}
}

// dropExpired removes every credential that has run out, along with any portal
// left holding none.
func (s *tokenStore) dropExpired(now time.Time) {
	for key, orgs := range s.Portals {
		maps.DeleteFunc(orgs, func(_ string, entry storedToken) bool {
			return !entry.ExpiresAt.After(now)
		})

		if len(orgs) == 0 {
			delete(s.Portals, key)
		}
	}
}

// readTokenStore reads the store off disk.
func readTokenStore(l log.Logger, fsys vfs.FS, path string) (tokenStore, error) {
	data, err := vfs.ReadFile(fsys, path)
	if errors.Is(err, fs.ErrNotExist) {
		return tokenStore{}, nil
	}

	if err != nil {
		return tokenStore{}, fmt.Errorf("reading the portal token store: %w", err)
	}

	var store tokenStore
	if err := json.Unmarshal(data, &store); err != nil {
		l.Debugf("Discarding an unreadable portal token store: %v", err)

		return tokenStore{}, nil
	}

	return store, nil
}

// writeFileAtomic writes a scratch file beside path and renames it over the
// destination, so a run that dies midway leaves the previous file whole rather
// than a truncated one. The rename also replaces a symlink at path instead of
// following it. [vfs.CreateTemp] chooses the scratch file's mode, so perm is
// applied before anything is written to it.
//
// NOTE: This duplicates vfs.WriteFileAtomic, which is not on main yet. Delete
// it and call that instead once https://github.com/gruntwork-io/terragrunt/pull/6776
// merges.
func writeFileAtomic(fsys vfs.FS, path string, data []byte, perm os.FileMode) error {
	file, err := vfs.CreateTemp(fsys, filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating a scratch file beside %s: %w", filepath.Base(path), err)
	}

	tmpPath := file.Name()

	if err := fsys.Chmod(tmpPath, perm); err != nil {
		return errors.Join(err, file.Close(), fsys.Remove(tmpPath))
	}

	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close(), fsys.Remove(tmpPath))
	}

	if err := file.Close(); err != nil {
		return errors.Join(err, fsys.Remove(tmpPath))
	}

	// The scratch file holds the credential too, so a rename that did not happen
	// must not leave a second copy of it lying around.
	if err := fsys.Rename(tmpPath, path); err != nil {
		return errors.Join(err, fsys.Remove(tmpPath))
	}

	return nil
}

// portalKey is the key credentials are filed under. It carries the scheme, so a
// credential issued over https is never handed back for a plaintext address
// naming the same machine, and a port the scheme does not imply, so two portals
// on one machine stay apart. The host is lowercased, so a portal written two
// ways reaches the entry the earlier login wrote.
func portalKey(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", &UnusablePortalURLError{URL: baseURL, Err: fmt.Errorf("%w: %w", ErrUnusablePortalURL, err)}
	}

	host := strings.ToLower(parsed.Host)
	if host == "" {
		return "", &UnusablePortalURLError{URL: baseURL, Err: ErrNoPortalHost}
	}

	var defaultPort string

	switch parsed.Scheme {
	case "https":
		defaultPort = ":443"
	case "http":
		defaultPort = ":80"
	default:
		return "", &UnusablePortalURLError{URL: baseURL, Err: ErrPortalSchemeUnsupported}
	}

	return parsed.Scheme + "://" + strings.TrimSuffix(host, defaultPort), nil
}
