package redesign

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// CopyCmd is a tea.ExecCommand that copies a unit or stack component's
// directory tree into the user's working directory. Unlike scaffold, it does
// not generate a new terragrunt.hcl; it materializes the component's files
// so the user can edit them in place.
type CopyCmd struct {
	component *Component
	opts      *options.TerragruntOptions
	logger    log.Logger
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

func (c *CopyCmd) Run() error {
	src, dst, err := c.resolvePaths()
	if err != nil {
		return err
	}

	c.logger.Debugf("Copying component %q to %q", src, dst)

	// Preflight: refuse before writing anything if any target file would
	// collide with something already in the working directory. Without this,
	// a mid-walk collision could leave the working tree in a half-copied
	// state.
	if err := preflightCopy(src, dst); err != nil {
		return err
	}

	configName := configFileForKind(c.component.Kind)

	var (
		refs    ValuesReferences
		hasRefs bool
	)

	if configName != "" {
		refs, err = CollectValuesReferences(filepath.Join(src, configName))
		if err != nil {
			return err
		}

		hasRefs = !refs.IsEmpty()

		// Also preflight the values stub destination so we can fail before
		// copying when a stub would be written but the destination has an
		// unrelated obstruction (e.g. it exists as a directory).
		if hasRefs {
			if err := preflightValuesStub(dst); err != nil {
				return err
			}
		}
	}

	if err := copyDir(src, dst); err != nil {
		return err
	}

	result := copyResult{workingDir: dst}

	if hasRefs {
		result.references = refs

		written, err := WriteValuesStub(dst, refs)
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
		return "", "", errors.New("CopyCmd: nil component or repo")
	}

	repoPath := c.component.Repo.Path()
	if repoPath == "" {
		return "", "", errors.New("CopyCmd: empty repo path")
	}

	src := repoPath
	if c.component.Dir != "" {
		src = filepath.Join(repoPath, filepath.FromSlash(c.component.Dir))
	}

	workingDir := c.opts.WorkingDir
	if workingDir == "" {
		return "", "", errors.New("CopyCmd: empty working directory")
	}

	return src, workingDir, nil
}

// skipDuringCopy reports whether a directory name should be excluded from the
// copied tree. These directories hold regenerated artifacts and must not be
// carried into the user's working tree.
func skipDuringCopy(name string) bool {
	return name == ".terragrunt-cache" || name == ".terragrunt-stack"
}

// copyDir recursively copies src to dst, preserving file modes and skipping
// regenerated artifact directories.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return errors.New(err)
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			if path != src && skipDuringCopy(d.Name()) {
				return filepath.SkipDir
			}

			info, err := d.Info()
			if err != nil {
				return errors.New(err)
			}

			return os.MkdirAll(target, info.Mode().Perm())
		}

		// Skip symlinks and irregular files; copy only regular files.
		if !d.Type().IsRegular() {
			return nil
		}

		return copyFile(path, target)
	})
}

// preflightCopy walks src and returns an error if any non-skipped regular
// file would land on a path that already exists in dst. This makes the copy
// step all-or-nothing for the common collision case, so a half-populated
// working directory cannot result from a mid-walk conflict.
func preflightCopy(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
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
			return errors.New(err)
		}

		target := filepath.Join(dst, rel)
		if _, err := os.Lstat(target); err == nil {
			return errors.Errorf("destination %q already exists; refusing to overwrite", target)
		} else if !os.IsNotExist(err) {
			return errors.New(err)
		}

		return nil
	})
}

// preflightValuesStub returns an error if WriteValuesStub would fail at the
// stub destination for any reason other than a pre-existing values file
// (which it intentionally leaves alone).
func preflightValuesStub(dst string) error {
	stub := filepath.Join(dst, valuesFileName)

	info, err := os.Lstat(stub)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return errors.New(err)
	}

	// A regular file at the stub path is fine; WriteValuesStub will leave
	// it alone. Anything else (directory, symlink, irregular) blocks us.
	if info.Mode().IsRegular() {
		return nil
	}

	return errors.Errorf("destination %q is not a regular file; refusing to overwrite", stub)
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return errors.New(err)
	}

	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = errors.New(cerr)
		}
	}()

	info, err := in.Stat()
	if err != nil {
		return errors.New(err)
	}

	// O_EXCL ensures we refuse to overwrite existing files in the working
	// directory, so copying a unit or stack can't silently clobber user edits.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		if os.IsExist(err) {
			return errors.Errorf("destination %q already exists; refusing to overwrite", dst)
		}

		return errors.New(err)
	}

	if _, err := io.Copy(out, in); err != nil {
		if cerr := out.Close(); cerr != nil {
			return errors.New(cerr)
		}

		return errors.New(err)
	}

	if err := out.Close(); err != nil {
		return errors.New(err)
	}

	return nil
}
