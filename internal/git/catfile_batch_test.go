//go:build !windows

// A fake git process lets these run in the default build, and its use of dup rules out windows.

package git_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBatchObject is the object every fake batch reply below names.
const fakeBatchObject = "aaaabeefcafefacedeadbeefcafefacedeadbeef"

// errFakeBatch is the failure the fakes below report.
var errFakeBatch = errors.New("fake batch failure")

func TestCatFileBatch_Responses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		request string
		reply   string
		want    string
	}{
		{
			name:    "blob",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob 5\nhello\n",
			want:    "hello",
		},
		{
			name:    "empty blob",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob 0\n\n",
		},
		{
			name:    "blob holding newlines",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob 5\na\n\nb\n\n",
			want:    "a\n\nb\n",
		},
		{
			name:    "abbreviated name resolves to the full id",
			request: fakeBatchObject[:8],
			reply:   fakeBatchObject + " blob 5\nhello\n",
			want:    "hello",
		},
		{
			name:    "missing object",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " missing\n",
			wantErr: git.ErrCatFileMissing,
		},
		{
			name:    "ambiguous name",
			request: fakeBatchObject[:4],
			reply:   fakeBatchObject[:4] + " ambiguous\n",
			wantErr: git.ErrCatFileAmbiguous,
		},
		{
			name:    "not a blob",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " commit 5\nhello\n",
			wantErr: git.ErrCatFileNotBlob,
		},
		{
			name:    "header naming another object",
			request: fakeBatchObject,
			reply:   "bbbbbeefcafefacedeadbeefcafefacedeadbeef blob 5\nhello\n",
			wantErr: git.ErrCatFileFraming,
		},
		{
			name:    "empty header",
			request: fakeBatchObject,
			reply:   "\n",
			wantErr: git.ErrCatFileFraming,
		},
		{
			name:    "unknown two field reply",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " unexpected\n",
			wantErr: git.ErrCatFileFraming,
		},
		{
			name:    "too many header fields",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob 5 extra\n",
			wantErr: git.ErrCatFileFraming,
		},
		{
			name:    "size is not a number",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob five\n",
			wantErr: git.ErrCatFileFraming,
		},
		{
			name:    "negative size",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob -1\n",
			wantErr: git.ErrCatFileFraming,
		},
		{
			name:    "header past the limit",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob " + strings.Repeat("1", 512) + "\n",
			wantErr: git.ErrCatFileFraming,
		},
		{
			name:    "header past the read buffer",
			request: fakeBatchObject,
			reply:   strings.Repeat("a", 8192),
			wantErr: git.ErrCatFileFraming,
		},
		{
			name:    "content not followed by a newline",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob 5\nhelloX",
			want:    "hello",
			wantErr: git.ErrCatFileFraming,
		},
		{
			name:    "stream ends before a header",
			request: fakeBatchObject,
			wantErr: git.ErrCatFileShortRead,
		},
		{
			name:    "stream ends inside a header",
			request: fakeBatchObject,
			reply:   fakeBatchObject,
			wantErr: git.ErrCatFileShortRead,
		},
		{
			name:    "stream ends inside the content",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob 5\nabc",
			want:    "abc",
			wantErr: git.ErrCatFileShortRead,
		},
		{
			name:    "stream ends before the closing newline",
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob 5\nhello",
			want:    "hello",
			wantErr: git.ErrCatFileShortRead,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			batch := startFakeBatch(t, t.Context(), &fakeBatchExec{replies: []string{tc.reply}})

			var got bytes.Buffer

			err := batch.ReadBlob(tc.request, &got)
			assert.Equal(t, tc.want, got.String())

			require.NoError(t, batch.Close())

			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}

			require.ErrorIs(t, err, tc.wantErr)

			var wrappedErr *git.WrappedError

			require.ErrorAs(t, err, &wrappedErr)
			assert.Equal(t, tc.request, wrappedErr.Context, "the failure should name the object that was requested")
		})
	}
}

// TestCatFileBatch_StaysInStepAfterRecoverableFailures pins that the next request still reads its own object.
func TestCatFileBatch_StaysInStepAfterRecoverableFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		sink    io.Writer
		wantErr error
		name    string
		request string
		reply   string
	}{
		{
			name:    "missing object",
			sink:    io.Discard,
			request: fakeBatchObject,
			reply:   fakeBatchObject + " missing\n",
			wantErr: git.ErrCatFileMissing,
		},
		{
			name:    "ambiguous name",
			sink:    io.Discard,
			request: fakeBatchObject[:4],
			reply:   fakeBatchObject[:4] + " ambiguous\n",
			wantErr: git.ErrCatFileAmbiguous,
		},
		{
			name:    "not a blob",
			sink:    io.Discard,
			request: fakeBatchObject,
			reply:   fakeBatchObject + " tree 5\nhello\n",
			wantErr: git.ErrCatFileNotBlob,
		},
		{
			name:    "writer failure",
			sink:    refusingWriter{},
			request: fakeBatchObject,
			reply:   fakeBatchObject + " blob 5\nhello\n",
			wantErr: errFakeBatch,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			batch := startFakeBatch(t, t.Context(), &fakeBatchExec{
				replies: []string{tc.reply, fakeBatchObject + " blob 3\nnew\n"},
			})

			require.ErrorIs(t, batch.ReadBlob(tc.request, tc.sink), tc.wantErr)

			var got bytes.Buffer

			require.NoError(t, batch.ReadBlob(fakeBatchObject, &got))
			assert.Equal(t, "new", got.String())

			require.NoError(t, batch.Close())
		})
	}
}

