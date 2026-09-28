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

// DestinationNotDirectoryError reports that the component carries a directory
// whose target in the working directory is already occupied by something that
// is not a directory. Match with errors.As.
type DestinationNotDirectoryError struct {
	Path string
}

func (e *DestinationNotDirectoryError) Error() string {
	return fmt.Sprintf("destination %q is not a directory; refusing to copy into it", e.Path)
}

// SymlinkEscapesRootError reports that a component carries a symbolic link
// resolving outside the tree it was fetched into. Following it would copy a
// file from the machine running Terragrunt into the working directory, so a
// catalog cannot use one to read its user's files. Match with errors.As.
type SymlinkEscapesRootError struct {
	Path   string
	Target string
}

func (e *SymlinkEscapesRootError) Error() string {
	return fmt.Sprintf(
		"symlink %q targets %q, which leads outside the component source; refusing to copy",
		e.Path,
		e.Target,
	)
}

// Paths locates a scaffold. Root is the tree the component was fetched into:
// the cloned catalog for the catalog user interface, the download directory
// for `scaffold`. Src is the component within it, which is Root itself when
// the component was fetched on its own. Dst is the working directory its
// files land in.
type Paths struct {
	Root string
	Src  string
	Dst  string
}

// Result records what scaffolding a component did beyond copying its files, so
// a caller can tell the user which values still need filling in.
type Result struct {
	Dir           string
	References    ValuesReferences
	ValuesWritten bool
	ValuesSkipped bool
}

// Scaffold materializes the component of the given kind into the destination,
// alongside a terragrunt.values.hcl carrying the `values.*` references its
// configuration makes. A unit or a stack is already a Terragrunt
// configuration, so nothing is generated from it: its own files land in the
// destination for the user to edit in place.
//
// The copy is all-or-nothing. A file that would land on an existing path, a
// directory whose path is held by a non-directory, or a symbolic link leading
// out of the fetched tree aborts before anything is written, so neither a
// collision nor a rejected link can leave a half-populated destination.
//
// Since a catalog is someone else's repository, a symbolic link resolving
// outside [Paths.Root] is refused rather than followed.
//
// values supplies HCL fragments for the generated terragrunt.values.hcl,
// keyed by the bare reference name (`name`, not `values.name`). References
// with no entry fall back to `"TODO"` or to the reference's own try()
// fallback, as they do when nothing is supplied.
func Scaffold(
	fsys vfs.FS,
	kind Kind,
	paths Paths,
	values map[string]string,
) (Result, error) {
	if !kind.IsCopyable() {
		return Result{}, fmt.Errorf("%w: %s", ErrNotCopyable, kind)
	}

	if err := preflightCopy(fsys, paths); err != nil {
		return Result{}, err
	}

	refs, err := Values(fsys, kind, paths.Src)
	if err != nil {
		return Result{}, err
	}

	hasRefs := !refs.IsEmpty()

	if hasRefs {
		if err := preflightValuesStub(fsys, paths.Dst); err != nil {
			return Result{}, err
		}
	}

	if err := copyDir(fsys, paths); err != nil {
		return Result{}, err
	}

	result := Result{Dir: paths.Dst}

	if !hasRefs {
		return result, nil
	}

	result.References = refs

	written, err := WriteValuesFile(fsys, paths.Dst, refs, values)
	if err != nil {
		return Result{}, err
	}

	result.ValuesWritten = written
	result.ValuesSkipped = !written

	return result, nil
}

// Values reports the `values.*` references the component of the given kind at
// dir makes, which is what [Scaffold] writes its terragrunt.values.hcl from.
// A kind with no configuration file of its own makes none.
func Values(fsys vfs.FS, kind Kind, dir string) (ValuesReferences, error) {
	configFile := kind.ConfigFile()
	if configFile == "" {
		return ValuesReferences{}, nil
	}

	return CollectValuesReferences(fsys, filepath.Join(dir, configFile))
}

// skipDuringCopy reports whether a directory name should be excluded from the
// copied tree. These directories hold regenerated artifacts and must not be
// carried into the user's working tree.
func skipDuringCopy(name string) bool {
	return name == ".terragrunt-cache" || name == ".terragrunt-stack"
}

