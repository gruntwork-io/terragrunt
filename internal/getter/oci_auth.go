package getter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/version"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
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

// ociDockerHubRegistries are the registry hosts login tools use for Docker Hub.
// Docker also writes the legacy index.docker.io/v1 URL, handled separately by
// ociCanonicalAuthKey after URL normalization.
var ociDockerHubRegistries = []string{
	"docker.io",
	"index.docker.io",
	"registry-1.docker.io",
}

// ErrOCIStaticCredentialConflict reports both a token and a username or password set.
var ErrOCIStaticCredentialConflict = errors.New("cannot set both a token and a username or password for oci sources")

// ErrOCIStaticCredentialIncomplete reports a username without a password, or the reverse.
var ErrOCIStaticCredentialIncomplete = errors.New("oci static credentials require both a username and a password")

// ErrOCIHelperMalformedOutput reports a credential helper whose output is not valid JSON.
var ErrOCIHelperMalformedOutput = errors.New("oci credential helper returned malformed output")

const (
	// ociCredentialHelperPrefix is the docker-credential binary name prefix.
	ociCredentialHelperPrefix = "docker-credential-"
	// ociHelperTokenUsername marks a bearer token in helper output.
	ociHelperTokenUsername = "<token>"
	// ociHelperCredentialsNotFound is the benign not-in-keychain helper message.
	ociHelperCredentialsNotFound = "credentials not found in native keychain"
	// ociCredentialHelperTimeout bounds a helper subprocess so it cannot wedge a run.
	ociCredentialHelperTimeout = 30 * time.Second
	// ociDockerHubIndexServer is the server address helpers store Docker Hub creds under.
	ociDockerHubIndexServer = "https://index.docker.io/v1/"
)

// OCICredentialHelperError reports a credential helper that could not be run or
// whose output could not be used. It never carries the helper's secret output.
type OCICredentialHelperError struct {
	Err      error
	Helper   string
	Registry string
}

func (err OCICredentialHelperError) Error() string {
	return fmt.Sprintf("oci credential helper %q for %s: %v", err.Helper, err.Registry, err.Err)
}

func (err OCICredentialHelperError) Unwrap() error {
	return err.Err
}

// OCIRemoteStore adapts oras' by-value Fetch to the pointer-taking [OCIRepositoryStore] seam.
type OCIRemoteStore struct {
	Repo *remote.Repository
}

// Resolve delegates reference resolution to the oras repository.
func (s OCIRemoteStore) Resolve(ctx context.Context, ref string) (ociv1.Descriptor, error) {
	return s.Repo.Resolve(ctx, ref)
}

// Fetch delegates blob fetching to the oras repository.
func (s OCIRemoteStore) Fetch(ctx context.Context, desc *ociv1.Descriptor) (io.ReadCloser, error) {
	return s.Repo.Fetch(ctx, *desc)
}

// The default store must satisfy the seam the getter consumes.
var _ OCIRepositoryStore = OCIRemoteStore{}

// ociUserAgent is the versioned User-Agent sent to registries.
func ociUserAgent() string {
	return "terragrunt/" + version.GetVersion()
}

// ociCredentialFactory binds credential resolution to one repository, since the
// most specific ambient entry depends on the repository being pulled.
type ociCredentialFactory func(repositoryName string) auth.CredentialFunc

// NewOCIRepositoryStore returns the default [OCINewStoreFunc]: static
// environment credentials when set, otherwise ambient discovery of Docker and
// containers auth files, and finally the Docker credential helpers those files
// configure. Helpers are re-minted per run, so registries with expiring tokens
// (e.g. Amazon ECR via ecr-login) work without a baked-in login.
func NewOCIRepositoryStore(l log.Logger, v venv.Venv) OCINewStoreFunc {
	v.RequireFS()
	v.RequireEnv()
	v.RequireGOOS()
	v.RequireUserHomeDir()
	v.RequireExec()

	resolveCredential := sync.OnceValues(func() (ociCredentialFactory, error) {
		return ociCredentialFunc(l, v)
	})
	caches := &ociCacheSet{caches: map[string]auth.Cache{}}

	return func(_ context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
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

		return OCIRemoteStore{Repo: repo}, nil
	}
}

