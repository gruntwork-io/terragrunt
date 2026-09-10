package git

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"golang.org/x/sync/errgroup"
)

const (
	// archiveDirPerms are the permissions given to directories created while
	// extracting an archive. Git records no permissions for a tree, and this is
	// what it gives a directory it checks out itself.
	archiveDirPerms = 0o755

	// maxArchiveEntries bounds how many entries [ExtractArchive] reads from a
	// stream. Git's own limit on the size of a tree is far higher, but no
	// Terragrunt repository comes close to this many files.
	maxArchiveEntries = 1 << 20

	// maxArchiveDepth bounds how deeply an entry in an archive may nest.
	maxArchiveDepth = 64

	// archiveStreamThreshold is the size at which an entry is written straight
	// from the tar stream rather than buffered and handed to a worker. It caps
	// how much file content extraction holds in memory at once.
	archiveStreamThreshold = 1 << 20

	// grepNoMatchExitCode is the exit code `git grep` returns when the pattern
	// matched nothing. Any other non-zero exit is a failure to run.
	grepNoMatchExitCode = 1
)

// Archive errors.
var (
	// ErrArchiveEntryOutsideDest reports an archive entry whose path leaves the
	// directory it is being extracted into.
	ErrArchiveEntryOutsideDest = errors.New("archive entry resolves outside the destination directory")
	// ErrArchiveTooManyEntries reports an archive carrying more entries than
	// [maxArchiveEntries].
	ErrArchiveTooManyEntries = errors.New("archive holds too many entries")
	// ErrArchiveTooDeep reports an archive entry nested deeper than
	// [maxArchiveDepth].
	ErrArchiveTooDeep = errors.New("archive entry is nested too deeply")
)

// ArchiveTree writes the tree at ref to w as an uncompressed tar stream. Given
// pathspecs, it writes only the paths they name.
//
// The stream is written as git produces it, so w has to be read while the
// command runs. Pass the write half of an [io.Pipe] and read the other half
// from another goroutine, as [ExtractArchive] expects.
func (g *GitRunner) ArchiveTree(
	ctx context.Context,
	v *venv.Venv,
	ref string,
	w io.Writer,
	pathspecs ...string,
) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	// Run from the repository root: `git archive` limits the archive to the
	// directory it runs in, and callers may be working from a subdirectory.
	root, err := GoRepoRoot(ctx, v, g.WorkDir)
	if err != nil {
		return err
	}

	args := []string{"--format=tar", ref}
	if len(pathspecs) > 0 {
		args = append(args, "--")
		args = append(args, pathspecs...)
	}

	cmd := g.prepareCommand(ctx, "archive", args...)
	cmd.SetDir(root)

	var stderr bytes.Buffer

	cmd.SetStdout(w)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_archive",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return nil
}

// HasArchiveAlteringAttributes reports whether ref carries gitattributes that
// make `git archive` produce something other than what a checkout of the same
// ref would put on disk. `export-ignore` drops paths from the archive,
// `export-subst` rewrites file content, and a filter such as Git LFS leaves
// pointer files in the archive where a checkout would resolve them.
//
// A caller materializing a ref from an archive has to check this first and fall
// back to a checkout when it reports true.
func (g *GitRunner) HasArchiveAlteringAttributes(
	ctx context.Context,
	v *venv.Venv,
	ref string,
) (bool, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return false, err
	}

	// The pathspecs below are relative to the directory git runs in, so the
	// search runs from the repository root to cover every .gitattributes in the
	// tree rather than those under a caller's subdirectory.
	root, err := GoRepoRoot(ctx, v, g.WorkDir)
	if err != nil {
		return false, err
	}

	cmd := g.prepareCommand(
		ctx,
		"grep", "--quiet",
		"-e", "export-ignore",
		"-e", "export-subst",
		"-e", "filter=",
		ref,
		"--", ".gitattributes", "*.gitattributes",
	)
	cmd.SetDir(root)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		if vexec.ExitCode(err) == grepNoMatchExitCode {
			return false, nil
		}

		return false, &WrappedError{
			Op:      "git_grep_attributes",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return true, nil
}

// ReadTree populates the index of the repository at the runner's working
// directory from ref, without touching the files around it.
func (g *GitRunner) ReadTree(ctx context.Context, ref string) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	cmd := g.prepareCommand(ctx, "read-tree", ref)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_read_tree",
			Context: stderr.String(),
			Err:     errors.Join(ErrReadTree, err),
		}
	}

	return nil
}

