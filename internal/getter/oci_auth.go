package getter

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"net/http"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/version"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// Interim static-credential environment variables for oci:// sources. The
// TG_TMP_ prefix marks them as a temporary, undocumented surface: do NOT
// rely on them; the durable configured-credential surface is a pending
// design decision and these will be replaced by it. When TG_TMP_OCI_REGISTRY
// is unset the credentials are sent to EVERY registry the run contacts, so a
// token can leave the intended host; set TG_TMP_OCI_REGISTRY to scope them.
const (
	EnvOCIUsername = "TG_TMP_OCI_USERNAME"
	EnvOCIPassword = "TG_TMP_OCI_PASSWORD"
	EnvOCIToken    = "TG_TMP_OCI_TOKEN"
	EnvOCIRegistry = "TG_TMP_OCI_REGISTRY"
)

// ErrOCIStaticCredentialConflict reports both a token and a username or
// password set in the static-credential environment.
var ErrOCIStaticCredentialConflict = errors.New("cannot set both a token and a username or password for oci sources")

// ErrOCIStaticCredentialIncomplete reports a username without a password, or
// the reverse, in the static-credential environment.
var ErrOCIStaticCredentialIncomplete = errors.New("oci static credentials require both a username and a password")

// The default store must satisfy the seam the getter consumes.
var _ OCIRepositoryStore = (*remote.Repository)(nil)

// ociUserAgent identifies Terragrunt to registry operators, versioned so
// traffic can be correlated to a release.
func ociUserAgent() string {
	return "terragrunt/" + version.GetVersion()
}

// NewOCIRepositoryStore returns the default Tier-1 [OCINewStoreFunc]: static
// credentials from v.Env when set, otherwise read-only ambient discovery of
// Docker and containers auth files through v. It never invokes credential
// helpers, so registries needing per-run token minting (e.g. Amazon ECR)
// only work for the lifetime of an externally obtained login in one of the
// ambient files. Ambient files that exist but cannot be used are skipped
// with a warning, so one corrupt file never breaks downloads that need
// weaker or no credentials.
//
// Credential discovery and the auth-token cache are resolved once and shared
// across every store this run constructs, so a run --all does not re-read the
// files or re-mint tokens per unit.
func NewOCIRepositoryStore(l log.Logger, v venv.Venv) OCINewStoreFunc {
	resolveCredential := sync.OnceValues(func() (auth.CredentialFunc, error) {
		return ociCredentialFunc(l, v)
	})
	cache := auth.NewCache()

	return func(_ context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
		credentialFn, err := resolveCredential()
		if err != nil {
			return nil, err
		}

		repo, err := remote.NewRepository(registryDomain + "/" + repositoryName)
		if err != nil {
			return nil, fmt.Errorf("parsing OCI repository reference: %w", err)
		}

		repo.Client = &auth.Client{
			Client:     retry.DefaultClient,
			Cache:      cache,
			Credential: credentialFn,
			Header:     http.Header{"User-Agent": []string{ociUserAgent()}},
		}

		return repo, nil
	}
}

// ociCredentialFunc resolves the credential source once: static credentials
// from the environment take precedence over ambient discovery.
func ociCredentialFunc(l log.Logger, v venv.Venv) (auth.CredentialFunc, error) {
	staticCred, found, err := ociStaticCredential(v.Env)
	if err != nil {
		return nil, err
	}

	if found {
		// Scope the credential to TG_TMP_OCI_REGISTRY when set so a token is
		// not offered to every registry the run touches; otherwise apply it
		// unconditionally (the documented interim behavior).
		if registry := v.Env[EnvOCIRegistry]; registry != "" {
			return auth.StaticCredential(registry, staticCred), nil
		}

		return func(_ context.Context, _ string) (auth.Credential, error) {
			return staticCred, nil
		}, nil
	}

	return ociAmbientCredentialFunc(l, v, runtime.GOOS), nil
}

