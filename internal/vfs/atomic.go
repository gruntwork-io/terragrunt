package vfs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

const (
	// atomicTempPattern names the scratch file after its destination so one left
	// behind is traceable, with the random component [CreateTemp] substitutes for "*".
	atomicTempPattern = ".*.tmp"

	// maxTempNameLen is the shortest per-component name limit Terragrunt can
	// land on, NAME_MAX on Linux and macOS. A filesystem allowing more costs
	// only a shorter scratch name than it had to have.
	maxTempNameLen = 255

	// maxTempRandomLen bounds the decimal random component [CreateTemp]
	// substitutes for "*".
	maxTempRandomLen = 10

	// maxTempBaseLen is what a scratch name has left over for the part naming
	// its destination, once the pattern's literal bytes and the random
	// component are accounted for.
	maxTempBaseLen = maxTempNameLen - (len(atomicTempPattern) - 1) - maxTempRandomLen
)

// WriteFileAtomic writes data to path through a scratch file that replaces path
// only once the content is on disk. See [StreamFileAtomic] for what that buys and
// how perm is applied.
func WriteFileAtomic(fsys FS, path string, data []byte, perm os.FileMode) error {
	return StreamFileAtomic(fsys, path, perm, func(w io.Writer) error {
		_, err := w.Write(data)

		return err
	})
}

// StreamFileAtomic passes a writer to write and renames what it produces onto
// path, creating path's parent directories if they are missing. The scratch file
// shares path's directory so the rename stays on one filesystem.
//
// Three properties come out of this that a plain [WriteFile] does not have. A
// reader never sees a half-written file, and a failed write leaves the previous
// one untouched rather than truncated. A symlink at path is replaced rather than
// followed, so the content lands where the caller named. An existing file takes
// perm rather than keeping the mode it already had, which a create cannot do.
//
// perm is applied exactly, not masked by the process umask, so a caller asking
// for a mode wider than 0600 gets it whatever the umask would have allowed.
func StreamFileAtomic(fsys FS, path string, perm os.FileMode, write func(w io.Writer) error) error {
	dir := filepath.Dir(path)
	if err := fsys.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	file, err := CreateTemp(fsys, dir, TempPattern(filepath.Base(path)))
	if err != nil {
		return err
	}

	tmpPath := file.Name()

	if err := fsys.Chmod(tmpPath, perm); err != nil {
		return errors.Join(err, file.Close(), fsys.Remove(tmpPath))
	}

	if err := write(file); err != nil {
		return errors.Join(err, file.Close(), fsys.Remove(tmpPath))
	}

	if err := file.Close(); err != nil {
		return errors.Join(err, fsys.Remove(tmpPath))
	}

	if err := fsys.Rename(tmpPath, path); err != nil {
		return errors.Join(err, fsys.Remove(tmpPath))
	}

	return nil
}

// TempPattern builds a [CreateTemp] pattern for a scratch file that sits beside
// a file named base, so one left behind still names what it was written for.
//
// base is trimmed as far as it must be for the name CreateTemp expands the
// pattern into to stay within maxTempNameLen. base can sit at that limit
// itself, and its scratch file would then be unnameable in the very directory
// that accepted it.
func TempPattern(base string) string {
	if len(base) > maxTempBaseLen {
		base = base[:maxTempBaseLen]

		// macOS rejects a name holding a partial encoding outright (EILSEQ),
		// so give back the bytes a cut through the middle of a rune split.
		for len(base) > 0 {
			if r, size := utf8.DecodeLastRuneInString(base); r != utf8.RuneError || size > 1 {
				break
			}

			base = base[:len(base)-1]
		}
	}

	return base + atomicTempPattern
}
