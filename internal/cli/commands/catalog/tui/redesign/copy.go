package redesign

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Sentinel errors returned by CopyCmd. Match with errors.Is.
var (
	ErrNilComponent    = errors.New("nil component or repo")
	ErrEmptyRepoPath   = errors.New("empty repo path")
	ErrEmptyWorkingDir = errors.New("empty working directory")
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

// CopyCmd is a tea.ExecCommand that copies a unit or stack component's
// directory tree into the user's working directory. Unlike scaffold, it does
// not generate a new terragrunt.hcl; it materializes the component's files
// so the user can edit them in place.
type CopyCmd struct {
	component *Component
	opts      *options.TerragruntOptions
	logger    log.Logger
	fsys      vfs.FS
	values    map[string]string
	result    copyResult
}

// copyResult records what the copy step did beyond the raw file copy, so the
// TUI can surface an appropriate exit message to the user.
type copyResult struct {
	workingDir    string
	references    ValuesReferences
	valuesWritten bool
	valuesSkipped bool
}

func NewCopyCmd(logger log.Logger, opts *options.TerragruntOptions, c *Component) *CopyCmd {
	return &CopyCmd{component: c, opts: opts, logger: logger}
}

// WithFS overrides the filesystem used for source reads and destination writes.
// When unset, Run uses vfs.NewOSFS().
func (c *CopyCmd) WithFS(fsys vfs.FS) *CopyCmd {
	c.fsys = fsys
	return c
}

// WithValues threads user-supplied HCL fragments into the generated
// terragrunt.values.hcl. The interactive scaffold form populates this map
// keyed by `values.<name>` reference; entries not in the map fall back to
// the same `"TODO"` / try-fallback behavior as the placeholder flow.
func (c *CopyCmd) WithValues(values map[string]string) *CopyCmd {
	c.values = values
	return c
}

func (c *CopyCmd) Run() error {
	fsys := c.fsys
	if fsys == nil {
		fsys = vfs.NewOSFS()
	}

	src, dst, err := c.resolvePaths()
	if err != nil {
		return err
	}

	c.logger.Debugf("Copying component %q to %q", src, dst)

	// Preflight: refuse before writing anything if any target file would
	// collide with something already in the working directory. Without this,
	// a mid-walk collision could leave the working tree in a half-copied
	// state.
	if err := preflightCopy(fsys, src, dst); err != nil {
		return err
	}

	configName := configFileForKind(c.component.Kind)

	var (
		refs    ValuesReferences
		hasRefs bool
	)

	if configName != "" {
		refs, err = CollectValuesReferences(fsys, filepath.Join(src, configName))
		if err != nil {
			return err
		}

		hasRefs = !refs.IsEmpty()

		// Also preflight the values stub destination so we can fail before
		// copying when a stub would be written but the destination has an
		// unrelated obstruction (e.g. it exists as a directory).
		if hasRefs {
			if err := preflightValuesStub(fsys, dst); err != nil {
				return err
			}
		}
	}

	if err := copyDir(fsys, src, dst); err != nil {
		return err
	}

	result := copyResult{workingDir: dst}

	if hasRefs {
		result.references = refs

		written, err := WriteValuesFile(fsys, dst, refs, c.values)
		if err != nil {
			return err
		}

		result.valuesWritten = written
		result.valuesSkipped = !written
	}

	c.result = result

	return nil
}

// Result exposes the outcome of the last Run call. Intended for the TUI
// update loop to format an exit message; tests may use it too.
func (c *CopyCmd) Result() copyResult {
	return c.result
}

// SetStdin is a no-op; CopyCmd does not interact with stdio and only
// implements this method to satisfy the tea.ExecCommand interface.
func (c *CopyCmd) SetStdin(io.Reader) {}

// SetStdout is a no-op; see SetStdin.
func (c *CopyCmd) SetStdout(io.Writer) {}

// SetStderr is a no-op; see SetStdin.
func (c *CopyCmd) SetStderr(io.Writer) {}

// resolvePaths returns the absolute source directory (inside the cloned repo)
// and the destination directory (the user's working directory) for this copy.
// Files from src are materialized directly into the working directory so the
// action mirrors how scaffold emits its output.
func (c *CopyCmd) resolvePaths() (string, string, error) {
	if c.component == nil || c.component.Repo == nil {
		return "", "", ErrNilComponent
	}

	repoPath := c.component.Repo.Path()
	if repoPath == "" {
		return "", "", ErrEmptyRepoPath
	}

	src := repoPath
	if c.component.Dir != "" {
		src = filepath.Join(repoPath, filepath.FromSlash(c.component.Dir))
	}

	workingDir := c.opts.WorkingDir
	if workingDir == "" {
		return "", "", ErrEmptyWorkingDir
	}

	return src, workingDir, nil
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
	stub := filepath.Join(dst, valuesFileName)

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
		if cerr := out.Close(); cerr != nil {
			return cerr
		}

		return err
	}

	if err := out.Close(); err != nil {
		return err
	}

	return nil
}
