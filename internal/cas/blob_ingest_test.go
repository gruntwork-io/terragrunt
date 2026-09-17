package cas_test

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const (
	coldIngestFileCount = 300

	// coldIngestLargeSize is past every buffer between git and the store so
	// at least one blob spans several reads.
	coldIngestLargeSize = 300 * 1024

	missingBlobID = "0000000000000000000000000000000000000000"
)

// TestCAS_ColdIngestBlobsMatchGit ingests a repository of a few hundred
// files through one cat-file batch and checks that every blob lands in the
// store under git's own object id with exactly the content committed.
func TestCAS_ColdIngestBlobsMatchGit(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()

	files := coldIngestFixture()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFiles(ctx, files, "cold ingest fixture"))

	repoURL, err := srv.Start(ctx)
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")
	targetPath := filepath.Join(tempDir, "repo")

	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
	require.NoError(t, err)

	require.NoError(t, c.Clone(ctx, l, v, repoURL, cas.WithDir(targetPath), cas.WithDepth(-1)))

	for path, want := range files {
		linked, err := os.ReadFile(filepath.Join(targetPath, filepath.FromSlash(path)))
		require.NoError(t, err, path)
		assert.Equal(t, want, linked, "materialized %s", path)

		oid := gitBlobID(want)

		stored, err := os.ReadFile(filepath.Join(c.BlobStore().Path(), oid[:2], oid))
		require.NoError(t, err, "blob %s for %s", oid, path)
		assert.Equal(t, want, stored, "stored blob for %s", path)
	}
}

// TestCAS_EnsureBlobClosesTempHandleOnReadFailure pins that a failed read
// leaves neither an open handle nor a temp file behind. An unclosed handle
// would live until its finalizer runs, and on Windows it would also block
// the temp file's removal.
func TestCAS_EnsureBlobClosesTempHandleOnReadFailure(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile(ctx, "a.txt", []byte("a\n"), "seed"))

	repoURL, err := srv.Start(ctx)
	require.NoError(t, err)

	runner, err := git.NewGitRunner(venvtest.NewOSWithEmptyEnv())
	require.NoError(t, err)

	runner = runner.WithWorkDir(helpers.TmpDirWOSymlinks(t))
	require.NoError(t, runner.Clone(ctx, repoURL, true, 1, "main"))

	batch, err := runner.StartCatFileBatch(ctx)
	require.NoError(t, err)

	// Not t.Cleanup. The test's context is already canceled by the time
	// cleanups run, and Close reports that as a failure.
	defer func() { require.NoError(t, batch.Close()) }()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
	require.NoError(t, err)

	fsys := &handleTrackingFS{FS: vfs.NewOSFS()}
	v := venvtest.NewOSWithEmptyEnv().WithFS(fsys)

	err = c.EnsureBlob(v, batch, missingBlobID, cas.StoredFilePerms)
	require.ErrorIs(t, err, git.ErrCatFileMissing)

	require.Len(t, fsys.created, 1, "one temp handle expected")

	tmp := fsys.created[0]
	assert.True(t, tmp.closed.Load(), "temp handle left open after a failed read")
	assert.NoFileExists(t, tmp.Name())
}

// handleTrackingFS records every file opened with O_CREATE so a test can
// assert the caller closed it.
type handleTrackingFS struct {
	vfs.FS
	created []*trackedHandle
}

func (fsys *handleTrackingFS) OpenFile(name string, flag int, perm os.FileMode) (vfs.File, error) {
	f, err := fsys.FS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	if flag&os.O_CREATE == 0 {
		return f, nil
	}

	tracked := &trackedHandle{File: f}
	fsys.created = append(fsys.created, tracked)

	return tracked, nil
}

// trackedHandle flags itself closed so handleTrackingFS can report a
// handle its caller dropped.
type trackedHandle struct {
	vfs.File
	closed atomic.Bool
}

func (f *trackedHandle) Close() error {
	f.closed.Store(true)

	return f.File.Close()
}

// coldIngestFixture builds the file map for the cold-ingest test. It is
// mostly distinct text files, plus binary content with NUL bytes and
// embedded newlines, empty files, duplicates that dedupe to one blob, and
// one file larger than any buffer on the read path.
func coldIngestFixture() map[string][]byte {
	files := make(map[string][]byte, coldIngestFileCount)

	for i := range coldIngestFileCount {
		path := fmt.Sprintf("dir%02d/file%03d", i%10, i)

		switch {
		case i%50 == 0:
			files[path+".empty"] = []byte{}
		case i%7 == 0:
			files[path+".bin"] = binaryContent(i)
		case i%11 == 0:
			files[path+".dup"] = []byte("shared content\n")
		default:
			files[path+".txt"] = []byte("file " + strconv.Itoa(i) + "\nline two\n")
		}
	}

	large := make([]byte, coldIngestLargeSize)
	for i := range large {
		large[i] = byte(i*13 + i/257)
	}

	files["large.bin"] = large

	return files
}

// binaryContent returns content with NUL bytes and newlines placed so the
// response framing cannot rely on either being absent.
func binaryContent(seed int) []byte {
	content := make([]byte, 0, 64)
	content = append(content, 0, '\n', byte(seed), 0xff, '\n', '\n', 0)
	content = append(content, []byte(strconv.Itoa(seed))...)
	content = append(content, 0, '\n')

	return content
}

// gitBlobID computes the SHA-1 object id git assigns to a blob with content.
func gitBlobID(content []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write(content)

	return hex.EncodeToString(h.Sum(nil))
}
