package cas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaterializeTree_FromSynthStore(t *testing.T) {
	t.Parallel()

	storeDir := helpers.TmpDirWOSymlinks(t)

	blobStore := cas.NewStore(filepath.Join(storeDir, "blobs"))
	treeStore := cas.NewStore(filepath.Join(storeDir, "trees"))
	synthStore := cas.NewStore(filepath.Join(storeDir, "synth", "trees"))

	for _, s := range []*cas.Store{blobStore, treeStore, synthStore} {
		require.NoError(t, os.MkdirAll(s.Path(), 0755))
	}

	v, err := cas.OSVenv()
	require.NoError(t, err)

	l := logger.CreateLogger()

	// Store a blob in the blob store
	blobData := []byte("hello world\n")
	blobHash := "abc123"

	blobContent := cas.NewContent(blobStore)
	require.NoError(t, blobContent.Store(l, v, blobHash, blobData))

	// Store a synthetic tree that references the blob
	treeData := []byte("100644 blob abc123\tREADME.md\n")
	treeHash := "synth999"

	synthContent := cas.NewContent(synthStore)
	require.NoError(t, synthContent.Store(l, v, treeHash, treeData))

	// Build a CAS instance using the same store paths
	c, err := cas.New(cas.WithStorePath(storeDir))
	require.NoError(t, err)

	destDir := helpers.TmpDirWOSymlinks(t)

	err = c.MaterializeTree(t.Context(), l, v, treeHash, destDir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, blobData, content)
}

func TestMaterializeTree_FromGitTreeStore(t *testing.T) {
	t.Parallel()

	storeDir := helpers.TmpDirWOSymlinks(t)

	blobStore := cas.NewStore(filepath.Join(storeDir, "blobs"))
	treeStore := cas.NewStore(filepath.Join(storeDir, "trees"))
	synthStore := cas.NewStore(filepath.Join(storeDir, "synth", "trees"))

	for _, s := range []*cas.Store{blobStore, treeStore, synthStore} {
		require.NoError(t, os.MkdirAll(s.Path(), 0755))
	}

	v, err := cas.OSVenv()
	require.NoError(t, err)

	l := logger.CreateLogger()

	blobData := []byte("module content\n")
	blobHash := "blob111"

	blobContent := cas.NewContent(blobStore)
	require.NoError(t, blobContent.Store(l, v, blobHash, blobData))

	// Store a tree in the git tree store (not synth)
	treeData := []byte("100644 blob blob111\tmain.tf\n")
	treeHash := "tree222"

	treeContent := cas.NewContent(treeStore)
	require.NoError(t, treeContent.Store(l, v, treeHash, treeData))

	c, err := cas.New(cas.WithStorePath(storeDir))
	require.NoError(t, err)

	destDir := helpers.TmpDirWOSymlinks(t)

	err = c.MaterializeTree(t.Context(), l, v, treeHash, destDir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destDir, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, blobData, content)
}

func TestMaterializeTree_NotFound(t *testing.T) {
	t.Parallel()

	storeDir := helpers.TmpDirWOSymlinks(t)

	c, err := cas.New(cas.WithStorePath(storeDir))
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	destDir := helpers.TmpDirWOSymlinks(t)
	l := logger.CreateLogger()

	err = c.MaterializeTree(t.Context(), l, v, "nonexistent", destDir)
	require.Error(t, err)
	assert.ErrorIs(t, err, cas.ErrTreeNotFound)
}