// TestCatFileBatch_ProcessGone pins that a request to a git that has already exited is a short read.
func TestCatFileBatch_ProcessGone(t *testing.T) {
	t.Parallel()

	batch := startFakeBatch(t, t.Context(), &fakeBatchExec{})

	err := batch.ReadBlob(fakeBatchObject, io.Discard)
	require.ErrorIs(t, err, git.ErrCatFileShortRead)

	require.NoError(t, batch.Close())
}

func TestCatFileBatch_CloseReportsExit(t *testing.T) {
	t.Parallel()

	batch := startFakeBatch(t, t.Context(), &fakeBatchExec{
		waitErr: errFakeBatch,
		stderr:  "fatal: not a git repository",
	})

	err := batch.Close()
	require.ErrorIs(t, err, git.ErrGitCommand)
	require.ErrorIs(t, err, errFakeBatch)

	var wrappedErr *git.WrappedError

	require.ErrorAs(t, err, &wrappedErr)
	assert.Equal(t, "fatal: not a git repository", wrappedErr.Context)
}

func TestCatFileBatch_StartFailure(t *testing.T) {
	t.Parallel()

	runner, err := git.NewGitRunner(venvtest.New().WithExec(&fakeBatchExec{startErr: errFakeBatch}))
	require.NoError(t, err)

	_, err = runner.WithWorkDir("/repo").StartCatFileBatch(t.Context())
	require.ErrorIs(t, err, git.ErrGitCommand)
	require.ErrorIs(t, err, errFakeBatch)
}

// TestCatFileBatch_CanceledContext pins that a canceled session never reads as a clean one, whatever git delivered.
func TestCatFileBatch_CanceledContext(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		reply   string
	}{
		{
			name:  "object delivered in full",
			reply: fakeBatchObject + " blob 5\nhello\n",
		},
		{
			name:    "object cut short",
			reply:   fakeBatchObject + " blob 5\nabc",
			wantErr: git.ErrCatFileShortRead,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())

			batch := startFakeBatch(t, ctx, &fakeBatchExec{replies: []string{tc.reply}})

			cancel()

			err := batch.ReadBlob(fakeBatchObject, io.Discard)
			require.ErrorIs(t, err, context.Canceled)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}

			require.ErrorIs(t, batch.Close(), context.Canceled)
		})
	}
}

// fakeBatchExec is a [vexec.Exec] answering `git cat-file --batch` requests from replies until they run out.
type fakeBatchExec struct {
	startErr error
	waitErr  error
	stderr   string
	replies  []string
}

// Command returns a process that answers from the replies.
func (e *fakeBatchExec) Command(ctx context.Context, name string, args ...string) vexec.Cmd {
	return &fakeBatchCmd{
		Cmd:  vexec.NewNoSpawnExec().Command(ctx, name, args...),
		exec: e,
		done: make(chan struct{}),
	}
}

// LookPath returns file unchanged.
func (e *fakeBatchExec) LookPath(file string) (string, error) {
	return file, nil
}

// fakeBatchCmd is one process of a [fakeBatchExec].
type fakeBatchCmd struct {
	vexec.Cmd
	exec   *fakeBatchExec
	stdin  *os.File
	stdout *os.File
	stderr io.Writer
	done   chan struct{}
}

func (c *fakeBatchCmd) SetStdin(r io.Reader) {
	c.stdin = r.(*os.File)
}

func (c *fakeBatchCmd) SetStdout(w io.Writer) {
	c.stdout = w.(*os.File)
}

func (c *fakeBatchCmd) SetStderr(w io.Writer) {
	c.stderr = w
}

// Start copies the pipe ends the way a child inherits them and answers requests until the replies run out.
func (c *fakeBatchCmd) Start() error {
	if c.exec.startErr != nil {
		return c.exec.startErr
	}

	// A process with no replies has already exited, so it holds no copies.
	if len(c.exec.replies) == 0 {
		close(c.done)

		return nil
	}

	stdin, err := inheritFile(c.stdin)
	if err != nil {
		return err
	}

	stdout, err := inheritFile(c.stdout)
	if err != nil {
		return errors.Join(err, stdin.Close())
	}

	go func() {
		defer close(c.done)
		defer stdin.Close()
		defer stdout.Close()

		requests := bufio.NewScanner(stdin)

		for _, reply := range c.exec.replies {
			if !requests.Scan() {
				return
			}

			if _, err := io.WriteString(stdout, reply); err != nil {
				return
			}
		}
	}()

	return nil
}

// Wait reports the exit once the process is done, with anything it said on stderr.
func (c *fakeBatchCmd) Wait() error {
	<-c.done

	if _, err := io.WriteString(c.stderr, c.exec.stderr); err != nil {
		return err
	}

	return c.exec.waitErr
}

// refusingWriter refuses every write with errFakeBatch.
type refusingWriter struct{}

func (refusingWriter) Write([]byte) (int, error) {
	return 0, errFakeBatch
}

// startFakeBatch starts a batch session against e, in a working directory that is never read.
func startFakeBatch(t *testing.T, ctx context.Context, e *fakeBatchExec) *git.CatFileBatch {
	t.Helper()

	runner, err := git.NewGitRunner(venvtest.New().WithExec(e))
	require.NoError(t, err)

	batch, err := runner.WithWorkDir("/repo").StartCatFileBatch(ctx)
	require.NoError(t, err)

	return batch
}

// inheritFile returns a descriptor of its own for f, which survives the parent closing f.
func inheritFile(f *os.File) (*os.File, error) {
	fd, err := syscall.Dup(int(f.Fd()))
	if err != nil {
		return nil, err
	}

	syscall.CloseOnExec(fd)

	return os.NewFile(uintptr(fd), f.Name()), nil
}
