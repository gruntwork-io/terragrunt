//go:build ocilive

package test_test

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

const (
	// ociLivePackCreated pins the packed manifest timestamp, so the fixture digest is reproducible locally.
	ociLivePackCreated = "2026-01-01T00:00:00Z"
	testFixtureOCILive = "fixtures/oci-live"
)

//go:embed fixtures/oci-live
var ociLiveFixtureFS embed.FS

// TestOCILiveECR pulls the published fixture from a real ECR repository through the ecr-login helper.
func TestOCILiveECR(t *testing.T) {
	t.Parallel()

	repository := venv.OSVenv().Env["TG_OCI_TEST_ECR_REPOSITORY"]
	if repository == "" {
		t.Skip("TG_OCI_TEST_ECR_REPOSITORY is required for live test")
	}

	registryHost, _, found := strings.Cut(repository, "/")
	require.True(t, found, "TG_OCI_TEST_ECR_REPOSITORY must be <registry-host>/<repository>")

	ensureOCILiveFixture(t, repository, ecrLoginCredential(t, registryHost))

	// The pull authenticates through the helper named in the ambient Docker config.
	home := t.TempDir()
	linkAWSConfigInto(t, home)

	dockerConfig := `{"credHelpers":{"` + registryHost + `":"ecr-login"}}`

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".docker"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".docker", "config.json"), []byte(dockerConfig), 0o600))

	manifest := ociLiveFixtureManifest(t)
	pullOCILiveModule(t, home, "oci://"+repository+"?tag="+ociLiveFixtureTag(manifest.Digest))
	pullOCILiveModule(t, home, "oci://"+repository+"?digest="+manifest.Digest.String())
}

