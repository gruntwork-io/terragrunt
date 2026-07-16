package getter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"maps"
	"net/http"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/version"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// Interim, undocumented static-credential env vars (unscoped without TG_TMP_OCI_REGISTRY).
const (
	EnvOCIUsername = "TG_TMP_OCI_USERNAME"
	EnvOCIPassword = "TG_TMP_OCI_PASSWORD"
	EnvOCIToken    = "TG_TMP_OCI_TOKEN"
	EnvOCIRegistry = "TG_TMP_OCI_REGISTRY"
)

// ociDockerHubKey is the lookup key every Docker Hub spelling folds onto.
const ociDockerHubKey = "docker.io"

// ociDockerHubAuthKeys are the names login tools write for Docker Hub: docker
// writes the legacy index URL, podman writes the bare domain, and oras routes
// Hub traffic through registry-1.docker.io.
var ociDockerHubAuthKeys = []string{
	"https://index.docker.io/v1/",
	"docker.io",
	"index.docker.io",
	"registry-1.docker.io",
}

// ErrOCIStaticCredentialConflict reports both a token and a username or password set.
var ErrOCIStaticCredentialConflict = errors.New("cannot set both a token and a username or password for oci sources")

// ErrOCIStaticCredentialIncomplete reports a username without a password, or the reverse.
var ErrOCIStaticCredentialIncomplete = errors.New("oci static credentials require both a username and a password")

// ErrOCINonOSFilesystem reports a non-OS-backed filesystem oras cannot read through.
var ErrOCINonOSFilesystem = errors.New("oci credential discovery requires an OS-backed filesystem")

// The default store must satisfy the seam the getter consumes.
var _ OCIRepositoryStore = (*remote.Repository)(nil)

// ociUserAgent is the versioned User-Agent sent to registries.
func ociUserAgent() string {
	return "terragrunt/" + version.GetVersion()
}

// ociCredentialFactory binds credential resolution to one repository, since the
// most specific ambient entry depends on the repository being pulled.
type ociCredentialFactory func(repositoryName string) auth.CredentialFunc

// NewOCIRepositoryStore returns the default Tier-1 store: static env creds, else ambient discovery.
func NewOCIRepositoryStore(l log.Logger, v venv.Venv) OCINewStoreFunc {
	resolveCredential := sync.OnceValues(func() (ociCredentialFactory, error) {
		return ociCredentialFunc(l, v)
	})
	caches := &ociCacheSet{caches: map[string]auth.Cache{}}

	return func(_ context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
		if !vfs.IsOSFS(v.FS) {
			return nil, ErrOCINonOSFilesystem
		}

		credentialFor, err := resolveCredential()
		if err != nil {
			return nil, err
		}

		reference := registryDomain + "/" + repositoryName

		repo, err := remote.NewRepository(reference)
		if err != nil {
			return nil, fmt.Errorf("parsing OCI repository reference: %w", err)
		}

		repo.Client = &auth.Client{
			Client:     retry.DefaultClient,
			Cache:      caches.get(reference),
			Credential: credentialFor(repositoryName),
			Header:     http.Header{"User-Agent": []string{ociUserAgent()}},
		}

		return repo, nil
	}
}

// ociCredentialFunc resolves static env credentials, falling back to ambient discovery.
func ociCredentialFunc(l log.Logger, v venv.Venv) (ociCredentialFactory, error) {
	staticCred, found, err := ociStaticCredential(v)
	if err != nil {
		return nil, err
	}

	ambient := ociAmbientCredentialFunc(l, v, runtime.GOOS)

	if !found {
		return ambient, nil
	}

	registry := v.Env[EnvOCIRegistry]
	if registry == "" {
		// Unscoped: the credential applies to every registry the run contacts.
		return func(_ string) auth.CredentialFunc {
			return func(_ context.Context, _ string) (auth.Credential, error) {
				return staticCred, nil
			}
		}, nil
	}

	// Scoped: static serves its registry, the rest still resolve ambient.
	scoped := auth.StaticCredential(registry, staticCred)

	return func(repositoryName string) auth.CredentialFunc {
		ambientFn := ambient(repositoryName)

		return func(ctx context.Context, hostport string) (auth.Credential, error) {
			cred, err := scoped(ctx, hostport)
			if err != nil {
				return auth.EmptyCredential, err
			}

			if cred != auth.EmptyCredential {
				return cred, nil
			}

			return ambientFn(ctx, hostport)
		}
	}, nil
}

// ociStaticCredential reads a token or a username plus password from v.Env.
func ociStaticCredential(v venv.Venv) (auth.Credential, bool, error) {
	username := v.Env[EnvOCIUsername]
	password := v.Env[EnvOCIPassword]
	token := v.Env[EnvOCIToken]

	if token != "" && (username != "" || password != "") {
		return auth.EmptyCredential, false, ErrOCIStaticCredentialConflict
	}

	if token != "" {
		return auth.Credential{AccessToken: token}, true, nil
	}

	if username != "" && password != "" {
		return auth.Credential{Username: username, Password: password}, true, nil
	}

	if username != "" || password != "" {
		return auth.EmptyCredential, false, ErrOCIStaticCredentialIncomplete
	}

	return auth.EmptyCredential, false, nil
}

// ociAmbientStore pairs a credential store with its source file, so warnings can
// name it, and with the keys that file declares.
type ociAmbientStore struct {
	store credentials.Store
	// declared maps a lookup key to the key the file spells it with. Only keys
	// present here are ever requested, because oras answers a bare hostname
	// query with an arbitrary repository-scoped entry when no exact key exists.
	declared map[string]string
	path     string
}

