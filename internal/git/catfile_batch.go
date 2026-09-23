package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
)

const (
	// catFileBatchHeaderLimit bounds one `git cat-file --batch` header line.
	// A SHA-256 object id, a type name, and a decimal size fit well inside
	// it. A longer line means the stream has drifted from the request.
	catFileBatchHeaderLimit = 256

	// catFileBatchHeaderFields is the field count of the header for an
	// object git found: <oid> SP <type> SP <size>.
	catFileBatchHeaderFields = 3

	// catFileBatchNoObjectFields is the field count of the reply for a name
	// git could not serve an object for: <name> SP <reason>.
	catFileBatchNoObjectFields = 2

	// catFileBatchWaitDelay bounds how long Wait blocks on a git process
	// that outlives its canceled context, before exec kills it outright.
	catFileBatchWaitDelay = 5 * time.Second

	catFileBatchOp = "git_cat_file_batch"

	// catFileBatchMissing is the reason git reports for a name it knows no
	// object by.
	catFileBatchMissing = "missing"

	// catFileBatchAmbiguous is the reason git reports for a name that
	// abbreviates more than one object.
	catFileBatchAmbiguous = "ambiguous"
)

// CatFileBatch is a long-lived `git cat-file --batch` process that serves
// many object reads without forking git for each one. Object names go to
// the process one per line, and answers come back on stdout in the same
// order.
//
// A batch serves one goroutine at a time. [CatFileBatch.ReadBlob] treats a
// request and its response as one unit on the shared pipes, so concurrent
// calls would interleave frames.
//
// Callers must call [CatFileBatch.Close] exactly once, whatever the outcome
// of the reads, so the process is reaped.
type CatFileBatch struct {
	// ctx is the context the process was started under. Canceling it kills
	// git, and the death arrives here as an ordinary EOF. Reads and Close
	// consult ctx so that a canceled session is not read as a clean one.
	ctx context.Context

	// cmd is the running git process, reaped by Close.
	cmd vexec.Cmd

	// stdin is where object names go, one per line. Closing it asks git to
	// exit, so only Close does that.
	stdin *os.File

	// stdoutR is the read end of git's stdout. Close needs the descriptor
	// itself, which a bufio.Reader cannot give it.
	stdoutR *os.File

	// stdout is where every response is read. One buffer fill can cover a
	// header line and a small object together, and bytes already pulled in
	// cannot be reached through stdoutR.
	stdout *bufio.Reader

	// stderr collects what git says about a request it could not serve. A
	// failed read and a non-zero exit both report it.
	stderr *bytes.Buffer
}

// StartCatFileBatch spawns `git cat-file --batch` against the configured
// working-directory repository. The process lives until
// [CatFileBatch.Close]. Canceling ctx kills it and fails any read in flight
// with an error wrapping ctx's error.
func (g *GitRunner) StartCatFileBatch(ctx context.Context) (*CatFileBatch, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	// These are OS pipes because exec hands an *os.File straight to the
	// child. Anything else gets a copier goroutine that Wait joins before
	// returning. The stdin copier would block in Read on this side once git
	// exits, with nothing left to release it, and Close would hang.
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		return nil, commandError(err, "")
	}

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return nil, commandError(errors.Join(err, stdinR.Close(), stdinW.Close()), "")
	}

	stderr := &bytes.Buffer{}

	cmd := g.prepareCommand(ctx, "cat-file", "--batch")
	cmd.SetStdin(stdinR)
	cmd.SetStdout(stdoutW)
	cmd.SetStderr(stderr)
	cmd.SetWaitDelay(catFileBatchWaitDelay)

	startErr := cmd.Start()

	// The child has its own copies of the ends it was handed. Closing ours
	// makes git's exit arrive here as EOF on stdout and EPIPE on stdin.
	childEndsErr := errors.Join(stdinR.Close(), stdoutW.Close())

	if startErr != nil {
		err := errors.Join(startErr, childEndsErr, stdinW.Close(), stdoutR.Close())

		return nil, commandError(err, stderr.String())
	}

	b := &CatFileBatch{
		ctx:     ctx,
		cmd:     cmd,
		stdin:   stdinW,
		stdoutR: stdoutR,
		stdout:  bufio.NewReader(stdoutR),
		stderr:  stderr,
	}

	if childEndsErr != nil {
		return nil, errors.Join(childEndsErr, b.Close())
	}

	return b, nil
}

// ReadBlob streams the content of the blob named by hash to w. It reports:
//
//   - [ErrCatFileMissing], for a name git does not know.
//   - [ErrCatFileAmbiguous], for an abbreviated name matching several
//     objects.
//   - [ErrCatFileNotBlob], for an object that is not a blob.
//   - w's own error, when w fails partway through the object.
//   - [ErrCatFileFraming], for a response that breaks git's framing or
//     names another object.
//   - [ErrCatFileShortRead], for a stream that ends early because git died
//     or ctx was canceled.
//
// The first four leave the batch usable. Whatever content the response
// includes is consumed before the error comes back, so the stream stays in
// step for the next request. The last two leave the batch out of step, and
// it should be closed.
func (b *CatFileBatch) ReadBlob(hash string, w io.Writer) error {
	if _, err := io.WriteString(b.stdin, hash+"\n"); err != nil {
		return b.failure(hash, errors.Join(ErrCatFileShortRead, err))
	}

	objType, size, err := b.readHeader(hash)
	if err != nil {
		return b.failure(hash, err)
	}

	dst := w
	if objType != EntryTypeBlob {
		dst = io.Discard
	}

	if err := b.readContent(dst, size); err != nil {
		return b.failure(hash, err)
	}

	if objType != EntryTypeBlob {
		return b.failure(hash, ErrCatFileNotBlob)
	}

	// Cancellation races the kill signal. git may have flushed the whole
	// object before it died, but the process is gone either way, so the
	// read has to be reported as failed.
	if err := b.ctx.Err(); err != nil {
		return b.failure(hash, err)
	}

	return nil
}

