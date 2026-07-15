package getter

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"net/http"
	"path/filepath"
	"runtime"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// Interim static-credential environment variables for oci:// sources,
// applying to every registry. The TG_TMP_ prefix marks them as a temporary
// surface: do NOT rely on them; the durable configured-credential surface is
// a pending design decision and these will be replaced by it.
const (
	EnvOCIUsername = "TG_TMP_OCI_USERNAME"
	EnvOCIPassword = "TG_TMP_OCI_PASSWORD"
	EnvOCIToken    = "TG_TMP_OCI_TOKEN"
)

// ociUserAgent identifies Terragrunt to registry operators.
const ociUserAgent = "terragrunt"

// ErrOCIStaticCredentialConflict reports both a token and a username or
// password set in the static-credential environment.
var ErrOCIStaticCredentialConflict = errors.New("cannot set both a token and a username or password for oci sources")

// ErrOCIStaticCredentialIncomplete reports a username without a password, or
// the reverse, in the static-credential environment.
var ErrOCIStaticCredentialIncomplete = errors.New("oci static credentials require both a username and a password")

// The default store must satisfy the seam the getter consumes.
var _ OCIRepositoryStore = (*remote.Repository)(nil)

// NewOCIRepositoryStore returns the default Tier-1 [OCINewStoreFunc]: static
// credentials from v.Env when set, otherwise read-only ambient discovery of
// Docker and containers auth files through v. It never invokes credential
// helpers, so registries needing per-run token minting (e.g. Amazon ECR)
// only work for the lifetime of an externally obtained login in one of the
// ambient files. Ambient files that exist but cannot be used are skipped
// with a warning, so one corrupt file never breaks downloads that need
// weaker or no credentials.
func NewOCIRepositoryStore(l log.Logger, v venv.Venv) OCINewStoreFunc {
	return func(_ context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
		credentialFn, err := ociCredentialFunc(l, v)
		if err != nil {
			return nil, err
		}

		repo, err := remote.NewRepository(registryDomain + "/" + repositoryName)
		if err != nil {
			return nil, fmt.Errorf("parsing OCI repository reference: %w", err)
		}

		repo.Client = &auth.Client{
			Client:     retry.DefaultClient,
			Cache:      auth.NewCache(),
			Credential: credentialFn,
			Header:     http.Header{"User-Agent": []string{ociUserAgent}},
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
		return func(_ context.Context, _ string) (auth.Credential, error) {
			return staticCred, nil
		}, nil
	}

	return ociAmbientCredentialFunc(l, v), nil
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

// ociAmbientCredentialFunc chains read-only file stores over the ambient
// credential files that exist, in [ociAmbientCredentialPaths] order. File
// stores parse credentials only; credential helpers are never invoked. A
// candidate that is missing is skipped silently; one that exists but cannot
// be read or parsed is skipped with a warning, so a corrupt file lower in
// the chain never breaks anonymous pulls or higher-priority credentials.
func ociAmbientCredentialFunc(l log.Logger, v venv.Venv) auth.CredentialFunc {
	candidates := ociAmbientCredentialPaths(v.Env)
	stores := make([]credentials.Store, 0, len(candidates))

	for _, path := range candidates {
		if _, err := v.FS.Stat(path); err != nil {
			if !errors.Is(err, iofs.ErrNotExist) {
				l.Warnf("Skipping OCI credential file %s: %v", path, err)
			}

			continue
		}

		store, err := credentials.NewFileStore(path)
		if err != nil {
			l.Warnf("Skipping unparseable OCI credential file %s: %v", path, err)

			continue
		}

		stores = append(stores, store)
	}

	if len(stores) == 0 {
		return func(_ context.Context, _ string) (auth.Credential, error) {
			return auth.EmptyCredential, nil
		}
	}

	return credentials.Credential(credentials.NewStoreWithFallbacks(stores[0], stores[1:]...))
}

// ociAmbientCredentialPaths returns the ambient credential file candidates
// in the containers-auth search order OpenTofu follows, part of the
// portability contract: a source string authenticating via ambient files
// must resolve the same credentials under tofu and Terragrunt. The runtime
// directory applies to Linux only, a set XDG_CONFIG_HOME replaces the
// default ~/.config location, and a set DOCKER_CONFIG replaces the default
// ~/.docker location.
func ociAmbientCredentialPaths(env map[string]string) []string {
	var paths []string

	if runtime.GOOS == "linux" {
		if dir := env["XDG_RUNTIME_DIR"]; dir != "" {
			paths = append(paths, filepath.Join(dir, "containers", "auth.json"))
		}
	}

	home := ociUserHome(env)

	configDir := env["XDG_CONFIG_HOME"]
	if configDir == "" && home != "" {
		configDir = filepath.Join(home, ".config")
	}

	if configDir != "" {
		paths = append(paths, filepath.Join(configDir, "containers", "auth.json"))
	}

	dockerConfigDir := env["DOCKER_CONFIG"]
	if dockerConfigDir == "" && home != "" {
		dockerConfigDir = filepath.Join(home, ".docker")
	}

	if dockerConfigDir != "" {
		paths = append(paths, filepath.Join(dockerConfigDir, "config.json"))
	}

	return paths
}

// ociUserHome resolves the home directory from env: HOME on Unix,
// USERPROFILE on Windows.
func ociUserHome(env map[string]string) string {
	if home := env["HOME"]; home != "" {
		return home
	}

	return env["USERPROFILE"]
}
