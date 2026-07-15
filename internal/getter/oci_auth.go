package getter

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"net/http"
	"path/filepath"
	"runtime"
	"slices"
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

// NewOCIRepositoryStore returns the default Tier-1 store: static env creds, else ambient discovery.
func NewOCIRepositoryStore(l log.Logger, v venv.Venv) OCINewStoreFunc {
	resolveCredential := sync.OnceValues(func() (auth.CredentialFunc, error) {
		return ociCredentialFunc(l, v)
	})
	cache := auth.NewCache()

	return func(_ context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
		if !vfs.IsOSFS(v.FS) {
			return nil, ErrOCINonOSFilesystem
		}

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

// ociCredentialFunc resolves static env credentials, falling back to ambient discovery.
func ociCredentialFunc(l log.Logger, v venv.Venv) (auth.CredentialFunc, error) {
	staticCred, found, err := ociStaticCredential(v)
	if err != nil {
		return nil, err
	}

	if !found {
		return ociAmbientCredentialFunc(l, v, runtime.GOOS), nil
	}

	registry := v.Env[EnvOCIRegistry]
	if registry == "" {
		// Unscoped: the credential applies to every registry the run contacts.
		return func(_ context.Context, _ string) (auth.Credential, error) {
			return staticCred, nil
		}, nil
	}

	// Scoped: static serves its registry, the rest still resolve ambient.
	scoped := auth.StaticCredential(registry, staticCred)
	ambient := ociAmbientCredentialFunc(l, v, runtime.GOOS)

	return func(ctx context.Context, hostport string) (auth.Credential, error) {
		cred, err := scoped(ctx, hostport)
		if err != nil {
			return auth.EmptyCredential, err
		}

		if cred != auth.EmptyCredential {
			return cred, nil
		}

		return ambient(ctx, hostport)
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

// ociAmbientStore pairs a credential store with its source file, so warnings can name it.
type ociAmbientStore struct {
	store credentials.Store
	path  string
}

// ociAmbientCredentialFunc consults existing ambient files in order, skipping unusable ones.
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
			// Malformed entry: warn without the error (it can echo decoded secrets) and fall through.
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
