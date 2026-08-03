package component

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// ErrNotCopyable is returned by [Scaffold] for a kind that is scaffolded by
// generating a configuration rather than by copying. Match with errors.Is.
var ErrNotCopyable = errors.New(
	"component kind is scaffolded by generating a configuration, not by copying",
)

// DestinationExistsError reports that a copy target collides with an
// existing file in the working directory. Match with errors.As to read
// the offending path.
type DestinationExistsError struct {
	Path string
}

func (e *DestinationExistsError) Error() string {
	return fmt.Sprintf("destination %q already exists; refusing to overwrite", e.Path)
}

// DestinationNotRegularError reports that a copy target exists but is
// not a regular file (e.g. a directory, symlink, or device node) and
// therefore cannot be safely overwritten. Match with errors.As.
type DestinationNotRegularError struct {
	Path string
}

func (e *DestinationNotRegularError) Error() string {
	return fmt.Sprintf("destination %q is not a regular file; refusing to overwrite", e.Path)
}

// Result records what scaffolding a component did beyond copying its files, so
// a caller can tell the user which values still need filling in.
type Result struct {
	Dir           string
	References    ValuesReferences
	ValuesWritten bool
	ValuesSkipped bool
}

// Scaffold materializes the component of the given kind at src into dst,
// alongside a terragrunt.values.hcl carrying the `values.*` references its
// configuration makes. A unit or a stack is already a Terragrunt
// configuration, so nothing is generated from it: its own files land in dst
// for the user to edit in place.
//
// The copy is all-or-nothing for the common case: a file that would land on
// an existing path aborts before anything is written, so a collision cannot
// leave a half-populated destination.
//
// values supplies HCL fragments for the generated terragrunt.values.hcl,
// keyed by the bare reference name (`name`, not `values.name`). References
// with no entry fall back to `"TODO"` or to the reference's own try()
// fallback, as they do when nothing is supplied.
func Scaffold(
	fsys vfs.FS,
	kind Kind,
	src, dst string,
	values map[string]string,
) (Result, error) {
	if !kind.IsCopyable() {
		return Result{}, fmt.Errorf("%w: %s", ErrNotCopyable, kind)
	}

	if err := preflightCopy(fsys, src, dst); err != nil {
		return Result{}, err
	}

	refs, err := CollectValuesReferences(fsys, filepath.Join(src, kind.ConfigFile()))
	if err != nil {
		return Result{}, err
	}

	hasRefs := !refs.IsEmpty()

	if hasRefs {
		if err := preflightValuesStub(fsys, dst); err != nil {
			return Result{}, err
		}
	}

	if err := copyDir(fsys, src, dst); err != nil {
		return Result{}, err
	}

	result := Result{Dir: dst}

	if !hasRefs {
		return result, nil
	}

	result.References = refs

	written, err := WriteValuesFile(fsys, dst, refs, values)
	if err != nil {
		return Result{}, err
	}

	result.ValuesWritten = written
	result.ValuesSkipped = !written

	return result, nil
}

// skipDuringCopy reports whether a directory name should be excluded from the
// copied tree. These directories hold regenerated artifacts and must not be
// carried into the user's working tree.
func skipDuringCopy(name string) bool {
	return name == ".terragrunt-cache" || name == ".terragrunt-stack"
}

// copyDir recursively copies src to dst on fsys, preserving file modes and
// skipping regenerated artifact directories.
func copyDir(fsys vfs.FS, src, dst string) error {
	return vfs.WalkDir(fsys, src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			if path != src && skipDuringCopy(d.Name()) {
				return filepath.SkipDir
			}

			info, err := d.Info()
			if err != nil {
				return err
			}

			return fsys.MkdirAll(target, info.Mode().Perm())
		}

		// Skip symlinks and irregular files; copy only regular files.
		if !d.Type().IsRegular() {
			return nil
		}

		return copyFile(fsys, path, target)
	})
}

// preflightCopy walks src and returns an error if any non-skipped regular
// file would land on a path that already exists in dst. This makes the copy
// step all-or-nothing for the common collision case, so a half-populated
// working directory cannot result from a mid-walk conflict.
func preflightCopy(fsys vfs.FS, src, dst string) error {
	return vfs.WalkDir(fsys, src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if path != src && skipDuringCopy(d.Name()) {
				return filepath.SkipDir
			}

			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		_, err = fsys.Stat(target)
		if err == nil {
			return &DestinationExistsError{Path: target}
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}

		return nil
	})
}

// preflightValuesStub returns an error if WriteValuesStub would fail at the
// stub destination for any reason other than a pre-existing values file
// (which it intentionally leaves alone).
func preflightValuesStub(fsys vfs.FS, dst string) error {
	stub := filepath.Join(dst, ValuesFileName)

	info, err := fsys.Stat(stub)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return err
	}

	// A regular file at the stub path is fine; WriteValuesStub will leave
	// it alone. Anything else (directory, symlink, irregular) blocks us.
	if info.Mode().IsRegular() {
		return nil
	}

	return &DestinationNotRegularError{Path: stub}
}

func copyFile(fsys vfs.FS, src, dst string) (err error) {
	in, err := fsys.Open(src)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	// O_EXCL ensures we refuse to overwrite existing files in the working
	// directory, so copying a unit or stack can't silently clobber user edits.
	out, err := fsys.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return &DestinationExistsError{Path: dst}
		}

		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		return errors.Join(err, out.Close())
	}

	return out.Close()
}
