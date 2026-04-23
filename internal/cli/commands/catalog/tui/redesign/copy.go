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
type copyResult struct { //nolint:govet // field order favors readability over GC-scan bytes
	references    ValuesReferences
	workingDir    string
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

	if err := copyDir(src, dst); err != nil {
		return err
	}

	c.result.workingDir = dst

	configName := configFileForKind(c.component.Kind)
	if configName == "" {
		return nil
	}

	refs, err := CollectValuesReferences(filepath.Join(src, configName))
	if err != nil {
		return err
	}

	if refs.IsEmpty() {
		return nil
	}

	c.result.references = refs

	written, err := WriteValuesStub(dst, refs)
	if err != nil {
		return err
	}

	c.result.valuesWritten = written
	c.result.valuesSkipped = !written

	return nil
}

// Result exposes the outcome of the last Run call. Intended for the TUI
// update loop to format an exit message; tests may use it too.
func (c *CopyCmd) Result() copyResult {
	return c.result
}

func (c *CopyCmd) SetStdin(io.Reader)  {}
func (c *CopyCmd) SetStdout(io.Writer) {}
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

func copyFile(src, dst string) error {
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