// ociCredentialFunc resolves static environment credentials, falling back to
// ambient discovery through the Venv.
func ociCredentialFunc(l log.Logger, v venv.Venv) (ociCredentialFactory, error) {
	staticCred, found, err := ociStaticCredential(v)
	if err != nil {
		return nil, err
	}

	ambient := ociAmbientCredentialFunc(l, v)

	if !found {
		return ambient, nil
	}

	if v.Env[EnvOCIRegistry] == "" {
		// Unscoped: the credential applies to every registry the run contacts.
		return func(_ string) auth.CredentialFunc {
			return func(_ context.Context, _ string) (auth.Credential, error) {
				return staticCred, nil
			}
		}, nil
	}

	// Scoped: static serves its registry, the rest still resolve ambient. The
	// env value is canonicalized like ambient keys so spellings such as
	// https://ghcr.io or a trailing slash match the requested host.
	scopedRegistry := ociCanonicalAuthKey(v.Env[EnvOCIRegistry])

	return func(repositoryName string) auth.CredentialFunc {
		ambientFn := ambient(repositoryName)

		return func(ctx context.Context, hostport string) (auth.Credential, error) {
			if ociCanonicalAuthKey(hostport) == scopedRegistry {
				return staticCred, nil
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

// ociHelperEntry is a resolved credential helper: its binary suffix, the server
// address to hand it (the spelling that stored the credentials), and whether it
// was chosen explicitly per-registry.
type ociHelperEntry struct {
	suffix        string
	serverAddress string
	// explicit is true for a per-registry credHelpers entry, false for the
	// global credsStore whose failures must not break unrelated pulls.
	explicit bool
}

// ociAmbientStore keeps one Venv-read auth file and its canonical key index.
type ociAmbientStore struct {
	auths map[string]json.RawMessage
	// declared maps a lookup key to every spelling the file uses for it. Only
	// keys present here are ever requested, because a hostname-only lookup can
	// return an arbitrary repository-scoped entry when no exact key exists.
	declared map[string][]string
	// credHelpers maps a canonical registry key to its per-registry helper.
	credHelpers map[string]ociHelperEntry
	// credsStore is the default helper suffix for hosts without a credHelpers entry.
	credsStore string
	path       string
}

// helperFor returns the helper serving hostport and the server address to hand
// it: the declared credHelpers spelling for a per-registry entry, else the
// requested host (folded to Docker's index server for Docker Hub) for credsStore.
func (s ociAmbientStore) helperFor(hostport string) (entry ociHelperEntry, ok bool) {
	if entry, ok := s.credHelpers[ociCanonicalAuthKey(hostport)]; ok {
		return entry, true
	}

	if s.credsStore == "" {
		return ociHelperEntry{}, false
	}

	return ociHelperEntry{suffix: s.credsStore, serverAddress: ociHelperServerAddress(hostport)}, true
}

// ociCredentialFromHelpers dispatches the first store helper that serves host.
// An explicit per-registry helper's failure propagates so a misconfiguration
// surfaces as an actionable error; the global credsStore is best-effort, so its
// failure is logged and skipped rather than breaking unrelated pulls.
func ociCredentialFromHelpers(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	stores []ociAmbientStore,
	hostport string,
) (auth.Credential, error) {
	for _, ambient := range stores {
		entry, ok := ambient.helperFor(hostport)
		if !ok {
			continue
		}

		cred, err := ociCredentialFromHelper(ctx, v, entry)
		if err != nil {
			if entry.explicit {
				return auth.EmptyCredential, err
			}

			l.Warnf("Skipping OCI credsStore helper %q for %s: %v", entry.suffix, hostport, err)

			continue
		}

		if cred != auth.EmptyCredential {
			return cred, nil
		}
	}

	return auth.EmptyCredential, nil
}

// ociHelperServerAddress folds a Docker Hub host to the index server helpers
// store Hub credentials under, and returns other hosts unchanged.
func ociHelperServerAddress(hostport string) string {
	if ociCanonicalAuthKey(hostport) == ociDockerHubKey {
		return ociDockerHubIndexServer
	}

	return hostport
}

// ociAmbientCredentialFunc consults ambient files in order. Each file is read
// only through v.FS, and only the selected entry is decoded, so one malformed
// entry cannot hide valid credentials elsewhere in the same file.
func ociAmbientCredentialFunc(l log.Logger, v venv.Venv) ociCredentialFactory {
	stores := loadOCIAmbientStores(l, v)

	return func(repositoryName string) auth.CredentialFunc {
		return func(ctx context.Context, hostport string) (auth.Credential, error) {
			// Helpers win over inline auths (Docker precedence), so a re-minting
			// helper is not shadowed by a stale plaintext login.
			cred, err := ociCredentialFromHelpers(ctx, l, v, stores, hostport)
			if err != nil || cred != auth.EmptyCredential {
				return cred, err
			}

			// Most specific inline key first, so a repository-scoped entry beats
			// a registry-wide one and no other namespace's entry can answer.
			for _, key := range ociCredentialKeys(hostport, repositoryName) {
				for _, ambient := range stores {
					if cred := ociCredentialFromAmbientStore(ctx, l, ambient, key); cred != auth.EmptyCredential {
						return cred, nil
					}
				}
			}

			return auth.EmptyCredential, nil
		}
	}
}

// ociCredentialFromHelper runs docker-credential-<helper> get for the entry's
// server address, under v.Env so process-scoped credentials (e.g. an assumed
// AWS role for ecr-login) reach the helper. A not-in-keychain result is empty,
// not an error. The helper's secret output is never placed in a returned error.
func ociCredentialFromHelper(ctx context.Context, v venv.Venv, entry ociHelperEntry) (auth.Credential, error) {
	bin := ociCredentialHelperPrefix + entry.suffix

	if _, err := v.Exec.LookPath(bin); err != nil {
		return auth.EmptyCredential, OCICredentialHelperError{Helper: entry.suffix, Registry: entry.serverAddress, Err: err}
	}

	ctx, cancel := context.WithTimeout(ctx, ociCredentialHelperTimeout)
	defer cancel()

	cmd := v.Exec.Command(ctx, bin, "get")
	cmd.SetStdin(strings.NewReader(entry.serverAddress))
	cmd.SetEnv(ociEnvSlice(v.Env))

	out, err := cmd.Output()
	if err != nil {
		// Helpers report not-found on stdout and exit non-zero, matching the
		// docker-credential protocol; that case is empty, not an error.
		if strings.TrimSpace(string(out)) == ociHelperCredentialsNotFound {
			return auth.EmptyCredential, nil
		}

		return auth.EmptyCredential, OCICredentialHelperError{Helper: entry.suffix, Registry: entry.serverAddress, Err: err}
	}

	return ociCredentialFromHelperOutput(entry, out)
}

// ociEnvSlice renders env as a deterministic KEY=VALUE slice for a subprocess.
func ociEnvSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}

	out := make([]string, 0, len(env))
	for _, key := range slices.Sorted(maps.Keys(env)) {
		out = append(out, key+"="+env[key])
	}

	return out
}

// ociCredentialFromHelperOutput decodes docker-credential-helper get output.
// A "<token>" username marks a bearer token, matching the helper protocol.
func ociCredentialFromHelperOutput(entry ociHelperEntry, out []byte) (auth.Credential, error) {
	var decoded struct {
		Username string `json:"Username"`
		Secret   string `json:"Secret"`
	}

	if err := json.Unmarshal(out, &decoded); err != nil {
		// The raw output holds the secret, so wrap a sentinel, never the bytes.
		return auth.EmptyCredential, OCICredentialHelperError{
			Helper:   entry.suffix,
			Registry: entry.serverAddress,
			Err:      ErrOCIHelperMalformedOutput,
		}
	}

	if decoded.Username == ociHelperTokenUsername {
		return auth.Credential{RefreshToken: decoded.Secret}, nil
	}

	return auth.Credential{Username: decoded.Username, Password: decoded.Secret}, nil
}

func loadOCIAmbientStores(l log.Logger, v venv.Venv) []ociAmbientStore {
	candidates := ociAmbientCredentialPaths(v)
	stores := make([]ociAmbientStore, 0, len(candidates))

	for _, candidate := range candidates {
		file, err := ociAuthFile(v, candidate)
		if err != nil {
			if !errors.Is(err, iofs.ErrNotExist) {
				l.Warnf("Skipping unparseable OCI credential file %s: %v", candidate, err)
			}

			continue
		}

		stores = append(stores, ociAmbientStore{
			auths:       file.auths,
			declared:    file.declared,
			credHelpers: file.credHelpers,
			credsStore:  file.credsStore,
			path:        candidate,
		})
	}

	return stores
}

func ociCredentialFromAmbientStore(
	ctx context.Context,
	l log.Logger,
	ambient ociAmbientStore,
	key string,
) auth.Credential {
	for _, declared := range ambient.declared[key] {
		cred, err := ociCredentialFromAuthConfig(ctx, ambient.auths[declared])
		if err != nil {
			// The decoder error can contain credential material, so do not log it.
			l.Warnf("Skipping unusable OCI credential entry for %s in %s", declared, ambient.path)

			continue
		}

		if cred != auth.EmptyCredential {
			return cred
		}
	}

	return auth.EmptyCredential
}

// ociAmbientCredentialPaths returns OpenTofu's containers-auth candidates in
// precedence order. The runtime file is Linux-only; macOS and Windows search
// literal ~/.config before XDG config; DOCKER_CONFIG is intentionally ignored.
func ociAmbientCredentialPaths(v venv.Venv) []string {
	var paths []string

	if v.Platform.GOOS == "linux" {
		if dir := v.Env["XDG_RUNTIME_DIR"]; dir != "" {
			paths = append(paths, filepath.Join(dir, "containers", "auth.json"))
		}
	}

	home, err := v.Platform.UserHomeDir()
	if err != nil {
		home = ""
	}

	if (v.Platform.GOOS == "windows" || v.Platform.GOOS == "darwin") && home != "" {
		paths = append(paths, filepath.Join(home, ".config", "containers", "auth.json"))
	}

	configDir := v.Env["XDG_CONFIG_HOME"]
	if configDir == "" && home != "" {
		configDir = filepath.Join(home, ".config")
	}

	if configDir != "" {
		paths = append(paths, filepath.Join(configDir, "containers", "auth.json"))
	}

	if home != "" {
		paths = append(paths, filepath.Join(home, ".docker", "config.json"))
	}

	return slices.Compact(paths)
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
// the resolver matches on, so the Docker Hub spellings and a legacy config's
// scheme all meet in one place.
func ociCanonicalAuthKey(key string) string {
	stripped := strings.TrimPrefix(key, "https://")
	stripped = strings.TrimPrefix(stripped, "http://")
	stripped = strings.TrimRight(stripped, "/")

	if stripped == "index.docker.io/v1" {
		return ociDockerHubKey
	}

	registry, repository, found := strings.Cut(stripped, "/")
	if slices.Contains(ociDockerHubRegistries, registry) {
		registry = ociDockerHubKey
	}

	if !found || repository == "" {
		return registry
	}

	return registry + "/" + repository
}

// ociAuthFile reads an auth file through v.FS and builds its exact-key index.
func ociAuthFile(v venv.Venv, path string) (authFile, error) {
	data, err := vfs.ReadFile(v.FS, path)
	if err != nil {
		return authFile{}, err
	}

	var file struct {
		Auths       map[string]json.RawMessage `json:"auths"`
		CredHelpers map[string]string          `json:"credHelpers"`
		CredsStore  string                     `json:"credsStore"`
	}

	if err := json.Unmarshal(data, &file); err != nil {
		return authFile{}, err
	}

	declaredKeys := make(map[string][]string, len(file.Auths))

	// Sorted, so multiple spellings of one key resolve the same way on every run.
	for _, declared := range slices.Sorted(maps.Keys(file.Auths)) {
		canonical := ociCanonicalAuthKey(declared)
		declaredKeys[canonical] = append(declaredKeys[canonical], declared)
	}

	credHelpers := make(map[string]ociHelperEntry, len(file.CredHelpers))
	// The declared spelling is the server address helpers store credentials
	// under (e.g. https://index.docker.io/v1/), so keep it, not the canonical key.
	for _, declared := range slices.Sorted(maps.Keys(file.CredHelpers)) {
		credHelpers[ociCanonicalAuthKey(declared)] = ociHelperEntry{
			suffix:        file.CredHelpers[declared],
			serverAddress: declared,
			explicit:      true,
		}
	}

	return authFile{auths: file.Auths, declared: declaredKeys, credHelpers: credHelpers, credsStore: file.CredsStore}, nil
}

// authFile is one parsed Docker/containers auth file.
type authFile struct {
	auths       map[string]json.RawMessage
	declared    map[string][]string
	credHelpers map[string]ociHelperEntry
	credsStore  string
}

// ociCredentialFromAuthConfig delegates one selected entry to ORAS's Docker
// config decoder without letting ORAS normalize the file's real lookup key.
func ociCredentialFromAuthConfig(ctx context.Context, raw json.RawMessage) (auth.Credential, error) {
	const syntheticKey = "terragrunt.invalid"

	data, err := json.Marshal(struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}{
		Auths: map[string]json.RawMessage{syntheticKey: raw},
	})
	if err != nil {
		return auth.EmptyCredential, err
	}

	store, err := credentials.NewMemoryStoreFromDockerConfig(data)
	if err != nil {
		return auth.EmptyCredential, err
	}

	return store.Get(ctx, syntheticKey)
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
