// Filesystem helpers shared by the object-store getters (s3, gcs). They
// reimplement what go-getter's Request helpers do against the real OS
// filesystem, routed through the venv's vfs.FS instead.

package getter

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path"
	"path/filepath"
	"strings"

	getter "github.com/hashicorp/go-getter/v2"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// DefaultModeScanLimit caps how many keys Mode inspects while deciding between
// file and directory mode. A single S3 list page returns at most this many, so
// the default only trips on a prefix with a pathological number of siblings.
// Getters take a lower one through WithModeScanLimit.
const DefaultModeScanLimit = 1000

// ErrModeScanLimit is returned when a mode scan hits its limit without
// resolving a mode. Guessing one would either truncate a directory download to
// its first object or send a directory fetch at a single key, so the scan
// reports the prefix instead of picking.
var ErrModeScanLimit = errors.New("too many objects under prefix to determine getter mode")

// ErrObjectEscapesDst is returned when an object key resolves to a path
// outside the download's destination.
var ErrObjectEscapesDst = errors.New("object key escapes the destination directory")

// Permissions the object-store getters create with before the request's umask
// is applied. They match the values go-getter passes to its own copy helpers,
// so a download lands with the same mode it did before.
const (
	objectDirMode  os.FileMode = 0755
	objectFileMode os.FileMode = 0666
)

// ScanMode walks at most limit names and returns the first mode decide
// resolves. A listing that runs out first means nothing below the prefix
// looked like a directory, so the prefix names a file. decide reports false to
// keep scanning.
func ScanMode(
	limit int,
	names iter.Seq2[string, error],
	decide func(name string) (getter.Mode, bool),
) (getter.Mode, error) {
	var scanned int

	for name, err := range names {
		if err != nil {
			return 0, err
		}

		if mode, ok := decide(name); ok {
			return mode, nil
		}

		scanned++
		if scanned >= limit {
			return 0, ErrModeScanLimit
		}
	}

	return getter.ModeFile, nil
}

// ObjectMode reports the mode a listed name resolves for a requested object.
// A name only makes the object a directory by sitting below it: a prefix
// listing also returns siblings like `modules-old/main.tf`, and reading one of
// those as proof that `modules` is a directory would send the download at a
// prefix with nothing under it.
//
// An object written with a trailing slash already names a prefix, so the
// listing is measured against one separator either way.
func ObjectMode(name, object string) (getter.Mode, bool) {
	if strings.HasPrefix(name, ListPrefix(object)) {
		return getter.ModeDir, true
	}

	if name == object {
		return getter.ModeFile, true
	}

	return 0, false
}

// ObjectDst maps an object key to its destination path, keeping the key's
// layout below prefix. Keys are always forward-slashed, so the conversion to
// host separators happens here rather than by joining the raw key.
//
// A key is remote input, so one that climbs out of dst is refused.
func ObjectDst(dst, prefix, key string) (string, error) {
	rel := cmp.Or(strings.TrimPrefix(strings.TrimPrefix(key, prefix), "/"), path.Base(key))

	local := filepath.FromSlash(rel)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("%w: %q", ErrObjectEscapesDst, key)
	}

	return filepath.Join(dst, local), nil
}

// ResetGetterDst clears a directory download's destination. Get is documented
// as always fetching the latest, so a stale tree left from a previous run
// would otherwise merge into the new one.
//
// A prefix holding nothing but directory placeholders writes no files, and the
// caller is still owed the directory it asked for.
func ResetGetterDst(fsys vfs.FS, req *getter.Request) error {
	exists, err := vfs.FileExists(fsys, req.Dst)
	if err != nil {
		return err
	}

	if exists {
		if err := fsys.RemoveAll(req.Dst); err != nil {
			return err
		}
	}

	return fsys.MkdirAll(req.Dst, req.Mode(objectDirMode))
}

// WriteGetterObject copies body to dst, creating parent directories, and
// closes body. The explicit Chmod matches go-getter: creating the file only
// applies the process umask, which may differ from the request's.
func WriteGetterObject(
	fsys vfs.FS,
	req *getter.Request,
	dst string,
	body io.ReadCloser,
) (retErr error) {
	defer func() {
		if err := body.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	if err := fsys.MkdirAll(filepath.Dir(dst), req.Mode(objectDirMode)); err != nil {
		return err
	}

	f, err := fsys.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, objectFileMode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, body); err != nil {
		return errors.Join(err, f.Close())
	}

	if err := f.Close(); err != nil {
		return err
	}

	return fsys.Chmod(dst, req.Mode(objectFileMode))
}

// ListPrefix returns the listing prefix for a directory download. Get is only
// reached once Mode has decided the key names a directory, which it does by
// finding keys below `<key>/`.
func ListPrefix(key string) string {
	return strings.TrimSuffix(key, "/") + "/"
}

// CloseOnSuccess closes c, reporting its error through retErr only when the
// operation otherwise succeeded. On failure the primary error already explains
// the outcome, and joining a close error would bury it.
func CloseOnSuccess(c io.Closer, retErr *error) {
	if err := c.Close(); err != nil && *retErr == nil {
		*retErr = err
	}
}