func TestMaterializeTree_SynthTakesPrecedence(t *testing.T) {
	t.Parallel()

	storeDir := helpers.TmpDirWOSymlinks(t)

	blobStore := cas.NewStore(filepath.Join(storeDir, "blobs"))
	treeStore := cas.NewStore(filepath.Join(storeDir, "trees"))
	synthStore := cas.NewStore(filepath.Join(storeDir, "synth", "trees"))

	for _, s := range []*cas.Store{blobStore, treeStore, synthStore} {
		require.NoError(t, os.MkdirAll(s.Path(), 0755))
	}

	v, err := cas.OSVenv()
	require.NoError(t, err)

	l := logger.CreateLogger()

	blobA := []byte("synth version\n")
	blobB := []byte("git version\n")

	blobContent := cas.NewContent(blobStore)
	require.NoError(t, blobContent.Store(l, v, "blobA", blobA))
	require.NoError(t, blobContent.Store(l, v, "blobB", blobB))

	hash := "samehash"

	// Store in synth store (references blobA)
	synthContent := cas.NewContent(synthStore)
	require.NoError(t, synthContent.Store(l, v, hash, []byte("100644 blob blobA\tfile.txt\n")))

	// Store in git tree store (references blobB)
	gitContent := cas.NewContent(treeStore)
	require.NoError(t, gitContent.Store(l, v, hash, []byte("100644 blob blobB\tfile.txt\n")))

	c, err := cas.New(cas.WithStorePath(storeDir))
	require.NoError(t, err)

	destDir := helpers.TmpDirWOSymlinks(t)

	err = c.MaterializeTree(t.Context(), l, v, hash, destDir)
	require.NoError(t, err)

	// Synth store is checked first, so the synth version should win
	content, err := os.ReadFile(filepath.Join(destDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, blobA, content)
}

func TestHashAlgorithmSum(t *testing.T) {
	t.Parallel()

	sha1Result := cas.HashSHA1.Sum([]byte("test"))
	assert.Len(t, sha1Result, 40)

	sha256Result := cas.HashSHA256.Sum([]byte("test"))
	assert.Len(t, sha256Result, 64)

	// Same input produces same output
	assert.Equal(t, sha1Result, cas.HashSHA1.Sum([]byte("test")))
	assert.Equal(t, sha256Result, cas.HashSHA256.Sum([]byte("test")))

	// Different inputs produce different outputs
	assert.NotEqual(t, sha1Result, cas.HashSHA1.Sum([]byte("other")))
}

func TestCASProtocolGetterGet(t *testing.T) {
	t.Parallel()

	storeDir := helpers.TmpDirWOSymlinks(t)

	blobStore := cas.NewStore(filepath.Join(storeDir, "blobs"))
	synthStore := cas.NewStore(filepath.Join(storeDir, "synth", "trees"))

	for _, s := range []*cas.Store{
		blobStore,
		cas.NewStore(filepath.Join(storeDir, "trees")),
		synthStore,
	} {
		require.NoError(t, os.MkdirAll(s.Path(), 0755))
	}

	v, err := cas.OSVenv()
	require.NoError(t, err)

	l := logger.CreateLogger()

	fileContent := []byte("resource {}\n")
	fileHash := "file123"

	blobContent := cas.NewContent(blobStore)
	require.NoError(t, blobContent.Store(l, v, fileHash, fileContent))

	treeHash := "tree456"
	treeData := []byte("100644 blob file123\tmain.tf\n")

	synthContent := cas.NewContent(synthStore)
	require.NoError(t, synthContent.Store(l, v, treeHash, treeData))

	c, err := cas.New(cas.WithStorePath(storeDir))
	require.NoError(t, err)

	g := getter.NewCASProtocolGetter(l, c, v)

	destDir := helpers.TmpDirWOSymlinks(t)

	req := &getter.Request{
		Src: "sha1:" + treeHash,
		Dst: destDir,
	}

	err = g.Get(t.Context(), req)
	require.NoError(t, err)

	result, err := os.ReadFile(filepath.Join(destDir, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, fileContent, result)
}

// TestCASProtocolGetterGet_Mutable verifies that setting Mutable=true on the
// protocol getter forces blob copies (independent inodes) instead of hardlinks
// from the CAS store.
func TestCASProtocolGetterGet_Mutable(t *testing.T) {
	t.Parallel()

	storeDir := helpers.TmpDirWOSymlinks(t)

	blobStore := cas.NewStore(filepath.Join(storeDir, "blobs"))
	synthStore := cas.NewStore(filepath.Join(storeDir, "synth", "trees"))

	for _, s := range []*cas.Store{
		blobStore,
		cas.NewStore(filepath.Join(storeDir, "trees")),
		synthStore,
	} {
		require.NoError(t, os.MkdirAll(s.Path(), 0755))
	}

	v, err := cas.OSVenv()
	require.NoError(t, err)

	l := logger.CreateLogger()

	fileContent := []byte("resource {}\n")
	fileHash := "filemut1"

	blobContent := cas.NewContent(blobStore)
	require.NoError(t, blobContent.Store(l, v, fileHash, fileContent))

	treeHash := "treemut1"
	treeData := []byte("100644 blob filemut1\tmain.tf\n")

	synthContent := cas.NewContent(synthStore)
	require.NoError(t, synthContent.Store(l, v, treeHash, treeData))

	c, err := cas.New(cas.WithStorePath(storeDir))
	require.NoError(t, err)

	g := getter.NewCASProtocolGetter(l, c, v)
	g.Mutable = true

	destDir := helpers.TmpDirWOSymlinks(t)

	req := &getter.Request{
		Src: "sha1:" + treeHash,
		Dst: destDir,
	}

	require.NoError(t, g.Get(t.Context(), req))

	sourcePath := filepath.Join(blobStore.Path(), fileHash[:2], fileHash)
	targetPath := filepath.Join(destDir, "main.tf")

	sourceInfo, err := os.Stat(sourcePath)
	require.NoError(t, err)

	targetInfo, err := os.Stat(targetPath)
	require.NoError(t, err)

	assert.False(t, os.SameFile(sourceInfo, targetInfo),
		"Mutable=true must produce an independent inode (copy, not hard link)")
}

func TestCASProtocolGetterGet_InvalidRef(t *testing.T) {
	t.Parallel()

	storeDir := helpers.TmpDirWOSymlinks(t)

	c, err := cas.New(cas.WithStorePath(storeDir))
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	l := logger.CreateLogger()
	g := getter.NewCASProtocolGetter(l, c, v)

	req := &getter.Request{
		Src: "badprefix:abc123",
		Dst: t.TempDir(),
	}

	err = g.Get(t.Context(), req)
	require.Error(t, err)
	assert.ErrorIs(t, err, cas.ErrCASRefMissingPrefix)
}