// Close ends the session and reaps the process. Callers must call it
// exactly once.
//
// A non-zero exit comes back as a [*WrappedError] with git's stderr as
// context. A canceled ctx is reported the same way even when git managed to
// exit cleanly first, since the session it was given is over.
func (b *CatFileBatch) Close() error {
	// git exits once its stdin reaches EOF.
	err := b.stdin.Close()

	// A frame left half-read by a failed ReadBlob is still queued in the
	// pipe. Draining it cannot hang, since git is already on its way out,
	// and the copy ends when the pipe closes.
	if _, drainErr := io.Copy(io.Discard, b.stdoutR); drainErr != nil {
		err = errors.Join(err, drainErr)
	}

	err = errors.Join(err, b.cmd.Wait(), b.stdoutR.Close(), b.ctx.Err())
	if err == nil {
		return nil
	}

	return commandError(err, b.stderr.String())
}

// readHeader parses the header line of the next response into the object's
// type and size. git frames a response as that header line, the object
// content, and a trailing newline. A header naming any object other than
// hash is refused as [ErrCatFileFraming].
func (b *CatFileBatch) readHeader(hash string) (objType string, size int64, err error) {
	line, err := b.readHeaderLine()
	if err != nil {
		return "", 0, err
	}

	fields := strings.Fields(line)

	// git answers with the name it resolved, so a header naming anything else
	// belongs to another request and the stream has drifted. An abbreviation
	// resolves to the full object id, which still starts with what was sent.
	if len(fields) == 0 || !strings.HasPrefix(fields[0], hash) {
		return "", 0, framingError(line)
	}

	// Neither reply includes content, so returning here leaves the stream in
	// step for the next request.
	if len(fields) == catFileBatchNoObjectFields {
		switch fields[1] {
		case catFileBatchMissing:
			return "", 0, ErrCatFileMissing
		case catFileBatchAmbiguous:
			return "", 0, ErrCatFileAmbiguous
		}
	}

	if len(fields) != catFileBatchHeaderFields {
		return "", 0, framingError(line)
	}

	size, err = strconv.ParseInt(fields[2], 10, 64)
	if err != nil || size < 0 {
		return "", 0, framingError(line)
	}

	return fields[1], size, nil
}

// readHeaderLine reads up to the newline ending a header, refusing lines
// longer than [catFileBatchHeaderLimit].
func (b *CatFileBatch) readHeaderLine() (string, error) {
	line, err := b.stdout.ReadSlice('\n')
	if err != nil {
		// A whole buffer with no newline in it is already past the limit.
		if errors.Is(err, bufio.ErrBufferFull) {
			return "", framingError(string(line[:min(len(line), catFileBatchHeaderLimit)]))
		}

		return "", errors.Join(ErrCatFileShortRead, err)
	}

	// ReadSlice stops at the buffer rather than at the limit, so an overlong
	// header is read further than it is accepted. Both outcomes end the
	// batch, so only the threshold has to be exact.
	if len(line) > catFileBatchHeaderLimit {
		return "", framingError(string(line[:catFileBatchHeaderLimit]))
	}

	return string(line[:len(line)-1]), nil
}

// readContent copies exactly size bytes of object content to dst, then
// finishes the frame.
func (b *CatFileBatch) readContent(dst io.Writer, size int64) error {
	frame := &io.LimitedReader{R: b.stdout, N: size}

	if _, err := io.Copy(dst, frame); err != nil {
		// Finishing the frame is right whether dst or the pipe failed. A
		// dead pipe fails again and marks the batch out of step. A failed
		// dst leaves the stream intact for the next request.
		return errors.Join(err, b.finishFrame(frame))
	}

	return b.finishFrame(frame)
}

// finishFrame discards whatever remains of the frame, then consumes the
// newline git prints after every object. What remains comes from the
// limited reader, not from the count io.Copy reports. A dst that fails
// partway through a write has consumed bytes from the stream that never
// reached it.
func (b *CatFileBatch) finishFrame(frame *io.LimitedReader) error {
	if _, err := io.Copy(io.Discard, frame); err != nil {
		return errors.Join(ErrCatFileShortRead, err)
	}

	if frame.N > 0 {
		return errors.Join(ErrCatFileShortRead, io.ErrUnexpectedEOF)
	}

	c, err := b.stdout.ReadByte()
	if err != nil {
		return errors.Join(ErrCatFileShortRead, err)
	}

	if c != '\n' {
		return framingError("object content not followed by a newline")
	}

	return nil
}

// failure wraps a ReadBlob error for hash, adding the context's error when
// cancellation cut the stream short.
func (b *CatFileBatch) failure(hash string, err error) error {
	if ctxErr := b.ctx.Err(); ctxErr != nil && !errors.Is(err, ctxErr) {
		err = errors.Join(ctxErr, err)
	}

	return &WrappedError{
		Op:      catFileBatchOp,
		Context: hash,
		Err:     err,
	}
}

// commandError reports a failure to run or finish the process, with
// whatever git wrote to stderr as context.
func commandError(err error, stderr string) error {
	return &WrappedError{
		Op:      catFileBatchOp,
		Context: stderr,
		Err:     errors.Join(ErrCommandSpawn, err),
	}
}

// framingError quotes the offending line so control bytes from a corrupt
// stream cannot reach a terminal unescaped.
func framingError(line string) error {
	return fmt.Errorf("%w: %q", ErrCatFileFraming, line)
}
