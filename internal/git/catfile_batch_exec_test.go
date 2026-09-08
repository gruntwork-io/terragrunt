//go:build exec && !windows

package git_test

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// batchLargeBlobSize is well past the bufio buffer and the io.Copy chunk
	// so a read that is interrupted mid-object still has content in flight.
	batchLargeBlobSize = 1 << 20

	// batchPromptness bounds how long a read or close may take after the
	// context is canceled. Past that, the test calls it hung.
	batchPromptness = 30 * time.Second

	missingObjectID = "0000000000000000000000000000000000000000"

	// ambiguousPrefixLen is git's shortest accepted abbreviation, and the
	// length a caller passing a truncated name would send.
	ambiguousPrefixLen = 4

	// ambiguousSearchBound is one past the number of distinct prefixes of
	// ambiguousPrefixLen hex digits. Searching that many distinct blobs
	// therefore hits a collision, by the pigeonhole principle.
	ambiguousSearchBound = 1<<(4*ambiguousPrefixLen) + 1
)

// errSinkFailed is the error the failing writers below report, so a test
// can tell it apart from anything the batch itself produces.
var errSinkFailed = errors.New("sink failed")

// batchFixture is a bare clone of a repository whose blobs are known by
// path, plus the hash of its head commit and an abbreviated object name
// that two of its blobs share.
type batchFixture struct {
	files           map[string][]byte
	hashes          map[string]string
	dir             string
	commitHash      string
	ambiguousPrefix string
}

func TestExecCatFileBatch_ReadBlob(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	fx := newBatchFixture(t)

	batch, err := batchRunner(t, fx.dir).StartCatFileBatch(ctx)
	require.NoError(t, err)

	for path, want := range fx.files {
		var got bytes.Buffer

		require.NoError(t, batch.ReadBlob(fx.hashes[path], &got), path)
		assert.Equal(t, want, got.Bytes(), path)
	}

	require.NoError(t, batch.Close())
}

func TestExecCatFileBatch_MissingObject(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	fx := newBatchFixture(t)

	batch, err := batchRunner(t, fx.dir).StartCatFileBatch(ctx)
	require.NoError(t, err)

	err = batch.ReadBlob(missingObjectID, &bytes.Buffer{})
	require.ErrorIs(t, err, git.ErrCatFileMissing)

	var wrappedErr *git.WrappedError

	require.ErrorAs(t, err, &wrappedErr)
	assert.Equal(t, missingObjectID, wrappedErr.Context)

	var got bytes.Buffer

	require.NoError(t, batch.ReadBlob(fx.hashes["text.txt"], &got))
	assert.Equal(t, fx.files["text.txt"], got.Bytes())

	require.NoError(t, batch.Close())
}

func TestExecCatFileBatch_AmbiguousName(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	fx := newBatchFixture(t)

	batch, err := batchRunner(t, fx.dir).StartCatFileBatch(ctx)
	require.NoError(t, err)

	var got bytes.Buffer

	err = batch.ReadBlob(fx.ambiguousPrefix, &got)
	require.ErrorIs(t, err, git.ErrCatFileAmbiguous)
	require.NotErrorIs(t, err, git.ErrCatFileFraming)
	assert.Empty(t, got.Bytes())

	require.NoError(t, batch.ReadBlob(fx.hashes["text.txt"], &got))
	assert.Equal(t, fx.files["text.txt"], got.Bytes())

	require.NoError(t, batch.Close())
}

// TestExecCatFileBatch_ResponseNamesAnotherObject pins that a response for
// an object the caller did not ask for is refused instead of being handed
// back as the answer. A newline inside the request smuggles a second object
// name past ReadBlob, so git answers twice and the first answer names only
// part of what was sent.
func TestExecCatFileBatch_ResponseNamesAnotherObject(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	fx := newBatchFixture(t)

	batch, err := batchRunner(t, fx.dir).StartCatFileBatch(ctx)
	require.NoError(t, err)

	var got bytes.Buffer

	smuggled := fx.hashes["text.txt"] + "\n" + fx.hashes["binary.bin"]

	err = batch.ReadBlob(smuggled, &got)
	require.ErrorIs(t, err, git.ErrCatFileFraming)
	assert.Empty(t, got.Bytes(), "content for another object must not reach the writer")

	require.NoError(t, batch.Close())
}