// copyDir recursively copies the component into the destination on fsys,
// preserving file modes and skipping regenerated artifact directories.
func copyDir(fsys vfs.FS, paths Paths) error {
	return vfs.WalkDir(fsys, paths.Src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(paths.Src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(paths.Dst, rel)

		if d.IsDir() {
			if path != paths.Src && skipDuringCopy(d.Name()) {
				return filepath.SkipDir
			}

			return mirrorDir(fsys, d, target)
		}

		if d.Type()&fs.ModeSymlink != 0 {
			return copySymlink(fsys, paths, path, target)
		}

		// Devices, sockets, and pipes have no meaning in a component.
		if !d.Type().IsRegular() {
			return nil
		}

		return copyFile(fsys, path, target)
	})
}

// mirrorDir creates target with the permissions of the directory d.
func mirrorDir(fsys vfs.FS, d fs.DirEntry, target string) error {
	info, err := d.Info()
	if err != nil {
		return err
	}

	return fsys.MkdirAll(target, info.Mode().Perm())
}

// copySymlink reproduces the link at path as target. A link resolving inside
// the component is recreated as a link, since the copy carries its target too.
// One resolving elsewhere under root has no target in the copy, so its content
// is materialized instead, which is what the module copy does with such a link.
func copySymlink(fsys vfs.FS, paths Paths, path, target string) error {
	insideComponent, err := checkSymlinkScope(fsys, paths, path)
	if err != nil {
		return err
	}

	if !insideComponent {
		return copyFile(fsys, path, target)
	}

	linkTarget, err := vfs.Readlink(fsys, path)
	if err != nil {
		return err
	}

	return vfs.Symlink(fsys, linkTarget, target)
}

// checkSymlinkScope validates the link at path and reports whether it lands
// inside the component. One leading out of root is rejected, so a catalog
// cannot use a link to read the files of whoever scaffolds it.
func checkSymlinkScope(fsys vfs.FS, paths Paths, path string) (bool, error) {
	linkTarget, err := vfs.Readlink(fsys, path)
	if err != nil {
		return false, err
	}

	if vfs.ValidateSymlinkTarget(paths.Root, path, linkTarget) != nil {
		return false, &SymlinkEscapesRootError{Path: path, Target: linkTarget}
	}

	// A link landing in the component is reproduced, not followed, and the
	// walk validates every other link in there, so such a chain cannot leave
	// root unnoticed. One landing elsewhere is materialized, and that read
	// follows the whole chain, so it is resolved below.
	if vfs.ValidateSymlinkTarget(paths.Src, path, linkTarget) == nil {
		return true, nil
	}

	// A chain that cannot be followed is reported as it is, so a link that
	// merely dangles inside the catalog is not blamed for escaping.
	if err := vfs.ValidateResolvedSymlinkTarget(fsys, paths.Root, path); err != nil {
		if errors.Is(err, vfs.ErrSymlinkEscapes) {
			return false, &SymlinkEscapesRootError{Path: path, Target: linkTarget}
		}

		return false, err
	}

	return false, nil
}

// claimsTarget reports whether copyDir writes anything for d. Devices,
// sockets, and pipes have no meaning in a component and are dropped, so they
// take up no path in the destination.
func claimsTarget(d fs.DirEntry) bool {
	return d.IsDir() || d.Type().IsRegular() || d.Type()&fs.ModeSymlink != 0
}

// preflightCopy walks the component and returns an error if any non-skipped
// entry would land on a destination path that cannot receive it, or carries a
// symbolic link leading out of the fetched tree. This makes the copy step
// all-or-nothing, so a half-populated working directory cannot result from a
// mid-walk conflict.
func preflightCopy(fsys vfs.FS, paths Paths) error {
	return vfs.WalkDir(fsys, paths.Src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() && path != paths.Src && skipDuringCopy(d.Name()) {
			return filepath.SkipDir
		}

		if !claimsTarget(d) {
			return nil
		}

		if d.Type()&fs.ModeSymlink != 0 {
			if _, err := checkSymlinkScope(fsys, paths, path); err != nil {
				return err
			}
		}

		rel, err := filepath.Rel(paths.Src, path)
		if err != nil {
			return err
		}

		return checkCopyTarget(fsys, d, filepath.Join(paths.Dst, rel))
	})
}

// checkCopyTarget reports whether target can receive the copied entry d. A
// directory may land on a directory that already exists, since copyDir merges
// into it; every other entry must land on a path that is still free.
func checkCopyTarget(fsys vfs.FS, d fs.DirEntry, target string) error {
	info, err := fsys.Stat(target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return err
	}

	if !d.IsDir() {
		return &DestinationExistsError{Path: target}
	}

	if !info.IsDir() {
		return &DestinationNotDirectoryError{Path: target}
	}

	return nil
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