// ociAmbientCredentialFunc consults existing ambient files in order, skipping unusable ones.
func ociAmbientCredentialFunc(l log.Logger, v venv.Venv, goos string) ociCredentialFactory {
	candidates := ociAmbientCredentialPaths(v, goos)
	stores := make([]ociAmbientStore, 0, len(candidates))

	for _, candidate := range candidates {
		if _, err := v.FS.Stat(candidate); err != nil {
			if !errors.Is(err, iofs.ErrNotExist) {
				l.Warnf("Skipping OCI credential file %s: %v", candidate, err)
			}

			continue
		}

		declared, err := ociAuthFileKeys(v, candidate)
		if err != nil {
			l.Warnf("Skipping unparseable OCI credential file %s: %v", candidate, err)

			continue
		}

		store, err := credentials.NewFileStore(candidate)
		if err != nil {
			l.Warnf("Skipping unparseable OCI credential file %s: %v", candidate, err)

			continue
		}

		stores = append(stores, ociAmbientStore{store: store, declared: declared, path: candidate})
	}

	return func(repositoryName string) auth.CredentialFunc {
		return func(ctx context.Context, hostport string) (auth.Credential, error) {
			// Most specific key first, so a repository-scoped entry beats a
			// registry-wide one and no other namespace's entry can answer.
			for _, key := range ociCredentialKeys(hostport, repositoryName) {
				for _, ambient := range stores {
					declared, ok := ambient.declared[key]
					if !ok {
						continue
					}

					// Malformed entry: warn without the error (it can echo decoded secrets) and fall through.
					cred, err := ambient.store.Get(ctx, declared)
					if err != nil {
						l.Warnf("Skipping unusable OCI credential entry for %s in %s", declared, ambient.path)

						continue
					}

					if cred != auth.EmptyCredential {
						return cred, nil
					}
				}
			}

			return auth.EmptyCredential, nil
		}
	}
}

// ociAmbientCredentialPaths returns tofu's containers-auth candidates for goos (best-effort parity).
func ociAmbientCredentialPaths(v venv.Venv, goos string) []string {
	var paths []string

	// Linux only: the containers runtime auth file.
	if goos == "linux" {
		if dir := v.Env["XDG_RUNTIME_DIR"]; dir != "" {
			paths = append(paths, filepath.Join(dir, "containers", "auth.json"))
		}
	}

	home := ociUserHome(v, goos)

	// Windows and macOS: the literal ~/.config location, always searched.
	if (goos == "windows" || goos == "darwin") && home != "" {
		paths = append(paths, filepath.Join(home, ".config", "containers", "auth.json"))
	}

	configDir := v.Env["XDG_CONFIG_HOME"]
	if configDir == "" && home != "" {
		configDir = filepath.Join(home, ".config")
	}

	if configDir != "" {
		paths = append(paths, filepath.Join(configDir, "containers", "auth.json"))
	}

	// The literal Docker CLI config location; DOCKER_CONFIG is not honored, matching tofu.
	if home != "" {
		paths = append(paths, filepath.Join(home, ".docker", "config.json"))
	}

	// Drop the ~/.config duplicate the macOS/Windows slot and XDG default can produce.
	return slices.Compact(paths)
}

// ociUserHome resolves the home directory: USERPROFILE on Windows, HOME elsewhere.
func ociUserHome(v venv.Venv, goos string) string {
	if goos == "windows" {
		return v.Env["USERPROFILE"]
	}

	return v.Env["HOME"]
}

// ociCredentialKeys returns the lookup keys serving repositoryName on hostport,
// most specific first: the repository-scoped containers-auth entries, then the
// registry itself.
func ociCredentialKeys(hostport, repositoryName string) []string {
	registry := ociCanonicalAuthKey(hostport)
	if registry == "" {
		return nil
	}

	var keys []string

	if repositoryName != "" {
		segments := strings.Split(repositoryName, "/")
		for i := len(segments); i > 0; i-- {
			keys = append(keys, registry+"/"+strings.Join(segments[:i], "/"))
		}
	}

	return append(keys, registry)
}

// ociCanonicalAuthKey folds an auth file key, or a request host, onto the key
// the resolver matches on, so the Docker Hub spellings and the scheme a legacy
// Docker config carries all meet in one place.
func ociCanonicalAuthKey(key string) string {
	if slices.Contains(ociDockerHubAuthKeys, key) {
		return ociDockerHubKey
	}

	stripped := strings.TrimPrefix(key, "https://")
	stripped = strings.TrimPrefix(stripped, "http://")

	return strings.TrimSuffix(stripped, "/")
}

// ociAuthFileKeys reads the auths keys path declares, mapping each to the key
// the file spells it with, so every lookup can be an exact match.
func ociAuthFileKeys(v venv.Venv, path string) (map[string]string, error) {
	data, err := vfs.ReadFile(v.FS, path)
	if err != nil {
		return nil, err
	}

	var file struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}

	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	keys := make(map[string]string, len(file.Auths))

	// Sorted, so two spellings of one key resolve the same way on every run.
	for _, declared := range slices.Sorted(maps.Keys(file.Auths)) {
		canonical := ociCanonicalAuthKey(declared)
		if _, taken := keys[canonical]; taken {
			continue
		}

		keys[canonical] = declared
	}

	return keys, nil
}

// ociCacheSet hands out one auth cache per repository, so a credential cached
// for one repository is never replayed for another on the same host.
type ociCacheSet struct {
	caches map[string]auth.Cache
	mu     sync.Mutex
}

func (s *ociCacheSet) get(reference string) auth.Cache {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cache, ok := s.caches[reference]; ok {
		return cache
	}

	cache := auth.NewCache()
	s.caches[reference] = cache

	return cache
}