// ociStaticCredential reads the interim static credentials from env: either
// a token or a username plus password.
func ociStaticCredential(env map[string]string) (auth.Credential, bool, error) {
	username := env[EnvOCIUsername]
	password := env[EnvOCIPassword]
	token := env[EnvOCIToken]

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

// ociAmbientStore pairs a credential store with the file it was loaded from
// so warnings can name the offending path.
type ociAmbientStore struct {
	store credentials.Store
	path  string
}

// ociAmbientCredentialFunc opens the ambient credential files that exist, in
// [ociAmbientCredentialPaths] order, and returns a credential function that
// consults them most-recent-first. File stores parse credentials only;
// credential helpers are never invoked. A missing candidate is skipped
// silently; one that cannot be opened, or whose entry for the requested
// registry is malformed, is skipped so a corrupt file never breaks anonymous
// pulls or higher-priority credentials.
func ociAmbientCredentialFunc(l log.Logger, v venv.Venv, goos string) auth.CredentialFunc {
	candidates := ociAmbientCredentialPaths(v, goos)
	stores := make([]ociAmbientStore, 0, len(candidates))

	for _, candidate := range candidates {
		if _, err := v.FS.Stat(candidate); err != nil {
			if !errors.Is(err, iofs.ErrNotExist) {
				l.Warnf("Skipping OCI credential file %s: %v", candidate, err)
			}

			continue
		}

		store, err := credentials.NewFileStore(candidate)
		if err != nil {
			l.Warnf("Skipping unparseable OCI credential file %s: %v", candidate, err)

			continue
		}

		stores = append(stores, ociAmbientStore{store: store, path: candidate})
	}

	return func(ctx context.Context, hostport string) (auth.Credential, error) {
		registry := credentials.ServerAddressFromHostname(hostport)
		if registry == "" {
			return auth.EmptyCredential, nil
		}

		for _, ambient := range stores {
			// A per-file lookup error means this file's entry for the
			// registry is malformed; warn without the error value (it can
			// echo decoded credential material) and fall through.
			cred, err := ambient.store.Get(ctx, registry)
			if err != nil {
				l.Warnf("Skipping unusable OCI credential entry for %s in %s", registry, ambient.path)

				continue
			}

			if cred != auth.EmptyCredential {
				return cred, nil
			}
		}

		return auth.EmptyCredential, nil
	}
}

// ociAmbientCredentialPaths returns the ambient credential file candidates in
// the containers-auth search order OpenTofu uses, so a source authenticating
// via ambient files resolves the same credentials under tofu and Terragrunt
// on the given goos. goos is injected (rather than read from runtime.GOOS) so
// the list is testable for every platform from any host.
//
// This is best-effort parity, not an exact contract. Known divergences from
// tofu, tracked separately: legacy $HOME/.dockercfg (bare-map format oras
// cannot parse) is not searched; credential helpers and a global credsStore
// are not honored; and lookups are domain-scoped rather than tofu's
// most-specific repository-path match.
func ociAmbientCredentialPaths(v venv.Venv, goos string) []string {
	var paths []string

	// Linux only: the containers runtime auth file.
	if goos == "linux" {
		if dir := v.Env["XDG_RUNTIME_DIR"]; dir != "" {
			paths = append(paths, filepath.Join(dir, "containers", "auth.json"))
		}
	}

	home := ociUserHome(v, goos)

	// Windows and macOS: the literal ~/.config location, always searched
	// even when XDG_CONFIG_HOME below redirects the default.
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

	// The literal Docker CLI config location. DOCKER_CONFIG is intentionally
	// not honored, matching tofu.
	if home != "" {
		paths = append(paths, filepath.Join(home, ".docker", "config.json"))
	}

	return paths
}

// ociUserHome resolves the home directory from v.Env the way Go's
// os.UserHomeDir does: USERPROFILE on Windows, HOME elsewhere.
func ociUserHome(v venv.Venv, goos string) string {
	if goos == "windows" {
		return v.Env["USERPROFILE"]
	}

	return v.Env["HOME"]
}
