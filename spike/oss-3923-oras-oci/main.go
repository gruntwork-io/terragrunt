// Spike for OSS-3923: prove oras.land/oras-go/v2 reproduces OpenTofu 1.10's
// observable OCI module contract, clean-room, behind the exact
// OCIRepositoryStore/NewStore seam planned in OSS-3927.
//
// Throwaway code: nothing here is merged under internal/.
package main

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

// Contract constants: must equal internal/getter/oci_contract.go.
const (
	artifactTypeModulePkg = "application/vnd.opentofu.modulepkg"
	mediaTypeModuleZip    = "archive/zip"
)

// OCIRepositoryStore is the EXACT seam interface from OSS-3927.
type OCIRepositoryStore interface {
	Resolve(ctx context.Context, ref string) (ocispec.Descriptor, error)
	Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error)
}

// NewStoreFunc is the EXACT injection point from OSS-3927.
type NewStoreFunc func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error)

// Compile-time proof that oras-go's remote.Repository satisfies the seam as-is.
var _ OCIRepositoryStore = (*remote.Repository)(nil)

// Compile-time proof the seam doubles as oras-go's content.Fetcher,
// so content.FetchAll / content.NewVerifyReader work through it.
var _ content.Fetcher = (OCIRepositoryStore)(nil)

func newORASStore(caFile string) NewStoreFunc {
	return func(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error) {
		repo, err := remote.NewRepository(registryDomain + "/" + repositoryName)
		if err != nil {
			return nil, fmt.Errorf("creating repository ref: %w", err)
		}

		httpClient := http.DefaultClient
		if caFile != "" {
			pem, err := os.ReadFile(caFile)
			if err != nil {
				return nil, fmt.Errorf("reading CA file: %w", err)
			}
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(pem)
			httpClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
		}

		// Tier-1 ambient credentials. Default mode reads only Docker config
		// (proven NOT to match OpenTofu's search order); -ambient-order mode
		// builds the OpenTofu-order multi-path store via NewStoreWithFallbacks.
		credStore, err := buildCredStore()
		if err != nil {
			return nil, fmt.Errorf("loading credential store: %w", err)
		}

		repo.Client = &auth.Client{
			Client:     httpClient,
			Cache:      auth.NewCache(),
			Credential: credentials.Credential(credStore),
		}
		return repo, nil
	}
}

// pull mirrors the planned OCIGetter.Get flow from OSS-3928.
func pull(ctx context.Context, newStore NewStoreFunc, registryDomain, repositoryName, ref, dst string) (map[string]any, error) {
	report := map[string]any{}

	store, err := newStore(ctx, registryDomain, repositoryName)
	if err != nil {
		return nil, err
	}

	// 1. Resolve tag/digest -> manifest descriptor.
	desc, err := store.Resolve(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", ref, err)
	}
	report["manifestDigest"] = desc.Digest.String()
	report["manifestMediaType"] = desc.MediaType

	// 2. Fetch + parse the OCI image manifest (digest+size verified by FetchAll).
	manifestBytes, err := content.FetchAll(ctx, store, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest: %w", err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	// 3. Contract: artifact type must be the OpenTofu module package type.
	report["artifactType"] = manifest.ArtifactType
	if manifest.ArtifactType != artifactTypeModulePkg {
		return report, fmt.Errorf("artifact type %q is not %q", manifest.ArtifactType, artifactTypeModulePkg)
	}

	// 4. Contract: exactly one archive/zip layer.
	var zipLayers []ocispec.Descriptor
	for _, layer := range manifest.Layers {
		if layer.MediaType == mediaTypeModuleZip {
			zipLayers = append(zipLayers, layer)
		}
	}
	report["zipLayerCount"] = len(zipLayers)
	if len(zipLayers) != 1 {
		return report, fmt.Errorf("expected exactly one %q layer, found %d", mediaTypeModuleZip, len(zipLayers))
	}
	layer := zipLayers[0]
	report["layerDigest"] = layer.Digest.String()

	// 5. Stream the blob with go-digest verification (content.NewVerifyReader).
	rc, err := store.Fetch(ctx, layer)
	if err != nil {
		return nil, fmt.Errorf("fetching layer: %w", err)
	}
	defer rc.Close()

	tmpZip, err := os.CreateTemp("", "spike-module-*.zip")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpZip.Name())

	vr := content.NewVerifyReader(rc, layer)
	if _, err := io.Copy(tmpZip, vr); err != nil {
		return nil, fmt.Errorf("streaming blob: %w", err)
	}
	if err := vr.Verify(); err != nil {
		return report, fmt.Errorf("digest verification: %w", err)
	}
	report["digestVerified"] = true
	if err := tmpZip.Close(); err != nil {
		return nil, err
	}

	// 6. Extract into dst.
	if err := unzip(tmpZip.Name(), dst); err != nil {
		return nil, fmt.Errorf("extracting: %w", err)
	}
	report["extractedTo"] = dst

	return report, nil
}