// TestOCILiveGHCR pulls the published fixture from GHCR through basic credentials in a CLI-config block.
func TestOCILiveGHCR(t *testing.T) {
	t.Parallel()

	env := venv.OSVenv().Env
	repository := env["TG_OCI_TEST_GHCR_REPOSITORY"]
	username := env["TG_OCI_TEST_GHCR_USERNAME"]
	token := env["TG_OCI_TEST_GHCR_TOKEN"]

	if repository == "" || username == "" || token == "" {
		t.Skip("TG_OCI_TEST_GHCR_REPOSITORY, TG_OCI_TEST_GHCR_USERNAME, and TG_OCI_TEST_GHCR_TOKEN are required")
	}

	registryHost, _, found := strings.Cut(repository, "/")
	require.True(t, found, "TG_OCI_TEST_GHCR_REPOSITORY must be <registry-host>/<repository>")

	cred := auth.Credential{Username: username, Password: token}

	ensureOCILiveFixture(t, repository, cred)

	// The pull authenticates through the token in an oci_credentials CLI-config block.
	home := t.TempDir()
	tofurc := `oci_credentials "` + registryHost + `" {
  username = "` + username + `"
  password = "` + token + `"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(home, ".tofurc"), []byte(tofurc), 0o600))

	manifest := ociLiveFixtureManifest(t)
	pullOCILiveModule(t, home, "oci://"+repository+"?tag="+ociLiveFixtureTag(manifest.Digest))
	pullOCILiveModule(t, home, "oci://"+repository+"?digest="+manifest.Digest.String())
}

// pullOCILiveModule downloads src through the production getter chain rooted at home and checks the tree.
func pullOCILiveModule(t *testing.T, home, src string) {
	t.Helper()

	v := venv.OSVenv().WithEnvCloned().WithUserHomeDir(func() (string, error) { return home, nil })
	v.Env["HOME"] = home

	// The hermetic home is the only credential source, so ambient developer config cannot leak in.
	for _, name := range []string{"TF_CLI_CONFIG_FILE", "TERRAFORM_CONFIG", "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR"} {
		delete(v.Env, name)
	}

	dst := filepath.Join(t.TempDir(), "module")
	client := getter.NewClient(v, getter.WithOCI(getter.NewOCIGetter(logger.CreateLogger(), v)))

	_, err := client.Get(t.Context(), &getter.Request{Src: src, Dst: dst})
	require.NoError(t, err)

	for name, content := range ociLiveFixtureFiles(t) {
		data, readErr := os.ReadFile(filepath.Join(dst, filepath.FromSlash(name)))
		require.NoError(t, readErr)
		require.Equal(t, content, data)
	}
}

// ensureOCILiveFixture publishes the fixture when the repository does not serve it yet.
func ensureOCILiveFixture(t *testing.T, repository string, cred auth.Credential) {
	t.Helper()

	registryHost, _, _ := strings.Cut(repository, "/")
	expected := ociLiveFixtureManifest(t)

	repo, err := remote.NewRepository(repository)
	require.NoError(t, err)

	repo.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Cache:      auth.NewCache(),
		Credential: auth.StaticCredential(registryHost, cred),
	}

	tag := ociLiveFixtureTag(expected.Digest)

	published, err := repo.Resolve(t.Context(), tag)
	switch {
	case err == nil && published.Digest == expected.Digest:
		return
	case err == nil, errors.Is(err, errdef.ErrNotFound):
		pushOCILiveFixture(t, repository, cred)
	default:
		require.NoError(t, err, "resolving the fixture from %s must succeed", repository)
	}
}

// ociLiveFixtureStaging packs the fixture into a fresh in-memory store and returns it with the manifest.
func ociLiveFixtureStaging(t *testing.T) (*memory.Store, ociv1.Descriptor) {
	t.Helper()

	staging := memory.New()

	var buf bytes.Buffer

	archive := zip.NewWriter(&buf)
	files := ociLiveFixtureFiles(t)

	// Sorted names keep the zip bytes, and therefore the fixture digest, reproducible.
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}

	slices.Sort(names)

	for _, name := range names {
		entry, err := archive.Create(name)
		require.NoError(t, err)

		_, err = entry.Write(files[name])
		require.NoError(t, err)
	}

	require.NoError(t, archive.Close())

	layer := ociv1.Descriptor{
		MediaType: getter.MediaTypeModuleZip,
		Digest:    digest.FromBytes(buf.Bytes()),
		Size:      int64(buf.Len()),
	}
	require.NoError(t, staging.Push(t.Context(), layer, bytes.NewReader(buf.Bytes())))

	manifest, err := oras.PackManifest(
		t.Context(),
		staging,
		oras.PackManifestVersion1_1,
		getter.ArtifactTypeModulePkg,
		oras.PackManifestOptions{
			Layers:              []ociv1.Descriptor{layer},
			ManifestAnnotations: map[string]string{ociv1.AnnotationCreated: ociLivePackCreated},
		},
	)
	require.NoError(t, err)

	return staging, manifest
}

// ociLiveFixtureFiles loads the module tree committed under test/fixtures.
func ociLiveFixtureFiles(t *testing.T) map[string][]byte {
	t.Helper()

	files := make(map[string][]byte)

	err := fs.WalkDir(ociLiveFixtureFS, testFixtureOCILive, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		data, err := ociLiveFixtureFS.ReadFile(path)
		if err != nil {
			return err
		}

		files[strings.TrimPrefix(path, testFixtureOCILive+"/")] = data

		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, files)

	return files
}

// ociLiveFixtureManifest computes the fixture's manifest descriptor locally, without any registry access.
func ociLiveFixtureManifest(t *testing.T) ociv1.Descriptor {
	t.Helper()

	_, manifest := ociLiveFixtureStaging(t)

	return manifest
}

// ociLiveFixtureTag isolates fixture revisions in concurrent runs.
func ociLiveFixtureTag(manifestDigest digest.Digest) string {
	return "live-fixture-" + manifestDigest.Encoded()
}

// pushOCILiveFixture publishes the fixture under repository at the fixture tag.
func pushOCILiveFixture(t *testing.T, repository string, cred auth.Credential) {
	t.Helper()

	registryHost, _, _ := strings.Cut(repository, "/")
	staging, manifest := ociLiveFixtureStaging(t)
	tag := ociLiveFixtureTag(manifest.Digest)
	require.NoError(t, staging.Tag(t.Context(), manifest, tag))

	repo, err := remote.NewRepository(repository)
	require.NoError(t, err)

	repo.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Cache:      auth.NewCache(),
		Credential: auth.StaticCredential(registryHost, cred),
	}

	_, err = oras.Copy(t.Context(), staging, tag, repo, tag, oras.DefaultCopyOptions)
	require.NoError(t, err, "publishing the fixture to %s must succeed", repository)
}

// ecrLoginCredential mints a registry credential through the same helper the pull under test uses.
func ecrLoginCredential(t *testing.T, registryHost string) auth.Credential {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "docker-credential-ecr-login", "get")
	cmd.Stdin = strings.NewReader(registryHost)

	output, err := cmd.Output()
	require.NoError(t, err, "docker-credential-ecr-login must be on PATH with ambient AWS credentials")

	var minted struct {
		Username string `json:"Username"`
		Secret   string `json:"Secret"`
	}
	require.NoError(t, json.Unmarshal(output, &minted))

	return auth.Credential{Username: minted.Username, Password: minted.Secret}
}

// linkAWSConfigInto links the real ~/.aws into the hermetic home, so SSO-based helper credentials keep working.
func linkAWSConfigInto(t *testing.T, home string) {
	t.Helper()

	realHome, err := os.UserHomeDir()
	if err != nil {
		return
	}

	awsDir := filepath.Join(realHome, ".aws")
	if _, err := os.Stat(awsDir); err != nil {
		return
	}

	require.NoError(t, os.Symlink(awsDir, filepath.Join(home, ".aws")))
}
