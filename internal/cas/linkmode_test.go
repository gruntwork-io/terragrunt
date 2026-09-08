package cas_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// linkModeTestHash is the blob hash the link mode tests store their content
// under. Only the first two characters matter to the store, which uses them
// as the partition directory.
const linkModeTestHash = "ab00112233445566778899aabbccddeeff001122"

// TestLinkModes pins what each mode leaves at the target path: the stored
// content either way, and permissions that say whether the destination is the
// store's own file or one of its own.
func TestLinkModes(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	blobData := []byte("module content\n")

	tests := []struct {
		name     string
		mode     cas.LinkMode
		wantPerm os.FileMode
		wantCopy int64
	}{
		{
			name:     "hardlink hands out the stored file without its write bits",
			mode:     cas.LinkModeHardlink,
			wantPerm: 0o444,
		},
		{
			name:     "clone keeps the write bits because the file is its own",
			mode:     cas.LinkModeClone,
			wantPerm: 0o644,
		},
		{
			name:     "copy keeps the write bits and reports what it wrote",
			mode:     cas.LinkModeCopy,
			wantPerm: 0o644,
			wantCopy: int64(len(blobData)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v, content := newLinkModeStore(t, l, blobData)

			targetPath := "/target/main.tf"

			outcome, err := content.Link(
				l,
				v,
				linkModeTestHash,
				targetPath,
				0o644,
				cas.WithFileLinkMode(tt.mode),
			)
			require.NoError(t, err)

			assert.Equal(t, tt.mode, outcome.Mode)
			assert.Equal(t, tt.wantCopy, outcome.BytesCopied)

			got, err := vfs.ReadFile(v.FS, targetPath)
			require.NoError(t, err)
			assert.Equal(t, blobData, got)

			info, err := v.FS.Stat(targetPath)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPerm, info.Mode().Perm())
		})
	}
}

// TestLinkTreeCloneModeFallsBackWithoutCloneSupport pins what a filesystem
// with no copy-on-write clone leaves behind: the tree is materialized anyway,
// and the span says the mode could not be served.
func TestLinkTreeCloneModeFallsBackWithoutCloneSupport(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	v := venvtest.New()
	require.NoError(t, v.FS.MkdirAll("/store", 0o755))

	v = v.WithFS(&noCloneFS{FS: v.FS})

	blobData := []byte("module content\n")
	store := cas.NewStore("/store")

	require.NoError(t, cas.NewContent(store).Store(l, v, linkModeTestHash, blobData, cas.StoredFilePerms))

	tree, err := git.ParseTree([]byte("100644 blob "+linkModeTestHash+" main.tf"), "/target")
	require.NoError(t, err)

	buf, tlm := newConsoleTelemeter(t, l)

	ctx := telemetry.ContextWithTelemeter(t.Context(), tlm)

	require.NoError(t, cas.LinkTree(
		ctx,
		l,
		v,
		store,
		store,
		tree,
		"/target",
		cas.WithTreeLinkMode(cas.LinkModeClone),
	))
	require.NoError(t, tlm.Shutdown(ctx))

	got, err := vfs.ReadFile(v.FS, "/target/main.tf")
	require.NoError(t, err)
	assert.Equal(t, blobData, got)

	var treeSpan decodedSpan

	for _, span := range decodeSpans(t, buf) {
		if span.Name == "cas_link_tree" {
			treeSpan = span
		}
	}

	assert.Equal(t, map[string]any{
		"path":         "/target",
		"mode":         cas.LinkModeClone.String(),
		"files_linked": float64(0),
		"files_cloned": float64(0),
		"files_copied": float64(1),
		"bytes_copied": float64(len(blobData)),
		"fallback":     string(cas.LinkFallbackCloneUnsupported),
	}, treeSpan.Attrs)
}

