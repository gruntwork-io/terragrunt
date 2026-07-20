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
var ErrOCIStaticCredentialConflict = errors.New(
	"cannot set both a token and a username or password for oci sources",
)

// ErrOCIStaticCredentialIncomplete reports a username without a password, or the reverse.
var ErrOCIStaticCredentialIncomplete = errors.New(
	"oci static credentials require both a username and a password",
)

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

// NewOCIRepositoryStore returns the default Tier-1 [OCINewStoreFunc]: static
// environment credentials when set, otherwise read-only ambient discovery of
// Docker and containers auth files. It never invokes credential helpers, so
// registries needing per-run token minting (e.g. Amazon ECR) only work for the
// lifetime of an externally obtained login in one of the ambient files.
func NewOCIRepositoryStore(l log.Logger, v venv.Venv) OCINewStoreFunc {
	v.RequireFS()
	v.RequireEnv()
	v.RequireGOOS()
	v.RequireUserHomeDir()
	v.RequireHTTP()

	resolveCredential := sync.OnceValues(func() (ociCredentialFactory, error) {
		return ociCredentialFunc(l, v)
	})
	caches := &ociCacheSet{caches: map[string]auth.Cache{}}

	// Registry requests ride the venv's outbound client instead of ORAS's
	// OS-default retry.DefaultClient; wrapping the venv transport in ORAS's
	// retry policy keeps the same transient-failure behavior.
	httpClient := *v.HTTP
	httpClient.Transport = retry.NewTransport(httpClient.Transport)

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
			Client:     &httpClient,
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

// ociAmbientStore keeps one Venv-read auth file and its canonical key index.
type ociAmbientStore struct {
	auths map[string]json.RawMessage
	// declared maps a lookup key to every spelling the file uses for it. Only
	// keys present here are ever requested, because a hostname-only lookup can
	// return an arbitrary repository-scoped entry when no exact key exists.
	declared map[string][]string
	path     string
}

// ociAmbientCredentialFunc consults ambient files in order. Each file is read
// only through v.FS, and only the selected entry is decoded, so one malformed
// entry cannot hide valid credentials elsewhere in the same file.
func ociAmbientCredentialFunc(l log.Logger, v venv.Venv) ociCredentialFactory {
	stores := loadOCIAmbientStores(l, v)

	return func(repositoryName string) auth.CredentialFunc {
		return func(ctx context.Context, hostport string) (auth.Credential, error) {
			// Most specific key first, so a repository-scoped entry beats a
			// registry-wide one and no other namespace's entry can answer.
			for _, key := range ociCredentialKeys(hostport, repositoryName) {
				for _, ambient := range stores {
					if cred := ociCredentialFromAmbientStore(
						ctx,
						l,
						ambient,
						key,
					); cred != auth.EmptyCredential {
						return cred, nil
					}
				}
			}

			return auth.EmptyCredential, nil
		}
	}
}

func loadOCIAmbientStores(l log.Logger, v venv.Venv) []ociAmbientStore {
	candidates := ociAmbientCredentialPaths(v)
	stores := make([]ociAmbientStore, 0, len(candidates))

	for _, candidate := range candidates {
		auths, declared, err := ociAuthFile(v, candidate)
		if err != nil {
			if !errors.Is(err, iofs.ErrNotExist) {
				l.Warnf("Skipping unparseable OCI credential file %s: %v", candidate, err)
			}

			continue
		}

		stores = append(stores, ociAmbientStore{
			auths:    auths,
			declared: declared,
			path:     candidate,
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
func ociAuthFile(
	v venv.Venv,
	path string,
) (map[string]json.RawMessage, map[string][]string, error) {
	data, err := vfs.ReadFile(v.FS, path)
	if err != nil {
		return nil, nil, err
	}

	var file struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}

	if err := json.Unmarshal(data, &file); err != nil {
		return nil, nil, err
	}

	declaredKeys := make(map[string][]string, len(file.Auths))

	// Sorted, so multiple spellings of one key resolve the same way on every run.
	for _, declared := range slices.Sorted(maps.Keys(file.Auths)) {
		canonical := ociCanonicalAuthKey(declared)
		declaredKeys[canonical] = append(declaredKeys[canonical], declared)
	}

	return file.Auths, declaredKeys, nil
}

// ociCredentialFromAuthConfig delegates one selected entry to ORAS's Docker
// config decoder without letting ORAS normalize the file's real lookup key.
func ociCredentialFromAuthConfig(
	ctx context.Context,
	raw json.RawMessage,
) (auth.Credential, error) {
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