func TestExecCatFileBatch_NotBlob(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	fx := newBatchFixture(t)

	batch, err := batchRunner(t, fx.dir).StartCatFileBatch(ctx)
	require.NoError(t, err)

	var got bytes.Buffer

	err = batch.ReadBlob(fx.commitHash, &got)
	require.ErrorIs(t, err, git.ErrCatFileNotBlob)
	assert.Empty(t, got.Bytes(), "commit content must not reach the writer")

	require.NoError(t, batch.ReadBlob(fx.hashes["binary.bin"], &got))
	assert.Equal(t, fx.files["binary.bin"], got.Bytes())

	require.NoError(t, batch.Close())
}

func TestExecCatFileBatch_CloseReportsExit(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// Not a repository: git prints a fatal error and exits 128.
	batch, err := batchRunner(t, helpers.TmpDirWOSymlinks(t)).StartCatFileBatch(ctx)
	require.NoError(t, err)

	err = batch.ReadBlob(missingObjectID, &bytes.Buffer{})
	require.ErrorIs(t, err, git.ErrCatFileShortRead)

	err = batch.Close()
	require.ErrorIs(t, err, git.ErrCommandSpawn)

	var wrappedErr *git.WrappedError

	require.ErrorAs(t, err, &wrappedErr)
	assert.Equal(t, 128, vexec.ExitCode(wrappedErr.Err))
	assert.NotEmpty(t, wrappedErr.Context, "stderr should carry git's diagnostic")
}

func TestExecCatFileBatch_WriterFailure(t *testing.T) {
	t.Parallel()

	fx := newBatchFixture(t)

	testCases := []struct {
		sink    io.Writer
		wantErr error
		name    string
	}{
		{
			name:    "write error",
			sink:    failOnWrite{},
			wantErr: errSinkFailed,
		},
		{
			name:    "short write",
			sink:    shortWrite{},
			wantErr: io.ErrShortWrite,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			batch, err := batchRunner(t, fx.dir).StartCatFileBatch(ctx)
			require.NoError(t, err)

			// The large blob guarantees the failure lands with most of the
			// object still queued in the pipe. The next read would otherwise
			// parse those bytes as its header.
			err = batch.ReadBlob(fx.hashes["large.bin"], tc.sink)
			require.ErrorIs(t, err, tc.wantErr)
			require.NotErrorIs(t, err, git.ErrCatFileShortRead)
			require.NotErrorIs(t, err, git.ErrCatFileFraming)

			var got bytes.Buffer

			require.NoError(t, batch.ReadBlob(fx.hashes["text.txt"], &got))
			assert.Equal(t, fx.files["text.txt"], got.Bytes())

			require.NoError(t, batch.Close())
		})
	}
}

func TestExecCatFileBatch_Cancel(t *testing.T) {
	t.Parallel()

	fx := newBatchFixture(t)

	t.Run("canceled before read", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())

		batch, err := batchRunner(t, fx.dir).StartCatFileBatch(ctx)
		require.NoError(t, err)

		cancel()

		err = awaitPromptly(t, func() error {
			return batch.ReadBlob(fx.hashes["text.txt"], &bytes.Buffer{})
		})
		require.ErrorIs(t, err, context.Canceled)

		err = awaitPromptly(t, batch.Close)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("canceled during read", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())

		batch, err := batchRunner(t, fx.dir).StartCatFileBatch(ctx)
		require.NoError(t, err)

		// Whether the kill lands before git has flushed the rest of the
		// object is up to the scheduler. Only the outcome is pinned. The
		// read fails even when every byte arrived.
		err = awaitPromptly(t, func() error {
			return batch.ReadBlob(fx.hashes["large.bin"], &cancelOnWrite{cancel: cancel})
		})
		require.ErrorIs(t, err, context.Canceled)

		err = awaitPromptly(t, batch.Close)
		require.ErrorIs(t, err, context.Canceled)
	})
}

// cancelOnWrite cancels the context on its first write, while the rest of
// the object is still queued behind it, and then keeps accepting data.
type cancelOnWrite struct {
	cancel context.CancelFunc
}