// TestLinkCloneModeOnOSFilesystem drives the real clone syscall. The
// destination has to be a file of its own carrying the permissions git
// recorded, which is what makes the mode usable for a source the caller
// edits.
func TestLinkCloneModeOnOSFilesystem(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()

	storeDir := t.TempDir()
	targetDir := t.TempDir()

	content := cas.NewContent(cas.NewStore(storeDir))
	blobData := []byte("module content\n")

	require.NoError(t, content.Store(l, v, linkModeTestHash, blobData, cas.StoredFilePerms))

	sourcePath := filepath.Join(storeDir, linkModeTestHash[:2], linkModeTestHash)
	skipWithoutCloneSupport(t, v, sourcePath, filepath.Join(targetDir, "probe"))

	for _, mutable := range []bool{false, true} {
		targetPath := filepath.Join(targetDir, "main.tf")

		opts := []cas.LinkOption{cas.WithFileLinkMode(cas.LinkModeClone)}
		if mutable {
			opts = append(opts, cas.WithLinkForceCopy())
		}

		outcome, err := content.Link(l, v, linkModeTestHash, targetPath, 0o644, opts...)
		require.NoError(t, err)
		assert.Equal(
			t,
			cas.LinkModeClone,
			outcome.Mode,
			"a mutable source is cloned rather than copied (mutable=%t)",
			mutable,
		)
		assert.Zero(t, outcome.BytesCopied, "a clone writes no content")

		got, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, blobData, got)

		targetInfo, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), targetInfo.Mode().Perm())

		sourceInfo, err := os.Stat(sourcePath)
		require.NoError(t, err)
		assert.False(
			t,
			os.SameFile(sourceInfo, targetInfo),
			"a clone must be its own file, not a second name for the stored blob",
		)
	}
}

// TestLinkMutableSourceClonesWhereSupported pins the mode a mutable source
// resolves to: a hard link cannot serve one, and a clone gives it a writable
// file of its own without copying the content.
func TestLinkMutableSourceClonesWhereSupported(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()

	storeDir := t.TempDir()
	targetDir := t.TempDir()

	content := cas.NewContent(cas.NewStore(storeDir))
	blobData := []byte("module content\n")

	require.NoError(t, content.Store(l, v, linkModeTestHash, blobData, cas.StoredFilePerms))

	sourcePath := filepath.Join(storeDir, linkModeTestHash[:2], linkModeTestHash)
	skipWithoutCloneSupport(t, v, sourcePath, filepath.Join(targetDir, "probe"))

	targetPath := filepath.Join(targetDir, "main.tf")

	outcome, err := content.Link(
		l,
		v,
		linkModeTestHash,
		targetPath,
		0o644,
		cas.WithFileLinkMode(cas.LinkModeHardlink),
		cas.WithLinkForceCopy(),
	)
	require.NoError(t, err)
	assert.Equal(t, cas.LinkModeClone, outcome.Mode)
	assert.Zero(t, outcome.BytesCopied, "a clone writes no content")

	got, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, blobData, got)

	targetInfo, err := os.Stat(targetPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), targetInfo.Mode().Perm(), "a mutable source stays writable")

	sourceInfo, err := os.Stat(sourcePath)
	require.NoError(t, err)
	assert.False(
		t,
		os.SameFile(sourceInfo, targetInfo),
		"a mutable source must not share the stored file",
	)
}

// TestLinkMutableSourceCopiesWithoutCloneSupport pins the fallback behind
// that resolution, which is the behavior every filesystem without
// copy-on-write clones gets.
func TestLinkMutableSourceCopiesWithoutCloneSupport(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	blobData := []byte("module content\n")

	v := venvtest.New()
	require.NoError(t, v.FS.MkdirAll("/store", 0o755))

	v = v.WithFS(&noCloneFS{FS: v.FS})

	content := cas.NewContent(cas.NewStore("/store"))
	require.NoError(t, content.Store(l, v, linkModeTestHash, blobData, cas.StoredFilePerms))

	targetPath := "/target/main.tf"

	outcome, err := content.Link(
		l,
		v,
		linkModeTestHash,
		targetPath,
		0o644,
		cas.WithFileLinkMode(cas.LinkModeHardlink),
		cas.WithLinkForceCopy(),
	)
	require.NoError(t, err)
	assert.Equal(t, cas.LinkModeCopy, outcome.Mode)
	assert.Equal(t, int64(len(blobData)), outcome.BytesCopied)

	got, err := vfs.ReadFile(v.FS, targetPath)
	require.NoError(t, err)
	assert.Equal(t, blobData, got)

	info, err := v.FS.Stat(targetPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "a mutable source stays writable")
}

