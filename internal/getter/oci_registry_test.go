package getter_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/ociregistry"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// ociRegistryRepository is the repository every local-registry test publishes under.
const ociRegistryRepository = "terraform-modules/vpc"

// ociRegistryFiles is the plain local module tree the local-registry tests publish.
var ociRegistryFiles = map[string]string{
	"main.tf": `output "root" {
  value = "root"
}
`,
	"modules/sub/main.tf": `output "sub" {
  value = "sub"
}
`,
}

// TestOCIGetterAgainstLocalRegistry downloads a published module through the real getter chain in memory.
func TestOCIGetterAgainstLocalRegistry(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	manifest := registry.PushModule(t, ociRegistryRepository, "1.0.0", ociRegistryFiles)
	base := "oci://" + registry.Address() + "/" + ociRegistryRepository

	testCases := []struct {
		name string
		src  string
	}{
		{name: "tag pin", src: base + "?tag=1.0.0"},
		{name: "digest pin", src: base + "?digest=" + manifest.Digest.String()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := ociRegistryVenv(registry)
			dst := "/module"

			_, err := ociRegistryClient(v).Get(t.Context(), &getter.Request{
				Src: tc.src,
				Dst: dst,
			})
			require.NoError(t, err)

			for name := range ociRegistryFiles {
				requireFileOnFS(t, v.FS, filepath.Join(dst, filepath.FromSlash(name)))
			}
		})
	}
}

// TestOCIGetterAgainstLocalRegistrySubdir needs the OS filesystem: go-getter's client promotes //subdir with os calls.
func TestOCIGetterAgainstLocalRegistrySubdir(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, ociRegistryRepository, "1.0.0", ociRegistryFiles)

	// An empty OS home keeps the OS-filesystem case as hermetic as the in-memory one.
	home := t.TempDir()
	v := ociRegistryVenv(registry).
		WithFS(vfs.NewOSFS()).
		WithUserHomeDir(func() (string, error) { return home, nil })
	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "module")

	_, err := ociRegistryClient(v).Get(t.Context(), &getter.Request{
		Src: "oci://" + registry.Address() + "/" + ociRegistryRepository + "//modules/sub?tag=1.0.0",
		Dst: dst,
	})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dst, "main.tf"))
	assert.NoFileExists(t, filepath.Join(dst, "modules", "sub", "main.tf"))
}

// TestOCIGetterAgainstLocalRegistryVerifiesLayerDigest fails the download when the served bytes moved.
func TestOCIGetterAgainstLocalRegistryVerifiesLayerDigest(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	manifest := registry.PushModule(t, ociRegistryRepository, "1.0.0", ociRegistryFiles)
	layer := ociRegistryLayer(t, registry, manifest.Digest.String())

	// Same length, so the size check cannot mask the digest check.
	registry.OverwriteBlob(t, ociRegistryRepository, layer.Digest, make([]byte, layer.Size))

	_, err := ociRegistryClient(ociRegistryVenv(registry)).Get(t.Context(), &getter.Request{
		Src: "oci://" + registry.Address() + "/" + ociRegistryRepository + "?tag=1.0.0",
		Dst: "/module",
	})

	var verificationErr getter.OCIDigestVerificationError
	require.ErrorAs(t, err, &verificationErr, "a moved layer must fail digest verification")
}

// TestOCIGetterAgainstLocalRegistryUnknownTag surfaces a typed error when the tag was never published.
func TestOCIGetterAgainstLocalRegistryUnknownTag(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, ociRegistryRepository, "1.0.0", ociRegistryFiles)

	_, err := ociRegistryClient(ociRegistryVenv(registry)).Get(t.Context(), &getter.Request{
		Src: "oci://" + registry.Address() + "/" + ociRegistryRepository + "?tag=2.0.0",
		Dst: "/module",
	})

	var resolutionErr getter.OCIReferenceResolutionError
	require.ErrorAs(t, err, &resolutionErr, "an unpublished tag must surface a typed resolution error")
	assert.Equal(t, "2.0.0", resolutionErr.Ref)
}

// TestOCIGetterAgainstLocalRegistryUnknownRepository keeps a published blob unreachable through another name.
func TestOCIGetterAgainstLocalRegistryUnknownRepository(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, ociRegistryRepository, "1.0.0", ociRegistryFiles)

	_, err := ociRegistryClient(ociRegistryVenv(registry)).Get(t.Context(), &getter.Request{
		Src: "oci://" + registry.Address() + "/terraform-modules/other?tag=1.0.0",
		Dst: "/module",
	})

	var resolutionErr getter.OCIReferenceResolutionError
	require.ErrorAs(t, err, &resolutionErr, "an unpublished repository must surface a typed resolution error")
}

// TestOCIGetterAgainstLocalRegistryRepositoryNamedLikeAPIPath keeps a repository segment from routing as an API verb.
func TestOCIGetterAgainstLocalRegistryRepositoryNamedLikeAPIPath(t *testing.T) {
	t.Parallel()

	const repository = "mycorp/manifests/vpc"

	registry := ociregistry.Start(t)
	registry.PushModule(t, repository, "1.0.0", ociRegistryFiles)

	v := ociRegistryVenv(registry)
	dst := "/module"

	_, err := ociRegistryClient(v).Get(t.Context(), &getter.Request{
		Src: "oci://" + registry.Address() + "/" + repository + "?tag=1.0.0",
		Dst: dst,
	})
	require.NoError(t, err)

	requireFileOnFS(t, v.FS, filepath.Join(dst, "main.tf"))
}

// ociRegistryClient builds the production getter chain resolving through v.
func ociRegistryClient(v *venv.Venv) *getter.Client {
	return getter.NewClient(getter.WithOCI(getter.NewOCIGetter(logger.CreateLogger(), v)))
}

// ociRegistryLayer resolves the published manifest down to its single module-zip layer.
func ociRegistryLayer(t *testing.T, registry *ociregistry.Registry, ref string) ociv1.Descriptor {
	t.Helper()

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), ociRegistryVenv(registry))

	store, err := newStore(t.Context(), registry.Address(), ociRegistryRepository)
	require.NoError(t, err)

	manifestDesc, err := store.Resolve(t.Context(), ref)
	require.NoError(t, err)

	reader, err := store.Fetch(t.Context(), &manifestDesc)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, reader.Close())
	}()

	var manifest ociv1.Manifest
	require.NoError(t, json.NewDecoder(reader).Decode(&manifest))
	require.Len(t, manifest.Layers, 1)

	return manifest.Layers[0]
}

// ociRegistryVenv builds the hermetic in-memory venv the local-registry tests resolve credentials through.
func ociRegistryVenv(registry *ociregistry.Registry) *venv.Venv {
	v := venvtest.New().WithUserHomeDir(func() (string, error) { return "/home/tester", nil })
	v.HTTP = registry.Client()

	return v
}

// requireFileOnFS asserts path exists on fsys, which assert.FileExists cannot see for a mem filesystem.
func requireFileOnFS(t *testing.T, fsys vfs.FS, path string) {
	t.Helper()

	exists, err := vfs.FileExists(fsys, path)
	require.NoError(t, err)
	require.True(t, exists, "expected %s on the getter's filesystem", path)
}
