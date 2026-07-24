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

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
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
	// ociCredentialHelperWaitDelay bounds the post-cancel drain of a helper's output pipes.
	ociCredentialHelperWaitDelay = 5 * time.Second
	// ociHelperStderrMax caps the helper stderr captured for diagnostics.
	ociHelperStderrMax = 2048
	// ociDockerHubIndexServer is the server address helpers store Docker Hub creds under.
	ociDockerHubIndexServer = "https://index.docker.io/v1/"
)

// OCICredentialHelperError reports a helper failure, carrying stderr diagnostics but never the secret stdout.
type OCICredentialHelperError struct {
	Err      error
	Helper   string
	Registry string
	Stderr   string
}

func (err OCICredentialHelperError) Error() string {
	if err.Stderr != "" {
		return fmt.Sprintf("oci credential helper %q for %s: %v: %s", err.Helper, err.Registry, err.Err, err.Stderr)
	}

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
// environment credentials when set, otherwise the most specific ambient source
// across Docker and containers auth files, running a configured credential
// helper (e.g. Amazon ECR via ecr-login) only when it is the selected source.
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

// ociHelperEntry is a resolved credential helper (suffix, server address, explicit).
type ociHelperEntry struct {
	suffix        string
	serverAddress string
	explicit      bool
}

// ociAmbientStore is one Venv-read auth file and its canonical key index.
type ociAmbientStore struct {
	auths       map[string]json.RawMessage
	declared    map[string][]string
	credHelpers map[string]ociHelperEntry
	credsStore  string
	path        string
}

// Ambient credential specificity: repository-path beats domain, which beats global credsStore.
const (
	ociGlobalSpecificity = 1
	ociDomainSpecificity = 2
)

// ociInlineSpecificity returns an inline key's specificity: domain plus one per matched repository-path segment.
func ociInlineSpecificity(key string) int {
	return ociDomainSpecificity + strings.Count(key, "/")
}

// ociCredentialCandidate is a ranked ambient source whose helper runs only if the candidate wins selection.
type ociCredentialCandidate struct {
	helper      *ociHelperEntry
	static      auth.Credential
	specificity int
}

// ociOutranks reports whether the candidate replaces best, a helper beating an inline auth at equal specificity.
func ociOutranks(specificity int, isHelper bool, best *ociCredentialCandidate) bool {
	if specificity != best.specificity {
		return specificity > best.specificity
	}

	// A per-registry helper is authoritative over a same-registry inline login, so it wins the equal-specificity tie.
	return isHelper && best.helper == nil && best.specificity > 0
}

// ociSelectCredentialCandidate returns the most specific source, ties broken by discovery order (inline first).
func ociSelectCredentialCandidate(
	ctx context.Context,
	l log.Logger,
	stores []ociAmbientStore,
	hostport, repositoryName string,
) ociCredentialCandidate {
	var best ociCredentialCandidate

	for _, ambient := range stores {
		// The most specific inline key that resolves in this file.
		for _, key := range ociCredentialKeys(hostport, repositoryName) {
			cred := ociCredentialFromAmbientStore(ctx, l, ambient, key)
			if cred == auth.EmptyCredential {
				continue
			}

			if spec := ociInlineSpecificity(key); ociOutranks(spec, false, &best) {
				best = ociCredentialCandidate{static: cred, specificity: spec}
			}

			break
		}

		// A per-registry credHelpers entry wins a tie with a domain inline login.
		entry, hasHelper := ambient.credHelpers[ociCanonicalAuthKey(hostport)]
		if hasHelper && ociOutranks(ociDomainSpecificity, true, &best) {
			winner := entry
			best = ociCredentialCandidate{helper: &winner, specificity: ociDomainSpecificity}
		}

		// The global credsStore is the least specific source.
		if ambient.credsStore != "" && ociOutranks(ociGlobalSpecificity, true, &best) {
			best = ociCredentialCandidate{
				helper:      &ociHelperEntry{suffix: ambient.credsStore, serverAddress: ociHelperServerAddress(hostport)},
				specificity: ociGlobalSpecificity,
			}
		}
	}

	return best
}

// ociExecuteCredentialCandidate runs the winning source now, with an explicit helper fatal and credsStore best-effort.
func ociExecuteCredentialCandidate(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	best *ociCredentialCandidate,
	hostport string,
) (auth.Credential, error) {
	if best.helper == nil {
		return best.static, nil
	}

	cred, err := ociCredentialFromHelper(ctx, v, *best.helper, ociCredentialHelperTimeout)
	if err != nil {
		if best.helper.explicit {
			return auth.EmptyCredential, err
		}

		l.Warnf("Skipping OCI credsStore helper %q for %s: %v", best.helper.suffix, hostport, err)

		return auth.EmptyCredential, nil
	}

	return cred, nil
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
			best := ociSelectCredentialCandidate(ctx, l, stores, hostport, repositoryName)

			return ociExecuteCredentialCandidate(ctx, l, v, &best, hostport)
		}
	}
}