// ExtractArchive writes the tar stream in r into dest, which must already
// exist. Entries are read in order, and file content is written by up to
// writers goroutines so extraction is not held to one write at a time.
//
// The stream is treated as untrusted: an entry naming a path outside dest, or
// nesting deeper than [maxArchiveDepth], ends extraction, as does a stream
// carrying more than [maxArchiveEntries] entries.
func ExtractArchive(
	ctx context.Context,
	v *venv.Venv,
	r io.Reader,
	dest string,
	writers int,
) error {
	v.RequireFS()

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(max(1, writers))

	// Directories are created here rather than by a worker so an entry never
	// races the directory it is written into.
	created := map[string]struct{}{dest: {}}

	mkdirAll := func(dir string) error {
		if _, ok := created[dir]; ok {
			return nil
		}

		created[dir] = struct{}{}

		return v.FS.MkdirAll(dir, archiveDirPerms)
	}

	readErr := readArchive(groupCtx, v, r, dest, mkdirAll, g)

	// Workers are waited on either way: on a read failure they are still
	// holding content to write, and the group has to drain before the
	// directory can be cleaned up.
	writeErr := g.Wait()

	return errors.Join(readErr, writeErr)
}

// readArchive reads every entry of a tar stream in order, creating directories
// and symlinks as it goes and handing file content to writes. It drains the
// stream on the way out so the process writing it is never left blocked.
func readArchive(
	ctx context.Context,
	v *venv.Venv,
	r io.Reader,
	dest string,
	mkdirAll func(string) error,
	writes *errgroup.Group,
) error {
	reader := tar.NewReader(r)

	for range maxArchiveEntries {
		header, err := reader.Next()

		if errors.Is(err, io.EOF) {
			// The archive ends before the stream does: git writes trailing
			// padding after the end-of-archive marker.
			_, err = io.Copy(io.Discard, r)

			return err
		}

		if err != nil {
			return fmt.Errorf("read archive entry for %s: %w", dest, err)
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		// Git leads its tar output with a global pax header carrying the commit
		// it archived. It describes the stream rather than naming a path.
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		target, err := archiveEntryPath(dest, header.Name)
		if err != nil {
			return err
		}

		if err := writeArchiveEntry(v, reader, header, dest, target, mkdirAll, writes); err != nil {
			return err
		}
	}

	return fmt.Errorf("%w: %s", ErrArchiveTooManyEntries, dest)
}

// writeArchiveEntry puts a single archive entry on disk. Content larger than
// [archiveStreamThreshold] is written straight from the stream, since buffering
// it would put the whole file in memory for a worker to pick up later.
func writeArchiveEntry(
	v *venv.Venv,
	reader io.Reader,
	header *tar.Header,
	dest, target string,
	mkdirAll func(string) error,
	writes *errgroup.Group,
) error {
	switch header.Typeflag {
	case tar.TypeDir:
		return mkdirAll(target)
	case tar.TypeSymlink:
		if err := mkdirAll(filepath.Dir(target)); err != nil {
			return err
		}

		if err := vfs.ValidateSymlinkTarget(dest, target, header.Linkname); err != nil {
			return err
		}

		return vfs.Symlink(v.FS, header.Linkname, target)
	default:
		if err := mkdirAll(filepath.Dir(target)); err != nil {
			return err
		}

		perm := header.FileInfo().Mode().Perm()

		if header.Size >= archiveStreamThreshold {
			return writeArchiveFile(v, reader, target, perm)
		}

		content := make([]byte, header.Size)
		if _, err := io.ReadFull(reader, content); err != nil {
			return fmt.Errorf("read archive entry %s: %w", header.Name, err)
		}

		writes.Go(func() error {
			return vfs.WriteFile(v.FS, target, content, perm)
		})

		return nil
	}
}

// writeArchiveFile copies one entry's content from the stream to disk.
func writeArchiveFile(v *venv.Venv, reader io.Reader, target string, perm fs.FileMode) (err error) {
	file, err := v.FS.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("create %s: %w", target, err)
	}

	defer func() {
		err = errors.Join(err, file.Close())
	}()

	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}

	return nil
}

// archiveEntryPath resolves an entry name against dest, rejecting a name that
// would land outside it or nest deeper than [maxArchiveDepth].
func archiveEntryPath(dest, name string) (string, error) {
	clean := path.Clean(strings.TrimPrefix(name, "./"))

	// An entry naming the root of the archive is the destination itself, which
	// the caller has already created.
	if clean == "." {
		return dest, nil
	}

	if strings.Count(clean, "/") >= maxArchiveDepth {
		return "", fmt.Errorf("%w: %s", ErrArchiveTooDeep, name)
	}

	local := filepath.FromSlash(clean)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("%w: %s", ErrArchiveEntryOutsideDest, name)
	}

	return filepath.Join(dest, local), nil
}
