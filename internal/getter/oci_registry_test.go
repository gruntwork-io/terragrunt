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

// ociRegistryFiles is the module tree the local-registry tests publish.
var ociRegistryFiles = map[string]string{
	"main.tf": `module "root" {
  source = "registry.opentofu.org/terraform-aws-modules/vpc/aws"
}
`,
	"modules/sub/main.tf": `output "sub" {
  value = "sub"
}
`,
}

// TestOCIGetterAgainstLocalRegistry downloads a published module through the real getter chain.
func TestOCIGetterAgainstLocalRegistry(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	manifest := registry.PushModule(t, ociRegistryRepository, "1.0.0", ociRegistryFiles)
	base := "oci://" + registry.Address() + "/" + ociRegistryRepository

	subPath := filepath.Join("modules", "sub", "main.tf")

	testCases := []struct {
		name        string
		src         string
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:        "tag pin",
			src:         base + "?tag=1.0.0",
			wantPresent: []string{"main.tf", subPath},
		},
		{
			name:        "digest pin",
			src:         base + "?digest=" + manifest.Digest.String(),
			wantPresent: []string{"main.tf", subPath},
		},
		{
			name:        "subdir selects one tree",
			src:         base + "//modules/sub?tag=1.0.0",
			wantPresent: []string{"main.tf"},
			wantAbsent:  []string{subPath},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "module")

			_, err := ociRegistryClient(t, registry).Get(t.Context(), &getter.Request{
				Src: tc.src,
				Dst: dst,
			})
			require.NoError(t, err)

			for _, name := range tc.wantPresent {
				assert.FileExists(t, filepath.Join(dst, name))
			}

			for _, name := range tc.wantAbsent {
				assert.NoFileExists(t, filepath.Join(dst, name))
			}
		})
	}
}

// TestOCIGetterAgainstLocalRegistryVerifiesLayerDigest fails the download when the served bytes moved.
func TestOCIGetterAgainstLocalRegistryVerifiesLayerDigest(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	manifest := registry.PushModule(t, ociRegistryRepository, "1.0.0", ociRegistryFiles)
	layer := ociRegistryLayer(t, registry, manifest.Digest.String())

	// Same length, so the size check cannot mask the digest check.
	registry.OverwriteBlob(t, ociRegistryRepository, layer.Digest, make([]byte, layer.Size))

	_, err := ociRegistryClient(t, registry).Get(t.Context(), &getter.Request{
		Src: "oci://" + registry.Address() + "/" + ociRegistryRepository + "?tag=1.0.0",
		Dst: filepath.Join(helpers.TmpDirWOSymlinks(t), "module"),
	})

	var verificationErr getter.OCIDigestVerificationError
	require.ErrorAs(t, err, &verificationErr, "a moved layer must fail digest verification")
}

// TestOCIGetterAgainstLocalRegistryUnknownTag surfaces a typed error when the tag was never published.
func TestOCIGetterAgainstLocalRegistryUnknownTag(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, ociRegistryRepository, "1.0.0", ociRegistryFiles)

	_, err := ociRegistryClient(t, registry).Get(t.Context(), &getter.Request{
		Src: "oci://" + registry.Address() + "/" + ociRegistryRepository + "?tag=2.0.0",
		Dst: filepath.Join(helpers.TmpDirWOSymlinks(t), "module"),
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

	_, err := ociRegistryClient(t, registry).Get(t.Context(), &getter.Request{
		Src: "oci://" + registry.Address() + "/terraform-modules/other?tag=1.0.0",
		Dst: filepath.Join(helpers.TmpDirWOSymlinks(t), "module"),
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

	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "module")

	_, err := ociRegistryClient(t, registry).Get(t.Context(), &getter.Request{
		Src: "oci://" + registry.Address() + "/" + repository + "?tag=1.0.0",
		Dst: dst,
	})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dst, "main.tf"))
}

// ociRegistryClient builds the production getter chain pointed at registry.
func ociRegistryClient(t *testing.T, registry *ociregistry.Registry) *getter.Client {
	t.Helper()

	return getter.NewClient(
		getter.WithOCI(getter.NewOCIGetter(logger.CreateLogger(), ociRegistryVenv(t, registry))),
	)
}

// ociRegistryLayer resolves the published manifest down to its single module-zip layer.
func ociRegistryLayer(t *testing.T, registry *ociregistry.Registry, ref string) ociv1.Descriptor {
	t.Helper()

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), ociRegistryVenv(t, registry))

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

// ociRegistryVenv builds the hermetic venv the local-registry tests resolve credentials through.
func ociRegistryVenv(t *testing.T, registry *ociregistry.Registry) *venv.Venv {
	t.Helper()

	home := t.TempDir()

	v := venvtest.New().
		WithFS(vfs.NewOSFS()).
		WithUserHomeDir(func() (string, error) { return home, nil })
	v.HTTP = registry.Client()

	return v
}