// TestLinkTreeProbesCloneSupportOnce pins the cost of asking for a clone on a
// filesystem that has none: one attempt for the whole tree, not one per file.
func TestLinkTreeProbesCloneSupportOnce(t *testing.T) {
	t.Parallel()

	const files = 12

	l := logger.CreateLogger()
	blobData := []byte("module content\n")

	v := venvtest.New()
	require.NoError(t, v.FS.MkdirAll("/store", 0o755))

	store := cas.NewStore("/store")
	content := cas.NewContent(store)

	var treeData []byte

	for i := range files {
		hash := fmt.Sprintf("%040x", i+1)
		require.NoError(t, content.Store(l, v, hash, blobData, cas.StoredFilePerms))

		treeData = fmt.Appendf(treeData, "100644 blob %s\tfile%02d.tf\n", hash, i)
	}

	tree, err := git.ParseTree(treeData, "/target")
	require.NoError(t, err)
	require.Len(t, tree.Entries(), files)

	fsys := &noCloneFS{FS: v.FS}

	require.NoError(t, cas.LinkTree(
		t.Context(),
		l,
		v.WithFS(fsys),
		store,
		store,
		tree,
		"/target",
		cas.WithTreeLinkMode(cas.LinkModeClone),
	))

	assert.Equal(
		t,
		int64(1),
		fsys.attempts.Load(),
		"the first blob settles clone support for the rest of the tree",
	)

	for i := range files {
		got, err := vfs.ReadFile(v.FS, fmt.Sprintf("/target/file%02d.tf", i))
		require.NoError(t, err)
		assert.Equal(t, blobData, got)
	}
}

// noCloneFS refuses every copy-on-write clone and counts how many were asked
// for, which is what says whether a tree probed once or once per file.
// Embedding the FS interface alone withholds the hard link too, so what it
// wraps has no way to materialize a blob but to copy it.
type noCloneFS struct {
	vfs.FS
	attempts atomic.Int64
}

func (fsys *noCloneFS) CloneFileIfPossible(oldname, newname string) error {
	fsys.attempts.Add(1)

	return &os.LinkError{
		Op:  "clonefile",
		Old: oldname,
		New: newname,
		Err: vfs.ErrNoCloneFile,
	}
}

// newLinkModeStore returns an in-memory venv holding blobData in a store at
// /store, along with the content handle addressing it.
func newLinkModeStore(t *testing.T, l log.Logger, blobData []byte) (*venv.Venv, *cas.Content) {
	t.Helper()

	v := venvtest.New()
	require.NoError(t, v.FS.MkdirAll("/store", 0o755))
	require.NoError(t, v.FS.MkdirAll("/target", 0o755))

	content := cas.NewContent(cas.NewStore("/store"))
	require.NoError(t, content.Store(l, v, linkModeTestHash, blobData, cas.StoredFilePerms))

	return v, content
}

// skipWithoutCloneSupport skips the test when the filesystem holding the
// temporary directories has no copy-on-write clone to offer, which is the
// case for ext4, HFS+, and every filesystem on Windows.
func skipWithoutCloneSupport(t *testing.T, v *venv.Venv, sourcePath, probePath string) {
	t.Helper()

	err := vfs.CloneFile(v.FS, sourcePath, probePath)
	if errors.Is(err, vfs.ErrNoCloneFile) {
		t.Skipf("filesystem under %s does not support copy-on-write clones", probePath)
	}

	require.NoError(t, err)
	require.NoError(t, os.Remove(probePath))
}