func main() {
	var (
		domain  = flag.String("domain", "127.0.0.1:5000", "registry domain (host:port)")
		repo    = flag.String("repo", "terraform-modules/vpc", "repository name (multi-segment kept whole)")
		ref     = flag.String("ref", "1.0.0", "tag or sha256:... digest")
		dst     = flag.String("dst", "", "extraction dir")
		caFile  = flag.String("ca", "", "PEM CA file for the registry TLS cert")
		negCase = flag.Bool("errdef-check", false, "resolve a missing tag and classify via errdef.ErrNotFound")
	)
	flag.BoolVar(&ambientOrder, "ambient-order", false, "use OpenTofu-order multi-path ambient credential discovery")
	flag.Parse()
	ctx := context.Background()

	newStore := newORASStore(*caFile)

	if *negCase {
		store, err := newStore(ctx, *domain, *repo)
		if err != nil {
			fatal(err)
		}
		_, err = store.Resolve(ctx, "no-such-tag")
		fmt.Printf("errdef check: err=%v\n", err)
		fmt.Printf("errors.Is(err, errdef.ErrNotFound) = %v\n", errors.Is(err, errdef.ErrNotFound))
		return
	}

	report, err := pull(ctx, newStore, *domain, *repo, *ref, *dst)
	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(out))
	if err != nil {
		fatal(err)
	}
}

// ambientOrder toggles OpenTofu-order multi-path ambient discovery.
var ambientOrder bool

// buildCredStore returns either the stock Docker-config store or a
// fallback chain over OpenTofu's ambient search order:
// $XDG_RUNTIME_DIR/containers/auth.json, $HOME/.config/containers/auth.json,
// $XDG_CONFIG_HOME/containers/auth.json, $HOME/.docker/config.json.
func buildCredStore() (credentials.Store, error) {
	if !ambientOrder {
		return credentials.NewStoreFromDocker(credentials.StoreOptions{})
	}

	var candidates []string
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "containers", "auth.json"))
	}
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, filepath.Join(home, ".config", "containers", "auth.json"))
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "containers", "auth.json"))
	}
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, filepath.Join(home, ".docker", "config.json"))
	}

	var stores []credentials.Store
	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		s, err := credentials.NewStore(path, credentials.StoreOptions{})
		if err != nil {
			return nil, fmt.Errorf("opening credential file %s: %w", path, err)
		}
		stores = append(stores, s)
	}
	if len(stores) == 0 {
		return credentials.NewStoreFromDocker(credentials.StoreOptions{})
	}
	return credentials.NewStoreWithFallbacks(stores[0], stores[1:]...), nil
}

func unzip(zipPath, dst string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		target := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("zip slip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			rc.Close()
			w.Close()
			return err
		}
		rc.Close()
		if err := w.Close(); err != nil {
			return err
		}
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
	os.Exit(1)
}