// ociCredentialFromHelper runs docker-credential-<helper> get under v.Env; timeout <= 0 uses the default cap.
func ociCredentialFromHelper(
	ctx context.Context,
	v venv.Venv,
	entry ociHelperEntry,
	timeout time.Duration,
) (auth.Credential, error) {
	bin := ociCredentialHelperPrefix + entry.suffix

	if _, err := v.Exec.LookPath(bin); err != nil {
		return auth.EmptyCredential, OCICredentialHelperError{Helper: entry.suffix, Registry: entry.serverAddress, Err: err}
	}

	if timeout <= 0 {
		timeout = ociCredentialHelperTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := v.Exec.Command(ctx, bin, "get")
	cmd.SetStdin(strings.NewReader(entry.serverAddress))
	cmd.SetEnv(ociEnvSlice(ctx, v.Env))
	cmd.SetWaitDelay(ociCredentialHelperWaitDelay)

	stderr := &boundedWriter{max: ociHelperStderrMax}
	cmd.SetStderr(stderr)

	out, err := cmd.Output()
	if err != nil {
		// Helpers report not-found on stdout and exit non-zero, matching the
		// docker-credential protocol; that case is empty, not an error.
		if strings.TrimSpace(string(out)) == ociHelperCredentialsNotFound {
			return auth.EmptyCredential, nil
		}

		return auth.EmptyCredential, OCICredentialHelperError{
			Helper:   entry.suffix,
			Registry: entry.serverAddress,
			Err:      err,
			Stderr:   strings.TrimSpace(stderr.String()),
		}
	}

	return ociCredentialFromHelperOutput(entry, out)
}

// ociEnvSlice renders env as a non-nil KEY=VALUE slice and injects TRACEPARENT from ctx when present.
func ociEnvSlice(ctx context.Context, env map[string]string) []string {
	n := len(env)
	traceParent := telemetry.TraceParentFromContext(ctx, nil)

	if traceParent != "" {
		if _, ok := env[telemetry.TraceParentEnv]; !ok {
			n++
		}
	}

	out := make([]string, 0, n)

	for _, key := range slices.Sorted(maps.Keys(env)) {
		if key == telemetry.TraceParentEnv && traceParent != "" {
			continue
		}

		out = append(out, key+"="+env[key])
	}

	if traceParent != "" {
		out = append(out, telemetry.TraceParentEnv+"="+traceParent)
	}

	return out
}

// boundedWriter accumulates at most max bytes and discards the rest.
type boundedWriter struct {
	buf       strings.Builder
	max       int
	truncated bool
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	n := len(p)

	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		w.truncated = true

		return n, nil
	}

	if remaining < len(p) {
		p = p[:remaining]
		w.truncated = true
	}

	w.buf.Write(p)

	return n, nil
}

// String returns the captured bytes, marking a trailing ellipsis when capped.
func (w *boundedWriter) String() string {
	if w.truncated {
		return w.buf.String() + "..."
	}

	return w.buf.String()
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
		// An empty helper name is not runnable, so skip it and fall through to credsStore or inline auths.
		suffix := file.CredHelpers[declared]
		if suffix == "" {
			continue
		}

		credHelpers[ociCanonicalAuthKey(declared)] = ociHelperEntry{
			suffix:        suffix,
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
