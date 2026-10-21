package getter

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gruntwork-io/terragrunt/internal/redact"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	getter "github.com/hashicorp/go-getter/v2"
)

// ErrSourceFileNotRegular is returned when the path of a single-file download
// from a repository is a directory, a symlink, or anything else that is not a
// regular file.
var ErrSourceFileNotRegular = errors.New("source file must be a regular file")

// The GOOS values the getters branch on.
const (
	goosWindows = "windows"
	goosDarwin  = "darwin"
)

const (
	// maxCommandOutput caps how much of a download command's output is kept
	// for its error message and parsing. The end of the output is kept.
	maxCommandOutput = 64 << 10

	// maxSourceErrorLen caps the length of a download error message, in runes.
	maxSourceErrorLen = 4096

	// sourceErrorHeadLen is how many runes from the start of an over-long
	// download error message are kept. The rest of [maxSourceErrorLen] comes
	// from its end.
	sourceErrorHeadLen = 1024

	// checkoutDirPerms is the mode of the parent directories created for a
	// single file copied out of a repository.
	checkoutDirPerms = 0o755
)

// sourceError is a download failure whose message has the credentials in the
// source address taken out.
type sourceError struct {
	err error
	msg string
}

func (e *sourceError) Error() string { return e.msg }

// sourceSpelling pairs one way a source address can appear in an error with
// the redacted rendering that replaces it.
type sourceSpelling struct {
	raw  string
	safe string
}

func (e *sourceError) Unwrap() error { return e.err }

// runDownloadCommand runs name with args through v's exec in dir and returns
// the last [maxCommandOutput] bytes of the command's combined output.
//
// It runs the command as one of Terragrunt's own rather than as a program
// from a configuration. An exec that restricts configured programs still runs
// it, and an exec that refuses all subprocesses refuses it.
func runDownloadCommand(ctx context.Context, v *venv.Venv, dir, name string, args ...string) (string, error) {
	cmd := v.Exec.Command(vexec.WithTrustedCommand(ctx), name, args...)
	cmd.SetEnv(venv.Environ(v.Env))

	if dir != "" {
		cmd.SetDir(dir)
	}

	out := &tailBuffer{limit: maxCommandOutput}

	cmd.SetStdout(out)
	cmd.SetStderr(out)

	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("running %s: %w: %s", name, err, out.String())
	}

	return out.String(), nil
}

// tailBuffer keeps the last limit bytes written to it, reporting every write
// as complete. When it has dropped earlier output, String starts with "…"
// and at a rune boundary.
type tailBuffer struct {
	buf     []byte
	limit   int
	dropped bool
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)

	if n > b.limit {
		p = p[n-b.limit:]
		b.dropped = true
	}

	b.buf = append(b.buf, p...)

	if extra := len(b.buf) - b.limit; extra > 0 {
		b.buf = b.buf[extra:]
		b.dropped = true
	}

	return n, nil
}

func (b *tailBuffer) String() string {
	if !b.dropped {
		return string(b.buf)
	}

	out := b.buf
	for i := 0; i < utf8.UTFMax-1 && len(out) > 0 && !utf8.RuneStart(out[0]); i++ {
		out = out[1:]
	}

	return "…" + string(out)
}

// redactSourceError returns err with each source address in raws replaced by
// its redacted rendering. A message over [maxSourceErrorLen] runes keeps its
// first [sourceErrorHeadLen] runes and its end. It returns nil for a nil err.
//
// It replaces each address as written and without its userinfo. git and hg
// print the address back in those forms. A credential printed in any other
// form is not caught.
func redactSourceError(err error, raws ...string) error {
	if err == nil {
		return nil
	}

	var spellings []sourceSpelling

	for _, raw := range raws {
		safe := redact.NewURL(raw).String()
		spellings = append(spellings, sourceSpelling{raw: raw, safe: safe})

		if u, parseErr := url.Parse(raw); parseErr == nil && u.User != nil {
			bare := *u
			bare.User = nil

			spellings = append(spellings, sourceSpelling{raw: bare.String(), safe: safe})
		}
	}

	slices.SortFunc(spellings, func(a, b sourceSpelling) int {
		return cmp.Compare(len(b.raw), len(a.raw))
	})

	replacements := []string{}
	for _, s := range spellings {
		replacements = append(replacements, s.raw, s.safe)
	}

	msg := strings.NewReplacer(replacements...).Replace(err.Error())

	if runes := []rune(msg); len(runes) > maxSourceErrorLen {
		tail := maxSourceErrorLen - sourceErrorHeadLen
		msg = string(runes[:sourceErrorHeadLen]) + "…" + string(runes[len(runes)-tail:])
	}

	return &sourceError{err: err, msg: msg}
}

// getFileFromCheckout serves GetFile for a getter that can only download
// whole repositories. It downloads the repository that contains the file at
// req's URL into a temporary directory with get, then copies that file to
// req.Dst.
//
// get receives req with its URL pointing at the file's directory and its
// destination at the temporary checkout. Both are put back before this
// returns.
func getFileFromCheckout(
	ctx context.Context,
	v *venv.Venv,
	req *getter.Request,
	get func(context.Context, *getter.Request) error,
) (err error) {
	tmp, err := vfs.MkdirTemp(v.FS, v.Platform.TempDir(), "terragrunt-getter-")
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, v.FS.RemoveAll(tmp))
	}()

	u := req.URL()
	origPath, origRawPath, dst := u.Path, u.RawPath, req.Dst
	filename := path.Base(origPath)
	checkout := filepath.Join(tmp, "checkout")

	u.Path, u.RawPath = path.Dir(origPath), ""
	req.Dst = checkout

	defer func() {
		u.Path, u.RawPath = origPath, origRawPath
		req.Dst = dst
	}()

	if err := get(ctx, req); err != nil {
		return err
	}

	return copyCheckoutFile(v.FS, checkout, filename, dst)
}

// copyCheckoutFile copies the regular file name, directly under checkout, to
// dst, creating dst's parent directories.
func copyCheckoutFile(fsys vfs.FS, checkout, name, dst string) error {
	if !filepath.IsLocal(name) {
		return fmt.Errorf("%w: %q", ErrSourceFileNotRegular, name)
	}

	src := filepath.Join(checkout, name)

	info, err := vfs.Lstat(fsys, src)
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrSourceFileNotRegular, name)
	}

	if err := fsys.MkdirAll(filepath.Dir(dst), checkoutDirPerms); err != nil {
		return err
	}

	return vfs.CopyFile(fsys, src, dst)
}

// removeIfCreated removes dst when existed reports it was not there before a
// failed download, so the failure leaves no empty destination behind.
func removeIfCreated(fsys vfs.FS, dst string, existed bool) error {
	if existed || dst == "" {
		return nil
	}

	if err := fsys.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return nil
}