func (w *cancelOnWrite) Write(p []byte) (int, error) {
	w.cancel()

	return len(p), nil
}

// failOnWrite refuses every write with errSinkFailed.
type failOnWrite struct{}

func (failOnWrite) Write([]byte) (int, error) {
	return 0, errSinkFailed
}

// shortWrite accepts half of every write and reports no error, which
// io.Copy turns into [io.ErrShortWrite].
type shortWrite struct{}

func (shortWrite) Write(p []byte) (int, error) {
	return len(p) / 2, nil
}

// awaitPromptly runs fn on its own goroutine and fails the test if it has
// not returned within batchPromptness.
func awaitPromptly(t *testing.T, fn func() error) error {
	t.Helper()

	done := make(chan error, 1)

	go func() { done <- fn() }()

	select {
	case err := <-done:
		return err
	case <-time.After(batchPromptness):
		t.Fatal("call did not return after the context was canceled")

		return nil
	}
}

// batchRunner returns a runner bound to dir.
func batchRunner(t *testing.T, dir string) *git.GitRunner {
	t.Helper()

	runner, err := git.NewGitRunner(venv.OSVenv())
	require.NoError(t, err)

	return runner.WithWorkDir(dir)
}

// newBatchFixture commits blobs that stress the response framing (embedded
// newlines and NUL bytes, an empty blob, one larger than any buffer on the
// path, two whose object ids share a prefix) and clones them into a bare
// repository.
func newBatchFixture(t *testing.T) *batchFixture {
	t.Helper()

	ctx := t.Context()

	large := make([]byte, batchLargeBlobSize)
	for i := range large {
		large[i] = byte(i*7 + i/251)
	}

	first, second, prefix := ambiguousBlobPair()

	files := map[string][]byte{
		"text.txt":    []byte("hello\nworld\n"),
		"binary.bin":  []byte("\x00\n\x01\xff\n\n\x00end\n\x00"),
		"empty":       {},
		"large.bin":   large,
		"no-newline":  []byte(strings.Repeat("x", 100)),
		"ambiguous/a": first,
		"ambiguous/b": second,
	}

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	require.NoError(t, srv.CommitFiles(ctx, files, "batch fixture"))

	commitHash, err := srv.Head(ctx)
	require.NoError(t, err)

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	dir := helpers.TmpDirWOSymlinks(t)
	runner := batchRunner(t, dir)

	require.NoError(t, runner.Clone(ctx, url, true, 1, "main"))

	tree, err := runner.LsTreeRecursive(ctx, commitHash)
	require.NoError(t, err)

	hashes := make(map[string]string, len(files))
	for _, entry := range tree.Entries() {
		hashes[entry.Path] = entry.Hash
	}

	require.Len(t, hashes, len(files))

	// The pair comes from hashing locally, and git's own ids confirm it.
	require.Equal(t, prefix, hashes["ambiguous/a"][:ambiguousPrefixLen])
	require.Equal(t, prefix, hashes["ambiguous/b"][:ambiguousPrefixLen])

	return &batchFixture{
		files:           files,
		hashes:          hashes,
		dir:             dir,
		commitHash:      commitHash,
		ambiguousPrefix: prefix,
	}
}

// ambiguousBlobPair returns two blob contents whose git object ids share
// their first ambiguousPrefixLen hex digits, plus that prefix. The search
// is deterministic, so every run commits the same pair.
func ambiguousBlobPair() (first, second []byte, prefix string) {
	seen := make(map[string][]byte, ambiguousSearchBound)

	for i := range ambiguousSearchBound {
		content := fmt.Appendf(nil, "ambiguous %d\n", i)
		prefix := gitBlobID(content)[:ambiguousPrefixLen]

		if earlier, ok := seen[prefix]; ok {
			return earlier, content, prefix
		}

		seen[prefix] = content
	}

	panic("pigeonhole bound exhausted without a prefix collision")
}

// gitBlobID computes the SHA-1 object id git assigns to a blob with content.
func gitBlobID(content []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write(content)

	return hex.EncodeToString(h.Sum(nil))
}
